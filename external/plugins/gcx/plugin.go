package gcx

import (
	"context"
	"strconv"
	"strings"

	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/mino/cmd"
	"github.com/codyconfer/mino/external/plugins/internal/errx"
	"github.com/codyconfer/mino/external/plugins/internal/params"
	"github.com/codyconfer/mino/plugin"
)

const (
	ViewIncidents = "incidents"
	ViewStatus    = "status"

	StatusActive = "active"
	StatusAll    = "all"

	defaultLimit = 20
	maxLimit     = 200
)

func Register() {
	if _, ok := plugin.Lookup(PluginID); ok {
		return
	}
	plugin.RegisterSignal(plugin.Descriptor{
		ID:                 PluginID,
		Kind:               plugin.KindSignal,
		Signal:             SignalName,
		Capabilities:       []plugin.Capability{plugin.CapQuery, plugin.CapAction},
		Credentials:        []string{TokenKey},
		SettingsNamespaces: []string{SignalName},
	}, plugin.Builders{
		Query: BuildQuery,
	})
	glyph.Register(GlyphID, glyph.Variants{Nerd: "󰡉", Uni: "☁", ASCII: "gx"})
	plugin.RegisterContext(PluginID, shared)
	plugin.RegisterStatusContribution(PluginID, func(_, _ string) glyph.StatusContribution {
		return StatusContribution()
	})
	plugin.RegisterQueryParams(SignalName,
		plugin.ParamSpec{Key: "view", Desc: "which gcx surface to read", Example: ViewIncidents,
			Values: []string{ViewIncidents, ViewStatus}},
		plugin.ParamSpec{Key: "stack", Desc: "Grafana Cloud stack host", Example: "myorg.grafana.net"},
		plugin.ParamSpec{Key: "status", Desc: "incident status filter", Example: StatusActive,
			Values: []string{StatusActive, "resolved", StatusAll}},
		plugin.ParamSpec{Key: "limit", Desc: "maximum incidents to return", Example: "20",
			Values: []string{"10", "20", "50", "100"}},
		plugin.ParamSpec{Key: "drills", Desc: "include drill incidents", Example: "false",
			Values: []string{"true", "false"}},
	)
	for _, a := range KnownActions() {
		plugin.RegisterAction(SignalName, a.Name(), a.Run)
	}
	plugin.RegisterLoginProvider(LoginProvider())
	plugin.RegisterSeeds(PluginID, []plugin.FileSeed{
		{RelPath: "queries/gcx-status.yaml", Content: []byte(ExampleDirective)},
		{RelPath: "queries/gcx-incidents.yaml", Content: []byte(IncidentsDirective)},
	})
	cmd.RegisterCommand(newGcxCmd)
}

// BuildQuery dispatches on the view param: status stays offline, incidents
// resolves a stack and token up front so misconfiguration fails at build time.
func BuildQuery(bc plugin.BuildContext) (plugin.Query, error) {
	p := bc.Params()
	settings := plugin.SettingsOf(bc, SignalName)

	switch view := params.Str(p, "view", plugin.Setting(settings, "view", ViewIncidents)); view {
	case ViewStatus:
		return NewSignal(tokenLookupFrom(bc)), nil
	case ViewIncidents:
		cfg, err := FromBuildContext(bc)
		if err != nil {
			return nil, err
		}
		stack, err := ResolveStack(context.Background(), p["stack"], settings)
		if err != nil {
			return nil, err
		}
		token, err := Token(cfg.Store, cfg.TokenEnv)
		if err != nil {
			return nil, err
		}
		client, err := NewClient(stack, token)
		if err != nil {
			return nil, err
		}
		client.Timeout = plugin.SettingDuration(settings, "timeout", defaultRPCTimeout)
		return Incidents{Client: client, Query: incidentQueryFrom(p, settings)}, nil
	default:
		return nil, errx.Newf("gcx: unknown view %q", view).
			WithHint("view must be one of: %s, %s", ViewIncidents, ViewStatus)
	}
}

func incidentQueryFrom(p map[string]string, settings map[string]any) IncidentQuery {
	limit := params.Int(p, "limit", plugin.SettingInt(settings, "limit", defaultLimit))
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}
	return IncidentQuery{
		Status:        params.Str(p, "status", plugin.Setting(settings, "status", StatusActive)),
		Limit:         limit,
		IncludeDrills: boolParam(p, "drills", plugin.SettingBool(settings, "drills", false)),
	}
}

func boolParam(p map[string]string, key string, def bool) bool {
	raw := strings.TrimSpace(p[key])
	if raw == "" {
		return def
	}
	if b, err := strconv.ParseBool(raw); err == nil {
		return b
	}
	return def
}

func splitList(raw string) []string {
	var out []string
	for _, part := range strings.Split(raw, ",") {
		if v := strings.TrimSpace(part); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// ExampleDirective is the offline status seed; it pins view so the default
// flip to incidents leaves it alone.
const ExampleDirective = `name: gcx-status
type: query
signal: gcx
params:
  view: status
`

// IncidentsDirective is the live IRM incidents seed.
const IncidentsDirective = `name: gcx-incidents
type: query
signal: gcx
params:
  view: incidents
  status: active
  limit: "20"
`
