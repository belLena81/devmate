package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(commitCmd)
	commitCmd.Example = `
  devmate commit
  devmate commit -t feat
  devmate commit --type fix --short
  devmate commit --detailed --explain
`
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
	Use:   "commit [text]",
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
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(cmd.OutOrStdout(), args[0])
		return nil
	},
}
