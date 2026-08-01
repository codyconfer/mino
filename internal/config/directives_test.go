package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/errs"
)

func seedDirectives(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	mkdir(t, filepath.Join(home, DirFlights))

	write(t, filepath.Join(home, DirQueries, "no-bots.yaml"), `
name: no-bots
type: filter
rules:
  - field: meta.author
    exclude: "(?i)bot$"
`)

	write(t, filepath.Join(home, DirQueries, "standup.yaml"), `
name: standup
type: query
signal: slack
params:
  channel: eng-standup
filters:
  - no-bots
  - exclude: "^:tada:"
`)
	write(t, filepath.Join(home, DirFlights, "morning.yaml"), `
name: morning
type: flight
queries: [standup, my-prs]
`)
	write(t, filepath.Join(home, "triage.yaml"), `
name: triage
type: role
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

func TestSerializeDirectivesRoundTrip(t *testing.T) {
	home := seedDirectives(t)

	blob, has, err := SerializeDirectives(home)
	if err != nil || !has {
		t.Fatalf("SerializeDirectives: has=%v err=%v", has, err)
	}
	s, err := ParseDirectives(blob)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Queries) != 2 || len(s.Flights) != 1 || len(s.Roles) != 1 {
		t.Fatalf("map sizes q=%d fl=%d r=%d, want 2/1/1", len(s.Queries), len(s.Flights), len(s.Roles))
	}
	if q, ok := s.Queries["standup"]; !ok || q.Signal != "slack" ||
		q.Filters[0].Ref != "no-bots" || q.Filters[1].Inline == nil {
		t.Errorf("query round-trip wrong: %#v", s.Queries)
	}
	if f, ok := s.Queries["no-bots"]; !ok || len(f.Rules) != 1 || f.Rules[0].Field != "meta.author" {
		t.Errorf("filter round-trip wrong: %#v", s.Queries)
	}
	if fl, ok := s.Flights["morning"]; !ok || len(fl.Queries) != 2 {
		t.Errorf("flight round-trip wrong: %#v", s.Flights)
	}

	rd, ok := s.Roles["triage"]
	if !ok || len(rd.Flights) != 1 || rd.Flights[0] != "morning" || rd.Home != "morning" {
		t.Fatalf("role round-trip wrong: %#v", s.Roles)
	}
	if rd.Contexts["kubectl"] != "prod" || rd.Contexts["gcx"] != "myorg.example.net" {
		t.Errorf("role contexts = %#v", rd.Contexts)
	}
	if !strings.Contains(rd.Hooks.Enter.Bash, "entering triage") ||
		!strings.Contains(rd.Hooks.Exit.PowerShell, "leaving triage") {
		t.Errorf("role hooks = %#v", rd.Hooks)
	}
	if len(rd.Status) != 1 || rd.Status[0].Glyph != "github" {
		t.Errorf("role status = %#v", rd.Status)
	}
}

func TestParseNameFromFilename(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	write(t, filepath.Join(home, DirQueries, "quiet.yaml"), "type: filter\nrules:\n  - exclude: noise\n")
	s, err := LoadDirectivesFromFiles(home)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Queries["quiet"]; !ok {
		t.Errorf("name should default to filename base: %v", s.QueryNames())
	}
}

func TestParseMultiDocYAMLFile(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	write(t, filepath.Join(home, DirQueries, "standup.yaml"), `
name: no-bots
type: filter
rules:
  - field: meta.author
    exclude: "(?i)bot$"
---
name: standup
type: query
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
  type: filter
  rules:
    - exclude: "(?i)bot$"
- name: standup
  type: query
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
	  { "name": "no-bots", "type": "filter", "rules": [ { "exclude": "(?i)bot$" } ] },
	  { "name": "standup", "type": "query", "signal": "slack", "filters": ["no-bots"] }
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
	write(t, filepath.Join(home, DirQueries, "set.yaml"), "type: query\nsignal: slack\n---\nname: other\ntype: query\nsignal: github\n")
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
	  "type": "filter",
	  "rules": [ { "field": "meta.author", "exclude": "(?i)bot$" } ]
	}`)
	write(t, filepath.Join(home, DirQueries, "standup.json"), `{
	  "name": "standup",
	  "type": "query",
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
	write(t, filepath.Join(home, DirQueries, "a.yaml"), "name: dup\ntype: filter\nrules:\n  - exclude: x\n")
	write(t, filepath.Join(home, DirQueries, "b.yaml"), "name: dup\ntype: filter\nrules:\n  - exclude: y\n")
	if _, err := LoadDirectivesFromFiles(home); err == nil {
		t.Fatal("expected duplicate-name error")
	}
}

func TestParseBadRegexFailsFast(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	write(t, filepath.Join(home, DirQueries, "bad.yaml"), "name: bad\ntype: filter\nrules:\n  - exclude: \"(\"\n")
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
type: query
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
	write(t, filepath.Join(home, DirQueries, "prs.yaml"), "name: prs\ntype: query\nsignal: github\n")
	write(t, filepath.Join(home, DirQueries, "other.yaml"), "name: other\ntype: query\nsignal: github\nfilters: [prs]\n")
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
type: query
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

func TestDirectoryNoLongerImpliesTheType(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirFlights))
	write(t, filepath.Join(home, DirFlights, "morning.yaml"), "name: morning\ntype: query\nsignal: github\n")
	write(t, filepath.Join(home, "oncall.yaml"), "name: oncall\ntype: flight\nqueries: [morning]\n")
	s, err := LoadDirectivesFromFiles(home)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Queries["morning"]; !ok {
		t.Errorf("`type: query` in %s should stay a query: %#v", DirFlights, s.Flights)
	}
	if _, ok := s.Flights["oncall"]; !ok {
		t.Errorf("`type: flight` at the home root should stay a flight: %#v", s.Roles)
	}
	if len(s.Roles) != 0 {
		t.Errorf("nothing declared a role: %#v", s.Roles)
	}
}

func TestTypelessDirectiveDocumentErrorsNamingTheFile(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, "team", "gh"))
	write(t, filepath.Join(home, "team", "gh", "prs.yaml"), "name: prs\nsignal: github\n")
	_, err := LoadDirectivesFromFiles(home)
	if err == nil {
		t.Fatal("expected an error for a document with directive fields but no type")
	}
	if !strings.Contains(err.Error(), "team/gh/prs.yaml") {
		t.Errorf("error should name the offending file, got %v", err)
	}
	if !strings.Contains(err.Error(), "type") {
		t.Errorf("error should mention the missing type, got %v", err)
	}
}

func TestTypelessUnrelatedDocumentIsIgnored(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, "team"))
	write(t, filepath.Join(home, "team", "notes.yaml"), "just: notes\n")
	write(t, filepath.Join(home, "team", "prs.yaml"), "name: prs\ntype: query\nsignal: github\n")
	s, err := LoadDirectivesFromFiles(home)
	if err != nil {
		t.Fatalf("an unrelated document should not error: %v", err)
	}
	if len(s.Queries) != 1 || len(s.Flights) != 0 || len(s.Roles) != 0 {
		t.Fatalf("unrelated document leaked into directives: %+v", s)
	}
	if got := s.DocCount("team/notes.yaml"); got != 0 {
		t.Errorf("DocCount(team/notes.yaml) = %d, want 0", got)
	}
}

func TestSourceAndDocCountTrackTheOriginFile(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, "team", "gh"))
	write(t, filepath.Join(home, "team", "gh", "prs.yaml"), `
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
	write(t, filepath.Join(home, "oncall.yaml"), "name: oncall\ntype: role\nqueries: [prs]\n")
	s, err := LoadDirectivesFromFiles(home)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Source(TypeQuery, "prs"); got != "team/gh/prs.yaml" {
		t.Errorf("Source(query, prs) = %q, want team/gh/prs.yaml", got)
	}
	if got := s.Source(TypeFilter, "no-bots"); got != "team/gh/prs.yaml" {
		t.Errorf("Source(filter, no-bots) = %q, want team/gh/prs.yaml", got)
	}
	if got := s.Source(TypeRole, "oncall"); got != "oncall.yaml" {
		t.Errorf("Source(role, oncall) = %q, want oncall.yaml", got)
	}
	if got := s.Source(TypeFlight, "prs"); got != "" {
		t.Errorf("Source(flight, prs) = %q, want empty", got)
	}
	if got := s.DocCount("team/gh/prs.yaml"); got != 2 {
		t.Errorf("DocCount(team/gh/prs.yaml) = %d, want 2", got)
	}
	if got := s.DocCount("oncall.yaml"); got != 1 {
		t.Errorf("DocCount(oncall.yaml) = %d, want 1", got)
	}
	if got := s.DocCount("nope.yaml"); got != 0 {
		t.Errorf("DocCount(nope.yaml) = %d, want 0", got)
	}
}

func TestNamesCollideOnlyWithinAKind(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	mkdir(t, filepath.Join(home, DirFlights))
	write(t, filepath.Join(home, DirQueries, "demo.yaml"), "name: demo\ntype: query\nsignal: github\n")
	write(t, filepath.Join(home, DirFlights, "demo.yaml"), "name: demo\ntype: flight\nqueries: [demo]\n")
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
	mkdir(t, filepath.Join(clash, "team"))
	write(t, filepath.Join(clash, DirQueries, "a.yaml"), "name: dup\ntype: flight\nqueries: [x]\n")
	write(t, filepath.Join(clash, "team", "b.yaml"), "name: dup\ntype: flight\nqueries: [y]\n")
	if _, err := LoadDirectivesFromFiles(clash); err == nil {
		t.Fatal("two flights with the same name should collide across directories")
	}
}

func TestValidAndResolveDirectiveArgs(t *testing.T) {
	if got := ValidDirectives(); !reflect.DeepEqual(got, []string{ConfigDirective, DirectivesDirective, "all"}) {
		t.Fatalf("ValidDirectives = %v", got)
	}
	for _, name := range []string{ConfigDirective, DirectivesDirective, "all"} {
		got, err := ResolveDirectiveArg(name)
		if err != nil || got != name {
			t.Errorf("ResolveDirectiveArg(%q) = %q, %v", name, got, err)
		}
	}
	for _, legacy := range []string{DirQueries, DirFlights, KindRoles} {
		got, err := ResolveDirectiveArg(legacy)
		if err != nil {
			t.Errorf("ResolveDirectiveArg(%q): %v", legacy, err)
			continue
		}
		if got != DirectivesDirective {
			t.Errorf("ResolveDirectiveArg(%q) = %q, want %q", legacy, got, DirectivesDirective)
		}
	}
	for _, bad := range []string{"filters", "bogus", ""} {
		if _, err := ResolveDirectiveArg(bad); err == nil {
			t.Errorf("want error for %q", bad)
		}
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
type: filter
aliases:
  REPOS_ALIAS: "repo:org/a repo:org/b"
`)
	write(t, filepath.Join(home, DirQueries, "prs.yaml"), `
name: prs
type: query
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
type: query
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
type: query
title: Open pull requests
signal: github
`)
	write(t, filepath.Join(home, DirQueries, "standup.yaml"), `
name: standup
type: query
signal: slack
`)

	s, err := LoadDirectivesFromFiles(home)
	if err != nil {
		t.Fatal(err)
	}
	queries := s.Queries

	titled, ok := queries["my-open-prs"]
	if !ok {
		t.Fatalf("my-open-prs missing from %v", s.QueryNames())
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

func TestTypoedDirectiveFieldErrorsInsteadOfVanishing(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	write(t, filepath.Join(home, DirQueries, "prs.yaml"), "name: prs\nsinal: github.prs\n")

	s, err := LoadDirectivesFromFiles(home)
	if err == nil {
		t.Fatalf("a misspelled `signal:` loaded cleanly with queries=%v; the query simply does not exist", s.QueryNames())
	}
	if !strings.Contains(err.Error(), "prs.yaml") {
		t.Errorf("error should name the file, got %v", err)
	}
	if !strings.Contains(err.Error(), "sinal") {
		t.Errorf("error should name the unknown key, got %v", err)
	}
}

func TestTypoedParamsKeyErrorsInsteadOfRunningUnparameterised(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	write(t, filepath.Join(home, DirQueries, "prs.yaml"),
		"name: prs\ntype: query\nsignal: github.prs\nparmas:\n  repo: mino\n")

	s, err := LoadDirectivesFromFiles(home)
	if err == nil {
		t.Fatalf("a misspelled `params:` loaded cleanly; the query would run unparameterised: %#v", s.Queries["prs"])
	}
	if !strings.Contains(err.Error(), "parmas") {
		t.Errorf("error should name the unknown key, got %v", err)
	}
}

func TestNameOnlyDocumentErrorsInsteadOfBeingSkipped(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	write(t, filepath.Join(home, DirQueries, "prs.yaml"), "name: prs\n")

	if _, err := LoadDirectivesFromFiles(home); err == nil {
		t.Fatal("a document holding only a name loaded as nothing at all, with nothing reported")
	}
}

func TestBadFilterRegexErrorNamesTheFile(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	write(t, filepath.Join(home, DirQueries, "a.yaml"), "name: a\ntype: filter\nrules:\n  - include: \"[\"\n")

	_, err := LoadDirectivesFromFiles(home)
	if err == nil {
		t.Fatal("expected a compile error for a bad regex")
	}
	if !strings.Contains(err.Error(), "a.yaml") {
		t.Errorf("a filter compile error must name the file it came from, got %v", err)
	}
}

func TestBadFormatterTemplateErrorNamesTheFile(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirFormatters))
	write(t, filepath.Join(home, DirFormatters, "x.yaml"), "name: x\ntype: formatter\ntemplate: \"{{ .Items\"\n")

	_, err := LoadDirectivesFromFiles(home)
	if err == nil {
		t.Fatal("expected a parse error for a malformed template")
	}
	if !strings.Contains(err.Error(), "x.yaml") {
		t.Errorf("a formatter parse error must name the file it came from, got %v", err)
	}
}

func TestValidationErrorNamesTheFileAndKeepsItsHint(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	write(t, filepath.Join(home, DirQueries, "b.yaml"), "name: b\ntype: query\n")

	_, err := LoadDirectivesFromFiles(home)
	if err == nil {
		t.Fatal("expected an error for a query with no signal")
	}
	if !strings.Contains(err.Error(), "b.yaml") {
		t.Errorf("a validation error must name the file it came from, got %v", err)
	}
	if hint := errs.Hint(err); hint == "" {
		t.Error("wrapping the validation error dropped its hint")
	}
}

func TestDuplicateNameErrorNamesBothFiles(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	write(t, filepath.Join(home, DirQueries, "a-first.yaml"), "name: prs\ntype: query\nsignal: github\n")
	write(t, filepath.Join(home, DirQueries, "b-second.yaml"), "name: prs\ntype: query\nsignal: gitlab\n")

	_, err := LoadDirectivesFromFiles(home)
	if err == nil {
		t.Fatal("expected a duplicate-name error")
	}
	full := err.Error() + " " + errs.Hint(err)
	for _, want := range []string{"a-first.yaml", "b-second.yaml"} {
		if !strings.Contains(full, want) {
			t.Errorf("duplicate error should name %s, got %q", want, full)
		}
	}
}

func TestWhollyMisspelledDocumentInADirectiveLocationErrors(t *testing.T) {
	cases := []struct {
		name string
		dir  string
		rel  string
		body string
		keys []string
	}{
		{
			name: "query with every field misspelled",
			dir:  DirQueries,
			rel:  DirQueries + "/prs.yaml",
			body: "sinal: github.prs\nparmas:\n  repo: mino\n",
			keys: []string{"sinal"},
		},
		{
			name: "query with type misspelled too",
			dir:  DirQueries,
			rel:  DirQueries + "/prs.yaml",
			body: "tpye: query\nsinal: github.prs\n",
			keys: []string{"tpye"},
		},
		{
			name: "flight with every field misspelled",
			dir:  DirFlights,
			rel:  DirFlights + "/morning.yaml",
			body: "qeuries: [standup]\n",
			keys: []string{"qeuries"},
		},
		{
			name: "formatter with every field misspelled",
			dir:  DirFormatters,
			rel:  DirFormatters + "/short.yaml",
			body: "tmeplate: \"{{ .Items }}\"\n",
			keys: []string{"tmeplate"},
		},
		{
			name: "role at the home root with every field misspelled",
			dir:  "",
			rel:  "dev.yaml",
			body: "qeuries: [prs]\nfilghts: [a]\n",
			keys: []string{"qeuries"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			if tc.dir != "" {
				mkdir(t, filepath.Join(home, tc.dir))
			}
			write(t, filepath.Join(home, filepath.FromSlash(tc.rel)), tc.body)

			s, err := LoadDirectivesFromFiles(home)
			if err == nil {
				t.Fatalf("%s loaded cleanly and vanished: queries=%v flights=%v roles=%v formatters=%v",
					tc.rel, s.QueryNames(), s.FlightNames(), s.RoleNames(), s.FormatterNames())
			}
			full := err.Error() + " " + errs.Hint(err)
			if !strings.Contains(full, tc.rel) && !strings.Contains(full, filepath.Base(tc.rel)) {
				t.Errorf("error should name %s, got %q", tc.rel, full)
			}
			for _, k := range tc.keys {
				if !strings.Contains(full, k) {
					t.Errorf("error should name the unknown key %q, got %q", k, full)
				}
			}
		})
	}
}

func TestUnrelatedDocumentOutsideDirectiveLocationsStillLoads(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, "team"))
	mkdir(t, filepath.Join(home, DirQueries))
	write(t, filepath.Join(home, "team", "notes.yaml"), "just: notes\ntopics:\n  - one\n")
	write(t, filepath.Join(home, "team", "notes.json"), "{\"just\":\"notes\"}\n")
	write(t, filepath.Join(home, DirQueries, "prs.yaml"), "name: prs\ntype: query\nsignal: github\n")

	s, err := LoadDirectivesFromFiles(home)
	if err != nil {
		t.Fatalf("an unrelated file outside the directive directories must still load: %v", err)
	}
	if len(s.Queries) != 1 {
		t.Fatalf("unrelated documents leaked into directives: %v", s.QueryNames())
	}
	if got := s.DocCount("team/notes.yaml"); got != 0 {
		t.Errorf("DocCount(team/notes.yaml) = %d, want 0", got)
	}
}

func TestJSONDirectiveRejectsUnknownFields(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	write(t, filepath.Join(home, DirQueries, "prs.json"),
		"{\"name\":\"prs\",\"type\":\"query\",\"signal\":\"github.prs\",\"parmas\":{\"repo\":\"mino\"}}\n")

	s, err := LoadDirectivesFromFiles(home)
	if err == nil {
		t.Fatalf("a misspelled `params` in JSON loaded cleanly; the query would run unparameterised: %#v", s.Queries["prs"])
	}
	full := err.Error() + " " + errs.Hint(err)
	for _, want := range []string{"prs.json", "parmas"} {
		if !strings.Contains(full, want) {
			t.Errorf("error should mention %q, got %q", want, full)
		}
	}
	if !strings.Contains(err.Error(), "unknown field") {
		t.Errorf("JSON should report unknown fields the way YAML does, got %q", err.Error())
	}
}

func TestJSONDirectiveRejectsUnknownFieldsThroughTheStoreBlob(t *testing.T) {
	blob := []byte(`{"queries/prs.json":"{\"name\":\"prs\",\"type\":\"query\",\"signal\":\"github.prs\",\"parmas\":{\"repo\":\"mino\"}}"}`)
	if s, err := NewDirectives(blob); err == nil {
		t.Fatalf("the stored blob accepted a misspelled key: %#v", s.Queries["prs"])
	}
}

func TestJSONDirectiveArrayRejectsUnknownFields(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	write(t, filepath.Join(home, DirQueries, "list.json"),
		"[{\"name\":\"a\",\"type\":\"query\",\"signal\":\"x\"},{\"name\":\"b\",\"type\":\"query\",\"signal\":\"y\",\"parmas\":{}}]\n")

	if _, err := LoadDirectivesFromFiles(home); err == nil {
		t.Fatal("a misspelled key inside a JSON array loaded cleanly")
	}
}

func TestDocumentIndexIsTheRealDocumentNumber(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	write(t, filepath.Join(home, DirQueries, "x.yaml"), "null\n---\nname: b\ntype: query\n")

	_, err := LoadDirectivesFromFiles(home)
	if err == nil {
		t.Fatal("expected an error for a query with no signal")
	}
	if !strings.Contains(err.Error(), "document 2") {
		t.Errorf("the bad document is the second one, got %q", err.Error())
	}
}

func TestSequenceItemIsReportedAsAnItemOfItsDocument(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	write(t, filepath.Join(home, DirQueries, "y.yaml"),
		"- {name: a, type: query, signal: x}\n- {name: b, type: query, signal: y}\n- {name: c, type: query}\n")

	_, err := LoadDirectivesFromFiles(home)
	if err == nil {
		t.Fatal("expected an error for a query with no signal")
	}
	got := err.Error()
	if !strings.Contains(got, "item 3 of document 1") {
		t.Errorf("a 3-item sequence in one document should be reported as item 3 of document 1, got %q", got)
	}
	if strings.Contains(got, "document 3") {
		t.Errorf("there is only one document in the file, got %q", got)
	}
}

func TestUnknownFieldHintNamesTheFileAndTheKey(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	write(t, filepath.Join(home, DirQueries, "prs.yaml"),
		"name: prs\ntype: query\nsignal: github\ndescription: my prs\n")

	_, err := LoadDirectivesFromFiles(home)
	if err == nil {
		t.Fatal("an extra key must not be tolerated silently")
	}
	hint := errs.Hint(err)
	for _, want := range []string{"queries/prs.yaml", "description"} {
		if !strings.Contains(hint, want) {
			t.Errorf("hint should name %q so the user can act on it, got %q", want, hint)
		}
	}
	if !strings.Contains(hint, "delete") && !strings.Contains(hint, "remove") {
		t.Errorf("hint should say how to get rid of the key, got %q", hint)
	}
}
