package views

import (
	"context"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/viewkit/forms"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/theme"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/munin/internal/app/loginflow"
	"github.com/codyconfer/munin/internal/keymap"
	mnotify "github.com/codyconfer/munin/internal/notify"
	"github.com/codyconfer/munin/internal/render"
	"github.com/codyconfer/munin/internal/render/glyph"
)

type loginAlreadyAuthedMsg struct{ label string }
type loginPage struct {
	kit   *Kit
	menu  vkdeck.View
	toast *vkdeck.Toaster
}

func (k *Kit) Login() vkdeck.View {
	page := &loginPage{kit: k, toast: vkdeck.NewToaster(4, 3*time.Second)}
	var items []vkdeck.MenuItem
	for i, p := range loginflow.Providers() {
		p := p
		items = append(items, vkdeck.MenuItem{
			Label: p.Label,
			Desc:  k.loginStatus(p),
			Icon:  loginIcon(p.Key),
			Hue:   i,
			Do: func(a *vkdeck.Model) tea.Cmd {
				if p.Authed(k.d.App) {
					return func() tea.Msg { return loginAlreadyAuthedMsg{label: p.Label} }
				}
				return a.Push(k.loginFlow(p))
			},
		})
	}
	page.menu = vkdeck.NewMenu("accounts", k.menuCtx(), items...)
	return page
}

func (p *loginPage) Title() string        { return p.menu.Title() }
func (p *loginPage) Context() [][2]string { return p.menu.Context() }
func (p *loginPage) Hints() [][2]string   { return p.menu.Hints() }
func (p *loginPage) Init() tea.Cmd        { return p.menu.Init() }

func (p *loginPage) Update(a *vkdeck.Model, msg tea.Msg) tea.Cmd {
	if cmd, handled := p.toast.Update(msg); handled {
		return cmd
	}
	if m, ok := msg.(loginAlreadyAuthedMsg); ok {
		return p.toast.Push(mnotify.AlreadyAuthed(m.label))
	}
	return p.menu.Update(a, msg)
}

func (p *loginPage) Body(width, height int) string {
	return p.toast.Body(p.menu.Body(width, height), width)
}

func (k *Kit) loginStatus(p loginflow.Provider) string {
	switch {
	case p.Authed(k.d.App):
		return "authorized"
	case len(p.Missing(k.d.App)) > 0:
		return "needs credentials"
	default:
		return "not authorized"
	}
}

func loginIcon(key string) string {
	if logo := glyph.ForTool(key); logo != "" {
		return logo
	}
	return glyph.Login()
}

const (
	loginStepForm = iota
	loginStepRun
	loginStepDone
)

type loginFlowView struct {
	kit   *Kit
	prov  loginflow.Provider
	step  int
	form  *forms.Form
	creds map[string]string

	spin   spinner.Model
	out    chan string
	result chan error
	cancel context.CancelFunc
	log    string
	err    error
}

func (k *Kit) loginFlow(p loginflow.Provider) vkdeck.View {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = theme.Cur().Accent

	v := &loginFlowView{kit: k, prov: p, creds: map[string]string{}, spin: sp}

	if p.Authed(k.d.App) {
		return v
	}

	missing := p.Missing(k.d.App)
	if len(missing) == 0 {
		v.step = loginStepRun
		return v
	}
	fields := make([]forms.Field, len(missing))
	for i, f := range missing {
		fields[i] = forms.Field{Key: f.Key, Label: f.Label, Kind: forms.FieldText, Secret: f.Secret}
	}
	v.form = forms.NewForm(fields...)
	v.step = loginStepForm
	return v
}

func (v *loginFlowView) Title() string        { return "accounts: " + strings.ToLower(v.prov.Label) }
func (v *loginFlowView) Context() [][2]string { return v.kit.menuCtx() }

func (v *loginFlowView) Hints() [][2]string {
	switch v.step {
	case loginStepForm:
		return [][2]string{{"↑/↓", "field"}, {"ctrl+s", "continue"}, {"esc", "cancel"}}
	case loginStepDone:
		return [][2]string{{"enter", "back"}}
	default:
		return nil
	}
}

func (v *loginFlowView) Init() tea.Cmd {
	if v.prov.Authed(v.kit.d.App) {
		return func() tea.Msg { return loginAlreadyAuthedMsg{label: v.prov.Label} }
	}
	if v.step == loginStepRun {
		return v.start()
	}
	return nil
}

func (v *loginFlowView) start() tea.Cmd {
	v.out = make(chan string, 64)
	v.result = make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	v.cancel = cancel
	app := v.kit.d.App
	prov := v.prov
	creds := v.creds
	go func(out chan string, result chan error) {
		err := prov.Login(ctx, app, creds, chanWriter{out})
		cancel()
		close(out)
		result <- err
	}(v.out, v.result)
	return tea.Batch(v.spin.Tick, v.readCmd())
}

func (v *loginFlowView) stop() {
	if v.cancel != nil {
		v.cancel()
		v.cancel = nil
	}
}

func (v *loginFlowView) readCmd() tea.Cmd {
	out, result := v.out, v.result
	return func() tea.Msg {
		line, ok := <-out
		if !ok {
			return loginDoneMsg{err: <-result}
		}
		return loginOutputMsg{line: line}
	}
}

type loginOutputMsg struct{ line string }
type loginDoneMsg struct{ err error }

type chanWriter struct{ ch chan string }

func (w chanWriter) Write(p []byte) (int, error) {
	w.ch <- string(p)
	return len(p), nil
}

func (v *loginFlowView) Update(a *vkdeck.Model, msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case loginAlreadyAuthedMsg:
		return tea.Sequence(a.Pop(), func() tea.Msg { return m })
	case loginOutputMsg:
		v.log += m.line
		return v.readCmd()
	case loginDoneMsg:
		v.stop()
		v.err = m.err
		v.step = loginStepDone
		if m.err == nil && v.prov.Key == "github" {
			v.kit.d.App.ResetGitHubAuth()
		}
		return nil
	case spinner.TickMsg:
		if v.step != loginStepRun {
			return nil
		}
		var cmd tea.Cmd
		v.spin, cmd = v.spin.Update(m)
		return cmd
	case tea.KeyMsg:
		return v.handleKey(a, m)
	}
	return nil
}

func (v *loginFlowView) handleKey(a *vkdeck.Model, key tea.KeyMsg) tea.Cmd {
	switch v.step {
	case loginStepForm:
		act, ok := keymap.Form().Action(key.String())
		if !ok {
			if key.String() == " " {
				v.form.Insert(" ")
			} else if key.Type == tea.KeyRunes {
				v.form.Insert(string(key.Runes))
			}
			return nil
		}
		switch act {
		case keys.Cancel:
			return a.Pop()
		case keymap.Save:
			return v.submit(a)
		default:
			v.form.Handle(act)
		}
	case loginStepRun:
		if act, ok := keymap.Menu().Action(key.String()); ok && act == keys.Cancel {
			v.stop()
			return a.Pop()
		}
	case loginStepDone:
		if act, ok := keymap.Menu().Action(key.String()); ok && (act == keys.Cancel || act == keys.Confirm) {
			return a.Pop()
		}
	}
	return nil
}

func (v *loginFlowView) submit(a *vkdeck.Model) tea.Cmd {
	for key, val := range v.form.Values() {
		s, _ := val.(string)
		s = strings.TrimSpace(s)
		if s == "" {
			return a.Push(vkdeck.NewMessage("accounts", theme.Cur().Cant.Render(key+" is required"), v.kit.menuCtx()))
		}
		v.creds[key] = s
	}
	if err := loginflow.PersistCredentials(v.kit.d.App, v.creds); err != nil {
		return a.Push(vkdeck.NewMessage("accounts", theme.Cur().Cant.Render(err.Error()), v.kit.menuCtx()))
	}
	v.step = loginStepRun
	return v.start()
}

func (v *loginFlowView) Body(width, _ int) string {
	f := layout.ScreenFrame(width)
	title := "ACCOUNTS · " + v.prov.Label
	th := theme.Cur()

	if v.prov.Authed(v.kit.d.App) {
		return render.TitledBox(f, true, title, th.Dim.Render("already authorized — returning to accounts…"))
	}

	switch v.step {
	case loginStepForm:
		intro := th.Dim.Render("OAuth client credentials not found — enter them to continue.")
		return intro + "\n\n" + v.form.Render(layout.NewFrame(width), title)
	case loginStepDone:
		var rows []string
		if v.log != "" {
			rows = append(rows, logLines(v.log)...)
			rows = append(rows, "")
		}
		if v.err != nil {
			rows = append(rows, th.Cant.Render(v.err.Error()))
		} else {
			rows = append(rows, render.Success(v.prov.Label+" authorized — token cached."))
		}
		return render.TitledBox(f, true, title, rows...)
	default:
		rows := logLines(v.log)
		rows = append(rows, v.spin.View()+th.Dim.Render(" waiting for authorization…"))
		return render.TitledBox(f, true, title, rows...)
	}
}

func logLines(s string) []string {
	return layout.Lines(s)
}
