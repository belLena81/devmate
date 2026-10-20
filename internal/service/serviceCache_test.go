package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"devmate/internal/domain"
)

type fakeCache struct {
	store    map[string]string
	setCalls int
	getCalls int
}

func newFakeCache() *fakeCache {
	return &fakeCache{store: make(map[string]string)}
}

func (f *fakeCache) Get(key string) (string, bool) {
	f.getCalls++
	v, ok := f.store[key]
	return v, ok
}

func (f *fakeCache) Set(key, value string) error {
	f.setCalls++
	f.store[key] = value
	return nil
}

func (f *fakeCache) Clear() error {
	f.store = make(map[string]string)
	return nil
}

// ─── commit ───────────────────────────────────────────────────────────────────

func TestDraftMessage_CacheMiss_CallsLLMAndStores(t *testing.T) {
	cache := newFakeCache()
	svc := Service{
		git:   &fakeGit{diff: "some diff"},
		llm:   &fakeLLM{response: "feat: add thing"},
		cache: cache,
		model: "test-model",
		log:   noopLogger(),
	}

	result, err := svc.DraftMessage(context.Background(), CommitOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "feat: add thing" {
		t.Errorf("unexpected result: %q", result)
	}
	if cache.setCalls != 1 {
		t.Errorf("expected 1 Set call on miss, got %d", cache.setCalls)
	}
}

func TestDraftMessage_CacheHit_DoesNotCallLLM(t *testing.T) {
	cache := newFakeCache()
	llmCalls := 0
	svc := Service{
		git:   &fakeGit{diff: "some diff"},
		llm:   &fakeLLM{response: "feat: cached", onGenerate: func(string) { llmCalls++ }},
		cache: cache,
		model: "test-model",
		log:   noopLogger(),
	}

	svc.DraftMessage(context.Background(), CommitOptions{})
	result, err := svc.DraftMessage(context.Background(), CommitOptions{})
	if err != nil {
		t.Fatalf("unexpected error on cache hit: %v", err)
	}
	if result != "feat: cached" {
		t.Errorf("expected cached value, got %q", result)
	}
	if llmCalls != 1 {
		t.Errorf("LLM should be called exactly once, got %d", llmCalls)
	}
}

func TestDraftMessage_LLMError_DoesNotCache(t *testing.T) {
	cache := newFakeCache()
	svc := Service{
		git:   &fakeGit{diff: "some diff"},
		llm:   &fakeLLM{err: errors.New("llm failed")},
		cache: cache,
		model: "test-model",
		log:   noopLogger(),
	}

	svc.DraftMessage(context.Background(), CommitOptions{})
	if cache.setCalls != 0 {
		t.Errorf("must not cache on LLM error, got %d Set calls", cache.setCalls)
	}
}

func TestDraftMessage_DifferentMode_IndependentCacheEntries(t *testing.T) {
	cache := newFakeCache()
	llmCalls := 0
	svc := Service{
		git:   &fakeGit{diff: "same diff"},
		llm:   &fakeLLM{response: "msg", onGenerate: func(string) { llmCalls++ }},
		cache: cache,
		model: "test-model",
		log:   noopLogger(),
	}

	svc.DraftMessage(context.Background(), CommitOptions{Options: domain.Options{Mode: domain.Short}})
	svc.DraftMessage(context.Background(), CommitOptions{Options: domain.Options{Mode: domain.Detailed}})

	if llmCalls != 2 {
		t.Errorf("different modes must have independent cache entries, LLM called %d times", llmCalls)
	}
}

func TestDraftMessage_DifferentDiff_IndependentCacheEntries(t *testing.T) {
	cache := newFakeCache()
	llmCalls := 0

	makeService := func(diff string) *Service {
		return &Service{
			git:   &fakeGit{diff: diff},
			llm:   &fakeLLM{response: "msg", onGenerate: func(string) { llmCalls++ }},
			cache: cache,
			model: "test-model",
			log:   noopLogger(),
		}
	}

	makeService("diff A").DraftMessage(context.Background(), CommitOptions{})
	makeService("diff B").DraftMessage(context.Background(), CommitOptions{})

	if llmCalls != 2 {
		t.Errorf("different diffs must have independent cache entries, LLM called %d times", llmCalls)
	}
}

// ─── branch ───────────────────────────────────────────────────────────────────

func TestDraftBranchName_CacheMiss_CallsLLMAndStores(t *testing.T) {
	cache := newFakeCache()
	svc := Service{
		llm:   &fakeLLM{response: "feat/add-auth"},
		cache: cache,
		model: "test-model",
		log:   noopLogger(),
	}

	result, err := svc.DraftBranchName(context.Background(), BranchOptions{Task: "add authentication"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != "feat/add-auth" {
		t.Errorf("unexpected result: %q", result)
	}
	if cache.setCalls != 1 {
		t.Errorf("expected 1 Set call, got %d", cache.setCalls)
	}
}

func TestDraftBranchName_CacheHit_DoesNotCallLLM(t *testing.T) {
	cache := newFakeCache()
	llmCalls := 0
	svc := Service{
		llm:   &fakeLLM{response: "feat/add-auth", onGenerate: func(string) { llmCalls++ }},
		cache: cache,
		model: "test-model",
		log:   noopLogger(),
	}

	opts := BranchOptions{Task: "add authentication"}
	svc.DraftBranchName(context.Background(), opts)
	svc.DraftBranchName(context.Background(), opts)

	if llmCalls != 1 {
		t.Errorf("LLM should be called once, got %d", llmCalls)
	}
}

func TestDraftBranchName_LLMError_DoesNotCache(t *testing.T) {
	cache := newFakeCache()
	svc := Service{
		llm:   &fakeLLM{err: errors.New("llm failed")},
		cache: cache,
		model: "test-model",
		log:   noopLogger(),
	}

	svc.DraftBranchName(context.Background(), BranchOptions{Task: "some task"})
	if cache.setCalls != 0 {
		t.Errorf("must not cache on LLM error, got %d Set calls", cache.setCalls)
	}
}

// ─── pr ───────────────────────────────────────────────────────────────────────

func TestDraftPrDescription_CacheMiss_CallsLLMAndStores(t *testing.T) {
	cache := newFakeCache()
	svc := Service{
		git:   &fakeGit{commits: []string{"feat: add login"}},
		llm:   &fakeLLM{response: "PR description"},
		cache: cache,
		model: "test-model",
		log:   noopLogger(),
	}

	_, err := svc.DraftPrDescription(context.Background(), PrOptions{SourceBranch: "feature/x", DestinationBranch: "main"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cache.setCalls != 1 {
		t.Errorf("expected 1 Set call, got %d", cache.setCalls)
	}
}

func TestDraftPrDescription_CacheHit_DoesNotCallLLM(t *testing.T) {
	cache := newFakeCache()
	llmCalls := 0
	svc := Service{
		git:   &fakeGit{commits: []string{"feat: add login"}},
		llm:   &fakeLLM{response: "PR desc", onGenerate: func(string) { llmCalls++ }},
		cache: cache,
		model: "test-model",
		log:   noopLogger(),
	}

	opts := PrOptions{SourceBranch: "feature/x", DestinationBranch: "main"}
	svc.DraftPrDescription(context.Background(), opts)
	svc.DraftPrDescription(context.Background(), opts)

	if llmCalls != 1 {
		t.Errorf("LLM should be called once, got %d", llmCalls)
	}
}

func TestDraftPrDescription_LLMError_DoesNotCache(t *testing.T) {
	cache := newFakeCache()
	svc := Service{
		git:   &fakeGit{commits: []string{"feat: something"}},
		llm:   &fakeLLM{err: errors.New("llm failed")},
		cache: cache,
		model: "test-model",
		log:   noopLogger(),
	}

	svc.DraftPrDescription(context.Background(), PrOptions{SourceBranch: "a", DestinationBranch: "b"})
	if cache.setCalls != 0 {
		t.Errorf("must not cache on LLM error, got %d Set calls", cache.setCalls)
	}
}

func TestDraftPrDescription_NewCommitAdded_CacheMiss(t *testing.T) {
	cache := newFakeCache()
	llmCalls := 0

	makeService := func(commits []string) *Service {
		return &Service{
			git:   &fakeGit{commits: commits},
			llm:   &fakeLLM{response: "pr", onGenerate: func(string) { llmCalls++ }},
			cache: cache,
			model: "test-model",
			log:   noopLogger(),
		}
	}

	makeService([]string{"feat: one", "feat: two"}).DraftPrDescription(context.Background(),
		PrOptions{SourceBranch: "feature/x", DestinationBranch: "main"},
	)
	makeService([]string{"feat: one", "feat: two", "feat: three"}).DraftPrDescription(context.Background(),
		PrOptions{SourceBranch: "feature/x", DestinationBranch: "main"},
	)

	if llmCalls != 2 {
		t.Errorf("new commit must invalidate cache, LLM called %d times (expected 2)", llmCalls)
	}
}

// ─── NoCache flag ─────────────────────────────────────────────────────────────

func TestDraftMessage_NoCache_SkipsCacheGet(t *testing.T) {
	cache := newFakeCache()
	llmCalls := 0
	svc := Service{
		git:   &fakeGit{diff: "some diff"},
		llm:   &fakeLLM{response: "feat: fresh", onGenerate: func(string) { llmCalls++ }},
		cache: cache,
		model: "test-model",
		log:   noopLogger(),
	}

	svc.DraftMessage(context.Background(), CommitOptions{})
	if llmCalls != 1 {
		t.Fatalf("expected 1 LLM call to prime cache, got %d", llmCalls)
	}

	svc.DraftMessage(context.Background(), CommitOptions{})
	if llmCalls != 1 {
		t.Fatalf("expected cache hit (still 1 LLM call), got %d", llmCalls)
	}

	svc.DraftMessage(context.Background(), CommitOptions{Options: domain.Options{NoCache: true}})
	if llmCalls != 2 {
		t.Errorf("NoCache must force a fresh LLM call, got %d total LLM calls (want 2)", llmCalls)
	}
}

func TestDraftMessage_NoCache_OverwritesCachedEntry(t *testing.T) {
	cache := newFakeCache()
	responses := []string{"feat: original", "feat: refreshed"}
	svc := Service{
		git:   &fakeGit{diff: "some diff"},
		llm:   &sequenceLLM{responses: responses},
		cache: cache,
		model: "test-model",
		log:   noopLogger(),
	}

	r1, _ := svc.DraftMessage(context.Background(), CommitOptions{})
	if r1 != "feat: original" {
		t.Fatalf("expected first response %q, got %q", "feat: original", r1)
	}

	r2, _ := svc.DraftMessage(context.Background(), CommitOptions{Options: domain.Options{NoCache: true}})
	if r2 != "feat: refreshed" {
		t.Fatalf("expected refreshed response %q, got %q", "feat: refreshed", r2)
	}

	r3, _ := svc.DraftMessage(context.Background(), CommitOptions{})
	if r3 != "feat: refreshed" {
		t.Errorf("cache should hold refreshed value %q, got %q", "feat: refreshed", r3)
	}
}

func TestDraftBranchName_NoCache_SkipsCacheGet(t *testing.T) {
	cache := newFakeCache()
	llmCalls := 0
	svc := Service{
		llm:   &fakeLLM{response: "feat/branch", onGenerate: func(string) { llmCalls++ }},
		cache: cache,
		model: "test-model",
		log:   noopLogger(),
	}

	opts := BranchOptions{Task: "add auth"}
	svc.DraftBranchName(context.Background(), opts)
	if llmCalls != 1 {
		t.Fatalf("expected 1 LLM call to prime cache, got %d", llmCalls)
	}

	svc.DraftBranchName(context.Background(), opts)
	if llmCalls != 1 {
		t.Fatalf("expected cache hit, got %d LLM calls", llmCalls)
	}

	noCacheOpts := BranchOptions{Task: "add auth", Options: domain.Options{NoCache: true}}
	svc.DraftBranchName(context.Background(), noCacheOpts)
	if llmCalls != 2 {
		t.Errorf("NoCache must force fresh LLM call, got %d total (want 2)", llmCalls)
	}
}

func TestDraftPrDescription_NoCache_SkipsCacheGet(t *testing.T) {
	cache := newFakeCache()
	llmCalls := 0
	svc := Service{
		git:   &fakeGit{commits: []string{"feat: thing"}},
		llm:   &fakeLLM{response: "PR desc", onGenerate: func(string) { llmCalls++ }},
		cache: cache,
		model: "test-model",
		log:   noopLogger(),
	}

	opts := PrOptions{SourceBranch: "feature/x", DestinationBranch: "main"}
	svc.DraftPrDescription(context.Background(), opts)
	if llmCalls != 1 {
		t.Fatalf("expected 1 LLM call to prime cache, got %d", llmCalls)
	}

	svc.DraftPrDescription(context.Background(), opts)
	if llmCalls != 1 {
		t.Fatalf("expected cache hit, got %d LLM calls", llmCalls)
	}

	noCacheOpts := PrOptions{
		SourceBranch:      "feature/x",
		DestinationBranch: "main",
		Options:           domain.Options{NoCache: true},
	}
	svc.DraftPrDescription(context.Background(), noCacheOpts)
	if llmCalls != 2 {
		t.Errorf("NoCache must force fresh LLM call, got %d total (want 2)", llmCalls)
	}
}

type sequenceLLM struct {
	mu        sync.Mutex
	responses []string
	idx       int
}

func (s *sequenceLLM) Generate(_ context.Context, _ string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.idx >= len(s.responses) {
		return "", errors.New("sequenceLLM: no more responses")
	}
	r := s.responses[s.idx]
	s.idx++
	return r, nil
}

func TestCache_CommitAndBranch_DoNotShareEntries(t *testing.T) {
	cache := newFakeCache()
	llmCalls := 0
	svc := Service{
		git:   &fakeGit{diff: "content"},
		llm:   &fakeLLM{response: "result", onGenerate: func(string) { llmCalls++ }},
		cache: cache,
		model: "test-model",
		log:   noopLogger(),
	}

	svc.DraftMessage(context.Background(), CommitOptions{})
	svc.DraftBranchName(context.Background(), BranchOptions{Task: "content"})

	if llmCalls != 2 {
		t.Errorf("commit and branch must have independent cache namespaces, LLM called %d times", llmCalls)
	}
}
