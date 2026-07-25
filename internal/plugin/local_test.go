package plugin

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDiscoverLocalMissingDir(t *testing.T) {
	got, err := DiscoverLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("got %v", got)
	}
}

func TestDiscoverLocalSeedPackAndIncompatible(t *testing.T) {
	home := t.TempDir()
	pack := filepath.Join(home, ".plugins", "demo-pack")
	if err := os.MkdirAll(filepath.Join(pack, "queries"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pack, "plugin.yaml"), []byte("id: local.demo\ndescription: demo seeds\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pack, "queries", "demo.yaml"), []byte("name: demo\nsignal: github\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	bare := filepath.Join(home, ".plugins", "go-only")
	if err := os.MkdirAll(bare, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(bare, "plugin.go"), []byte("package x\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := DiscoverLocal(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("len=%d got=%+v", len(got), got)
	}
	byID := map[string]LocalPlugin{}
	for _, lp := range got {
		byID[lp.ID] = lp
	}
	demo := byID["local.demo"]
	if !demo.Installable || demo.Registered || len(demo.Seeds) != 1 {
		t.Fatalf("demo = %+v", demo)
	}
	if demo.Description != "demo seeds" {
		t.Fatalf("desc = %q", demo.Description)
	}
	bad := byID["go-only"]
	if bad.Installable || bad.Reason == "" {
		t.Fatalf("go-only = %+v", bad)
	}
}

func TestInstallCandidateEntrySeedPack(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	home := t.TempDir()
	pack := filepath.Join(home, ".plugins", "pack")
	if err := os.MkdirAll(filepath.Join(pack, "queries"), 0o700); err != nil {
		t.Fatal(err)
	}
	body := []byte("name: from-pack\nsignal: github\n")
	if err := os.WriteFile(filepath.Join(pack, "queries", "from-pack.yaml"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	cands, err := ListInstallCandidates(home)
	if err != nil {
		t.Fatal(err)
	}
	var packCand InstallCandidate
	for _, c := range cands {
		if c.ID == "pack" && c.Source == "local" {
			packCand = c
			break
		}
	}
	if packCand.ID == "" || !packCand.Installable {
		t.Fatalf("missing local pack candidate: %+v", cands)
	}
	res, err := InstallCandidateEntry(home, packCand, InstallOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(res.Written) != 1 || res.Written[0] != "queries/from-pack.yaml" {
		t.Fatalf("written = %v", res.Written)
	}
	got, err := os.ReadFile(filepath.Join(home, "queries", "from-pack.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(body) {
		t.Fatalf("content = %q", got)
	}
}

func TestListInstallCandidatesMergesLocalRegistry(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	LoadEnabled()
	home := t.TempDir()
	id := "munin.ntr"
	if _, ok := Lookup(id); !ok {
		t.Skip("munin.ntr not registered in this binary")
	}
	pack := filepath.Join(home, ".plugins", "ntr-extra")
	if err := os.MkdirAll(filepath.Join(pack, "queries"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pack, "plugin.yaml"), []byte("id: "+id+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pack, "queries", "ntr-extra.yaml"), []byte("name: ntr-extra\nsignal: ntr\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cands, err := ListInstallCandidates(home)
	if err != nil {
		t.Fatal(err)
	}
	var ntr InstallCandidate
	ntrCount := 0
	for _, c := range cands {
		if c.ID == id {
			ntrCount++
			ntr = c
		}
	}
	if ntrCount != 1 {
		t.Fatalf("expected merged ntr once, got %d in %+v", ntrCount, cands)
	}
	if ntr.Source != "local+registry" || len(ntr.Seeds) != 1 {
		t.Fatalf("ntr = %+v", ntr)
	}
}
