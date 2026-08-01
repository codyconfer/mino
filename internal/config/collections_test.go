package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/filter"
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
	for _, rel := range []string{"../escape.yaml", "queries/plain.txt", ".plugins/vendored.yaml"} {
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

func symlink(t *testing.T, target, link string) {
	t.Helper()
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}
}

func assertNotSymlink(t *testing.T, path string) {
	t.Helper()
	fi, err := os.Lstat(path)
	if err != nil {
		t.Fatal(err)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		t.Fatalf("%s is still a symlink; a later write would escape the mino home again", path)
	}
}

func TestWriteDirectivesReplacesASymlinkInsteadOfWritingThroughIt(t *testing.T) {
	home := t.TempDir()
	victim := filepath.Join(t.TempDir(), "victim.yaml")
	write(t, victim, "victim: untouched\n")
	mkdir(t, filepath.Join(home, DirQueries))
	link := filepath.Join(home, DirQueries, "team.yaml")
	symlink(t, victim, link)

	payload := "name: team\ntype: query\nsignal: github\n"
	blob, err := json.Marshal(map[string]string{DirQueries + "/team.yaml": payload})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WriteDirectives(home, blob); err != nil {
		t.Fatal(err)
	}

	if raw, err := os.ReadFile(victim); err != nil || string(raw) != "victim: untouched\n" {
		t.Fatalf("a file outside the mino home was overwritten through a symlink: %q (err=%v)", raw, err)
	}
	assertNotSymlink(t, link)
	if raw, err := os.ReadFile(link); err != nil || string(raw) != payload {
		t.Fatalf("%s = %q (err=%v), want the directive payload", link, raw, err)
	}
}

func TestWriteDirectivesRollsBackWhenALaterFileCannotBeWritten(t *testing.T) {
	home := t.TempDir()
	mkdir(t, filepath.Join(home, DirQueries))
	original := "name: a\ntype: query\nsignal: github\n"
	write(t, filepath.Join(home, DirQueries, "a.yaml"), original)
	write(t, filepath.Join(home, "zblock"), "a file, not a directory\n")

	blob, err := json.Marshal(map[string]string{
		DirQueries + "/a.yaml": "name: a\ntype: query\nsignal: gitlab\n",
		"zblock/b.yaml":        "name: b\ntype: query\nsignal: github\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WriteDirectives(home, blob); err == nil {
		t.Fatal("WriteDirectives reported success even though zblock/b.yaml could not be written")
	}

	raw, err := os.ReadFile(filepath.Join(home, DirQueries, "a.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != original {
		t.Fatalf("queries/a.yaml = %q, want the pre-failure content %q: a failed export half-migrated the home", raw, original)
	}
	if raw, err := os.ReadFile(filepath.Join(home, "zblock")); err != nil || !strings.Contains(string(raw), "not a directory") {
		t.Fatalf("zblock = %q (err=%v), want it left alone", raw, err)
	}
}

func TestWriteDirectivesRollbackRemovesFilesItCreated(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, "zblock"), "a file, not a directory\n")
	blob, err := json.Marshal(map[string]string{
		DirQueries + "/fresh.yaml": "name: fresh\ntype: query\nsignal: github\n",
		"zblock/b.yaml":            "name: b\ntype: query\nsignal: github\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WriteDirectives(home, blob); err == nil {
		t.Fatal("WriteDirectives reported success on an unwritable target")
	}
	if _, err := os.Stat(filepath.Join(home, DirQueries, "fresh.yaml")); err == nil {
		t.Fatal("queries/fresh.yaml survived a failed WriteDirectives; the home is half-migrated")
	}
}

func TestSaveDirectiveRefusesASymlinkedDerivedTarget(t *testing.T) {
	home := t.TempDir()
	victim := filepath.Join(t.TempDir(), "victim.yaml")
	original := "name: team\ntype: query\nsignal: github\n"
	write(t, victim, original)
	mkdir(t, filepath.Join(home, DirQueries))
	symlink(t, victim, filepath.Join(home, DirQueries, "team.yaml"))

	_, _, err := SaveDirective(nil, home, "", TypeQuery, "team", Query{Name: "team", Signal: "gitlab"})
	if err == nil {
		t.Fatal("SaveDirective wrote through a symlink out of the mino home")
	}
	if !strings.Contains(err.Error(), "symlink") {
		t.Errorf("error should say the target is a symlink, got %v", err)
	}
	if raw, rerr := os.ReadFile(victim); rerr != nil || string(raw) != original {
		t.Fatalf("the file outside the home changed: %q (err=%v)", raw, rerr)
	}
}

func TestSaveDirectiveWithExplicitPathReplacesASymlink(t *testing.T) {
	home := t.TempDir()
	victim := filepath.Join(t.TempDir(), "victim.yaml")
	write(t, victim, "victim: untouched\n")
	mkdir(t, filepath.Join(home, DirQueries))
	link := filepath.Join(home, DirQueries, "team.yaml")
	symlink(t, victim, link)

	if _, _, err := SaveDirective(nil, home, DirQueries+"/team.yaml", TypeQuery, "team", Query{Name: "team", Signal: "github"}); err != nil {
		t.Fatal(err)
	}
	if raw, err := os.ReadFile(victim); err != nil || string(raw) != "victim: untouched\n" {
		t.Fatalf("a file outside the mino home was overwritten through a symlink: %q (err=%v)", raw, err)
	}
	assertNotSymlink(t, link)
}

func TestReservedRootIgnoresCase(t *testing.T) {
	reserved := []string{
		"config.yaml", "Config.yaml", "CONFIG.YAML", "config.YML", "Config.Json",
	}
	for _, name := range reserved {
		if !reservedRoot(name) {
			t.Errorf("reservedRoot(%q) = false; on a case-insensitive filesystem that file is the live config and would be swept up as a directive", name)
		}
	}
	for _, name := range []string{"team/Config.yaml", "configs.yaml", "myconfig.yaml", "config.txt"} {
		if reservedRoot(name) {
			t.Errorf("reservedRoot(%q) = true, want false", name)
		}
	}
}

func TestDirectiveFilesSkipsAMiscasedRootConfig(t *testing.T) {
	home := t.TempDir()
	logs := captureLog(t)
	write(t, filepath.Join(home, "Config.yaml"), "google:\n  oauth_client_secret: SUPERSECRET\n")
	write(t, filepath.Join(home, "dev.yaml"), "name: dev\ntype: role\n")

	got, err := DirectiveFiles(home)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, []string{"dev.yaml"}) {
		t.Fatalf("DirectiveFiles = %v, want only dev.yaml: Config.yaml is a config file, not a directive", got)
	}

	blob, has, err := SerializeDirectives(home)
	if err != nil || !has {
		t.Fatalf("SerializeDirectives: has=%v err=%v", has, err)
	}
	if strings.Contains(string(blob), "SUPERSECRET") {
		t.Fatalf("an unredacted config secret leaked into the directives blob: %s", blob)
	}
	if out := logs.String(); !strings.Contains(out, "Config.yaml") {
		t.Errorf("a root file that differs only in case from config.yaml was ignored with no diagnostic, got %q", out)
	}
}

func TestWriteDirectivesSkipsAReservedRootConfigKey(t *testing.T) {
	for _, rel := range []string{"config.yaml", "Config.yaml", "CONFIG.YML"} {
		t.Run(rel, func(t *testing.T) {
			home := t.TempDir()
			logs := captureLog(t)
			write(t, filepath.Join(home, "config.yaml"), "output: terminal\n")
			blob, err := json.Marshal(map[string]string{
				rel:                      "google:\n  oauth_client_secret: SUPERSECRET\n",
				DirQueries + "/prs.yaml": "name: prs\ntype: query\nsignal: github\n",
			})
			if err != nil {
				t.Fatal(err)
			}
			written, err := WriteDirectives(home, blob)
			if err != nil {
				t.Fatalf("one unusable row must not brick the other rows: %v", err)
			}
			if slices.Contains(written, rel) {
				t.Errorf("WriteDirectives claims it wrote %q: %v", rel, written)
			}
			if !slices.Contains(written, DirQueries+"/prs.yaml") {
				t.Errorf("the usable row was dropped along with the bad one: %v", written)
			}
			if raw, _ := os.ReadFile(filepath.Join(home, "config.yaml")); string(raw) != "output: terminal\n" {
				t.Fatalf("WriteDirectives materialized a config file out of a directive row: %q", raw)
			}
			for _, name := range []string{rel, "config.yaml"} {
				if raw, err := os.ReadFile(filepath.Join(home, name)); err == nil && strings.Contains(string(raw), "SUPERSECRET") {
					t.Fatalf("the directive row's content landed in %s: %q", name, raw)
				}
			}
			if out := logs.String(); !strings.Contains(out, rel) || !strings.Contains(out, "mino import directives") {
				t.Errorf("the skipped row must be reported with a way to remove it from the store, got %q", out)
			}
		})
	}
}

func TestWriteDirectivesRollbackRestoresASymlinkItReplaced(t *testing.T) {
	home := t.TempDir()
	outside := filepath.Join(t.TempDir(), "victim.yaml")
	write(t, outside, "victim: untouched\n")
	mkdir(t, filepath.Join(home, DirQueries))
	link := filepath.Join(home, DirQueries, "team.yaml")
	symlink(t, outside, link)
	write(t, filepath.Join(home, "zblock"), "a file, not a directory\n")

	blob, err := json.Marshal(map[string]string{
		DirQueries + "/team.yaml": "name: team\ntype: query\nsignal: github\n",
		"zblock/b.yaml":           "name: b\ntype: query\nsignal: github\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WriteDirectives(home, blob); err == nil {
		t.Fatal("WriteDirectives reported success even though zblock/b.yaml could not be written")
	}

	fi, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("queries/team.yaml is gone after a rollback that claims nothing was applied: %v", err)
	}
	if fi.Mode()&os.ModeSymlink == 0 {
		body, _ := os.ReadFile(link)
		t.Fatalf("rollback destroyed the user's symlink and left a regular file holding %q", body)
	}
	dest, err := os.Readlink(link)
	if err != nil {
		t.Fatal(err)
	}
	if dest != outside {
		t.Errorf("restored link points at %q, want %q", dest, outside)
	}
	if raw, err := os.ReadFile(outside); err != nil || string(raw) != "victim: untouched\n" {
		t.Fatalf("the file outside the home was changed: %q (err=%v)", raw, err)
	}
}

func TestWriteDirectivesRollbackRemovesDirectoriesItCreated(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, "zblock"), "a file, not a directory\n")
	blob, err := json.Marshal(map[string]string{
		"deep/nest/fresh.yaml": "name: fresh\ntype: query\nsignal: github\n",
		"zblock/b.yaml":        "name: b\ntype: query\nsignal: github\n",
	})
	if err != nil {
		t.Fatal(err)
	}
	werr := WriteDirectivesErr(t, home, blob)
	for _, leftover := range []string{filepath.Join(home, "deep", "nest"), filepath.Join(home, "deep")} {
		if _, err := os.Stat(leftover); err == nil {
			t.Errorf("%s survived a rollback that claims %q", leftover, errs.Hint(werr))
		}
	}
}

func WriteDirectivesErr(t *testing.T, home string, blob []byte) error {
	t.Helper()
	_, err := WriteDirectives(home, blob)
	if err == nil {
		t.Fatal("WriteDirectives reported success on an unwritable target")
	}
	return err
}

func TestRollbackSaysWhatIsStuckWhenItCannotUndo(t *testing.T) {
	home := t.TempDir()
	blocked := filepath.Join(home, "blocker")
	write(t, blocked, "a file, not a directory\n")
	stuck := undoStep{target: filepath.Join(blocked, "a.yaml"), prior: priorFile, body: []byte("name: a\n")}
	fine := undoStep{target: filepath.Join(home, "gone.yaml")}

	err := rollbackDirectives([]undoStep{fine, stuck}, errs.New(errs.KindConfig, "writing z.yaml failed"))
	hint := errs.Hint(err)
	if strings.Contains(hint, "nothing was applied") {
		t.Fatalf("rollback failed but still claims nothing was applied: %q", hint)
	}
	if !strings.Contains(hint, stuck.target) {
		t.Errorf("the hint must name what is still changed on disk, got %q", hint)
	}
	if !strings.Contains(hint, "1 of the 2") {
		t.Errorf("the hint must say how much is stuck, got %q", hint)
	}
}

func TestRollbackClaimsNothingWasAppliedOnlyWhenItSucceeded(t *testing.T) {
	home := t.TempDir()
	target := filepath.Join(home, "a.yaml")
	write(t, target, "name: a\n")
	err := rollbackDirectives([]undoStep{{target: target, prior: priorFile, body: []byte("name: original\n")}},
		errs.New(errs.KindConfig, "writing z.yaml failed"))
	if hint := errs.Hint(err); !strings.Contains(hint, "nothing was applied") {
		t.Errorf("a clean rollback should say so, got %q", hint)
	}
	if raw, rerr := os.ReadFile(target); rerr != nil || string(raw) != "name: original\n" {
		t.Errorf("%s = %q (err=%v), want the captured content restored", target, raw, rerr)
	}
}
