//go:build !nodaemon

package views

import (
	"github.com/codyconfer/viewkit/forms"

	"github.com/codyconfer/munin/internal/config"
)

// setvDaemonFields are the daemon.* rows of the edit-config form. See
// settings_nodaemon.go for the build without daemon mode.
func setvDaemonFields(c *config.Config) []forms.Field {
	return []forms.Field{
		{Key: "daemon.interval", Label: "daemon.interval", Kind: forms.FieldText, Text: c.Daemon.Interval},
		{Key: "daemon.bell", Label: "daemon.bell", Kind: forms.FieldToggle, On: c.Daemon.Bell},
		{Key: "daemon.desktop", Label: "daemon.desktop", Kind: forms.FieldToggle, On: c.Daemon.Desktop},
		{Key: "daemon.tray", Label: "daemon.tray", Kind: forms.FieldToggle, On: c.Daemon.Tray},
		{Key: "daemon.theme", Label: "daemon.theme", Kind: forms.FieldSelect, Options: setvFirst([]string{"dark", "light"}, c.Daemon.Theme)},
	}
}

// setvDaemonValues copies the submitted daemon.* form values into set.
func setvDaemonValues(vals, set map[string]any) {
	set["daemon.interval"] = setvString(vals["daemon.interval"])
	set["daemon.bell"] = setvBool(vals["daemon.bell"])
	set["daemon.desktop"] = setvBool(vals["daemon.desktop"])
	set["daemon.tray"] = setvBool(vals["daemon.tray"])
	set["daemon.theme"] = setvString(vals["daemon.theme"])
}

// setvStatusBarDaemonEntries is the daemon chip's show/hide toggle.
func setvStatusBarDaemonEntries() []statusBarEntry {
	return []statusBarEntry{{"daemon", "daemon"}}
}
