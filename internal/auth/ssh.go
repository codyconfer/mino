package auth

import (
	"context"

	"github.com/codyconfer/mino/internal/errs"
)

func SSHKeygen(ctx context.Context, args ...string) ([]byte, error) {
	return runTool(ctx, []string{"ssh-keygen"}, "ssh-keygen", errs.KindConfig,
		"ssh-keygen is not installed or not on PATH",
		"install OpenSSH to manage an SSH signing key",
		"",
		args...)
}
