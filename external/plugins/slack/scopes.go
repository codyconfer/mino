package slack

import (
	"strings"

	"github.com/codyconfer/mino/external/plugins/internal/errx"
)

func scopeError(err error, what string) error {
	if err == nil {
		return nil
	}
	switch apiError(err) {
	case "missing_scope", "not_allowed_token_type", "no_permission":
		return errx.Wrapf(err, "slack: not authorized for %s", what).
			WithHint("re-run `mino login slack` to grant the scopes this surface needs, or set `plugins.slack.user_scopes`")
	case "not_in_channel", "channel_not_found":
		return errx.Wrapf(err, "slack: cannot read %s", what).
			WithHint("invite the token's user to the channel, or check the channel name")
	default:
		return err
	}
}

func apiError(err error) string {
	if err == nil {
		return ""
	}
	return strings.TrimSpace(err.Error())
}
