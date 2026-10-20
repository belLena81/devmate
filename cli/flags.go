package cli

import (
	"devmate/internal/domain"
	"slices"
	"strings"

	"github.com/spf13/cobra"
)

// generateFlags are the flags shared by the commit, branch and pr commands.
type generateFlags struct {
	cmdType  string
	short    bool
	detailed bool
	explain  bool
	noCache  bool
}

func bindGenerateFlags(cmd *cobra.Command, f *generateFlags, noun string) {
	types := strings.Join(cmdTypeNames(), ", ")
	fl := cmd.Flags()
	fl.StringVarP(&f.cmdType, "type", "t", "", "force "+noun+" type ("+types+")")
	fl.BoolVar(&f.explain, "explain", false, "explain why this "+noun+" was generated")
	fl.BoolVar(&f.short, "short", false, "generate a concise "+noun+" (default)")
	fl.BoolVar(&f.detailed, "detailed", false, "generate a detailed "+noun)
	fl.BoolVar(&f.noCache, "no-cache", false, "bypass the response cache and always call the LLM")
	cmd.MarkFlagsMutuallyExclusive("short", "detailed")
	_ = cmd.RegisterFlagCompletionFunc("type",
		cobra.FixedCompletions(cmdTypeNames(), cobra.ShellCompDirectiveNoFileComp))
}

func cmdTypeNames() []string {
	names := make([]string, 0, len(domain.CmdTypeIndex))
	for name := range domain.CmdTypeIndex {
		if name != "" {
			names = append(names, name)
		}
	}
	slices.Sort(names)
	return names
}

func parseCmdType(raw string) (domain.CmdType, error) {
	cmdType, ok := domain.CmdTypeIndex[raw]
	if !ok {
		return domain.Undefined, domain.ErrInvalidCmdType
	}
	return cmdType, nil
}

func parseCmdMode(detailed bool) domain.CmdMode {
	if detailed {
		return domain.Detailed
	}
	return domain.Short
}
