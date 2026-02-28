package cli

import (
	"github.com/spf13/cobra"
)

func buildRootCmd(app *App) *cobra.Command {
	root := &cobra.Command{
		Use:   "devmate",
		Short: "Devmate is a read-only developer assistant",
		Long: `devmate is a CLI tool that streamlines common development workflows.

It helps you standardize commits, create branches from JIRA tickets,
and open pull requests with consistent naming conventions.`,
	}
	root.CompletionOptions.DisableDefaultCmd = true
	root.AddCommand(newCommitCmd(app))
	root.AddCommand(newBranchCmd(app))
	root.AddCommand(newPrCmd(app))
	root.AddCommand(newCacheCmd(app))
	return root
}
