package cli

import (
	"devmate/internal/domain"
	"errors"
	"testing"
)

// newPrApp returns a minimal App suitable for pr command tests.
func newPrApp() *App {
	app := &App{commitService: &fakeCommitService{}, branchService: &fakeBranchService{}, prService: &fakePrService{}}
	app.rootCmd = buildRootCmd(app)
	return app
}

func TestPrCmd_IsRegistered(t *testing.T) {
	app := newPrApp()
	cmd, _, err := app.rootCmd.Find([]string{"pr"})
	if err != nil || cmd.Name() != "pr" {
		t.Fatal("pr command not registered")
	}
}

func TestPrCmd_Flags(t *testing.T) {
	app := newPrApp()
	cmd, _, _ := app.rootCmd.Find([]string{"pr"})
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

func TestPrCmd_FlagDefaults(t *testing.T) {
	app := newPrApp()
	cmd, _, _ := app.rootCmd.Find([]string{"pr"})
	f := cmd.Flags()

	if f.Lookup("type").DefValue != "" {
		t.Error("--type default should be empty string")
	}
	if f.Lookup("explain").DefValue != "false" {
		t.Error("--explain default should be false")
	}
}

func TestPrCmd_ShortAndDetailedMutuallyExclusive(t *testing.T) {
	app := newPrApp()
	app.rootCmd.SetArgs([]string{"pr", "--short", "--detailed", "feature/foo", "main"})

	if err := app.Execute(); err == nil {
		t.Error("expected error when --short and --detailed used together")
	}
}

func TestPrCmd_RejectsOnePositionalArg(t *testing.T) {
	app := newPrApp()
	app.rootCmd.SetArgs([]string{"pr", "feature/foo"})

	if err := app.Execute(); err == nil {
		t.Error("expected error when only one positional arg is passed")
	}
}

func TestPrCmd_RejectsZeroPositionalArgs(t *testing.T) {
	app := newPrApp()
	app.rootCmd.SetArgs([]string{"pr"})

	if err := app.Execute(); err == nil {
		t.Error("expected error when no positional args are passed")
	}
}

func TestPrCmd_RejectsThreePositionalArgs(t *testing.T) {
	app := newPrApp()
	app.rootCmd.SetArgs([]string{"pr", "feature/foo", "main", "extra"})

	if err := app.Execute(); err == nil {
		t.Error("expected error when three positional args are passed")
	}
}

func TestPrCmd_RunsWithExactTwoArgs(t *testing.T) {
	app := newPrApp()
	// args[0] = source, args[1] = destination
	app.rootCmd.SetArgs([]string{"pr", "feature/foo", "main"})

	if err := app.Execute(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPrCmd_RunsWithFlags(t *testing.T) {
	app := newPrApp()
	app.rootCmd.SetArgs([]string{"pr", "--explain", "--detailed", "feature/foo", "main"})

	if err := app.Execute(); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPrCmd_InvalidType(t *testing.T) {
	app := newPrApp()
	app.rootCmd.SetArgs([]string{"pr", "--type", "invalid", "feature/foo", "main"})

	err := app.Execute()
	if err == nil {
		t.Fatal("expected error for invalid --type value")
	}
	if !errors.Is(err, domain.ErrInvalidCmdType) {
		t.Errorf("expected ErrInvalidCmdType, got %v", err)
	}
}

func TestPrCmd_ValidTypes(t *testing.T) {
	types := []string{"feat", "fix", "chore", "docs", "refactor"}
	for _, tt := range types {
		t.Run(tt, func(t *testing.T) {
			app := newPrApp()
			app.rootCmd.SetArgs([]string{"pr", "--type", tt, "feature/foo", "main"})
			if err := app.Execute(); err != nil {
				t.Errorf("unexpected error for type %q: %v", tt, err)
			}
		})
	}
}

// ─── NewPr unit tests ────────────────────────────────────────────────────────

func TestNewPr_MissingSource(t *testing.T) {
	_, err := NewPr("", "main", "", false, false, false)
	if !errors.Is(err, domain.ErrMissingSourceBranch) {
		t.Errorf("expected MissingSourceBranch, got %v", err)
	}
}

func TestNewPr_MissingDestination(t *testing.T) {
	_, err := NewPr("feature/foo", "", "", false, false, false)
	if !errors.Is(err, domain.ErrMissingTargetBranch) {
		t.Errorf("expected MissingTargetBranch, got %v", err)
	}
}

func TestNewPr_ValidConstruction(t *testing.T) {
	opts, err := NewPr("feature/foo", "main", "feat", false, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Arg order: source first, destination second.
	if opts.SourceBranch != "feature/foo" {
		t.Errorf("expected SourceBranch %q, got %q", "feature/foo", opts.SourceBranch)
	}
	if opts.DestinationBranch != "main" {
		t.Errorf("expected DestinationBranch %q, got %q", "main", opts.DestinationBranch)
	}
	if opts.Type != domain.Feat {
		t.Errorf("expected Feat, got %v", opts.Type)
	}
}

func TestNewPr_InvalidType(t *testing.T) {
	_, err := NewPr("feature/foo", "main", "invalid", false, false, false)
	if !errors.Is(err, domain.ErrInvalidCmdType) {
		t.Errorf("expected ErrInvalidCmdType, got %v", err)
	}
}

func TestPrCmd_ArgOrder_SourceFirst_DestinationSecond(t *testing.T) {
	opts, err := NewPr("feature/login", "main", "", false, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.SourceBranch != "feature/login" {
		t.Errorf("first arg should be source, got SourceBranch=%q", opts.SourceBranch)
	}
	if opts.DestinationBranch != "main" {
		t.Errorf("second arg should be destination, got DestinationBranch=%q", opts.DestinationBranch)
	}
}
