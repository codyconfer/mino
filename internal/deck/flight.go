package deck

import (
	"context"
	"strings"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/codyconfer/viewkit/keys"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/keymap"
	"github.com/codyconfer/munin/internal/render"
	"github.com/codyconfer/munin/internal/signals"
)

type Task struct {
	Label string
	Run   func(ctx context.Context) []signals.Section
}

func RunFlight(ctx context.Context, tasks []Task) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	sp := spinner.New()
	sp.Spinner = spinner.Dot
	sp.Style = lipgloss.NewStyle().Foreground(lipgloss.Color("12"))

	m := model{
		ctx:    ctx,
		tasks:  tasks,
		panels: make([]panel, len(tasks)),
		spin:   sp,
		left:   len(tasks),
	}
	for i, t := range tasks {
		m.panels[i].label = t.Label
	}

	if _, err := tea.NewProgram(m, tea.WithContext(ctx)).Run(); err != nil {
		return errs.Wrap(errs.KindInternal, err, "run flight view")
	}
	return nil
}

type panel struct {
	label   string
	done    bool
	content string
}

type doneMsg struct {
	idx     int
	content string
}

type model struct {
	ctx    context.Context
	tasks  []Task
	panels []panel
	spin   spinner.Model
	left   int
}

func (m model) Init() tea.Cmd {
	cmds := make([]tea.Cmd, 0, len(m.tasks)+1)
	cmds = append(cmds, m.spin.Tick)
	for i := range m.tasks {
		cmds = append(cmds, m.runTask(i))
	}
	return tea.Batch(cmds...)
}

func (m model) runTask(i int) tea.Cmd {
	return func() tea.Msg {
		sections := m.tasks[i].Run(m.ctx)
		return doneMsg{idx: i, content: render.RenderTerminalString(sections)}
	}
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		if act, ok := keymap.Menu().Action(msg.String()); ok && (act == keys.Quit || act == keys.Cancel) {
			return m, tea.Quit
		}
	case doneMsg:
		if !m.panels[msg.idx].done {
			m.panels[msg.idx].done = true
			m.panels[msg.idx].content = msg.content
			m.left--
		}
		if m.left == 0 {
			return m, tea.Quit
		}
		return m, nil
	case spinner.TickMsg:
		var cmd tea.Cmd
		m.spin, cmd = m.spin.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m model) View() string {
	parts := make([]string, len(m.panels))
	for i, p := range m.panels {
		if p.done {
			parts[i] = p.content
			continue
		}
		parts[i] = render.LoadingPanel(p.label, m.spin.View()+" loading…")
	}
	return strings.Join(parts, "\n") + "\n"
}
