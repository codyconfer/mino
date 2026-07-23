package auth

import (
	"bytes"
	"context"
	"os/exec"
	"strings"

	"github.com/codyconfer/munin/internal/errs"
)

func Git(ctx context.Context, args ...string) ([]byte, error) {
	if _, err := exec.LookPath("git"); err != nil {
		return nil, errs.New(errs.KindConfig, "git is not installed or not on PATH").
			WithHint("install git to configure a signing key")
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, errs.Wrapf(errs.KindConfig, err, "git %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.Bytes(), nil
}
