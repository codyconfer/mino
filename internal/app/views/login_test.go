package views

import (
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"

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

func TestLoginFlowStepSelection(t *testing.T) {
	kit := testKit(t)
	p, ok := loginflow.Resolve("github")
	if !ok {
		t.Fatal("github provider not found")
	}

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
