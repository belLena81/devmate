package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

type PrOptions struct {
	SourceBranch      string
	DestinationBranch string
	Options
}

var PrOpts PrOptions

func NewPr(source, target string, cmdType string, short, detailed, explain bool) (PrOptions, error) {
	ct, err := parseCmdType(cmdType)
	if err != nil {
		return PrOptions{}, err
	}
	if source == "" {
		return PrOptions{}, MissingSourceBranch
	}
	if target == "" {
		return PrOptions{}, MissingTargetBranch
	}
	return PrOptions{source, target, Options{ct, parseCmdMode(detailed), explain}}, nil
}

var prCmd = &cobra.Command{
	Use:   "pr [-t feat|fix|chore|docs|refactor] [--short|--detailed] [--explain] target source",
	Short: "Create a pr text from git log between heads given branch names",
	Long: `Generates pull request title and description from the git log between two branches.

The command takes two required arguments — the source branch (your feature branch)
and the target branch (e.g. main or develop) — and produces a PR title and body
summarizing the changes between them.

Devmate reads the git log in read-only mode and never opens, creates, or modifies
pull requests or repository state.

By default, Devmate auto-detects the most appropriate PR type (feat, fix,
chore, docs, refactor) from the commit history. You may explicitly provide
a type override using --type.

Arguments:
  source    The branch containing your changes (e.g. feature/add-auth)
  target    The branch to merge into (e.g. main, develop)

Flags:
  -t, --type string     Override PR type (feat, fix, chore, docs, refactor)
      --explain         Provide reasoning behind the generated PR description
      --short           Generate a concise PR title and summary (default)
      --detailed        Generate a detailed PR description with full change breakdown

Note:
  --short and --detailed are mutually exclusive.
  If neither is provided, a short format is used.
`,
	Args: cobra.ExactArgs(2),
	RunE: validateAndRunPr,
}

func init() {
	rootCmd.AddCommand(prCmd)

	prCmd.Flags().StringVarP(
		&rawCmdType,
		"type",
		"t",
		"",
		"force branch task type (feat, fix, chore, docs, refactor)",
	)

	prCmd.Flags().BoolVar(
		&explain,
		"explain",
		false,
		"explain why this branch name was generated",
	)

	prCmd.Flags().BoolVar(
		&rawShort,
		"short",
		false,
		"generate concise few words branch name (default)",
	)

	prCmd.Flags().BoolVar(
		&rawDetailed,
		"detailed",
		false,
		"generate detailed verbose branch name",
	)

	// Make short and detailed mutually exclusive
	prCmd.MarkFlagsMutuallyExclusive("short", "detailed")
}

func validateAndRunPr(cmd *cobra.Command, args []string) error {
	var err error
	PrOpts, err = NewPr(args[0], args[1], rawCmdType, rawShort, rawDetailed, explain)
	if err != nil {
		return err
	}
	fmt.Fprintln(cmd.OutOrStdout(), PrOpts)
	//call service to run a command
	return nil
}
