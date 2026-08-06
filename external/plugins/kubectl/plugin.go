package kubectl

import (
	"context"
	"strings"
	"time"

	"github.com/codyconfer/mino/external/plugins/internal/params"
	"github.com/codyconfer/mino/external/plugins/internal/stream"
	"github.com/codyconfer/mino/plugin"
)

const (
	DefaultInterval = 60 * time.Second
	probeTimeout    = 3 * time.Second
)

func BuildQuery(bc plugin.BuildContext) (plugin.Query, error) {
	s, opts := resolve(bc)
	return NewSignal(nil, s, opts), nil
}

func BuildStream(bc plugin.BuildContext) (plugin.Stream, error) {
	interval, err := params.PollInterval(bc.Params(), SignalName, DefaultInterval)
	if err != nil {
		return nil, err
	}
	s, opts := resolve(bc)
	return NewActive(NewSignal(nil, s, opts), interval, stream.StateOf(bc)), nil
}

func resolve(bc plugin.BuildContext) (scope, options) {
	p := bc.Params()
	set := plugin.SettingsOf(bc, SignalName)

	opts := options{
		Binary:           plugin.Setting(set, "binary", DefaultBinary),
		Collectors:       parseCollectors(params.Str(p, "what", strings.Join(plugin.SettingList(set, "kinds"), ","))),
		Since:            params.Duration(p, "since", plugin.SettingDuration(set, "since", DefaultSince)),
		Limit:            params.Int(p, "limit", plugin.SettingInt(set, "limit", DefaultLimit)),
		RestartThreshold: params.Int(p, "restarts", plugin.SettingInt(set, "restart_threshold", DefaultRestartThreshold)),
		Timeout:          plugin.SettingDuration(set, "timeout", DefaultTimeout),
	}

	s := scope{
		context: resolveContext(p["context"], plugin.Setting(set, "context", "")),
		timeout: opts.Timeout,
	}
	if ns := params.Str(p, "namespace", plugin.Setting(set, "namespace", "")); ns != "" {
		s.namespace = ns
	} else {
		s.allNamespaces = true
	}
	return s, opts
}

func resolveContext(param, setting string) string {
	if param != "" {
		return param
	}
	if name := shared.selected(); name != "" {
		return name
	}
	return setting
}

func registerParams() {
	plugin.RegisterQueryParams(SignalName,
		plugin.ParamSpec{Key: "context", Desc: "kube context to read; never written back to kubeconfig", Example: "prod-us-east"},
		plugin.ParamSpec{Key: "namespace", Desc: "namespace to read; empty reads all namespaces", Example: "payments"},
		plugin.ParamSpec{
			Key: "what", Desc: "collectors to run", Example: "pods,events",
			Values: KnownCollectors(), Delim: ",",
		},
		plugin.ParamSpec{Key: "since", Desc: "warning-event window", Example: "1h", Values: []string{"15m", "1h", "6h", "24h"}},
		plugin.ParamSpec{Key: "limit", Desc: "maximum items per section", Example: "25", Values: []string{"10", "25", "50", "100"}},
		plugin.ParamSpec{Key: "restarts", Desc: "restart count that flags a running pod", Example: "5"},
		plugin.ParamSpec{Key: "interval", Desc: "stream poll interval", Example: "60s"},
	)
}

func probeContext(ctx context.Context, binary string) (string, bool) {
	if !binaryAvailable(binary) {
		return "", false
	}
	out, err := newExecRunner(binary, probeTimeout).Run(ctx, "config", "current-context")
	if err != nil {
		return "", false
	}
	name := strings.TrimSpace(string(out))
	return name, name != ""
}
