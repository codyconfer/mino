package gitlab

import (
	"context"
	"strings"

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/signals"
)

const (
	signalName     = "gitlab"
	defaultPerPage = 30
)

var defaultSelectors = []string{
	"kind:mr scope:assigned state:opened",
	"kind:mr reviewer:@me state:opened",
}

var defaultTitles = map[string]string{
	"kind:mr scope:assigned state:opened": "Assigned Merge Requests",
	"kind:mr reviewer:@me state:opened":   "Review Requests",
}

func DefaultQueries() []string {
	return append([]string(nil), defaultSelectors...)
}

type query struct {
	sel   Selector
	title string
}

type Signal struct {
	queries []query
	backend Backend
	max     int
	viewer  *viewer
	detail  Cache
	policy  CachePolicy
	rate    *RateHint
	since   string
}

// New returns an error where the github signal does not: a selector is parsed at build
// time, and a bad one is a config mistake that must not render as an empty section.
func New(selectors []string, backend Backend, max int, opts ...Option) (*Signal, error) {
	if len(selectors) == 0 {
		selectors = defaultSelectors
	}
	if max <= 0 {
		max = defaultPerPage
	}
	o := applyOptions(opts)

	qs := make([]query, 0, len(selectors))
	for _, raw := range selectors {
		sel, err := ParseSelector(raw)
		if err != nil {
			return nil, err
		}
		qs = append(qs, query{sel: sel, title: titleFor(raw, o.title, len(selectors))})
	}
	return &Signal{
		queries: qs,
		backend: backend,
		max:     max,
		viewer:  newViewer(backend, o.viewer),
		detail:  o.detail,
		policy:  o.policy,
		rate:    o.rate,
	}, nil
}

func titleFor(raw, override string, count int) string {
	if override != "" && count == 1 {
		return override
	}
	if t, ok := defaultTitles[raw]; ok {
		return t
	}
	if raw == "" {
		return "Merge Requests"
	}
	return raw
}

func (s *Signal) Name() string { return signalName }

func (s *Signal) setSince(iso string) { s.since = iso }

func (s *Signal) Fetch(ctx context.Context) ([]signals.Section, error) {
	sections := make([]signals.Section, 0, len(s.queries))
	for _, q := range s.queries {
		sec, err := s.fetchOne(ctx, q)
		if err != nil {
			return nil, wrapQuery(q.sel.Raw(), err)
		}
		sections = append(sections, sec)
	}
	return sections, nil
}

func (s *Signal) fetchOne(ctx context.Context, q query) (signals.Section, error) {
	sel := q.sel
	sel.Params = cloneValues(q.sel.Params)
	title := q.title

	if sel.NeedsViewer() {
		login, err := s.viewer.Login(ctx)
		if err != nil {
			return signals.Section{}, err
		}
		if err := sel.ResolveViewer(login); err != nil {
			return signals.Section{}, err
		}
		title = strings.ReplaceAll(title, ViewerAlias, login)
	}
	if s.since != "" {
		sel.Params.Set("updated_after", s.since)
	}

	items, meta, err := s.fetchSurface(ctx, sel)
	if err != nil {
		return signals.Section{}, err
	}
	return signals.Section{
		Signal: signalName,
		Title:  title,
		Items:  items,
		Meta:   meta.sectionMeta(),
	}, nil
}

func (s *Signal) fetchSurface(ctx context.Context, sel Selector) ([]signals.Item, pageMeta, error) {
	perPage := min(s.max, 100)
	switch sel.Kind {
	case KindIssue:
		return fetchIssues(ctx, s.backend, sel, perPage, s.max)
	case KindPipeline:
		return fetchPipelines(ctx, s.backend, sel, perPage, s.max)
	default:
		return fetchMergeRequests(ctx, s.backend, sel, perPage, s.max)
	}
}

func wrapQuery(raw string, err error) error {
	werr := errs.Wrapf(errs.KindOf(err), err, "gitlab: query %q", raw)
	if h := errs.Hint(err); h != "" {
		werr = werr.WithHint("%s", h)
	}
	return werr
}
