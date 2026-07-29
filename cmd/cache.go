package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/munin/internal/render/glyph"
	"github.com/codyconfer/munin/internal/signals/cache"
)

func newCacheCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "cache",
		Short: "Inspect or drop cached signal results",
		Long: "Signal results are cached in .data/cache.duckdb for `cache.ttl` (default 60s),\n" +
			"so re-running a flight does not re-hit the GitHub, Google, and Slack APIs.\n" +
			"Set `cache.ttl: \"0\"` to disable, or override one signal under `cache.signals`.\n" +
			"Use --refresh on any command to fetch live and re-warm the cache.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cacheStats(cmd)
		},
	}
	c.AddCommand(
		&cobra.Command{
			Use:   "stats",
			Short: "Show what is currently cached",
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				return cacheStats(cmd)
			},
		},
		&cobra.Command{
			Use:               "clear [signal]",
			Short:             "Drop cached results for one signal, or all of them",
			Args:              cobra.MaximumNArgs(1),
			ValidArgsFunction: completeCacheSignals,
			RunE: func(cmd *cobra.Command, args []string) error {
				return cacheClear(cmd, args)
			},
		},
	)
	return c
}

func cacheStats(cmd *cobra.Command) error {
	stats, err := shared.Cache.Stats(cmd.Context())
	if err != nil {
		return err
	}
	w := cmd.OutOrStdout()
	if len(stats) == 0 {
		fmt.Fprintln(w, "cache is empty")
		return nil
	}
	th := theme.Cur()
	marker := th.Accent.Render(glyph.Bullet())
	now := time.Now()
	for _, s := range stats {
		detail := fmt.Sprintf("%d %s, %d fresh", s.Entries, plural(s.Entries, "entry", "entries"), s.Fresh)
		if !s.Newest.IsZero() {
			detail += fmt.Sprintf(", newest %s ago", now.Sub(s.Newest).Round(time.Second))
		}
		fmt.Fprintf(w, "%s %-24s %s\n", marker, s.Label, th.Dim.Render(detail))
	}
	return nil
}

func cacheClear(cmd *cobra.Command, args []string) error {
	namespace, label := "", "cache"
	if len(args) == 1 {
		namespace, label = cache.Namespace(args[0]), args[0]
	}
	n, err := shared.Cache.Clear(cmd.Context(), namespace)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "cleared %d %s from %s\n", n, plural(int(n), "entry", "entries"), label)
	return nil
}
