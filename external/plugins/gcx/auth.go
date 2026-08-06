package gcx

import (
	"context"
	"os"
	"strings"

	"github.com/codyconfer/mino/external/plugins/internal/errx"
	"github.com/codyconfer/mino/plugin"
)

const (
	// DefaultTokenEnv is the env override for the sealed gcx token.
	DefaultTokenEnv = "GCX_TOKEN"
	// CredScope is recorded on the sealed credential.
	CredScope = "irm"
	// IRMAPIPath is the IRM resource root on a Grafana stack.
	IRMAPIPath = "/api/plugins/grafana-irm-app/resources/api/v1"
)

// Config is the resolved gcx host configuration.
type Config struct {
	Store    plugin.CredentialStore
	Settings map[string]any
	TokenEnv string
}

// FromHost reads the gcx configuration off a mino host.
func FromHost(h plugin.Host) Config {
	if h == nil {
		return Config{TokenEnv: DefaultTokenEnv}
	}
	s := h.Settings(SignalName)
	return Config{
		Store:    h.Credentials(),
		Settings: s,
		TokenEnv: plugin.Setting(s, "token_env", DefaultTokenEnv),
	}
}

// FromBuildContext reads the gcx configuration off a signal build context.
func FromBuildContext(bc plugin.BuildContext) (Config, error) {
	h, ok := plugin.HostOf(bc)
	if !ok {
		return Config{}, errx.New("gcx signals require a mino host build context")
	}
	return FromHost(h), nil
}

// Token resolves the Grafana service-account token: env override first, then
// the sealed store.
func Token(store plugin.CredentialStore, envName string) (string, error) {
	if envName == "" {
		envName = DefaultTokenEnv
	}
	if tok := strings.TrimSpace(os.Getenv(envName)); tok != "" {
		return tok, nil
	}
	if store != nil {
		if c, ok, err := store.Get(context.Background(), TokenKey); err == nil && ok {
			if tok := strings.TrimSpace(c.AccessToken); tok != "" {
				return tok, nil
			}
		}
	}
	return "", errx.New("gcx: no Grafana Cloud token available").
		WithHint("run `mino login gcx`, or export a service account token ($%s=glsa_…)", envName)
}

// Authed reports whether a token is reachable from the store or the environment.
func Authed(store plugin.CredentialStore, envName string) bool {
	_, err := Token(store, envName)
	return err == nil
}

// ResolveStack walks param, then the registered context provider (which a role
// `contexts:` binding populates at startup), then the plugins.gcx.stack setting.
func ResolveStack(ctx context.Context, param string, settings map[string]any) (string, error) {
	if s := strings.TrimSpace(param); s != "" {
		return normalizeStack(s)
	}
	if n, ok, err := shared.Current(ctx); err == nil && ok && strings.TrimSpace(n) != "" {
		return normalizeStack(n)
	}
	if s := plugin.Setting(settings, "stack", ""); s != "" {
		return normalizeStack(s)
	}
	return "", errx.New("gcx: no Grafana Cloud stack selected").
		WithHint("pass --stack, bind one with a role `contexts: {gcx: myorg.grafana.net}`, or set plugins.gcx.stack")
}

func normalizeStack(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if i := strings.Index(s, "://"); i >= 0 {
		switch strings.ToLower(s[:i]) {
		case "http", "https":
		default:
			return "", errx.Newf("gcx: unsupported stack scheme %q", s[:i]).
				WithHint("use a bare host like myorg.grafana.net")
		}
		s = s[i+3:]
	}
	s = strings.TrimSuffix(s, "/")
	if s == "" || strings.ContainsAny(s, " \t/?#") {
		return "", errx.Newf("gcx: %q is not a stack host", raw).
			WithHint("use a bare host like myorg.grafana.net")
	}
	return strings.ToLower(s), nil
}

// IRMBaseURL builds the IRM resource root for a stack host.
func IRMBaseURL(stack string) (string, error) {
	host, err := normalizeStack(stack)
	if err != nil {
		return "", err
	}
	return "https://" + host + IRMAPIPath, nil
}
