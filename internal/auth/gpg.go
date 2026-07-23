package auth

import (
	"bytes"
	"context"
	"os/exec"
	"strings"

	"github.com/codyconfer/munin/internal/errs"
)

func GPG(ctx context.Context, args ...string) ([]byte, error) {
	bin := "gpg"
	if _, err := exec.LookPath(bin); err != nil {
		if _, err2 := exec.LookPath("gpg2"); err2 != nil {
			return nil, errs.New(errs.KindConfig, "gpg is not installed or not on PATH").
				WithHint("install GnuPG to manage a commit-signing key")
		}
		bin = "gpg2"
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, errs.Wrapf(errs.KindConfig, err, "gpg %s: %s", strings.Join(args, " "), msg)
	}
	return stdout.Bytes(), nil
}
