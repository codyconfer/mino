package httpapi

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/log"
)

const (
	sessionTokenPrefix = "mino_s_"
	principalKey       = "mino.principal"
)

type Session struct {
	ID        string    `json:"id"`
	Provider  string    `json:"provider"`
	Login     string    `json:"login"`
	UserID    int64     `json:"user_id"`
	Binding   string    `json:"binding"`
	CreatedAt time.Time `json:"created_at"`
	ExpiresAt time.Time `json:"expires_at"`
}

type SessionStore interface {
	Put(ctx context.Context, hash string, s Session) error
	Delete(ctx context.Context, hash string) error
	List(ctx context.Context) (map[string]Session, error)
}

type Principal struct {
	Kind      string
	Provider  string
	Login     string
	UserID    int64
	SessionID string
	ExpiresAt time.Time
	hash      string
}

type sessions struct {
	store SessionStore
	ttl   time.Duration

	mu sync.RWMutex
	by map[string]Session
}

func newSessions(store SessionStore, ttl time.Duration) *sessions {
	return &sessions{store: store, ttl: ttl, by: map[string]Session{}}
}

func (s *sessions) load(ctx context.Context, binding string) {
	if s == nil || s.store == nil {
		return
	}
	recs, err := s.store.List(ctx)
	if err != nil {
		log.Warnf("serve: http api: reading stored api sessions: %v", err)
		return
	}
	now := time.Now()
	kept := 0
	s.mu.Lock()
	for hash, rec := range recs {
		if now.After(rec.ExpiresAt) || rec.Binding != binding {
			s.mu.Unlock()
			_ = s.store.Delete(ctx, hash)
			s.mu.Lock()
			continue
		}
		s.by[hash] = rec
		kept++
	}
	s.mu.Unlock()
	if kept > 0 {
		log.Debugf("serve: http api: restored %d api session(s)", kept)
	}
}

func (s *sessions) mint(ctx context.Context, provider, login string, userID int64, binding string) (string, Session, error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", Session{}, errs.Wrap(errs.KindInternal, err, "generating a session token")
	}
	id := make([]byte, 6)
	if _, err := rand.Read(id); err != nil {
		return "", Session{}, errs.Wrap(errs.KindInternal, err, "generating a session id")
	}
	tok := sessionTokenPrefix + base64.RawURLEncoding.EncodeToString(raw)
	now := time.Now()
	rec := Session{
		ID:        hex.EncodeToString(id),
		Provider:  provider,
		Login:     login,
		UserID:    userID,
		Binding:   binding,
		CreatedAt: now,
		ExpiresAt: now.Add(s.ttl),
	}
	hash := hashToken(tok)
	if err := s.store.Put(ctx, hash, rec); err != nil {
		return "", Session{}, err
	}
	s.mu.Lock()
	s.by[hash] = rec
	s.mu.Unlock()
	return tok, rec, nil
}

func (s *sessions) lookup(hash, binding string) (Session, bool) {
	if s == nil {
		return Session{}, false
	}
	s.mu.RLock()
	rec, ok := s.by[hash]
	s.mu.RUnlock()
	if !ok {
		return Session{}, false
	}
	if time.Now().After(rec.ExpiresAt) || rec.Binding != binding {
		s.revoke(context.Background(), hash)
		return Session{}, false
	}
	return rec, true
}

func (s *sessions) revoke(ctx context.Context, hash string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	delete(s.by, hash)
	s.mu.Unlock()
	if s.store != nil {
		if err := s.store.Delete(ctx, hash); err != nil {
			log.Debugf("serve: http api: dropping a stored session: %v", err)
		}
	}
}

func hashToken(tok string) string {
	sum := sha256.Sum256([]byte(tok))
	return hex.EncodeToString(sum[:])
}

func MarshalSession(s Session) (string, error) {
	b, err := json.Marshal(s)
	if err != nil {
		return "", errs.Wrap(errs.KindInternal, err, "encoding a session record")
	}
	return string(b), nil
}

func UnmarshalSession(raw string) (Session, error) {
	var s Session
	if err := json.Unmarshal([]byte(raw), &s); err != nil {
		return Session{}, errs.Wrap(errs.KindStore, err, "decoding a session record")
	}
	return s, nil
}

type sessionInfo struct {
	Kind      string `json:"kind"`
	Provider  string `json:"provider,omitempty"`
	Login     string `json:"login,omitempty"`
	UserID    int64  `json:"user_id,omitempty"`
	SessionID string `json:"session_id,omitempty"`
	ExpiresAt string `json:"expires_at,omitempty"`
}

func (a *API) authSession(c *gin.Context) {
	p, ok := principalOf(c)
	if !ok {
		abortErrStatus(c, http.StatusInternalServerError, errs.KindInternal,
			"the request carried no principal", "the serve log has the detail")
		return
	}
	out := sessionInfo{Kind: p.Kind}
	if p.Kind == "session" {
		out.Provider, out.Login, out.UserID = p.Provider, p.Login, p.UserID
		out.SessionID = p.SessionID
		out.ExpiresAt = p.ExpiresAt.UTC().Format(time.RFC3339)
	}
	renderJSON(c, http.StatusOK, out)
}

func (a *API) authLogout(c *gin.Context) {
	p, ok := principalOf(c)
	if !ok || p.Kind != "session" {
		abortErrStatus(c, http.StatusConflict, errs.KindUsage,
			"that credential is not a session",
			"rotate the static token by deleting it and restarting serve")
		return
	}
	a.sessions.revoke(c.Request.Context(), p.hash)
	a.auditAuth("auth.session.revoke", map[string]string{
		"provider": p.Provider, "login": p.Login, "session_id": p.SessionID, "by": "self",
	})
	log.Infof("serve: http api: session %s revoked (%s:%s)", p.SessionID, p.Provider, p.Login)
	c.Status(http.StatusNoContent)
}

func principalOf(c *gin.Context) (Principal, bool) {
	v, ok := c.Get(principalKey)
	if !ok {
		return Principal{}, false
	}
	p, ok := v.(Principal)
	return p, ok
}
