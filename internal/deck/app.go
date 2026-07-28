package deck

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/render/glyph"
)

type Option func(*vkdeck.Model)

func WithStatus(fn StatusFunc) Option {
	return func(h *vkdeck.Model) {
		vkdeck.WithStatus(func(ctx context.Context) vkdeck.StatusInfo {
			return adaptStatus(fn(ctx))
		})(h)
	}
}

func WithKeyHook(fn func(m *vkdeck.Model, key tea.KeyMsg) (tea.Cmd, bool)) Option {
	return Option(vkdeck.WithKeyHook(fn))
}

func WithMsgHook(fn func(m *vkdeck.Model, msg tea.Msg) (tea.Cmd, bool)) Option {
	return Option(vkdeck.WithMsgHook(fn))
}

func New(root vkdeck.View, opts ...Option) *vkdeck.Model {
	return vkdeck.New(root, muninOpts(opts...)...)
}

func Run(root vkdeck.View, opts ...Option) error {
	if err := vkdeck.Run(root, muninOpts(opts...)...); err != nil {
		return errs.Wrap(errs.KindInternal, err, "deck program exited with error")
	}
	return nil
}

func muninOpts(opts ...Option) []vkdeck.Option {
	out := []vkdeck.Option{
		vkdeck.WithChrome(vkdeck.Chrome{
			Brand:      "MUNIN",
			BrandGlyph: glyph.Brand(),
			Subtitle:   "netrunner deck",
			ClockGlyph: glyph.Pad(glyph.Clock()),
		}),
		vkdeck.WithKeyMapQuit(),
	}
	for _, o := range opts {
		out = append(out, vkdeck.Option(o))
	}
	return out
}
