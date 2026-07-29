package cmd

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/render/glyph"
	"github.com/codyconfer/munin/internal/signals/build"
)

var listKinds = []string{"queries", "filters", "flights", "formatters", "roles"}

func newListCmd() *cobra.Command {
	var all bool
	c := &cobra.Command{
		Use:   "list [queries|filters|flights|formatters|roles]",
		Short: "List saved queries, filters, flights, formatters, and roles",
		Long: "List what the active role can see. Queries and filters share the\n" +
			"queries/ collection: a document with a signal is a query, a document with\n" +
			"rules/aliases/keywords is a filter, and a document with both shows up in\n" +
			"each list.\n\n" +
			"With no argument, every kind is listed. Pass --all to ignore the active\n" +
			"role and list everything; with no role set that is already the behaviour.",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeListKinds,
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = build.KnownSignals()
			out := cmd.OutOrStdout()
			if len(args) == 0 {
				listQueriesSection(out, all)
				fmt.Fprintln(out)
				listFiltersSection(out, all)
				fmt.Fprintln(out)
				listFlightsSection(out, all)
				fmt.Fprintln(out)
				listFormattersSection(out, all)
				fmt.Fprintln(out)
				listRolesSection(out)
				return nil
			}
			switch args[0] {
			case "queries":
				listQueriesSection(out, all)
			case "filters":
				listFiltersSection(out, all)
			case "flights":
				listFlightsSection(out, all)
			case "formatters":
				listFormattersSection(out, all)
			case "roles":
				listRolesSection(out)
			default:
				return errs.Newf(errs.KindUsage, "unknown kind %q: want one of %v", args[0], listKinds)
			}
			return nil
		},
	}
	c.Flags().BoolVarP(&all, "all", "a", false, "ignore the active role and list everything")
	return c
}

func scopedNames(all bool, names []string, visible func(string) bool) []string {
	if all {
		return names
	}
	out := make([]string, 0, len(names))
	for _, n := range names {
		if visible(n) {
			out = append(out, n)
		}
	}
	return out
}

func listHeading(w io.Writer, title string) {
	fmt.Fprintln(w, theme.Cur().Accent.Render(title))
}

func listLine(w io.Writer, name, detail string) {
	line := fmt.Sprintf("  %s %-24s", theme.Cur().Accent.Render(glyph.Bullet()), name)
	if detail != "" {
		line += " " + detail
	}
	fmt.Fprintln(w, line)
}

func listEmpty(w io.Writer, msg string) {
	fmt.Fprintln(w, "  "+theme.Cur().Dim.Render(msg))
}

func listScopeNote(all bool) string {
	r := shared.Cfg.Role
	switch {
	case r == "":
		return ""
	case all:
		return theme.Cur().Dim.Render(" (all, ignoring role " + r + ")")
	}
	return theme.Cur().Dim.Render(" (role: " + r + ")")
}

func listQueriesSection(w io.Writer, all bool) {
	names := scopedNames(all, shared.Directives.RunnableNames(), access().QueryVisible)
	listHeading(w, "queries"+listScopeNote(all))
	if len(names) == 0 {
		listEmpty(w, "none (add a YAML file with a `signal:` under ~/.munin/queries)")
		return
	}
	for _, n := range names {
		q := shared.Directives.Queries[n]
		detail := "signal=" + q.Signal
		if q.HasRules() {
			detail += " +rules"
		}
		if q.Title != "" {
			detail += "  " + theme.Cur().Dim.Render(q.Title)
		}
		listLine(w, n, detail)
	}
}

func listFiltersSection(w io.Writer, all bool) {
	names := scopedNames(all, shared.Directives.FilterNames(), access().QueryVisible)
	listHeading(w, "filters"+listScopeNote(all))
	seen := map[string]bool{}
	for _, n := range names {
		f, _ := shared.Directives.Filter(n)
		detail := fmt.Sprintf("%d rule(s) %d alias(es)", len(f.Rules), len(f.Aliases))
		if shared.Directives.Queries[n].Runnable() {
			detail += " " + theme.Cur().Dim.Render("(also a query)")
		}
		listLine(w, n, detail)
		seen[n] = true
	}
	var plugins int
	for _, n := range plugin.FilterNames() {
		if seen[n] {
			continue
		}
		kind := "rules"
		if plugin.HasFilterEngine(n) {
			kind = "engine"
		}
		f, _ := plugin.LookupFilter(n)
		detail := fmt.Sprintf("%d rule(s) %d alias(es) %s", len(f.Rules), len(f.Aliases),
			theme.Cur().Dim.Render("(plugin "+kind+")"))
		listLine(w, n, detail)
		plugins++
	}
	if len(names) == 0 && plugins == 0 {
		listEmpty(w, "none (add `rules:` to a YAML file under ~/.munin/queries)")
	}
}

func listFlightsSection(w io.Writer, all bool) {
	names := scopedNames(all, shared.Directives.FlightNames(), access().FlightVisible)
	listHeading(w, "flights"+listScopeNote(all))
	if len(names) == 0 {
		listEmpty(w, "none (add a YAML file under ~/.munin/flights)")
		return
	}
	for _, n := range names {
		fl := shared.Directives.Flights[n]
		listLine(w, n, fmt.Sprintf("%d quer%s", len(fl.Queries), plural(len(fl.Queries), "y", "ies")))
	}
}

func listFormattersSection(w io.Writer, all bool) {
	names := scopedNames(all, shared.Directives.FormatterNames(), access().FormatterVisible)
	listHeading(w, "formatters"+listScopeNote(all))
	if len(names) == 0 {
		listEmpty(w, "none (add a YAML file with a `template:` under ~/.munin/formatters)")
		return
	}
	for _, n := range names {
		fd := shared.Directives.Formatters[n]
		lines := len(strings.Split(strings.TrimRight(fd.Template, "\n"), "\n"))
		detail := fmt.Sprintf("%d line(s)", lines)
		if fd.Title != "" {
			detail += "  " + theme.Cur().Dim.Render(fd.Title)
		}
		listLine(w, n, detail)
	}
}

func listRolesSection(w io.Writer) {
	names := shared.Directives.RoleNames()
	listHeading(w, "roles")
	if len(names) == 0 {
		listEmpty(w, "none (add a <name>.yaml at the top of ~/.munin)")
		return
	}
	for _, n := range names {
		rd := shared.Directives.Roles[n]
		detail := fmt.Sprintf("%d flight(s) %d quer%s", len(rd.Flights), len(rd.Queries),
			plural(len(rd.Queries), "y", "ies"))
		if len(rd.Formatters) > 0 {
			detail += fmt.Sprintf(" %d formatter(s)", len(rd.Formatters))
		}
		if n == shared.Cfg.Role {
			detail += " " + theme.Cur().Can.Render("(active)")
		}
		listLine(w, n, detail)
	}
}

func plural(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}
