package config

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codyconfer/sisyphus/configdb"
	"github.com/codyconfer/sisyphus/redact"
)

const exportTestConfig = "output: terminal\nbackup:\n  secret_backend: keyring\n  secret_name: munin-backup-key\ngoogle:\n  oauth_client_secret: super-secret-value\n"

func exportTestStore(t *testing.T, home, content string) *configdb.Store {
	t.Helper()
	db, err := configdb.Open(context.Background(), DataPath(home, ConfigDB))
	if err != nil {
		t.Fatalf("configdb.Open: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	if err := db.Import(context.Background(), ConfigDirective, []byte(content), "yaml"); err != nil {
		t.Fatalf("Import config: %v", err)
	}
	return db
}

func liveConfigBody(t *testing.T, home string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(home, "config.yaml"))
	if err != nil {
		t.Fatalf("reading live config: %v", err)
	}
	return string(b)
}

func TestExportConfigRefusesToMaskTheLiveConfig(t *testing.T) {
	for _, directive := range []string{ConfigDirective, "all"} {
		t.Run(directive, func(t *testing.T) {
			home := t.TempDir()
			db := exportTestStore(t, home, exportTestConfig)
			onDisk := "# hand written\n" + exportTestConfig
			if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte(onDisk), 0o600); err != nil {
				t.Fatal(err)
			}

			var buf bytes.Buffer
			err := Export(&buf, db, "", home, directive, false)
			if err == nil {
				t.Fatalf("Export(out=munin home, includeSecrets=false) succeeded; it must refuse\n%s", buf.String())
			}
			if !strings.Contains(err.Error(), "refusing") {
				t.Errorf("err = %v, want it to say it is refusing the write", err)
			}
			if got := liveConfigBody(t, home); got != onDisk {
				t.Errorf("live config was rewritten:\n%s", got)
			}
			if strings.Contains(liveConfigBody(t, home), redact.Mask) {
				t.Error("the mask marker landed in the live config; backups would break and the key would rotate")
			}
		})
	}
}

func TestExportConfigRefusesTheLiveHomeSpelledDifferently(t *testing.T) {
	home := t.TempDir()
	db := exportTestStore(t, home, exportTestConfig)
	if err := os.WriteFile(filepath.Join(home, "config.yaml"), []byte(exportTestConfig), 0o600); err != nil {
		t.Fatal(err)
	}

	sub := filepath.Join(home, "sub")
	if err := os.MkdirAll(sub, 0o700); err != nil {
		t.Fatal(err)
	}
	aliases := []string{
		home + string(filepath.Separator),
		filepath.Join(home, "."),
		filepath.Join(sub, ".."),
	}
	for _, alias := range aliases {
		var buf bytes.Buffer
		if err := Export(&buf, db, alias, home, ConfigDirective, false); err == nil {
			t.Errorf("Export(out=%q) succeeded; %q is the live home\n%s", alias, alias, buf.String())
		}
		if got := liveConfigBody(t, home); got != exportTestConfig {
			t.Fatalf("live config rewritten via alias %q:\n%s", alias, got)
		}
	}
}

func TestExportConfigMasksOnlyOutsideTheLiveHome(t *testing.T) {
	home := t.TempDir()
	out := t.TempDir()
	db := exportTestStore(t, home, exportTestConfig)

	var buf bytes.Buffer
	if err := Export(&buf, db, out, home, ConfigDirective, false); err != nil {
		t.Fatalf("Export to a separate dir: %v\n%s", err, buf.String())
	}
	if _, err := os.Stat(filepath.Join(home, "config.yaml")); !os.IsNotExist(err) {
		t.Errorf("nothing should have been written into the live home, stat err = %v", err)
	}

	body, err := os.ReadFile(filepath.Join(out, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(body)
	if strings.Contains(got, "super-secret-value") {
		t.Errorf("oauth_client_secret was not masked:\n%s", got)
	}
	if !strings.Contains(got, redact.Mask) {
		t.Errorf("expected a masked value:\n%s", got)
	}
	if !strings.Contains(got, "keyring") || !strings.Contains(got, "munin-backup-key") {
		t.Errorf("backup selectors must survive the masked export:\n%s", got)
	}
	if !strings.Contains(buf.String(), "warning") {
		t.Errorf("the masking path must warn about what it produced, got:\n%s", buf.String())
	}

	var live bytes.Buffer
	if err := Export(&live, db, home, home, ConfigDirective, false); err == nil {
		t.Fatalf("the same masked export aimed at the live home must be refused\n%s", live.String())
	}
	if _, err := os.Stat(filepath.Join(home, "config.yaml")); !os.IsNotExist(err) {
		t.Errorf("the refused export still touched the live home, stat err = %v", err)
	}
}

func TestExportConfigWithSecretsMaterializesIntoTheLiveHome(t *testing.T) {
	home := t.TempDir()
	db := exportTestStore(t, home, exportTestConfig)

	var buf bytes.Buffer
	if err := Export(&buf, db, "", home, ConfigDirective, true); err != nil {
		t.Fatalf("Export(includeSecrets=true): %v\n%s", err, buf.String())
	}
	got := liveConfigBody(t, home)
	if got != exportTestConfig {
		t.Errorf("live config = %q, want the stored content verbatim", got)
	}
	if !strings.Contains(buf.String(), "cleartext") {
		t.Errorf("the cleartext path must warn, got:\n%s", buf.String())
	}

	var masked bytes.Buffer
	if err := Export(&masked, db, "", home, ConfigDirective, false); err == nil {
		t.Fatalf("only --include-secrets may write the live home; the masked form must be refused\n%s", masked.String())
	}
	if got := liveConfigBody(t, home); got != exportTestConfig {
		t.Errorf("the refused masked export overwrote the cleartext config:\n%s", got)
	}
	if strings.Contains(liveConfigBody(t, home), redact.Mask) {
		t.Error("the mask marker landed in the live config")
	}
}

func TestSamePath(t *testing.T) {
	dir := t.TempDir()
	same := []string{dir, dir + string(filepath.Separator), filepath.Join(dir, "."), filepath.Join(dir, "x", "..")}
	for _, p := range same {
		if !SamePath(p, dir) {
			t.Errorf("SamePath(%q, %q) = false, want true", p, dir)
		}
	}
	if SamePath(filepath.Join(dir, "sub"), dir) {
		t.Errorf("SamePath(%q/sub, %q) = true, want false", dir, dir)
	}

	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(dir, link); err != nil {
		t.Skipf("symlinks unavailable here: %v", err)
	}
	if !SamePath(link, dir) {
		t.Errorf("SamePath(%q, %q) = false: a symlink to the munin home is the munin home", link, dir)
	}
	if !SamePath(filepath.Join(link, "."), dir) {
		t.Errorf("SamePath(%q/., %q) = false", link, dir)
	}
	if SamePath(link, filepath.Dir(link)) {
		t.Errorf("SamePath(%q, %q) = true, want false", link, filepath.Dir(link))
	}
}

func importDirectiveRows(t *testing.T, db *configdb.Store, rows map[string]string) {
	t.Helper()
	blob, err := json.Marshal(rows)
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Import(context.Background(), DirectivesDirective, blob, "collection"); err != nil {
		t.Fatalf("Import directives: %v", err)
	}
}

func TestExportAllToFilesSkipsALegacyConfigRowInsteadOfLosingEveryDirective(t *testing.T) {
	home := t.TempDir()
	db := exportTestStore(t, home, exportTestConfig)
	importDirectiveRows(t, db, map[string]string{
		"Config.yaml":            "output: terminal\n",
		DirQueries + "/prs.yaml": "name: prs\ntype: query\nsignal: github\n",
	})

	written, err := ExportAllToFiles(db, home)
	if err != nil {
		t.Fatalf("one unusable row bricked every apply: %v", err)
	}
	prs := filepath.Join(home, DirQueries, "prs.yaml")
	raw, err := os.ReadFile(prs)
	if err != nil {
		t.Fatalf("queries/prs.yaml was lost to the bad row: %v", err)
	}
	if !strings.Contains(string(raw), "signal: github") {
		t.Errorf("queries/prs.yaml = %q", raw)
	}
	if got := liveConfigBody(t, home); got != exportTestConfig {
		t.Errorf("config.yaml = %q, want the stored config, not the skipped row", got)
	}
	for _, p := range written {
		if filepath.Base(p) == "Config.yaml" {
			t.Errorf("the skipped row was reported as written: %v", written)
		}
	}
}

func TestExportAllToFilesValidatesEveryRowBeforeWritingTheConfig(t *testing.T) {
	home := t.TempDir()
	db := exportTestStore(t, home, exportTestConfig)
	importDirectiveRows(t, db, map[string]string{
		DirQueries + "/plain.txt": "name: nope\ntype: query\nsignal: github\n",
		DirQueries + "/prs.yaml":  "name: prs\ntype: query\nsignal: github\n",
	})

	if written, err := ExportAllToFiles(db, home); err == nil {
		t.Fatalf("ExportAllToFiles accepted an unusable row: written=%v", written)
	}
	if _, err := os.Stat(filepath.Join(home, "config.yaml")); !os.IsNotExist(err) {
		t.Errorf("config.yaml was written before the directive set was checked, so one bad row half-applies the store (stat err = %v)", err)
	}
	if _, err := os.Stat(filepath.Join(home, DirQueries, "prs.yaml")); !os.IsNotExist(err) {
		t.Errorf("a directive was written even though the apply failed (stat err = %v)", err)
	}
}
