package ntr

import (
	"context"
	"fmt"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/forms"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/munin/internal/keymap"
	"github.com/codyconfer/munin/internal/plugin"
)

func init() {
	plugin.RegisterView(PluginID, "ntr.home", func() vkdeck.View { return &HomeView{} })
	plugin.RegisterView(PluginID, "ntr.notes", func() vkdeck.View { return &notesList{} })
	plugin.RegisterView(PluginID, "ntr.tasks", func() vkdeck.View { return &tasksList{} })
	plugin.RegisterView(PluginID, "ntr.reminders", func() vkdeck.View { return &remindList{} })
}

// RunTUI opens the NTR deck on viewkit/deck.
func RunTUI(home, role string) error {
	if role == "" {
		role = "default"
	}
	return vkdeck.Run(NewHomeView(home, role), vkdeck.WithChrome(vkdeck.Chrome{
		Brand:    "MUNIN",
		Subtitle: "notes",
	}), vkdeck.WithKeyMapQuit())
}

// HomeView is the NTR root menu (notes / tasks / reminders).
type HomeView struct {
	home string
	role string
	menu *vkdeck.Menu
}

// NewHomeView constructs the NTR home menu.
func NewHomeView(home, role string) *HomeView {
	if role == "" {
		role = "default"
	}
	v := &HomeView{home: home, role: role}
	v.menu = vkdeck.NewMenu("notes", [][2]string{{"role", role}},
		vkdeck.MenuItem{Label: "Notes", Desc: "create · edit · delete", Do: func(h *vkdeck.Model) tea.Cmd {
			return h.Push(&notesList{home: home, role: role})
		}},
		vkdeck.MenuItem{Label: "Tasks", Desc: "create · toggle · delete", Do: func(h *vkdeck.Model) tea.Cmd {
			return h.Push(&tasksList{home: home, role: role})
		}},
		vkdeck.MenuItem{Label: "Reminders", Desc: "create · complete", Do: func(h *vkdeck.Model) tea.Cmd {
			return h.Push(&remindList{home: home, role: role})
		}},
	)
	return v
}

// NewNoteForm opens the create-note form (hotkey / deep-link entry).
func NewNoteForm(home, role string) vkdeck.View {
	return newNoteForm(home, role, 0, "", "", nil)
}

// NewTaskForm opens the create-task form (hotkey / deep-link entry).
func NewTaskForm(home, role string) vkdeck.View {
	return newTaskForm(home, role, nil)
}

// NewRemindForm opens the create-reminder form (hotkey / deep-link entry).
func NewRemindForm(home, role string) vkdeck.View {
	return newRemindForm(home, role, nil)
}

func (v *HomeView) Title() string                             { return v.menu.Title() }
func (v *HomeView) Init() tea.Cmd                             { return v.menu.Init() }
func (v *HomeView) Update(h *vkdeck.Model, m tea.Msg) tea.Cmd { return v.menu.Update(h, m) }
func (v *HomeView) Body(w, ht int) string                     { return v.menu.Body(w, ht) }
func (v *HomeView) Hints() [][2]string                        { return v.menu.Hints() }
func (v *HomeView) Context() [][2]string                      { return v.menu.Context() }

func openStore(home, role string) (*Store, context.Context, context.CancelFunc, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	st, err := Open(ctx, home, role)
	if err != nil {
		cancel()
		return nil, nil, nil, err
	}
	return st, ctx, cancel, nil
}

func listCursor(key tea.KeyMsg, cursor, n int) (int, keys.Action, bool) {
	act, ok := keymap.Menu().Action(key.String())
	if !ok {
		return cursor, "", false
	}
	switch act {
	case keys.Up:
		if cursor > 0 {
			cursor--
		}
	case keys.Down:
		if cursor < n-1 {
			cursor++
		}
	}
	return cursor, act, true
}

func renderRows(width, cursor int, title string, rows []string, empty string) string {
	th := theme.Cur()
	f := layout.ScreenFrame(width)
	if len(rows) == 0 {
		return f.TitledBox(title, th.Dim.Render(empty))
	}
	lines := make([]string, len(rows))
	for i, row := range rows {
		cursorMark := "  "
		body := th.Val.Render(row)
		if i == cursor {
			cursorMark = th.Key.Render("▸ ")
			body = th.Key.Render(row)
		}
		lines[i] = cursorMark + body
	}
	return f.TitledBox(title, lines...)
}

type notesList struct {
	home   string
	role   string
	notes  []Note
	cursor int
	err    string
	loaded bool
}

type notesLoadedMsg struct {
	notes []Note
	err   string
}

func (v *notesList) Title() string { return "notes" }
func (v *notesList) Context() [][2]string {
	return [][2]string{{"role", v.role}}
}
func (v *notesList) Hints() [][2]string {
	return [][2]string{{"n", "new"}, {"e", "edit"}, {"d", "delete"}, {"r", "reload"}}
}
func (v *notesList) Init() tea.Cmd { return v.reload() }

func (v *notesList) reload() tea.Cmd {
	home, role := v.home, v.role
	return func() tea.Msg {
		st, ctx, cancel, err := openStore(home, role)
		if err != nil {
			return notesLoadedMsg{err: err.Error()}
		}
		defer cancel()
		defer st.Close()
		notes, err := st.ListNotes(ctx)
		if err != nil {
			return notesLoadedMsg{err: err.Error()}
		}
		return notesLoadedMsg{notes: notes}
	}
}

func (v *notesList) Update(h *vkdeck.Model, msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case notesLoadedMsg:
		v.notes, v.err, v.loaded = m.notes, m.err, true
		if v.cursor >= len(v.notes) {
			v.cursor = max(len(v.notes)-1, 0)
		}
		return nil
	case tea.KeyMsg:
		switch m.String() {
		case "r":
			return v.reload()
		case "n":
			return h.Push(newNoteForm(v.home, v.role, 0, "", "", func() tea.Cmd { return v.reload() }))
		case "e":
			if len(v.notes) == 0 {
				return nil
			}
			n := v.notes[v.cursor]
			return h.Push(newNoteForm(v.home, v.role, n.ID, n.Title, n.Body, func() tea.Cmd { return v.reload() }))
		case "d":
			if len(v.notes) == 0 {
				return nil
			}
			id := v.notes[v.cursor].ID
			home, role := v.home, v.role
			return func() tea.Msg {
				st, ctx, cancel, err := openStore(home, role)
				if err != nil {
					return notesLoadedMsg{err: err.Error()}
				}
				defer cancel()
				defer st.Close()
				if err := st.DeleteNote(ctx, id); err != nil {
					return notesLoadedMsg{err: err.Error()}
				}
				notes, err := st.ListNotes(ctx)
				if err != nil {
					return notesLoadedMsg{err: err.Error()}
				}
				return notesLoadedMsg{notes: notes}
			}
		}
		var act keys.Action
		v.cursor, act, _ = listCursor(m, v.cursor, len(v.notes))
		if act == keys.Cancel {
			return h.Pop()
		}
	}
	return nil
}

func (v *notesList) Body(width, _ int) string {
	th := theme.Cur()
	if v.err != "" {
		return th.Cant.Render(v.err)
	}
	if !v.loaded {
		return th.Dim.Render("loading…")
	}
	rows := make([]string, len(v.notes))
	for i, n := range v.notes {
		rows[i] = fmt.Sprintf("%d  %s", n.ID, n.Title)
	}
	return renderRows(width, v.cursor, "NOTES", rows, "(none — press n)")
}

type noteForm struct {
	home, role string
	id         int64
	form       *forms.Form
	onSaved    func() tea.Cmd
	err        string
}

type savedMsg struct {
	err    string
	reload func() tea.Cmd
}

func newNoteForm(home, role string, id int64, title, body string, onSaved func() tea.Cmd) *noteForm {
	return &noteForm{
		home: home, role: role, id: id, onSaved: onSaved,
		form: forms.NewForm(
			forms.Field{Key: "title", Label: "title", Kind: forms.FieldText, Text: title},
			forms.Field{Key: "body", Label: "body", Kind: forms.FieldMultiline, Text: body},
		),
	}
}

func (v *noteForm) Title() string {
	if v.id == 0 {
		return "new note"
	}
	return "edit note"
}
func (v *noteForm) Init() tea.Cmd        { return nil }
func (v *noteForm) Context() [][2]string { return [][2]string{{"role", v.role}} }
func (v *noteForm) Hints() [][2]string {
	return [][2]string{{"↑/↓", "field"}, {"ctrl+s", "save"}}
}
func (v *noteForm) Body(width, _ int) string {
	body := v.form.Render(layout.NewFrame(width), v.Title())
	if v.err != "" {
		return body + "\n" + theme.Cur().Cant.Render(v.err)
	}
	return body
}

func (v *noteForm) Update(h *vkdeck.Model, msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case savedMsg:
		if m.err != "" {
			v.err = m.err
			return nil
		}
		cmds := []tea.Cmd{h.Pop()}
		if m.reload != nil {
			cmds = append(cmds, m.reload())
		}
		return tea.Batch(cmds...)
	case tea.KeyMsg:
		act, ok := keymap.Form().Action(m.String())
		if !ok {
			if m.Type == tea.KeyRunes {
				v.form.Insert(string(m.Runes))
			}
			return nil
		}
		switch act {
		case keys.Cancel:
			return h.Pop()
		case keymap.Save:
			return v.save()
		default:
			v.form.Handle(act)
		}
	}
	return nil
}

func (v *noteForm) save() tea.Cmd {
	vals := v.form.Values()
	title, _ := vals["title"].(string)
	body, _ := vals["body"].(string)
	title = strings.TrimSpace(title)
	if title == "" {
		v.err = "title required"
		return nil
	}
	home, role, id, onSaved := v.home, v.role, v.id, v.onSaved
	return func() tea.Msg {
		st, ctx, cancel, err := openStore(home, role)
		if err != nil {
			return savedMsg{err: err.Error()}
		}
		defer cancel()
		defer st.Close()
		if id == 0 {
			_, err = st.CreateNote(ctx, title, body)
		} else {
			err = st.UpdateNote(ctx, id, title, body)
		}
		if err != nil {
			return savedMsg{err: err.Error()}
		}
		return savedMsg{reload: onSaved}
	}
}

type tasksList struct {
	home   string
	role   string
	tasks  []Task
	cursor int
	err    string
	loaded bool
}

type tasksLoadedMsg struct {
	tasks []Task
	err   string
}

func (v *tasksList) Title() string { return "tasks" }
func (v *tasksList) Context() [][2]string {
	return [][2]string{{"role", v.role}}
}
func (v *tasksList) Hints() [][2]string {
	return [][2]string{{"n", "new"}, {"enter", "toggle"}, {"d", "delete"}, {"r", "reload"}}
}
func (v *tasksList) Init() tea.Cmd { return v.reload() }

func (v *tasksList) reload() tea.Cmd {
	home, role := v.home, v.role
	return func() tea.Msg {
		st, ctx, cancel, err := openStore(home, role)
		if err != nil {
			return tasksLoadedMsg{err: err.Error()}
		}
		defer cancel()
		defer st.Close()
		tasks, err := st.ListTasks(ctx, true)
		if err != nil {
			return tasksLoadedMsg{err: err.Error()}
		}
		return tasksLoadedMsg{tasks: tasks}
	}
}

func (v *tasksList) Update(h *vkdeck.Model, msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case tasksLoadedMsg:
		v.tasks, v.err, v.loaded = m.tasks, m.err, true
		if v.cursor >= len(v.tasks) {
			v.cursor = max(len(v.tasks)-1, 0)
		}
		return nil
	case tea.KeyMsg:
		switch m.String() {
		case "r":
			return v.reload()
		case "n":
			return h.Push(newTaskForm(v.home, v.role, func() tea.Cmd { return v.reload() }))
		case "d":
			if len(v.tasks) == 0 {
				return nil
			}
			id := v.tasks[v.cursor].ID
			home, role := v.home, v.role
			return func() tea.Msg {
				st, ctx, cancel, err := openStore(home, role)
				if err != nil {
					return tasksLoadedMsg{err: err.Error()}
				}
				defer cancel()
				defer st.Close()
				if err := st.DeleteTask(ctx, id); err != nil {
					return tasksLoadedMsg{err: err.Error()}
				}
				tasks, err := st.ListTasks(ctx, true)
				if err != nil {
					return tasksLoadedMsg{err: err.Error()}
				}
				return tasksLoadedMsg{tasks: tasks}
			}
		}
		var act keys.Action
		v.cursor, act, _ = listCursor(m, v.cursor, len(v.tasks))
		switch act {
		case keys.Confirm:
			if len(v.tasks) == 0 {
				return nil
			}
			t := v.tasks[v.cursor]
			home, role := v.home, v.role
			return func() tea.Msg {
				st, ctx, cancel, err := openStore(home, role)
				if err != nil {
					return tasksLoadedMsg{err: err.Error()}
				}
				defer cancel()
				defer st.Close()
				if err := st.SetTaskDone(ctx, t.ID, !t.Done); err != nil {
					return tasksLoadedMsg{err: err.Error()}
				}
				tasks, err := st.ListTasks(ctx, true)
				if err != nil {
					return tasksLoadedMsg{err: err.Error()}
				}
				return tasksLoadedMsg{tasks: tasks}
			}
		case keys.Cancel:
			return h.Pop()
		}
	}
	return nil
}

func (v *tasksList) Body(width, _ int) string {
	th := theme.Cur()
	if v.err != "" {
		return th.Cant.Render(v.err)
	}
	if !v.loaded {
		return th.Dim.Render("loading…")
	}
	rows := make([]string, len(v.tasks))
	for i, t := range v.tasks {
		mark := "[ ]"
		if t.Done {
			mark = "[x]"
		}
		rows[i] = fmt.Sprintf("%s %d  %s", mark, t.ID, t.Title)
	}
	return renderRows(width, v.cursor, "TASKS", rows, "(none — press n)")
}

type taskForm struct {
	home, role string
	form       *forms.Form
	onSaved    func() tea.Cmd
	err        string
}

func newTaskForm(home, role string, onSaved func() tea.Cmd) *taskForm {
	return &taskForm{
		home: home, role: role, onSaved: onSaved,
		form: forms.NewForm(
			forms.Field{Key: "title", Label: "title", Kind: forms.FieldText},
		),
	}
}

func (v *taskForm) Title() string        { return "new task" }
func (v *taskForm) Init() tea.Cmd        { return nil }
func (v *taskForm) Context() [][2]string { return [][2]string{{"role", v.role}} }
func (v *taskForm) Hints() [][2]string {
	return [][2]string{{"ctrl+s", "save"}}
}
func (v *taskForm) Body(width, _ int) string {
	body := v.form.Render(layout.NewFrame(width), v.Title())
	if v.err != "" {
		return body + "\n" + theme.Cur().Cant.Render(v.err)
	}
	return body
}

func (v *taskForm) Update(h *vkdeck.Model, msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case savedMsg:
		if m.err != "" {
			v.err = m.err
			return nil
		}
		cmds := []tea.Cmd{h.Pop()}
		if m.reload != nil {
			cmds = append(cmds, m.reload())
		}
		return tea.Batch(cmds...)
	case tea.KeyMsg:
		act, ok := keymap.Form().Action(m.String())
		if !ok {
			if m.Type == tea.KeyRunes {
				v.form.Insert(string(m.Runes))
			}
			return nil
		}
		switch act {
		case keys.Cancel:
			return h.Pop()
		case keymap.Save:
			vals := v.form.Values()
			title, _ := vals["title"].(string)
			title = strings.TrimSpace(title)
			if title == "" {
				v.err = "title required"
				return nil
			}
			home, role, onSaved := v.home, v.role, v.onSaved
			return func() tea.Msg {
				st, ctx, cancel, err := openStore(home, role)
				if err != nil {
					return savedMsg{err: err.Error()}
				}
				defer cancel()
				defer st.Close()
				if _, err := st.CreateTask(ctx, title, time.Time{}); err != nil {
					return savedMsg{err: err.Error()}
				}
				return savedMsg{reload: onSaved}
			}
		default:
			v.form.Handle(act)
		}
	}
	return nil
}

type remindList struct {
	home   string
	role   string
	items  []Reminder
	cursor int
	err    string
	loaded bool
}

type remindLoadedMsg struct {
	items []Reminder
	err   string
}

func (v *remindList) Title() string { return "reminders" }
func (v *remindList) Context() [][2]string {
	return [][2]string{{"role", v.role}}
}
func (v *remindList) Hints() [][2]string {
	return [][2]string{{"n", "new"}, {"enter", "done"}, {"r", "reload"}}
}
func (v *remindList) Init() tea.Cmd { return v.reload() }

func (v *remindList) reload() tea.Cmd {
	home, role := v.home, v.role
	return func() tea.Msg {
		st, ctx, cancel, err := openStore(home, role)
		if err != nil {
			return remindLoadedMsg{err: err.Error()}
		}
		defer cancel()
		defer st.Close()
		items, err := st.ListReminders(ctx, false)
		if err != nil {
			return remindLoadedMsg{err: err.Error()}
		}
		return remindLoadedMsg{items: items}
	}
}

func (v *remindList) Update(h *vkdeck.Model, msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case remindLoadedMsg:
		v.items, v.err, v.loaded = m.items, m.err, true
		if v.cursor >= len(v.items) {
			v.cursor = max(len(v.items)-1, 0)
		}
		return nil
	case tea.KeyMsg:
		switch m.String() {
		case "r":
			return v.reload()
		case "n":
			return h.Push(newRemindForm(v.home, v.role, func() tea.Cmd { return v.reload() }))
		}
		var act keys.Action
		v.cursor, act, _ = listCursor(m, v.cursor, len(v.items))
		switch act {
		case keys.Confirm:
			if len(v.items) == 0 {
				return nil
			}
			id := v.items[v.cursor].ID
			home, role := v.home, v.role
			return func() tea.Msg {
				st, ctx, cancel, err := openStore(home, role)
				if err != nil {
					return remindLoadedMsg{err: err.Error()}
				}
				defer cancel()
				defer st.Close()
				if err := st.MarkReminderDone(ctx, id); err != nil {
					return remindLoadedMsg{err: err.Error()}
				}
				items, err := st.ListReminders(ctx, false)
				if err != nil {
					return remindLoadedMsg{err: err.Error()}
				}
				return remindLoadedMsg{items: items}
			}
		case keys.Cancel:
			return h.Pop()
		}
	}
	return nil
}

func (v *remindList) Body(width, _ int) string {
	th := theme.Cur()
	if v.err != "" {
		return th.Cant.Render(v.err)
	}
	if !v.loaded {
		return th.Dim.Render("loading…")
	}
	rows := make([]string, len(v.items))
	for i, r := range v.items {
		due := r.Due.Local().Format("2006-01-02 15:04")
		if r.Due.IsZero() {
			due = "?"
		}
		rows[i] = fmt.Sprintf("%d  %s  %s", r.ID, due, r.Title)
	}
	return renderRows(width, v.cursor, "REMINDERS", rows, "(none — press n)")
}

type remindForm struct {
	home, role string
	form       *forms.Form
	onSaved    func() tea.Cmd
	err        string
}

func newRemindForm(home, role string, onSaved func() tea.Cmd) *remindForm {
	return &remindForm{
		home: home, role: role, onSaved: onSaved,
		form: forms.NewForm(
			forms.Field{Key: "title", Label: "title", Kind: forms.FieldText},
			forms.Field{Key: "due", Label: "due (RFC3339 or +1h)", Kind: forms.FieldText, Text: "+1h"},
		),
	}
}

func (v *remindForm) Title() string        { return "new reminder" }
func (v *remindForm) Init() tea.Cmd        { return nil }
func (v *remindForm) Context() [][2]string { return [][2]string{{"role", v.role}} }
func (v *remindForm) Hints() [][2]string {
	return [][2]string{{"ctrl+s", "save"}}
}
func (v *remindForm) Body(width, _ int) string {
	body := v.form.Render(layout.NewFrame(width), v.Title())
	if v.err != "" {
		return body + "\n" + theme.Cur().Cant.Render(v.err)
	}
	return body
}

func (v *remindForm) Update(h *vkdeck.Model, msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case savedMsg:
		if m.err != "" {
			v.err = m.err
			return nil
		}
		cmds := []tea.Cmd{h.Pop()}
		if m.reload != nil {
			cmds = append(cmds, m.reload())
		}
		return tea.Batch(cmds...)
	case tea.KeyMsg:
		act, ok := keymap.Form().Action(m.String())
		if !ok {
			if m.Type == tea.KeyRunes {
				v.form.Insert(string(m.Runes))
			}
			return nil
		}
		switch act {
		case keys.Cancel:
			return h.Pop()
		case keymap.Save:
			vals := v.form.Values()
			title, _ := vals["title"].(string)
			dueRaw, _ := vals["due"].(string)
			title = strings.TrimSpace(title)
			if title == "" {
				v.err = "title required"
				return nil
			}
			due, err := parseDue(dueRaw)
			if err != nil {
				v.err = err.Error()
				return nil
			}
			home, role, onSaved := v.home, v.role, v.onSaved
			return func() tea.Msg {
				st, ctx, cancel, err := openStore(home, role)
				if err != nil {
					return savedMsg{err: err.Error()}
				}
				defer cancel()
				defer st.Close()
				if _, err := st.CreateReminder(ctx, title, due); err != nil {
					return savedMsg{err: err.Error()}
				}
				return savedMsg{reload: onSaved}
			}
		default:
			v.form.Handle(act)
		}
	}
	return nil
}

func parseDue(s string) (time.Time, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Now().UTC().Add(time.Hour), nil
	}
	if strings.HasPrefix(s, "+") {
		d, err := time.ParseDuration(s[1:])
		if err != nil {
			return time.Time{}, fmt.Errorf("due duration: %w", err)
		}
		return time.Now().UTC().Add(d), nil
	}
	for _, layout := range []string{time.RFC3339, "2006-01-02 15:04", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, time.Local); err == nil {
			return t.UTC(), nil
		}
	}
	return time.Time{}, fmt.Errorf("due: want RFC3339, YYYY-MM-DD, or +1h")
}
