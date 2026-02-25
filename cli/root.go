package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "devmate",
	Short: "Devmate is a read-only developer assistant",
}

type Options struct {
	Type    CmdType
	Mode    CmdMode
	Explain bool
}

var (
	rawCmdType  string
	explain     bool
	rawShort    bool
	rawDetailed bool
)

type CmdType int

const (
	Undefined CmdType = iota
	Feat
	Fix
	Chore
	Docs
	Refactor
	cmdTypeSentinel // cmdTypeSentinel marks the end of the valid CmdType constants.
)

func (t CmdType) String() (string, error) {
	switch t {
	case Undefined:
		return "", nil
	case Feat:
		return "feat", nil
	case Fix:
		return "fix", nil
	case Chore:
		return "chore", nil
	case Docs:
		return "docs", nil
	case Refactor:
		return "refactor", nil
	default:
		return "", ErrInvalidCmdType
	}
}

type CmdMode int

const (
	Short CmdMode = iota
	Detailed
)

var cmdTypeIndex = func() map[string]CmdType {
	index := make(map[string]CmdType)
	for t := Undefined; t < cmdTypeSentinel; t++ {
		str, _ := t.String()
		index[str] = t
	}
	return index
}()

var cmdTypes = [5]string{"feat", "fix", "chore", "docs", "refactor"}

var ErrInvalidCmdType = fmt.Errorf("invalid commit type, must be one of %v", cmdTypes)
var MissingTaskDescription = fmt.Errorf("missing task description")

func (m CmdMode) String() string {
	switch m {
	case Detailed:
		return "detailed"
	default:
		return "short"
	}
}

func Execute() {
	if err := rootCmd.Execute(); err != nil {
		os.Exit(1)
	}
}
