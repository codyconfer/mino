package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
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
