package service

import (
	"errors"
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
	return "", nil
}

type fakeLLM struct {
	response   string
	onGenerate func(string)
}

func (f *fakeLLM) Generate(prompt string) (string, error) {
	if f.onGenerate != nil {
		f.onGenerate(prompt)
	}
	return f.response, nil
}

func TestCommitService_DraftsMessage(t *testing.T) {
	svc := Service{
		Git: &fakeGit{
			diff: "diff --git a/a.go b/a.go",
		},
		LLM: &fakeLLM{
			response: "feat: add new feature",
		},
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
	}

	opts := CommitOptions{}

	_, err := svc.DraftMessage(opts)
	if err == nil {
		t.Fatal("expected error")
	}
}
