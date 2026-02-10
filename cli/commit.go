package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

func init() {
	rootCmd.AddCommand(commitCmd)
}

var commitCmd = &cobra.Command{
	Use:   "commit [text]",
	Short: "Create a commit message from the given diff",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		fmt.Fprintln(cmd.OutOrStdout(), args[0])
		return nil
	},
}
