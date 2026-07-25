package verify

import (
	"context"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/theme"
	"gopkg.in/yaml.v3"

	"github.com/codyconfer/sisyphus/redact"

	"github.com/codyconfer/munin/internal/app/onboard"
	"github.com/codyconfer/munin/internal/auth"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/filter"
	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/render"
	"github.com/codyconfer/munin/internal/render/glyph"
	"github.com/codyconfer/munin/internal/signals/build"
	gh "github.com/codyconfer/munin/internal/signals/github"
)

type Finding struct {
	Name    string
	Msg     string
	Snippet string
	OK      bool
	Warn    bool
}

func Run(ctx context.Context, w io.Writer, cfg *config.Config, directives *config.Directives, tokens auth.TokenStore, target string) error {
	sty := render.NewReportStyles(w)

	sections := []struct {
		key   string
		title string
		run   func() []Finding
	}{
		{"config", "Config", func() []Finding { return Config(cfg, directives) }},
		{"roles", "Roles", func() []Finding { return Roles(directives) }},
		{"flights", "Flights", func() []Finding { return Flights(directives) }},
		{"queries", "Queries", func() []Finding { return Queries(directives) }},
		{"plugins", "Plugins", func() []Finding { return Plugins() }},
		{"onboarding", "Onboarding", func() []Finding { return Onboarding(ctx, tokens, cfg.GitHub.APIURL) }},
	}

	problems := 0
	for _, s := range sections {
		if target != "all" && target != s.key {
			continue
		}
		findings := s.run()
		fmt.Fprintln(w, sty.Title.Render(s.title))
		if len(findings) == 0 {
			fmt.Fprintln(w, "  "+sty.Dim.Render("(none)"))
		}
		for _, f := range findings {
			problems += printFinding(w, sty, f)
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
		check("active role", ok, fmt.Sprintf("role %q is not defined (no matching *.yaml in the config dir)", cfg.Role), "role: "+cfg.Role)
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
		if rd.Home != "" {
			if _, ok := directives.Flights[rd.Home]; !ok {
				missing = append(missing, "home flight "+rd.Home)
			}
		}
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
			if _, ok := directives.Filters[fl]; ok {
				continue
			}
			if plugin.HasFilter(fl) {
				continue
			}
			missing = append(missing, "filter "+fl)
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

// Plugins verifies plugin↔host registry sync for every Kind.
func Plugins() []Finding {
	var out []Finding
	builders := build.BuilderSignals()
	for _, d := range plugin.All() {
		name := d.ID
		if !plugin.Enabled(d.ID) {
			out = append(out, Finding{Name: name, OK: true, Msg: "disabled"})
			continue
		}
		switch d.Kind {
		case plugin.KindSignal:
			if d.Signal == "" {
				out = append(out, Finding{Name: name, OK: false, Msg: "KindSignal missing Signal"})
				continue
			}
			if !builders[d.Signal] {
				out = append(out, Finding{
					Name: name, OK: false,
					Msg: fmt.Sprintf("enabled plugin signal %q has no host builder", d.Signal),
				})
				continue
			}
			if plugin.HasCapability(d.Signal, plugin.CapAction) && len(plugin.ActionsFor(d.Signal)) == 0 {
				out = append(out, Finding{
					Name: name, OK: false,
					Msg: fmt.Sprintf("signal %q advertises CapAction but has no registered actions", d.Signal),
				})
				continue
			}
			if plugin.HasCapability(d.Signal, plugin.CapStream) && !build.HasActiveBuilder(d.Signal) {
				out = append(out, Finding{
					Name: name, OK: false,
					Msg: fmt.Sprintf("signal %q advertises CapStream but has no active builder", d.Signal),
				})
				continue
			}
			out = append(out, Finding{Name: name, OK: true, Msg: fmt.Sprintf("signal=%s caps=%v", d.Signal, d.Capabilities)})
		case plugin.KindFilter:
			if !plugin.HasFilter(d.Ref) {
				out = append(out, Finding{Name: name, OK: false, Msg: fmt.Sprintf("KindFilter ref %q has no filter contribution", d.Ref)})
				continue
			}
			kind := "rules"
			if plugin.HasFilterEngine(d.Ref) {
				kind = "engine"
			}
			out = append(out, Finding{Name: name, OK: true, Msg: fmt.Sprintf("filter=%s (%s)", d.Ref, kind)})
		case plugin.KindAction:
			sig, act, ok := plugin.SplitActionRef(d.Ref)
			if !ok {
				out = append(out, Finding{Name: name, OK: false, Msg: fmt.Sprintf("KindAction ref %q want signal/name", d.Ref)})
				continue
			}
			if _, ok := plugin.LookupAction(sig, act); !ok {
				out = append(out, Finding{Name: name, OK: false, Msg: fmt.Sprintf("KindAction %s/%s has no RegisterAction binding", sig, act)})
				continue
			}
			out = append(out, Finding{Name: name, OK: true, Msg: "action=" + d.Ref})
		case plugin.KindView:
			if !plugin.HasView(d.Ref) {
				out = append(out, Finding{Name: name, OK: false, Msg: fmt.Sprintf("KindView ref %q missing from deck registry", d.Ref)})
				continue
			}
			out = append(out, Finding{Name: name, OK: true, Msg: "view=" + d.Ref})
		case plugin.KindTheme:
			if !plugin.HasTheme(d.Ref) {
				out = append(out, Finding{Name: name, OK: false, Msg: fmt.Sprintf("KindTheme ref %q missing from theme registry", d.Ref)})
				continue
			}
			out = append(out, Finding{Name: name, OK: true, Msg: "theme=" + d.Ref})
		case plugin.KindContext:
			if !plugin.HasContextProvider(d.Ref) {
				out = append(out, Finding{Name: name, OK: false, Msg: fmt.Sprintf("KindContext ref %q has no ContextProvider", d.Ref)})
				continue
			}
			out = append(out, Finding{Name: name, OK: true, Msg: "context=" + d.Ref})
		default:
			out = append(out, Finding{Name: name, OK: false, Msg: fmt.Sprintf("unknown kind %q", d.Kind)})
		}
	}
	for sig := range builders {
		if !plugin.KnownSignals()[sig] {
			out = append(out, Finding{
				Name: "build:" + sig, OK: false,
				Msg: fmt.Sprintf("host builder %q missing from plugin registry", sig),
			})
		}
	}
	for _, tool := range plugin.ContextTools() {
		if !plugin.KnownRefs(plugin.KindContext)[tool] {
			out = append(out, Finding{
				Name: "context:" + tool, OK: false,
				Msg: fmt.Sprintf("ContextProvider %q missing KindContext descriptor (use plugin.RegisterContext)", tool),
			})
		}
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
		case !build.KnownSignals()[q.Signal]:
			f.OK, f.Msg, f.Snippet = false, fmt.Sprintf("unknown signal %q", q.Signal), snippet()
		case !build.HasBuilder(q.Signal):
			f.OK, f.Msg, f.Snippet = false, fmt.Sprintf("signal %q registered but has no host builder", q.Signal), snippet()
		case !plugin.SignalEnabled(q.Signal):
			f.OK, f.Msg, f.Snippet = false, fmt.Sprintf("signal %q references disabled plugin", q.Signal), snippet()
		default:
			var missing []string
			for _, qf := range q.Filters {
				if qf.Ref != "" {
					if _, ok := directives.Filters[qf.Ref]; ok {
						continue
					}
					if plugin.HasFilter(qf.Ref) {
						continue
					}
					missing = append(missing, qf.Ref)
				}
			}
			if len(missing) > 0 {
				f.OK, f.Msg, f.Snippet = false, "unknown filters: "+strings.Join(missing, ", "), snippet()
			} else if resolved, err := directives.Resolve(q); err != nil {
				f.OK, f.Msg, f.Snippet = false, err.Error(), snippet()
			} else if _, err := filter.ExpandParams(q.Params, resolved); err != nil {
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

func printFinding(w io.Writer, sty render.ReportStyles, f Finding) int {
	switch {
	case f.OK:
		fmt.Fprintf(w, "  %s %s\n", sty.OK.Render(glyph.Check()), sty.Name.Render(f.Name))
		return 0
	case f.Warn:
		fmt.Fprintf(w, "  %s %s  %s\n", sty.Warn.Render(glyph.Warn()), sty.Name.Render(f.Name), sty.Warn.Render(f.Msg))
	default:
		fmt.Fprintf(w, "  %s %s  %s\n", sty.Err.Render(glyph.Cross()), sty.Name.Render(f.Name), sty.Err.Render(f.Msg))
	}
	if f.Snippet != "" {
		for _, line := range strings.Split(strings.TrimRight(redact.Line(f.Snippet), "\n"), "\n") {
			fmt.Fprintln(w, "      "+sty.Snippet.Render(line))
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
