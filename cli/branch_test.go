package cli

import (
	"devmate/internal/domain"
	"errors"
	"testing"
)

// newBranchApp returns a minimal App suitable for branch command tests.
func newBranchApp() *App {
	app := &App{commitService: &fakeCommitService{}, branchService: &fakeBranchService{}, prService: &fakePrService{}}
	app.rootCmd = buildRootCmd(app)
	return app
}

func TestBranchCmd_IsRegistered(t *testing.T) {
	app := newBranchApp()
	cmd, _, err := app.rootCmd.Find([]string{"branch"})
	if err != nil || cmd.Name() != "branch" {
		t.Fatal("branch command not registered")
	}
}

func TestBranchCmd_Flags(t *testing.T) {
	app := newBranchApp()
	cmd, _, _ := app.rootCmd.Find([]string{"branch"})
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

func TestBranchCmd_FlagDefaults(t *testing.T) {
	app := newBranchApp()
	cmd, _, _ := app.rootCmd.Find([]string{"branch"})
	f := cmd.Flags()

	if f.Lookup("type").DefValue != "" {
		t.Error("--type default should be empty string")
	}
	if f.Lookup("explain").DefValue != "false" {
		t.Error("--explain default should be false")
	}
}

func TestBranchCmd_ShortAndDetailedMutuallyExclusive(t *testing.T) {
	app := newBranchApp()
	app.rootCmd.SetArgs([]string{"branch", "--short", "--detailed", "some task description"})

	if err := app.Execute(); err == nil {
		t.Error("expected error when --short and --detailed used together")
	}
}

func TestBranchCmd_RejectsZeroPositionalArgs(t *testing.T) {
	app := newBranchApp()
	app.rootCmd.SetArgs([]string{"branch"})

	if err := app.Execute(); err == nil {
		t.Error("expected error when no positional arg is passed")
	}
}

func TestBranchCmd_RejectsTwoPositionalArgs(t *testing.T) {
	app := newBranchApp()
	app.rootCmd.SetArgs([]string{"branch", "task one", "task two"})

	if err := app.Execute(); err == nil {
		t.Error("expected error when two positional args are passed")
	}
}

func TestBranchCmd_RunsWithExactOneArg(t *testing.T) {
	app := newBranchApp()
	app.rootCmd.SetArgs([]string{"branch", "some task description"})

	if err := app.Execute(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBranchCmd_InvalidType(t *testing.T) {
	app := newBranchApp()
	app.rootCmd.SetArgs([]string{"branch", "--type", "invalid", "some task description"})

	err := app.Execute()
	if err == nil {
		t.Fatal("expected error for invalid --type value")
	}
	if !errors.Is(err, domain.ErrInvalidCmdType) {
		t.Errorf("expected ErrInvalidCmdType, got %v", err)
	}
}

func TestBranchCmd_ValidTypes(t *testing.T) {
	types := []string{"feat", "fix", "chore", "docs", "refactor"}
	for _, tt := range types {
		t.Run(tt, func(t *testing.T) {
			app := newBranchApp()
			app.rootCmd.SetArgs([]string{"branch", "--type", tt, "some task description"})
			if err := app.Execute(); err != nil {
				t.Errorf("unexpected error for type %q: %v", tt, err)
			}
		})
	}
}

func TestNewBranch_MissingTask(t *testing.T) {
	_, err := NewBranch("", "", false, false, false, false)
	if !errors.Is(err, domain.ErrMissingTaskDescription) {
		t.Errorf("expected MissingTaskDescription, got %v", err)
	}
}

func TestNewBranch_ValidConstruction(t *testing.T) {
	opts, err := NewBranch("add auth", "feat", false, false, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.Task != "add auth" {
		t.Errorf("expected task %q, got %q", "add auth", opts.Task)
	}
	if opts.Type != domain.Feat {
		t.Errorf("expected Feat, got %v", opts.Type)
	}
}

func TestNewBranch_InvalidType(t *testing.T) {
	_, err := NewBranch("task", "invalid", false, false, false, false)
	if !errors.Is(err, domain.ErrInvalidCmdType) {
		t.Errorf("expected ErrInvalidCmdType, got %v", err)
	}
}

// ─── --no-cache flag ──────────────────────────────────────────────────────────

func TestBranchCmd_NoCacheFlag_IsRegistered(t *testing.T) {
	app := newBranchApp()
	cmd, _, _ := app.rootCmd.Find([]string{"branch"})
	if cmd.Flags().Lookup("no-cache") == nil {
		t.Error("missing --no-cache flag on branch command")
	}
}

func TestBranchCmd_NoCacheFlag_DefaultIsFalse(t *testing.T) {
	app := newBranchApp()
	cmd, _, _ := app.rootCmd.Find([]string{"branch"})
	if cmd.Flags().Lookup("no-cache").DefValue != "false" {
		t.Error("--no-cache default should be false")
	}
}

func TestNewBranch_NoCacheTrue_PropagatedToOptions(t *testing.T) {
	opts, err := NewBranch("add auth", "feat", false, false, false, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !opts.NoCache {
		t.Error("expected NoCache=true when noCache argument is true")
	}
}

func TestNewBranch_NoCacheFalse_DefaultBehaviour(t *testing.T) {
	opts, err := NewBranch("add auth", "", false, false, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.NoCache {
		t.Error("expected NoCache=false by default")
	}
}

func TestBranchCmd_NoCacheFlag_PropagatesNoCache(t *testing.T) {
	spy := &fakeBranchService{}
	app := newBranchApp()
	InjectBranchService(app, spy)
	app.rootCmd.SetArgs([]string{"branch", "--no-cache", "add auth feature"})

	if err := app.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !spy.options.NoCache {
		t.Error("expected NoCache=true to be passed to service when --no-cache is set")
	}
}

func TestBranchCmd_WithoutNoCacheFlag_NoCacheIsFalse(t *testing.T) {
	spy := &fakeBranchService{}
	app := newBranchApp()
	InjectBranchService(app, spy)
	app.rootCmd.SetArgs([]string{"branch", "add auth feature"})

	if err := app.Execute(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if spy.options.NoCache {
		t.Error("expected NoCache=false when --no-cache is not set")
	}
}
