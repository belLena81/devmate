package cli

import (
	"devmate/internal/domain"
	"devmate/internal/service"
	"fmt"

	"github.com/spf13/cobra"
)

var BranchOpts service.BranchOptions

func NewBranch(task string, cmdType string, short, detailed, explain bool) (service.BranchOptions, error) {
	ct, err := parseCmdType(cmdType)
	if err != nil {
		return service.BranchOptions{}, err
	}
	if task == "" {
		return service.BranchOptions{}, domain.MissingTaskDescription
	}
	return service.BranchOptions{task, domain.Options{ct, parseCmdMode(detailed), explain}}, nil
}

func newBranchCmd(a *App) *cobra.Command {
	validateAndRunBranch := func(cmd *cobra.Command, args []string) error {
		var err error
		BranchOpts, err = NewBranch(args[0], rawCmdType, rawShort, rawDetailed, explain)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), BranchOpts)
		//call service to run a command
		return nil
	}

	branchCmd := &cobra.Command{
		Use:   "branch [-t feat|fix|chore|docs|refactor] [--short|--detailed] [--explain] text",
		Short: "Create a branch name from the given task description",
		Long: `Generates a branch name from a plain-text task description.

The command takes a single required argument — a short description of the task
(e.g. "add user authentication" or "fix login page crash") — and produces a
branch name following the convention: <type>/<slug>.

By default, Devmate auto-detects the most appropriate branch type (feat, fix,
chore, docs, refactor). You may explicitly provide a type override using --type.

The output is plain text written to stdout. Devmate never creates or switches
branches or mutates repository state.

Flags:
  -t, --type string     Override branch type (feat, fix, chore, docs, refactor)
      --explain         Provide reasoning behind the suggested branch name
      --short           Generate a concise branch name (default)
      --detailed        Generate a more descriptive branch name

Note:
  --short and --detailed are mutually exclusive.
  If neither is provided, a short format is used.
`,
		Args: cobra.ExactArgs(1),
		RunE: validateAndRunBranch,
	}
	//a.rootCmd.AddCommand(branchCmd)

	branchCmd.Flags().StringVarP(
		&rawCmdType,
		"type",
		"t",
		"",
		"force branch task type (feat, fix, chore, docs, refactor)",
	)

	branchCmd.Flags().BoolVar(
		&explain,
		"explain",
		false,
		"explain why this branch name was generated",
	)

	branchCmd.Flags().BoolVar(
		&rawShort,
		"short",
		false,
		"generate concise few words branch name (default)",
	)

	branchCmd.Flags().BoolVar(
		&rawDetailed,
		"detailed",
		false,
		"generate detailed verbose branch name",
	)

	// Make short and detailed mutually exclusive
	branchCmd.MarkFlagsMutuallyExclusive("short", "detailed")

	return branchCmd
}
