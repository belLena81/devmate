package cli

import (
	"devmate/internal/config"
	"runtime/debug"

	"github.com/spf13/cobra"
)

// Version is set at build time with -ldflags "-X devmate/cli.Version=v1.2.3"; when empty, module build info is used.
var Version string

func buildRootCmd(app *App) *cobra.Command {
	root := &cobra.Command{
		Use:   "devmate",
		Short: "Devmate is a read-only developer assistant",
		Long: `devmate is a CLI tool that streamlines common development workflows.

It drafts Conventional Commit messages, branch names, and pull request
descriptions from your repository using a locally running LLM. It only
reads repository state and never runs git mutations.`,
		Version:           version(),
		SilenceUsage:      true,
		SilenceErrors:     true,
		PersistentPreRunE: app.ensureServices,
	}

	pf := root.PersistentFlags()
	pf.StringVar(&app.flags.ConfigPath, "config", "", "config file (default "+config.DefaultPath()+")")
	pf.BoolVarP(&app.flags.Verbose, "verbose", "v", false, "enable debug logging")
	pf.BoolVarP(&app.flags.Quiet, "quiet", "q", false, "log errors only")
	root.MarkFlagsMutuallyExclusive("verbose", "quiet")

	root.AddCommand(newCommitCmd(app))
	root.AddCommand(newBranchCmd(app))
	root.AddCommand(newPrCmd(app))
	root.AddCommand(newCacheCmd(app))
	return root
}

func version() string {
	if Version != "" {
		return Version
	}
	if info, ok := debug.ReadBuildInfo(); ok && info.Main.Version != "" {
		return info.Main.Version
	}
	return "(devel)"
}
