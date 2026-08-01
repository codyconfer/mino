package deck

import (
	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/keymap"
)

func Prompt(spec vkdeck.PromptSpec) (map[string]any, bool, error) {
	if spec.Keys == nil {
		spec.Keys = keymap.Form()
	}
	if spec.Save == "" {
		spec.Save = keymap.Save
	}
	vals, ok, err := vkdeck.Prompt(spec)
	if err != nil {
		return nil, false, errs.Wrap(errs.KindInternal, err, "run prompt")
	}
	return vals, ok, nil
}
