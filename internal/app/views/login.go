package views

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/viewkit/forms"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/munin/internal/app/loginflow"
	"github.com/codyconfer/munin/internal/deck"
	"github.com/codyconfer/munin/internal/keymap"
	"github.com/codyconfer/munin/internal/render"
	"github.com/codyconfer/munin/internal/render/glyph"
)

func (k *Kit) Login() deck.View {
	var items []deck.MenuItem
	for i, p := range loginflow.Providers() {
		items = append(items, deck.MenuItem{
			Label: p.Label,
			Desc:  k.loginStatus(p),
			Icon:  loginIcon(p.Key),
			Hue:   i,
			Do:    func(a *deck.State) tea.Cmd { return a.Push(k.loginFlow(p)) },
		})
	}
	return deck.NewMenu("login", k.menuCtx(), items...)
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
	switch key {
	case "github":
		return glyph.GitHub()
	case "google":
		return glyph.Google()
	case "slack":
		return glyph.Slack()
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
	log    string
	err    error
}

func (k *Kit) loginFlow(p loginflow.Provider) deck.View {
	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = theme.Cur().Accent

	v := &loginFlowView{kit: k, prov: p, creds: map[string]string{}, spin: sp}

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

func (v *loginFlowView) Title() string        { return "login: " + strings.ToLower(v.prov.Label) }
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
	if v.step == loginStepRun {
		return v.start()
	}
	return nil
}

func (v *loginFlowView) start() tea.Cmd {
	v.out = make(chan string, 64)
	v.result = make(chan error, 1)
	app := v.kit.d.App
	prov := v.prov
	creds := v.creds
	go func(out chan string, result chan error) {
		err := prov.Login(context.Background(), app, creds, chanWriter{out})
		close(out)
		result <- err
	}(v.out, v.result)
	return tea.Batch(v.spin.Tick, v.readCmd())
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

func (v *loginFlowView) Update(a *deck.State, msg tea.Msg) tea.Cmd {
	switch m := msg.(type) {
	case loginOutputMsg:
		v.log += m.line
		return v.readCmd()
	case loginDoneMsg:
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

func (v *loginFlowView) handleKey(a *deck.State, key tea.KeyMsg) tea.Cmd {
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
	case loginStepDone:
		if act, ok := keymap.Menu().Action(key.String()); ok && (act == keys.Cancel || act == keys.Confirm) {
			return a.Pop()
		}
	}
	return nil
}

func (v *loginFlowView) submit(a *deck.State) tea.Cmd {
	for key, val := range v.form.Values() {
		s, _ := val.(string)
		s = strings.TrimSpace(s)
		if s == "" {
			return a.Push(deck.NewMessage("login", theme.Cur().Cant.Render(key+" is required"), v.kit.menuCtx()))
		}
		v.creds[key] = s
	}
	if err := loginflow.PersistCredentials(v.kit.d.App, v.creds); err != nil {
		return a.Push(deck.NewMessage("login", theme.Cur().Cant.Render(err.Error()), v.kit.menuCtx()))
	}
	v.step = loginStepRun
	return v.start()
}

func (v *loginFlowView) Body(width, _ int) string {
	f := layout.ScreenFrame(width)
	title := "LOGIN · " + v.prov.Label
	th := theme.Cur()

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
	return strings.Split(strings.TrimRight(s, "\n"), "\n")
}
