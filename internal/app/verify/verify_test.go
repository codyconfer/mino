package verify

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"strings"
	"testing"

	"github.com/codyconfer/sisyphus/secret"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/testenv"
)

func directivesFrom(t *testing.T, files map[string]string) *config.Directives {
	t.Helper()
	blob, err := json.Marshal(files)
	if err != nil {
		t.Fatal(err)
	}
	d, err := config.ParseDirectives(blob)
	if err != nil {
		t.Fatalf("ParseDirectives: %v", err)
	}
	return d
}

func formatterFixture(t *testing.T) *config.Directives {
	t.Helper()
	return directivesFrom(t, map[string]string{
		"queries/prs.yaml":         "name: prs\ntype: query\nsignal: github\n",
		"flights/morning.yaml":     "name: morning\ntype: flight\nqueries: [prs]\nformatter: standup\n",
		"formatters/standup.yaml":  "name: standup\ntype: formatter\ntitle: Standup\ntemplate: |\n  {{ .Count }} items in {{ .Name }}\n",
		"formatters/leaflet.yaml":  "name: leaflet\ntype: formatter\ntemplate: |\n  nothing to interpolate\n",
		"formatters/greedy.yaml":   "name: greedy\ntype: formatter\ntemplate: |\n  {{ (index .Items 0).Title }}\n",
		"roles/scoped.yaml":        "name: scoped\ntype: role\nflights: [morning]\nformatters: [standup]\n",
		"roles/unscoped.yaml":      "name: unscoped\ntype: role\nflights: [morning]\n",
		"roles/dangling.yaml":      "name: dangling\ntype: role\nformatters: [ghost]\n",
		"roles/queryscoped.yaml":   "name: queryscoped\ntype: role\nqueries: [prs]\n",
		"formatters/quietly.yaml":  "name: quietly\ntype: formatter\ntemplate: |\n  {{ range .Items }}{{ .Title }}{{ end }}\n",
		"queries/reviews.yaml":     "name: reviews\ntype: query\nsignal: github\nformatter: standup\n",
		"flights/afternoon.yaml":   "name: afternoon\ntype: flight\nqueries: [prs]\n",
		"formatters/rendered.yaml": "name: rendered\ntype: formatter\ntemplate: \"{{ .Count }}\"\n",
	})
}

func findingByName(t *testing.T, findings []Finding, name string) Finding {
	t.Helper()
	for _, f := range findings {
		if f.Name == name {
			return f
		}
	}
	t.Fatalf("no finding named %q in %+v", name, findings)
	return Finding{}
}

func TestFormatterChecks(t *testing.T) {
	cases := []struct {
		name     string
		fd       config.FormatterDef
		ok       bool
		warn     bool
		contains string
	}{
		{
			name: "healthy",
			fd:   config.FormatterDef{Name: "standup", Template: "{{ .Count }} items"},
			ok:   true,
		},
		{
			name:     "no template",
			fd:       config.FormatterDef{Name: "bare"},
			contains: "formatter has no template",
		},
		{
			name:     "broken template",
			fd:       config.FormatterDef{Name: "broken", Template: "{{ .Count "},
			contains: "parsing formatter",
		},
		{
			name:     "static template",
			fd:       config.FormatterDef{Name: "static", Template: "no interpolation here\n"},
			warn:     true,
			contains: "template interpolates nothing",
		},
		{
			name:     "empty result set",
			fd:       config.FormatterDef{Name: "greedy", Template: "{{ (index .Items 0).Title }}"},
			warn:     true,
			contains: "fails on an empty result set",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			f := Formatter(tc.fd.Name, tc.fd)
			if f.OK != tc.ok {
				t.Errorf("OK = %v, want %v (msg %q)", f.OK, tc.ok, f.Msg)
			}
			if f.Warn != tc.warn {
				t.Errorf("Warn = %v, want %v (msg %q)", f.Warn, tc.warn, f.Msg)
			}
			if tc.contains == "" {
				if f.Msg != "" {
					t.Errorf("Msg = %q, want empty", f.Msg)
				}
				return
			}
			if !strings.Contains(f.Msg, tc.contains) {
				t.Errorf("Msg = %q, want it to mention %q", f.Msg, tc.contains)
			}
			if f.Snippet == "" {
				t.Error("a problem finding should carry the config snippet")
			}
		})
	}
}

func TestFormattersCoversEveryLoadedFormatter(t *testing.T) {
	d := formatterFixture(t)
	findings := Formatters(d)

	if len(findings) != len(d.Formatters) {
		t.Fatalf("Formatters returned %d findings for %d formatters", len(findings), len(d.Formatters))
	}
	if f := findingByName(t, findings, "standup"); !f.OK || f.Warn {
		t.Errorf("standup = %+v, want a clean OK", f)
	}
	if f := findingByName(t, findings, "quietly"); !f.OK || f.Warn {
		t.Errorf("quietly = %+v, want a clean OK", f)
	}
	if f := findingByName(t, findings, "leaflet"); f.OK || !f.Warn ||
		!strings.Contains(f.Msg, "template interpolates nothing") {
		t.Errorf("leaflet = %+v, want the interpolates-nothing warning", f)
	}
	if f := findingByName(t, findings, "greedy"); f.OK || !f.Warn ||
		!strings.Contains(f.Msg, "fails on an empty result set") {
		t.Errorf("greedy = %+v, want the empty-result-set warning", f)
	}
}

func TestFlightFlagsUnknownFormatter(t *testing.T) {
	d := formatterFixture(t)

	good := Flight(d, "morning", d.Flights["morning"])
	if !good.OK {
		t.Fatalf("morning names a defined formatter: %+v", good)
	}

	fl := d.Flights["morning"]
	fl.Formatter = "ghost"
	bad := Flight(d, "morning", fl)
	if bad.OK || bad.Warn || !strings.Contains(bad.Msg, `unknown formatter "ghost"`) {
		t.Errorf("flight finding = %+v, want an unknown formatter problem", bad)
	}
	if bad.Snippet == "" {
		t.Error("flight problem should carry the config snippet")
	}
}

func TestQueryFlagsUnknownFormatter(t *testing.T) {
	d := formatterFixture(t)

	good := Query(d, "reviews", config.Query{Name: "reviews", Formatter: "standup"})
	if !good.OK {
		t.Fatalf("query naming a defined formatter = %+v", good)
	}

	bad := Query(d, "reviews", config.Query{Name: "reviews", Formatter: "ghost"})
	if bad.OK || bad.Warn || !strings.Contains(bad.Msg, `unknown formatter "ghost"`) {
		t.Errorf("query finding = %+v, want an unknown formatter problem", bad)
	}
}

func TestRolesFlagsUndefinedFormatter(t *testing.T) {
	findings := Roles(formatterFixture(t))

	f := findingByName(t, findings, "dangling")
	if f.OK || f.Warn {
		t.Errorf("dangling = %+v, want a hard problem", f)
	}
	if !strings.Contains(f.Msg, "references undefined: formatter ghost") {
		t.Errorf("Msg = %q, want it to flow through the references-undefined message", f.Msg)
	}
}

func TestRolesWarnsOnFormatterOutOfScope(t *testing.T) {
	findings := Roles(formatterFixture(t))

	f := findingByName(t, findings, "unscoped")
	if f.OK || !f.Warn {
		t.Fatalf("unscoped = %+v, want a warning", f)
	}
	for _, want := range []string{"formatters not in role scope", "flight morning", `"standup"`} {
		if !strings.Contains(f.Msg, want) {
			t.Errorf("Msg = %q, want it to mention %q", f.Msg, want)
		}
	}

	if f := findingByName(t, findings, "scoped"); !f.OK || f.Warn {
		t.Errorf("scoped lists the formatter it reaches, got %+v", f)
	}
}

func TestRolesWarnsOnQueryFormatterOutOfScope(t *testing.T) {
	d := directivesFrom(t, map[string]string{
		"queries/reviews.yaml":    "name: reviews\ntype: query\nsignal: github\nformatter: standup\n",
		"formatters/standup.yaml": "name: standup\ntype: formatter\ntemplate: \"{{ .Count }}\"\n",
		"roles/ic.yaml":           "name: ic\ntype: role\nqueries: [reviews]\n",
		"roles/lead.yaml":         "name: lead\ntype: role\nqueries: [reviews]\nformatters: [standup]\n",
	})
	findings := Roles(d)

	f := findingByName(t, findings, "ic")
	if f.OK || !f.Warn || !strings.Contains(f.Msg, "query reviews") {
		t.Errorf("ic = %+v, want a query-scope warning", f)
	}
	if f := findingByName(t, findings, "lead"); !f.OK || f.Warn {
		t.Errorf("lead = %+v, want a clean OK", f)
	}
}

func TestRolesWarnsOnHomeFlightFormatterOutOfScope(t *testing.T) {
	d := directivesFrom(t, map[string]string{
		"queries/prs.yaml":        "name: prs\ntype: query\nsignal: github\n",
		"flights/morning.yaml":    "name: morning\ntype: flight\nqueries: [prs]\nformatter: standup\n",
		"formatters/standup.yaml": "name: standup\ntype: formatter\ntemplate: \"{{ .Count }}\"\n",
		"roles/ic.yaml":           "name: ic\ntype: role\nhome: morning\n",
	})

	f := findingByName(t, Roles(d), "ic")
	if f.OK || !f.Warn || !strings.Contains(f.Msg, "flight morning") {
		t.Errorf("ic = %+v, want the home flight formatter warning", f)
	}
	if strings.Count(f.Msg, "flight morning") != 1 {
		t.Errorf("Msg = %q, want the flight named once", f.Msg)
	}
}

func TestRunFormattersTargetCountsProblems(t *testing.T) {
	cfg := &config.Config{Output: "terminal"}

	clean := directivesFrom(t, map[string]string{
		"formatters/standup.yaml": "name: standup\ntype: formatter\ntemplate: \"{{ .Count }} items\"\n",
	})
	var buf bytes.Buffer
	if err := Run(context.Background(), &buf, cfg, clean, nil, "formatters"); err != nil {
		t.Fatalf("Run on a healthy formatter = %v\n%s", err, buf.String())
	}
	out := buf.String()
	if !strings.Contains(out, "Formatters") || !strings.Contains(out, "standup") {
		t.Errorf("output missing the formatters section:\n%s", out)
	}
	if strings.Contains(out, "Queries") {
		t.Errorf("target=formatters should only run its own section:\n%s", out)
	}

	broken := formatterFixture(t)
	broken.Formatters["bare"] = config.FormatterDef{Name: "bare", Type: config.TypeFormatter}
	buf.Reset()
	err := Run(context.Background(), &buf, cfg, broken, nil, "formatters")
	if err == nil {
		t.Fatalf("Run with a template-less formatter should fail\n%s", buf.String())
	}
	if !strings.Contains(err.Error(), "1 problem(s) found") {
		t.Errorf("err = %v, want the problem count to include only the hard failure", err)
	}
	if !strings.Contains(buf.String(), "formatter has no template") {
		t.Errorf("output missing the diagnosis:\n%s", buf.String())
	}
}

func TestRunAllIncludesFormatters(t *testing.T) {
	var buf bytes.Buffer
	d := directivesFrom(t, map[string]string{
		"formatters/standup.yaml": "name: standup\ntype: formatter\ntemplate: \"{{ .Count }} items\"\n",
	})
	_ = Run(context.Background(), &buf, &config.Config{Output: "terminal"}, d, nil, "all")

	out := buf.String()
	for _, want := range []string{"Config", "Roles", "Flights", "Queries", "Formatters"} {
		if !strings.Contains(out, want) {
			t.Errorf("verify all output missing the %q section:\n%s", want, out)
		}
	}
	if i, j := strings.Index(out, "Queries"), strings.Index(out, "Formatters"); i > j {
		t.Errorf("formatters section should follow queries:\n%s", out)
	}
}

func TestRolePerItemMatchesTheCollection(t *testing.T) {
	directives := formatterFixture(t)

	for _, name := range directives.RoleNames() {
		want := findingByName(t, Roles(directives), name)
		got := Role(directives, name, directives.Roles[name])
		if got.OK != want.OK || got.Warn != want.Warn || got.Msg != want.Msg {
			t.Errorf("Role(%q) = %+v, want %+v", name, got, want)
		}
	}
}

func TestRoleChecksAnUnsavedDraft(t *testing.T) {
	directives := formatterFixture(t)

	clean := Role(directives, "draft", config.RoleDef{Name: "draft"})
	if !clean.OK || clean.Warn {
		t.Errorf("an empty draft = %+v, want a clean finding", clean)
	}

	bad := Role(directives, "draft", config.RoleDef{
		Name:    "draft",
		Home:    "ghost-flight",
		Queries: []string{"ghost-query"},
	})
	if bad.OK {
		t.Fatalf("a draft with unknown references = %+v, want a problem", bad)
	}
	for _, want := range []string{"home flight ghost-flight", "query ghost-query"} {
		if !strings.Contains(bad.Msg, want) {
			t.Errorf("Msg = %q, want it to mention %q", bad.Msg, want)
		}
	}
}

func TestRunRejectsAnUnknownTarget(t *testing.T) {
	var buf bytes.Buffer
	err := Run(context.Background(), &buf, &config.Config{Output: "terminal"},
		directivesFrom(t, map[string]string{}), nil, "flight")
	if err == nil {
		t.Fatalf("Run with a typo'd target = nil, want a usage error\n%s", buf.String())
	}
	if errs.KindOf(err) != errs.KindUsage {
		t.Errorf("kind = %v, want KindUsage (err: %v)", errs.KindOf(err), err)
	}
	if buf.Len() != 0 {
		t.Errorf("an unknown target still printed a report: %q", buf.String())
	}
}

func TestRunAcceptsEveryAdvertisedTarget(t *testing.T) {
	testenv.Isolate(t)
	for _, target := range Targets() {
		var buf bytes.Buffer
		err := Run(context.Background(), &buf, &config.Config{Output: "terminal"},
			directivesFrom(t, map[string]string{}), nil, target)
		if errs.KindOf(err) == errs.KindUsage {
			t.Errorf("target %q rejected: %v", target, err)
		}
		if buf.Len() == 0 {
			t.Errorf("target %q printed nothing", target)
		}
	}
	if !slices.Contains(Targets(), "plugins") {
		t.Error("Targets() omits the live `plugins` section")
	}
}

func TestSecretBackendMatchesSisyphus(t *testing.T) {
	testenv.Isolate(t)
	backendFinding := func(t *testing.T, backend string) Finding {
		t.Helper()
		cfg := &config.Config{Output: "terminal"}
		cfg.Backup.SecretBackend = backend
		return findingByName(t, Config(cfg, directivesFrom(t, map[string]string{})), "backup.secret_backend")
	}

	for _, backend := range append(secret.Backends(), "") {
		if f := backendFinding(t, backend); !f.OK {
			t.Errorf("secret_backend %q reported as a problem (%q) but sisyphus accepts it", backend, f.Msg)
		}
	}
	for _, backend := range []string{"bw", "op"} {
		if f := backendFinding(t, backend); !f.OK {
			t.Errorf("secret_backend %q must validate; `mino backup` already accepts it", backend)
		}
	}
	f := backendFinding(t, "nope")
	if f.OK {
		t.Fatal("an unknown backend was accepted")
	}
	for _, want := range []string{"bitwarden", "1password", "keyring"} {
		if !strings.Contains(f.Msg, want) {
			t.Errorf("Msg = %q, want it to list %q", f.Msg, want)
		}
	}
}
