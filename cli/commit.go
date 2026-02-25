package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

type CommitOptions struct {
	Options
}

func NewCommit(cmdType string, short, detailed, explain bool) (CommitOptions, error) {
	ct, err := parseCmdType(cmdType)
	if err != nil {
		return CommitOptions{}, err
	}
	return CommitOptions{Options{ct, parseCmdMode(detailed), explain}}, nil
}

var CommitOpts CommitOptions

func init() {
	rootCmd.AddCommand(commitCmd)

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
}

var commitCmd = &cobra.Command{
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

func parseCmdType(raw string) (CmdType, error) {
	cmdType, ok := cmdTypeIndex[raw]
	if !ok {
		return Undefined, ErrInvalidCmdType
	}
	return cmdType, nil
}

func parseCmdMode(detailed bool) CmdMode {
	if detailed {
		return Detailed
	}
	return Short
}

func validateAndRunCommit(cmd *cobra.Command, args []string) error {
	//validate type and prepare options
	var err error
	CommitOpts, err = NewCommit(rawCmdType, rawShort, rawDetailed, explain)
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), CommitOpts)
	//call service to run a command
	return nil
}
