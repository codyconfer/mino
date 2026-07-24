package auth

import (
	"context"

	"github.com/codyconfer/munin/internal/errs"
)

func Git(ctx context.Context, args ...string) ([]byte, error) {
	return runTool(ctx, []string{"git"}, "git", errs.KindConfig,
		"git is not installed or not on PATH",
		"install git to configure a signing key",
		"",
		args...)
}
