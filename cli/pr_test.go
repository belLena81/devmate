package cli

import (
	"bytes"
	"devmate/internal/domain"
	"errors"
	"testing"
)

func TestPrCmd_IsRegistered(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"pr"})
	if err != nil || cmd.Name() != "pr" {
		t.Fatal("pr command not registered")
	}
}

func TestPrCmd_Flags(t *testing.T) {
	f := prCmd.Flags()

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
	if prCmd.Flags().Lookup("type").DefValue != "" {
		t.Error("--type default should be empty string")
	}
	if prCmd.Flags().Lookup("explain").DefValue != "false" {
		t.Error("--explain default should be false")
	}
}

func TestPrCmd_ShortAndDetailedMutuallyExclusive(t *testing.T) {
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		resetFlags()
	})
	rootCmd.SetArgs([]string{"pr", "--short", "--detailed", "source", "target"})
	err := rootCmd.Execute()
	if err == nil {
		t.Error("expected error when --short and --detailed used together")
	}
}

func TestPrCmd_RejectsNonTwoPositionalArg(t *testing.T) {
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		resetFlags()
	})
	rootCmd.SetArgs([]string{"pr", "target"})

	err := rootCmd.Execute()
	if err == nil {
		t.Error("expected error when positional args shorter than 2")
	}
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		resetFlags()
	})
	rootCmd.SetArgs([]string{"pr"})

	err = rootCmd.Execute()
	if err == nil {
		t.Error("expected error when positional args shorter than 2")
	}

	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		resetFlags()
	})
	rootCmd.SetArgs([]string{"pr", "target", "source", "branch name"})

	err = rootCmd.Execute()
	if err == nil {
		t.Error("expected error when positional args longer than 2")
	}
}

func TestPrCmd_RunsWithExactTwoArg(t *testing.T) {
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		resetFlags()
	})
	rootCmd.SetArgs([]string{"pr", "target", "source", "--explain", "--detailed"})

	err := rootCmd.Execute()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestPrCmd_InvalidType(t *testing.T) {
	buf := &bytes.Buffer{}
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		resetFlags()
	})
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"pr", "--type", "invalid", "target", "source"})

	err := rootCmd.Execute()
	if err == nil {
		t.Error("expected error for invalid --type value")
	}
	if !errors.Is(err, domain.ErrInvalidCmdType) {
		t.Errorf("expected ErrInvalidCmdType, got %v", err)
	}
}

func TestPrCmd_ValidTypes(t *testing.T) {
	types := []string{"feat", "fix", "chore", "docs", "refactor"}
	for _, tt := range types {
		t.Run(tt, func(t *testing.T) {
			t.Cleanup(func() {
				rootCmd.SetArgs(nil)
				resetFlags()
			})
			rootCmd.SetArgs([]string{"pr", "--type", tt, "target", "source"})
			if err := rootCmd.Execute(); err != nil {
				t.Errorf("unexpected error for type %q: %v", tt, err)
			}
		})
	}
}

func TestNewPr_MissingTarget(t *testing.T) {
	_, err := NewPr("source", "", "", true, false, false)
	if !errors.Is(err, domain.MissingTargetBranch) {
		t.Errorf("expected MissingTargetBranch, got %v", err)
	}
}

func TestNewPr_MissingSource(t *testing.T) {
	_, err := NewPr("", "target", "", true, false, false)
	if !errors.Is(err, domain.MissingSourceBranch) {
		t.Errorf("expected MissingSourceBranch, got %v", err)
	}
}

func TestNewPr_ValidConstruction(t *testing.T) {
	opts, err := NewPr("main", "feature/foo", "feat", false, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.SourceBranch != "main" || opts.DestinationBranch != "feature/foo" {
		t.Error("branch names not set correctly")
	}
	if opts.Type != domain.Feat {
		t.Error("type not set correctly")
	}
}

func TestNewPr_InvalidType(t *testing.T) {
	_, err := NewPr("main", "feature/foo", "invalid", false, false, false)
	if !errors.Is(err, domain.ErrInvalidCmdType) {
		t.Errorf("expected ErrInvalidCmdType, got %v", err)
	}
}
