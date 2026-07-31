package drive

import (
	"context"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/cmd"
	"github.com/codyconfer/munin/external/plugins/internal/googleauth"
	"github.com/codyconfer/munin/external/plugins/internal/params"
	"github.com/codyconfer/munin/plugin"
)

const (
	PluginID    = "external.drive"
	SignalName  = "drive"
	Destination = "gdrive"
)

func Register() {
	plugin.RegisterSignal(plugin.Descriptor{
		ID:           PluginID,
		Kind:         plugin.KindSignal,
		Signal:       SignalName,
		Capabilities: []plugin.Capability{plugin.CapQuery, plugin.CapAction, plugin.CapCacheable},
	}, plugin.Builders{
		Query: BuildQuery,
	})
	plugin.RegisterBackupDestination(PluginID, Destination, openBackup)
	cmd.RegisterCommand(newDriveCmd)
}

func BuildQuery(bc plugin.BuildContext) (plugin.Query, error) {
	ga, err := googleauth.FromBuildContext(bc)
	if err != nil {
		return nil, err
	}
	s := plugin.SettingsOf(bc, SignalName)
	recent := params.Int(bc.Params(), "recent", plugin.SettingInt(s, "recent", 20))
	return New(ga, plugin.SettingList(s, "folders"), recent), nil
}

type appDataSink struct {
	auth googleauth.Auth
}

func openBackup(h plugin.Host) (plugin.BackupDestination, error) {
	return appDataSink{auth: googleauth.FromHost(h)}, nil
}

func (s appDataSink) Name() string { return "Google Drive app data" }

func (s appDataSink) Upload(ctx context.Context, name string, data []byte, contentType string) (plugin.Item, error) {
	return UploadAppData(ctx, s.auth, name, data, contentType)
}

func (s appDataSink) Prune(ctx context.Context, prefix string, keep int) ([]string, error) {
	return PruneAppData(ctx, s.auth, prefix, keep)
}

func newDriveCmd() *cobra.Command {
	parent := cmd.SignalCmd(SignalName, "Google Drive (read any file; create files only in the configured directory)")

	var content, mime, dir string
	add := &cobra.Command{
		Use:   "add <name>",
		Short: "Create a file in the writable directory (plugins.drive.dir)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return addFile(c, strings.Join(args, " "), content, mime, dir)
		},
	}
	add.Flags().StringVar(&content, "content", "", "text content for the file")
	add.Flags().StringVar(&mime, "mime", "text/plain", "MIME type of the file")
	add.Flags().StringVar(&dir, "dir", "", "target directory; must be the configured writable dir")
	parent.AddCommand(add)
	return parent
}

func addFile(c *cobra.Command, name, content, mime, dir string) error {
	host := cmd.Host()
	configured := plugin.Setting(host.Settings(SignalName), "dir", "")
	target, err := cmd.ResolveWriteTarget("directory", "plugins.drive.dir", configured, dir)
	if err != nil {
		return err
	}
	started := time.Now()
	item, err := CreateFile(c.Context(), googleauth.FromHost(host), target, name, content, mime)
	if err != nil {
		return err
	}
	sections := []plugin.Section{{Signal: SignalName, Title: "Created file in " + target, Items: []plugin.Item{item}}}
	cmd.RecordAction("drive add", started, time.Now(), sections)
	return cmd.EmitSections(c, SignalName, sections)
}
