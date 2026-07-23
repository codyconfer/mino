package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
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

func GHAPIGet(ctx context.Context, store TokenStore, apiURL, path string) ([]byte, error) {
	if GHAvailable() {
		return GH(ctx, "api", path)
	}
	tok, _ := GitHubToken(store)
	if tok == "" {
		return nil, errs.New(errs.KindAuth, "no GitHub authentication available").
			WithHint("install the gh CLI and run `gh auth login`, set GITHUB_TOKEN, or run `munin login github`")
	}
	base := strings.TrimRight(apiURL, "/")
	if base == "" {
		base = "https://api.github.com"
	}
	u := base + "/" + strings.TrimLeft(path, "/")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, errs.Wrap(errs.KindSignal, err, "github: building request")
	}
	req.Header.Set("Authorization", "Bearer "+tok)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, errs.Wrap(errs.KindSignal, err, "github: request failed")
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, errs.Newf(errs.KindAuth, "github api %s: %s", resp.Status, strings.TrimSpace(string(body))).
			WithHint("your GitHub token may be missing or lack scopes; run `munin login github` or set $GITHUB_TOKEN")
	}
	if resp.StatusCode >= 400 {
		return nil, errs.Newf(errs.KindSignal, "github api %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return body, nil
}

func GHGraphQL(ctx context.Context, query string, vars map[string]string, out any) error {
	args := []string{"api", "graphql", "-f", "query=" + query}
	for k, v := range vars {
		args = append(args, "-f", k+"="+v)
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
