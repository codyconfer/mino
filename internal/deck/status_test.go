package deck

import (
	"context"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	vkdeck "github.com/codyconfer/viewkit/deck"
	vkglyph "github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/plugin"
	"github.com/codyconfer/mino/internal/render/glyph"
	"github.com/codyconfer/mino/internal/role"
	"github.com/codyconfer/mino/internal/testenv"
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
		Services:   append([]ServiceStatus{{Name: "github", Severity: vkglyph.SeverityPositive}}, svcs...),
	}
	menu := vkdeck.NewMenu("main", nil, vkdeck.MenuItem{Label: "Alpha"})
	app := New(menu, WithStatus(nil, func(context.Context) StatusInfo { return info }))
	app = drive(app, tea.WindowSizeMsg{Width: 120, Height: 40})
	app.SetStatus(adaptStatus(nil, info))
	view := ansi.Strip(app.View())
	if !strings.Contains(view, "deckplug") || !strings.Contains(view, marker) {
		t.Fatalf("status chrome missing plugin contrib\n---\n%s", view)
	}
}

func TestPluginServicesPrefersBrandGlyph(t *testing.T) {
	id := "test.deck.brandglyph"
	logo := "BRAND-LOGO"
	if _, ok := plugin.Lookup(id); !ok {
		plugin.Register(plugin.Descriptor{
			ID:           id,
			Kind:         plugin.KindSignal,
			Signal:       "testbrandglyph",
			Capabilities: []plugin.Capability{plugin.CapQuery},
		})
	}
	plugin.RegisterStatusContribution(id, func(_, _ string) vkglyph.StatusContribution {
		return vkglyph.StatusContribution{
			BrandGlyph: logo,
			Info:       func() string { return "plaintext" },
			Status:     func() (string, vkglyph.Severity) { return "OK", vkglyph.SeverityPositive },
		}
	})

	for _, s := range PluginServices("", "") {
		if s.Name == logo {
			return
		}
	}
	t.Fatalf("expected BrandGlyph %q as service name, got %+v", logo, PluginServices("", ""))
}

func TestServiceChipLogoDetailSpacing(t *testing.T) {
	name, detail := serviceChip("github", "1/2")
	want := glyph.Lead(glyph.GitHub()) + "1/2"
	if name != want || detail != "" {
		t.Fatalf("logo+detail = (%q, %q), want (%q, \"\")", name, detail, want)
	}
	name, detail = serviceChip("google", "")
	if name != glyph.Google() || detail != "" {
		t.Fatalf("logo-only = (%q, %q), want (%q, \"\")", name, detail, glyph.Google())
	}
	name, detail = serviceChip("daemon", "running")
	if name != "daemon" || detail != "running" {
		t.Fatalf("text+detail = (%q, %q), want (\"daemon\", \"running\")", name, detail)
	}
}

func TestAdaptStatusUsesToolLogos(t *testing.T) {
	info := StatusInfo{Services: []ServiceStatus{
		{Name: "github", Detail: "1/2", Severity: vkglyph.SeverityPositive},
		{Name: "slack", Severity: vkglyph.SeverityPositive},
		{ID: "google", Name: "google", Severity: vkglyph.SeverityNeutral},
	}}
	got := adaptStatus(nil, info)
	if len(got.Services) != 3 {
		t.Fatalf("services = %d, want 3", len(got.Services))
	}
	wantGH := glyph.Lead(glyph.GitHub()) + "1/2"
	if got.Services[0].Name != wantGH || got.Services[0].Detail != "" {
		t.Errorf("github chip = %+v, want Name=%q Detail=\"\"", got.Services[0], wantGH)
	}
	if got.Services[1].Name != glyph.Slack() {
		t.Errorf("slack chip = %+v", got.Services[1])
	}
	if got.Services[2].Name != glyph.Google() || got.Services[2].Detail != "" {
		t.Errorf("google chip = %+v, want logo-only %q", got.Services[2], glyph.Google())
	}
}

func TestAdaptStatusHidesConfiguredEntries(t *testing.T) {
	testenv.Isolate(t)
	if err := config.SetHiddenStatusBar([]string{"slack", "mino.hide.test"}); err != nil {
		t.Fatalf("SetHiddenStatusBar: %v", err)
	}

	info := StatusInfo{Services: []ServiceStatus{
		{Name: "github", Severity: vkglyph.SeverityPositive},
		{Name: "slack", Severity: vkglyph.SeverityPositive},
		{ID: "mino.hide.test", Name: "SECRET-LOGO", Severity: vkglyph.SeverityPositive, Glyph: "G"},
		{ID: "mino.show.test", Name: "notes", Severity: vkglyph.SeverityPositive, Glyph: "N"},
	}}
	got := adaptStatus(nil, info)
	if len(got.Services) != 2 {
		t.Fatalf("services = %d, want 2 (hidden filtered): %+v", len(got.Services), got.Services)
	}
	if got.Services[0].Name != glyph.GitHub() {
		t.Errorf("first chip = %+v, want github logo", got.Services[0])
	}
	wantNotes := serviceLabel("notes")
	if got.Services[1].Name != wantNotes {
		t.Errorf("second chip = %+v, want %q", got.Services[1], wantNotes)
	}
}

func TestAdaptStatusHidesGoogleViaLegacyKey(t *testing.T) {
	testenv.Isolate(t)
	gs := config.LoadGlobalSettings()
	gs.HiddenStatusBar = []string{"gmail"}
	if err := config.SaveGlobalSettings(gs); err != nil {
		t.Fatalf("SaveGlobalSettings: %v", err)
	}

	info := StatusInfo{Services: []ServiceStatus{
		{Name: "github", Severity: vkglyph.SeverityPositive},
		{ID: "google", Name: "google", Severity: vkglyph.SeverityPositive},
	}}
	got := adaptStatus(nil, info)
	if len(got.Services) != 1 || got.Services[0].Name != glyph.GitHub() {
		t.Fatalf("expected only github after legacy google hide, got %+v", got.Services)
	}
}

func TestRoleServicesAppearInStatusStrip(t *testing.T) {
	t.Cleanup(role.ClearStatusChips)
	role.SetStatusChips([]role.Chip{
		{Glyph: "github", Text: "triage-ctx", Index: 0},
	})
	svcs := RoleServices()
	if len(svcs) != 1 || svcs[0].ID != "role-status-0" || svcs[0].Name != "github" || svcs[0].Detail != "triage-ctx" {
		t.Fatalf("RoleServices = %+v", svcs)
	}

	info := StatusInfo{Services: append([]ServiceStatus{{Name: "slack", Severity: vkglyph.SeverityPositive}}, svcs...)}
	got := adaptStatus(nil, info)
	if len(got.Services) != 2 {
		t.Fatalf("adaptStatus services = %+v", got.Services)
	}
	roleSvc := got.Services[1]
	wantRole := glyph.Lead(glyph.GitHub()) + "triage-ctx"
	if roleSvc.Name != wantRole || roleSvc.Detail != "" {
		t.Fatalf("role chip = %+v, want Name=%q Detail=\"\"", roleSvc, wantRole)
	}

	menu := vkdeck.NewMenu("main", nil, vkdeck.MenuItem{Label: "Alpha"})
	app := New(menu, WithStatus(nil, func(context.Context) StatusInfo { return info }))
	app = drive(app, tea.WindowSizeMsg{Width: 120, Height: 40})
	app.SetStatus(adaptStatus(nil, info))
	view := ansi.Strip(app.View())
	if !strings.Contains(view, "triage-ctx") {
		t.Fatalf("status chrome missing role status text\n---\n%s", view)
	}
}

func TestPluginServicesSetsStableID(t *testing.T) {
	id := "test.deck.status.id"
	if _, ok := plugin.Lookup(id); !ok {
		plugin.Register(plugin.Descriptor{
			ID:           id,
			Kind:         plugin.KindSignal,
			Signal:       "testdeckstatusid",
			Capabilities: []plugin.Capability{plugin.CapQuery},
		})
	}
	plugin.RegisterStatusContribution(id, func(_, _ string) vkglyph.StatusContribution {
		return vkglyph.StatusContribution{
			BrandGlyph: "LOGO",
			Info:       func() string { return "plaintext" },
			Status:     func() (string, vkglyph.Severity) { return "OK", vkglyph.SeverityPositive },
		}
	})
	for _, s := range PluginServices("", "") {
		if s.ID == id && s.Name == "LOGO" {
			return
		}
	}
	t.Fatalf("PluginServices missing ID %q: %+v", id, PluginServices("", ""))
}
