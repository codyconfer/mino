package views

import (
	"context"
	"io"
	"regexp"
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/app/loginflow"
	"github.com/codyconfer/munin/internal/deck"
)

var loginAnsi = regexp.MustCompile("\x1b\\[[0-9;]*m")

func TestLoginMenuSmoke(t *testing.T) {
	kit := testKit(t)
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("login menu panicked: %v", r)
		}
	}()
	a := deck.New(kit.Login())
	a = step(a, tea.WindowSizeMsg{Width: 100, Height: 40})
	_ = a.View()
	a = step(a, tea.KeyMsg{Type: tea.KeyEnter})
	_ = a.View()
	a = step(a, tea.KeyMsg{Type: tea.KeyEsc})
	_ = a.View()
}

func TestLoginAlreadyAuthedStaysOnLoginWithToast(t *testing.T) {
	kit := testKit(t)
	page := kit.Login().(*loginPage)
	a := deck.New(page)
	a = step(a, tea.WindowSizeMsg{Width: 100, Height: 40})
	a = step(a, loginAlreadyAuthedMsg{label: "GitHub"})

	if !page.toast.Queue().Active() {
		t.Fatal("expected toast queue to be active after already-authed msg")
	}
	n, ok := page.toast.Queue().Current()
	if !ok || n.Message != "GitHub already authorized" {
		t.Fatalf("toast = %+v ok=%v", n, ok)
	}
	if a.Top().Title() != "accounts" {
		t.Fatalf("top view = %q, want accounts page", a.Top().Title())
	}
	body := loginAnsi.ReplaceAllString(a.View(), "")
	if !strings.Contains(body, "already authorized") {
		t.Fatalf("view missing toast copy:\n%s", body)
	}
}

func TestLoginFlowAlreadyAuthedRedirects(t *testing.T) {
	kit := testKit(t)
	page := kit.Login().(*loginPage)
	p := loginflow.Provider{
		Key:    "github",
		Label:  "GitHub",
		Authed: func(*app.App) bool { return true },
	}
	flow := kit.loginFlow(p).(*loginFlowView)

	a := deck.New(page)
	a = step(a, tea.WindowSizeMsg{Width: 100, Height: 40})
	_ = a.Push(flow)
	if a.Top().Title() != "accounts: github" {
		t.Fatalf("top = %q after push", a.Top().Title())
	}

	msg := flow.Init()()
	_ = flow.Update(a, msg)
	if a.Top().Title() != "accounts" {
		t.Fatalf("top = %q, want accounts after redirect", a.Top().Title())
	}
	_ = step(a, msg)
	if !page.toast.Queue().Active() {
		t.Fatal("expected login page toast after redirect")
	}
}

func TestLoginFlowStepSelection(t *testing.T) {
	kit := testKit(t)
	p, ok := loginflow.Resolve("github")
	if !ok {
		t.Fatal("github provider not found")
	}
	p.Authed = func(*app.App) bool { return false }

	v := kit.loginFlow(p).(*loginFlowView)
	if v.step != loginStepForm || v.form == nil {
		t.Fatalf("expected credential form step, got step=%d form=%v", v.step, v.form)
	}

	kit.d.App.Cfg.GitHub.OAuthClientID = "client-id"
	v = kit.loginFlow(p).(*loginFlowView)
	if v.step != loginStepRun || v.form != nil {
		t.Fatalf("expected run step, got step=%d form=%v", v.step, v.form)
	}
}

func TestLoginRunStepEscCancelsTheLogin(t *testing.T) {
	kit := testKit(t)
	started := make(chan context.Context, 1)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })

	p := loginflow.Provider{
		Key:    "github",
		Label:  "GitHub",
		Authed: func(*app.App) bool { return false },
		Login: func(ctx context.Context, _ *app.App, _ map[string]string, _ io.Writer) error {
			started <- ctx
			select {
			case <-ctx.Done():
			case <-release:
			}
			return ctx.Err()
		},
	}

	v := kit.loginFlow(p).(*loginFlowView)
	if v.step != loginStepRun {
		t.Fatalf("step = %d, want the device-flow run step", v.step)
	}

	a := deck.New(vkdeck.NewMenu("accounts", nil))
	a = step(a, tea.WindowSizeMsg{Width: 100, Height: 40})
	_ = a.Push(v)

	var ctx context.Context
	select {
	case ctx = <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("login never started")
	}

	_ = v.Update(a, tea.KeyMsg{Type: tea.KeyEsc})

	select {
	case <-ctx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("esc during the run step did not cancel the login context")
	}
	if a.Top() == vkdeck.View(v) {
		t.Fatal("esc during the run step did not leave the login view")
	}
}

func TestLoginFlowMasksClientSecret(t *testing.T) {
	kit := testKit(t)
	p, _ := loginflow.Resolve("google")
	v := kit.loginFlow(p).(*loginFlowView)
	a := deck.New(v)
	a = step(a, tea.WindowSizeMsg{Width: 100, Height: 40})

	a = step(a, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("id-123")})
	a = step(a, tea.KeyMsg{Type: tea.KeyDown})
	step(a, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("sekret")})

	body := loginAnsi.ReplaceAllString(v.Body(100, 40), "")
	if strings.Contains(body, "sekret") {
		t.Errorf("client secret leaked in form body:\n%s", body)
	}
	if !strings.Contains(body, strings.Repeat("•", len("sekret"))) {
		t.Errorf("client secret not masked with bullets:\n%s", body)
	}
	if !strings.Contains(body, "id-123") {
		t.Errorf("non-secret client id should be visible:\n%s", body)
	}
}
