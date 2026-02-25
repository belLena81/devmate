package cli

import (
	"testing"
)

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
	t.Cleanup(func() { rootCmd.SetArgs(nil) })
	err := rootCmd.Execute()
	if err == nil {
		t.Error("expected error when --short and --detailed used together")
	}
}
