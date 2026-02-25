package cli

import (
	"bytes"
	"errors"
	"testing"

	"github.com/spf13/pflag"
)

func resetFlags() {
	rawCmdType = ""
	explain = false
	rawShort = false
	rawDetailed = false
	commitCmd.Flags().VisitAll(func(f *pflag.Flag) {
		f.Value.Set(f.DefValue)
		f.Changed = false
	})
}

func TestCommitCmd_IsRegistered(t *testing.T) {
	cmd, _, err := rootCmd.Find([]string{"commit"})
	if err != nil || cmd.Name() != "commit" {
		t.Fatal("commit command not registered")
	}
}

func TestCommitCmd_Flags(t *testing.T) {
	f := commitCmd.Flags()

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
	if commitCmd.Flags().Lookup("type").DefValue != "" {
		t.Error("--type default should be empty string")
	}
	if commitCmd.Flags().Lookup("explain").DefValue != "false" {
		t.Error("--explain default should be false")
	}
}

func TestCommitCmd_ShortAndDetailedMutuallyExclusive(t *testing.T) {
	rootCmd.SetArgs([]string{"commit", "--short", "--detailed"})
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		resetFlags()
	})
	err := rootCmd.Execute()
	if err == nil {
		t.Error("expected error when --short and --detailed used together")
	}
}

func TestCommitCmd_RejectsPositionalArgs(t *testing.T) {
	rootCmd.SetArgs([]string{"commit", "some-arg"})
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		resetFlags()
	})

	err := rootCmd.Execute()
	if err == nil {
		t.Error("expected error when positional args are passed")
	}
}

func TestCommitCmd_RunsWithoutArgs(t *testing.T) {
	rootCmd.SetArgs([]string{"commit"})
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		resetFlags()
	})

	err := rootCmd.Execute()
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestCommitCmd_InvalidType(t *testing.T) {
	buf := &bytes.Buffer{}
	rootCmd.SetErr(buf)
	rootCmd.SetArgs([]string{"commit", "--type", "invalid"})
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		resetFlags()
	})

	err := rootCmd.Execute()
	if err == nil {
		t.Error("expected error for invalid --type value")
	}
	if !errors.Is(err, ErrInvalidCmdType) {
		t.Errorf("expected ErrInvalidCmdType, got %v", err)
	}
}

func TestCommitCmd_ValidTypes(t *testing.T) {
	types := []string{"feat", "fix", "chore", "docs", "refactor"}
	for _, tt := range types {
		t.Run(tt, func(t *testing.T) {
			rootCmd.SetArgs([]string{"commit", "--type", tt})
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
