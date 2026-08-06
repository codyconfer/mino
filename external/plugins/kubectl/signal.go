package kubectl

import (
	"context"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/codyconfer/mino/external/plugins/internal/errx"
	"github.com/codyconfer/mino/plugin"
)

const (
	CollectorPods      = "pods"
	CollectorEvents    = "events"
	CollectorNodes     = "nodes"
	CollectorWorkloads = "workloads"
	CollectorContext   = "context"
)

const (
	titlePods      = "unhealthy pods"
	titleEvents    = "warning events"
	titleNodes     = "nodes"
	titleWorkloads = "rollouts"
	titleContext   = "context"
)

const DefaultLimit = 25

func DefaultCollectors() []string {
	return []string{CollectorPods, CollectorEvents, CollectorNodes, CollectorWorkloads}
}

func KnownCollectors() []string {
	return []string{CollectorPods, CollectorEvents, CollectorNodes, CollectorWorkloads, CollectorContext}
}

type options struct {
	Binary           string
	Collectors       []string
	Since            time.Duration
	Limit            int
	RestartThreshold int
	Timeout          time.Duration

	Now func() time.Time
}

func (o options) now() time.Time {
	if o.Now != nil {
		return o.Now()
	}
	return time.Now()
}

type Signal struct {
	run   runner
	scope scope
	opts  options
}

func NewSignal(r runner, s scope, opts options) Signal {
	return Signal{run: r, scope: s, opts: opts}
}

func (Signal) Name() string { return SignalName }

func (s Signal) Fetch(ctx context.Context) ([]plugin.Section, error) {
	if s.run == nil {
		if !binaryAvailable(s.opts.Binary) {
			return nil, errx.Newf("kubectl: %q is not on PATH", binaryName(s.opts.Binary)).
				WithHint("install kubectl, or set plugins.kubectl.binary to its full path")
		}
		s.run = newExecRunner(s.opts.Binary, s.opts.Timeout)
	}

	collectors := s.opts.Collectors
	if len(collectors) == 0 {
		collectors = DefaultCollectors()
	}

	sections := make([]plugin.Section, len(collectors))
	var wg sync.WaitGroup
	for i, name := range collectors {
		fetch := collectorFor(name)
		if fetch == nil {
			sections[i] = errSection(name, errx.Newf("kubectl: unknown collector %q", name).
				WithHint("what accepts %s", strings.Join(KnownCollectors(), ", ")))
			continue
		}
		wg.Add(1)
		go func(idx int, f collector) {
			defer wg.Done()
			sections[idx] = f(ctx, s.run, s.scope, s.opts)
		}(i, fetch)
	}
	wg.Wait()

	shared.grade(countUnhealthy(sections))
	return sections, nil
}

type collector func(context.Context, runner, scope, options) plugin.Section

func collectorFor(name string) collector {
	switch name {
	case CollectorPods:
		return fetchPods
	case CollectorEvents:
		return fetchEvents
	case CollectorNodes:
		return fetchNodes
	case CollectorWorkloads:
		return fetchWorkloads
	case CollectorContext:
		return fetchContext
	default:
		return nil
	}
}

func fetchContext(ctx context.Context, _ runner, s scope, opts options) plugin.Section {
	name := s.context
	if name == "" {
		if probed, ok := probeContext(ctx, opts.Binary); ok {
			name = probed
		}
	}
	body := name
	if body == "" {
		body = "(no context)"
	}
	ns := s.namespace
	if s.allNamespaces || ns == "" {
		ns = "(all namespaces)"
	}
	return plugin.Section{
		Signal: SignalName,
		Title:  titleContext,
		Items: []plugin.Item{{
			Kind:     "context",
			Title:    "current context",
			Subtitle: ns,
			Body:     body,
			Meta:     map[string]string{"context": name, "namespace": s.namespace},
		}},
	}
}

func countUnhealthy(sections []plugin.Section) int {
	total := 0
	for _, sec := range sections {
		if sec.Err != nil || sec.Title == titleContext {
			continue
		}
		total += len(sec.Items)
	}
	return total
}

func parseCollectors(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return DefaultCollectors()
	}
	seen := map[string]bool{}
	var out []string
	for _, part := range strings.Split(raw, ",") {
		name := strings.ToLower(strings.TrimSpace(part))
		if name == "" || seen[name] {
			continue
		}
		seen[name] = true
		out = append(out, name)
	}
	if len(out) == 0 {
		return DefaultCollectors()
	}
	order := map[string]int{}
	for i, name := range KnownCollectors() {
		order[name] = i
	}
	rank := func(name string) int {
		if i, ok := order[name]; ok {
			return i
		}
		return len(order)
	}
	sort.SliceStable(out, func(i, j int) bool { return rank(out[i]) < rank(out[j]) })
	return out
}
