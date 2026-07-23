package cmd

import (
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/theme"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/filter"
	"github.com/codyconfer/sisyphus/redact"
)

var knownSignals = map[string]bool{
	"github": true, "calendar": true, "gmail": true, "docs": true,
	"drive": true, "tasks": true, "slack": true, "demo": true,
}

func newVerifyCmd() *cobra.Command {
	return &cobra.Command{
		Use:       "verify [config|roles|flights|queries|all]",
		Short:     "Validate config, roles, flights, and queries with detailed diagnostics",
		Long:      "Checks referential integrity (flights → queries, roles → flights, queries →\nfilters/signals) and enum values. On a problem it prints the offending config\nsnippet with secrets masked.",
		ValidArgs: []string{"config", "roles", "flights", "queries", "all"},
		Args:      cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "all"
			if len(args) == 1 {
				target = args[0]
			}
			return runVerify(cmd, target)
		},
	}
}

type finding struct {
	name    string
	ok      bool
	warn    bool
	msg     string
	snippet string
}

func runVerify(cmd *cobra.Command, target string) error {
	v := newVerifyStyles(cmd.OutOrStdout())
	w := cmd.OutOrStdout()

	sections := []struct {
		key   string
		title string
		run   func() []finding
	}{
		{"config", "Config", verifyConfig},
		{"roles", "Roles", verifyRoles},
		{"flights", "Flights", verifyFlights},
		{"queries", "Queries", verifyQueries},
	}

	problems := 0
	for _, s := range sections {
		if target != "all" && target != s.key {
			continue
		}
		findings := s.run()
		fmt.Fprintln(w, v.title.Render(s.title))
		if len(findings) == 0 {
			fmt.Fprintln(w, "  "+v.dim.Render("(none)"))
		}
		for _, f := range findings {
			problems += v.print(w, f)
		}
		fmt.Fprintln(w)
	}

	if problems > 0 {
		return errs.Newf(errs.KindConfig, "%d problem(s) found", problems)
	}
	return nil
}

func verifyConfig() []finding {
	c := shared.cfg
	var out []finding

	check := func(name string, ok bool, msg, snippet string) {
		out = append(out, finding{name: name, ok: ok, msg: msg, snippet: snippet})
	}

	check("output", c.Output == "terminal" || c.Output == "json",
		fmt.Sprintf("output=%q (want terminal or json)", c.Output), "output: "+c.Output)

	if c.Timeout != "" {
		_, err := time.ParseDuration(c.Timeout)
		check("timeout", err == nil, fmt.Sprintf("timeout=%q is not a valid duration", c.Timeout), "timeout: "+c.Timeout)
	}

	check("backup.keep", c.Backup.Keep >= 0,
		fmt.Sprintf("backup.keep=%d must be >= 0", c.Backup.Keep), toYAML(c.Backup))

	if c.GitHub.APIURL != "" {
		ok := strings.HasPrefix(c.GitHub.APIURL, "http://") || strings.HasPrefix(c.GitHub.APIURL, "https://")
		check("github.api_url", ok, fmt.Sprintf("api_url=%q should be an http(s) URL", c.GitHub.APIURL), "github:\n  api_url: "+c.GitHub.APIURL)
	}

	switch c.Backup.SecretBackend {
	case "", "auto", "bitwarden", "1password", "keyring":
		check("backup.secret_backend", true, "", "")
	default:
		check("backup.secret_backend", false,
			fmt.Sprintf("unknown backend %q", c.Backup.SecretBackend), toYAML(c.Backup))
	}
	switch c.Backup.Destination {
	case "", "local", "gdrive":
		check("backup.destination", true, "", "")
	default:
		check("backup.destination", false,
			fmt.Sprintf("unknown destination %q", c.Backup.Destination), toYAML(c.Backup))
	}

	if c.Role != "" {
		_, ok := shared.directives.Roles[c.Role]
		check("active role", ok, fmt.Sprintf("role %q is not defined (in roles/)", c.Role), "role: "+c.Role)
	}

	if gs := config.LoadGlobalSettings(); gs.Theme != "" {
		_, ok := theme.Named(gs.Theme)
		check("theme (global)", ok,
			fmt.Sprintf("unknown theme %q (have: %s)", gs.Theme, strings.Join(theme.Keys(), ", ")),
			"theme: "+gs.Theme)
	}

	if gs := config.LoadGlobalSettings(); gs.Keys != "" {
		_, ok := keys.Named(gs.Keys)
		check("keys (global)", ok,
			fmt.Sprintf("unknown key scheme %q (have: %s)", gs.Keys, strings.Join(keys.Keys(), ", ")),
			"keys: "+gs.Keys)
	}
	return out
}

func verifyRoles() []finding {
	var out []finding
	for _, name := range shared.directives.RoleNames() {
		rd := shared.directives.Roles[name]
		var missing []string
		for _, f := range rd.Flights {
			if _, ok := shared.directives.Flights[f]; !ok {
				missing = append(missing, "flight "+f)
			}
		}
		for _, q := range rd.Queries {
			if _, ok := shared.directives.Queries[q]; !ok {
				missing = append(missing, "query "+q)
			}
		}
		for _, fl := range rd.Filters {
			if _, ok := shared.directives.Filters[fl]; !ok {
				missing = append(missing, "filter "+fl)
			}
		}
		if len(missing) > 0 {
			out = append(out, finding{name: name, ok: false,
				msg:     "references undefined: " + strings.Join(missing, ", "),
				snippet: toYAML(rd)})
			continue
		}
		out = append(out, finding{name: name, ok: true})
	}
	return out
}

func verifyFlights() []finding {
	var out []finding
	for _, name := range shared.directives.FlightNames() {
		fl := shared.directives.Flights[name]
		f := finding{name: name, ok: true}
		snippet := func() string { return toYAML(fl) }

		if len(fl.Queries) == 0 {
			f.ok, f.msg, f.snippet = false, "flight has no queries", snippet()
		} else {
			var missing []string
			for _, q := range fl.Queries {
				if _, ok := shared.directives.Queries[q]; !ok {
					missing = append(missing, q)
				}
			}
			if len(missing) > 0 {
				f.ok, f.msg, f.snippet = false,
					"unknown queries: "+strings.Join(missing, ", "), snippet()
			}
		}
		out = append(out, f)
	}
	return out
}

func verifyQueries() []finding {
	var out []finding
	for _, name := range shared.directives.QueryNames() {
		q := shared.directives.Queries[name]
		f := finding{name: name, ok: true}
		snippet := func() string { return toYAML(q) }

		switch {
		case !knownSignals[q.Signal]:
			f.ok, f.msg, f.snippet = false, fmt.Sprintf("unknown signal %q", q.Signal), snippet()
		default:
			var missing []string
			for _, qf := range q.Filters {
				if qf.Ref != "" {
					if _, ok := shared.directives.Filters[qf.Ref]; !ok {
						missing = append(missing, qf.Ref)
					}
				}
			}
			if len(missing) > 0 {
				f.ok, f.msg, f.snippet = false, "unknown filters: "+strings.Join(missing, ", "), snippet()
			} else if resolved, err := shared.directives.Resolve(q); err != nil {
				f.ok, f.msg, f.snippet = false, err.Error(), snippet()
			} else if _, err := filter.CompileAll(resolved); err != nil {
				f.ok, f.msg, f.snippet = false, err.Error(), snippet()
			}
		}
		out = append(out, f)
	}
	return out
}

type verifyStyles struct {
	title, ok, err, warn, name, dim, snippet lipgloss.Style
}

func newVerifyStyles(w io.Writer) verifyStyles {
	r := lipgloss.NewRenderer(w)
	return verifyStyles{
		title:   r.NewStyle().Bold(true).Underline(true),
		ok:      r.NewStyle().Foreground(lipgloss.Color("10")),
		err:     r.NewStyle().Foreground(lipgloss.Color("9")).Bold(true),
		warn:    r.NewStyle().Foreground(lipgloss.Color("11")),
		name:    r.NewStyle().Bold(true),
		dim:     r.NewStyle().Faint(true),
		snippet: r.NewStyle().Faint(true),
	}
}

func (v verifyStyles) print(w io.Writer, f finding) int {
	switch {
	case f.ok:
		fmt.Fprintf(w, "  %s %s\n", v.ok.Render("✓"), v.name.Render(f.name))
		return 0
	case f.warn:
		fmt.Fprintf(w, "  %s %s  %s\n", v.warn.Render("⚠"), v.name.Render(f.name), v.warn.Render(f.msg))
	default:
		fmt.Fprintf(w, "  %s %s  %s\n", v.err.Render("✗"), v.name.Render(f.name), v.err.Render(f.msg))
	}
	if f.snippet != "" {
		for _, line := range strings.Split(strings.TrimRight(redact.Line(f.snippet), "\n"), "\n") {
			fmt.Fprintln(w, "      "+v.snippet.Render(line))
		}
	}
	if f.warn {
		return 0
	}
	return 1
}

func toYAML(v any) string {
	b, err := yaml.Marshal(v)
	if err != nil {
		return fmt.Sprintf("%v", v)
	}
	return string(b)
}
