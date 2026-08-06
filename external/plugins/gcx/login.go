package gcx

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/charmbracelet/x/term"

	"github.com/codyconfer/mino/external/plugins/internal/errx"
	"github.com/codyconfer/mino/plugin"
)

const maxTokenBytes = 4 << 10

var (
	stdinIsTTY           = func() bool { return term.IsTerminal(os.Stdin.Fd()) }
	stdin      io.Reader = os.Stdin
)

// LoginProvider is the `mino login gcx` binding. It carries no Fields — see
// SPIKE.md §6.
func LoginProvider() plugin.LoginProvider {
	return plugin.LoginProvider{
		PluginID: PluginID,
		Key:      TokenKey,
		Label:    "Grafana Cloud",
		Signals:  []string{SignalName},
		Authed: func(h plugin.Host) bool {
			cfg := FromHost(h)
			return Authed(cfg.Store, cfg.TokenEnv)
		},
		Login: login,
	}
}

func login(ctx context.Context, h plugin.Host, _ map[string]string, w io.Writer) error {
	cfg := FromHost(h)
	if cfg.Store == nil {
		return errx.New("gcx: no credential store is available").
			WithHint("run `mino login gcx` from an initialized mino home")
	}
	token, err := readToken(w, cfg.TokenEnv)
	if err != nil {
		return err
	}
	if !strings.HasPrefix(token, "glsa_") {
		fmt.Fprintln(w, "note: that does not look like a Grafana service account token (glsa_…)")
	}
	return cfg.Store.Put(ctx, TokenKey, plugin.Credential{AccessToken: token, Scope: CredScope})
}

func readToken(w io.Writer, envName string) (string, error) {
	if envName == "" {
		envName = DefaultTokenEnv
	}
	if v := strings.TrimSpace(os.Getenv(envName)); v != "" {
		fmt.Fprintf(w, "sealing the token from $%s\n", envName)
		return v, nil
	}
	if !stdinIsTTY() {
		raw, err := io.ReadAll(io.LimitReader(stdin, maxTokenBytes))
		if err != nil {
			return "", errx.Wrap(err, "gcx: reading token from stdin")
		}
		token := strings.TrimSpace(string(raw))
		if token == "" {
			return "", errx.New("gcx: no token on stdin").
				WithHint("pipe the token in, export $%s, or run `mino login gcx` on a terminal", envName)
		}
		return token, nil
	}
	fmt.Fprint(w, "Grafana service account token for IRM (input hidden): ")
	raw, err := term.ReadPassword(os.Stdin.Fd())
	fmt.Fprintln(w)
	if err != nil {
		return "", errx.Wrap(err, "gcx: reading token")
	}
	token := strings.TrimSpace(string(raw))
	if token == "" {
		return "", errx.New("gcx: empty token").
			WithHint("create a service account token in the stack's IRM app, or export $%s", envName)
	}
	return token, nil
}
