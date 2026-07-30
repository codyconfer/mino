package backup

import (
	"testing"

	"github.com/codyconfer/munin/internal/config"
)

const sisyphusDefaultKeyName = "backup-key"

func TestSecretNameCoercesEmptyToMuninDefault(t *testing.T) {
	want := config.Defaults().Backup.SecretName
	if want == "" || want == sisyphusDefaultKeyName {
		t.Fatalf("munin default backup key name = %q, want a non-empty name distinct from sisyphus's %q",
			want, sisyphusDefaultKeyName)
	}

	for _, cfg := range []*config.Config{
		{},
		{Backup: config.BackupConfig{SecretBackend: "keyring"}},
		{Backup: config.BackupConfig{SecretName: ""}},
	} {
		if got := secretName(cfg); got != want {
			t.Errorf("secretName(%+v) = %q, want %q: an empty name lets sisyphus fall back to %q and mint a fresh key",
				cfg.Backup, got, want, sisyphusDefaultKeyName)
		}
	}

	explicit := &config.Config{Backup: config.BackupConfig{SecretName: "my-key"}}
	if got := secretName(explicit); got != "my-key" {
		t.Errorf("secretName with an explicit name = %q, want %q", got, "my-key")
	}
}

func TestBackupSpecNeverSendsAnEmptySecretName(t *testing.T) {
	want := config.Defaults().Backup.SecretName
	cfg := &config.Config{Backup: config.BackupConfig{SecretBackend: "keyring"}}

	spec := backupSpec(cfg, []string{"a.duckdb"})
	if spec.SecretName != want {
		t.Errorf("backupSpec.SecretName = %q, want %q", spec.SecretName, want)
	}
	if spec.SecretService != secretService {
		t.Errorf("backupSpec.SecretService = %q, want %q", spec.SecretService, secretService)
	}

	rspec := restoreSpec(cfg, []byte("sealed"), t.TempDir())
	if rspec.SecretName != want {
		t.Errorf("restoreSpec.SecretName = %q, want %q: restore must look for the same key backup wrote", rspec.SecretName, want)
	}
	if rspec.SecretName != spec.SecretName {
		t.Errorf("backup and restore disagree on the key name: %q vs %q", spec.SecretName, rspec.SecretName)
	}
}
