package deck

import (
	"errors"

	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/keymap"
)

func Prompt(spec vkdeck.PromptSpec) (map[string]any, error) {
	if spec.Keys == nil {
		spec.Keys = keymap.Form()
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
