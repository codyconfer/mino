package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"os/exec"
	"strings"

	"github.com/codyconfer/munin/internal/errs"
)

func GH(ctx context.Context, args ...string) ([]byte, error) {
	if _, err := exec.LookPath("gh"); err != nil {
		return nil, errs.New(errs.KindAuth, "the GitHub CLI `gh` is not installed or not on PATH").
			WithHint("install gh and run `gh auth login`, or run `munin login github`")
	}
	var stdout, stderr bytes.Buffer
	cmd := exec.CommandContext(ctx, "gh", args...)
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		return nil, errs.Wrapf(errs.KindAuth, err, "gh %s: %s", strings.Join(args, " "), msg).
			WithHint("run `gh auth login` or `munin login github` to (re)authenticate")
	}
	return stdout.Bytes(), nil
}

func GHGraphQL(ctx context.Context, query string, vars map[string]string, out any) error {
	args := []string{"api", "graphql", "-f", "query=" + query}
	for k, v := range vars {
		args = append(args, "-F", k+"="+v)
	}
	raw, err := GH(ctx, args...)
	if err != nil {
		return err
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return errs.Wrap(errs.KindSignal, err, "decoding gh graphql response")
	}
	if err := json.Unmarshal(envelope.Data, out); err != nil {
		return errs.Wrap(errs.KindSignal, err, "decoding gh graphql data")
	}
	return nil
}
