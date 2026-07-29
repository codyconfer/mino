package daemon

import (
	"github.com/codyconfer/viewkit/forms"

	"github.com/codyconfer/munin/cmd"
	"github.com/codyconfer/munin/internal/app/statusstrip"
	"github.com/codyconfer/munin/internal/app/views"
	"github.com/codyconfer/munin/internal/config"
)

func init() {
	cmd.RegisterCommand(newDaemonCmd)
	statusstrip.RegisterChip(statusChip)
	views.RegisterSettingsSection(views.SettingsSection{
		Fields:    trayFields,
		Values:    trayValues,
		StatusBar: []views.StatusBarEntry{{ID: "daemon", Label: "daemon"}},
	})
}

func trayFields(c *config.Config) []forms.Field {
	return []forms.Field{
		{Key: "daemon.tray", Label: "daemon.tray", Kind: forms.FieldToggle, On: c.Daemon.Tray},
	}
}

func trayValues(vals, set map[string]any) {
	set["daemon.tray"] = forms.Bool(vals, "daemon.tray")
}
