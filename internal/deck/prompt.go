package deck

import (
	"errors"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/ui"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/keymap"
)

// Prompt runs spec as a one-shot form in scope (nil snapshots the process
// defaults), defaulting the keys and save action to mino's bindings.
func Prompt(scope *ui.Scope, spec vkdeck.PromptSpec) (map[string]any, error) {
	if scope == nil {
		scope = ui.Default()
	}
	if spec.UI == nil {
		spec.UI = scope
	}
	if spec.Keys == nil {
		spec.Keys = keymap.Form(spec.UI.Keys)
	}
	if spec.Save == "" {
		spec.Save = keymap.Save
	}
	vals, err := vkdeck.Prompt(spec)
	switch {
	case errors.Is(err, vkdeck.ErrCancelled):
		return nil, err
	case err != nil:
		return nil, errs.Wrap(errs.KindInternal, err, "run prompt")
	}
	return vals, nil
}
