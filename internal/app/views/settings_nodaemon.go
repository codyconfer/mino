//go:build nodaemon

package views

import (
	"github.com/codyconfer/viewkit/forms"

	"github.com/codyconfer/munin/internal/config"
)

// setvDaemonFields is empty: `nodaemon` builds read no daemon config, so the
// edit-config form omits the daemon.* rows entirely.
func setvDaemonFields(*config.Config) []forms.Field { return nil }

// setvDaemonValues is a no-op so saving the form leaves any daemon.* keys
// already on disk untouched.
func setvDaemonValues(_, _ map[string]any) {}

// setvStatusBarDaemonEntries is empty: there is no daemon chip to show or hide.
func setvStatusBarDaemonEntries() []statusBarEntry { return nil }
