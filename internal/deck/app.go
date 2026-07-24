package deck

import (
	"context"

	tea "github.com/charmbracelet/bubbletea"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/keymap"
	"github.com/codyconfer/munin/internal/render/glyph"
)

// State is the deck host (viewkit/deck). Kept as an alias so existing munin
// views keep Update(*State, …) / MenuItem.Do signatures.
type State = vkdeck.Host

// View is the navigable screen contract from viewkit/deck.
type View = vkdeck.View

// Option configures the host.
type Option func(*vkdeck.Host)

// WithStatus installs an async status loader (munin StatusInfo → deck chrome).
func WithStatus(fn StatusFunc) Option {
	return func(h *vkdeck.Host) {
		vkdeck.WithStatus(func(ctx context.Context) vkdeck.StatusInfo {
			return adaptStatus(fn(ctx))
		})(h)
	}
}

// New builds a host with munin chrome defaults.
func New(root View, opts ...Option) *State {
	vkOpts := muninChrome()
	for _, o := range opts {
		vkOpts = append(vkOpts, vkdeck.Option(o))
	}
	return vkdeck.New(root, vkOpts...)
}

// Run starts the tea program (runtime lives in viewkit/deck).
func Run(root View, opts ...Option) error {
	vkOpts := muninChrome()
	for _, o := range opts {
		vkOpts = append(vkOpts, vkdeck.Option(o))
	}
	if err := vkdeck.Run(root, vkOpts...); err != nil {
		return errs.Wrap(errs.KindInternal, err, "deck program exited with error")
	}
	return nil
}

func muninChrome() []vkdeck.Option {
	return []vkdeck.Option{
		vkdeck.WithChrome(vkdeck.Chrome{
			Brand:      "MUNIN",
			BrandGlyph: glyph.Brand(),
			Subtitle:   "ono-sendai deck",
			ClockGlyph: glyph.Clock(),
		}),
		vkdeck.WithQuitCheck(keymap.IsQuit),
	}
}

// Push is a small helper for tests and callers that hold *State.
func Push(a *State, v View) tea.Cmd { return a.Push(v) }
