package ntr

import (
	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/keys"

	"github.com/codyconfer/mino/internal/plugin"
	"github.com/codyconfer/mino/internal/render/glyph"
)

func init() {
	plugin.RegisterView(PluginID, "ntr.home", func() vkdeck.View { return &HomeView{} })
	plugin.RegisterView(PluginID, "ntr.notes", func() vkdeck.View { return newNotesList("", "") })
	plugin.RegisterView(PluginID, "ntr.tasks", func() vkdeck.View { return newTasksList("", "") })
	plugin.RegisterView(PluginID, "ntr.reminders", func() vkdeck.View { return newRemindersList("", "") }, plugin.WithServiceOnly())
}

func RunTUI(home, role string) error {
	if role == "" {
		role = "default"
	}
	return vkdeck.Run(NewHomeView(home, role), vkdeck.WithChrome(vkdeck.Chrome{
		Brand:    "MINO",
		Subtitle: "notes",
	}), vkdeck.WithKeyMapQuit())
}

type HomeView struct {
	home string
	role string
	menu *vkdeck.Menu
}

func NewHomeView(home, role string) *HomeView {
	if role == "" {
		role = "default"
	}
	v := &HomeView{home: home, role: role}
	items := []vkdeck.MenuItem{
		{
			Label: "Notes",
			Desc:  "build, edit, and delete notes",
			Icon:  glyph.Notes(),
			Hue:   recordHue(kindNote),
			OnSelect: func(h *vkdeck.Model) tea.Cmd {
				return h.Push(newNotesList(home, role))
			},
		},
		{
			Label: "Tasks",
			Desc:  "build, edit, complete, and delete tasks",
			Icon:  glyph.Check(),
			Hue:   recordHue(kindTask),
			OnSelect: func(h *vkdeck.Model) tea.Cmd {
				return h.Push(newTasksList(home, role))
			},
		},
	}
	if plugin.ViewUIVisible("ntr.reminders") {
		items = append(items, vkdeck.MenuItem{
			Label: "Reminders",
			Desc:  "build, edit, and complete reminders",
			Icon:  glyph.Clock(),
			Hue:   recordHue(kindReminder),
			OnSelect: func(h *vkdeck.Model) tea.Cmd {
				return h.Push(newRemindersList(home, role))
			},
		})
	}
	v.menu = vkdeck.NewMenu("notes", []keys.Hint{{Key: "role", Label: role}}, items...)
	return v
}

func RemindersUIVisible() bool {
	return plugin.ViewUIVisible("ntr.reminders")
}

func NewNoteBuilder(home, role string) vkdeck.View {
	return newNoteView(home, role, record{Kind: kindNote}, nil)
}

func NewTaskBuilder(home, role string) vkdeck.View {
	return newTaskView(home, role, record{Kind: kindTask}, nil)
}

func NewRemindBuilder(home, role string) vkdeck.View {
	return newRemindView(home, role, record{Kind: kindReminder}, nil)
}

func (v *HomeView) Title() string                             { return v.menu.Title() }
func (v *HomeView) Init() tea.Cmd                             { return v.menu.Init() }
func (v *HomeView) Update(h *vkdeck.Model, m tea.Msg) tea.Cmd { return v.menu.Update(h, m) }
func (v *HomeView) Body(w, ht int) string                     { return v.menu.Body(w, ht) }
func (v *HomeView) Hints() []keys.Hint                        { return v.menu.Hints() }
func (v *HomeView) Context() []keys.Hint                      { return v.menu.Context() }
