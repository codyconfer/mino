package deck

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/ui"

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
	if err := vkdeck.Run(root, minoOpts(opts...)...); err != nil {
		return errs.Wrap(errs.KindInternal, err, "deck program exited with error")
	}
	return nil
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
