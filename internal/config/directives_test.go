package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	sconfig "github.com/codyconfer/sisyphus/config"
)

func seedDirectives(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	mkdir(t, filepath.Join(home, DirFilters))
	mkdir(t, filepath.Join(home, DirFlights))

	write(t, filepath.Join(home, DirFilters, "no-bots.yaml"), `
name: no-bots
rules:
  - field: meta.author
    exclude: "(?i)bot$"
`)

	write(t, filepath.Join(home, DirQueries, "standup.yaml"), `
name: standup
signal: slack
params:
  channel: eng-standup
filters:
  - no-bots
  - exclude: "^:tada:"
`)
	write(t, filepath.Join(home, DirFlights, "morning.yaml"), `
name: morning
queries: [standup, my-prs]
`)
	write(t, filepath.Join(home, "triage.yaml"), `
name: triage
home: morning
flights: [morning]
queries: [standup]
filters: [no-bots]
contexts:
  kubectl: prod
  gcx: myorg.grafana.net
hooks:
  enter:
    bash: |
      echo entering triage
    powershell: |
      Write-Host entering triage
  exit:
    bash: |
      echo leaving triage
    powershell: |
      Write-Host leaving triage
status:
  - glyph: github
    bash: |
      echo triage-chip
    powershell: |
      Write-Output triage-chip
`)
	return home
}

func TestLoadStoreFromFiles(t *testing.T) {
	s, err := LoadDirectivesFromFiles(seedDirectives(t))
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Queries) != 1 || len(s.Filters) != 1 || len(s.Flights) != 1 || len(s.Roles) != 1 {
		t.Fatalf("map sizes q=%d f=%d fl=%d r=%d, want 1 each",
			len(s.Queries), len(s.Filters), len(s.Flights), len(s.Roles))
	}

	q := s.Queries["standup"]
	if q.Signal != "slack" || q.Params["channel"] != "eng-standup" {
		t.Errorf("query not parsed: %+v", q)
	}
	if len(q.Filters) != 2 {
		t.Fatalf("expected 2 filter entries, got %d", len(q.Filters))
	}
	if q.Filters[0].Ref != "no-bots" {
		t.Errorf("first filter should be a ref, got %+v", q.Filters[0])
	}
	if q.Filters[1].Inline == nil || q.Filters[1].Inline.Exclude != "^:tada:" {
		t.Errorf("second filter should be an inline exclude, got %+v", q.Filters[1])
	}

	fl := s.Flights["morning"]
	if fl.Name != "morning" || len(fl.Queries) != 2 || fl.Queries[0] != "standup" {
		t.Errorf("flight not parsed: %#v", fl)
	}

	rd := s.Roles["triage"]
	if rd.Name != "triage" || len(rd.Flights) != 1 || rd.Flights[0] != "morning" ||
		len(rd.Queries) != 1 || rd.Queries[0] != "standup" ||
		len(rd.Filters) != 1 || rd.Filters[0] != "no-bots" {
		t.Errorf("role not parsed: %#v", rd)
	}
	if !strings.Contains(rd.Hooks.Enter.Bash, "entering triage") ||
		!strings.Contains(rd.Hooks.Enter.PowerShell, "entering triage") ||
		!strings.Contains(rd.Hooks.Exit.Bash, "leaving triage") ||
		!strings.Contains(rd.Hooks.Exit.PowerShell, "leaving triage") {
		t.Errorf("role hooks not parsed: %#v", rd.Hooks)
	}
	if len(rd.Status) != 1 || rd.Status[0].Glyph != "github" ||
		!strings.Contains(rd.Status[0].Bash, "triage-chip") ||
		!strings.Contains(rd.Status[0].PowerShell, "triage-chip") {
		t.Errorf("role status not parsed: %#v", rd.Status)
	}

	if got := s.QueryNames(); len(got) != 1 || got[0] != "standup" {
		t.Errorf("QueryNames = %v", got)
	}
	if got := s.FilterNames(); len(got) != 1 || got[0] != "no-bots" {
		t.Errorf("FilterNames = %v", got)
	}
	if got := s.FlightNames(); len(got) != 1 || got[0] != "morning" {
		t.Errorf("FlightNames = %v", got)
	}
	if got := s.RoleNames(); len(got) != 1 || got[0] != "triage" {
		t.Errorf("RoleNames = %v", got)
	}
}

func TestSerializeDirRoundTrip(t *testing.T) {
	home := seedDirectives(t)

	qBlob, has, err := sconfig.SerializeDir(filepath.Join(home, DirQueries))
	if err != nil || !has {
		t.Fatalf("SerializeDir queries: has=%v err=%v", has, err)
	}
	queries, err := ParseQueries(qBlob)
	if err != nil {
		t.Fatal(err)
	}
	if q, ok := queries["standup"]; !ok || q.Signal != "slack" ||
		q.Filters[0].Ref != "no-bots" || q.Filters[1].Inline == nil {
		t.Errorf("ParseQueries round-trip wrong: %#v", queries)
	}

	fBlob, _, err := sconfig.SerializeDir(filepath.Join(home, DirFilters))
	if err != nil {
		t.Fatal(err)
	}
	filters, err := ParseFilters(fBlob)
	if err != nil {
		t.Fatal(err)
	}
	if f, ok := filters["no-bots"]; !ok || len(f.Rules) != 1 || f.Rules[0].Field != "meta.author" {
		t.Errorf("ParseFilters round-trip wrong: %#v", filters)
	}

	flBlob, _, err := sconfig.SerializeDir(filepath.Join(home, DirFlights))
	if err != nil {
		t.Fatal(err)
	}
	flights, err := ParseFlights(flBlob)
	if err != nil {
		t.Fatal(err)
	}
	if fl, ok := flights["morning"]; !ok || len(fl.Queries) != 2 {
		t.Errorf("ParseFlights round-trip wrong: %#v", flights)
	}

	rBlob, _, err := SerializeCollection(home, KindRoles)
	if err != nil {
		t.Fatal(err)
	}
	roles, err := ParseRoles(rBlob)
	if err != nil {
		t.Fatal(err)
	}
	if rd, ok := roles["triage"]; !ok || len(rd.Flights) != 1 || rd.Flights[0] != "morning" || rd.Home != "morning" {
		t.Errorf("ParseRoles round-trip wrong: %#v", roles)
	} else if rd.Contexts["kubectl"] != "prod" || rd.Contexts["gcx"] != "myorg.grafana.net" {
		t.Errorf("ParseRoles contexts = %#v", rd.Contexts)
	} else if !strings.Contains(rd.Hooks.Enter.Bash, "entering triage") ||
		!strings.Contains(rd.Hooks.Exit.PowerShell, "leaving triage") {
		t.Errorf("ParseRoles hooks = %#v", rd.Hooks)
	} else if len(rd.Status) != 1 || rd.Status[0].Glyph != "github" {
		t.Errorf("ParseRoles status = %#v", rd.Status)
	}

	s, err := NewDirectives(qBlob, fBlob, flBlob, rBlob)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Queries) != 1 || len(s.Filters) != 1 || len(s.Flights) != 1 || len(s.Roles) != 1 {
		t.Errorf("NewDirectives map sizes wrong: %+v", s)
	}
}

func TestParseNameFromFilename(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirFilters))
	write(t, filepath.Join(home, DirFilters, "quiet.yaml"), "rules:\n  - exclude: noise\n")
	blob, _, err := sconfig.SerializeDir(filepath.Join(home, DirFilters))
	if err != nil {
		t.Fatal(err)
	}
	filters, err := ParseFilters(blob)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := filters["quiet"]; !ok {
		t.Errorf("name should default to filename base: %v", filters)
	}
}

func TestLoadStoreFromFilesMissingDirs(t *testing.T) {
	s, err := LoadDirectivesFromFiles(t.TempDir())
	if err != nil {
		t.Fatalf("missing dirs should be fine: %v", err)
	}
	if len(s.Queries) != 0 || len(s.Filters) != 0 || len(s.Flights) != 0 || len(s.Roles) != 0 {
		t.Errorf("expected empty directives, got %+v", s)
	}
}

func TestLoadStoreFromFilesJSON(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	mkdir(t, filepath.Join(home, DirFilters))
	write(t, filepath.Join(home, DirFilters, "no-bots.json"), `{
	  "name": "no-bots",
	  "rules": [ { "field": "meta.author", "exclude": "(?i)bot$" } ]
	}`)
	write(t, filepath.Join(home, DirQueries, "standup.json"), `{
	  "name": "standup",
	  "signal": "slack",
	  "params": { "channel": "eng-standup" },
	  "filters": [ "no-bots", { "exclude": "^:tada:" } ]
	}`)

	s, err := LoadDirectivesFromFiles(home)
	if err != nil {
		t.Fatal(err)
	}
	q, ok := s.Queries["standup"]
	if !ok || q.Signal != "slack" {
		t.Fatalf("json query not parsed: %#v", q)
	}

	if len(q.Filters) != 2 || q.Filters[0].Ref != "no-bots" || q.Filters[1].Inline == nil ||
		q.Filters[1].Inline.Exclude != "^:tada:" {
		t.Fatalf("json query filters not parsed: %#v", q.Filters)
	}
	if _, ok := s.Filters["no-bots"]; !ok {
		t.Fatalf("json filter not loaded: %v", s.FilterNames())
	}
}

func TestParseDuplicateNameErrors(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirFilters))
	write(t, filepath.Join(home, DirFilters, "a.yaml"), "name: dup\nrules: []\n")
	write(t, filepath.Join(home, DirFilters, "b.yaml"), "name: dup\nrules: []\n")
	if _, err := LoadDirectivesFromFiles(home); err == nil {
		t.Fatal("expected duplicate-name error")
	}
}

func TestParseBadRegexFailsFast(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirFilters))
	write(t, filepath.Join(home, DirFilters, "bad.yaml"), "name: bad\nrules:\n  - exclude: \"(\"\n")
	if _, err := LoadDirectivesFromFiles(home); err == nil {
		t.Fatal("expected bad-regex error at load time")
	}
}

func TestResolveDereferencesNamedFilters(t *testing.T) {
	s, err := LoadDirectivesFromFiles(seedDirectives(t))
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := s.Resolve(s.Queries["standup"])
	if err != nil {
		t.Fatal(err)
	}

	if len(resolved) != 2 {
		t.Fatalf("expected 2 resolved filters, got %d", len(resolved))
	}
	if resolved[0].Name != "no-bots" {
		t.Errorf("first resolved should be the named set, got %q", resolved[0].Name)
	}
	if len(resolved[1].Rules) != 1 || resolved[1].Rules[0].Exclude != "^:tada:" {
		t.Errorf("inline set wrong: %+v", resolved[1])
	}
}

func TestResolveUnknownFilterErrors(t *testing.T) {
	s, err := LoadDirectivesFromFiles(seedDirectives(t))
	if err != nil {
		t.Fatal(err)
	}
	q := s.Queries["standup"]
	q.Filters = []QueryFilter{{Ref: "does-not-exist"}}
	if _, err := s.Resolve(q); err == nil {
		t.Fatal("expected error for unknown filter reference")
	}
}

func TestExpandParamsFromFilterAliases(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	mkdir(t, filepath.Join(home, DirFilters))
	write(t, filepath.Join(home, DirFilters, "repos.yaml"), `
name: repos
aliases:
  REPOS_ALIAS: "repo:org/a repo:org/b"
`)
	write(t, filepath.Join(home, DirQueries, "prs.yaml"), `
name: prs
signal: github
params:
  query: "is:open is:pr {REPOS_ALIAS} created:(3 days ago)"
filters: [repos]
`)
	s, err := LoadDirectivesFromFiles(home)
	if err != nil {
		t.Fatal(err)
	}
	params, err := s.ExpandParams(s.Queries["prs"])
	if err != nil {
		t.Fatal(err)
	}
	q := params["query"]
	if !strings.Contains(q, "repo:org/a repo:org/b") {
		t.Fatalf("aliases not expanded: %q", q)
	}
	if !strings.Contains(q, "created:>=") {
		t.Fatalf("relative created not expanded: %q", q)
	}
	if strings.Contains(q, "{REPOS_ALIAS}") || strings.Contains(q, "days ago") {
		t.Fatalf("shorthand left unexpanded: %q", q)
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}
