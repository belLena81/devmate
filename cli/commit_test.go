package cli

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/spf13/cobra"
)

func newEchoCmd() *cobra.Command {
	return &cobra.Command{
		Use:  "echo [text]",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintln(cmd.OutOrStdout(), args[0])
			return nil
		},
	}
}

func TestEchoCommand(t *testing.T) {
	buf := new(bytes.Buffer)

	root := &cobra.Command{
		Use: "devmate",
	}

	root.SetOut(buf)
	root.SetArgs([]string{"echo", "hello"})
	root.AddCommand(newEchoCmd())

	err := root.Execute()
	if err != nil {
		t.Fatal(err)
	}

	if buf.String() != "hello\n" {
		t.Fatalf("unexpected output: %q", buf.String())
	}
}
