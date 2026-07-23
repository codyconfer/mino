package cmd

import (
	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/auth"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/signals"
	slacksrc "github.com/codyconfer/munin/internal/signals/slack"
)

func newSlackCmd() *cobra.Command {
	parent := &cobra.Command{Use: "slack", Short: "Slack channel activity"}

	var ff filterFlags
	var channel string
	query := &cobra.Command{
		Use:   "query",
		Short: "Fetch a Slack channel now, with optional filters",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			params := map[string]string{}
			if channel != "" {
				params["channel"] = channel
			}
			return runSignal(cmd, "slack", params, &ff)
		},
	}
	query.Flags().StringVar(&channel, "channel", "", "channel to read (name or ID)")
	ff.bind(query)
	parent.AddCommand(query)
	return parent
}

func buildSlack(params map[string]string) (signals.Signal, error) {
	channel := params["channel"]
	if channel == "" {
		return nil, errs.New(errs.KindUsage, "slack: a channel is required").WithHint("use --channel or a query param")
	}
	token, err := auth.SlackToken(shared.tokens, shared.cfg.Slack.TokenEnv)
	if err != nil {
		return nil, errs.Wrapf(errs.KindAuth, err, "slack authentication").WithHint("set %s", shared.cfg.Slack.TokenEnv)
	}
	limit := paramInt(params, "limit", shared.cfg.Slack.Limit)
	return slacksrc.New(token, channel, limit), nil
}

func buildActiveSlack(params map[string]string) (signals.ActiveSignal, error) {
	appToken, err := auth.SlackAppToken(shared.tokens, shared.cfg.Slack.AppTokenEnv)
	if err != nil {
		return nil, errNoActiveSignal
	}
	botToken, err := auth.SlackBotToken(shared.tokens, shared.cfg.Slack.BotTokenEnv)
	if err != nil {
		return nil, errNoActiveSignal
	}
	if appToken == "" || botToken == "" {
		return nil, errNoActiveSignal
	}
	return slacksrc.NewActive(botToken, appToken), nil
}
