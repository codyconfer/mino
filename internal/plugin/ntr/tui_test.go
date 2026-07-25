package ntr

import (
	"strings"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/munin/internal/deck"
	"github.com/codyconfer/munin/internal/plugin"
	pub "github.com/codyconfer/munin/plugin"
)

func TestParseDueRelative(t *testing.T) {
	got, err := parseDue("+2h")
	if err != nil {
		t.Fatal(err)
	}
	if d := time.Until(got); d < time.Hour || d > 3*time.Hour {
		t.Fatalf("until = %v", d)
	}
}

func TestParseDueBad(t *testing.T) {
	if _, err := parseDue("not-a-date"); err == nil {
		t.Fatal("expected error")
	}
}

func TestNTRViewsRegistered(t *testing.T) {
	for _, id := range []string{"ntr.home", "ntr.notes", "ntr.tasks", "ntr.reminders"} {
		if _, ok := vkdeck.LookupView(id); !ok {
			t.Fatalf("missing view %s (have %v)", id, vkdeck.ViewIDs())
		}
	}
	if !strings.Contains(strings.Join(vkdeck.ViewIDs(), ","), "ntr.") {
		t.Fatal(vkdeck.ViewIDs())
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
	if !strings.Contains(got, "Reminders") {
		t.Fatalf("home menu missing Reminders while attached: %q", got)
	}
}

func step(a *vkdeck.Model, msg tea.Msg) *vkdeck.Model {
	m, _ := a.Update(msg)
	return m.(*vkdeck.Model)
}
