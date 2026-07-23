package tui

import (
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/keymap"
	"github.com/codyconfer/viewkit/keys"
	"github.com/codyconfer/viewkit/layout"
	"github.com/codyconfer/viewkit/theme"
)

type View interface {
	Title() string
	Init() tea.Cmd
	Update(a *App, msg tea.Msg) tea.Cmd
	Body(width, height int) string
	Hints() [][2]string
	Context() [][2]string
}

type tickMsg time.Time

type App struct {
	stack  []View
	width  int
	height int
	clock  string
}

func New(root View) *App {
	return &App{stack: []View{root}, clock: time.Now().Format("15:04:05")}
}

func Run(root View) error {
	_, err := tea.NewProgram(New(root), tea.WithAltScreen()).Run()
	if err != nil {
		return errs.Wrap(errs.KindInternal, err, "tui program exited with error")
	}
	return nil
}

func (a *App) top() View {
	return a.stack[len(a.stack)-1]
}

func (a *App) Push(v View) tea.Cmd {
	a.stack = append(a.stack, v)
	return tea.Batch(v.Init(), a.resizeCmd())
}

func (a *App) Pop() tea.Cmd {
	if len(a.stack) <= 1 {
		return tea.Quit
	}
	a.stack = a.stack[:len(a.stack)-1]
	return a.resizeCmd()
}

func (a *App) resizeCmd() tea.Cmd {
	return func() tea.Msg { return tea.WindowSizeMsg{Width: a.width, Height: a.height} }
}

func (a *App) Init() tea.Cmd {
	return tea.Batch(a.tick(), a.top().Init())
}

func (a *App) tick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func (a *App) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch m := msg.(type) {
	case tea.WindowSizeMsg:
		a.width, a.height = m.Width, m.Height
		return a, a.top().Update(a, msg)
	case tickMsg:
		a.clock = time.Time(m).Format("15:04:05")
		return a, a.tick()
	case tea.KeyMsg:
		if keymap.IsQuit(m.String()) {
			return a, tea.Quit
		}
		return a, a.top().Update(a, msg)
	default:
		return a, a.top().Update(a, msg)
	}
}

func (a *App) View() string {
	if a.width == 0 {
		return "initializing deck…"
	}
	if !layout.FitsScreenWidth(a.width) {
		return layout.TooNarrow(a.width)
	}
	v := a.top()
	header := a.header(v)
	footer := a.footer(v)
	bodyHeight := a.height - layout.CountLines(header) - layout.CountLines(footer) - 1
	if bodyHeight < 1 {
		bodyHeight = 1
	}
	return layout.StackTight(header, v.Body(a.width, bodyHeight), footer)
}

func (a *App) header(v View) string {
	f := layout.ScreenFrame(a.width)
	th := theme.Cur()
	brand := th.Accent.Render("▚▚ MUNIN") + th.Dim.Render("  ono-sendai deck")
	clock := th.Key.Render("▓▒░ ") + th.Accent.Render(a.clock)
	line := f.Spread(brand, clock)

	title := th.PanelTitle.Render(strings.ToUpper(v.Title()))
	ctx := contextLine(v.Context())
	head := title
	if ctx != "" {
		head = f.Spread(title, ctx)
	}
	return layout.StackTight(line, head, f.Rule())
}

func (a *App) footer(v View) string {
	f := layout.ScreenFrame(a.width)
	hints := append([][2]string{}, v.Hints()...)
	hints = append(hints, keymap.Hint(keys.Cancel), keymap.Hint(keys.Quit))
	return layout.StackTight(f.Rule(), f.HintLine(hints...))
}

func contextLine(cues [][2]string) string {
	if len(cues) == 0 {
		return ""
	}
	th := theme.Cur()
	parts := make([]string, 0, len(cues))
	for _, c := range cues {
		if c[1] == "" {
			continue
		}
		parts = append(parts, th.Dim.Render(c[0]+" ⟩ ")+th.Val.Render(c[1]))
	}
	return strings.Join(parts, th.Dim.Render("   ·   "))
}
