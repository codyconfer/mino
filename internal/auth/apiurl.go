package auth

import (
	"net"
	"net/url"
	"strings"

	"github.com/codyconfer/mino/internal/errs"
)

const (
	githubAPIURLHint = "set github.api_url to an https:// endpoint, e.g. https://api.github.com or your enterprise host"
	giteaURLHint     = "set gitea.url to your instance root, e.g. https://git.example.com (MINO_GITEA_URL also works); mino appends /api/v1"
	giteaAPIURLHint  = "set gitea.api_url to your instance API root, e.g. https://git.example.com/api/v1"
)

type forgeURL struct {
	forge    string
	field    string
	hint     string
	loopback bool
}

func NormalizeGitHubAPIURL(raw string) (string, error) {
	return forgeURL{forge: "github", field: "api_url", hint: githubAPIURLHint}.normalize(raw)
}

func NormalizeGiteaURL(raw string) (string, error) {
	return forgeURL{forge: "gitea", field: "url", hint: giteaURLHint, loopback: true}.normalize(raw)
}

func NormalizeGiteaAPIURL(raw string) (string, error) {
	return forgeURL{forge: "gitea", field: "api_url", hint: giteaAPIURLHint, loopback: true}.normalize(raw)
}

func (f forgeURL) normalize(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", errs.Wrapf(errs.KindConfig, err, "%s: invalid %s %q", f.forge, f.field, raw)
	}
	if !f.allowsScheme(u) {
		return "", errs.Newf(errs.KindConfig, "%s: %s must use https (refusing to send token over %q)", f.forge, f.field, raw).
			WithHint("%s", f.hint)
	}
	if u.Host == "" {
		return "", errs.Newf(errs.KindConfig, "%s: %s has no host: %q", f.forge, f.field, raw)
	}
	return strings.TrimRight(raw, "/"), nil
}

func (f forgeURL) allowsScheme(u *url.URL) bool {
	if u.Scheme == "https" {
		return true
	}
	return f.loopback && u.Scheme == "http" && loopbackHost(u.Hostname())
}

func loopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
