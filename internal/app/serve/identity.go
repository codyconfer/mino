package serve

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"

	"github.com/codyconfer/sisyphus/kv"

	"github.com/codyconfer/mino/internal/app/serve/httpapi"
	"github.com/codyconfer/mino/internal/auth"
	"github.com/codyconfer/mino/internal/errs"
)

const nsHTTPSession = "httpsession"

type HTTPIdentityOptions struct {
	Enabled       bool
	Provider      string
	ClientID      string
	Scopes        string
	APIURL        string
	DeviceURL     string
	TokenURL      string
	AllowedLogins []string
	SessionTTL    time.Duration
}

func (o HTTPIdentityOptions) binding(home string) string {
	logins := append([]string(nil), o.AllowedLogins...)
	for i := range logins {
		logins[i] = strings.ToLower(strings.TrimSpace(logins[i]))
	}
	sort.Strings(logins)
	h := sha256.New()
	for _, part := range []string{
		"mino-http-identity-v1", home, o.Provider, o.ClientID, o.Scopes, o.APIURL,
		strings.Join(logins, "\x00"),
	} {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:32]
}

type sessionStore struct{ kv *kv.Store }

func newSessionStore(store *kv.Store) httpapi.SessionStore {
	if store == nil {
		return nil
	}
	return sessionStore{kv: store}
}

func (s sessionStore) Put(ctx context.Context, hash string, rec httpapi.Session) error {
	raw, err := httpapi.MarshalSession(rec)
	if err != nil {
		return err
	}
	if err := s.kv.Put(ctx, nsHTTPSession, hash, raw, rec.ExpiresAt); err != nil {
		return errs.Wrap(errs.KindStore, err, "persisting an api session")
	}
	return nil
}

func (s sessionStore) Delete(ctx context.Context, hash string) error {
	if err := s.kv.Delete(ctx, nsHTTPSession, hash); err != nil {
		return errs.Wrap(errs.KindStore, err, "dropping an api session")
	}
	return nil
}

func (s sessionStore) List(ctx context.Context) (map[string]httpapi.Session, error) {
	entries, err := s.kv.List(ctx, nsHTTPSession)
	if err != nil {
		return nil, errs.Wrap(errs.KindStore, err, "reading api sessions")
	}
	out := make(map[string]httpapi.Session, len(entries))
	for hash, e := range entries {
		rec, err := httpapi.UnmarshalSession(e.Value)
		if err != nil {
			continue
		}
		out[hash] = rec
	}
	return out, nil
}

type githubIdentity struct {
	clientID  string
	scopes    string
	apiURL    string
	deviceURL string
	tokenURL  string
}

func (g githubIdentity) Start(ctx context.Context) (httpapi.DeviceAuth, error) {
	st, err := auth.GitHubDeviceStart(ctx, g.deviceURL, g.clientID, g.scopes)
	if err != nil {
		return httpapi.DeviceAuth{}, err
	}
	return httpapi.DeviceAuth{
		DeviceCode:              st.DeviceCode,
		UserCode:                st.UserCode,
		VerificationURI:         st.VerificationURI,
		VerificationURIComplete: st.VerificationURIComplete,
		Interval:                st.Interval,
		ExpiresIn:               st.ExpiresIn,
	}, nil
}

func (g githubIdentity) Poll(ctx context.Context, deviceCode string) (httpapi.DeviceResult, error) {
	res, err := auth.GitHubDevicePoll(ctx, g.tokenURL, g.clientID, deviceCode)
	if err != nil {
		return httpapi.DeviceResult{}, err
	}
	switch {
	case res.Denied:
		return httpapi.DeviceResult{Denied: true}, nil
	case res.Expired:
		return httpapi.DeviceResult{Expired: true}, nil
	case res.Pending:
		return httpapi.DeviceResult{Pending: true, SlowDown: res.SlowDown}, nil
	}
	id, err := auth.GitHubWhoAmI(ctx, g.apiURL, res.AccessToken)
	if err != nil {
		return httpapi.DeviceResult{}, err
	}
	return httpapi.DeviceResult{Login: id.Login, UserID: id.ID, Kind: id.Type}, nil
}

func apiIdentityProviders(o HTTPIdentityOptions) map[string]httpapi.IdentityProvider {
	if !o.Enabled {
		return nil
	}
	switch o.Provider {
	case "github", "":
		return map[string]httpapi.IdentityProvider{
			"github": githubIdentity{
				clientID:  o.ClientID,
				scopes:    o.Scopes,
				apiURL:    o.APIURL,
				deviceURL: o.DeviceURL,
				tokenURL:  o.TokenURL,
			},
		}
	}
	return nil
}
