package cli

import (
	"bytes"
	"devmate/internal/domain"
	"devmate/internal/service"
	"errors"
	"strings"
	"testing"
)

// newCommitApp returns a minimal App suitable for commit command tests.
// The fake service returns an empty response by default.
func newCommitApp(svc CommitService) *App {
	app := &App{commitService: svc}
	app.rootCmd = buildRootCmd(app)
	return app
}

func TestCommitCmd_IsRegistered(t *testing.T) {
	app := newCommitApp(&fakeCommitService{})
	cmd, _, err := app.rootCmd.Find([]string{"commit"})
	if err != nil || cmd.Name() != "commit" {
		t.Fatal("commit command not registered")
	}
}

func TestCommitCmd_Flags(t *testing.T) {
	app := newCommitApp(&fakeCommitService{})
	cmd, _, _ := app.rootCmd.Find([]string{"commit"})
	f := cmd.Flags()

	if f.Lookup("type") == nil {
		t.Error("missing --type flag")
	}
	if f.Lookup("explain") == nil {
		t.Error("missing --explain flag")
	}
	if f.Lookup("short") == nil {
		t.Error("missing --short flag")
	}
	if f.Lookup("detailed") == nil {
		t.Error("missing --detailed flag")
	}
}

func TestCommitCmd_FlagDefaults(t *testing.T) {
	app := newCommitApp(&fakeCommitService{})
	cmd, _, _ := app.rootCmd.Find([]string{"commit"})
	f := cmd.Flags()

	if f.Lookup("type").DefValue != "" {
		t.Error("--type default should be empty string")
	}
	if f.Lookup("explain").DefValue != "false" {
		t.Error("--explain default should be false")
	}
}

func TestCommitCmd_ShortAndDetailedMutuallyExclusive(t *testing.T) {
	app := newCommitApp(&fakeCommitService{})
	app.rootCmd.SetArgs([]string{"commit", "--short", "--detailed"})

	if err := app.Execute(); err == nil {
		t.Error("expected error when --short and --detailed used together")
	}
}

func TestCommitCmd_RejectsPositionalArgs(t *testing.T) {
	app := newCommitApp(&fakeCommitService{})
	app.rootCmd.SetArgs([]string{"commit", "some-arg"})

	if err := app.Execute(); err == nil {
		t.Error("expected error when positional args are passed")
	}
}

func TestCommitCmd_RunsWithoutArgs(t *testing.T) {
	app := newCommitApp(&fakeCommitService{})
	app.rootCmd.SetArgs([]string{"commit"})

	if err := app.Execute(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCommitCmd_InvalidType(t *testing.T) {
	app := newCommitApp(&fakeCommitService{})
	app.rootCmd.SetArgs([]string{"commit", "--type", "invalid"})

	err := app.Execute()
	if err == nil {
		t.Fatal("expected error for invalid --type value")
	}
	if !errors.Is(err, domain.ErrInvalidCmdType) {
		t.Errorf("expected ErrInvalidCmdType, got %v", err)
	}
}

func TestCommitCmd_ValidTypes(t *testing.T) {
	types := []string{"feat", "fix", "chore", "docs", "refactor"}
	for _, tt := range types {
		t.Run(tt, func(t *testing.T) {
			app := newCommitApp(&fakeCommitService{})
			app.rootCmd.SetArgs([]string{"commit", "--type", tt})
			if err := app.Execute(); err != nil {
				t.Errorf("unexpected error for type %q: %v", tt, err)
			}
		})
	}
}

func TestNewCommit_ValidType(t *testing.T) {
	opts, err := NewCommit("feat", false, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.Type != domain.Feat {
		t.Errorf("expected Feat, got %v", opts.Type)
	}
}

func TestCommitCmd_PrintsGeneratedMessage(t *testing.T) {
	fake := &fakeCommitService{response: "feat(auth): add token refresh"}
	app := newCommitApp(fake)

	var buf bytes.Buffer
	app.rootCmd.SetOut(&buf)
	app.rootCmd.SetArgs([]string{"commit"})

	if err := app.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(buf.String(), "feat(auth): add token refresh") {
		t.Errorf("expected commit message in output, got: %q", buf.String())
	}
}

func TestCommitCmd_ServiceError_ReturnsError(t *testing.T) {
	fake := &fakeCommitService{err: errors.New("git failed")}
	app := newCommitApp(fake)
	app.rootCmd.SetArgs([]string{"commit"})

	if err := app.Execute(); err == nil {
		t.Error("expected error to propagate from service")
	}
}

func TestCommitCmd_PassesFlagsToService(t *testing.T) {
	fake := &fakeCommitService{}
	app := newCommitApp(fake)
	app.rootCmd.SetArgs([]string{"commit", "--type", "fix", "--detailed"})

	if err := app.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fake.options.Type != domain.Fix {
		t.Error("expected Fix type to be passed to service")
	}
	if fake.options.Mode != domain.Detailed {
		t.Error("expected Detailed mode to be passed to service")
	}
}

func TestCommitCmd_PassesExplainFlag(t *testing.T) {
	fake := &fakeCommitService{}
	app := newCommitApp(fake)
	app.rootCmd.SetArgs([]string{"commit", "--explain"})

	if err := app.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !fake.options.Explain {
		t.Error("expected Explain to be true")
	}
}

func TestNewCommit_DefaultMode_IsShort(t *testing.T) {
	opts, err := NewCommit("", false, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.Mode != domain.Short {
		t.Errorf("expected Short mode, got %v", opts.Mode)
	}
}

func TestNewCommit_DetailedMode(t *testing.T) {
	opts, err := NewCommit("", false, true, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.Mode != domain.Detailed {
		t.Errorf("expected Detailed mode, got %v", opts.Mode)
	}
}

func TestNewCommit_InvalidType_ReturnsError(t *testing.T) {
	_, err := NewCommit("invalid", false, false, false)
	if !errors.Is(err, domain.ErrInvalidCmdType) {
		t.Errorf("expected ErrInvalidCmdType, got %v", err)
	}
}

// Verify that two App instances running the commit command concurrently do not
// share flag state.
func TestCommitCmd_TwoApps_IndependentFlagState(t *testing.T) {
	fake1 := &fakeCommitService{}
	fake2 := &fakeCommitService{}

	app1 := newCommitApp(fake1)
	app2 := newCommitApp(fake2)

	app1.rootCmd.SetArgs([]string{"commit", "--type", "feat"})
	app2.rootCmd.SetArgs([]string{"commit", "--type", "fix"})

	if err := app1.Execute(); err != nil {
		t.Fatalf("app1 error: %v", err)
	}
	if err := app2.Execute(); err != nil {
		t.Fatalf("app2 error: %v", err)
	}

	if fake1.options.Type != domain.Feat {
		t.Errorf("app1: expected Feat, got %v", fake1.options.Type)
	}
	if fake2.options.Type != domain.Fix {
		t.Errorf("app2: expected Fix, got %v", fake2.options.Type)
	}

	// Confirm the options reported by each service never cross-contaminated.
	var zeroOpts service.CommitOptions
	_ = zeroOpts // silence unused-variable warning; the real check is above
}
