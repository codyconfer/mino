package argocd

import (
	"context"

	"github.com/codyconfer/mino/plugin"
)

type Signal struct {
	client *Client
	cfg    Config
}

func New(cfg Config, tokens TokenLookup) *Signal {
	return &Signal{client: NewClient(cfg, tokens), cfg: cfg}
}

func (s *Signal) Name() string { return SignalName }

func (s *Signal) Fetch(ctx context.Context) ([]plugin.Section, error) {
	list, _, err := s.client.Applications(ctx)
	if err != nil {
		return nil, err
	}
	return sectionsFrom(list.Items, s.cfg), nil
}
