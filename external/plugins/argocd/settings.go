package argocd

import (
	"net/url"
	"strings"

	"github.com/codyconfer/mino/external/plugins/internal/errx"
	"github.com/codyconfer/mino/external/plugins/internal/params"
	"github.com/codyconfer/mino/plugin"
)

const (
	defaultMax           = 50
	defaultAppNamespace  = "argocd"
	groupByNone          = "none"
	groupByProject       = "project"
	groupByCluster       = "cluster"
	maxDetailResourceRow = 40
	maxHistoryRows       = 5
)

type Config struct {
	ServerURL     string
	TokenEnv      string
	Projects      []string
	Selector      string
	AppNamespace  string
	Namespaces    []string
	App           string
	Max           int
	OnlyUnhealthy bool
	GroupBy       string
	CAFile        string
}

func configFrom(bc plugin.BuildContext) (Config, error) {
	s := plugin.SettingsOf(bc, SignalName)
	p := bc.Params()

	server, err := normalizeServerURL(plugin.Setting(s, "server_url", ""))
	if err != nil {
		return Config{}, err
	}

	cfg := Config{
		ServerURL:     server,
		TokenEnv:      plugin.Setting(s, "token_env", DefaultTokenEnv),
		Projects:      listParam(p, "project", plugin.SettingList(s, "projects")),
		Selector:      params.Str(p, "selector", plugin.Setting(s, "selector", "")),
		AppNamespace:  params.Str(p, "app_namespace", plugin.Setting(s, "app_namespace", "")),
		Namespaces:    listParam(p, "namespace", plugin.SettingList(s, "namespaces")),
		App:           params.Str(p, "app", ""),
		Max:           params.Int(p, "max", plugin.SettingInt(s, "max", defaultMax)),
		OnlyUnhealthy: boolParam(p, "only_unhealthy", plugin.SettingBool(s, "only_unhealthy", false)),
		GroupBy:       normalizeGroupBy(params.Str(p, "group_by", plugin.Setting(s, "group_by", groupByNone))),
		CAFile:        plugin.Setting(s, "ca_file", ""),
	}
	if cfg.Max <= 0 {
		cfg.Max = defaultMax
	}
	return cfg, nil
}

func listParam(p map[string]string, key string, def []string) []string {
	raw := strings.TrimSpace(p[key])
	if raw == "" {
		return def
	}
	out := make([]string, 0, strings.Count(raw, ",")+1)
	for _, part := range strings.Split(raw, ",") {
		if part = strings.TrimSpace(part); part != "" {
			out = append(out, part)
		}
	}
	if len(out) == 0 {
		return def
	}
	return out
}

func boolParam(p map[string]string, key string, def bool) bool {
	switch strings.ToLower(strings.TrimSpace(p[key])) {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	}
	return def
}

func normalizeGroupBy(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case groupByProject:
		return groupByProject
	case groupByCluster:
		return groupByCluster
	default:
		return groupByNone
	}
}

func normalizeServerURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errx.New("argocd: server_url is required").
			WithHint("set plugins.argocd.server_url, e.g. https://argocd.example.com")
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "", errx.Wrapf(err, "argocd: server_url %q is not a URL", raw).
			WithHint("use the ArgoCD UI root, e.g. https://argocd.example.com")
	}
	if u.Scheme != "https" {
		return "", errx.Newf("argocd: server_url must use https (refusing to send the API token over %q)", raw).
			WithHint("use an https URL; for a private CA set plugins.argocd.ca_file instead of downgrading")
	}
	if u.Host == "" {
		return "", errx.Newf("argocd: server_url %q has no host", raw).
			WithHint("use the ArgoCD UI root, e.g. https://argocd.example.com")
	}
	u.Path = strings.TrimSuffix(strings.TrimSuffix(strings.TrimRight(u.Path, "/"), "/api/v1"), "/api")
	u.RawQuery, u.Fragment = "", ""
	return strings.TrimRight(u.String(), "/"), nil
}

func appURL(server, name, appNamespace string) string {
	if name == "" {
		return server
	}
	if ns := strings.TrimSpace(appNamespace); ns != "" && ns != defaultAppNamespace {
		return server + "/applications/" + ns + "/" + name
	}
	return server + "/applications/" + name
}

func serverHost(server string) string {
	u, err := url.Parse(server)
	if err != nil || u.Host == "" {
		return server
	}
	return u.Host
}
