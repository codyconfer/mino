package views

import (
	"github.com/codyconfer/viewkit/forms"

	"github.com/codyconfer/mino/internal/config"
)

type StatusBarEntry struct {
	ID    string
	Label string
}

type SettingsSection struct {
	Fields    func(*config.Config) []forms.Field
	Values    func(vals, set map[string]any)
	StatusBar []StatusBarEntry
}

var settingsSections []SettingsSection

func RegisterSettingsSection(s SettingsSection) {
	settingsSections = append(settingsSections, s)
}

func SettingsSections() []SettingsSection { return settingsSections }

func sectionFields(c *config.Config) []forms.Field {
	var out []forms.Field
	for _, s := range settingsSections {
		if s.Fields == nil {
			continue
		}
		out = append(out, s.Fields(c)...)
	}
	return out
}

func sectionValues(vals, set map[string]any) {
	for _, s := range settingsSections {
		if s.Values == nil {
			continue
		}
		s.Values(vals, set)
	}
}

func sectionStatusBarEntries() []statusBarEntry {
	var out []statusBarEntry
	for _, s := range settingsSections {
		for _, e := range s.StatusBar {
			out = append(out, statusBarEntry{e.ID, e.Label})
		}
	}
	return out
}
