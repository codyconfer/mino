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

	"github.com/codyconfer/munin/internal/auth"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/signals/gdrive"

	_ "github.com/codyconfer/munin/internal/plugin/ntr" // RegisterDataPath
)

const secretService = "munin"

func Run(ctx context.Context, w io.Writer, cfg *config.Config, closeDBs func(), ga auth.GoogleAuth, outDir string) error {
	home := cfg.Home

	closeDBs()

	files := []string{
		filepath.Join(home, "config.duckdb"),
		filepath.Join(home, "audit.duckdb"),
		filepath.Join(home, "tokens.duckdb"),
	}
	// Open-session paths (sisyphus RegisterBackupPath) plus known plugin DBs
	// that may not have been opened this run (ADR-11).
	files = append(files, store.BackupPaths()...)
	files = append(files, plugin.DataPaths(home)...)

	sealed, storeName, err := sisyphus.Backup(ctx, sisyphus.BackupSpec{
		Files:         files,
		SecretBackend: cfg.Backup.SecretBackend,
		SecretName:    cfg.Backup.SecretName,
		SecretService: secretService,
	})
	if err != nil {
		return errs.Wrap(errs.KindBackup, err, "creating encrypted backup").
			WithHint("configure a secret backend (backup.secret_backend) or ensure the OS keyring is available")
	}

	name := fmt.Sprintf("%s%s.tar.enc", Prefix, time.Now().Format("20060102-150405"))
	keep := cfg.Backup.Keep

	if cfg.Backup.Destination == "gdrive" {
		item, err := gdrive.UploadAppData(ctx, ga, name, sealed, "application/octet-stream")
		if err != nil {
			return errs.Wrap(errs.KindBackup, err, "uploading backup to Google Drive")
		}
		fmt.Fprintf(w, "encrypted backup uploaded to Google Drive app data: %s (key in %s)\n", item.Title, storeName)
		if deleted, err := gdrive.PruneAppData(ctx, ga, Prefix, keep); err == nil && len(deleted) > 0 {
			fmt.Fprintf(w, "pruned %d old backup(s) (keep=%d)\n", len(deleted), keep)
		}
		return nil
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
		dest = cfg.Home
	}

	closeDBs()

	names, storeName, err := sisyphus.Restore(ctx, sisyphus.RestoreSpec{
		Sealed:        sealed,
		SecretBackend: cfg.Backup.SecretBackend,
		SecretName:    cfg.Backup.SecretName,
		SecretService: secretService,
		DestDir:       dest,
	})
	if err != nil {
		return errs.Wrap(errs.KindBackup, err, "restoring backup").
			WithHint("ensure the same secret backend (backup.secret_backend) that created the backup is configured")
	}
	fmt.Fprintf(w, "restored %s into %s (key from %s)\n",
		strings.Join(names, ", "), dest, storeName)
	return nil
}
