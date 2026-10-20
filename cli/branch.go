package cli

import (
	"context"
	"devmate/internal/domain"
	"devmate/internal/service"
	"fmt"

	"github.com/spf13/cobra"
)

func NewBranch(task string, cmdType string, short, detailed, explain, noCache bool) (service.BranchOptions, error) {
	ct, err := parseCmdType(cmdType)
	if err != nil {
		return service.BranchOptions{}, err
	}
	if task == "" {
		return service.BranchOptions{}, domain.ErrMissingTaskDescription
	}
	return service.BranchOptions{Task: task, Options: domain.Options{Type: ct, Mode: parseCmdMode(detailed), Explain: explain, NoCache: noCache}}, nil
}

type BranchService interface {
	DraftBranchName(ctx context.Context, opt service.BranchOptions) (string, error)
}

func newBranchCmd(a *App) *cobra.Command {
	var f generateFlags

	cmd := &cobra.Command{
		Use:   "branch <description>",
		Short: "Draft a branch name from a task description",
		Long: `Generates a branch name of the form <type>/<slug> from a plain-text task
description such as "add user authentication" or "fix login page crash".

By default the branch type (feat, fix, chore, docs, refactor) is auto-detected;
use --type to force one. The name is written to stdout. devmate never creates
or switches branches.`,
		Example: `  devmate branch "add retry logic to payment webhook handler"
  devmate branch "fix login crash on empty password" --type fix
  devmate branch "add retry logic" --detailed --no-cache`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := NewBranch(args[0], f.cmdType, f.short, f.detailed, f.explain, f.noCache)
			if err != nil {
				return err
			}
			if a.branchService == nil {
				return domain.ErrServiceNotInitialized
			}
			name, err := a.branchService.DraftBranchName(cmd.Context(), opts)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), name)
			return nil
		},
	}

	bindGenerateFlags(cmd, &f, "branch name")
	return cmd
}
