package gitea

import (
	"context"
	"encoding/json"
	"strings"
	"sync"

	"github.com/codyconfer/mino/internal/errs"
)

type viewerCache struct {
	configured string

	mu    sync.Mutex
	login string
	done  bool
}

func (s *Signal) resolveViewer(ctx context.Context, q Query) (Query, error) {
	if !q.NeedsViewerLogin() {
		return q, nil
	}
	login, err := s.viewer.resolve(ctx, s.backend)
	if err != nil {
		return Query{}, err
	}
	return q.WithViewer(login), nil
}

func (v *viewerCache) resolve(ctx context.Context, backend Backend) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if login := strings.TrimSpace(v.configured); login != "" {
		return login, nil
	}
	if v.done {
		return v.login, nil
	}
	raw, err := backend.Whoami(ctx)
	if err != nil {
		return "", errs.Wrap(errs.KindOf(err), err, "gitea: resolving @me")
	}
	login, err := parseWhoami(raw)
	if err != nil {
		return "", err
	}
	v.login, v.done = login, true
	return login, nil
}

func parseWhoami(raw []byte) (string, error) {
	var u struct {
		Login string `json:"login"`
	}
	if err := json.Unmarshal(raw, &u); err != nil {
		return "", errs.Wrap(errs.KindSignal, err, "gitea: decoding the authenticated user")
	}
	if u.Login == "" {
		return "", errs.New(errs.KindConfig, "gitea: the instance named no authenticated user for @me").
			WithHint("set gitea.viewer to the login mino should stand in for")
	}
	return u.Login, nil
}
