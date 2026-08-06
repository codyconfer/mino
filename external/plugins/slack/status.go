package slack

import (
	"sync"
	"time"

	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/mino/cmd"
	"github.com/codyconfer/mino/external/plugins/internal/slackauth"
)

const statusTTL = 60 * time.Second

var authStatus struct {
	mu   sync.Mutex
	at   time.Time
	ok   bool
	done bool
}

func StatusContribution() glyph.StatusContribution {
	return glyph.StatusContribution{
		BrandGlyph: glyph.ResolveID(SignalName),
		Info:       func() string { return SignalName },
		Status: func() (string, glyph.Severity) {
			if tokenPresent() {
				return glyph.StatusOK(), glyph.SeverityPositive
			}
			return glyph.StatusMuted(), glyph.SeverityNeutral
		},
	}
}

func tokenPresent() bool {
	authStatus.mu.Lock()
	defer authStatus.mu.Unlock()
	if authStatus.done && time.Since(authStatus.at) < statusTTL {
		return authStatus.ok
	}
	cfg := slackauth.FromHost(cmd.Host(SignalName))
	authStatus.ok = slackauth.Authed(cfg.Store, cfg.TokenEnv)
	authStatus.at = time.Now()
	authStatus.done = true
	return authStatus.ok
}
