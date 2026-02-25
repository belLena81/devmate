package cli

import (
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "devmate",
	Short: "Devmate is a read-only developer assistant",
}

var (
	rawCmdType  string
	explain     bool
	rawShort    bool
	rawDetailed bool

	cmdType CmdType
	mode    CmdMode
)

type CmdType int

const (
	Undefined CmdType = iota
	Feat
	Fix
	Chore
	Docs
	Refactor
)

func (t CmdType) String() string {
	switch t {
	case Feat:
		return "feat"
	case Fix:
		return "fix"
	case Chore:
		return "chore"
	case Docs:
		return "docs"
	case Refactor:
		return "refactor"
	default:
		return ""
	}
}

type CmdMode int

const (
	Short CmdMode = iota
	Detailed
)

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
