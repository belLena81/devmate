package cli

import (
	"bytes"
	"devmate/internal/domain"
	"errors"
	"strings"
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
	branchCmd.Flags().VisitAll(func(f *pflag.Flag) {
		f.Value.Set(f.DefValue)
		f.Changed = false
	})
	prCmd.Flags().VisitAll(func(f *pflag.Flag) {
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
	t.Cleanup(func() {
		rootCmd.SetArgs(nil)
		resetFlags()
	})
	rootCmd.SetArgs([]string{"commit"})
	cmdService = &fakeCommitService{}

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
	if !errors.Is(err, domain.ErrInvalidCmdType) {
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

func TestNewCommit_ValidType(t *testing.T) {
	opts, err := NewCommit("feat", false, false, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.Type != domain.Feat {
		t.Errorf("expected Feat, got %v", opts.Type)
	}
}

func TestRunCommit_PrintsGeneratedMessage(t *testing.T) {
	var buf bytes.Buffer
	rootCmd.SetOut(&buf)
	t.Cleanup(func() { rootCmd.SetOut(nil) })

	// inject fake service
	cmdService = &fakeCommitService{
		response: "feat(auth): add token refresh",
	}
	t.Cleanup(func() { cmdService = nil })

	rootCmd.SetArgs([]string{"commit"})
	t.Cleanup(func() { rootCmd.SetArgs(nil) })

	if err := rootCmd.Execute(); err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(buf.String(), "feat(auth): add token refresh") {
		t.Errorf("expected commit message in output, got: %q", buf.String())
	}
}

func TestRunCommit_ServiceError_ReturnsError(t *testing.T) {
	cmdService = &fakeCommitService{
		err: errors.New("git failed"),
	}
	t.Cleanup(func() { cmdService = nil })

	rootCmd.SetArgs([]string{"commit"})
	t.Cleanup(func() { rootCmd.SetArgs(nil) })

	if err := rootCmd.Execute(); err == nil {
		t.Error("expected error to propagate from service")
	}
}

func TestRunCommit_PassesFlagsToService(t *testing.T) {
	fake := &fakeCommitService{}
	cmdService = fake
	t.Cleanup(func() { cmdService = nil })

	rootCmd.SetArgs([]string{"commit", "--type", "fix", "--detailed"})
	t.Cleanup(func() { rootCmd.SetArgs(nil) })

	rootCmd.Execute()

	if fake.options.Type != domain.Fix {
		t.Error("expected Fix type to be passed to service")
	}
	if fake.options.Mode != domain.Detailed {
		t.Error("expected Detailed mode to be passed to service")
	}
}
