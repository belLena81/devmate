package cli

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
)

func TestEchoCommand(t *testing.T) {
	buf := new(bytes.Buffer)

	root := &cobra.Command{
		Use: "devmate",
	}

	root.SetOut(buf)
	root.SetArgs([]string{"echo", "hello"})
	root.AddCommand(echoCmd)

	err := root.Execute()
	if err != nil {
		t.Fatal(err)
	}

	if buf.String() != "hello\n" {
		t.Fatalf("unexpected output: %q", buf.String())
	}
}
