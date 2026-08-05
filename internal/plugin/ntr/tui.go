package ntr

import (
	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/ui"

	"github.com/codyconfer/mino/internal/plugin"
	"github.com/codyconfer/mino/internal/render/glyph"
)

const BucketsViewID = "ntr.buckets"

const remindersViewID = "ntr.reminders"

func init() {
	plugin.RegisterView(PluginID, "ntr.home", func() vkdeck.View { return &HomeView{} })
	plugin.RegisterView(PluginID, "ntr.notes", func() vkdeck.View { return newNotesList("", "") })
	plugin.RegisterView(PluginID, "ntr.tasks", func() vkdeck.View { return newTasksList("", "") })
	plugin.RegisterView(PluginID, remindersViewID, func() vkdeck.View { return newRemindersList("", "") }, plugin.WithServiceOnly())
	plugin.RegisterView(PluginID, BucketsViewID, func() vkdeck.View { return newBucketList("", "") })
}

func NewBucketsView(home, role string) vkdeck.View { return newBucketList(home, role) }

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
	if plugin.ViewUIVisible(remindersViewID) {
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
	items = append(items, vkdeck.MenuItem{
		Label: "Buckets",
		Desc:  "group records; anchor them to items and runs",
		Icon:  glyph.Bucket(),
		Hue:   bucketHue,
		OnSelect: func(h *vkdeck.Model) tea.Cmd {
			return h.Push(newBucketList(home, role))
		},
	})
	v.menu = vkdeck.NewMenu("notes", []keys.Hint{{Key: "role", Label: role}}, items...)
	return v
}

func RemindersUIVisible() bool {
	return plugin.ViewUIVisible(remindersViewID)
}

func NewNoteBuilder(home, role string, sc keys.Scheme) vkdeck.View {
	return newNoteView(home, role, record{Kind: kindNote}, nil, sc)
}

func NewTaskBuilder(home, role string, sc keys.Scheme) vkdeck.View {
	return newTaskView(home, role, record{Kind: kindTask}, nil, sc)
}

func NewRemindBuilder(home, role string, sc keys.Scheme) vkdeck.View {
	return newRemindView(home, role, record{Kind: kindReminder}, nil, sc)
}

func (v *HomeView) Title() string                             { return v.menu.Title() }
func (v *HomeView) Init() tea.Cmd                             { return v.menu.Init() }
func (v *HomeView) Update(h *vkdeck.Model, m tea.Msg) tea.Cmd { return v.menu.Update(h, m) }
func (v *HomeView) Body(f layout.Frame) string                { return v.menu.Body(f) }
func (v *HomeView) Hints(scope *ui.Scope) []keys.Hint         { return v.menu.Hints(scope) }
func (v *HomeView) Context(scope *ui.Scope) []keys.Hint       { return v.menu.Context(scope) }
