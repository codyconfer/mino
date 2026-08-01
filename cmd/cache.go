package cmd

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/mino/internal/app/suggest"
	"github.com/codyconfer/mino/internal/render/glyph"
	"github.com/codyconfer/mino/internal/signals/cache"
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
			Use:   "clear [target]",
			Short: "Drop cached results for one signal, or all of them",
			Long: "With no argument every namespace is dropped. With a signal name that signal's\n" +
				"results go along with the side tables it owns (github also keeps `github:team`\n" +
				"rosters and `github:detail` entries). Naming one of those namespaces directly\n" +
				"clears just it; `mino cache stats` lists everything that is clearable.",
			Args:              cobra.MaximumNArgs(1),
			ValidArgsFunction: completeCacheTargets,
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
	n, label, err := clearTarget(cmd, args)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "cleared %d %s from %s\n", n, plural(int(n), "entry", "entries"), label)
	return nil
}

func clearTarget(cmd *cobra.Command, args []string) (int64, string, error) {
	if len(args) == 0 {
		n, err := shared.Cache.Clear(cmd.Context(), "")
		return n, "cache", err
	}
	target := args[0]
	if strings.Contains(target, ":") {
		n, err := shared.Cache.Clear(cmd.Context(), target)
		return n, target, err
	}
	n, err := shared.Cache.ClearSignal(cmd.Context(), target)
	return n, target, err
}

func completeCacheTargets(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completeNames(func() []string {
		names := suggest.Signals()
		ctx := context.Background()
		if cmd != nil && cmd.Context() != nil {
			ctx = cmd.Context()
		}
		stats, err := shared.Cache.Stats(ctx)
		if err != nil {
			return names
		}
		for _, s := range stats {
			if _, isSignal := cache.SignalOf(s.Namespace); isSignal {
				continue
			}
			if !slices.Contains(names, s.Namespace) {
				names = append(names, s.Namespace)
			}
		}
		return names
	})(cmd, args, toComplete)
}
