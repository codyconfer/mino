package cmd

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/codyconfer/viewkit/layout"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/render"
	"github.com/codyconfer/munin/internal/signals"
	"github.com/codyconfer/munin/internal/signals/build"
)

const defaultDetailSignal = "github"

func newShowCmd() *cobra.Command {
	var signalName string
	c := &cobra.Command{
		Use:               "show <url>",
		Short:             "Show details for an issue or pull request",
		Long:              "Fetch and print one item's details: body, labels, reviewers, CI checks, and recent comments.",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: cobra.NoFileCompletions,
		RunE: func(cmd *cobra.Command, args []string) error {
			return runShow(cmd, args[0], signalName)
		},
	}
	c.Flags().StringVar(&signalName, "signal", "", "signal that owns the item (default "+defaultDetailSignal+")")
	bindFlagCompletion(c, "signal", completeDetailSignals)
	return c
}

func runShow(cmd *cobra.Command, url, signalName string) error {
	url = strings.TrimSpace(url)
	if url == "" {
		return errs.New(errs.KindUsage, "show requires an item URL").
			WithHint("try `munin show https://github.com/owner/repo/pull/123`")
	}
	if signalName == "" {
		signalName = defaultDetailSignal
	}
	it := signals.Item{URL: url}
	d, err := build.Detail(cmd.Context(), signalName, it, shared.Cfg, shared.Tokens, shared.Cache)
	if err != nil {
		return err
	}
	w := cmd.OutOrStdout()
	if render.Format(shared.Cfg.Output) == render.FormatJSON {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.SetEscapeHTML(false)
		return enc.Encode(d)
	}
	it.Kind = d.Kind
	it.Title = d.Title
	ref := render.ItemRef{Signal: signalName, Item: it}
	_, err = fmt.Fprintln(w, render.DetailPanel(layout.FrameFor(w), ref, &d))
	return err
}
