package deck

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	vkdeck "github.com/codyconfer/viewkit/deck"
	vkglyph "github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/munin/internal/plugin"
)

func TestPluginServicesAppearInStatusStrip(t *testing.T) {
	id := "test.deck.status"
	marker := "DECK-MARK"
	if _, ok := plugin.Lookup(id); !ok {
		plugin.Register(plugin.Descriptor{
			ID:           id,
			Kind:         plugin.KindSignal,
			Signal:       "testdeckstatus",
			Capabilities: []plugin.Capability{plugin.CapQuery},
		})
	}
	plugin.RegisterStatusContribution(id, func(_, _ string) vkglyph.StatusContribution {
		return vkglyph.StatusContribution{
			Info:   func() string { return "deckplug" },
			Status: func() (string, vkglyph.Severity) { return marker, vkglyph.SeverityPositive },
		}
	})

	svcs := PluginServices("", "")
	found := false
	for _, s := range svcs {
		if s.Name == "deckplug" && s.Glyph == marker {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("PluginServices missing deckplug/%s: %+v", marker, svcs)
	}

	info := StatusInfo{
		GitHubUser: "cody",
		Services:   append([]ServiceStatus{{Name: "github", Level: StatusOK}}, svcs...),
	}
	menu := vkdeck.NewMenu("main", nil, vkdeck.MenuItem{Label: "Alpha"})
	app := New(menu, WithStatus(func(context.Context) StatusInfo { return info }))
	app = drive(app, tea.WindowSizeMsg{Width: 120, Height: 40})
	app.SetStatus(adaptStatus(info))
	view := ansi.Strip(app.View())
	if !strings.Contains(view, "deckplug") || !strings.Contains(view, marker) {
		t.Fatalf("status chrome missing plugin contrib\n---\n%s", view)
	}
}
