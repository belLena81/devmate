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
	var f generateFlags

	cmd := &cobra.Command{
		Use:   "pr <source> <target>",
		Short: "Draft a pull request description from the commits between two branches",
		Long: `Generates a pull request title and description from the git log between the
source branch (your feature branch) and the target branch (e.g. main).

By default the PR type (feat, fix, chore, docs, refactor) is auto-detected from
the commit history; use --type to force one. The text is written to stdout.
devmate never opens, creates, or modifies pull requests.`,
		Example: `  devmate pr feat/payment-webhook-retry main
  devmate pr feat/payment-webhook-retry main --detailed
  devmate pr feat/payment-webhook-retry develop --type fix --no-cache`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			opts, err := NewPr(args[0], args[1], f.cmdType, f.short, f.detailed, f.explain, f.noCache)
			if err != nil {
				return err
			}
			if a.prService == nil {
				return domain.ErrServiceNotInitialized
			}
			pr, err := a.prService.DraftPrDescription(cmd.Context(), opts)
			if err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), pr)
			return nil
		},
	}

	bindGenerateFlags(cmd, &f, "PR description")
	return cmd
}
