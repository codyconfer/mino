package verify

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/theme"
	"gopkg.in/yaml.v3"

	"github.com/codyconfer/sisyphus/redact"

	"github.com/codyconfer/munin/internal/auth"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/filter"
	"github.com/codyconfer/munin/internal/onboard"
	gh "github.com/codyconfer/munin/internal/signals/github"
)

type Finding struct {
	Name    string
	Msg     string
	Snippet string
	OK      bool
	Warn    bool
}

var knownSignals = map[string]bool{
	"github": true, "calendar": true, "gmail": true, "docs": true,
	"drive": true, "tasks": true, "slack": true, "demo": true,
}

func Run(ctx context.Context, w io.Writer, cfg *config.Config, directives *config.Directives, tokens auth.TokenStore, target string) error {
	v := newStyles(w)

	sections := []struct {
		key   string
		title string
		run   func() []Finding
	}{
		{"config", "Config", func() []Finding { return Config(cfg, directives) }},
		{"roles", "Roles", func() []Finding { return Roles(directives) }},
		{"flights", "Flights", func() []Finding { return Flights(directives) }},
		{"queries", "Queries", func() []Finding { return Queries(directives) }},
		{"onboarding", "Onboarding", func() []Finding { return Onboarding(ctx, tokens, cfg.GitHub.APIURL) }},
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

func Config(cfg *config.Config, directives *config.Directives) []Finding {
	var out []Finding

	check := func(name string, ok bool, msg, snippet string) {
		out = append(out, Finding{Name: name, OK: ok, Msg: msg, Snippet: snippet})
	}

	check("output", cfg.Output == "terminal" || cfg.Output == "json",
		fmt.Sprintf("output=%q (want terminal or json)", cfg.Output), "output: "+cfg.Output)

	if cfg.Timeout != "" {
		_, err := time.ParseDuration(cfg.Timeout)
		check("timeout", err == nil, fmt.Sprintf("timeout=%q is not a valid duration", cfg.Timeout), "timeout: "+cfg.Timeout)
	}

	check("backup.keep", cfg.Backup.Keep >= 0,
		fmt.Sprintf("backup.keep=%d must be >= 0", cfg.Backup.Keep), toYAML(cfg.Backup))

	if cfg.GitHub.APIURL != "" {
		_, err := gh.NormalizeAPIURL(cfg.GitHub.APIURL)
		check("github.api_url", err == nil, fmt.Sprintf("api_url=%q must be an https URL", cfg.GitHub.APIURL), "github:\n  api_url: "+cfg.GitHub.APIURL)
	}

	switch cfg.Backup.SecretBackend {
	case "", "auto", "bitwarden", "1password", "keyring":
		check("backup.secret_backend", true, "", "")
	default:
		check("backup.secret_backend", false,
			fmt.Sprintf("unknown backend %q", cfg.Backup.SecretBackend), toYAML(cfg.Backup))
	}
	switch cfg.Backup.Destination {
	case "", "local", "gdrive":
		check("backup.destination", true, "", "")
	default:
		check("backup.destination", false,
			fmt.Sprintf("unknown destination %q", cfg.Backup.Destination), toYAML(cfg.Backup))
	}

	if cfg.Role != "" {
		_, ok := directives.Roles[cfg.Role]
		check("active role", ok, fmt.Sprintf("role %q is not defined (in roles/)", cfg.Role), "role: "+cfg.Role)
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

func Roles(directives *config.Directives) []Finding {
	var out []Finding
	for _, name := range directives.RoleNames() {
		rd := directives.Roles[name]
		var missing []string
		for _, f := range rd.Flights {
			if _, ok := directives.Flights[f]; !ok {
				missing = append(missing, "flight "+f)
			}
		}
		for _, q := range rd.Queries {
			if _, ok := directives.Queries[q]; !ok {
				missing = append(missing, "query "+q)
			}
		}
		for _, fl := range rd.Filters {
			if _, ok := directives.Filters[fl]; !ok {
				missing = append(missing, "filter "+fl)
			}
		}
		if len(missing) > 0 {
			out = append(out, Finding{Name: name, OK: false,
				Msg:     "references undefined: " + strings.Join(missing, ", "),
				Snippet: toYAML(rd)})
			continue
		}
		out = append(out, Finding{Name: name, OK: true})
	}
	return out
}

func Flights(directives *config.Directives) []Finding {
	var out []Finding
	for _, name := range directives.FlightNames() {
		fl := directives.Flights[name]
		f := Finding{Name: name, OK: true}
		snippet := func() string { return toYAML(fl) }

		if len(fl.Queries) == 0 {
			f.OK, f.Msg, f.Snippet = false, "flight has no queries", snippet()
		} else {
			var missing []string
			for _, q := range fl.Queries {
				if _, ok := directives.Queries[q]; !ok {
					missing = append(missing, q)
				}
			}
			if len(missing) > 0 {
				f.OK, f.Msg, f.Snippet = false,
					"unknown queries: "+strings.Join(missing, ", "), snippet()
			}
		}
		out = append(out, f)
	}
	return out
}

func Queries(directives *config.Directives) []Finding {
	var out []Finding
	for _, name := range directives.QueryNames() {
		q := directives.Queries[name]
		f := Finding{Name: name, OK: true}
		snippet := func() string { return toYAML(q) }

		switch {
		case !knownSignals[q.Signal]:
			f.OK, f.Msg, f.Snippet = false, fmt.Sprintf("unknown signal %q", q.Signal), snippet()
		default:
			var missing []string
			for _, qf := range q.Filters {
				if qf.Ref != "" {
					if _, ok := directives.Filters[qf.Ref]; !ok {
						missing = append(missing, qf.Ref)
					}
				}
			}
			if len(missing) > 0 {
				f.OK, f.Msg, f.Snippet = false, "unknown filters: "+strings.Join(missing, ", "), snippet()
			} else if resolved, err := directives.Resolve(q); err != nil {
				f.OK, f.Msg, f.Snippet = false, err.Error(), snippet()
			} else if _, err := filter.CompileAll(resolved); err != nil {
				f.OK, f.Msg, f.Snippet = false, err.Error(), snippet()
			}
		}
		out = append(out, f)
	}
	return out
}

func Onboarding(ctx context.Context, tokens auth.TokenStore, apiURLRaw string) []Finding {
	apiURL, err := gh.NormalizeAPIURL(apiURLRaw)
	if err != nil {
		return []Finding{{Name: "github.api_url", OK: false, Msg: err.Error()}}
	}
	st := onboard.Check(ctx, tokens, apiURL)
	var out []Finding
	for _, r := range st.Results {
		f := Finding{Name: r.Title, OK: r.OK, Warn: !r.OK, Msg: r.Detail}
		if !r.OK && len(r.Fix) > 0 {
			f.Snippet = strings.Join(r.Fix, "\n")
		}
		out = append(out, f)
	}
	return out
}

type styles struct {
	title, ok, err, warn, name, dim, snippet lipgloss.Style
}

func newStyles(w io.Writer) styles {
	r := lipgloss.NewRenderer(w)
	return styles{
		title:   r.NewStyle().Bold(true).Underline(true),
		ok:      r.NewStyle().Foreground(lipgloss.Color("10")),
		err:     r.NewStyle().Foreground(lipgloss.Color("9")).Bold(true),
		warn:    r.NewStyle().Foreground(lipgloss.Color("11")),
		name:    r.NewStyle().Bold(true),
		dim:     r.NewStyle().Faint(true),
		snippet: r.NewStyle().Faint(true),
	}
}

func (v styles) print(w io.Writer, f Finding) int {
	switch {
	case f.OK:
		fmt.Fprintf(w, "  %s %s\n", v.ok.Render("✓"), v.name.Render(f.Name))
		return 0
	case f.Warn:
		fmt.Fprintf(w, "  %s %s  %s\n", v.warn.Render("⚠"), v.name.Render(f.Name), v.warn.Render(f.Msg))
	default:
		fmt.Fprintf(w, "  %s %s  %s\n", v.err.Render("✗"), v.name.Render(f.Name), v.err.Render(f.Msg))
	}
	if f.Snippet != "" {
		for _, line := range strings.Split(strings.TrimRight(redact.Line(f.Snippet), "\n"), "\n") {
			fmt.Fprintln(w, "      "+v.snippet.Render(line))
		}
	}
	if f.Warn {
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
