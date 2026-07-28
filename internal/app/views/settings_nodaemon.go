//go:build nodaemon

package views

import (
	"github.com/codyconfer/viewkit/forms"

	"github.com/codyconfer/munin/internal/config"
)

func setvDaemonFields(*config.Config) []forms.Field { return nil }

func setvDaemonValues(_, _ map[string]any) {}

func setvStatusBarDaemonEntries() []statusBarEntry { return nil }
