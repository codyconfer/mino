package cmd

import (
	"context"
	"io"
	"time"

	"github.com/codyconfer/munin/internal/app/flight"
	"github.com/codyconfer/munin/internal/deck"
	"github.com/codyconfer/munin/internal/log"
	"github.com/codyconfer/munin/internal/signals"
)

const defaultSignalTimeout = 30 * time.Second

func sourceTimeout() time.Duration {
	if shared.Cfg != nil && shared.Cfg.Timeout != "" {
		if d, err := time.ParseDuration(shared.Cfg.Timeout); err == nil && d > 0 {
			return d
		}
	}
	return defaultSignalTimeout
}

type query = flight.Query

func emit(w io.Writer, root string, sections []signals.Section) error {
	return flight.Emit(w, shared.Cfg.Output, root, sections)
}

func fetchQuery(ctx context.Context, q query, parentID int64) []signals.Section {
	return flight.FetchQuery(ctx, shared.Audit, shared.Cfg.Role, sourceTimeout(), q, parentID)
}

func fetchQueries(ctx context.Context, queries []query, parentID int64) []signals.Section {
	return flight.FetchQueries(ctx, shared.Audit, shared.Cfg.Role, sourceTimeout(), queries, parentID)
}

func runQueries(ctx context.Context, w io.Writer, root string, queries []query, parentID int64) error {
	if !interactiveTTY() {
		return emit(w, root, fetchQueries(ctx, queries, parentID))
	}
	tasks := make([]deck.Task, len(queries))
	for i, q := range queries {
		tasks[i] = deck.Task{
			Label: q.Display(),
			Run:   func(ctx context.Context) []signals.Section { return fetchQuery(ctx, q, parentID) },
		}
	}
	return deck.RunFlight(ctx, tasks)
}

func verbosef(format string, args ...any) {
	log.Debugf(format, args...)
}
