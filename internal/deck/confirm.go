package deck

import (
	vkdeck "github.com/codyconfer/viewkit/deck"

	"github.com/codyconfer/mino/internal/errs"
)

func Confirm(title, message, yesLabel, noLabel string) (bool, error) {
	ok, err := vkdeck.Confirm(title, message, yesLabel, noLabel)
	if err != nil {
		return false, errs.Wrap(errs.KindInternal, err, "run confirm prompt")
	}
	return ok, nil
}
