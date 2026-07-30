package backup

import (
	"testing"

	"github.com/codyconfer/munin/internal/config"
)

const sisyphusDefaultKeyName = "backup-key"

func TestBackupSpecNeverSendsAnEmptySecretName(t *testing.T) {
	want := config.Defaults().Backup.SecretName
	if want == "" || want == sisyphusDefaultKeyName {
		t.Fatalf("munin default backup key name = %q, want a non-empty name distinct from sisyphus's %q; if "+
			"they ever match, coercing an empty name buys nothing and this guard is moot",
			want, sisyphusDefaultKeyName)
	}

	for _, cfg := range []*config.Config{
		{},
		{Backup: config.BackupConfig{SecretBackend: "keyring"}},
		{Backup: config.BackupConfig{SecretName: ""}},
	} {
		spec := backupSpec(cfg, []string{"a.duckdb"})
		if spec.SecretName != want {
			t.Errorf("backupSpec(%+v).SecretName = %q, want %q: an empty name lets sisyphus fall back to %q and "+
				"mint a fresh key, orphaning every prior backup", cfg.Backup, spec.SecretName, want,
				sisyphusDefaultKeyName)
		}
		rspec := restoreSpec(cfg, []byte("sealed"), t.TempDir())
		if rspec.SecretName != want {
			t.Errorf("restoreSpec(%+v).SecretName = %q, want %q: restore must look for the same key backup wrote",
				cfg.Backup, rspec.SecretName, want)
		}
		if rspec.SecretName != spec.SecretName {
			t.Errorf("backup and restore disagree on the key name: %q vs %q", spec.SecretName, rspec.SecretName)
		}
	}

	cfg := &config.Config{Backup: config.BackupConfig{SecretBackend: "keyring"}}
	if spec := backupSpec(cfg, []string{"a.duckdb"}); spec.SecretService != secretService {
		t.Errorf("backupSpec.SecretService = %q, want %q", spec.SecretService, secretService)
	}

	explicit := &config.Config{Backup: config.BackupConfig{SecretName: "my-key"}}
	if got := backupSpec(explicit, []string{"a.duckdb"}).SecretName; got != "my-key" {
		t.Errorf("backupSpec with an explicit name = %q, want %q; coercion must not override the user", got, "my-key")
	}
	if got := restoreSpec(explicit, []byte("sealed"), t.TempDir()).SecretName; got != "my-key" {
		t.Errorf("restoreSpec with an explicit name = %q, want %q", got, "my-key")
	}
}
