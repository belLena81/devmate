package cli

import (
	"devmate/internal/domain"
	"devmate/internal/service"
	"fmt"
	"text/tabwriter"

	"github.com/spf13/cobra"
)

// CacheService is the interface the cache subcommands require.
type CacheService interface {
	Clean() error
	// Stat returns every cache entry, newest first.
	Stat() ([]service.CacheEntry, error)
}

func newCacheCmd(a *App) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "cache",
		Short: "Manage the local LLM response cache",
		Long: `Inspect and manage the on-disk cache of LLM responses.

Responses are cached per input so repeated requests with identical inputs are
served instantly without contacting the LLM.`,
	}
	cmd.AddCommand(newCacheCleanCmd(a))
	cmd.AddCommand(newCacheStatCmd(a))
	return cmd
}

func newCacheCleanCmd(a *App) *cobra.Command {
	return &cobra.Command{
		Use:   "clean",
		Short: "Remove all cached LLM responses",
		Long: `Deletes every entry from the local LLM response cache.

Useful after upgrading the model, changing templates, or when you suspect a
stale response is being served.`,
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
		Long: `Prints every entry in the local LLM response cache with its key (a SHA-256
hex digest), size in bytes, and last-written time, newest first.`,
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
			w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
			fmt.Fprintln(w, "KEY\tSIZE (B)\tMODIFIED")
			for _, e := range entries {
				fmt.Fprintf(w, "%s\t%d\t%s\n", e.Key, e.SizeBytes, e.ModTime.Format("2006-01-02 15:04:05"))
			}
			return w.Flush()
		},
	}
}
