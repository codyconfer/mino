package backup

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/codyconfer/sisyphus"
	"github.com/codyconfer/sisyphus/store"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/pluginhost"
	"github.com/codyconfer/munin/internal/token"

	_ "github.com/codyconfer/munin/internal/plugin/ntr"
)

const secretService = "munin"

func secretName(cfg *config.Config) string {
	if cfg != nil && cfg.Backup.SecretName != "" {
		return cfg.Backup.SecretName
	}
	return config.Defaults().Backup.SecretName
}

func backupSpec(cfg *config.Config, files []string) sisyphus.BackupSpec {
	return sisyphus.BackupSpec{
		Files:         files,
		SecretBackend: cfg.Backup.SecretBackend,
		SecretName:    secretName(cfg),
		SecretService: secretService,
	}
}

func restoreSpec(cfg *config.Config, sealed []byte, dest string) sisyphus.RestoreSpec {
	return sisyphus.RestoreSpec{
		Sealed:        sealed,
		SecretBackend: cfg.Backup.SecretBackend,
		SecretName:    secretName(cfg),
		SecretService: secretService,
		DestDir:       dest,
	}
}

func Run(ctx context.Context, w io.Writer, cfg *config.Config, closeDBs func(), tokens *token.Store, outDir string) error {
	home := cfg.Home

	closeDBs()

	files := []string{
		config.DataPath(home, config.ConfigDB),
		config.DataPath(home, config.AuditDB),
		config.DataPath(home, config.TokensDB),
	}
	files = append(files, store.BackupPaths()...)
	files = append(files, plugin.DataPaths(home)...)

	sealed, storeName, err := sisyphus.Backup(ctx, backupSpec(cfg, files))
	if err != nil {
		return errs.Wrap(errs.KindBackup, err, "creating encrypted backup").
			WithHint("configure a secret backend (backup.secret_backend) or ensure the OS keyring is available")
	}

	name := fmt.Sprintf("%s%s.tar.enc", Prefix, time.Now().Format("20060102-150405"))
	keep := cfg.Backup.Keep

	if dest := cfg.Backup.Destination; dest != "" && dest != "local" {
		return uploadTo(ctx, w, cfg, tokens, dest, name, sealed, storeName, keep)
	}

	path := filepath.Join(outDir, name)
	if err := os.WriteFile(path, sealed, 0o600); err != nil {
		return errs.Wrapf(errs.KindBackup, err, "writing backup to %s", path)
	}
	fmt.Fprintf(w, "encrypted backup written to %s (%d bytes; key in %s)\n", path, len(sealed), storeName)
	if pruned := PruneLocal(outDir, keep); len(pruned) > 0 {
		fmt.Fprintf(w, "pruned %d old backup(s) (keep=%d)\n", len(pruned), keep)
	}
	return nil
}

func Restore(ctx context.Context, w io.Writer, cfg *config.Config, closeDBs func(), file, dest string) error {
	sealed, err := os.ReadFile(file)
	if err != nil {
		return errs.Wrap(errs.KindBackup, err, "reading backup file")
	}
	if dest == "" {
		dest = config.DataDir(cfg.Home)
	}

	closeDBs()

	names, storeName, err := sisyphus.Restore(ctx, restoreSpec(cfg, sealed, dest))
	if err != nil {
		return errs.Wrap(errs.KindBackup, err, "restoring backup").
			WithHint("ensure the same secret backend (backup.secret_backend) that created the backup is configured")
	}
	fmt.Fprintf(w, "restored %s into %s (key from %s)\n",
		strings.Join(names, ", "), dest, storeName)
	return nil
}

func uploadTo(ctx context.Context, w io.Writer, cfg *config.Config, tokens *token.Store, dest, name string, sealed []byte, storeName string, keep int) error {
	open, ok := plugin.LookupBackupDestination(dest)
	if !ok {
		return errs.Newf(errs.KindConfig, "unknown backup destination %q", dest).
			WithHint("set backup.destination to local or one of: %s", strings.Join(plugin.BackupDestinations(), ", "))
	}
	sink, err := open(pluginhost.New(cfg, tokens))
	if err != nil {
		return errs.Wrapf(errs.KindBackup, err, "opening backup destination %q", dest)
	}
	item, err := sink.Upload(ctx, name, sealed, "application/octet-stream")
	if err != nil {
		return errs.Wrapf(errs.KindBackup, err, "uploading backup to %s", sink.Name())
	}
	fmt.Fprintf(w, "encrypted backup uploaded to %s: %s (key in %s)\n", sink.Name(), item.Title, storeName)
	if deleted, err := sink.Prune(ctx, Prefix, keep); err == nil && len(deleted) > 0 {
		fmt.Fprintf(w, "pruned %d old backup(s) (keep=%d)\n", len(deleted), keep)
	}
	return nil
}
