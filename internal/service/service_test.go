package service

import (
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
)

type fakeGit struct {
	diff   string
	branch string
}

func (f *fakeGit) DiffCached() (string, error) {
	return f.diff, nil
}

func (f *fakeGit) CurrentBranch() (string, error) {
	return f.branch, nil
}

func (f *fakeGit) Compare(base, head string) (string, error) {
	return f.diff, nil
}

type fakeLLM struct {
	response   string
	err        error
	onGenerate func(string)
}

func (f *fakeLLM) Generate(prompt string) (string, error) {
	if f.onGenerate != nil {
		f.onGenerate(prompt)
	}
	return f.response, f.err
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

	msg, err := svc.DraftMessage(opts)
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

	_, _ = svc.DraftMessage(opts)

	if !strings.Contains(receivedPrompt, "STAGED DIFF") {
		t.Fatal("diff not included in prompt")
	}
}

type errorGit struct{}

func (e *errorGit) DiffCached() (string, error) {
	return "", errors.New("git failed")
}
func (e *errorGit) CurrentBranch() (string, error) {
	return "", errors.New("git failed")
}

func (e *errorGit) Compare(base, head string) (string, error) {
	return "", errors.New("git failed")
}

func TestCommitService_GitError(t *testing.T) {
	svc := Service{
		Git: &errorGit{},
		LLM: &fakeLLM{},
		Log: noopLogger(),
	}

	opts := CommitOptions{}

	_, err := svc.DraftMessage(opts)
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
	result, err := svc.DraftBranchName(opts)
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
	svc.DraftBranchName(BranchOptions{Task: "fix login bug"})
	if !strings.Contains(received, "fix login bug") {
		t.Errorf("task not in prompt, got: %q", received)
	}
}

func TestPrService_DraftsPrDescription(t *testing.T) {
	svc := Service{
		Git: &fakeGit{},
		LLM: &fakeLLM{response: "feat: add login"},
		Log: noopLogger(),
	}
	opts := PrOptions{SourceBranch: "feature/login", DestinationBranch: "main"}
	result, err := svc.DraftPrDescription(opts)
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
		Git: &fakeGit{diff: "PR DIFF"},
		LLM: &fakeLLM{onGenerate: func(p string) { received = p }},
		Log: noopLogger(),
	}
	svc.DraftPrDescription(PrOptions{SourceBranch: "feature/x", DestinationBranch: "main"})
	if !strings.Contains(received, "PR DIFF") {
		t.Errorf("diff not in prompt, got: %q", received)
	}
}

func TestPrService_GitError(t *testing.T) {
	svc := Service{
		Git: &errorGit{},
		LLM: &fakeLLM{},
		Log: noopLogger(),
	}
	_, err := svc.DraftPrDescription(PrOptions{SourceBranch: "a", DestinationBranch: "b"})
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
	_, err := svc.DraftPrDescription(PrOptions{SourceBranch: "a", DestinationBranch: "b"})
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
	_, err := svc.DraftMessage(CommitOptions{})
	if err == nil {
		t.Fatal("expected error from LLM")
	}
}
