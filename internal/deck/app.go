package deck

import (
	"context"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/keymap"
	"github.com/codyconfer/munin/internal/render/glyph"
)

const statusRefreshInterval = 60 * time.Second

type View interface {
	Title() string
	Init() tea.Cmd
	Update(a *State, msg tea.Msg) tea.Cmd
	Body(width, height int) string
	Hints() [][2]string
	Context() [][2]string
}

type tickMsg time.Time

type statusMsg struct{ info StatusInfo }

type statusRefreshMsg struct{}

type Option func(*State)

func WithStatus(fn StatusFunc) Option {
	return func(s *State) { s.statusFn = fn }
}

type State struct {
	stack  []View
	width  int
	height int
	clock  string

	statusFn  StatusFunc
	status    StatusInfo
	hasStatus bool
}

func New(root View, opts ...Option) *State {
	s := &State{stack: []View{root}, clock: time.Now().Format("15:04:05")}
	for _, o := range opts {
		o(s)
	}
	return s
}

func Run(root View, opts ...Option) error {
	_, err := tea.NewProgram(New(root, opts...), tea.WithAltScreen()).Run()
	if err != nil {
		return errs.Wrap(errs.KindInternal, err, "deck program exited with error")
	}
	return nil
}

func (a *State) top() View {
	return a.stack[len(a.stack)-1]
}

func (a *State) Push(v View) tea.Cmd {
	a.stack = append(a.stack, v)
	return tea.Batch(v.Init(), a.resizeCmd())
}

func (a *State) Pop() tea.Cmd {
	if len(a.stack) <= 1 {
		return tea.Quit
	}
	a.stack = a.stack[:len(a.stack)-1]
	return a.resizeCmd()
}

func (a *State) resizeCmd() tea.Cmd {
	return func() tea.Msg { return tea.WindowSizeMsg{Width: a.width, Height: a.height} }
}

func (a *State) Init() tea.Cmd {
	cmds := []tea.Cmd{a.tick(), a.top().Init()}
	if a.statusFn != nil {
		cmds = append(cmds, a.fetchStatus())
	}
	return tea.Batch(cmds...)
}

func (a *State) tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (a *State) fetchStatus() tea.Cmd {
	fn := a.statusFn
	return func() tea.Msg {
		ctx, cancel := context.WithTimeout(context.Background(), 12*time.Second)
		defer cancel()
		return statusMsg{info: fn(ctx)}
	}
}

func (a *State) scheduleStatusRefresh() tea.Cmd {
	return tea.Tick(statusRefreshInterval, func(time.Time) tea.Msg { return statusRefreshMsg{} })
}

func (a *State) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = m.Width, m.Height
		return a, a.top().Update(a, msg)
	case tickMsg:
		a.clock = time.Time(m).Format("15:04:05")
		return a, a.tick()
	case statusMsg:
		a.status, a.hasStatus = m.info, true
		return a, a.scheduleStatusRefresh()
	case statusRefreshMsg:
		return a, a.fetchStatus()
	case tea.KeyMsg:
		if keymap.IsQuit(m.String()) {
			return a, tea.Quit
		}
		return a, a.top().Update(a, msg)
	default:
		return a, a.top().Update(a, msg)
	}
}

func (a *State) View() string {
	if a.width == 0 {
		return "initializing deck…"
	}
	if !layout.FitsScreenWidth(a.width) {
		return theme.AppMargin(layout.TooNarrow(a.width))
	}
	v := a.top()
	header := a.header(v)
	footer := a.footer(v)
	bodyHeight := max(a.height-layout.CountLines(header)-layout.CountLines(footer)-chromeGaps-1, 1)
	body := fillHeight(v.Body(a.width, bodyHeight), bodyHeight)
	return theme.AppMargin(layout.Stack(header, body, footer))
}

const chromeGaps = 2

func fillHeight(body string, height int) string { return layout.FillHeight(body, height) }

const chromePad = 1

func indentLines(s string, n int) string { return layout.IndentLines(s, n) }

func (a *State) header(v View) string {
	f := layout.ScreenFrame(a.width)
	full := f.BodyWidth() + 4
	th := theme.Cur()
	muted := th.Dim.GetForeground()
	brand := stripText(muted, " ") + stripBold(th.Accent.GetForeground(), glyph.Brand()+" MUNIN") + stripText(muted, " · ono-sendai deck")
	clock := stripBold(th.Accent.GetForeground(), glyph.Clock()+" "+a.clock)
	right := clock
	if id := a.identity(); id != "" {
		right = id + stripText(muted, "   ") + clock
	}
	return stripBlock(full,
		stripLine(full, brand, right+stripText(muted, " ")),
		stripLine(full, a.breadcrumbs(), a.contextCues(v)),
	)
}

func (a *State) breadcrumbs() string {
	th := theme.Cur()
	muted := th.Dim.GetForeground()
	sep := stripText(muted, " ⟩ ")
	parts := make([]string, len(a.stack))
	for i, v := range a.stack {
		if i == len(a.stack)-1 {
			parts[i] = stripBold(th.Accent.GetForeground(), v.Title())
		} else {
			parts[i] = stripText(muted, v.Title())
		}
	}
	return stripText(muted, " ") + strings.Join(parts, sep)
}

func (a *State) contextCues(v View) string {
	th := theme.Cur()
	muted := th.Dim.GetForeground()
	parts := make([]string, 0)
	for _, c := range v.Context() {
		if c[1] == "" {
			continue
		}
		parts = append(parts, stripText(muted, c[0]+": ")+stripText(th.Val.GetForeground(), c[1]))
	}
	if len(parts) == 0 {
		return ""
	}
	return strings.Join(parts, stripText(muted, " · ")) + stripText(muted, " ")
}

func (a *State) footer(v View) string {
	f := layout.ScreenFrame(a.width)
	full := f.BodyWidth() + 4
	hints := append([][2]string{}, v.Hints()...)
	hints = append(hints, keymap.Hint(keys.Cancel), keymap.Hint(keys.Quit))
	legend := indentLines(f.HintLine(hints...), chromePad)
	bar := stripBlock(full, stripLine(full, a.statusSegments(), ""))
	return layout.Stack(bar, legend)
}

func (a *State) identity() string {
	if !a.hasStatus || a.status.GitHubUser == "" {
		return ""
	}
	th := theme.Cur()
	mark := stripText(th.Cant.GetForeground(), glyph.SigningBad())
	if a.status.SigningVerified {
		mark = stripText(th.Can.GetForeground(), glyph.SigningOK())
	}
	return stripBold(th.Key.GetForeground(), "@"+a.status.GitHubUser) + stripText(th.Dim.GetForeground(), " ") + mark
}

func (a *State) statusSegments() string {
	if !a.hasStatus || len(a.status.Services) == 0 {
		return ""
	}
	th := theme.Cur()
	sep := stripText(th.Dim.GetForeground(), " · ")
	parts := make([]string, 0, len(a.status.Services))
	for _, s := range a.status.Services {
		label := s.Name
		if s.Detail != "" {
			label += " " + s.Detail
		}
		parts = append(parts, stripText(statusColor(s.Level), glyph.Lead(glyphForLevel(s.Level)))+stripText(th.Val.GetForeground(), label))
	}
	return stripText(th.Dim.GetForeground(), " ") + strings.Join(parts, sep)
}
