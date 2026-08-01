package plugin_test

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/glyph"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/mino/plugin"
)

func mustNotPanic(t *testing.T, what string, fn func()) {
	t.Helper()
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("%s panicked (%v): a bad argument from one plugin must not abort registration", what, r)
		}
	}()
	fn()
}

type guardView struct{}

func (guardView) Title() string                       { return "guard" }
func (guardView) Init() tea.Cmd                       { return nil }
func (guardView) Update(*deck.Model, tea.Msg) tea.Cmd { return nil }
func (guardView) Body(int, int) string                { return "body" }
func (guardView) Hints() []keys.Hint                  { return nil }
func (guardView) Context() []keys.Hint                { return nil }

func TestRegisterThemeWithoutKeyIsDiagnosticNotPanic(t *testing.T) {
	const id = "guard.theme.nokey"
	mustNotPanic(t, "RegisterTheme", func() {
		plugin.RegisterTheme(id, "", "No Key", theme.Palette{Text: "15"})
	})
	findDiagnostic(t, id, "RegisterTheme")
}

func TestRegisterViewBadArgsIsDiagnosticNotPanic(t *testing.T) {
	ctor := func() deck.View { return guardView{} }

	mustNotPanic(t, "RegisterView(no parent)", func() {
		plugin.RegisterView("", "guardviewnoparent", ctor)
	})
	mustNotPanic(t, "RegisterView(no viewID)", func() {
		plugin.RegisterView("guard.view.noviewid", "", ctor)
	})
	mustNotPanic(t, "RegisterView(nil ctor)", func() {
		plugin.RegisterView("guard.view.noctor", "guardviewnoctor", nil)
	})

	findDiagnostic(t, "guard.view.noviewid", "RegisterView")
	findDiagnostic(t, "guard.view.noctor", "RegisterView")
	if plugin.HasView("guardviewnoparent") {
		t.Error("RegisterView with no parent id still installed the deck view")
	}
	if plugin.HasView("guardviewnoctor") {
		t.Error("RegisterView with a nil ctor still installed the deck view")
	}
}

func TestRegisterStatusContributionBadArgsIsDiagnosticNotPanic(t *testing.T) {
	factory := func(string, string) glyph.StatusContribution { return glyph.StatusContribution{} }

	mustNotPanic(t, "RegisterStatusContribution(no id)", func() {
		plugin.RegisterStatusContribution("", factory)
	})
	mustNotPanic(t, "RegisterStatusContribution(nil factory)", func() {
		plugin.RegisterStatusContribution("guard.status.nofactory", nil)
	})

	findDiagnostic(t, "guard.status.nofactory", "RegisterStatusContribution")
	for _, id := range plugin.StatusContributionIDs() {
		if id == "" || id == "guard.status.nofactory" {
			t.Errorf("StatusContributionIDs kept an invalid entry %q", id)
		}
	}
}

type guardProvider struct {
	tool     string
	id       string
	switched *string
}

func (p *guardProvider) Tool() string { return p.tool }

func (p *guardProvider) Switch(_ context.Context, _ string) error {
	if p.switched != nil {
		*p.switched = p.id
	}
	return nil
}

func (p *guardProvider) Current(context.Context) (string, bool, error) {
	return p.id, true, nil
}

func TestRegisterContextBadArgsIsDiagnosticNotPanic(t *testing.T) {
	mustNotPanic(t, "RegisterContext(no parent)", func() {
		plugin.RegisterContext("", &guardProvider{tool: "guardctxnoparent", id: "orphan"})
	})
	mustNotPanic(t, "RegisterContext(nil provider)", func() {
		plugin.RegisterContext("guard.ctx.nilprovider", nil)
	})
	mustNotPanic(t, "RegisterContext(no tool)", func() {
		plugin.RegisterContext("guard.ctx.notool", &guardProvider{tool: "", id: "notool"})
	})

	findDiagnostic(t, "guard.ctx.nilprovider", "RegisterContext")
	findDiagnostic(t, "guard.ctx.notool", "RegisterContext")
	if plugin.HasContextProvider("guardctxnoparent") {
		t.Error("RegisterContext with no parent id still installed the live provider")
	}
}

func TestRegisterContextCollisionKeepsIncumbentProvider(t *testing.T) {
	const tool = "guardctxtool"
	const first = "guard.ctx.first"
	const second = "guard.ctx.second"

	plugin.Register(plugin.Descriptor{ID: first, Kind: plugin.KindSignal, Signal: "guardctxfirst"})
	plugin.Register(plugin.Descriptor{ID: second, Kind: plugin.KindSignal, Signal: "guardctxsecond"})

	var ran string
	plugin.RegisterContext(first, &guardProvider{tool: tool, id: "incumbent", switched: &ran})
	plugin.RegisterContext(second, &guardProvider{tool: tool, id: "EVIL", switched: &ran})

	d, ok := plugin.ByKind(plugin.KindContext, tool)
	if !ok || d.ID != first+"/context/"+tool {
		t.Fatalf("ByKind(KindContext, %q) = %+v, %v; want the incumbent", tool, d, ok)
	}

	cur, ok := plugin.CurrentContext(context.Background(), tool)
	if !ok || cur.Name != "incumbent" {
		t.Fatalf("CurrentContext = %+v, %v; the live provider is not the registered owner", cur, ok)
	}

	if err := plugin.SwitchContext(context.Background(), tool, "target"); err != nil {
		t.Fatalf("SwitchContext: %v", err)
	}
	if ran != "incumbent" {
		t.Fatalf("Switch executed by provider %q; a later plugin hijacked tool %q from %q", ran, tool, first)
	}

	findDiagnostic(t, second, tool, first)
}

func TestDisabledPluginContextProviderIsRevoked(t *testing.T) {
	const tool = "guardctxdisabled"
	const owner = "guard.ctx.disabled"

	plugin.Register(plugin.Descriptor{ID: owner, Kind: plugin.KindSignal, Signal: "guardctxdisabledsig"})

	var ran string
	plugin.RegisterContext(owner, &guardProvider{tool: tool, id: owner, switched: &ran})

	plugin.SetPluginEnabledFunc(func(id string) bool { return id != owner+"/context/"+tool })
	t.Cleanup(func() { plugin.SetPluginEnabledFunc(nil) })

	if err := plugin.SwitchContext(context.Background(), tool, "target"); err == nil {
		t.Fatal("SwitchContext succeeded for a disabled plugin: disabling does not revoke the provider")
	}
	if ran != "" {
		t.Fatalf("a disabled plugin's Switch still executed (ran=%q)", ran)
	}
	if err := plugin.ApplyRoleContexts(context.Background(), map[string]string{tool: "target"}); err == nil {
		t.Fatal("ApplyRoleContexts silently ran a disabled plugin's provider")
	}
	if ran != "" {
		t.Fatalf("a disabled plugin's Switch still executed via ApplyRoleContexts (ran=%q)", ran)
	}
}
