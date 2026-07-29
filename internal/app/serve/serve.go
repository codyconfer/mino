package serve

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	sysdaemon "github.com/codyconfer/sisyphus/daemon"
	"github.com/codyconfer/sisyphus/desktop"
	"github.com/codyconfer/sisyphus/kv"
	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/app/views"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/deck"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/filter"
	"github.com/codyconfer/munin/internal/log"
	mnotify "github.com/codyconfer/munin/internal/notify"
	"github.com/codyconfer/munin/internal/signals"
	"github.com/codyconfer/munin/internal/signals/active"
	"github.com/codyconfer/munin/internal/signals/build"
)

const serveBuffer = 256

type Server struct {
	*app.App
}

func NewServer(a *app.App) *Server {
	return &Server{App: a}
}

type activeQuery struct {
	label   string
	src     signals.ActiveSignal
	filters []filter.Compiled
}

type notifySink struct {
	bell     bool
	desktop  bool
	terminal bool
	onState  func(sysdaemon.State)
}

func (n notifySink) handle(ev signals.Event) {
	st := stateForEvent(ev)
	if n.onState != nil {
		n.onState(st)
	}
	note, show := mnotify.FromEvent(ev)
	if !show {
		return
	}
	if n.desktop {
		icon, _ := sysdaemon.StateIcon(st)
		_ = desktop.Notify(desktop.Notification{
			Title:   note.Title,
			Message: note.Message,
			Icon:    desktop.Icon{Name: icon.Name, MIME: icon.MIME, Bytes: icon.Bytes},
		})
	}
	if n.terminal {
		if n.bell {
			fmt.Fprint(os.Stdout, "\a")
		}
		fmt.Fprintln(os.Stdout, mnotify.Render(note))
	}
}

func (s *Server) SocketPath() string { return config.ServeSocketPath(s.Cfg.Home) }

func (s *Server) openState() (*active.State, func()) {
	store, err := kv.Open(context.Background(), config.DataPath(s.Cfg.Home, config.ServeDB))
	if err != nil {
		log.Debugf("serve: cursor persistence unavailable: %v", err)
		return active.NewState(nil), func() {}
	}
	return active.NewState(store), func() { _ = store.Close() }
}

func (s *Server) events(ctx context.Context, name string, interval time.Duration, state *active.State) (<-chan signals.Event, error) {
	flight := s.Directives.Flights[name]
	queries := s.activeQueries(name, flight.Queries, interval, state)

	var chans []<-chan signals.Event
	for _, q := range queries {
		ch, err := q.src.Stream(ctx)
		if err != nil {
			log.Warnf("serve: %s: %v (skipping)", q.label, err)
			continue
		}
		chans = append(chans, applyFilters(ctx, ch, q.filters))
		fmt.Fprintf(os.Stderr, "watching %-10s %s\n", q.src.Name(), latencyLabel(q.src.LatencyFloor()))
	}
	if sch := s.scheduledEvents(ctx, name, flight.Queries, state); sch != nil {
		chans = append(chans, sch)
	}
	if len(chans) == 0 {
		return nil, errs.Newf(errs.KindUsage, "flight %q has no signals with realtime or scheduled support", name).
			WithHint("active: slack, github, calendar, tasks, demo; scheduled: ntr")
	}
	return sysdaemon.FanIn(ctx, chans...), nil
}

func (s *Server) activeQueries(flight string, names []string, interval time.Duration, state *active.State) []activeQuery {
	var out []activeQuery
	for _, name := range names {
		q, ok := s.Directives.Queries[name]
		if !ok {
			log.Debugf("serve: unknown query %q in flight %q", name, flight)
			continue
		}
		if !q.Runnable() {
			log.Debugf("serve: %q is filter-only, not a runnable query (skipping)", name)
			continue
		}
		resolved, err := s.Directives.Resolve(q)
		if err != nil {
			log.Warnf("serve: query %q: %v (skipping)", name, err)
			continue
		}
		params, err := filter.ExpandParams(q.Params, resolved)
		if err != nil {
			log.Warnf("serve: query %q: %v (skipping)", name, err)
			continue
		}
		hs, err := build.ActiveSignal(q.Signal, activeParams(params, interval), s.Cfg, s.Tokens, state)
		if err != nil {
			if errors.Is(err, build.ErrNoActive) {
				log.Debugf("serve: query %q signal %q has no realtime support (skipping)", name, q.Signal)
			} else {
				log.Warnf("serve: query %q: %v (skipping)", name, err)
			}
			continue
		}
		compiled, err := filter.CompileAll(resolved)
		if err != nil {
			log.Warnf("serve: query %q: %v (skipping)", name, err)
			continue
		}
		out = append(out, activeQuery{label: name, src: hs, filters: compiled})
	}
	return out
}

func activeParams(params map[string]string, interval time.Duration) map[string]string {
	out := make(map[string]string, len(params)+1)
	for k, v := range params {
		out[k] = v
	}
	if out["interval"] == "" {
		out["interval"] = interval.String()
	}
	return out
}

func applyFilters(ctx context.Context, in <-chan signals.Event, filters []filter.Compiled) <-chan signals.Event {
	if len(filters) == 0 {
		return in
	}
	out := make(chan signals.Event)
	go func() {
		defer close(out)
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-in:
				if !ok {
					return
				}
				if ev.Section.Err == nil {
					ev.Section.Items = filter.ApplyAll(filters, ev.Section.Items)
					if len(ev.Section.Items) == 0 {
						continue
					}
				}
				select {
				case out <- ev:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return out
}

func (s *Server) observeAudit(ch <-chan signals.Event, flightID int64) {
	for ev := range ch {
		s.Audit.RecordQuery(flightID, ev.Source, s.Cfg.Role, ev.At, time.Now(), []signals.Section{ev.Section})
	}
}

func observeNotify(ctx context.Context, ch <-chan signals.Event, sink notifySink) {
	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-ch:
			if !ok {
				return
			}
			sink.handle(ev)
		}
	}
}

func (s *Server) observable(ctx context.Context, name string, interval time.Duration, state *active.State) (*sysdaemon.Subject[signals.Event], error) {
	events, err := s.events(ctx, name, interval, state)
	if err != nil {
		return nil, err
	}
	subj := sysdaemon.NewSubject[signals.Event]()
	go subj.Pump(ctx, events)
	return subj, nil
}

func (s *Server) socket(ctx context.Context, subj *sysdaemon.Subject[signals.Event]) func() {
	path := s.SocketPath()
	if sysdaemon.IsListening(config.SocketPrefix, path) {
		log.Debugf("serve: another daemon already owns %s; not exposing a socket", path)
		return func() {}
	}
	ln, err := sysdaemon.Listen(config.SocketPrefix, path)
	if err != nil {
		log.Debugf("serve: socket unavailable: %v", err)
		return func() {}
	}
	go sysdaemon.Broadcast(ctx, ln, subj, serveBuffer, Encode)
	return func() { _ = ln.Close() }
}

func (s *Server) Dial(ctx context.Context) (<-chan signals.Event, bool) {
	events, err := sysdaemon.Dial(ctx, config.SocketPrefix, s.SocketPath(), Decode)
	if err != nil {
		return nil, false
	}
	return events, true
}

func (s *Server) WatchAttached(events <-chan signals.Event) error {
	return deck.Run(s.serveView("attached", events))
}

func (s *Server) serveView(name string, events <-chan signals.Event) *views.ServeView {
	v := views.NewServeView(name, events)
	v.FetchDetail = s.fetchDetail
	return v
}

func (s *Server) fetchDetail(signal string, it signals.Item) (*signals.ItemDetail, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.SourceTimeout())
	defer cancel()
	d, err := build.Detail(ctx, signal, it, s.Cfg, s.Tokens, s.Cache)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *Server) Attach(ctx context.Context) error {
	events, ok := s.Dial(ctx)
	if !ok {
		return errs.Newf(errs.KindUsage, "no running munin daemon at %s", s.SocketPath()).
			WithHint("start one with `munin serve <flight>` (foreground) or `munin daemon` (installed service), or open `munin deck`")
	}
	return s.WatchAttached(events)
}

type RunOptions struct {
	Flight   string
	Interval time.Duration
	Bell     bool
	Desktop  bool
	Terminal bool
	OnState  func(sysdaemon.State)
}

func (s *Server) Run(ctx context.Context, opt RunOptions) error {
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	state, closeState := s.openState()
	defer closeState()

	subj, err := s.observable(cctx, opt.Flight, opt.Interval, state)
	if err != nil {
		return err
	}
	defer subj.Close()

	closeSock := s.socket(cctx, subj)
	defer closeSock()

	flightID := s.Audit.StartFlight("serve", s.Cfg.Role)
	defer s.Audit.FinishFlight(flightID)

	go s.observeAudit(subj.Subscribe(serveBuffer), flightID)

	sink := notifySink{bell: opt.Bell, desktop: opt.Desktop, terminal: opt.Terminal, onState: opt.OnState}
	observeNotify(cctx, subj.Subscribe(serveBuffer), sink)
	return nil
}

func stateForEvent(ev signals.Event) sysdaemon.State {
	if ev.Section.Err != nil {
		return sysdaemon.StateError
	}
	worst := glyph.SeverityNeutral
	for _, it := range ev.Section.Items {
		if s := signals.ClassifyKind(it.Kind); s > worst {
			worst = s
		}
	}
	switch worst {
	case glyph.SeverityNegative:
		return sysdaemon.StateError
	case glyph.SeverityWarning:
		return sysdaemon.StateWarn
	default:
		return sysdaemon.StateNotify
	}
}

func latencyLabel(d time.Duration) string {
	if d <= 0 {
		return "(push/realtime)"
	}
	return "(~" + d.String() + " poll)"
}
