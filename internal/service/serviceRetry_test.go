package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// ─── countingLLM ────────────────────────────────────────────────────────────

// countingLLM fails the first `failTimes` calls then succeeds.
// Thread-safe via atomic counter.
type countingLLM struct {
	failTimes int32
	calls     atomic.Int32
	response  string
}

func (c *countingLLM) Generate(_ context.Context, prompt string) (string, error) {
	n := c.calls.Add(1)
	if n <= c.failTimes {
		return "", fmt.Errorf("transient error on attempt %d", n)
	}
	return c.response, nil
}

// testRetryDelay is a negligible delay used in retry tests so that back-off
// sleeps do not slow down the test suite. Production code uses defaultRetryBaseDelay
// (2 s), which would make exhausted-retry tests take 6+ seconds each.
const testRetryDelay = time.Millisecond

func TestGenerateWithRetry_NilLLM_ReturnsError(t *testing.T) {
	svc := &Service{
		LLM: nil,
		Log: noopLogger(),
	}

	var err error
	require_no_panic_svc(t, func() {
		_, err = svc.generateWithRetry(context.Background(), "prompt")
	})
	if err == nil {
		t.Error("generateWithRetry with nil LLM must return an error, not panic")
	}
}

// require_no_panic_svc is a local copy of the helper to avoid cross-package
// dependency while keeping the test self-contained.
func require_no_panic_svc(t *testing.T, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Errorf("unexpected panic: %v", r)
		}
	}()
	fn()
}

// ─── PR: mapReducePr ────────────────────────────────────────────────────────

func TestDraftPrDescription_ChunkedCommits_UsesMapReduce(t *testing.T) {
	// Build commits whose joined size exceeds the threshold.
	commits := make([]string, 10)
	for i := range commits {
		commits[i] = fmt.Sprintf("feat: change %s", strings.Repeat("x", 60))
	}

	var callCount atomic.Int32
	svc := Service{
		Git: &fakeGit{commits: commits},
		LLM: &fakeLLM{
			response:   "- bullet summary",
			onGenerate: func(string) { callCount.Add(1) },
		},
		Log:            noopLogger(),
		ChunkThreshold: 100,
	}

	result, err := svc.DraftPrDescription(context.Background(), PrOptions{SourceBranch: "feature/x", DestinationBranch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if result == "" {
		t.Error("expected non-empty result from map-reduce PR")
	}
	// Must have at least 2 calls: chunk summaries + synthesis.
	if callCount.Load() < 2 {
		t.Errorf("expected at least 2 LLM calls for chunked PR, got %d", callCount.Load())
	}
}

func TestDraftPrDescription_SmallCommits_SingleLLMCall(t *testing.T) {
	var callCount atomic.Int32
	svc := Service{
		Git: &fakeGit{commits: []string{"feat: small"}},
		LLM: &fakeLLM{
			response:   "PR description",
			onGenerate: func(string) { callCount.Add(1) },
		},
		Log:            noopLogger(),
		ChunkThreshold: 1000,
	}

	result, err := svc.DraftPrDescription(context.Background(), PrOptions{SourceBranch: "feature/x", DestinationBranch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}
	if callCount.Load() != 1 {
		t.Errorf("expected exactly 1 LLM call for small commits, got %d", callCount.Load())
	}
}

func TestDraftPrDescription_ReturnsResultAfterSingleCall(t *testing.T) {
	// Regression: the original code had a shadowed :=  variable bug that
	// caused DraftPrDescription to always return "" for non-chunked paths.
	svc := Service{
		Git: &fakeGit{commits: []string{"feat: add login"}},
		LLM: &fakeLLM{response: "## PR description text"},
		Log: noopLogger(),
	}

	result, err := svc.DraftPrDescription(context.Background(), PrOptions{SourceBranch: "feature/login", DestinationBranch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if result == "" {
		t.Error("DraftPrDescription must not return empty string on success (shadowed variable regression)")
	}
}

func TestDraftPrDescription_LLMError_StillCallsDone(t *testing.T) {
	fp := &fakeProgress{}
	svc := Service{
		Git:      &fakeGit{commits: []string{"feat: something"}},
		LLM:      &fakeLLM{err: errors.New("fail")},
		Log:      noopLogger(),
		Progress: fp,
	}
	svc.DraftPrDescription(context.Background(), PrOptions{SourceBranch: "a", DestinationBranch: "b"})
	if fp.doneCount() == 0 {
		t.Error("expected Done to be called even on error")
	}
}

func TestDraftPrDescription_MapReduce_ReportsChunkProgress(t *testing.T) {
	commits := make([]string, 10)
	for i := range commits {
		commits[i] = fmt.Sprintf("feat: change %s", strings.Repeat("x", 60))
	}

	fp := &fakeProgress{}
	svc := Service{
		Git:            &fakeGit{commits: commits},
		LLM:            &fakeLLM{response: "- bullet"},
		Log:            noopLogger(),
		Progress:       fp,
		ChunkThreshold: 100,
	}

	_, err := svc.DraftPrDescription(context.Background(), PrOptions{SourceBranch: "feature/x", DestinationBranch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if !fp.hasStatusContaining("Summarizing") {
		t.Error("expected chunk summarization progress for PR")
	}
	if !fp.hasStatusContaining("Synthesizing") {
		t.Error("expected synthesis progress for PR")
	}
	if fp.doneCount() == 0 {
		t.Error("expected Done to be called")
	}
}

// ─── PR: cache with map-reduce ──────────────────────────────────────────────

func TestDraftPrDescription_MapReduce_CachesResult(t *testing.T) {
	commits := make([]string, 10)
	for i := range commits {
		commits[i] = fmt.Sprintf("feat: change %s", strings.Repeat("x", 60))
	}

	cache := newFakeCache()
	var llmCalls atomic.Int32
	svc := Service{
		Git: &fakeGit{commits: commits},
		LLM: &fakeLLM{
			response:   "- summary",
			onGenerate: func(string) { llmCalls.Add(1) },
		},
		Cache:          cache,
		Model:          "test-model",
		Log:            noopLogger(),
		ChunkThreshold: 100,
	}

	opts := PrOptions{SourceBranch: "feature/x", DestinationBranch: "main"}
	svc.DraftPrDescription(context.Background(), opts) // miss
	firstCalls := llmCalls.Load()

	svc.DraftPrDescription(context.Background(), opts) // should hit cache
	if llmCalls.Load() != firstCalls {
		t.Errorf("second call should use cache, but LLM was called again (total=%d, after_first=%d)",
			llmCalls.Load(), firstCalls)
	}
}

// ─── Branch: long task truncation / guard ──────────────────────────────────

func TestDraftBranchName_LongTask_StillReturnsResult(t *testing.T) {
	// Branch command receives a very long user task description.
	// Service should handle it without panicking or returning an error.
	longTask := strings.Repeat("add feature for user management system ", 200) // ~7600 chars
	svc := Service{
		LLM:            &fakeLLM{response: "feat/user-management"},
		Log:            noopLogger(),
		ChunkThreshold: DefaultChunkThreshold,
	}

	result, err := svc.DraftBranchName(context.Background(), BranchOptions{Task: longTask})
	if err != nil {
		t.Fatalf("unexpected error for long task: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result for long task")
	}
}

func TestDraftBranchName_LLMError_StillCallsDone(t *testing.T) {
	fp := &fakeProgress{}
	svc := Service{
		LLM:      &fakeLLM{err: errors.New("fail")},
		Log:      noopLogger(),
		Progress: fp,
	}
	svc.DraftBranchName(context.Background(), BranchOptions{Task: "some task"})
	if fp.doneCount() == 0 {
		t.Error("expected Done to be called even on error")
	}
}

// ─── Service-level retry ────────────────────────────────────────────────────

func TestDraftMessage_Retry_SucceedsAfterTransientFailure(t *testing.T) {
	llm := &countingLLM{failTimes: 2, response: "feat: add thing"}
	svc := Service{
		Git:            &fakeGit{diff: "some diff"},
		LLM:            llm,
		Log:            noopLogger(),
		MaxRetries:     3,
		RetryBaseDelay: testRetryDelay,
	}

	result, err := svc.DraftMessage(context.Background(), CommitOptions{})
	if err != nil {
		t.Fatalf("expected success after retries, got: %v", err)
	}
	if result != "feat: add thing" {
		t.Errorf("unexpected result: %q", result)
	}
	if llm.calls.Load() != 3 {
		t.Errorf("expected 3 total calls (2 failures + 1 success), got %d", llm.calls.Load())
	}
}

func TestDraftMessage_Retry_ExhaustedReturnsError(t *testing.T) {
	llm := &countingLLM{failTimes: 10, response: "never"}
	svc := Service{
		Git:            &fakeGit{diff: "some diff"},
		LLM:            llm,
		Log:            noopLogger(),
		MaxRetries:     2,
		RetryBaseDelay: testRetryDelay,
	}

	_, err := svc.DraftMessage(context.Background(), CommitOptions{})
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
}

func TestDraftMessage_Retry_ZeroRetries_FailsImmediately(t *testing.T) {
	llm := &countingLLM{failTimes: 1, response: "msg"}
	svc := Service{
		Git:            &fakeGit{diff: "some diff"},
		LLM:            llm,
		Log:            noopLogger(),
		MaxRetries:     0, // no retries — first failure is final
		RetryBaseDelay: testRetryDelay,
	}

	_, err := svc.DraftMessage(context.Background(), CommitOptions{})
	if err == nil {
		t.Fatal("expected error with 0 retries")
	}
	if llm.calls.Load() != 1 {
		t.Errorf("expected exactly 1 call with MaxRetries=0, got %d", llm.calls.Load())
	}
}

func TestDraftPrDescription_Retry_SucceedsAfterTransientFailure(t *testing.T) {
	llm := &countingLLM{failTimes: 1, response: "PR description"}
	svc := Service{
		Git:            &fakeGit{commits: []string{"feat: thing"}},
		LLM:            llm,
		Log:            noopLogger(),
		MaxRetries:     2,
		RetryBaseDelay: testRetryDelay,
	}

	result, err := svc.DraftPrDescription(context.Background(), PrOptions{SourceBranch: "a", DestinationBranch: "b"})
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if result == "" {
		t.Error("expected non-empty result")
	}
}

func TestDraftBranchName_Retry_SucceedsAfterTransientFailure(t *testing.T) {
	llm := &countingLLM{failTimes: 1, response: "feat/add-auth"}
	svc := Service{
		LLM:            llm,
		Log:            noopLogger(),
		MaxRetries:     2,
		RetryBaseDelay: testRetryDelay,
	}

	result, err := svc.DraftBranchName(context.Background(), BranchOptions{Task: "add authentication"})
	if err != nil {
		t.Fatalf("expected success after retry, got: %v", err)
	}
	if result != "feat/add-auth" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestDraftBranchName_Retry_ExhaustedReturnsError(t *testing.T) {
	llm := &countingLLM{failTimes: 10, response: "never"}
	svc := Service{
		LLM:            llm,
		Log:            noopLogger(),
		MaxRetries:     2,
		RetryBaseDelay: testRetryDelay,
	}

	_, err := svc.DraftBranchName(context.Background(), BranchOptions{Task: "some task"})
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
}

func TestDraftPrDescription_Retry_ExhaustedReturnsError(t *testing.T) {
	llm := &countingLLM{failTimes: 10, response: "never"}
	svc := Service{
		Git:            &fakeGit{commits: []string{"feat: thing"}},
		LLM:            llm,
		Log:            noopLogger(),
		MaxRetries:     2,
		RetryBaseDelay: testRetryDelay,
	}

	_, err := svc.DraftPrDescription(context.Background(), PrOptions{SourceBranch: "a", DestinationBranch: "b"})
	if err == nil {
		t.Fatal("expected error after exhausting retries")
	}
}

// ─── PR synthesis prompt ────────────────────────────────────────────────────

func TestBuildPrSynthesisPrompt_ContainsSummaries(t *testing.T) {
	summaries := []string{"- added auth endpoint", "- updated tests"}
	opts := PrOptions{SourceBranch: "feature/auth", DestinationBranch: "main"}
	prompt := BuildPrSynthesisPrompt(summaries, opts)

	if !strings.Contains(prompt, "added auth endpoint") {
		t.Error("synthesis prompt should contain summaries")
	}
	if !strings.Contains(prompt, "feature/auth") {
		t.Error("synthesis prompt should contain source branch")
	}
	if !strings.Contains(prompt, "main") {
		t.Error("synthesis prompt should contain destination branch")
	}
}

func TestBuildPrSynthesisPrompt_ContainsTitleInstruction(t *testing.T) {
	opts := PrOptions{SourceBranch: "feature/x", DestinationBranch: "main"}
	prompt := BuildPrSynthesisPrompt([]string{"- change"}, opts)
	if !strings.Contains(prompt, "title") {
		t.Errorf("synthesis prompt should request PR title, got:\n%s", prompt)
	}
}
