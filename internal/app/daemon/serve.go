package daemon

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	sysdaemon "github.com/codyconfer/sisyphus/daemon"
	"github.com/codyconfer/sisyphus/kv"

	"github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/app/views"
	"github.com/codyconfer/munin/internal/deck"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/filter"
	"github.com/codyconfer/munin/internal/log"
	mnotify "github.com/codyconfer/munin/internal/notify"
	"github.com/codyconfer/munin/internal/signals"
	"github.com/codyconfer/munin/internal/signals/active"
	"github.com/codyconfer/munin/internal/signals/build"
)

const daemonName = "munin"

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
	tray     *sysdaemon.Tray
}

func (n notifySink) handle(ev signals.Event) {
	st := stateForEvent(ev)
	if n.tray != nil {
		n.tray.SetState(st)
	}
	note, show := mnotify.FromEvent(ev)
	if !show {
		return
	}
	if n.desktop {
		icon, _ := sysdaemon.StateIcon(st)
		_ = sysdaemon.Notify(sysdaemon.Notification{Title: note.Title, Message: note.Message, Icon: icon})
	}
	if n.terminal {
		if n.bell {
			fmt.Fprint(os.Stdout, "\a")
		}
		fmt.Fprintln(os.Stdout, mnotify.Render(note))
	}
}

func (s *Server) SocketPath() string { return filepath.Join(s.Cfg.Home, "serve.sock") }

func (s *Server) openState() (*active.State, func()) {
	store, err := kv.Open(filepath.Join(s.Cfg.Home, "serve.duckdb"))
	if err != nil {
		log.Debugf("serve: cursor persistence unavailable: %v", err)
		return active.NewState(nil), func() {}
	}
	return active.NewState(store), func() { _ = store.Close() }
}

func (s *Server) events(ctx context.Context, name string, interval time.Duration, state *active.State) (<-chan signals.Event, error) {
	flight := s.Directives.Flights[name]
	queries := s.activeQueries(name, flight.Queries, interval, state)
	if len(queries) == 0 {
		return nil, errs.Newf(errs.KindUsage, "flight %q has no signals with realtime support", name).
			WithHint("active signals: slack, github, calendar, tasks, demo")
	}

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
	if len(chans) == 0 {
		return nil, errs.New(errs.KindSignal, "no signals could be opened for watching")
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
		hs, err := build.ActiveSignal(q.Signal, activeParams(q.Params, interval), s.Cfg, s.Tokens, state)
		if err != nil {
			if errors.Is(err, build.ErrNoActive) {
				log.Debugf("serve: query %q signal %q has no realtime support (skipping)", name, q.Signal)
			} else {
				log.Warnf("serve: query %q: %v (skipping)", name, err)
			}
			continue
		}
		resolved, err := s.Directives.Resolve(q)
		if err != nil {
			log.Warnf("serve: query %q: %v (skipping)", name, err)
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
	if sysdaemon.IsListening(path) {
		log.Debugf("serve: another daemon already owns %s; not exposing a socket", path)
		return func() {}
	}
	ln, err := sysdaemon.Listen(path)
	if err != nil {
		log.Debugf("serve: socket unavailable: %v", err)
		return func() {}
	}
	go sysdaemon.Broadcast(ctx, ln, subj, serveBuffer, Encode)
	return func() { _ = ln.Close() }
}

func (s *Server) Dial(ctx context.Context) (<-chan signals.Event, bool) {
	events, err := sysdaemon.Dial(ctx, s.SocketPath(), Decode)
	if err != nil {
		return nil, false
	}
	return events, true
}

func (s *Server) WatchAttached(events <-chan signals.Event) error {
	return deck.Run(views.NewServeView("attached", events))
}

func (s *Server) Attach(ctx context.Context) error {
	events, ok := s.Dial(ctx)
	if !ok {
		return errs.Newf(errs.KindUsage, "no running munin daemon at %s", s.SocketPath()).
			WithHint("start one with `munin serve <flight>` or `munin serve start`, or run `munin serve <flight> --tui`")
	}
	return s.WatchAttached(events)
}

func (s *Server) RunTUI(ctx context.Context, name string, interval time.Duration, desktop bool) error {
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	state, closeState := s.openState()
	defer closeState()

	subj, err := s.observable(cctx, name, interval, state)
	if err != nil {
		return err
	}
	defer subj.Close()

	closeSock := s.socket(cctx, subj)
	defer closeSock()

	flightID := s.Audit.StartFlight("serve", s.Cfg.Role)
	defer s.Audit.FinishFlight(flightID)

	go s.observeAudit(subj.Subscribe(serveBuffer), flightID)
	if desktop {
		go observeNotify(cctx, subj.Subscribe(serveBuffer), notifySink{desktop: true})
	}
	return deck.Run(views.NewServeView(name, subj.Subscribe(serveBuffer)))
}

func (s *Server) Run(ctx context.Context, name string, interval time.Duration, bell, desktop bool, tray *sysdaemon.Tray) error {
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	state, closeState := s.openState()
	defer closeState()

	subj, err := s.observable(cctx, name, interval, state)
	if err != nil {
		return err
	}
	defer subj.Close()

	closeSock := s.socket(cctx, subj)
	defer closeSock()

	flightID := s.Audit.StartFlight("serve", s.Cfg.Role)
	defer s.Audit.FinishFlight(flightID)

	go s.observeAudit(subj.Subscribe(serveBuffer), flightID)

	sink := notifySink{bell: bell, desktop: desktop, terminal: tray == nil, tray: tray}
	observeNotify(cctx, subj.Subscribe(serveBuffer), sink)
	return nil
}

func (s *Server) RunTray(parent context.Context, name string, interval time.Duration, bell, desktop bool) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	errCh := make(chan error, 1)
	ready := make(chan struct{})
	var tray *sysdaemon.Tray
	tray = sysdaemon.NewTray(sysdaemon.TrayConfig{
		Title:   "munin",
		Tooltip: "munin",
		Icons:   sysdaemon.DefaultStateIcons(),
		OnQuit:  cancel,
		OnReady: func() {
			close(ready)
			go func() {
				tray.SetState(sysdaemon.StateRunning)
				errCh <- s.Run(ctx, name, interval, bell, desktop, tray)
				cancel()
				tray.Stop()
			}()
		},
	})
	go func() {
		<-ctx.Done()
		tray.Stop()
	}()
	tray.Run()
	select {
	case <-ready:
		return <-errCh
	default:
		return nil
	}
}

func stateForEvent(ev signals.Event) sysdaemon.State {
	if ev.Section.Err != nil {
		return sysdaemon.StateError
	}
	for _, it := range ev.Section.Items {
		switch strings.ToLower(it.Kind) {
		case "mention", "review-requested", "review_requested", "assigned", "alert", "incident", "warn", "warning":
			return sysdaemon.StateWarn
		}
	}
	return sysdaemon.StateNotify
}

func (s *Server) Service(name string, interval time.Duration, bell, desktop bool, theme string, userService bool) (*sysdaemon.Service, error) {
	args := []string{"serve", name, "--interval", interval.String()}
	if !bell {
		args = append(args, "--bell=false")
	}
	if desktop {
		args = append(args, "--desktop")
	}
	if theme != "" && theme != "dark" {
		args = append(args, "--theme", theme)
	}
	return sysdaemon.NewService(sysdaemon.ServiceConfig{
		Name:        daemonName,
		DisplayName: "munin",
		Description: "munin realtime signal watcher",
		Arguments:   args,
		UserService: userService,
	}, func(ctx context.Context) error {
		return s.Run(ctx, name, interval, bell, desktop, nil)
	})
}

func latencyLabel(d time.Duration) string {
	if d <= 0 {
		return "(push/realtime)"
	}
	return "(~" + d.String() + " poll)"
}
