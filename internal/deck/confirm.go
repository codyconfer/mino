package deck

import (
	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/mino/internal/errs"
)

func Confirm(spec vkdeck.ConfirmSpec) (bool, error) {
	ok, err := vkdeck.Confirm(spec)
	if err != nil {
		return false, errs.Wrap(errs.KindInternal, err, "run confirm prompt")
	}
	return ok, nil
}
