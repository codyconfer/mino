package deck

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/render/glyph"
)

// Option configures the munin-branded host.
type Option func(*vkdeck.Model)

// WithStatus installs an async status loader (munin StatusInfo → deck chrome).
func WithStatus(fn StatusFunc) Option {
	return func(h *vkdeck.Model) {
		vkdeck.WithStatus(func(ctx context.Context) vkdeck.StatusInfo {
			return adaptStatus(fn(ctx))
		})(h)
	}
}

// WithKeyHook installs a global hotkey interceptor on the deck host.
func WithKeyHook(fn func(m *vkdeck.Model, key tea.KeyMsg) (tea.Cmd, bool)) Option {
	return Option(vkdeck.WithKeyHook(fn))
}

// WithMsgHook installs a global message interceptor on the deck host
// (e.g. debounced role-lifecycle settle).
func WithMsgHook(fn func(m *vkdeck.Model, msg tea.Msg) (tea.Cmd, bool)) Option {
	return Option(vkdeck.WithMsgHook(fn))
}

// New builds a host with munin chrome defaults.
func New(root vkdeck.View, opts ...Option) *vkdeck.Model {
	return vkdeck.New(root, muninOpts(opts...)...)
}

// Run starts the tea program with munin chrome (runtime lives in viewkit/deck).
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
			// Pad (not Lead): viewkit chrome already appends one space
			// before the time; Lead's nerd double-space made the gap too wide.
			ClockGlyph: glyph.Pad(glyph.Clock()),
		}),
		vkdeck.WithKeyMapQuit(),
	}
	for _, o := range opts {
		out = append(out, vkdeck.Option(o))
	}
	return out
}
