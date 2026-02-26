package cli_test

// app_test.go uses the external test package (cli_test) so it can only access
// exported identifiers — the same surface a real caller (main.go) would use.
// This keeps the tests honest about the public API of the App.

import (
	"bytes"
	"devmate/internal/domain"
	"devmate/internal/service"
	"errors"
	"strings"
	"testing"

	"devmate/cli"
)

// ---------------------------------------------------------------------------
// Test doubles
// ---------------------------------------------------------------------------

type stubGit struct {
	diff   string
	branch string
	err    error
}

func (s *stubGit) DiffCached() (string, error)                    { return s.diff, s.err }
func (s *stubGit) LogBetween(base, head string) ([]string, error) { return nil, s.err }

type stubLLM struct {
	response string
	err      error
	received string
}

func (s *stubLLM) Generate(prompt string) (string, error) {
	s.received = prompt
	return s.response, s.err
}

// ---------------------------------------------------------------------------
// App construction
// ---------------------------------------------------------------------------

func TestNewApp_ReturnsNonNilApp(t *testing.T) {
	app := cli.NewApp(&stubGit{}, &stubLLM{})
	if app == nil {
		t.Fatal("NewApp returned nil")
	}
}

func TestApp_RootCmd_IsNamed_devmate(t *testing.T) {
	app := cli.NewApp(&stubGit{}, &stubLLM{})
	if app.RootCmd().Name() != "devmate" {
		t.Errorf("expected root command name %q, got %q", "devmate", app.RootCmd().Name())
	}
}

// ---------------------------------------------------------------------------
// Subcommand registration
// ---------------------------------------------------------------------------

func TestApp_CommitCmd_IsRegistered(t *testing.T) {
	app := cli.NewApp(&stubGit{}, &stubLLM{})
	cmd, _, err := app.RootCmd().Find([]string{"commit"})
	if err != nil || cmd.Name() != "commit" {
		t.Fatal("commit command not registered on App")
	}
}

func TestApp_BranchCmd_IsRegistered(t *testing.T) {
	app := cli.NewApp(&stubGit{}, &stubLLM{})
	cmd, _, err := app.RootCmd().Find([]string{"branch"})
	if err != nil || cmd.Name() != "branch" {
		t.Fatal("branch command not registered on App")
	}
}

func TestApp_PrCmd_IsRegistered(t *testing.T) {
	app := cli.NewApp(&stubGit{}, &stubLLM{})
	cmd, _, err := app.RootCmd().Find([]string{"pr"})
	if err != nil || cmd.Name() != "pr" {
		t.Fatal("pr command not registered on App")
	}
}

// ---------------------------------------------------------------------------
// Execute wires through to the service
// ---------------------------------------------------------------------------

func TestApp_Execute_Commit_PrintsLLMResponse(t *testing.T) {
	llm := &stubLLM{response: "feat(auth): add token refresh"}
	app := cli.NewApp(&stubGit{diff: "some diff"}, llm)

	var buf bytes.Buffer
	app.RootCmd().SetOut(&buf)
	app.RootCmd().SetArgs([]string{"commit"})

	if err := app.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(buf.String(), "feat(auth): add token refresh") {
		t.Errorf("expected LLM response in stdout, got %q", buf.String())
	}
}

func TestApp_Execute_Commit_GitError_ReturnsError(t *testing.T) {
	app := cli.NewApp(
		&stubGit{err: errors.New("not a git repo")},
		&stubLLM{},
	)
	app.RootCmd().SetArgs([]string{"commit"})

	if err := app.Execute(); err == nil {
		t.Fatal("expected error when git fails")
	}
}

func TestApp_Execute_Commit_LLMError_ReturnsError(t *testing.T) {
	app := cli.NewApp(
		&stubGit{diff: "some diff"},
		&stubLLM{err: errors.New("LLM unavailable")},
	)
	app.RootCmd().SetArgs([]string{"commit"})

	if err := app.Execute(); err == nil {
		t.Fatal("expected error when LLM fails")
	}
}

// ---------------------------------------------------------------------------
// Each App instance is fully independent — no shared global state
// ---------------------------------------------------------------------------

func TestApp_TwoInstances_DoNotShareState(t *testing.T) {
	llm1 := &stubLLM{response: "feat: one"}
	llm2 := &stubLLM{response: "fix: two"}

	app1 := cli.NewApp(&stubGit{diff: "d1"}, llm1)
	app2 := cli.NewApp(&stubGit{diff: "d2"}, llm2)

	var buf1, buf2 bytes.Buffer
	app1.RootCmd().SetOut(&buf1)
	app2.RootCmd().SetOut(&buf2)

	app1.RootCmd().SetArgs([]string{"commit"})
	app2.RootCmd().SetArgs([]string{"commit"})

	if err := app1.Execute(); err != nil {
		t.Fatalf("app1 error: %v", err)
	}
	if err := app2.Execute(); err != nil {
		t.Fatalf("app2 error: %v", err)
	}

	if !strings.Contains(buf1.String(), "feat: one") {
		t.Errorf("app1: expected %q, got %q", "feat: one", buf1.String())
	}
	if !strings.Contains(buf2.String(), "fix: two") {
		t.Errorf("app2: expected %q, got %q", "fix: two", buf2.String())
	}
}

// ---------------------------------------------------------------------------
// Flag wiring through App.Execute
// ---------------------------------------------------------------------------

func TestApp_Execute_Commit_PassesTypeFlag(t *testing.T) {
	llm := &stubLLM{response: "fix: patch"}
	app := cli.NewApp(&stubGit{diff: "d"}, llm)
	app.RootCmd().SetArgs([]string{"commit", "--type", "fix"})

	if err := app.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestApp_Execute_Commit_InvalidType_ReturnsError(t *testing.T) {
	app := cli.NewApp(&stubGit{diff: "d"}, &stubLLM{})
	app.RootCmd().SetArgs([]string{"commit", "--type", "invalid"})

	err := app.Execute()
	if err == nil {
		t.Fatal("expected error for invalid type")
	}
	if !errors.Is(err, domain.ErrInvalidCmdType) {
		t.Errorf("expected ErrInvalidCmdType, got %v", err)
	}
}

func TestApp_Execute_Commit_ShortAndDetailed_MutuallyExclusive(t *testing.T) {
	app := cli.NewApp(&stubGit{}, &stubLLM{})
	app.RootCmd().SetArgs([]string{"commit", "--short", "--detailed"})

	if err := app.Execute(); err == nil {
		t.Fatal("expected error for mutually exclusive flags")
	}
}

// ---------------------------------------------------------------------------
// Commit service options propagation
// ---------------------------------------------------------------------------

func TestApp_Execute_Commit_DetailedFlag_PropagatesMode(t *testing.T) {
	var capturedOpts service.CommitOptions
	spy := &spyCommitService{}

	app := cli.NewApp(&stubGit{diff: "d"}, &stubLLM{})
	app.RootCmd().SetArgs([]string{"commit", "--detailed"})

	// Replace the service after construction so we can capture options.
	// This is done via the exported InjectCommitService test helper.
	cli.InjectCommitService(app, spy)

	if err := app.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	capturedOpts = spy.receivedOpts
	if capturedOpts.Mode != domain.Detailed {
		t.Errorf("expected Detailed mode, got %v", capturedOpts.Mode)
	}
}

func TestApp_Execute_Commit_TypeFlag_PropagatesType(t *testing.T) {
	spy := &spyCommitService{}
	app := cli.NewApp(&stubGit{diff: "d"}, &stubLLM{})
	app.RootCmd().SetArgs([]string{"commit", "--type", "refactor"})
	cli.InjectCommitService(app, spy)

	if err := app.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spy.receivedOpts.Type != domain.Refactor {
		t.Errorf("expected Refactor type, got %v", spy.receivedOpts.Type)
	}
}

// spyCommitService captures the options passed to DraftMessage.
type spyCommitService struct {
	receivedOpts service.CommitOptions
	response     string
	err          error
}

func (s *spyCommitService) DraftMessage(o service.CommitOptions) (string, error) {
	s.receivedOpts = o
	return s.response, s.err
}
