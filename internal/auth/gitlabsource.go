package auth

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/codyconfer/mino/internal/errs"
)

const (
	gitlabServiceCredKey = "gitlab-service"
	gitlabCredKey        = "gitlab"
)

type GitLabMechanism int

const (
	GitLabNone GitLabMechanism = iota
	GitLabServiceToken
	GitLabCLI
	GitLabEnvToken
	GitLabStoredToken
)

func (m GitLabMechanism) String() string {
	switch m {
	case GitLabServiceToken:
		return "service token"
	case GitLabCLI:
		return "glab CLI"
	case GitLabEnvToken:
		return "env token"
	case GitLabStoredToken:
		return "stored token"
	}
	return "none"
}

type GitLabSource interface {
	Token(ctx context.Context) (string, error)
}

type GitLabSpec struct {
	APIURL        string
	ServiceToken  string
	OAuthClientID string
	Store         TokenStore
}

type GitLabSelection struct {
	Mech   GitLabMechanism
	Origin string
	APIURL string
	trace  string
	src    GitLabSource
}

func (s GitLabSelection) Token(ctx context.Context) (string, error) {
	if s.src == nil {
		return "", errs.New(errs.KindAuth, "no GitLab authentication available").
			WithHint("configure gitlab.service_token for a service identity, install the glab CLI " +
				"and run `glab auth login`, set GITLAB_TOKEN, or run `mino login gitlab`")
	}
	return s.src.Token(ctx)
}

func (s GitLabSelection) UsesGLabCLI() bool { return s.Mech == GitLabCLI }

func (s GitLabSelection) ServiceIdentity() bool { return s.Mech == GitLabServiceToken }

func (s GitLabSelection) Authenticated() bool { return s.Mech != GitLabNone }

func (s GitLabSelection) Trace() string { return s.trace }

func (s GitLabSelection) Invalidate() {
	if inv, ok := s.src.(interface{ Invalidate() }); ok {
		inv.Invalidate()
	}
}

func StaticGitLabSelection(tok, origin string) GitLabSelection {
	return GitLabSelection{Mech: GitLabEnvToken, Origin: origin, src: staticSource{token: tok}}
}

func CLIGitLabSelection(apiURL string) GitLabSelection {
	return GitLabSelection{
		Mech:   GitLabCLI,
		Origin: "the glab CLI",
		APIURL: apiURL,
		src:    &glabCLISource{apiURL: apiURL},
	}
}

type glabCLISource struct {
	apiURL string

	mu   sync.Mutex
	tok  string
	done bool
}

func (c *glabCLISource) Token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.done {
		return c.tok, nil
	}
	args := append([]string{"auth", "token"}, GLabHostFlag(c.apiURL)...)
	out, err := GLab(ctx, args...)
	if err != nil {
		return "", err
	}
	tok := strings.TrimSpace(string(out))
	if tok == "" {
		return "", errs.New(errs.KindAuth, "the glab CLI returned no token").
			WithHint("run `glab auth login`, upgrade glab if it has no `auth token` subcommand, " +
				"or set GITLAB_TOKEN")
	}
	c.tok, c.done = tok, true
	return tok, nil
}

const refreshExpiryMargin = time.Minute

type refreshingSource struct {
	store       TokenStore
	clientID    string
	instanceURL string

	mu   sync.Mutex
	cred Credential
}

func (r *refreshingSource) Token(ctx context.Context) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.cred.Expiry.IsZero() || time.Until(r.cred.Expiry) > refreshExpiryMargin {
		return r.cred.AccessToken, nil
	}
	if r.cred.RefreshToken == "" || r.clientID == "" {
		return r.cred.AccessToken, nil
	}
	fresh, err := refreshGitLabToken(ctx, r.instanceURL, r.clientID, r.cred.RefreshToken)
	if err != nil {
		return "", err
	}
	r.cred = fresh
	if err := CacheGitLabCredential(r.store, fresh); err != nil {
		return "", errs.Wrap(errs.KindAuth, err, "caching the refreshed GitLab token")
	}
	return fresh.AccessToken, nil
}

func (r *refreshingSource) Invalidate() {
	r.mu.Lock()
	r.cred = Credential{}
	r.mu.Unlock()
}

func SelectGitLab(spec GitLabSpec) (GitLabSelection, error) {
	tiers := make([]string, 0, 5)
	note := func(format string, args ...any) { tiers = append(tiers, fmt.Sprintf(format, args...)) }

	sel := GitLabSelection{APIURL: spec.APIURL}
	finish := func(mech GitLabMechanism, origin string, src GitLabSource) (GitLabSelection, error) {
		note("-> selected %s (%s)", mech, origin)
		sel.Mech, sel.Origin, sel.src = mech, origin, src
		sel.trace = "gitlab: auth tiers: " + strings.Join(tiers, " ")
		return sel, nil
	}

	if tok := strings.TrimSpace(spec.ServiceToken); tok != "" {
		note("service_token=set")
		return finish(GitLabServiceToken, "gitlab.service_token", staticSource{token: tok})
	}
	note("service_token=unset")

	if c, ok := getCred(spec.Store, gitlabServiceCredKey); ok && c.AccessToken != "" {
		note("store:%s=hit", gitlabServiceCredKey)
		return finish(GitLabServiceToken, "sealed store "+strconv.Quote(gitlabServiceCredKey),
			staticSource{token: c.AccessToken})
	}
	note("store:%s=miss", gitlabServiceCredKey)

	if GLabAvailable() {
		note("glab=available")
		cli := CLIGitLabSelection(spec.APIURL)
		return finish(GitLabCLI, cli.Origin, cli.src)
	}
	note("glab=absent")

	if tok, origin := GitLabToken(spec.Store); tok != "" {
		note("ambient=%s", origin)
		if origin == originGitLabStored {
			c, _ := GitLabCredential(spec.Store)
			return finish(GitLabStoredToken, origin, &refreshingSource{
				store:       spec.Store,
				clientID:    spec.OAuthClientID,
				instanceURL: GitLabInstanceURL(spec.APIURL),
				cred:        c,
			})
		}
		return finish(GitLabEnvToken, origin, staticSource{token: tok})
	}
	note("ambient=none")

	sel.trace = "gitlab: auth tiers: " + strings.Join(tiers, " ") + " -> none"
	return sel, nil
}
