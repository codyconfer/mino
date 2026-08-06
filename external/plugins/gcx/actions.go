package gcx

import (
	"context"
	"strings"

	"github.com/codyconfer/mino/cmd"
	"github.com/codyconfer/mino/external/plugins/internal/errx"
	"github.com/codyconfer/mino/external/plugins/internal/params"
	"github.com/codyconfer/mino/plugin"
)

var (
	hostFn      = func() plugin.Host { return cmd.Host(SignalName) }
	newClientFn = NewClient
)

type namedAction struct {
	name string
	run  plugin.ActionFunc
}

func (a namedAction) Name() string { return a.name }

func (a namedAction) Run(ctx context.Context, p map[string]string) error { return a.run(ctx, p) }

// KnownActions are the write-side bindings registered against CapAction.
func KnownActions() []plugin.Action {
	return []plugin.Action{
		namedAction{name: "declare-incident", run: declareIncident},
		namedAction{name: "add-activity", run: addActivity},
	}
}

func actionClient(ctx context.Context, p map[string]string) (*Client, map[string]any, error) {
	h := hostFn()
	if h == nil {
		return nil, nil, errx.New("gcx: no mino host is available for this action").
			WithHint("run gcx actions through `mino action run gcx …` or `mino gcx …`")
	}
	cfg := FromHost(h)
	if !plugin.SettingBool(cfg.Settings, "allow_write", false) {
		return nil, nil, errx.New("gcx: write actions are disabled").
			WithHint("set `plugins.gcx.allow_write: true` to let mino declare incidents and post activity")
	}
	stack, err := ResolveStack(ctx, p["stack"], cfg.Settings)
	if err != nil {
		return nil, nil, err
	}
	token, err := Token(cfg.Store, cfg.TokenEnv)
	if err != nil {
		return nil, nil, err
	}
	client, err := newClientFn(stack, token)
	if err != nil {
		return nil, nil, err
	}
	client.Timeout = plugin.SettingDuration(cfg.Settings, "timeout", defaultRPCTimeout)
	return client, cfg.Settings, nil
}

func declareIncident(ctx context.Context, p map[string]string) error {
	if strings.TrimSpace(p["title"]) == "" {
		return errx.New("gcx declare-incident: a title is required").
			WithHint(`mino action run gcx declare-incident --param title="API latency elevated"`)
	}
	client, settings, err := actionClient(ctx, p)
	if err != nil {
		return err
	}
	_, err = client.CreateIncident(ctx, newIncidentFrom(p, settings))
	return err
}

func newIncidentFrom(p map[string]string, settings map[string]any) NewIncident {
	return NewIncident{
		Title:    strings.TrimSpace(p["title"]),
		Severity: params.Str(p, "severity", plugin.Setting(settings, "default_severity", "minor")),
		Status:   params.Str(p, "status", StatusActive),
		Summary:  p["summary"],
		Labels:   splitList(p["labels"]),
		IsDrill:  boolParam(p, "drill", plugin.SettingBool(settings, "default_drill", false)),
	}
}

func addActivity(ctx context.Context, p map[string]string) error {
	id := strings.TrimSpace(params.Str(p, "incident", p["id"]))
	body := strings.TrimSpace(p["body"])
	if id == "" || body == "" {
		return errx.New("gcx add-activity: incident and body are required").
			WithHint(`mino action run gcx add-activity --param incident=incident-100 --param body="rolled back the deploy"`)
	}
	client, _, err := actionClient(ctx, p)
	if err != nil {
		return err
	}
	return client.AddActivity(ctx, id, body, params.Str(p, "kind", "userNote"))
}
