package auth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"fmt"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/codyconfer/viewkit/browser"

	"github.com/codyconfer/munin/internal/errs"
)

const loopbackTimeout = 3 * time.Minute

var openBrowser = browser.Open

func randomState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func loopbackAuthCode(ctx context.Context, w io.Writer, service string, buildURL func(redirect, state string) string) (code, redirect string, err error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return "", "", errs.Wrap(errs.KindAuth, err, "starting local callback server")
	}
	defer ln.Close()

	redirect = fmt.Sprintf("http://127.0.0.1:%d/callback", ln.Addr().(*net.TCPAddr).Port)
	state, err := randomState()
	if err != nil {
		return "", "", errs.Wrap(errs.KindAuth, err, "generating oauth state")
	}

	type result struct {
		code string
		err  error
	}
	resCh := make(chan result, 1)

	mux := http.NewServeMux()
	mux.HandleFunc("/callback", func(rw http.ResponseWriter, r *http.Request) {
		rw.Header().Set("Content-Type", "text/plain; charset=utf-8")
		q := r.URL.Query()
		if e := q.Get("error"); e != "" {
			fmt.Fprintf(rw, "Authorization failed: %s. You can close this window.", e)
			resCh <- result{err: errs.Newf(errs.KindAuth, "authorization error: %s", e)}
			return
		}
		if subtle.ConstantTimeCompare([]byte(q.Get("state")), []byte(state)) != 1 {
			http.Error(rw, "state mismatch", http.StatusBadRequest)
			resCh <- result{err: errs.New(errs.KindAuth, "state mismatch (possible CSRF); aborting")}
			return
		}
		fmt.Fprintf(rw, "munin is now authorized for %s. You can close this window and return to the terminal.", service)
		resCh <- result{code: q.Get("code")}
	})
	srv := &http.Server{Handler: mux}
	go func() { _ = srv.Serve(ln) }()
	defer srv.Close()

	authURL := buildURL(redirect, state)
	fmt.Fprintf(w, "\nOpen this URL to authorize munin for %s:\n\n  %s\n\nWaiting for authorization…\n", service, authURL)
	_ = openBrowser(authURL)

	select {
	case <-ctx.Done():
		return "", "", ctx.Err()
	case <-time.After(loopbackTimeout):
		return "", "", errs.New(errs.KindAuth, "timed out waiting for authorization").
			WithHint("run the login command again")
	case res := <-resCh:
		if res.err != nil {
			return "", "", res.err
		}
		if res.code == "" {
			return "", "", errs.New(errs.KindAuth, "no authorization code received")
		}
		return res.code, redirect, nil
	}
}
