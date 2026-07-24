package auth

import (
	"context"

	"github.com/codyconfer/munin/internal/errs"
)

func GPG(ctx context.Context, args ...string) ([]byte, error) {
	return runTool(ctx, []string{"gpg", "gpg2"}, "gpg", errs.KindConfig,
		"gpg is not installed or not on PATH",
		"install GnuPG to manage a commit-signing key",
		"",
		args...)
}
