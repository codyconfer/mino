package backup

import (
	"testing"

	"github.com/codyconfer/mino/internal/config"
)

const sisyphusDefaultKeyName = "backup-key"

func TestBackupSpecNeverSendsAnEmptySecretName(t *testing.T) {
	want := config.Defaults().Backup.SecretName
	if want == "" || want == sisyphusDefaultKeyName {
		t.Fatalf("mino default backup key name = %q, want a non-empty name distinct from sisyphus's %q; if "+
			"they ever match, coercing an empty name buys nothing and this guard is moot",
			want, sisyphusDefaultKeyName)
	}

	for _, cfg := range []*config.Config{
		{},
		{Backup: config.BackupConfig{SecretBackend: "keyring"}},
		{Backup: config.BackupConfig{SecretName: ""}},
	} {
		spec := backupSpec(cfg, []string{"a.duckdb"})
		if spec.Secret.Name != want {
			t.Errorf("backupSpec(%+v).Secret.Name = %q, want %q: an empty name lets sisyphus fall back to %q and "+
				"mint a fresh key, orphaning every prior backup", cfg.Backup, spec.Secret.Name, want,
				sisyphusDefaultKeyName)
		}
		rspec := restoreSpec(cfg, []byte("sealed"), t.TempDir())
		if rspec.Secret.Name != want {
			t.Errorf("restoreSpec(%+v).Secret.Name = %q, want %q: restore must look for the same key backup wrote",
				cfg.Backup, rspec.Secret.Name, want)
		}
		if rspec.Secret.Name != spec.Secret.Name {
			t.Errorf("backup and restore disagree on the key name: %q vs %q", spec.Secret.Name, rspec.Secret.Name)
		}
	}

	cfg := &config.Config{Backup: config.BackupConfig{SecretBackend: "keyring"}}
	if spec := backupSpec(cfg, []string{"a.duckdb"}); spec.Secret.Service != secretService {
		t.Errorf("backupSpec.Secret.Service = %q, want %q", spec.Secret.Service, secretService)
	}

	explicit := &config.Config{Backup: config.BackupConfig{SecretName: "my-key"}}
	if got := backupSpec(explicit, []string{"a.duckdb"}).Secret.Name; got != "my-key" {
		t.Errorf("backupSpec with an explicit name = %q, want %q; coercion must not override the user", got, "my-key")
	}
	if got := restoreSpec(explicit, []byte("sealed"), t.TempDir()).Secret.Name; got != "my-key" {
		t.Errorf("restoreSpec with an explicit name = %q, want %q", got, "my-key")
	}
}
