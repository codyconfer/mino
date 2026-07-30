package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/codyconfer/munin/internal/filter"
)

func seedNestedHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries, "gh"))
	mkdir(t, filepath.Join(home, "team"))
	mkdir(t, DataDir(home))
	mkdir(t, PluginsDir(home))
	mkdir(t, filepath.Join(home, ".archive"))
	mkdir(t, filepath.Join(home, DirLogs))

	write(t, filepath.Join(home, "config.yaml"), "output: terminal\n")
	write(t, filepath.Join(home, "dev.yaml"), "name: dev\ntype: role\nflights: [morning]\n")
	write(t, filepath.Join(home, "notes.txt"), "not a directive\n")
	write(t, filepath.Join(home, DirQueries, "gh", "prs.yaml"), "name: prs\ntype: query\nsignal: github\n")
	write(t, filepath.Join(home, "team", "oncall.yaml"), "name: oncall\ntype: role\nqueries: [prs]\n")
	write(t, filepath.Join(home, "team", "config.yaml"), "name: morning\ntype: flight\nqueries: [prs]\n")
	write(t, filepath.Join(DataDir(home), ConfigDB), "binary")
	write(t, filepath.Join(PluginsDir(home), "vendored.yaml"), "name: vendored\ntype: query\nsignal: github\n")
	write(t, filepath.Join(home, ".archive", "old.yaml"), "name: old\ntype: query\nsignal: github\n")
	write(t, filepath.Join(home, DirLogs, "run.yaml"), "name: run\ntype: query\nsignal: github\n")
	return home
}

func nestedRels() []string {
	return []string{"dev.yaml", "queries/gh/prs.yaml", "team/config.yaml", "team/oncall.yaml"}
}

func TestDirectiveFilesWalksAnyDepth(t *testing.T) {
	got, err := DirectiveFiles(seedNestedHome(t))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, nestedRels()) {
		t.Fatalf("DirectiveFiles = %v, want %v", got, nestedRels())
	}
}

func TestDirectiveFilesLoadsEveryNestedDocument(t *testing.T) {
	s, err := LoadDirectivesFromFiles(seedNestedHome(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := s.Queries["prs"]; !ok {
		t.Errorf("queries/gh/prs.yaml not loaded: %v", s.QueryNames())
	}
	if _, ok := s.Roles["oncall"]; !ok {
		t.Errorf("team/oncall.yaml not loaded: %v", s.RoleNames())
	}
	if _, ok := s.Roles["dev"]; !ok {
		t.Errorf("dev.yaml not loaded: %v", s.RoleNames())
	}
	if _, ok := s.Flights["morning"]; !ok {
		t.Errorf("team/config.yaml is not reserved and should load: %v", s.FlightNames())
	}
	for _, name := range []string{"vendored", "old", "run"} {
		if _, ok := s.Queries[name]; ok {
			t.Errorf("%q came from a reserved directory: %v", name, s.QueryNames())
		}
	}
}

func TestDirectiveFilesReservesEveryRootConfigName(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, "team"))
	for _, name := range []string{"config.yaml", "config.yml", "config.json"} {
		write(t, filepath.Join(home, name), "output: terminal\n")
	}
	write(t, filepath.Join(home, "team", "config.yaml"), "name: nested\ntype: flight\nqueries: [a]\n")
	got, err := DirectiveFiles(home)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"team/config.yaml"}) {
		t.Fatalf("DirectiveFiles = %v, want only the nested config.yaml", got)
	}
}

func TestSerializeDirectivesEmptyHome(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, "config.yaml"), "output: terminal\n")
	blob, has, err := SerializeDirectives(home)
	if err != nil {
		t.Fatal(err)
	}
	if has || blob != nil {
		t.Fatalf("want no directives, got has=%v blob=%s", has, blob)
	}
}

func TestSerializeDirectivesKeysByRelPath(t *testing.T) {
	blob, has, err := SerializeDirectives(seedNestedHome(t))
	if err != nil || !has {
		t.Fatalf("SerializeDirectives: has=%v err=%v", has, err)
	}
	var files map[string]string
	if err := json.Unmarshal(blob, &files); err != nil {
		t.Fatal(err)
	}
	if len(files) != len(nestedRels()) {
		t.Fatalf("serialized files = %#v, want %v", files, nestedRels())
	}
	for _, rel := range nestedRels() {
		if _, ok := files[rel]; !ok {
			t.Errorf("missing %q from %#v", rel, files)
		}
	}
}

func TestWriteDirectivesRoundTripsNesting(t *testing.T) {
	home := seedNestedHome(t)
	blob, _, err := SerializeDirectives(home)
	if err != nil {
		t.Fatal(err)
	}

	fresh := t.TempDir()
	written, err := WriteDirectives(fresh, blob)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(written, nestedRels()) {
		t.Fatalf("WriteDirectives = %v, want %v", written, nestedRels())
	}
	found, err := DirectiveFiles(fresh)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(found, nestedRels()) {
		t.Fatalf("fresh home holds %v, want %v", found, nestedRels())
	}
	for _, rel := range nestedRels() {
		want, err := os.ReadFile(filepath.Join(home, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		got, err := os.ReadFile(filepath.Join(fresh, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		if string(got) != string(want) {
			t.Errorf("%s = %q, want %q", rel, got, want)
		}
	}
}

func TestWriteDirectivesRejectsBadKeys(t *testing.T) {
	for _, rel := range []string{"../escape.yaml", "config.yaml", "queries/plain.txt", ".plugins/vendored.yaml"} {
		t.Run(rel, func(t *testing.T) {
			home := t.TempDir()
			write(t, filepath.Join(home, "config.yaml"), "output: terminal\n")
			blob, err := json.Marshal(map[string]string{rel: "name: oops\ntype: role\n"})
			if err != nil {
				t.Fatal(err)
			}
			if _, err := WriteDirectives(home, blob); err == nil {
				t.Fatalf("want error writing %q", rel)
			}
			if raw, _ := os.ReadFile(filepath.Join(home, "config.yaml")); string(raw) != "output: terminal\n" {
				t.Fatalf("config.yaml clobbered: %q", raw)
			}
			if rels, err := DirectiveFiles(home); err != nil || len(rels) != 0 {
				t.Fatalf("rejected blob still wrote %v (err=%v)", rels, err)
			}
		})
	}
}

func TestWriteDirectivesRejectsWholeBlobOnOneBadKey(t *testing.T) {
	home := t.TempDir()
	blob, err := json.Marshal(map[string]string{
		"team/oncall.yaml": "name: oncall\ntype: role\n",
		"../escape.yaml":   "name: oops\ntype: role\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WriteDirectives(home, blob); err == nil {
		t.Fatal("want error for a blob carrying an escaping key")
	}
	if rels, err := DirectiveFiles(home); err != nil || len(rels) != 0 {
		t.Fatalf("partial write left %v (err=%v)", rels, err)
	}
}

func TestClearDirectivesKeepsEverythingElse(t *testing.T) {
	home := seedNestedHome(t)
	removed, err := ClearDirectives(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != len(nestedRels()) {
		t.Fatalf("removed = %v, want %d files", removed, len(nestedRels()))
	}
	for _, keep := range []string{"config.yaml", "notes.txt"} {
		if _, err := os.Stat(filepath.Join(home, keep)); err != nil {
			t.Errorf("%s removed: %v", keep, err)
		}
	}
	if _, err := os.Stat(filepath.Join(PluginsDir(home), "vendored.yaml")); err != nil {
		t.Errorf("%s content removed: %v", DirPlugins, err)
	}
	if _, err := os.Stat(filepath.Join(DataDir(home), ConfigDB)); err != nil {
		t.Errorf("%s removed: %v", DirData, err)
	}
	if rels, err := DirectiveFiles(home); err != nil || len(rels) != 0 {
		t.Fatalf("DirectiveFiles after clear = %v (err=%v)", rels, err)
	}
}

func TestRemoveDirectiveTakesRelPath(t *testing.T) {
	home := seedNestedHome(t)
	removed, err := RemoveDirective(home, "queries/gh/prs.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 || removed[0] != filepath.Join(home, DirQueries, "gh", "prs.yaml") {
		t.Fatalf("removed = %v", removed)
	}
	if _, err := os.Stat(removed[0]); !os.IsNotExist(err) {
		t.Fatalf("file still there: %v", err)
	}
}

func TestRemoveDirectiveRejectsConfigAndEscapes(t *testing.T) {
	home := seedNestedHome(t)
	for _, rel := range []string{"config.yaml", "../escape.yaml"} {
		if _, err := RemoveDirective(home, rel); err == nil {
			t.Errorf("want error removing %q", rel)
		}
	}
	if _, err := os.Stat(filepath.Join(home, "config.yaml")); err != nil {
		t.Fatalf("config.yaml removed: %v", err)
	}
}

func TestRemoveDirectiveMissingFileIsNoop(t *testing.T) {
	removed, err := RemoveDirective(t.TempDir(), "team/ghost.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if removed != nil {
		t.Fatalf("removed = %v, want nil", removed)
	}
}

func TestDefaultDirectivePathByKind(t *testing.T) {
	cases := map[DirectiveType]string{
		TypeQuery:  DirQueries + "/prs.yaml",
		TypeFilter: DirQueries + "/prs.yaml",
		TypeFlight: DirFlights + "/prs.yaml",
		TypeRole:   "prs.yaml",
		TypeAuto:   DirQueries + "/prs.yaml",
	}
	for kind, want := range cases {
		if got := DefaultDirectivePath(kind, "prs"); got != want {
			t.Errorf("DefaultDirectivePath(%q) = %q, want %q", kind, got, want)
		}
	}
}

const twoQueryDocs = "name: alpha\ntype: query\nsignal: github\n---\nname: beta\ntype: query\nsignal: github\n"

func TestSaveDirectiveRefusesToClobberAMultiDocFile(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	target := filepath.Join(home, DirQueries, "team.yaml")
	write(t, target, twoQueryDocs)

	_, _, err := SaveDirective(nil, home, "", TypeQuery, "team", Query{Name: "team", Signal: "github"})
	if err == nil {
		t.Fatal("SaveDirective overwrote a multi-document file")
	}
	if !strings.Contains(err.Error(), DirQueries+"/team.yaml") {
		t.Errorf("error must name the file: %v", err)
	}
	raw, readErr := os.ReadFile(target)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(raw) != twoQueryDocs {
		t.Fatalf("%s rewritten:\n%s", target, raw)
	}
	s, err := LoadDirectivesFromFiles(home)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"alpha", "beta"} {
		if _, ok := s.Queries[name]; !ok {
			t.Errorf("%q lost: %v", name, s.QueryNames())
		}
	}
	if _, ok := s.Queries["team"]; ok {
		t.Error("refused save still landed in the collection")
	}
}

func TestSaveDirectiveRefusesToClobberADifferentDirective(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	target := filepath.Join(home, DirQueries, "team.yaml")
	body := "name: alpha\ntype: query\nsignal: github\n"
	write(t, target, body)

	if _, _, err := SaveDirective(nil, home, "", TypeQuery, "team", Query{Name: "team", Signal: "github"}); err == nil {
		t.Fatal("SaveDirective overwrote a file holding another directive")
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != body {
		t.Fatalf("%s rewritten:\n%s", target, raw)
	}
}

func TestSaveDirectiveRefusesToClobberAMultiDocRoleFile(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, "team.yaml")
	body := "name: dev\ntype: role\nqueries: [prs]\n---\nname: ops\ntype: role\nqueries: [prs]\n"
	write(t, target, body)

	if _, _, err := SaveDirective(nil, home, "", TypeRole, "team", RoleDef{Name: "team"}); err == nil {
		t.Fatal("SaveDirective overwrote a multi-document role file")
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != body {
		t.Fatalf("%s rewritten:\n%s", target, raw)
	}
}

func TestSaveDirectiveOverwritesItsOwnFile(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	target := filepath.Join(home, DirQueries, "team.yaml")
	write(t, target, "name: team\ntype: query\nsignal: github\n")

	path, _, err := SaveDirective(nil, home, "", TypeQuery, "team", Query{
		Name:   "team",
		Signal: "github",
		Params: map[string]string{"query": "is:open"},
	})
	if err != nil {
		t.Fatalf("editing an existing single-directive file must work: %v", err)
	}
	if path != target {
		t.Fatalf("path = %q, want %q", path, target)
	}
	s, err := LoadDirectivesFromFiles(home)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Queries["team"]; got.Params["query"] != "is:open" {
		t.Fatalf("edit not written: %#v", got)
	}
}

func TestSaveDirectiveOverwritesANamelessFileMatchingItsBase(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	target := filepath.Join(home, DirQueries, "team.yaml")
	write(t, target, "type: query\nsignal: github\n")

	if _, _, err := SaveDirective(nil, home, "", TypeQuery, "team", Query{Name: "team", Signal: "gitlab"}); err != nil {
		t.Fatalf("nameless single-directive file takes its base name: %v", err)
	}
	s, err := LoadDirectivesFromFiles(home)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Queries["team"]; got.Signal != "gitlab" {
		t.Fatalf("edit not written: %#v", got)
	}
}

func TestSaveDirectiveTurnsAQueryIntoAFilterInPlace(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	write(t, filepath.Join(home, DirQueries, "team.yaml"), "name: team\ntype: query\nsignal: github\n")

	f := Query{Name: "team", Rules: []filter.Rule{{Exclude: "bot$"}}}
	if _, _, err := SaveDirective(nil, home, "", TypeFilter, "team", f); err != nil {
		t.Fatalf("query -> filter in its own file must work: %v", err)
	}
	s, err := LoadDirectivesFromFiles(home)
	if err != nil {
		t.Fatal(err)
	}
	if got := s.Queries["team"]; got.Kind() != TypeFilter {
		t.Fatalf("stored kind = %q: %#v", got.Kind(), got)
	}
}

func TestSaveDirectiveRefusesAnUnparseableTarget(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	target := filepath.Join(home, DirQueries, "team.yaml")
	body := "name: team\n\ttype: query\n"
	write(t, target, body)

	if _, _, err := SaveDirective(nil, home, "", TypeQuery, "team", Query{Name: "team", Signal: "github"}); err == nil {
		t.Fatal("SaveDirective overwrote a file it could not read")
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != body {
		t.Fatalf("%s rewritten:\n%s", target, raw)
	}
}

func TestSaveDirectiveExplicitRelStillOverwrites(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	target := filepath.Join(home, DirQueries, "team.yaml")
	write(t, target, twoQueryDocs)

	if _, _, err := SaveDirective(nil, home, DirQueries+"/team.yaml", TypeQuery, "team", Query{Name: "team", Signal: "github"}); err != nil {
		t.Fatalf("an explicit path is the escape hatch and must still write: %v", err)
	}
	raw, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "alpha") {
		t.Fatalf("explicit save did not rewrite the file:\n%s", raw)
	}
}

func TestDataPathsUnderDotData(t *testing.T) {
	home := t.TempDir()
	got := DataPath(home, ConfigDB)
	want := filepath.Join(home, DirData, ConfigDB)
	if got != want {
		t.Fatalf("DataPath = %q, want %q", got, want)
	}
	if fi, err := os.Stat(DataDir(home)); err != nil || !fi.IsDir() {
		t.Fatalf("DataPath must create %s: %v", DataDir(home), err)
	}
	rels, err := DirectiveFiles(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(rels) != 0 {
		t.Fatalf("%s must not read as directives: %v", DirData, rels)
	}
}
