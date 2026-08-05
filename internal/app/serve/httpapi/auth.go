package httpapi

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/log"
)

const (
	maxPendingAuths     = 16
	maxPendingPerIP     = 2
	maxAuthBodyBytes    = 4 << 10
	maxAuthOutbound     = 2
	authOutboundTimeout = 10 * time.Second
	maxPollViolations   = 10
	authStartWindow     = 5 * time.Minute
	maxStartsPerIP      = 3
	maxRateEntries      = 512
)

var authClock = time.Now

type pendingAuth struct {
	provider   string
	code       string
	remoteIP   string
	flowID     string
	interval   time.Duration
	nextPollAt time.Time
	expiresAt  time.Time
	violations int
	polling    bool
}

type rateEntry struct {
	starts []time.Time
	seen   time.Time
}

func (a *API) hostGuardOnly() gin.HandlerFunc {
	return func(c *gin.Context) {
		if a.hostGuard && !a.loopbackHost(c.Request.Host) {
			c.Header("WWW-Authenticate", `Bearer realm="mino"`)
			abortErrStatus(c, http.StatusUnauthorized, errs.KindAuth,
				"missing or invalid bearer token", a.authHint)
			return
		}
		c.Next()
	}
}

func (a *API) authDevice(c *gin.Context) {
	if !a.requireJSON(c) {
		return
	}
	name := strings.ToLower(strings.TrimSpace(c.Param("provider")))
	p, ok := a.identityProvider(name)
	if !ok {
		abortErr(c, withStatus(http.StatusNotFound, errs.Newf(errs.KindUsage, "no login provider named %q", clip(name, 32)).
			WithHint("this build serves %s", strings.Join(a.providerNames, ", "))))
		return
	}
	ip := c.RemoteIP()
	if !a.allowStart(ip) {
		a.tooManyAuth(c, "too many sign-in attempts", "wait a few minutes and start again")
		return
	}
	if !a.canStartPending(ip) {
		a.tooManyAuth(c, "too many sign-ins already in progress",
			"finish or abandon one, then start again")
		return
	}
	start, err := a.startDevice(c.Request.Context(), p)
	if err != nil {
		abortErr(c, err)
		return
	}
	id, flowID, err := a.trackPending(name, ip, start)
	if err != nil {
		abortErr(c, err)
		return
	}
	a.auditAuth("auth.device.start", map[string]string{
		"provider": name, "flow_id": flowID, "remote_ip": ip,
	})
	uri := start.VerificationURI
	renderJSON(c, http.StatusCreated, map[string]any{
		"auth_id":          id,
		"user_code":        start.UserCode,
		"verification_uri": uri,
		"interval":         int(start.Interval.Seconds()),
		"expires_in":       int(start.ExpiresIn.Seconds()),
	})
}

type authTokenRequest struct {
	AuthID string `json:"auth_id"`
}

func (a *API) authToken(c *gin.Context) {
	if !a.requireJSON(c) {
		return
	}
	name := strings.ToLower(strings.TrimSpace(c.Param("provider")))
	p, ok := a.identityProvider(name)
	if !ok {
		abortErr(c, withStatus(http.StatusNotFound, errs.Newf(errs.KindUsage, "no login provider named %q", clip(name, 32)).
			WithHint("this build serves %s", strings.Join(a.providerNames, ", "))))
		return
	}
	var req authTokenRequest
	if err := readAuthBody(c, &req); err != nil {
		abortErr(c, err)
		return
	}
	if strings.TrimSpace(req.AuthID) == "" {
		abortErr(c, errs.New(errs.KindUsage, "auth_id is required").
			WithHint("use the auth_id from POST /api/v1/auth/device/%s", name))
		return
	}
	key := hashToken(req.AuthID)

	pend, wait, err := a.claimPoll(key, name)
	if err != nil {
		abortErr(c, err)
		return
	}
	if wait > 0 {
		a.retryAfter(c, wait)
		abortErrStatus(c, http.StatusTooManyRequests, errs.KindUsage,
			"that authorization was polled too soon",
			"wait for the interval the start response reported")
		return
	}
	defer a.finishPoll(key)

	res, perr := a.pollDevice(c.Request.Context(), p, pend.code)
	if perr != nil {
		a.advancePoll(key)
		abortErr(c, perr)
		return
	}
	switch {
	case res.Denied:
		a.dropPending(key)
		a.auditAuth("auth.device.denied", map[string]string{"provider": name, "flow_id": pend.flowID})
		a.gonePending(c, name)
		return
	case res.Expired:
		a.dropPending(key)
		a.auditAuth("auth.device.expired", map[string]string{"provider": name, "flow_id": pend.flowID})
		a.gonePending(c, name)
		return
	case res.Pending:
		interval := a.advancePollSlowDown(key, res.SlowDown)
		a.retryAfter(c, interval)
		renderJSON(c, http.StatusAccepted, map[string]any{
			"status":   "pending",
			"interval": int(interval.Seconds()),
		})
		return
	}

	if !a.takePending(key) {
		a.gonePending(c, name)
		return
	}
	a.mintSession(c, name, pend, res)
}

func (a *API) mintSession(c *gin.Context, provider string, pend pendingAuth, res DeviceResult) {
	if res.Login == "" || res.UserID == 0 {
		abortErr(c, errs.New(errs.KindSignal, "the provider returned no identity for that authorization"))
		return
	}
	if res.Kind != "" && !strings.EqualFold(res.Kind, "user") {
		a.auditAuth("auth.denied.kind", map[string]string{
			"provider": provider, "login": res.Login, "kind": res.Kind, "flow_id": pend.flowID,
		})
		abortErr(c, withStatus(http.StatusForbidden,
			errs.Newf(errs.KindAuth, "%s is a %s account, not a person", res.Login, strings.ToLower(res.Kind)).
				WithHint("sign in as a user account")))
		return
	}
	if !a.permitted(res.Login) {
		a.auditAuth("auth.denied.allowlist", map[string]string{
			"provider": provider, "login": res.Login, "user_id": itoa(res.UserID),
			"remote_ip": pend.remoteIP, "flow_id": pend.flowID,
		})
		log.Warnf("serve: http api: refused a session for %s:%s (not on daemon.http.identity.allowed_logins)",
			provider, res.Login)
		abortErr(c, withStatus(http.StatusForbidden,
			errs.Newf(errs.KindAuth, "%s is not permitted to authenticate here", res.Login).
				WithHint("ask the operator to add it to daemon.http.identity.allowed_logins")))
		return
	}
	tok, rec, err := a.sessions.mint(c.Request.Context(), provider, res.Login, res.UserID, a.binding())
	if err != nil {
		abortErr(c, err)
		return
	}
	a.auditAuth("auth.session.mint", map[string]string{
		"provider": provider, "login": rec.Login, "user_id": itoa(rec.UserID),
		"session_id": rec.ID, "expires_at": rec.ExpiresAt.UTC().Format(time.RFC3339),
		"remote_ip": pend.remoteIP, "flow_id": pend.flowID,
	})
	log.Infof("serve: http api: session %s minted for %s:%s (expires %s)",
		rec.ID, provider, rec.Login, rec.ExpiresAt.UTC().Format(time.RFC3339))
	renderJSON(c, http.StatusOK, map[string]any{
		"session_token": tok,
		"token_type":    "Bearer",
		"provider":      provider,
		"login":         rec.Login,
		"session_id":    rec.ID,
		"expires_at":    rec.ExpiresAt.UTC().Format(time.RFC3339),
	})
}

func (a *API) gonePending(c *gin.Context, provider string) {
	abortErr(c, withStatus(http.StatusGone, errs.New(errs.KindUsage, "that authorization is no longer valid").
		WithHint("POST /api/v1/auth/device/%s to start a new one", provider)))
}

func (a *API) permitted(login string) bool {
	return a.allowed[strings.ToLower(strings.TrimSpace(login))]
}

func (a *API) identityProvider(name string) (IdentityProvider, bool) {
	if !a.identity || name == "" {
		return nil, false
	}
	p, ok := a.deps.Identity[name]
	return p, ok
}

func (a *API) binding() string {
	if a.deps.AuthBinding == nil {
		return ""
	}
	return a.deps.AuthBinding()
}

func (a *API) auditAuth(event string, attrs map[string]string) {
	if a.deps.AuditAuth == nil {
		return
	}
	a.deps.AuditAuth(event, attrs)
}

func (a *API) startDevice(ctx context.Context, p IdentityProvider) (DeviceAuth, error) {
	if !a.acquireAuthSlot() {
		return DeviceAuth{}, withStatus(http.StatusTooManyRequests,
			errs.New(errs.KindUsage, "too many sign-ins are talking to the provider").
				WithHint("retry shortly"))
	}
	defer a.releaseAuthSlot()
	octx, cancel := context.WithTimeout(ctx, authOutboundTimeout)
	defer cancel()
	return p.Start(octx)
}

func (a *API) pollDevice(ctx context.Context, p IdentityProvider, code string) (DeviceResult, error) {
	if !a.acquireAuthSlot() {
		return DeviceResult{}, withStatus(http.StatusTooManyRequests,
			errs.New(errs.KindUsage, "too many sign-ins are talking to the provider").
				WithHint("retry shortly"))
	}
	defer a.releaseAuthSlot()
	octx, cancel := context.WithTimeout(ctx, authOutboundTimeout)
	defer cancel()
	return p.Poll(octx, code)
}

func (a *API) acquireAuthSlot() bool {
	select {
	case a.authSlot <- struct{}{}:
		return true
	default:
		return false
	}
}

func (a *API) releaseAuthSlot() { <-a.authSlot }

func (a *API) requireJSON(c *gin.Context) bool {
	ct := strings.TrimSpace(strings.Split(c.GetHeader("Content-Type"), ";")[0])
	if strings.EqualFold(ct, "application/json") {
		return true
	}
	abortErrStatus(c, http.StatusUnsupportedMediaType, errs.KindUsage,
		"this endpoint needs Content-Type: application/json",
		"send -H 'Content-Type: application/json'")
	return false
}

func readAuthBody(c *gin.Context, v any) error {
	if c.Request.Body == nil {
		return nil
	}
	body, err := io.ReadAll(http.MaxBytesReader(c.Writer, c.Request.Body, maxAuthBodyBytes))
	if err != nil {
		return errs.Wrap(errs.KindUsage, err, "reading the request body")
	}
	if len(strings.TrimSpace(string(body))) == 0 {
		return nil
	}
	dec := json.NewDecoder(strings.NewReader(string(body)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return errs.Wrap(errs.KindUsage, err, "parsing the request body")
	}
	return nil
}

func (a *API) retryAfter(c *gin.Context, d time.Duration) {
	secs := int(math.Ceil(d.Seconds()))
	if secs < 1 {
		secs = 1
	}
	c.Header("Retry-After", itoa(int64(secs)))
}

func (a *API) tooManyAuth(c *gin.Context, msg, hint string) {
	a.retryAfter(c, minDevicePollGrace)
	abortErrStatus(c, http.StatusTooManyRequests, errs.KindUsage, msg, hint)
}

const minDevicePollGrace = 30 * time.Second

func (a *API) allowStart(ip string) bool {
	now := authClock()
	a.pendingMu.Lock()
	defer a.pendingMu.Unlock()
	if len(a.rate) >= maxRateEntries {
		a.evictStalestRate()
	}
	e := a.rate[ip]
	if e == nil {
		e = &rateEntry{}
		a.rate[ip] = e
	}
	e.seen = now
	kept := e.starts[:0]
	for _, t := range e.starts {
		if now.Sub(t) < authStartWindow {
			kept = append(kept, t)
		}
	}
	e.starts = kept
	if len(e.starts) >= maxStartsPerIP {
		return false
	}
	e.starts = append(e.starts, now)
	return true
}

func (a *API) evictStalestRate() {
	var oldestIP string
	var oldest time.Time
	for ip, e := range a.rate {
		if oldestIP == "" || e.seen.Before(oldest) {
			oldestIP, oldest = ip, e.seen
		}
	}
	if oldestIP != "" {
		delete(a.rate, oldestIP)
	}
}

func (a *API) canStartPending(ip string) bool {
	now := authClock()
	a.pendingMu.Lock()
	defer a.pendingMu.Unlock()
	for k, p := range a.pending {
		if now.After(p.expiresAt) && !p.polling {
			delete(a.pending, k)
		}
	}
	if len(a.pending) >= maxPendingAuths {
		return false
	}
	mine := 0
	for _, p := range a.pending {
		if p.remoteIP == ip {
			mine++
		}
	}
	return mine < maxPendingPerIP
}

func (a *API) trackPending(provider, ip string, start DeviceAuth) (authID, flowID string, err error) {
	raw := make([]byte, 32)
	if _, err := rand.Read(raw); err != nil {
		return "", "", errs.Wrap(errs.KindInternal, err, "generating an auth id")
	}
	fid := make([]byte, 6)
	if _, err := rand.Read(fid); err != nil {
		return "", "", errs.Wrap(errs.KindInternal, err, "generating a flow id")
	}
	authID = base64.RawURLEncoding.EncodeToString(raw)
	flowID = hex.EncodeToString(fid)
	now := authClock()
	a.pendingMu.Lock()
	defer a.pendingMu.Unlock()
	if len(a.pending) >= maxPendingAuths {
		return "", "", withStatus(http.StatusTooManyRequests,
			errs.New(errs.KindUsage, "too many sign-ins already in progress").
				WithHint("finish or abandon one, then start again"))
	}
	mine := 0
	for _, p := range a.pending {
		if p.remoteIP == ip {
			mine++
		}
	}
	if mine >= maxPendingPerIP {
		return "", "", withStatus(http.StatusTooManyRequests,
			errs.New(errs.KindUsage, "too many sign-ins already in progress").
				WithHint("finish or abandon one, then start again"))
	}
	a.pending[hashToken(authID)] = &pendingAuth{
		provider:   provider,
		code:       start.DeviceCode,
		remoteIP:   ip,
		flowID:     flowID,
		interval:   start.Interval,
		nextPollAt: now.Add(start.Interval),
		expiresAt:  now.Add(start.ExpiresIn),
	}
	return authID, flowID, nil
}

func (a *API) claimPoll(key, provider string) (pendingAuth, time.Duration, error) {
	now := authClock()
	a.pendingMu.Lock()
	defer a.pendingMu.Unlock()
	p, ok := a.pending[key]
	if !ok || p.provider != provider {
		return pendingAuth{}, 0, withStatus(http.StatusGone,
			errs.New(errs.KindUsage, "that authorization is no longer valid").
				WithHint("POST /api/v1/auth/device/%s to start a new one", provider))
	}
	if now.After(p.expiresAt) {
		delete(a.pending, key)
		return pendingAuth{}, 0, withStatus(http.StatusGone,
			errs.New(errs.KindUsage, "that authorization is no longer valid").
				WithHint("POST /api/v1/auth/device/%s to start a new one", provider))
	}
	if p.polling {
		return pendingAuth{}, 0, withStatus(http.StatusConflict,
			errs.New(errs.KindUsage, "that authorization is already being polled").
				WithHint("one poll at a time"))
	}
	if now.Before(p.nextPollAt) {
		p.violations++
		if p.violations > maxPollViolations {
			delete(a.pending, key)
			return pendingAuth{}, 0, withStatus(http.StatusGone,
				errs.New(errs.KindUsage, "that authorization is no longer valid").
					WithHint("POST /api/v1/auth/device/%s to start a new one", provider))
		}
		return pendingAuth{}, p.nextPollAt.Sub(now), nil
	}
	p.polling = true
	return *p, 0, nil
}

func (a *API) finishPoll(key string) {
	a.pendingMu.Lock()
	defer a.pendingMu.Unlock()
	if p, ok := a.pending[key]; ok {
		p.polling = false
	}
}

func (a *API) advancePoll(key string) {
	a.pendingMu.Lock()
	defer a.pendingMu.Unlock()
	if p, ok := a.pending[key]; ok {
		p.nextPollAt = authClock().Add(p.interval)
	}
}

func (a *API) advancePollSlowDown(key string, slow bool) time.Duration {
	a.pendingMu.Lock()
	defer a.pendingMu.Unlock()
	p, ok := a.pending[key]
	if !ok {
		return minDevicePollGrace
	}
	if slow {
		p.interval += 5 * time.Second
	}
	p.nextPollAt = authClock().Add(p.interval)
	return p.interval
}

func (a *API) dropPending(key string) {
	a.pendingMu.Lock()
	defer a.pendingMu.Unlock()
	delete(a.pending, key)
}

func (a *API) takePending(key string) bool {
	a.pendingMu.Lock()
	defer a.pendingMu.Unlock()
	if _, ok := a.pending[key]; !ok {
		return false
	}
	delete(a.pending, key)
	return true
}

func (a *API) PendingAuths() int {
	a.pendingMu.Lock()
	defer a.pendingMu.Unlock()
	return len(a.pending)
}

func clip(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }
