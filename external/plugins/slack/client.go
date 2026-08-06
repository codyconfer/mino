package slack

import (
	"time"

	slackapi "github.com/slack-go/slack"

	"github.com/codyconfer/mino/external/plugins/internal/httpx"
)

const (
	retryMaxDefault    = 2
	retryAfterFallback = 5 * time.Second
)

func clientFor(token, apiURL string, retryMax int) *slackapi.Client {
	if retryMax < 0 {
		retryMax = 0
	}
	opts := []slackapi.Option{
		slackapi.OptionHTTPClient(httpx.Client()),
		slackapi.OptionRetryConfig(retryConfig(retryMax)),
	}
	if apiURL != "" {
		opts = append(opts, slackapi.OptionAPIURL(apiURL))
	}
	return slackapi.New(token, opts...)
}

func retryConfig(retryMax int) slackapi.RetryConfig {
	cfg := slackapi.DefaultRetryConfig()
	cfg.MaxRetries = retryMax
	cfg.RetryAfterDuration = retryAfterFallback
	cfg.Handlers = slackapi.AllBuiltinRetryHandlers(cfg)
	return cfg
}
