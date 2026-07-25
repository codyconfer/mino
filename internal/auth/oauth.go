package auth

import (
	"context"
	"io"

	sauth "github.com/codyconfer/sisyphus/auth"
	"github.com/codyconfer/viewkit/browser"

	"github.com/codyconfer/munin/internal/errs"
)

func init() {
	sauth.OpenURL = browser.Open
}

var openBrowser = browser.Open

func loopbackAuthCode(ctx context.Context, w io.Writer, service string, buildURL func(redirect, state string) string) (code, redirect string, err error) {
	code, redirect, err = sauth.LoopbackAuthCode(ctx, w, service, sauth.LoopbackOptions{
		Product: "munin",
		Open:    openBrowser,
	}, buildURL)
	if err != nil {
		return "", "", errs.Wrap(errs.KindAuth, err, "oauth loopback")
	}
	return code, redirect, nil
}
