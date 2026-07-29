package cmd

import (
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/signals"
	"github.com/codyconfer/munin/internal/signals/build"
	"github.com/codyconfer/munin/internal/signals/gdrive"
)

func newDriveCmd() *cobra.Command {
	parent := &cobra.Command{
		Use:   "drive",
		Short: "Google Drive (read any file; create files only in the configured directory)",
	}

	var ff filterFlags
	query := &cobra.Command{
		Use:   "query",
		Short: "List Drive files (recent, or within the configured folders)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSignal(cmd, "drive", nil, &ff)
		},
	}
	ff.bind(query)

	var content, mime, dir string
	add := &cobra.Command{
		Use:   "add <name>",
		Short: "Create a file in the writable directory (drive.dir)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return addDriveFile(cmd, strings.Join(args, " "), content, mime, dir)
		},
	}
	add.Flags().StringVar(&content, "content", "", "text content for the file")
	add.Flags().StringVar(&mime, "mime", "text/plain", "MIME type of the file")
	add.Flags().StringVar(&dir, "dir", "", "target directory; must be the configured writable dir")

	parent.AddCommand(query, add)
	return parent
}

func addDriveFile(cmd *cobra.Command, name, content, mime, dir string) error {
	target, err := build.ResolveWriteTarget("directory", "drive.dir", shared.Cfg.Drive.Dir, dir)
	if err != nil {
		return err
	}
	started := time.Now()
	item, err := gdrive.CreateFile(cmd.Context(), googleAuth(), target, name, content, mime)
	if err != nil {
		return err
	}
	sections := []signals.Section{{Signal: "drive", Title: "Created file in " + target, Items: []signals.Item{item}}}
	shared.Audit.RecordAction("drive add", shared.Cfg.Role, started, time.Now(), sections)
	return emit(cmd.OutOrStdout(), "drive", sections)
}
