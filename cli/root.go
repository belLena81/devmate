package cli

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "devmate",
	Short: "Devmate is a read-only developer assistant",
	Long: `devmate is a CLI tool that streamlines common development workflows.

It helps you standardize commits, create branches from JIRA tickets,
and open pull requests with consistent naming conventions.`,
}
var (
	rawCmdType  string
	explain     bool
	rawShort    bool
	rawDetailed bool
)

func init() {
	rootCmd.CompletionOptions.DisableDefaultCmd = true
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
