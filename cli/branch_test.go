package cli

import (
	"bytes"
	"devmate/internal/domain"
	"errors"
	"testing"
)

func TestBranchCmd_IsRegistered(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"branch"})
	if err != nil || cmd.Name() != "branch" {
		t.Fatal("branch command not registered")
	}
}

func TestBranchCmd_Flags(t *testing.T) {
	f := branchCmd.Flags()

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
	if branchCmd.Flags().Lookup("type").DefValue != "" {
		t.Error("--type default should be empty string")
	}
	if branchCmd.Flags().Lookup("explain").DefValue != "false" {
		t.Error("--explain default should be false")
	}
}

func TestBranchCmd_ShortAndDetailedMutuallyExclusive(t *testing.T) {
	rootCmd.SetArgs([]string{"branch", "--short", "--detailed", "some task description"})
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		resetFlags()
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Error("expected error when --short and --detailed used together")
	}
}

func TestBranchCmd_RejectsSecondPositionalArg(t *testing.T) {
	rootCmd.SetArgs([]string{"branch", "some task description", "another task description"})
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		resetFlags()
	})

	err := rootCmd.Execute()
	if err == nil {
		t.Error("expected error when positional args longer than 1")
	}
	rootCmd.SetArgs([]string{"branch"})
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		resetFlags()
	})

	err = rootCmd.Execute()
	if err == nil {
		t.Error("expected error when positional args shorter than 1")
	}
}

func TestBranchCmd_RunsWithExactOneArg(t *testing.T) {
	rootCmd.SetArgs([]string{"branch", "some task description"})
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		resetFlags()
	})

	err := rootCmd.Execute()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestBranchCmd_InvalidType(t *testing.T) {
	buf := &bytes.Buffer{}
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"branch", "--type", "invalid", "some task description"})
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		resetFlags()
	})

	err := rootCmd.Execute()
	if err == nil {
		t.Error("expected error for invalid --type value")
	}
	if !errors.Is(err, domain.ErrInvalidCmdType) {
		t.Errorf("expected ErrInvalidCmdType, got %v", err)
	}
}

func TestBranchCmd_ValidTypes(t *testing.T) {
	types := []string{"feat", "fix", "chore", "docs", "refactor"}
	for _, tt := range types {
		t.Run(tt, func(t *testing.T) {
			rootCmd.SetArgs([]string{"branch", "--type", tt, "some task description"})
			t.Cleanup(func() {
				rootCmd.SetArgs(nil)
				resetFlags()
			})
			if err := rootCmd.Execute(); err != nil {
				t.Errorf("unexpected error for type %q: %v", tt, err)
			}
		})
	}
}

func TestNewBranch_MissingTask(t *testing.T) {
	_, err := NewBranch("", "", false, false, false)
	if !errors.Is(err, domain.MissingTaskDescription) {
		t.Errorf("expected MissingTaskDescription, got %v", err)
	}
}
