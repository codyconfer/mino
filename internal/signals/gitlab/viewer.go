package gitlab

import (
	"context"
	"encoding/json"
	"sync"

	"github.com/codyconfer/mino/internal/errs"
)

type viewer struct {
	backend Backend

	mu    sync.Mutex
	login string
	fixed bool
	done  bool
	err   error
}

func newViewer(b Backend, configured string) *viewer {
	return &viewer{backend: b, login: configured, fixed: configured != ""}
}

// Login memoizes both the result and the failure, so a broken credential costs one
// /user call rather than one per query.
func (v *viewer) Login(ctx context.Context) (string, error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.fixed || v.done {
		return v.login, v.err
	}
	v.done = true
	if v.backend == nil {
		v.err = errs.New(errs.KindAuth, "gitlab: no backend to resolve the current user")
		return "", v.err
	}
	p, err := v.backend.Get(ctx, "user", nil)
	if err != nil {
		v.err = err
		return "", err
	}
	var u struct {
		Username string `json:"username"`
	}
	if err := json.Unmarshal(p.Body, &u); err != nil {
		v.err = errs.Wrap(errs.KindSignal, err, "gitlab: decoding user")
		return "", v.err
	}
	v.login = u.Username
	return v.login, nil
}
