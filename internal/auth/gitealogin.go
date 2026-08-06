package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/codyconfer/mino/internal/errs"
)

func LoginGitea(ctx context.Context, store TokenStore, spec GiteaSpec, token string, w io.Writer) error {
	if spec.APIBase() == "" {
		return errs.New(errs.KindConfig, "gitea.url is not set").WithHint("%s", giteaURLHint)
	}
	token = strings.TrimSpace(token)
	if token == "" {
		return errs.New(errs.KindUsage, "no personal access token supplied").
			WithHint("create one at %s with the read:user scope, then run `mino login %s` again",
				GiteaWebURL(spec.WebBase(), "/user/settings/applications"), spec.forge())
	}

	raw, err := GiteaAPIGet(ctx, StaticGiteaSelection(spec, token, "the supplied token"), "user")
	if err != nil {
		return err
	}
	var u struct {
		Login string `json:"login"`
	}
	if err := json.Unmarshal(raw, &u); err != nil {
		return errs.Wrapf(errs.KindSignal, err, "%s: decoding the authenticated user", spec.forge())
	}
	if u.Login == "" {
		return errs.Newf(errs.KindAuth, "%s accepted the token but named no user", spec.forge()).
			WithHint("check that gitea.url points at a Gitea instance, not a proxy in front of one")
	}
	fmt.Fprintf(w, "authenticated as %s\n", u.Login)

	if err := CacheGiteaToken(store, token); err != nil {
		return errs.Wrap(errs.KindAuth, err, "caching token")
	}
	return nil
}

func GiteaAuthed(spec GiteaSpec) bool {
	if spec.APIBase() == "" {
		return false
	}
	sel, err := SelectGitea(spec)
	return err == nil && sel.Authenticated()
}
