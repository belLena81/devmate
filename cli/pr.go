package cli

import (
	"context"
	"devmate/internal/domain"
	"devmate/internal/service"
	"fmt"

	"github.com/spf13/cobra"
)

func NewPr(source, target string, cmdType string, short, detailed, explain, noCache bool) (service.PrOptions, error) {
	ct, err := parseCmdType(cmdType)
	if err != nil {
		return service.PrOptions{}, err
	}
	if source == "" {
		return service.PrOptions{}, domain.ErrMissingSourceBranch
	}
	if target == "" {
		return service.PrOptions{}, domain.ErrMissingTargetBranch
	}
	return service.PrOptions{
		SourceBranch:      source,
		DestinationBranch: target,
		Options:           domain.Options{Type: ct, Mode: parseCmdMode(detailed), Explain: explain, NoCache: noCache},
	}, nil
}

type PrService interface {
	DraftPrDescription(ctx context.Context, opt service.PrOptions) (string, error)
}

func newPrCmd(a *App) *cobra.Command {
	var rawCmdType string
	var explain, rawShort, rawDetailed, noCache bool

	validateAndRunPr := func(cmd *cobra.Command, args []string) error {
		prOpts, err := NewPr(args[0], args[1], rawCmdType, rawShort, rawDetailed, explain, noCache)
		if err != nil {
			return err
		}
		if a.prService == nil {
			return domain.ErrServiceNotInitialized
		}
		pr, err := a.prService.DraftPrDescription(cmd.Context(), prOpts)
		if err != nil {
			return err
		}
		fmt.Fprintln(cmd.OutOrStdout(), pr)
		return nil
	}
	prCmd := &cobra.Command{
		Use:   "pr [-t feat|fix|chore|docs|refactor] [--short|--detailed] [--explain] [--no-cache] target source",
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
      --no-cache        Bypass the response cache and always call the LLM;
                        the fresh response overwrites any existing cached entry

Note:
  --short and --detailed are mutually exclusive.
  If neither is provided, a short format is used.
`,
		Args: cobra.ExactArgs(2),
		RunE: validateAndRunPr,
	}

	prCmd.Flags().StringVarP(
		&rawCmdType,
		"type",
		"t",
		"",
		"force pr description type (feat, fix, chore, docs, refactor)",
	)

	prCmd.Flags().BoolVar(
		&explain,
		"explain",
		false,
		"explain why this pr description was generated",
	)

	prCmd.Flags().BoolVar(
		&rawShort,
		"short",
		false,
		"generate concise bullet points only pr description (default)",
	)

	prCmd.Flags().BoolVar(
		&rawDetailed,
		"detailed",
		false,
		"generate detailed verbose pr description",
	)

	prCmd.Flags().BoolVar(
		&noCache,
		"no-cache",
		false,
		"bypass the response cache and always call the LLM",
	)

	// Make short and detailed mutually exclusive
	prCmd.MarkFlagsMutuallyExclusive("short", "detailed")
	return prCmd
}
