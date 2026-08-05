package httpapi

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"

	sconfig "github.com/codyconfer/sisyphus/config"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
)

// minTokenLen rejects a configured token too short to survive guessing.
const minTokenLen = 16

// ResolveToken returns the bearer token and where it came from, generating and
// persisting one when neither config nor the token file supplies it.
func ResolveToken(home, configured string) (tok, source string, err error) {
	path := config.HTTPTokenPath(home)
	if t := strings.TrimSpace(configured); t != "" {
		if len(t) < minTokenLen {
			return "", "", errs.Newf(errs.KindConfig, "daemon.http.token is only %d characters", len(t)).
				WithHint("use at least %d, or leave it unset and mino generates one in %s", minTokenLen, path)
		}
		return t, "daemon.http.token", nil
	}
	if b, readErr := os.ReadFile(path); readErr == nil {
		if t := strings.TrimSpace(string(b)); t != "" {
			return t, path, nil
		}
	}
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", errs.Wrap(errs.KindInternal, err, "generating an http api token")
	}
	tok = base64.RawURLEncoding.EncodeToString(raw)
	// WriteItem is atomic and mode 0600, and creates .data/ if needed.
	if _, err := sconfig.WriteItem(config.DataDir(home), config.HTTPTokenFile, []byte(tok+"\n")); err != nil {
		return "", "", errs.Wrap(errs.KindStore, err, "writing the http api token")
	}
	return tok, path, nil
}

func LoopbackBind(host string) bool {
	h := strings.TrimSpace(host)
	if h == "" || strings.EqualFold(h, "localhost") {
		return true
	}
	ip := net.ParseIP(strings.Trim(h, "[]"))
	return ip != nil && ip.IsLoopback()
}

func (a *API) authenticate(r *http.Request) (Principal, bool) {
	if a.hostGuard && !a.loopbackHost(r.Host) {
		return Principal{}, false
	}
	scheme, presented, ok := strings.Cut(r.Header.Get("Authorization"), " ")
	if !ok || !strings.EqualFold(scheme, "bearer") {
		return Principal{}, false
	}
	got := strings.TrimSpace(presented)
	if got == "" {
		return Principal{}, false
	}
	staticOK := a.token != "" && subtle.ConstantTimeCompare([]byte(got), []byte(a.token)) == 1
	if staticOK {
		return Principal{Kind: "token"}, true
	}
	if !a.identity {
		return Principal{}, false
	}
	hash := hashToken(got)
	rec, found := a.sessions.lookup(hash, a.binding())
	if !found {
		return Principal{}, false
	}
	return Principal{
		Kind:      "session",
		Provider:  rec.Provider,
		Login:     rec.Login,
		UserID:    rec.UserID,
		SessionID: rec.ID,
		ExpiresAt: rec.ExpiresAt,
		hash:      hash,
	}, true
}

func (a *API) loopbackHost(host string) bool {
	h := host
	if i := strings.LastIndex(h, ":"); i != -1 && !strings.HasSuffix(h, "]") {
		h = h[:i]
	}
	h = strings.Trim(h, "[]")
	switch h {
	case config.HTTPLoopback, "localhost", "::1":
		return true
	}
	return false
}

func (a *API) authRequired() gin.HandlerFunc {
	return func(c *gin.Context) {
		p, ok := a.authenticate(c.Request)
		if !ok {
			c.Header("WWW-Authenticate", `Bearer realm="mino"`)
			abortErrStatus(c, http.StatusUnauthorized, errs.KindAuth,
				"missing or invalid bearer token", a.authHint)
			return
		}
		c.Set(principalKey, p)
		c.Next()
	}
}
