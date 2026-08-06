package gitlab

import (
	"context"
	"time"
)

type Cache interface {
	Get(ctx context.Context, namespace, key string) (string, bool)
	Put(ctx context.Context, namespace, key, value string, expiry time.Time)
}

type CachePolicy struct {
	Read  bool
	Write bool
	TTL   time.Duration
}

type signalOpts struct {
	viewer string
	title  string
	detail Cache
	policy CachePolicy
	rate   *RateHint
}

type Option func(*signalOpts)

func WithViewer(login string) Option {
	return func(o *signalOpts) { o.viewer = login }
}

func WithTitle(title string) Option {
	return func(o *signalOpts) { o.title = title }
}

func WithDetailCache(c Cache, policy CachePolicy) Option {
	return func(o *signalOpts) { o.detail, o.policy = c, policy }
}

func WithRateHint(r *RateHint) Option {
	return func(o *signalOpts) { o.rate = r }
}

func applyOptions(opts []Option) signalOpts {
	var o signalOpts
	for _, fn := range opts {
		if fn != nil {
			fn(&o)
		}
	}
	return o
}
