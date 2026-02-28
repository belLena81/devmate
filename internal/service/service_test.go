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
		Git: &fakeGit{
			diff: "diff --git a/a.go b/a.go",
		},
		LLM: &fakeLLM{
			response: "feat: add new feature",
		},
		Log: noopLogger(),
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
		Git: &fakeGit{diff: "STAGED DIFF"},
		LLM: &fakeLLM{
			onGenerate: func(prompt string) {
				receivedPrompt = prompt
			},
			response: "msg",
		},
		Log: noopLogger(),
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
		Git: &errorGit{},
		LLM: &fakeLLM{},
		Log: noopLogger(),
	}

	opts := CommitOptions{}

	_, err := svc.DraftMessage(context.Background(), opts)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBranchService_DraftsBranchName(t *testing.T) {
	svc := Service{
		LLM: &fakeLLM{response: "feat/add-auth"},
		Log: noopLogger(),
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
		LLM: &fakeLLM{onGenerate: func(p string) { received = p }},
		Log: noopLogger(),
	}
	svc.DraftBranchName(context.Background(), BranchOptions{Task: "fix login bug"})
	if !strings.Contains(received, "fix login bug") {
		t.Errorf("task not in prompt, got: %q", received)
	}
}

func TestPrService_DraftsPrDescription(t *testing.T) {
	svc := Service{
		Git: &fakeGit{commits: []string{"feat: add login"}},
		LLM: &fakeLLM{response: "feat: add login"},
		Log: noopLogger(),
	}
	opts := PrOptions{SourceBranch: "feature/login", DestinationBranch: "main"}
	result, err := svc.DraftPrDescription(context.Background(), opts)
	if err != nil {
		t.Fatal(err)
	}
	if result != "feat: add login" {
		t.Errorf("unexpected result: %q", result)
	}
}

func TestPrService_PassesDiffToLLM(t *testing.T) {
	var received string
	svc := Service{
		Git: &fakeGit{commits: []string{"one", "two"}},
		LLM: &fakeLLM{onGenerate: func(p string) { received = p }},
		Log: noopLogger(),
	}
	svc.DraftPrDescription(context.Background(), PrOptions{SourceBranch: "feature/x", DestinationBranch: "main"})
	if !strings.Contains(received, "- one") || !strings.Contains(received, "- two") {
		t.Errorf("commits are not in prompt, got: %q", received)
	}
}

func TestPrService_GitError(t *testing.T) {
	svc := Service{
		Git: &errorGit{},
		LLM: &fakeLLM{},
		Log: noopLogger(),
	}
	_, err := svc.DraftPrDescription(context.Background(), PrOptions{SourceBranch: "a", DestinationBranch: "b"})
	if err == nil {
		t.Fatal("expected error from git")
	}
}

func TestPrService_LLMError(t *testing.T) {
	svc := Service{
		Git: &fakeGit{diff: "some diff"},
		LLM: &fakeLLM{err: errors.New("LLM failed")},
		Log: noopLogger(),
	}
	_, err := svc.DraftPrDescription(context.Background(), PrOptions{SourceBranch: "a", DestinationBranch: "b"})
	if err == nil {
		t.Fatal("expected error from LLM")
	}
}

func TestCommitService_LLMError(t *testing.T) {
	svc := Service{
		Git: &fakeGit{diff: "some diff"},
		LLM: &fakeLLM{err: errors.New("LLM failed")},
		Log: noopLogger(),
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
		Git:      &fakeGit{diff: "small diff"},
		LLM:      &fakeLLM{response: "feat: thing"},
		Log:      noopLogger(),
		Progress: fp,
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
		Git:            &fakeGit{diff: diff.String()},
		LLM:            &fakeLLM{response: "- bullet"},
		Log:            noopLogger(),
		Progress:       fp,
		ChunkThreshold: 500,
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
		LLM:      &fakeLLM{response: "feat/add-auth"},
		Log:      noopLogger(),
		Progress: fp,
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
		Git:      &fakeGit{commits: []string{"feat: login"}},
		LLM:      &fakeLLM{response: "PR desc"},
		Log:      noopLogger(),
		Progress: fp,
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
		Git:      &fakeGit{diff: "diff"},
		LLM:      &fakeLLM{err: errors.New("fail")},
		Log:      noopLogger(),
		Progress: fp,
	}
	svc.DraftMessage(context.Background(), CommitOptions{})
	if fp.doneCount() == 0 {
		t.Error("expected Done to be called even on error (to clear the spinner)")
	}
}

func TestDraftMessage_NilProgress_DoesNotPanic(t *testing.T) {
	// Progress is nil — the progress() accessor should return NoopProgress.
	svc := Service{
		Git: &fakeGit{diff: "diff"},
		LLM: &fakeLLM{response: "msg"},
		Log: noopLogger(),
	}
	_, err := svc.DraftMessage(context.Background(), CommitOptions{})
	if err != nil {
		t.Fatal(err)
	}
}

func TestPrService_EmptyCommits_ReturnsErrEmptyPR(t *testing.T) {
	svc := Service{
		Git: &fakeGit{commits: []string{}},
		LLM: &fakeLLM{},
		Log: noopLogger(),
	}
	_, err := svc.DraftPrDescription(context.Background(), PrOptions{SourceBranch: "feature/x", DestinationBranch: "main"})
	if !errors.Is(err, domain.ErrEmptyPR) {
		t.Errorf("expected ErrEmptyPR when no commits exist between branches, got %v", err)
	}
}
