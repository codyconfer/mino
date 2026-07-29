package build

import (
	"context"
	"sort"
	"strings"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/signals"
	"github.com/codyconfer/munin/internal/signals/cache"
	"github.com/codyconfer/munin/internal/token"
)

func DetailSignals() []string {
	var out []string
	for name, ok := range BuilderSignals() {
		if ok && plugin.HasCapability(name, plugin.CapDetail) {
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func Detail(ctx context.Context, signal string, it signals.Item, cfg *config.Config, tokens *token.Store, results *cache.Store) (signals.ItemDetail, error) {
	if !plugin.HasBuilder(signal) {
		return signals.ItemDetail{}, errs.Newf(errs.KindConfig, "unknown signal %q", signal)
	}
	if !plugin.HasCapability(signal, plugin.CapDetail) {
		err := errs.Newf(errs.KindUsage, "signal %q does not support details", signal)
		if named := DetailSignals(); len(named) > 0 {
			err = err.WithHint("signals with details: %s", strings.Join(named, ", "))
		}
		return signals.ItemDetail{}, err
	}
	q, err := plugin.BuildQuery(signal, hostBuildCtx{cfg: cfg, tokens: tokens, cache: results})
	if err != nil {
		return signals.ItemDetail{}, err
	}
	d, ok := q.(signals.Detailer)
	if !ok {
		return signals.ItemDetail{}, errs.Newf(errs.KindInternal,
			"signal %q advertises the detail capability but does not implement it", signal)
	}
	return d.Detail(ctx, it)
}
