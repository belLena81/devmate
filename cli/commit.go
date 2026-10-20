package cli

import (
	"context"
	"devmate/internal/domain"
	"devmate/internal/service"
	"fmt"

	"github.com/spf13/cobra"
)

func NewCommit(cmdType string, short, detailed, explain, noCache bool) (service.CommitOptions, error) {
	ct, err := parseCmdType(cmdType)
	if err != nil {
		return service.CommitOptions{}, err
	}
	return service.CommitOptions{Options: domain.Options{Type: ct, Mode: parseCmdMode(detailed), Explain: explain, NoCache: noCache}}, nil
}

type CommitService interface {
	DraftMessage(ctx context.Context, opt service.CommitOptions) (string, error)
}

func newCommitCmd(a *App) *cobra.Command {
	var f generateFlags

	cmd := &cobra.Command{
		Use:   "commit",
		Short: "Draft a commit message from the staged diff",
		Long: `Analyzes staged changes (git diff --cached) and drafts a Conventional Commit message.

By default the commit type (feat, fix, chore, docs, refactor) is auto-detected;
use --type to force one. The message is written to stdout. devmate never runs
git commit or mutates repository state.`,
		Example: `  devmate commit
  devmate commit --type fix --detailed
  devmate commit --explain --no-cache`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			opts, err := NewCommit(f.cmdType, f.short, f.detailed, f.explain, f.noCache)
			if err != nil {
				return err
			}
			if a.commitService == nil {
				return domain.ErrServiceNotInitialized
			}
			msg, err := a.commitService.DraftMessage(cmd.Context(), opts)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), msg)
			return nil
		},
	}

	bindGenerateFlags(cmd, &f, "commit message")
	return cmd
}
