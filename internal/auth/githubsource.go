package auth

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/codyconfer/mino/internal/errs"
)

const (
	serviceCredKey = "github-service"
	appCredKey     = "github-app"
	envAppKey      = "MINO_GITHUB_APP_PRIVATE_KEY"
)

type GitHubMechanism int

const (
	GitHubNone GitHubMechanism = iota
	GitHubAppAuth
	GitHubServiceToken
	GitHubCLI
	GitHubEnvToken
	GitHubStoredToken
)

func (m GitHubMechanism) String() string {
	switch m {
	case GitHubAppAuth:
		return "app"
	case GitHubServiceToken:
		return "service token"
	case GitHubCLI:
		return "gh CLI"
	case GitHubEnvToken:
		return "env token"
	case GitHubStoredToken:
		return "stored token"
	}
	return "none"
}

type TokenSource interface {
	Token(ctx context.Context) (string, error)
}

type GitHubSource = TokenSource

type GitHubAppSpec struct {
	ID             string
	InstallationID string
	PrivateKeyPath string
}

func (a GitHubAppSpec) Requested() bool { return a != GitHubAppSpec{} }

type GitHubSpec struct {
	APIURL       string
	App          GitHubAppSpec
	ServiceToken string
	Store        TokenStore
}

type GitHubSelection struct {
	Mech   GitHubMechanism
	Origin string
	APIURL string
	trace  string
	src    GitHubSource
}

func (s GitHubSelection) Token(ctx context.Context) (string, error) {
	if s.src == nil {
		return "", errs.New(errs.KindAuth, "no GitHub authentication available").
			WithHint("configure github.app or github.service_token for a service identity, " +
				"install the gh CLI and run `gh auth login`, set GITHUB_TOKEN, or run `mino login github`")
	}
	return s.src.Token(ctx)
}

func (s GitHubSelection) UsesGHCLI() bool { return s.Mech == GitHubCLI }

func (s GitHubSelection) ServiceIdentity() bool {
	return s.Mech == GitHubAppAuth || s.Mech == GitHubServiceToken
}

func (s GitHubSelection) Authenticated() bool { return s.Mech != GitHubNone }

func (s GitHubSelection) Trace() string { return s.trace }

func (s GitHubSelection) Invalidate() {
	if inv, ok := s.src.(interface{ Invalidate() }); ok {
		inv.Invalidate()
	}
}

type staticSource struct{ token string }

func (s staticSource) Token(context.Context) (string, error) { return s.token, nil }

func StaticGitHubToken(tok string) GitHubSource { return staticSource{token: tok} }

func StaticGitHubSelection(tok, origin string) GitHubSelection {
	return GitHubSelection{Mech: GitHubEnvToken, Origin: origin, src: staticSource{token: tok}}
}

func CLIGitHubSelection(apiURL string) GitHubSelection {
	return GitHubSelection{
		Mech:   GitHubCLI,
		Origin: "the gh CLI",
		APIURL: apiURL,
		src:    &cliSource{apiURL: apiURL},
	}
}

type cliSource struct {
	apiURL string

	mu   sync.Mutex
	tok  string
	done bool
}

func (c *cliSource) Token(ctx context.Context) (string, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.done {
		return c.tok, nil
	}
	args := append([]string{"auth", "token"}, GHHostFlag(c.apiURL)...)
	out, err := GH(ctx, args...)
	if err != nil {
		return "", err
	}
	tok := strings.TrimSpace(string(out))
	if tok == "" {
		return "", errs.New(errs.KindAuth, "the gh CLI returned no token").
			WithHint("run `gh auth login`, or set GITHUB_TOKEN")
	}
	c.tok, c.done = tok, true
	return tok, nil
}

func SelectGitHub(spec GitHubSpec) (GitHubSelection, error) {
	tiers := make([]string, 0, 6)
	note := func(format string, args ...any) { tiers = append(tiers, fmt.Sprintf(format, args...)) }

	sel := GitHubSelection{APIURL: spec.APIURL}
	finish := func(mech GitHubMechanism, origin string, src GitHubSource) (GitHubSelection, error) {
		note("-> selected %s (%s)", mech, origin)
		sel.Mech, sel.Origin, sel.src = mech, origin, src
		sel.trace = "github: auth tiers: " + strings.Join(tiers, " ")
		return sel, nil
	}

	if spec.App.Requested() {
		note("app=configured")
		src, origin, err := newAppSource(spec)
		if err != nil {
			return GitHubSelection{}, err
		}
		return finish(GitHubAppAuth, origin, src)
	}
	note("app=unset")

	if tok := strings.TrimSpace(spec.ServiceToken); tok != "" {
		note("service_token=set")
		return finish(GitHubServiceToken, "github.service_token", staticSource{token: tok})
	}
	note("service_token=unset")

	if c, ok := getCred(spec.Store, serviceCredKey); ok && c.AccessToken != "" {
		note("store:%s=hit", serviceCredKey)
		return finish(GitHubServiceToken, "sealed store "+strconv.Quote(serviceCredKey),
			staticSource{token: c.AccessToken})
	}
	note("store:%s=miss", serviceCredKey)

	if GHAvailable() {
		note("gh=available")
		cli := CLIGitHubSelection(spec.APIURL)
		return finish(GitHubCLI, cli.Origin, cli.src)
	}
	note("gh=absent")

	if tok, origin := GitHubToken(spec.Store); tok != "" {
		note("ambient=%s", origin)
		mech := GitHubEnvToken
		if origin == originStoredToken {
			mech = GitHubStoredToken
		}
		return finish(mech, origin, staticSource{token: tok})
	}
	note("ambient=none")

	sel.trace = "github: auth tiers: " + strings.Join(tiers, " ") + " -> none"
	return sel, nil
}

func appKeyPEM(spec GitHubSpec) ([]byte, string, error) {
	if raw := strings.TrimSpace(os.Getenv(envAppKey)); raw != "" {
		return decodeAppKey(raw), "$" + envAppKey, nil
	}
	if path := strings.TrimSpace(spec.App.PrivateKeyPath); path != "" {
		b, err := os.ReadFile(path)
		if err != nil {
			return nil, "", errs.Wrapf(errs.KindConfig, err, "reading github.app.private_key_path %s", path).
				WithHint("the path must be readable by the user mino runs as")
		}
		return b, "github.app.private_key_path (" + path + ")", nil
	}
	if c, ok := getCred(spec.Store, appCredKey); ok && c.AccessToken != "" {
		return decodeAppKey(c.AccessToken), "sealed store " + strconv.Quote(appCredKey), nil
	}
	return nil, "", nil
}
