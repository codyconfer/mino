package deck

import (
	"context"
	"os"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/ansi"

	vkdeck "github.com/codyconfer/viewkit/deck"
	vkglyph "github.com/codyconfer/viewkit/glyph"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/mino/internal/keymap"
	"github.com/codyconfer/mino/internal/render/glyph"
)

func TestMain(m *testing.M) {
	keymap.Register()
	os.Exit(m.Run())
}

func drive(a *vkdeck.Model, msg tea.Msg) *vkdeck.Model {
	m, _ := a.Update(msg)
	return m.(*vkdeck.Model)
}

func key(s string) tea.KeyMsg {
	if s == "enter" {
		return tea.KeyMsg{Type: tea.KeyEnter}
	}
	if s == "down" {
		return tea.KeyMsg{Type: tea.KeyDown}
	}
	if s == "esc" {
		return tea.KeyMsg{Type: tea.KeyEsc}
	}
	return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
}

func TestAppRendersChromeAndMenu(t *testing.T) {
	menu := vkdeck.NewMenu("", []keys.Hint{{Key: "role", Label: "triage"}},
		vkdeck.MenuItem{Label: "Alpha", Desc: "first"},
		vkdeck.MenuItem{Label: "Beta", Desc: "second", OnSelect: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(vkdeck.NewMessage("beta screen", "hello from beta", nil))
		}},
	)
	app := New(menu)
	app = drive(app, tea.WindowSizeMsg{Width: 100, Height: 40})

	view := app.View()
	for _, want := range []string{"MINO", "netrunner", "role", "triage", "Alpha", "Beta", "quit"} {
		if !strings.Contains(view, want) {
			t.Errorf("main menu frame missing %q\n---\n%s", want, view)
		}
	}
	if strings.Contains(view, "MAIN MENU") {
		t.Errorf("main menu should omit title label\n---\n%s", view)
	}
}

func TestAppNavigation(t *testing.T) {
	menu := vkdeck.NewMenu("main", nil,
		vkdeck.MenuItem{Label: "Alpha"},
		vkdeck.MenuItem{Label: "Beta", OnSelect: func(a *vkdeck.Model) tea.Cmd {
			return a.Push(vkdeck.NewMessage("beta screen", "body", nil))
		}},
	)
	app := New(menu)
	app = drive(app, tea.WindowSizeMsg{Width: 100, Height: 40})

	app = drive(app, key("down"))
	app = drive(app, key("enter"))
	if got := app.Top().Title(); got != "beta screen" {
		t.Fatalf("after enter, top = %q, want beta screen", got)
	}
	if !strings.Contains(app.View(), "BETA SCREEN") {
		t.Errorf("pushed view frame missing title:\n%s", app.View())
	}

	app = drive(app, key("esc"))
	if got := app.Top().Title(); got != "main" {
		t.Fatalf("after esc, top = %q, want main", got)
	}
}

func TestAppRendersStatusFromProvider(t *testing.T) {
	info := StatusInfo{
		GitHubUser:      "cody",
		SigningVerified: true,
		Services: []ServiceStatus{
			{Name: "github", Detail: "4998/5000", Severity: vkglyph.SeverityPositive},
			{Name: "slack", Severity: vkglyph.SeverityPositive},
			{ID: "google", Name: "google", Severity: vkglyph.SeverityNeutral},
		},
	}
	menu := vkdeck.NewMenu("main", nil, vkdeck.MenuItem{Label: "Alpha"})
	app := New(menu, WithStatus(nil, func(context.Context) StatusInfo { return info }))
	app = drive(app, tea.WindowSizeMsg{Width: 100, Height: 40})

	if strings.Contains(app.View(), "@cody") {
		t.Fatal("identity rendered before status was fetched")
	}

	app.SetStatus(adaptStatus(nil, info))
	view := app.View()
	ghChip := glyph.Lead(glyph.GitHub()) + "4998/5000"
	for _, want := range []string{"@cody", glyph.SigningOK(), ghChip, glyph.Slack(), glyph.Google()} {
		if !strings.Contains(view, want) {
			t.Errorf("status chrome missing %q\n---\n%s", want, view)
		}
	}

	lines := strings.Split(view, "\n")
	statusIdx, hintIdx := -1, -1
	for i, ln := range lines {
		plain := ansi.Strip(ln)
		if strings.Contains(plain, ghChip) {
			statusIdx = i
		}
		if strings.Contains(plain, "quit") {
			hintIdx = i
		}
	}
	if statusIdx < 0 || hintIdx < 0 {
		t.Fatalf("missing status strip or key legend:\n%s", view)
	}
	if statusIdx >= hintIdx {
		t.Errorf("expected the status strip directly above the key legend:\n%s", view)
	}
	if hintIdx != statusIdx+3 {
		t.Errorf("expected the key legend below the padded status bar, got status=%d hint=%d:\n%s", statusIdx, hintIdx, view)
	}
}

func TestAppUnverifiedSigningGlyph(t *testing.T) {
	info := StatusInfo{GitHubUser: "cody", SigningVerified: false}
	app := New(vkdeck.NewMenu("main", nil, vkdeck.MenuItem{Label: "Alpha"}),
		WithStatus(nil, func(context.Context) StatusInfo { return info }))
	app = drive(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	app.SetStatus(adaptStatus(nil, info))
	if !strings.Contains(app.View(), glyph.SigningBad()) {
		t.Errorf("expected unverified glyph %q in header:\n%s", glyph.SigningBad(), app.View())
	}
}

func TestAppHeaderBreadcrumbs(t *testing.T) {
	menu := vkdeck.NewMenu("main", nil, vkdeck.MenuItem{Label: "Alpha", OnSelect: func(a *vkdeck.Model) tea.Cmd {
		return a.Push(vkdeck.NewMessage("details", "body", nil))
	}})
	app := New(menu)
	app = drive(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	app = drive(app, key("enter"))

	lines := strings.Split(app.View(), "\n")
	brandIdx, crumbIdx := -1, -1
	for i, ln := range lines {
		p := ansi.Strip(ln)
		if strings.Contains(p, "MINO") {
			brandIdx = i
		}
		if strings.Contains(p, "main") && strings.Contains(p, "details") {
			crumbIdx = i
		}
	}
	if brandIdx < 0 || crumbIdx < 0 {
		t.Fatalf("expected a breadcrumb line (main ⟩ details) below the brand line:\n%s", app.View())
	}
	if crumbIdx != brandIdx+1 {
		t.Errorf("expected the breadcrumb strip directly below the brand line, got brand=%d crumb=%d:\n%s", brandIdx, crumbIdx, app.View())
	}
}

func TestAppPinsFooterToBottom(t *testing.T) {
	const height = 40
	menu := vkdeck.NewMenu("main", nil, vkdeck.MenuItem{Label: "Alpha"})
	app := New(menu)
	app = drive(app, tea.WindowSizeMsg{Width: 100, Height: height})

	lines := strings.Split(app.View(), "\n")
	if len(lines) < height-1 {
		t.Fatalf("view fills only %d lines, want it to reach the bottom (~%d)", len(lines), height-1)
	}
	footerIdx := -1
	for i, ln := range lines {
		if strings.Contains(ansi.Strip(ln), "quit") {
			footerIdx = i
		}
	}
	if footerIdx < 0 {
		t.Fatalf("footer hint line not found:\n%s", app.View())
	}
	if gap := len(lines) - 1 - footerIdx; gap > 1 {
		t.Errorf("footer sits %d lines above the bottom, want it pinned", gap)
	}
}

func TestAppEvenHorizontalMargins(t *testing.T) {
	const width = 100
	menu := vkdeck.NewMenu("", []keys.Hint{{Key: "role", Label: "triage"}},
		vkdeck.MenuItem{Label: "Alpha", Desc: "first"},
		vkdeck.MenuItem{Label: "Beta", Desc: "second"},
	)
	app := New(menu)
	app = drive(app, tea.WindowSizeMsg{Width: width, Height: 40})

	sawRule := false
	for i, ln := range strings.Split(app.View(), "\n") {
		plain := ansi.Strip(ln)
		if w := ansi.StringWidth(plain); w != width {
			t.Errorf("line %d fills %d cols, want %d: %q", i, w, width, plain)
		}
		if strings.ContainsAny(plain, "─╰╯") {
			sawRule = true
			if lead := len(plain) - len(strings.TrimLeft(plain, " ")); lead != theme.AppMarginX {
				t.Errorf("line %d left inset = %d, want %d: %q", i, lead, theme.AppMarginX, plain)
			}
			if trail := len(plain) - len(strings.TrimRight(plain, " ")); trail != theme.AppMarginX {
				t.Errorf("line %d right inset = %d, want %d: %q", i, trail, theme.AppMarginX, plain)
			}
		}
	}
	if !sawRule {
		t.Fatal("no full-width rule line found to check margins")
	}
}
