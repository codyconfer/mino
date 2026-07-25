package config

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestCollectionDirPutsRolesAtHome(t *testing.T) {
	home := t.TempDir()
	if got := CollectionDir(home, KindRoles); got != home {
		t.Fatalf("CollectionDir roles = %q, want %q", got, home)
	}
	if got, want := CollectionDir(home, DirQueries), filepath.Join(home, DirQueries); got != want {
		t.Fatalf("CollectionDir queries = %q, want %q", got, want)
	}
}

func TestSerializeRolesSkipsConfigAndDirs(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, "config.yaml"), "output: terminal\n")
	write(t, filepath.Join(home, "triage.yaml"), "name: triage\nflights: [morning]\n")
	write(t, filepath.Join(home, "ops.json"), `{"name":"ops"}`)
	write(t, filepath.Join(home, "notes.txt"), "not a role")
	mkdir(t, filepath.Join(home, DirQueries))
	write(t, filepath.Join(home, DirQueries, "standup.yaml"), "name: standup\nsignal: slack\n")
	mkdir(t, DataDir(home))
	write(t, filepath.Join(DataDir(home), "config.duckdb"), "binary")

	blob, has, err := SerializeCollection(home, KindRoles)
	if err != nil || !has {
		t.Fatalf("SerializeCollection roles: has=%v err=%v", has, err)
	}
	var files map[string]string
	if err := json.Unmarshal(blob, &files); err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("role files = %#v, want triage.yaml + ops.json only", files)
	}
	for _, skip := range []string{"config.yaml", "notes.txt", "standup.yaml", "config.duckdb"} {
		if _, ok := files[skip]; ok {
			t.Errorf("%s must not be serialized as a role", skip)
		}
	}

	roles, err := ParseRoles(blob)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := roles["triage"]; !ok {
		t.Fatalf("roles = %#v", roles)
	}
}

func TestSerializeRolesEmptyHome(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, "config.yaml"), "output: terminal\n")
	blob, has, err := SerializeCollection(home, KindRoles)
	if err != nil {
		t.Fatal(err)
	}
	if has || blob != nil {
		t.Fatalf("want no roles, got has=%v blob=%s", has, blob)
	}
}

func TestWriteCollectionRolesRejectsConfigName(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, "config.yaml"), "output: terminal\n")
	blob, err := json.Marshal(map[string]string{"config.yaml": "name: oops\n"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := WriteCollection(home, KindRoles, blob); err == nil {
		t.Fatal("want error writing a role named config.yaml")
	}
	if raw, _ := os.ReadFile(filepath.Join(home, "config.yaml")); string(raw) != "output: terminal\n" {
		t.Fatalf("config.yaml clobbered: %q", raw)
	}
}

func TestWriteCollectionRolesRoundTrip(t *testing.T) {
	home := t.TempDir()
	blob, err := json.Marshal(map[string]string{"triage.yaml": "name: triage\n"})
	if err != nil {
		t.Fatal(err)
	}
	names, err := WriteCollection(home, KindRoles, blob)
	if err != nil || len(names) != 1 {
		t.Fatalf("WriteCollection roles: names=%v err=%v", names, err)
	}
	if _, err := os.Stat(filepath.Join(home, "triage.yaml")); err != nil {
		t.Fatalf("role not written at home root: %v", err)
	}
}

func TestClearCollectionRolesKeepsConfig(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, "config.yaml"), "output: terminal\n")
	write(t, filepath.Join(home, "triage.yaml"), "name: triage\n")
	mkdir(t, filepath.Join(home, DirQueries))

	removed, err := ClearCollection(home, KindRoles)
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 {
		t.Fatalf("removed = %v, want just triage.yaml", removed)
	}
	if _, err := os.Stat(filepath.Join(home, "config.yaml")); err != nil {
		t.Fatalf("config.yaml removed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, DirQueries)); err != nil {
		t.Fatalf("queries/ removed: %v", err)
	}
}

func TestRemoveCollectionItemRolesRejectsConfig(t *testing.T) {
	home := t.TempDir()
	write(t, filepath.Join(home, "config.yaml"), "output: terminal\n")
	if _, err := RemoveCollectionItem(home, KindRoles, "config"); err == nil {
		t.Fatal("want error removing the role named config")
	}
	if _, err := os.Stat(filepath.Join(home, "config.yaml")); err != nil {
		t.Fatalf("config.yaml removed: %v", err)
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
	if RoleFiles(home) != nil {
		t.Fatalf("%s must not read as a role dir: %v", DirData, RoleFiles(home))
	}
}
