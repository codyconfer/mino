package serve

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/codyconfer/sisyphus/desktop"
	"github.com/codyconfer/sisyphus/ipc"
	"github.com/codyconfer/sisyphus/kv"
	"github.com/codyconfer/sisyphus/stream"
	"github.com/codyconfer/sisyphus/tray"
	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/mino/internal/app"
	"github.com/codyconfer/mino/internal/app/serve/httpapi"
	"github.com/codyconfer/mino/internal/app/views"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/deck"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/filter"
	"github.com/codyconfer/mino/internal/log"
	mnotify "github.com/codyconfer/mino/internal/notify"
	"github.com/codyconfer/mino/internal/signals"
	"github.com/codyconfer/mino/internal/signals/active"
	"github.com/codyconfer/mino/internal/signals/build"
)

const (
	serveBuffer      = 256
	sourceDrainGrace = 2 * time.Second
)

var (
	auditEnqueueGrace = 2 * time.Second
	auditDrainGrace   = 10 * time.Second
	auditAbortGrace   = 2 * time.Second
)

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
	onState  func(tray.State)
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
		icon, _ := tray.IconFor(st)
		_ = desktop.Notify(desktop.Notification{
			Title:   note.Title,
			Message: note.Message,
			Icon:    desktop.IconFrom(icon.Name, icon.MIME, icon.Bytes),
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

func (s *Server) sources(ctx context.Context, name string, interval time.Duration, state *active.State) (sources, error) {
	flight := s.Dirs().Flights[name]
	queries, skipped := s.activeQueries(name, flight.Queries, interval, state)

	var wg sync.WaitGroup
	var chans []<-chan signals.Event
	var infos []httpapi.SourceInfo
	for _, open := range s.openStreams(ctx, queries) {
		chans = append(chans, applyFilters(ctx, &wg, track(ctx, &wg, open.ch, open.stop), open.q.filters))
		fmt.Fprintf(os.Stderr, "watching %-10s %s\n", open.q.src.Name(), latencyLabel(open.q.src.LatencyFloor()))
		infos = append(infos, httpapi.SourceInfo{
			Signal:       open.q.src.Name(),
			LatencyFloor: open.q.src.LatencyFloor().String(),
		})
	}
	if sch := s.scheduledEvents(ctx, name, flight.Queries, state); sch != nil {
		chans = append(chans, track(ctx, &wg, sch, nil))
	}
	if len(chans) == 0 {
		e := errs.Newf(errs.KindUsage, "flight %q has no signals with realtime or scheduled support", name)
		if len(skipped) > 0 {
			return sources{}, e.WithHint("%s", strings.Join(skipped, "; "))
		}
		return sources{}, e.WithHint("active: slack, github, calendar, tasks, demo; scheduled: ntr")
	}
	return sources{events: stream.FanIn(ctx, chans...), join: wg.Wait, infos: infos}, nil
}

type sources struct {
	events <-chan signals.Event
	join   func()
	infos  []httpapi.SourceInfo
}

type openStream struct {
	q    activeQuery
	ch   <-chan signals.Event
	stop context.CancelFunc
}

func (s *Server) openStreams(ctx context.Context, queries []activeQuery) []openStream {
	timeout := s.SourceTimeout()
	opened := make([]openStream, len(queries))
	var wg sync.WaitGroup
	for i := range queries {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ch, stop, err := openStreamWithin(ctx, queries[i], timeout)
			if err != nil {
				log.Warnf("serve: %s: %v (skipping)", queries[i].label, err)
				return
			}
			opened[i] = openStream{q: queries[i], ch: ch, stop: stop}
		}()
	}
	wg.Wait()
	out := make([]openStream, 0, len(opened))
	for _, o := range opened {
		if o.ch != nil {
			out = append(out, o)
		}
	}
	return out
}

func openStreamWithin(ctx context.Context, q activeQuery, timeout time.Duration) (<-chan signals.Event, context.CancelFunc, error) {
	sctx, cancel := context.WithCancel(ctx)
	type result struct {
		ch  <-chan signals.Event
		err error
	}
	done := make(chan result, 1)
	go func() {
		ch, err := q.src.Stream(sctx)
		done <- result{ch: ch, err: err}
	}()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case o := <-done:
		if o.err == nil && o.ch == nil {
			o.err = errs.Newf(errs.KindSignal, "opened no event stream")
		}
		if o.err != nil {
			cancel()
			return nil, nil, o.err
		}
		return o.ch, cancel, nil
	case <-timer.C:
		cancel()
		return nil, nil, errs.Newf(errs.KindSignal, "did not open an event stream within %s", timeout)
	case <-ctx.Done():
		cancel()
		return nil, nil, ctx.Err()
	}
}

func track(ctx context.Context, wg *sync.WaitGroup, in <-chan signals.Event, stop context.CancelFunc) <-chan signals.Event {
	out := make(chan signals.Event)
	wg.Add(1)
	go func() {
		defer wg.Done()
		if stop != nil {
			defer stop()
		}
		defer close(out)
		for {
			select {
			case ev, ok := <-in:
				if !ok {
					return
				}
				select {
				case out <- ev:
				case <-ctx.Done():
					drain(in)
					return
				}
			case <-ctx.Done():
				drain(in)
				return
			}
		}
	}()
	return out
}

func drain(in <-chan signals.Event) {
	t := time.NewTimer(sourceDrainGrace)
	defer t.Stop()
	for {
		select {
		case _, ok := <-in:
			if !ok {
				return
			}
		case <-t.C:
			return
		}
	}
}

func (s *Server) activeQueries(flight string, names []string, interval time.Duration, state *active.State) ([]activeQuery, []string) {
	var out []activeQuery
	var skipped []string
	skip := func(query, reason string) { skipped = append(skipped, query+": "+reason) }
	d := s.Dirs()
	for _, name := range names {
		q, ok := d.Queries[name]
		if !ok {
			log.Debugf("serve: unknown query %q in flight %q", name, flight)
			skip(name, "no such query")
			continue
		}
		if !q.Runnable() {
			log.Debugf("serve: %q is filter-only, not a runnable query (skipping)", name)
			skip(name, "filter-only, not a runnable query")
			continue
		}
		resolved, err := d.Resolve(q)
		if err != nil {
			log.Warnf("serve: query %q: %v (skipping)", name, err)
			skip(name, err.Error())
			continue
		}
		params, err := filter.ExpandParams(q.Params, resolved)
		if err != nil {
			log.Warnf("serve: query %q: %v (skipping)", name, err)
			skip(name, err.Error())
			continue
		}
		hs, err := build.ActiveSignal(q.Signal, activeParams(params, interval), s.Role(), s.Cfg, s.Tokens, state)
		if err != nil {
			if errors.Is(err, build.ErrNoActive) {
				log.Debugf("serve: query %q signal %q has no realtime support (skipping)", name, q.Signal)
				skip(name, q.Signal+" has no realtime support")
			} else {
				log.Warnf("serve: query %q: %v (skipping)", name, err)
				skip(name, err.Error())
			}
			continue
		}
		compiled, err := filter.CompileAll(resolved)
		if err != nil {
			log.Warnf("serve: query %q: %v (skipping)", name, err)
			skip(name, err.Error())
			continue
		}
		out = append(out, activeQuery{label: name, src: hs, filters: compiled})
	}
	return out, skipped
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

func applyFilters(ctx context.Context, wg *sync.WaitGroup, in <-chan signals.Event, filters []filter.Compiled) <-chan signals.Event {
	if len(filters) == 0 {
		return in
	}
	out := make(chan signals.Event)
	wg.Add(1)
	go func() {
		defer wg.Done()
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

type auditFeed struct {
	ch       chan signals.Event
	seen     atomic.Int64
	recorded atomic.Int64
	dropped  atomic.Int64
}

func newAuditFeed(buffer int) *auditFeed {
	return &auditFeed{ch: make(chan signals.Event, buffer)}
}

func (f *auditFeed) tee(ctx context.Context, in <-chan signals.Event) <-chan signals.Event {
	out := make(chan signals.Event)
	go func() {
		defer close(out)
		defer close(f.ch)
		for {
			select {
			case <-ctx.Done():
				return
			case ev, ok := <-in:
				if !ok {
					return
				}
				f.seen.Add(1)
				f.offer(ev)
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

func (f *auditFeed) offer(ev signals.Event) {
	select {
	case f.ch <- ev:
		return
	default:
	}
	t := time.NewTimer(auditEnqueueGrace)
	defer t.Stop()
	select {
	case f.ch <- ev:
	case <-t.C:
		f.dropped.Add(1)
		log.Debugf("serve: audit queue still full after %s; dropping a %s event", auditEnqueueGrace, ev.Source)
	}
}

func (f *auditFeed) missing() int64 {
	return f.seen.Load() - f.recorded.Load()
}

func (s *Server) observeAudit(ctx context.Context, f *auditFeed, flightID int64) {
	role := s.Role()
	for ev := range f.ch {
		s.Audit.RecordQueryContext(ctx, flightID, ev.Source, role, ev.At, time.Now(), []signals.Section{ev.Section})
		f.recorded.Add(1)
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

func (s *Server) socket(ctx context.Context, subj *stream.Subject[signals.Event]) func() {
	path := s.SocketPath()
	if ipc.IsListening(config.SocketPrefix, path) {
		log.Debugf("serve: another daemon already owns %s; not exposing a socket", path)
		return func() {}
	}
	ln, err := ipc.Listen(config.SocketPrefix, path)
	if err != nil {
		if errors.Is(err, ipc.ErrInUse) {
			log.Debugf("serve: another daemon already owns %s: %v", path, err)
		} else {
			log.Debugf("serve: socket unavailable: %v", err)
		}
		return func() {}
	}
	go ipc.Broadcast(ctx, ln, subj, serveBuffer, Encode)
	return func() { _ = ln.Close() }
}

func (s *Server) Dial(ctx context.Context) (<-chan signals.Event, bool) {
	events, err := ipc.Dial(ctx, config.SocketPrefix, s.SocketPath(), Decode)
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
	if interval, err := time.ParseDuration(s.Cfg.Daemon.Interval); err == nil && interval > 0 {
		v.DetailPollInterval = interval
	} else {
		v.DetailPollInterval = time.Minute
	}
	return v
}

func (s *Server) fetchDetail(signal string, it signals.Item) (*signals.ItemDetail, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.SourceTimeout())
	defer cancel()
	d, err := build.Detail(ctx, signal, it, s.Role(), s.Cfg, s.Tokens, s.Cache)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func (s *Server) Attach(ctx context.Context) error {
	events, ok := s.Dial(ctx)
	if !ok {
		return errs.Newf(errs.KindUsage, "no running mino daemon at %s", s.SocketPath()).
			WithHint("start one with `mino serve <flight>` (foreground) or `mino daemon` (installed service), or open `mino deck`")
	}
	return s.WatchAttached(events)
}

type RunOptions struct {
	Flight   string
	Interval time.Duration
	Bell     bool
	Desktop  bool
	Terminal bool
	OnState  func(tray.State)

	// HTTP exposes the trigger API on HTTPHost:HTTPPort while serving.
	HTTP            bool
	HTTPHost        string
	HTTPPort        int
	HTTPToken       string
	HTTPTokenSource string
}

func (s *Server) Run(ctx context.Context, opt RunOptions) error {
	cctx, cancel := context.WithCancel(ctx)

	// Bind before opening any source, so a taken port fails with nothing started.
	ln, err := s.apiListener(opt)
	if err != nil {
		cancel()
		return err
	}

	state, closeState := s.openState()
	defer closeState()

	src, err := s.sources(cctx, opt.Flight, opt.Interval, state)
	if err != nil {
		if ln != nil {
			_ = ln.Close()
		}
		cancel()
		return err
	}
	sink := notifySink{bell: opt.Bell, desktop: opt.Desktop, terminal: opt.Terminal, onState: opt.OnState}
	s.watch(cctx, cancel, src, sink, ln, opt)
	return nil
}

type session struct {
	cancel    context.CancelFunc
	src       sources
	closeSock func()
	closeAPI  func()
	subj      *stream.Subject[signals.Event]
	audited   <-chan struct{}
	stopAudit context.CancelFunc
	feed      *auditFeed
	flightID  int64
}

func (s *Server) watch(ctx context.Context, cancel context.CancelFunc, src sources, sink notifySink, ln net.Listener, opt RunOptions) {
	subj := stream.NewSubject[signals.Event]()
	closeSock := s.socket(ctx, subj)
	closeAPI := s.httpAPI(ctx, ln, subj, opt, src.infos)
	flightID := s.Audit.StartFlightContext(ctx, "serve", s.Role())

	auditCtx, stopAudit := context.WithCancel(context.WithoutCancel(ctx))
	feed := newAuditFeed(serveBuffer)
	audited := make(chan struct{})
	notifyCh := subj.Subscribe(serveBuffer)
	go func() {
		defer close(audited)
		s.observeAudit(auditCtx, feed, flightID)
	}()
	go subj.Pump(ctx, feed.tee(ctx, src.events))

	defer s.endSession(session{
		cancel:    cancel,
		src:       src,
		closeSock: closeSock,
		closeAPI:  closeAPI,
		subj:      subj,
		audited:   audited,
		stopAudit: stopAudit,
		feed:      feed,
		flightID:  flightID,
	})
	observeNotify(ctx, notifyCh, sink)
}

func (s *Server) endSession(ses session) {
	ses.cancel()
	// Before subj.Close so SSE handlers unsubscribe from a live Subject, and
	// before drainAudit/FinishFlight so an in-flight run's audit rows land
	// inside the flight they belong to.
	if ses.closeAPI != nil {
		ses.closeAPI()
	}
	if ses.src.join != nil {
		ses.src.join()
	}
	if ses.closeSock != nil {
		ses.closeSock()
	}
	ses.subj.Close()
	s.drainAudit(ses)
	s.Audit.FinishFlight(ses.flightID)
}

func (s *Server) drainAudit(ses session) {
	defer func() {
		if ses.stopAudit != nil {
			ses.stopAudit()
		}
	}()
	if ses.audited == nil {
		return
	}
	t := time.NewTimer(auditDrainGrace)
	defer t.Stop()
	select {
	case <-ses.audited:
		s.reportAuditLoss(ses.feed, "")
		return
	case <-t.C:
	}
	if ses.stopAudit != nil {
		ses.stopAudit()
	}
	select {
	case <-ses.audited:
	case <-time.After(auditAbortGrace):
	}
	s.reportAuditLoss(ses.feed, "audit drain exceeded "+auditDrainGrace.String()+"; ")
}

func (s *Server) reportAuditLoss(f *auditFeed, prefix string) {
	if f == nil {
		return
	}
	missing := f.missing()
	if missing <= 0 && prefix == "" {
		return
	}
	log.Warnf("serve: %s%d of %d event(s) were not recorded to the audit log (%d dropped by a full audit queue)",
		prefix, missing, f.seen.Load(), f.dropped.Load())
}

func stateForEvent(ev signals.Event) tray.State {
	if ev.Section.Err != nil {
		return tray.StateError
	}
	worst := glyph.SeverityNeutral
	for _, it := range ev.Section.Items {
		if s := signals.ClassifyKind(it.Kind); s > worst {
			worst = s
		}
	}
	switch worst {
	case glyph.SeverityNegative:
		return tray.StateError
	case glyph.SeverityWarning:
		return tray.StateWarn
	default:
		return tray.StateNotify
	}
}

func latencyLabel(d time.Duration) string {
	if d <= 0 {
		return "(push/realtime)"
	}
	return "(~" + d.String() + " poll)"
}
