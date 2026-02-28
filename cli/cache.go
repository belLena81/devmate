package cli

import (
	"devmate/internal/domain"
	"devmate/internal/service"
	"fmt"

	"github.com/spf13/cobra"
)

// CacheService is the interface the cache subcommands require.
// It is satisfied by *service.diskCache (via a thin adapter) and by
// fakeCacheService in tests.
type CacheService interface {
	// Clean removes all cached entries.
	Clean() error
	// Stat returns metadata for every entry currently in the cache,
	// sorted by modification time, newest first.
	Stat() ([]service.CacheEntry, error)
}

func newCacheCmd(a *App) *cobra.Command {
	cacheCmd := &cobra.Command{
		Use:   "cache",
		Short: "Manage the local LLM response cache",
		Long: `Inspect and manage the on-disk cache of LLM responses.

devmate stores responses from the LLM in a local cache directory
(default: ~/.cache/devmate/) so that repeated requests with identical
inputs are served instantly without contacting the LLM.

Subcommands:
  clean   Remove all cached entries
  stat    List all cached entries with size and age
`,
	}
	cacheCmd.AddCommand(newCacheCleanCmd(a))
	cacheCmd.AddCommand(newCacheStatCmd(a))
	return cacheCmd
}

func newCacheCleanCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "clean",
		Short: "Remove all cached LLM responses",
		Long: `Deletes every entry from the local LLM response cache.

After running this command the next request for any command (commit, branch, pr)
will contact the LLM even if the same input was seen before.

This is useful after upgrading the model, changing templates, or when you
suspect a stale response is being served.
`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if a.cacheService == nil {
				return domain.ErrServiceNotInitialized
			}
			if err := a.cacheService.Clean(); err != nil {
				return err
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Cache cleared.")
			return nil
		},
	}
}

func newCacheStatCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "stat",
		Short: "List all cached LLM responses",
		Long: `Prints a summary of every entry currently in the local LLM response cache.

Each line shows the cache key (a SHA-256 hex digest), the size of the stored
response in bytes, and when the entry was last written.

Output is sorted newest-first so the most recently used entries appear at
the top.
`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if a.cacheService == nil {
				return domain.ErrServiceNotInitialized
			}
			entries, err := a.cacheService.Stat()
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			if len(entries) == 0 {
				fmt.Fprintln(out, "No cached entries.")
				return nil
			}
			fmt.Fprintf(out, "%-64s  %10s  %s\n", "KEY", "SIZE (B)", "MODIFIED")
			for _, e := range entries {
				fmt.Fprintf(out, "%-64s  %10d  %s\n",
					e.Key,
					e.SizeBytes,
					e.ModTime.Format("2006-01-02 15:04:05"),
				)
			}
			return nil
		},
	}
}
