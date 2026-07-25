package plugin_test

import (
	"context"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/munin/plugin"
)

func TestKnownKindsRouted(t *testing.T) {
	for _, k := range plugin.KnownKinds() {
		if !plugin.ValidKind(k) {
			t.Errorf("KnownKinds entry %q not ValidKind", k)
		}
	}
	if plugin.ValidKind(plugin.Kind("nope")) {
		t.Fatal("bogus kind should be invalid")
	}
}

func TestRegisterContextLinksKindContext(t *testing.T) {
	id := "test.kinds.ctx"
	tool := "test-kinds-tool"
	if _, ok := plugin.Lookup(id); !ok {
		plugin.Register(plugin.Descriptor{ID: id, Kind: plugin.KindSignal, Signal: "testkindssig", Capabilities: []plugin.Capability{plugin.CapQuery}})
	}
	p := &stubProvider{tool: tool}
	plugin.RegisterContext(id, p)

	d, ok := plugin.ByKind(plugin.KindContext, tool)
	if !ok {
		t.Fatal("expected KindContext descriptor")
	}
	if d.Parent != id || d.Ref != tool {
		t.Fatalf("descriptor = %+v", d)
	}
	if !plugin.HasContextProvider(tool) {
		t.Fatal("expected ContextProvider")
	}
	if err := plugin.SwitchContext(context.Background(), tool, "prod"); err != nil {
		t.Fatal(err)
	}
}

type stubProvider struct{ tool, cur string }

func (s *stubProvider) Tool() string { return s.tool }
func (s *stubProvider) Switch(_ context.Context, name string) error {
	s.cur = name
	return nil
}
func (s *stubProvider) Current(context.Context) (string, bool, error) {
	if s.cur == "" {
		return "", false, nil
	}
	return s.cur, true, nil
}

func TestRegisterActionLinksKindAction(t *testing.T) {
	id := "test.kinds.act"
	sig := "testkindsact"
	if _, ok := plugin.Lookup(id); !ok {
		plugin.Register(plugin.Descriptor{
			ID: id, Kind: plugin.KindSignal, Signal: sig,
			Capabilities: []plugin.Capability{plugin.CapQuery, plugin.CapAction},
		})
	}
	plugin.RegisterAction(sig, "ping", func(context.Context, map[string]string) error { return nil })

	d, ok := plugin.ByKind(plugin.KindAction, sig+"/ping")
	if !ok {
		t.Fatal("expected KindAction companion")
	}
	if d.Parent != id {
		t.Fatalf("parent = %q, want %q", d.Parent, id)
	}
	if _, ok := plugin.LookupAction(sig, "ping"); !ok {
		t.Fatal("expected action binding")
	}
}

type stubDeckView struct{}

func (stubDeckView) Title() string                       { return "stub" }
func (stubDeckView) Init() tea.Cmd                       { return nil }
func (stubDeckView) Update(*deck.Model, tea.Msg) tea.Cmd { return nil }
func (stubDeckView) Body(int, int) string                { return "body" }
func (stubDeckView) Hints() [][2]string                  { return nil }
func (stubDeckView) Context() [][2]string                { return nil }

func TestRegisterViewLinksKindView(t *testing.T) {
	id := "test.kinds.view"
	viewID := "test.kinds.view.home"
	if _, ok := plugin.Lookup(id); !ok {
		plugin.Register(plugin.Descriptor{ID: id, Kind: plugin.KindSignal, Signal: "testkindsview", Capabilities: []plugin.Capability{plugin.CapQuery}})
	}
	plugin.RegisterView(id, viewID, func() deck.View { return stubDeckView{} })

	d, ok := plugin.ByKind(plugin.KindView, viewID)
	if !ok {
		t.Fatal("expected KindView descriptor")
	}
	if d.Parent != id {
		t.Fatalf("parent = %q", d.Parent)
	}
	if !plugin.HasView(viewID) {
		t.Fatal("expected deck view")
	}
}

func TestRegisterThemeLinksKindTheme(t *testing.T) {
	key := "test-kinds-theme"
	plugin.RegisterTheme("", key, "Test", theme.Palette{Text: "15"})
	d, ok := plugin.ByKind(plugin.KindTheme, key)
	if !ok {
		t.Fatal("expected KindTheme descriptor")
	}
	if d.Parent != "" || d.ID != "theme."+key {
		t.Fatalf("descriptor = %+v", d)
	}
	if !plugin.HasTheme(key) {
		t.Fatal("expected theme registry entry")
	}
	found := false
	for _, p := range plugin.Primaries() {
		if p.ID == d.ID {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("KindTheme primary missing from Primaries")
	}
}

func TestSplitActionRef(t *testing.T) {
	sig, name, ok := plugin.SplitActionRef("ntr/note.add")
	if !ok || sig != "ntr" || name != "note.add" {
		t.Fatalf("got %q %q %v", sig, name, ok)
	}
	if _, _, ok := plugin.SplitActionRef("ntr"); ok {
		t.Fatal("expected failure without slash")
	}
}
