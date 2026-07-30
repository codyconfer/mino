package config

import (
	"bytes"
	"context"
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
}
