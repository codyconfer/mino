package plugin

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/testenv"
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
	if err := os.WriteFile(filepath.Join(pack, "queries", "demo.yaml"), []byte("name: demo\ntype: query\nsignal: github\n"), 0o600); err != nil {
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
	testenv.Isolate(t)
	home := t.TempDir()
	pack := filepath.Join(home, ".plugins", "pack")
	if err := os.MkdirAll(filepath.Join(pack, "queries"), 0o700); err != nil {
		t.Fatal(err)
	}
	body := []byte("name: from-pack\ntype: query\nsignal: github\n")
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
	testenv.Isolate(t)
	LoadEnabled()
	home := t.TempDir()
	id := "test.local.merge"
	if _, ok := Lookup(id); !ok {
		Register(Descriptor{ID: id, Kind: KindSignal, Signal: "testlocalmerge", Capabilities: []Capability{CapQuery}})
	}
	pack := filepath.Join(home, ".plugins", id)
	if err := os.MkdirAll(filepath.Join(pack, "queries"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pack, "plugin.yaml"), []byte("id: "+id+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(pack, "queries", "merge-extra.yaml"), []byte("name: merge-extra\ntype: query\nsignal: testlocalmerge\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	cands, err := ListInstallCandidates(home)
	if err != nil {
		t.Fatal(err)
	}
	var got InstallCandidate
	count := 0
	for _, c := range cands {
		if c.ID == id {
			count++
			got = c
		}
	}
	if count != 1 {
		t.Fatalf("expected merged candidate once, got %d in %+v", count, cands)
	}
	if got.Source != "local+registry" || len(got.Seeds) != 1 {
		t.Fatalf("candidate = %+v", got)
	}
	if got.LocalDir != pack {
		t.Fatalf("LocalDir = %q, want %q", got.LocalDir, pack)
	}
	if !strings.Contains(got.Desc, ".plugins/"+id) {
		t.Fatalf("Desc must name the originating directory: %q", got.Desc)
	}
}

func TestLocalManifestCannotClaimRegisteredID(t *testing.T) {
	testenv.Isolate(t)
	LoadEnabled()
	home := t.TempDir()
	id := "test.local.spoof"
	if _, ok := Lookup(id); !ok {
		Register(Descriptor{ID: id, Kind: KindSignal, Signal: "testlocalspoof", Capabilities: []Capability{CapQuery}})
	}
	evil := filepath.Join(home, ".plugins", "evil")
	if err := os.MkdirAll(filepath.Join(evil, "queries"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(evil, "plugin.yaml"), []byte("id: "+id+"\nname: "+id+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(evil, "queries", "evil.yaml"), []byte("name: evil\ntype: query\nsignal: testlocalspoof\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	locals, err := DiscoverLocal(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(locals) != 1 {
		t.Fatalf("locals = %+v", locals)
	}
	lp := locals[0]
	if lp.ID == id {
		t.Fatalf("local pack bound itself to registered id %q: %+v", id, lp)
	}
	if lp.ID != "evil" {
		t.Fatalf("expected the directory name as id, got %+v", lp)
	}
	if lp.Installable || lp.Reason == "" {
		t.Fatalf("a spoofed id must be reported, not installed: %+v", lp)
	}

	cands, err := ListInstallCandidates(home)
	if err != nil {
		t.Fatal(err)
	}
	var reg InstallCandidate
	for _, c := range cands {
		if c.ID == id {
			reg = c
		}
	}
	if reg.ID == "" {
		t.Fatalf("registry candidate missing: %+v", cands)
	}
	if reg.Source != "registry" || reg.LocalDir != "" || len(reg.Seeds) != 0 {
		t.Fatalf("registry candidate hijacked by .plugins/evil: %+v", reg)
	}
	if _, err := InstallCandidateEntry(home, reg, InstallOptions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(home, "queries", "evil.yaml")); !os.IsNotExist(err) {
		t.Fatalf("installing %q wrote the attacker's directive: %v", id, err)
	}
}

func TestDuplicateLocalIDsDoNotShadow(t *testing.T) {
	home := t.TempDir()
	for _, name := range []string{"a-pack", "b-pack"} {
		dir := filepath.Join(home, ".plugins", name)
		if err := os.MkdirAll(filepath.Join(dir, "queries"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "plugin.yaml"), []byte("id: local.dup\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "queries", name+".yaml"), []byte("name: "+name+"\ntype: query\nsignal: github\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	locals, err := DiscoverLocal(home)
	if err != nil {
		t.Fatal(err)
	}
	if len(locals) != 2 {
		t.Fatalf("both packs must be listed, got %+v", locals)
	}
	byName := map[string]LocalPlugin{}
	for _, lp := range locals {
		byName[lp.Name] = lp
	}
	winner := byName["a-pack"]
	loser := byName["b-pack"]
	if winner.ID != "local.dup" || !winner.Installable {
		t.Fatalf("first pack should keep the id: %+v", winner)
	}
	if loser.Installable {
		t.Fatalf("second pack claiming the same id must not be installable: %+v", loser)
	}
	if loser.Reason == "" || !strings.Contains(loser.Reason, "a-pack") {
		t.Fatalf("conflict reason must name the owning directory: %+v", loser)
	}
}

func TestDiscoverLocalSkipsBrokenPacks(t *testing.T) {
	home := t.TempDir()
	broken := filepath.Join(home, ".plugins", "broken")
	if err := os.MkdirAll(broken, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, "plugin.yaml"), []byte("id: [unterminated\n  nope\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	good := filepath.Join(home, ".plugins", "good")
	if err := os.MkdirAll(filepath.Join(good, "queries"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(good, "queries", "good.yaml"), []byte("name: good\ntype: query\nsignal: github\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	locals, err := DiscoverLocal(home)
	if err != nil {
		t.Fatalf("one bad pack must not fail discovery: %v", err)
	}
	if len(locals) != 2 {
		t.Fatalf("locals = %+v", locals)
	}
	byID := map[string]LocalPlugin{}
	for _, lp := range locals {
		byID[lp.ID] = lp
	}
	bad, ok := byID["broken"]
	if !ok {
		t.Fatalf("broken pack missing: %+v", locals)
	}
	if bad.Installable {
		t.Fatalf("broken pack must not be installable: %+v", bad)
	}
	if bad.Reason == "" || !strings.Contains(bad.Reason, "plugin.yaml") {
		t.Fatalf("reason must name the failing file: %+v", bad)
	}
	okPack, ok := byID["good"]
	if !ok || !okPack.Installable || len(okPack.Seeds) != 1 {
		t.Fatalf("healthy pack must still be installable: %+v", okPack)
	}
}

func TestDiscoverLocalSkipsUnreadableSeedDir(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("file modes do not deny reads on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root ignores file modes")
	}
	home := t.TempDir()
	bad := filepath.Join(home, ".plugins", "unreadable")
	if err := os.MkdirAll(filepath.Join(bad, "queries"), 0o700); err != nil {
		t.Fatal(err)
	}
	seed := filepath.Join(bad, "queries", "x.yaml")
	if err := os.WriteFile(seed, []byte("name: x\ntype: query\nsignal: github\n"), 0o000); err != nil {
		t.Fatal(err)
	}
	good := filepath.Join(home, ".plugins", "good")
	if err := os.MkdirAll(filepath.Join(good, "queries"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(good, "queries", "good.yaml"), []byte("name: good\ntype: query\nsignal: github\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	locals, err := DiscoverLocal(home)
	if err != nil {
		t.Fatalf("one unreadable seed must not fail discovery: %v", err)
	}
	byID := map[string]LocalPlugin{}
	for _, lp := range locals {
		byID[lp.ID] = lp
	}
	if got := byID["unreadable"]; got.Installable || got.Reason == "" {
		t.Fatalf("unreadable pack = %+v", got)
	}
	if got := byID["good"]; !got.Installable {
		t.Fatalf("healthy pack = %+v", got)
	}
}

func TestListInstallCandidatesSurvivesBrokenPack(t *testing.T) {
	testenv.Isolate(t)
	LoadEnabled()
	home := t.TempDir()
	id := "test.local.survives"
	if _, ok := Lookup(id); !ok {
		Register(Descriptor{ID: id, Kind: KindSignal, Signal: "testlocalsurvives", Capabilities: []Capability{CapQuery}})
	}
	broken := filepath.Join(home, ".plugins", "broken")
	if err := os.MkdirAll(broken, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(broken, "plugin.json"), []byte("{not json"), 0o600); err != nil {
		t.Fatal(err)
	}

	cands, err := ListInstallCandidates(home)
	if err != nil {
		t.Fatalf("one bad pack must not hide every candidate: %v", err)
	}
	var reg, bad InstallCandidate
	for _, c := range cands {
		switch c.ID {
		case id:
			reg = c
		case "broken":
			bad = c
		}
	}
	if reg.ID == "" || !reg.Installable || reg.Source != "registry" {
		t.Fatalf("pure-registry candidate must remain installable: %+v", cands)
	}
	if bad.ID == "" {
		t.Fatalf("broken pack must appear as a row: %+v", cands)
	}
	if bad.Installable || bad.Reason == "" {
		t.Fatalf("broken row = %+v", bad)
	}
	if !strings.Contains(bad.Desc, ".plugins/broken") {
		t.Fatalf("broken row must name its directory: %q", bad.Desc)
	}
}

func TestListInstallCandidatesOmitsInternal(t *testing.T) {
	testenv.Isolate(t)
	LoadEnabled()
	id := "mino.ntr"
	if _, ok := Lookup(id); !ok {
		t.Skip("mino.ntr not registered in this binary")
	}
	if !Installed(id) {
		t.Fatal("internal plugin should report installed without settings entry")
	}
	cands, err := ListInstallCandidates(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, c := range cands {
		if IsInternal(c.ID) && c.Source != "local" {
			t.Fatalf("internal plugin in install candidates: %+v", c)
		}
	}
}
