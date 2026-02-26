package cli

import (
	"devmate/internal/domain"
	"devmate/internal/service"
	"fmt"

	"github.com/spf13/cobra"
)

func NewCommit(cmdType string, short, detailed, explain bool) (service.CommitOptions, error) {
	ct, err := parseCmdType(cmdType)
	if err != nil {
		return service.CommitOptions{}, err
	}
	return service.CommitOptions{domain.Options{ct, parseCmdMode(detailed), explain}}, nil
}

var commitOpts service.CommitOptions

type CommitService interface {
	DraftMessage(opt service.CommitOptions) (string, error)
}

func newCommitCmd(a *App) *cobra.Command {
	validateAndRunCommit := func(cmd *cobra.Command, args []string) error {
		//validate type and prepare options
		var err error
		commitOpts, err = NewCommit(rawCmdType, rawShort, rawDetailed, explain)
		if err != nil {
			return err
		}
		if a.commitService == nil {
			// real construction — wired once you have infra ready
			return domain.ServiceNotInitialized
		}
		msg, err := a.commitService.DraftMessage(commitOpts)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), msg)
		return nil
	}

	commitCmd := &cobra.Command{
		Use:   "commit [-t feat|fix|chore|docs|refactor] [--short|--detailed] [--explain]",
		Short: "Create a commit message from the given diff",
		Long: `Analyzes staged changes (git diff --cached) and drafts a Conventional Commit message.

The command reads the current repository state in read-only mode and generates
a suggested commit message using the configured LLM.

By default, Devmate auto-detects the most appropriate commit type (feat, fix,
chore, docs, refactor). You may explicitly provide a type override using --type.

The output is plain text written to stdout. Devmate never executes git commit
or mutates repository state.

Flags:
  -t, --type string     Override commit type (feat, fix, chore, docs, refactor)
      --explain         Provide reasoning behind the suggested commit message
      --short           Generate a concise commit message (default)
      --detailed        Generate a more descriptive commit message

Note:
  --short and --detailed are mutually exclusive.
  If neither is provided, a short format is used.
`,
		Args: cobra.NoArgs,
		RunE: validateAndRunCommit,
	}

	commitCmd.Flags().StringVarP(
		&rawCmdType,
		"type",
		"t",
		"",
		"force commit type (feat, fix, chore, docs, refactor)",
	)

	commitCmd.Flags().BoolVar(
		&explain,
		"explain",
		false,
		"explain why this commit message was generated",
	)

	commitCmd.Flags().BoolVar(
		&rawShort,
		"short",
		false,
		"generate short single-line commit message (default)",
	)

	commitCmd.Flags().BoolVar(
		&rawDetailed,
		"detailed",
		false,
		"generate detailed commit message with body",
	)

	// Make short and detailed mutually exclusive
	commitCmd.MarkFlagsMutuallyExclusive("short", "detailed")
	return commitCmd
}

func parseCmdType(raw string) (domain.CmdType, error) {
	cmdType, ok := domain.CmdTypeIndex[raw]
	if !ok {
		return domain.Undefined, domain.ErrInvalidCmdType
	}
	return cmdType, nil
}

func parseCmdMode(detailed bool) domain.CmdMode {
	if detailed {
		return domain.Detailed
	}
	return domain.Short
}
