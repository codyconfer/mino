package ntr

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/mino/internal/deck"
	"github.com/codyconfer/mino/internal/plugin"
	pub "github.com/codyconfer/mino/plugin"
)

func TestNTRViewsRegistered(t *testing.T) {
	for _, id := range []string{"ntr.home", "ntr.notes", "ntr.tasks", "ntr.reminders"} {
		if _, ok := vkdeck.NamedView(id); !ok {
			t.Fatalf("missing view %s (have %v)", id, vkdeck.ViewKeys())
		}
	}
	if !strings.Contains(strings.Join(vkdeck.ViewKeys(), ","), "ntr.") {
		t.Fatal(vkdeck.ViewKeys())
	}
}

func TestRemindersViewServiceOnly(t *testing.T) {
	d, ok := plugin.ByKind(plugin.KindView, "ntr.reminders")
	if !ok {
		t.Fatal("missing ntr.reminders descriptor")
	}
	if !d.ServiceOnly {
		t.Fatalf("ntr.reminders ServiceOnly = false, want true: %+v", d)
	}

	pub.SetServiceAttachedFunc(func() bool { return false })
	t.Cleanup(func() { pub.SetServiceAttachedFunc(plugin.ServiceAttached) })
	if RemindersUIVisible() {
		t.Fatal("reminders should be hidden when service detached")
	}

	v := NewHomeView(t.TempDir(), "default")
	app := deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	got := app.View()
	if strings.Contains(got, "Reminders") {
		t.Fatalf("home menu showed Reminders while detached: %q", got)
	}
	for _, want := range []string{"Notes", "Tasks"} {
		if !strings.Contains(got, want) {
			t.Fatalf("home menu missing %q while detached: %q", want, got)
		}
	}

	pub.SetServiceAttachedFunc(func() bool { return true })
	if !RemindersUIVisible() {
		t.Fatal("reminders should be visible when service attached")
	}
	v = NewHomeView(t.TempDir(), "default")
	app = deck.New(v)
	app = step(app, tea.WindowSizeMsg{Width: 100, Height: 40})
	got = app.View()
	for _, want := range []string{"Notes", "Tasks", "Reminders"} {
		if !strings.Contains(got, want) {
			t.Fatalf("home menu missing %q while attached: %q", want, got)
		}
	}
}

func TestBuildersTitle(t *testing.T) {
	home := t.TempDir()
	cases := []struct {
		name string
		view vkdeck.View
		want string
	}{
		{"note", NewNoteBuilder(home, "r", testScheme()), "build note"},
		{"task", NewTaskBuilder(home, "r", testScheme()), "build task"},
		{"reminder", NewRemindBuilder(home, "r", testScheme()), "build reminder"},
	}
	for _, c := range cases {
		if got := c.view.Title(); got != c.want {
			t.Errorf("%s builder title = %q, want %q", c.name, got, c.want)
		}
	}
}

func step(a *vkdeck.Model, msg tea.Msg) *vkdeck.Model {
	m, _ := a.Update(msg)
	return m.(*vkdeck.Model)
}

func update(a *vkdeck.Model, msg tea.Msg) (*vkdeck.Model, tea.Cmd) {
	m, cmd := a.Update(msg)
	return m.(*vkdeck.Model), cmd
}

func cmdOf(_ *vkdeck.Model, cmd tea.Cmd) tea.Cmd { return cmd }

func flattenCmds(cmd tea.Cmd) []tea.Cmd {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if batch, ok := msg.(tea.BatchMsg); ok {
		return batch
	}
	return []tea.Cmd{func() tea.Msg { return msg }}
}

func settle(a *vkdeck.Model, cmd tea.Cmd) *vkdeck.Model {
	return settleDepth(a, cmd, 8)
}

func settleDepth(a *vkdeck.Model, cmd tea.Cmd, depth int) *vkdeck.Model {
	if depth == 0 {
		return a
	}
	for _, c := range flattenCmds(cmd) {
		if c == nil {
			continue
		}
		msg := c()
		if msg == nil {
			continue
		}
		next, more := update(a, msg)
		a = settleDepth(next, more, depth-1)
	}
	return a
}
