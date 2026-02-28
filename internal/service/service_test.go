package service

import (
	"context"
	"devmate/internal/domain"
	"errors"
	"io"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeGit struct {
	diff    string
	commits []string
}

func (f *fakeGit) DiffCached() (string, error) {
	return f.diff, nil
}

func (f *fakeGit) LogBetween(base, head string) ([]string, error) {
	return f.commits, nil
}

type fakeLLM struct {
	mu         sync.Mutex
	response   string
	err        error
	onGenerate func(string)
}

func (f *fakeLLM) Generate(_ context.Context, prompt string) (string, error) {
	f.mu.Lock()
	cb := f.onGenerate
	f.mu.Unlock()

	if cb != nil {
		cb(prompt)
	}
	return f.response, f.err
}

// fakeProgress records all Status and Done calls for test assertions.
// Thread-safe: goroutines in summarizeChunksParallel call Status concurrently.
type fakeProgress struct {
	mu       sync.Mutex
	statuses []string
	dones    []string
}

func (f *fakeProgress) Status(msg string) {
	f.mu.Lock()
	f.statuses = append(f.statuses, msg)
	f.mu.Unlock()
}

func (f *fakeProgress) Done(msg string) {
	f.mu.Lock()
	f.dones = append(f.dones, msg)
	f.mu.Unlock()
}

func (f *fakeProgress) statusCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.statuses)
}

func (f *fakeProgress) hasStatusContaining(sub string) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, s := range f.statuses {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
}

func (f *fakeProgress) doneCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.dones)
}

func TestCommitService_DraftsMessage(t *testing.T) {
	svc := Service{
		git: &fakeGit{
			diff: "diff --git a/a.go b/a.go",
		},
		llm: &fakeLLM{
			response: "feat: add new feature",
		},
		log: noopLogger(),
	}

	opts := CommitOptions{}

	msg, err := svc.DraftMessage(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}

	if msg != "feat: add new feature" {
		t.Fatalf("unexpected message: %q", msg)
	}
}

func noopLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestCommitService_PassesDiffToLLM(t *testing.T) {
	var receivedPrompt string

	svc := Service{
		git: &fakeGit{diff: "STAGED DIFF"},
		llm: &fakeLLM{
			onGenerate: func(prompt string) {
				receivedPrompt = prompt
			},
			response: "msg",
		},
		log: noopLogger(),
	}

	opts := CommitOptions{}

	_, _ = svc.DraftMessage(context.Background(), opts)

	if !strings.Contains(receivedPrompt, "STAGED DIFF") {
		t.Fatal("diff not included in prompt")
	}
}

type errorGit struct{}

func (e *errorGit) DiffCached() (string, error) {
	return "", errors.New("git failed")
}

func (e *errorGit) LogBetween(base, head string) ([]string, error) {
	return nil, errors.New("git failed")
}

func TestCommitService_GitError(t *testing.T) {
	svc := Service{
		git: &errorGit{},
		llm: &fakeLLM{},
		log: noopLogger(),
	}

	opts := CommitOptions{}

	_, err := svc.DraftMessage(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBranchService_DraftsBranchName(t *testing.T) {
	svc := Service{
		llm: &fakeLLM{response: "feat/add-auth"},
		log: noopLogger(),
	}
	opts := BranchOptions{Task: "add authentication"}
	result, err := svc.DraftBranchName(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if result != "feat/add-auth" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestBranchService_PassesTaskToLLM(t *testing.T) {
	var received string
	svc := Service{
		llm: &fakeLLM{onGenerate: func(p string) { received = p }},
		log: noopLogger(),
	}
	_, err := svc.DraftBranchName(context.Background(), BranchOptions{Task: "fix login bug"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !strings.Contains(received, "fix login bug") {
		t.Errorf("task not in prompt, got: %q", received)
	}
}

func TestPrService_DraftsPrDescription(t *testing.T) {
	svc := Service{
		git: &fakeGit{commits: []string{"feat: add login"}},
		llm: &fakeLLM{response: "feat: add login"},
		log: noopLogger(),
	}
	opts := PrOptions{SourceBranch: "feature/login", DestinationBranch: "main"}
	result, err := svc.DraftPrDescription(context.Background(), opts)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if result != "feat: add login" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestPrService_PassesDiffToLLM(t *testing.T) {
	var received string
	svc := Service{
		git: &fakeGit{commits: []string{"one", "two"}},
		llm: &fakeLLM{onGenerate: func(p string) { received = p }},
		log: noopLogger(),
	}
	_, err := svc.DraftPrDescription(context.Background(), PrOptions{SourceBranch: "feature/x", DestinationBranch: "main"})
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if !strings.Contains(received, "- one") || !strings.Contains(received, "- two") {
		t.Errorf("commits are not in prompt, got: %q", received)
	}
}

func TestPrService_GitError(t *testing.T) {
	svc := Service{
		git: &errorGit{},
		llm: &fakeLLM{},
		log: noopLogger(),
	}
	_, err := svc.DraftPrDescription(context.Background(), PrOptions{SourceBranch: "a", DestinationBranch: "b"})
	if err == nil {
		t.Fatal("expected error from git")
	}
}

func TestPrService_LLMError(t *testing.T) {
	svc := Service{
		git: &fakeGit{diff: "some diff"},
		llm: &fakeLLM{err: errors.New("LLM failed")},
		log: noopLogger(),
	}
	_, err := svc.DraftPrDescription(context.Background(), PrOptions{SourceBranch: "a", DestinationBranch: "b"})
	if err == nil {
		t.Fatal("expected error from LLM")
	}
}

func TestCommitService_LLMError(t *testing.T) {
	svc := Service{
		git: &fakeGit{diff: "some diff"},
		llm: &fakeLLM{err: errors.New("LLM failed")},
		log: noopLogger(),
	}
	_, err := svc.DraftMessage(context.Background(), CommitOptions{})
	if err == nil {
		t.Fatal("expected error from LLM")
	}
}

// ─── progress integration ───────────────────────────────────────────────────

func TestDraftMessage_SingleShot_ReportsProgress(t *testing.T) {
	fp := &fakeProgress{}
	svc := Service{
		git:      &fakeGit{diff: "small diff"},
		llm:      &fakeLLM{response: "feat: thing"},
		log:      noopLogger(),
		progress: fp,
	}
	_, err := svc.DraftMessage(context.Background(), CommitOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !fp.hasStatusContaining("Generating commit message") {
		t.Error("expected progress status for single-shot commit")
	}
	if fp.doneCount() == 0 {
		t.Error("expected Done to be called after completion")
	}
}

func TestDraftMessage_MapReduce_ReportsChunkProgress(t *testing.T) {
	var diff strings.Builder
	for i := 0; i < 6; i++ {
		diff.WriteString("diff --git a/f.go b/f.go\n")
		diff.WriteString("+" + strings.Repeat("x", 200) + "\n")
	}

	fp := &fakeProgress{}
	svc := Service{
		git:            &fakeGit{diff: diff.String()},
		llm:            &fakeLLM{response: "- bullet"},
		log:            noopLogger(),
		progress:       fp,
		chunkThreshold: 500,
	}
	_, err := svc.DraftMessage(context.Background(), CommitOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if !fp.hasStatusContaining("Summarizing chunk") {
		t.Error("expected chunk summarization progress")
	}
	if !fp.hasStatusContaining("Synthesizing") {
		t.Error("expected synthesis progress")
	}
	if fp.doneCount() == 0 {
		t.Error("expected Done to be called")
	}
}

func TestDraftBranchName_ReportsProgress(t *testing.T) {
	fp := &fakeProgress{}
	svc := Service{
		llm:      &fakeLLM{response: "feat/add-auth"},
		log:      noopLogger(),
		progress: fp,
	}
	_, err := svc.DraftBranchName(context.Background(), BranchOptions{Task: "add auth"})
	if err != nil {
		t.Fatal(err)
	}
	if !fp.hasStatusContaining("Generating branch name") {
		t.Error("expected branch name progress status")
	}
	if fp.doneCount() == 0 {
		t.Error("expected Done to be called")
	}
}

func TestDraftPrDescription_ReportsProgress(t *testing.T) {
	fp := &fakeProgress{}
	svc := Service{
		git:      &fakeGit{commits: []string{"feat: login"}},
		llm:      &fakeLLM{response: "PR desc"},
		log:      noopLogger(),
		progress: fp,
	}
	_, err := svc.DraftPrDescription(context.Background(), PrOptions{SourceBranch: "feature/x", DestinationBranch: "main"})
	if err != nil {
		t.Fatal(err)
	}
	if !fp.hasStatusContaining("Generating PR description") {
		t.Error("expected PR description progress status")
	}
	if fp.doneCount() == 0 {
		t.Error("expected Done to be called")
	}
}

func TestDraftMessage_LLMError_StillCallsDone(t *testing.T) {
	fp := &fakeProgress{}
	svc := Service{
		git:      &fakeGit{diff: "diff"},
		llm:      &fakeLLM{err: errors.New("fail")},
		log:      noopLogger(),
		progress: fp,
	}
	_, err := svc.DraftMessage(context.Background(), CommitOptions{})
	if err == nil {
		t.Error("DraftMessage with failing LLM must return an error, not panic")
	}
	if fp.doneCount() == 0 {
		t.Error("expected Done to be called even on error (to clear the spinner)")
	}
}

func TestDraftMessage_NilProgress_DoesNotPanic(t *testing.T) {
	// Progress is nil — the progress() accessor should return NoopProgress.
	svc := Service{
		git: &fakeGit{diff: "diff"},
		llm: &fakeLLM{response: "msg"},
		log: noopLogger(),
	}
	_, err := svc.DraftMessage(context.Background(), CommitOptions{})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPrService_EmptyCommits_ReturnsErrEmptyPR(t *testing.T) {
	svc := Service{
		git: &fakeGit{commits: []string{}},
		llm: &fakeLLM{},
		log: noopLogger(),
	}
	_, err := svc.DraftPrDescription(context.Background(), PrOptions{SourceBranch: "feature/x", DestinationBranch: "main"})
	if !errors.Is(err, domain.ErrEmptyPR) {
		t.Errorf("expected ErrEmptyPR when no commits exist between branches, got %v", err)
	}
}

func TestNew_Defaults(t *testing.T) {
	svc := New(nil, &fakeLLM{}, NoopCache{}, "model", noopLogger())

	if svc.ChunkThreshold() != DefaultChunkThreshold {
		t.Errorf("ChunkThreshold: got %d, want %d", svc.ChunkThreshold(), DefaultChunkThreshold)
	}
	if svc.concurrency() != DefaultServiceMaxConcurrency {
		t.Errorf("MaxConcurrency: got %d, want %d", svc.concurrency(), DefaultServiceMaxConcurrency)
	}
	if svc.MaxRetries() != 0 {
		t.Errorf("MaxRetries: got %d, want 0", svc.MaxRetries())
	}
	if svc.RetryBaseDelay() != defaultRetryBaseDelay {
		t.Errorf("RetryBaseDelay: got %v, want %v (uses package default)", svc.RetryBaseDelay(), defaultRetryBaseDelay)
	}
	if _, ok := svc.Progress().(domain.NoopProgress); !ok {
		t.Error("Progress: expected NoopProgress by default")
	}
}

func TestNew_WithProgress(t *testing.T) {
	fp := &fakeProgress{}
	svc := New(nil, &fakeLLM{}, NoopCache{}, "model", noopLogger(), WithProgress(fp))
	if svc.Progress() != fp {
		t.Error("WithProgress: progress reporter not applied")
	}
}

func TestNew_WithChunkThreshold(t *testing.T) {
	svc := New(nil, &fakeLLM{}, NoopCache{}, "model", noopLogger(), WithChunkThreshold(8000))
	if svc.ChunkThreshold() != 8000 {
		t.Errorf("WithChunkThreshold: got %d, want 8000", svc.ChunkThreshold())
	}
}

// WithChunkThreshold(0) must be a no-op — zero is not a valid threshold.
func TestNew_WithChunkThreshold_ZeroIgnored(t *testing.T) {
	svc := New(nil, &fakeLLM{}, NoopCache{}, "model", noopLogger(), WithChunkThreshold(0))
	if svc.ChunkThreshold() != DefaultChunkThreshold {
		t.Errorf("WithChunkThreshold(0) should keep default %d, got %d", DefaultChunkThreshold, svc.ChunkThreshold())
	}
}

func TestNew_WithMaxConcurrency(t *testing.T) {
	svc := New(nil, &fakeLLM{}, NoopCache{}, "model", noopLogger(), WithMaxConcurrency(4))
	if svc.concurrency() != 4 {
		t.Errorf("WithMaxConcurrency: got %d, want 4", svc.concurrency())
	}
}

func TestNew_WithMaxRetries(t *testing.T) {
	svc := New(nil, &fakeLLM{}, NoopCache{}, "model", noopLogger(), WithMaxRetries(3))
	if svc.MaxRetries() != 3 {
		t.Errorf("WithMaxRetries: got %d, want 3", svc.MaxRetries())
	}
}

// WithMaxRetries(0) is a valid value (no retries) and must be applied.
func TestNew_WithMaxRetries_ZeroIsValid(t *testing.T) {
	svc := New(nil, &fakeLLM{}, NoopCache{}, "model", noopLogger(), WithMaxRetries(5), WithMaxRetries(0))
	if svc.MaxRetries() != 0 {
		t.Errorf("WithMaxRetries(0): got %d, want 0", svc.MaxRetries())
	}
}

func TestNew_WithRetryBaseDelay(t *testing.T) {
	svc := New(nil, &fakeLLM{}, NoopCache{}, "model", noopLogger(), WithRetryBaseDelay(500*time.Millisecond))
	if svc.RetryBaseDelay() != 500*time.Millisecond {
		t.Errorf("WithRetryBaseDelay: got %v, want 500ms", svc.RetryBaseDelay())
	}
}

// Multiple options applied in order — last write wins on the same field.
func TestNew_MultipleOptions_AppliedInOrder(t *testing.T) {
	svc := New(nil, &fakeLLM{}, NoopCache{}, "model", noopLogger(),
		WithChunkThreshold(1000),
		WithMaxConcurrency(6),
		WithMaxRetries(2),
		WithRetryBaseDelay(100*time.Millisecond),
	)
	if svc.ChunkThreshold() != 1000 {
		t.Errorf("ChunkThreshold: got %d, want 1000", svc.ChunkThreshold())
	}
	if svc.concurrency() != 6 {
		t.Errorf("MaxConcurrency: got %d, want 6", svc.concurrency())
	}
	if svc.MaxRetries() != 2 {
		t.Errorf("MaxRetries: got %d, want 2", svc.MaxRetries())
	}
	if svc.RetryBaseDelay() != 100*time.Millisecond {
		t.Errorf("RetryBaseDelay: got %v, want 100ms", svc.RetryBaseDelay())
	}
}
