package cmd

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"

	"github.com/codyconfer/viewkit/layout"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/render"
	"github.com/codyconfer/mino/internal/signals"
	"github.com/codyconfer/mino/internal/signals/build"
)

const defaultDetailSignal = "github"

func newShowCmd() *cobra.Command {
	var signalName string
	c := &cobra.Command{
		Use:   "show <url>",
		Short: "Show details for an issue, pull request or merge request",
		Long: "Fetch and print one item's details: body, labels, reviewers, CI checks, and recent " +
			"comments. The signal is inferred from the URL's host unless --signal says otherwise.",
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

// detailSignalFor picks the signal from the URL's host. Configured endpoints are matched
// first, which is what makes a self-managed host route correctly; without this a GitLab
// URL reaches github's ParseRef and dies telling the user to pass a GitHub URL.
func detailSignalFor(rawURL string, cfg *config.Config) string {
	host := detailHost(rawURL)
	if host == "" {
		return defaultDetailSignal
	}
	if cfg != nil {
		if h := detailHost(cfg.GitLab.APIURL); h != "" && h == host {
			return enabledDetailSignal("gitlab")
		}
		if h := detailHost(cfg.GitHub.APIURL); h != "" && h == host {
			return enabledDetailSignal("github")
		}
	}
	switch {
	case host == "gitlab.com" || strings.HasPrefix(host, "gitlab."):
		return enabledDetailSignal("gitlab")
	case host == "github.com" || host == "api.github.com" || strings.HasPrefix(host, "github."):
		return enabledDetailSignal("github")
	}
	return defaultDetailSignal
}

func detailHost(rawURL string) string {
	u, err := url.Parse(strings.TrimSpace(rawURL))
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Hostname())
}

func enabledDetailSignal(name string) string {
	for _, s := range build.DetailSignals() {
		if s == name {
			return name
		}
	}
	return defaultDetailSignal
}

func runShow(cmd *cobra.Command, url, signalName string) error {
	url = strings.TrimSpace(url)
	if url == "" {
		return errs.New(errs.KindUsage, "show requires an item URL").
			WithHint("try `mino show https://github.com/owner/repo/pull/123` or " +
				"`mino show https://gitlab.com/group/project/-/merge_requests/42`")
	}
	if signalName == "" {
		signalName = detailSignalFor(url, shared.Cfg)
	}
	it := signals.Item{URL: url}
	d, err := build.Detail(cmd.Context(), signalName, it, shared.Role(), shared.Cfg, shared.Tokens, shared.Cache)
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
	_, err = fmt.Fprintln(w, render.DetailPanel(layout.FrameFor(w).WithUI(Scope()), ref, &d))
	return err
}
