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
	mkdir(t, filepath.Join(home, DirFlights))

	write(t, filepath.Join(home, DirQueries, "no-bots.yaml"), `
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
queries: [standup, no-bots]
contexts:
  kubectl: prod
  gcx: myorg.example.net
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
	if len(s.Queries) != 2 || len(s.Flights) != 1 || len(s.Roles) != 1 {
		t.Fatalf("map sizes q=%d fl=%d r=%d, want 2/1/1",
			len(s.Queries), len(s.Flights), len(s.Roles))
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
		len(rd.Queries) != 2 || rd.Queries[0] != "standup" {
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

	if got := s.QueryNames(); len(got) != 2 || got[0] != "no-bots" || got[1] != "standup" {
		t.Errorf("QueryNames = %v", got)
	}
	if got := s.RunnableNames(); len(got) != 1 || got[0] != "standup" {
		t.Errorf("RunnableNames = %v", got)
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
	if f, ok := queries["no-bots"]; !ok || len(f.Rules) != 1 || f.Rules[0].Field != "meta.author" {
		t.Errorf("filter document round-trip wrong: %#v", queries)
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
	} else if rd.Contexts["kubectl"] != "prod" || rd.Contexts["gcx"] != "myorg.example.net" {
		t.Errorf("ParseRoles contexts = %#v", rd.Contexts)
	} else if !strings.Contains(rd.Hooks.Enter.Bash, "entering triage") ||
		!strings.Contains(rd.Hooks.Exit.PowerShell, "leaving triage") {
		t.Errorf("ParseRoles hooks = %#v", rd.Hooks)
	} else if len(rd.Status) != 1 || rd.Status[0].Glyph != "github" {
		t.Errorf("ParseRoles status = %#v", rd.Status)
	}

	s, err := NewDirectives(qBlob, flBlob, rBlob)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Queries) != 2 || len(s.Flights) != 1 || len(s.Roles) != 1 {
		t.Errorf("NewDirectives map sizes wrong: %+v", s)
	}
}

func TestParseNameFromFilename(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	write(t, filepath.Join(home, DirQueries, "quiet.yaml"), "rules:\n  - exclude: noise\n")
	blob, _, err := sconfig.SerializeDir(filepath.Join(home, DirQueries))
	if err != nil {
		t.Fatal(err)
	}
	queries, err := ParseQueries(blob)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := queries["quiet"]; !ok {
		t.Errorf("name should default to filename base: %v", queries)
	}
}

func TestParseMultiDocYAMLFile(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	write(t, filepath.Join(home, DirQueries, "standup.yaml"), `
name: no-bots
rules:
  - field: meta.author
    exclude: "(?i)bot$"
---
name: standup
signal: slack
filters: [no-bots]
params:
  channel: eng-standup
`)
	s, err := LoadDirectivesFromFiles(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Queries) != 2 {
		t.Fatalf("expected both documents, got %v", s.QueryNames())
	}
	if got := s.RunnableNames(); len(got) != 1 || got[0] != "standup" {
		t.Errorf("RunnableNames = %v", got)
	}
	if got := s.FilterNames(); len(got) != 1 || got[0] != "no-bots" {
		t.Errorf("FilterNames = %v", got)
	}
	resolved, err := s.Resolve(s.Queries["standup"])
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 1 || resolved[0].Name != "no-bots" {
		t.Errorf("cross-document reference not resolved: %+v", resolved)
	}
}

func TestParseYAMLSequenceFile(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	write(t, filepath.Join(home, DirQueries, "set.yaml"), `
- name: no-bots
  rules:
    - exclude: "(?i)bot$"
- name: standup
  signal: slack
`)
	s, err := LoadDirectivesFromFiles(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Queries) != 2 {
		t.Fatalf("expected both list entries, got %v", s.QueryNames())
	}
}

func TestParseJSONArrayFile(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	write(t, filepath.Join(home, DirQueries, "set.json"), `[
	  { "name": "no-bots", "rules": [ { "exclude": "(?i)bot$" } ] },
	  { "name": "standup", "signal": "slack", "filters": ["no-bots"] }
	]`)
	s, err := LoadDirectivesFromFiles(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Queries) != 2 {
		t.Fatalf("expected both array entries, got %v", s.QueryNames())
	}
	if _, err := s.Resolve(s.Queries["standup"]); err != nil {
		t.Fatal(err)
	}
}

func TestParseMultiDocRequiresNames(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	write(t, filepath.Join(home, DirQueries, "set.yaml"), "signal: slack\n---\nname: other\nsignal: github\n")
	_, err := LoadDirectivesFromFiles(home)
	if err == nil {
		t.Fatal("expected an error for an unnamed document in a multi-document file")
	}
	if !strings.Contains(err.Error(), "no name") {
		t.Errorf("error should explain the missing name, got %v", err)
	}
}

func TestLoadStoreFromFilesMissingDirs(t *testing.T) {
	s, err := LoadDirectivesFromFiles(t.TempDir())
	if err != nil {
		t.Fatalf("missing dirs should be fine: %v", err)
	}
	if len(s.Queries) != 0 || len(s.Flights) != 0 || len(s.Roles) != 0 {
		t.Errorf("expected empty directives, got %+v", s)
	}
}

func TestLoadStoreFromFilesJSON(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	write(t, filepath.Join(home, DirQueries, "no-bots.json"), `{
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
	if _, ok := s.Filter("no-bots"); !ok {
		t.Fatalf("json filter not loaded: %v", s.FilterNames())
	}
}

func TestParseDuplicateNameErrors(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	write(t, filepath.Join(home, DirQueries, "a.yaml"), "name: dup\nrules: []\n")
	write(t, filepath.Join(home, DirQueries, "b.yaml"), "name: dup\nrules: []\n")
	if _, err := LoadDirectivesFromFiles(home); err == nil {
		t.Fatal("expected duplicate-name error")
	}
}

func TestParseBadRegexFailsFast(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	write(t, filepath.Join(home, DirQueries, "bad.yaml"), "name: bad\nrules:\n  - exclude: \"(\"\n")
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

func TestResolveIncludesOwnRules(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	write(t, filepath.Join(home, DirQueries, "prs.yaml"), `
name: prs
signal: github
rules:
  - field: meta.author
    exclude: "(?i)bot$"
filters:
  - include: "deploy"
`)
	s, err := LoadDirectivesFromFiles(home)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := s.Resolve(s.Queries["prs"])
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 1 {
		t.Fatalf("own rules and inline rules should form one set, got %d", len(resolved))
	}
	if len(resolved[0].Rules) != 2 ||
		resolved[0].Rules[0].Exclude != "(?i)bot$" || resolved[0].Rules[1].Include != "deploy" {
		t.Errorf("own rules should come before inline rules: %+v", resolved[0].Rules)
	}
}

func TestFilterRejectsSignalOnlyQuery(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	write(t, filepath.Join(home, DirQueries, "prs.yaml"), "name: prs\nsignal: github\n")
	write(t, filepath.Join(home, DirQueries, "other.yaml"), "name: other\nsignal: github\nfilters: [prs]\n")
	s, err := LoadDirectivesFromFiles(home)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Filter("prs"); ok {
		t.Error("a query with no rules/aliases/keywords is not a filter")
	}
	if _, err := s.Resolve(s.Queries["other"]); err == nil {
		t.Fatal("referencing a query with no filter content should error")
	}
}

func TestExplicitTypes(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	write(t, filepath.Join(home, DirQueries, "set.yaml"), `
name: no-bots
type: filter
rules:
  - exclude: "(?i)bot$"
---
name: prs
type: query
signal: github
filters: [no-bots]
`)
	s, err := LoadDirectivesFromFiles(home)
	if err != nil {
		t.Fatal(err)
	}
	if k := s.Queries["no-bots"].Kind(); k != TypeFilter {
		t.Errorf("no-bots Kind = %q, want filter", k)
	}
	if k := s.Queries["prs"].Kind(); k != TypeQuery {
		t.Errorf("prs Kind = %q, want query", k)
	}
	if s.Queries["no-bots"].Runnable() {
		t.Error("type: filter should never be runnable")
	}
	if got := s.RunnableNames(); len(got) != 1 || got[0] != "prs" {
		t.Errorf("RunnableNames = %v", got)
	}
	if got := s.FilterNames(); len(got) != 1 || got[0] != "no-bots" {
		t.Errorf("FilterNames = %v", got)
	}
}

func TestTypeQueryKeepsItsRulesPrivate(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	write(t, filepath.Join(home, DirQueries, "set.yaml"), `
name: prs
type: query
signal: github
rules:
  - exclude: "(?i)bot$"
---
name: other
signal: github
filters: [prs]
`)
	s, err := LoadDirectivesFromFiles(home)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Filter("prs"); ok {
		t.Error("a type: query document should not be referenceable as a filter")
	}
	if got := s.FilterNames(); len(got) != 0 {
		t.Errorf("FilterNames = %v, want none", got)
	}
	if _, err := s.Resolve(s.Queries["other"]); err == nil {
		t.Fatal("referencing a type: query document should error")
	}
	resolved, err := s.Resolve(s.Queries["prs"])
	if err != nil {
		t.Fatal(err)
	}
	if len(resolved) != 1 || len(resolved[0].Rules) != 1 {
		t.Errorf("its own rules should still apply to itself: %+v", resolved)
	}
}

func TestInvalidTypesRejected(t *testing.T) {
	cases := map[string]string{
		"query without a signal": "name: x\ntype: query\n",
		"filter with a signal":   "name: x\ntype: filter\nsignal: github\nrules:\n  - exclude: y\n",
		"filter with no rules":   "name: x\ntype: filter\n",
		"flight with a signal":   "name: x\ntype: flight\nsignal: github\nqueries: [a]\n",
		"flight with no queries": "name: x\ntype: flight\n",
		"flight with rules":      "name: x\ntype: flight\nqueries: [a]\nrules:\n  - exclude: y\n",
		"role with a signal":     "name: x\ntype: role\nsignal: github\n",
		"unknown type":           "name: x\ntype: dashboard\nsignal: github\n",
	}
	for label, body := range cases {
		t.Run(label, func(t *testing.T) {
			home := t.TempDir()
			mkdir(t, filepath.Join(home, DirQueries))
			write(t, filepath.Join(home, DirQueries, "x.yaml"), body)
			if _, err := LoadDirectivesFromFiles(home); err == nil {
				t.Fatalf("expected an error for %s", label)
			}
		})
	}
}

func TestTypeRoutesDocsAcrossCollections(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	mkdir(t, filepath.Join(home, DirFlights))
	write(t, filepath.Join(home, DirQueries, "triage.yaml"), `
name: triage
type: flight
queries: [incidents]
---
name: incidents
type: query
signal: github
---
name: oncall
type: role
flights: [triage]
`)
	write(t, filepath.Join(home, DirFlights, "shared.yaml"), `
name: no-bots
type: filter
rules:
  - exclude: "(?i)bot$"
`)
	s, err := LoadDirectivesFromFiles(home)
	if err != nil {
		t.Fatal(err)
	}
	fl, ok := s.Flights["triage"]
	if !ok || len(fl.Queries) != 1 || fl.Queries[0] != "incidents" {
		t.Fatalf("flight declared in %s not routed: %#v", DirQueries, s.Flights)
	}
	if fl.Type != TypeFlight {
		t.Errorf("flight Type = %q, want flight", fl.Type)
	}
	if _, ok := s.Queries["triage"]; ok {
		t.Error("a type: flight document should not land in Queries")
	}
	rd, ok := s.Roles["oncall"]
	if !ok || len(rd.Flights) != 1 || rd.Flights[0] != "triage" {
		t.Fatalf("role declared in %s not routed: %#v", DirQueries, s.Roles)
	}
	if _, ok := s.Filter("no-bots"); !ok {
		t.Errorf("filter declared in %s not routed: %v", DirFlights, s.FilterNames())
	}
	if got := s.RunnableNames(); len(got) != 1 || got[0] != "incidents" {
		t.Errorf("RunnableNames = %v", got)
	}
}

func TestCollectionStillDefaultsTheType(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirFlights))
	write(t, filepath.Join(home, DirFlights, "morning.yaml"), "name: morning\nqueries: [a, b]\n")
	write(t, filepath.Join(home, "oncall.yaml"), "name: oncall\nqueries: [a]\n")
	s, err := LoadDirectivesFromFiles(home)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Flights["morning"]; !ok {
		t.Errorf("untyped doc in %s should stay a flight: %#v", DirFlights, s.Flights)
	}
	if _, ok := s.Roles["oncall"]; !ok {
		t.Errorf("untyped doc at the config root should stay a role: %#v", s.Roles)
	}
	if s.Flights["morning"].Type != TypeAuto {
		t.Errorf("inferred flight should keep an empty Type, got %q", s.Flights["morning"].Type)
	}
}

func TestNamesCollideOnlyWithinAKind(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	mkdir(t, filepath.Join(home, DirFlights))
	write(t, filepath.Join(home, DirQueries, "demo.yaml"), "name: demo\nsignal: github\n")
	write(t, filepath.Join(home, DirFlights, "demo.yaml"), "name: demo\nqueries: [demo]\n")
	s, err := LoadDirectivesFromFiles(home)
	if err != nil {
		t.Fatalf("a query and a flight may share a name: %v", err)
	}
	if _, ok := s.Queries["demo"]; !ok {
		t.Error("query demo missing")
	}
	if _, ok := s.Flights["demo"]; !ok {
		t.Error("flight demo missing")
	}

	clash := t.TempDir()
	mkdir(t, filepath.Join(clash, DirQueries))
	mkdir(t, filepath.Join(clash, DirFlights))
	write(t, filepath.Join(clash, DirQueries, "a.yaml"), "name: dup\ntype: flight\nqueries: [x]\n")
	write(t, filepath.Join(clash, DirFlights, "b.yaml"), "name: dup\nqueries: [y]\n")
	if _, err := LoadDirectivesFromFiles(clash); err == nil {
		t.Fatal("two flights with the same name should collide across collections")
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
	write(t, filepath.Join(home, DirQueries, "repos.yaml"), `
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

func TestExpandParamsFromOwnAliases(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	write(t, filepath.Join(home, DirQueries, "prs.yaml"), `
name: prs
signal: github
aliases:
  REPOS_ALIAS: "repo:org/a"
params:
  query: "is:open is:pr {REPOS_ALIAS}"
`)
	s, err := LoadDirectivesFromFiles(home)
	if err != nil {
		t.Fatal(err)
	}
	params, err := s.ExpandParams(s.Queries["prs"])
	if err != nil {
		t.Fatal(err)
	}
	if got := params["query"]; !strings.Contains(got, "repo:org/a") {
		t.Fatalf("a query's own aliases should expand its params: %q", got)
	}
}

func TestQueryTitleParsesAndFallsBackToName(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	write(t, filepath.Join(home, DirQueries, "my-open-prs.yaml"), `
name: my-open-prs
title: Open pull requests
signal: github
`)
	write(t, filepath.Join(home, DirQueries, "standup.yaml"), `
name: standup
signal: slack
`)

	blob, _, err := sconfig.SerializeDir(filepath.Join(home, DirQueries))
	if err != nil {
		t.Fatal(err)
	}
	queries, err := ParseQueries(blob)
	if err != nil {
		t.Fatal(err)
	}

	titled, ok := queries["my-open-prs"]
	if !ok {
		t.Fatalf("my-open-prs missing from %v", queries)
	}
	if titled.Title != "Open pull requests" {
		t.Errorf("Title = %q, want %q", titled.Title, "Open pull requests")
	}
	if got := titled.Display(); got != "Open pull requests" {
		t.Errorf("Display = %q, want the title", got)
	}

	untitled := queries["standup"]
	if got := untitled.Display(); got != "standup" {
		t.Errorf("Display without a title = %q, want the name", got)
	}
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}
