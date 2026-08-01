package deck

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/ui"

	"github.com/codyconfer/mino/internal/console"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/render/glyph"
)

type Option func(*vkdeck.Model)

// WithScope installs the rendering scope on the deck model.
func WithScope(scope *ui.Scope) Option {
	return Option(vkdeck.WithScope(scope))
}

// WithStatus installs the async status loader; scope is captured for the
// off-goroutine theme/glyph resolution (nil snapshots the process defaults).
func WithStatus(scope *ui.Scope, fn StatusFunc) Option {
	if scope == nil {
		scope = ui.Default()
	}
	return func(h *vkdeck.Model) {
		vkdeck.WithStatus(func(ctx context.Context) vkdeck.StatusInfo {
			return adaptStatus(scope, fn(ctx))
		})(h)
	}
}

func WithKeyHook(fn func(m *vkdeck.Model, key tea.KeyMsg) (tea.Cmd, bool)) Option {
	return Option(vkdeck.WithKeyHook(fn))
}

func WithMsgHook(fn func(m *vkdeck.Model, msg tea.Msg) (tea.Cmd, bool)) Option {
	return Option(vkdeck.WithMsgHook(fn))
}

func WithInitCmd(cmd tea.Cmd) Option {
	return Option(vkdeck.WithInitCmd(cmd))
}

func New(root vkdeck.View, opts ...Option) *vkdeck.Model {
	return vkdeck.New(root, minoOpts(opts...)...)
}

func Run(root vkdeck.View, opts ...Option) error {
	return RunContext(root, "", opts...)
}

func RunContext(root vkdeck.View, context string, opts ...Option) error {
	model := New(root, opts...)
	titled := &titleModel{Model: model, context: context}
	defer titled.stop()
	if _, err := tea.NewProgram(titled, tea.WithAltScreen()).Run(); err != nil {
		return errs.Wrap(errs.KindInternal, err, "deck program exited with error")
	}
	return nil
}

type titleModel struct {
	*vkdeck.Model
	context     string
	last        string
	stopLoading func()
}

func (m *titleModel) Init() tea.Cmd {
	cmd := m.Model.Init()
	m.last = m.title()
	console.Remember(m.last)
	m.syncLoading()
	return tea.Batch(cmd, tea.SetWindowTitle(m.last))
}

func (m *titleModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	next, cmd := m.Model.Update(msg)
	if model, ok := next.(*vkdeck.Model); ok {
		m.Model = model
	}
	title := m.title()
	if title != m.last {
		console.Remember(title)
	}
	m.syncLoading()
	if title == m.last {
		return m, cmd
	}
	m.last = title
	return m, tea.Batch(cmd, tea.SetWindowTitle(title))
}

func (m *titleModel) title() string {
	return console.Title("deck", m.Top().Title(), m.context)
}

func (m *titleModel) syncLoading() {
	loading := false
	if view, ok := m.Top().(interface{ ConsoleLoading() bool }); ok {
		loading = view.ConsoleLoading()
	}
	if view, ok := m.Top().(interface{ Running() bool }); ok {
		loading = loading || view.Running()
	}
	if loading && m.stopLoading == nil {
		m.stopLoading = console.StartLoading()
	}
	if !loading && m.stopLoading != nil {
		m.stopLoading()
		m.stopLoading = nil
	}
}

func (m *titleModel) stop() {
	if m.stopLoading != nil {
		m.stopLoading()
		m.stopLoading = nil
	}
}

// minoOpts applies the caller's options first (so WithScope lands), then
// builds the chrome glyphs from the model's scope.
func minoOpts(opts ...Option) []vkdeck.Option {
	out := make([]vkdeck.Option, 0, len(opts)+2)
	for _, o := range opts {
		out = append(out, vkdeck.Option(o))
	}
	return append(out,
		vkdeck.Option(func(h *vkdeck.Model) {
			g := h.UI().Glyphs
			vkdeck.WithChrome(vkdeck.Chrome{
				Brand:      "MINO",
				BrandGlyph: glyph.BrandIn(g),
				Subtitle:   "netrunner deck",
				ClockGlyph: glyph.Pad(g.Clock()),
			})(h)
		}),
		vkdeck.WithKeyMapQuit(),
	)
}
