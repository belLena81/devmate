package service

import (
	"context"
	"errors"
	"sync"
	"testing"

	"devmate/internal/domain"
)

// fakeCache is an in-memory Cache for service tests.
// Records call counts so tests can assert caching behaviour without disk I/O.
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
		Git:   &fakeGit{diff: "some diff"},
		LLM:   &fakeLLM{response: "feat: add thing"},
		Cache: cache,
		Model: "test-model",
		Log:   noopLogger(),
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
		Git:   &fakeGit{diff: "some diff"},
		LLM:   &fakeLLM{response: "feat: cached", onGenerate: func(string) { llmCalls++ }},
		Cache: cache,
		Model: "test-model",
		Log:   noopLogger(),
	}

	svc.DraftMessage(context.Background(), CommitOptions{})                // miss — populates cache
	result, err := svc.DraftMessage(context.Background(), CommitOptions{}) // hit
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
		Git:   &fakeGit{diff: "some diff"},
		LLM:   &fakeLLM{err: errors.New("llm failed")},
		Cache: cache,
		Model: "test-model",
		Log:   noopLogger(),
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
		Git:   &fakeGit{diff: "same diff"},
		LLM:   &fakeLLM{response: "msg", onGenerate: func(string) { llmCalls++ }},
		Cache: cache,
		Model: "test-model",
		Log:   noopLogger(),
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
			Git:   &fakeGit{diff: diff},
			LLM:   &fakeLLM{response: "msg", onGenerate: func(string) { llmCalls++ }},
			Cache: cache,
			Model: "test-model",
			Log:   noopLogger(),
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
		LLM:   &fakeLLM{response: "feat/add-auth"},
		Cache: cache,
		Model: "test-model",
		Log:   noopLogger(),
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
		LLM:   &fakeLLM{response: "feat/add-auth", onGenerate: func(string) { llmCalls++ }},
		Cache: cache,
		Model: "test-model",
		Log:   noopLogger(),
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
		LLM:   &fakeLLM{err: errors.New("llm failed")},
		Cache: cache,
		Model: "test-model",
		Log:   noopLogger(),
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
		Git:   &fakeGit{commits: []string{"feat: add login"}},
		LLM:   &fakeLLM{response: "PR description"},
		Cache: cache,
		Model: "test-model",
		Log:   noopLogger(),
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
		Git:   &fakeGit{commits: []string{"feat: add login"}},
		LLM:   &fakeLLM{response: "PR desc", onGenerate: func(string) { llmCalls++ }},
		Cache: cache,
		Model: "test-model",
		Log:   noopLogger(),
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
		Git:   &fakeGit{commits: []string{"feat: something"}},
		LLM:   &fakeLLM{err: errors.New("llm failed")},
		Cache: cache,
		Model: "test-model",
		Log:   noopLogger(),
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
			Git:   &fakeGit{commits: commits},
			LLM:   &fakeLLM{response: "pr", onGenerate: func(string) { llmCalls++ }},
			Cache: cache,
			Model: "test-model",
			Log:   noopLogger(),
		}
	}

	makeService([]string{"feat: one", "feat: two"}).DraftPrDescription(context.Background(),
		PrOptions{SourceBranch: "feature/x", DestinationBranch: "main"},
	)
	// A new commit was added — different input, must miss.
	makeService([]string{"feat: one", "feat: two", "feat: three"}).DraftPrDescription(context.Background(),
		PrOptions{SourceBranch: "feature/x", DestinationBranch: "main"},
	)

	if llmCalls != 2 {
		t.Errorf("new commit must invalidate cache, LLM called %d times (expected 2)", llmCalls)
	}
}

// ─── NoCache flag ─────────────────────────────────────────────────────────────

// When NoCache is true the cache Get must be skipped entirely — the LLM is
// called even when a cached entry exists for the same key.
func TestDraftMessage_NoCache_SkipsCacheGet(t *testing.T) {
	cache := newFakeCache()
	llmCalls := 0
	svc := Service{
		Git:   &fakeGit{diff: "some diff"},
		LLM:   &fakeLLM{response: "feat: fresh", onGenerate: func(string) { llmCalls++ }},
		Cache: cache,
		Model: "test-model",
		Log:   noopLogger(),
	}

	// Prime the cache with a normal call.
	svc.DraftMessage(context.Background(), CommitOptions{})
	if llmCalls != 1 {
		t.Fatalf("expected 1 LLM call to prime cache, got %d", llmCalls)
	}

	// A second call without NoCache must hit the cache.
	svc.DraftMessage(context.Background(), CommitOptions{})
	if llmCalls != 1 {
		t.Fatalf("expected cache hit (still 1 LLM call), got %d", llmCalls)
	}

	// A call with NoCache must bypass the cache and call the LLM again.
	svc.DraftMessage(context.Background(), CommitOptions{Options: domain.Options{NoCache: true}})
	if llmCalls != 2 {
		t.Errorf("NoCache must force a fresh LLM call, got %d total LLM calls (want 2)", llmCalls)
	}
}

// When NoCache is true the fresh LLM response must overwrite the cached entry
// so subsequent normal calls serve the updated value.
func TestDraftMessage_NoCache_OverwritesCachedEntry(t *testing.T) {
	cache := newFakeCache()
	callCount := 0
	responses := []string{"feat: original", "feat: refreshed"}
	svc := Service{
		Git: &fakeGit{diff: "some diff"},
		LLM: &fakeLLM{onGenerate: func(string) {
			callCount++
		}},
		Cache: cache,
		Model: "test-model",
		Log:   noopLogger(),
	}
	// Override Generate to return different values per call.
	svc.LLM = &sequenceLLM{responses: responses}

	// Prime cache.
	r1, _ := svc.DraftMessage(context.Background(), CommitOptions{})
	if r1 != "feat: original" {
		t.Fatalf("expected first response %q, got %q", "feat: original", r1)
	}

	// NoCache call — must call LLM and overwrite cache.
	r2, _ := svc.DraftMessage(context.Background(), CommitOptions{Options: domain.Options{NoCache: true}})
	if r2 != "feat: refreshed" {
		t.Fatalf("expected refreshed response %q, got %q", "feat: refreshed", r2)
	}

	// Normal call after NoCache — must now serve the refreshed value from cache.
	r3, _ := svc.DraftMessage(context.Background(), CommitOptions{})
	if r3 != "feat: refreshed" {
		t.Errorf("cache should hold refreshed value %q, got %q", "feat: refreshed", r3)
	}
}

func TestDraftBranchName_NoCache_SkipsCacheGet(t *testing.T) {
	cache := newFakeCache()
	llmCalls := 0
	svc := Service{
		LLM:   &fakeLLM{response: "feat/branch", onGenerate: func(string) { llmCalls++ }},
		Cache: cache,
		Model: "test-model",
		Log:   noopLogger(),
	}

	opts := BranchOptions{Task: "add auth"}
	svc.DraftBranchName(context.Background(), opts)
	if llmCalls != 1 {
		t.Fatalf("expected 1 LLM call to prime cache, got %d", llmCalls)
	}

	// Cache hit — LLM must not be called.
	svc.DraftBranchName(context.Background(), opts)
	if llmCalls != 1 {
		t.Fatalf("expected cache hit, got %d LLM calls", llmCalls)
	}

	// NoCache — LLM must be called again.
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
		Git:   &fakeGit{commits: []string{"feat: thing"}},
		LLM:   &fakeLLM{response: "PR desc", onGenerate: func(string) { llmCalls++ }},
		Cache: cache,
		Model: "test-model",
		Log:   noopLogger(),
	}

	opts := PrOptions{SourceBranch: "feature/x", DestinationBranch: "main"}
	svc.DraftPrDescription(context.Background(), opts)
	if llmCalls != 1 {
		t.Fatalf("expected 1 LLM call to prime cache, got %d", llmCalls)
	}

	// Cache hit.
	svc.DraftPrDescription(context.Background(), opts)
	if llmCalls != 1 {
		t.Fatalf("expected cache hit, got %d LLM calls", llmCalls)
	}

	// NoCache — must bypass cache.
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

// sequenceLLM returns responses from a pre-defined list in order.
// Used when tests need distinct responses per LLM call.
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
	// Both commands receive identical content strings — keys must still differ
	// because they incorporate different template hashes.
	cache := newFakeCache()
	llmCalls := 0
	svc := Service{
		Git:   &fakeGit{diff: "content"},
		LLM:   &fakeLLM{response: "result", onGenerate: func(string) { llmCalls++ }},
		Cache: cache,
		Model: "test-model",
		Log:   noopLogger(),
	}

	svc.DraftMessage(context.Background(), CommitOptions{})
	svc.DraftBranchName(context.Background(), BranchOptions{Task: "content"})

	if llmCalls != 2 {
		t.Errorf("commit and branch must have independent cache namespaces, LLM called %d times", llmCalls)
	}
}
