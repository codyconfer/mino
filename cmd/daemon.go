package cmd

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/codyconfer/sisyphus/daemon"
	"github.com/codyconfer/sisyphus/kv"

	"github.com/codyconfer/munin/internal/active"
	mdaemon "github.com/codyconfer/munin/internal/daemon"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/filter"
	"github.com/codyconfer/munin/internal/icons"
	mnotify "github.com/codyconfer/munin/internal/notify"
	"github.com/codyconfer/munin/internal/signals"
	"github.com/codyconfer/munin/internal/signals/demo"
	"github.com/codyconfer/munin/internal/tui"
	"github.com/codyconfer/munin/internal/views"
)

var errNoActiveSignal = errs.New(errs.KindUsage, "signal has no active (streaming) implementation")

type activeJob struct {
	label   string
	src     signals.ActiveSignal
	filters []filter.Compiled
}

const daemonName = "munin"

func newDaemonCmd() *cobra.Command {
	var interval time.Duration
	var bell, desktop, tray, tuiMode bool
	var theme string
	c := &cobra.Command{
		Use:   "serve [flight]",
		Short: "Run munin as a long-running daemon that watches signals in realtime and notifies",
		Long: "Runs munin as a long-running process that opens each of the flight's signals\n" +
			"that supports realtime (an active signal), fans their events into one loop, and\n" +
			"emits a notification for each new item.\n\n" +
			"Run in the foreground (Ctrl-C exits), with --tui for an interactive inbox of\n" +
			"live notifications, --desktop for OS notifications and/or --tray for a system-tray\n" +
			"icon that reflects state, or manage it as a system daemon\n" +
			"with the install/uninstall/start/stop/restart/status subcommands (systemd user\n" +
			"unit on Linux, launchd agent on macOS, Windows service). Per-state icons load\n" +
			"from <home>/icons/\n" +
			"(inactive|running|notify|warn|error).png. Only Slack is a true websocket; the\n" +
			"rest are polled at --interval; unsupported signals are skipped.",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeFlightNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			f := cmd.Flags()
			if !f.Changed("interval") {
				interval = configServeInterval()
			}
			if !f.Changed("bell") {
				bell = shared.cfg.Daemon.Bell
			}
			if !f.Changed("desktop") {
				desktop = shared.cfg.Daemon.Desktop
			}
			if !f.Changed("tray") {
				tray = shared.cfg.Daemon.Tray
			}
			if !f.Changed("theme") {
				theme = configServeTheme()
			}
			if tuiMode {
				if events, ok := dialServe(cmd.Context()); ok {
					verbosef("serve: attaching to running daemon at %s", serveSocketPath())
					err := tui.Run(views.NewServeView("attached", events))
					fmt.Fprintln(cmd.ErrOrStderr(), "serve: shutting down")
					return err
				}
			}
			name, err := resolveServeFlight(args)
			if err != nil {
				return err
			}
			icons.LoadStateIcons(shared.cfg.Home, theme)
			if tuiMode {
				err := runServeTUI(cmd.Context(), name, interval, desktop)
				fmt.Fprintln(cmd.ErrOrStderr(), "serve: shutting down")
				return err
			}
			if tray {
				return runServeTray(cmd.Context(), name, interval, bell, desktop)
			}
			if daemon.Interactive() {
				err := runServe(cmd.Context(), name, interval, bell, desktop, nil)
				fmt.Fprintln(cmd.ErrOrStderr(), "serve: shutting down")
				return err
			}
			svc, err := serveDaemon(name, interval, bell, desktop, theme, true)
			if err != nil {
				return err
			}
			return svc.Run()
		},
	}
	c.Flags().DurationVar(&interval, "interval", 60*time.Second, "poll interval floor for polled (non-websocket) signals")
	c.Flags().BoolVar(&bell, "bell", true, "ring the terminal bell on each notification")
	c.Flags().BoolVar(&desktop, "desktop", false, "send OS desktop notifications (uses per-state icons from <home>/icons/)")
	c.Flags().BoolVar(&tray, "tray", false, "show a system-tray icon reflecting daemon state (foreground desktop session only)")
	c.Flags().BoolVar(&tuiMode, "tui", false, "show the live-notification TUI inbox; attaches to a running daemon if one exists, else starts one (foreground)")
	c.Flags().StringVar(&theme, "theme", "dark", "icon theme for tray/desktop notifications: dark or light")
	c.AddCommand(newServeControlCmds()...)
	return c
}

func configServeInterval() time.Duration {
	if d, err := time.ParseDuration(shared.cfg.Daemon.Interval); err == nil && d > 0 {
		return d
	}
	return 60 * time.Second
}

func configServeTheme() string {
	if t := shared.cfg.Daemon.Theme; t != "" {
		return t
	}
	return "dark"
}

func resolveServeFlight(args []string) (string, error) {
	name := defaultFlightName()
	if len(args) == 1 {
		name = args[0]
	}
	if _, ok := shared.directives.Flights[name]; !ok {
		return "", errs.Newf(errs.KindUsage, "no flight named %q%s", name, availableFlightSuffix()).
			WithHint("run `munin fly` with no argument to list available flights")
	}
	if !access().FlightVisible(name) {
		return "", notInRoleError("flight", name)
	}
	return name, nil
}

func openServeState() (*active.State, func()) {
	store, err := kv.Open(filepath.Join(shared.cfg.Home, "serve.duckdb"))
	if err != nil {
		verbosef("serve: cursor persistence unavailable: %v", err)
		return active.NewState(nil), func() {}
	}
	return active.NewState(store), func() { _ = store.Close() }
}

func serveEvents(ctx context.Context, name string, interval time.Duration, state *active.State) (<-chan signals.Event, error) {
	flight := shared.directives.Flights[name]
	jobs := activeJobs(name, flight.Queries, interval, state)
	if len(jobs) == 0 {
		return nil, errs.Newf(errs.KindUsage, "flight %q has no signals with realtime support", name).
			WithHint("active signals: slack, github, calendar, tasks, demo")
	}

	var chans []<-chan signals.Event
	for _, j := range jobs {
		ch, err := j.src.Stream(ctx)
		if err != nil {
			warnf("serve: %s: %v (skipping)", j.label, err)
			continue
		}
		chans = append(chans, applyFilters(ctx, ch, j.filters))
		fmt.Fprintf(os.Stderr, "watching %-10s %s\n", j.src.Name(), latencyLabel(j.src.LatencyFloor()))
	}
	if len(chans) == 0 {
		return nil, errs.New(errs.KindSignal, "no signals could be opened for watching")
	}
	return daemon.FanIn(ctx, chans...), nil
}

const serveBuffer = 256

type notifySink struct {
	bell     bool
	desktop  bool
	terminal bool
	tray     *daemon.Tray
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
		icon, _ := daemon.StateIcon(st)
		_ = daemon.Notify(daemon.Notification{Title: note.Title, Message: note.Message, Icon: icon})
	}
	if n.terminal {
		if n.bell {
			fmt.Fprint(os.Stdout, "\a")
		}
		fmt.Fprintln(os.Stdout, mnotify.Render(note))
	}
}

func observeAudit(ch <-chan signals.Event, flightID int64) {
	for ev := range ch {
		shared.audit.RecordQuery(flightID, ev.Source, shared.cfg.Role, ev.At, time.Now(), []signals.Section{ev.Section})
	}
}

func observeNotify(ch <-chan signals.Event, sink notifySink) {
	for ev := range ch {
		sink.handle(ev)
	}
}

func observeNotifyLoop(ctx context.Context, ch <-chan signals.Event, sink notifySink) {
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

func serveObservable(ctx context.Context, name string, interval time.Duration, state *active.State) (*daemon.Subject[signals.Event], error) {
	events, err := serveEvents(ctx, name, interval, state)
	if err != nil {
		return nil, err
	}
	subj := daemon.NewSubject[signals.Event]()
	go subj.Pump(ctx, events)
	return subj, nil
}

func serveSocketPath() string { return filepath.Join(shared.cfg.Home, "serve.sock") }

func serveSocket(ctx context.Context, subj *daemon.Subject[signals.Event]) func() {
	path := serveSocketPath()
	if daemon.IsListening(path) {
		verbosef("serve: another daemon already owns %s; not exposing a socket", path)
		return func() {}
	}
	ln, err := daemon.Listen(path)
	if err != nil {
		verbosef("serve: socket unavailable: %v", err)
		return func() {}
	}
	go daemon.Broadcast(ctx, ln, subj, serveBuffer, mdaemon.Encode)
	return func() { _ = ln.Close() }
}

func dialServe(ctx context.Context) (<-chan signals.Event, bool) {
	events, err := daemon.Dial(ctx, serveSocketPath(), mdaemon.Decode)
	if err != nil {
		return nil, false
	}
	return events, true
}

func runServeAttach(ctx context.Context) error {
	events, ok := dialServe(ctx)
	if !ok {
		return errs.Newf(errs.KindUsage, "no running munin daemon at %s", serveSocketPath()).
			WithHint("start one with `munin serve <flight>` or `munin serve start`, or run `munin serve <flight> --tui`")
	}
	return tui.Run(views.NewServeView("attached", events))
}

func runServeTUI(ctx context.Context, name string, interval time.Duration, desktop bool) error {
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	state, closeState := openServeState()
	defer closeState()

	subj, err := serveObservable(cctx, name, interval, state)
	if err != nil {
		return err
	}
	defer subj.Close()

	closeSock := serveSocket(cctx, subj)
	defer closeSock()

	flightID := shared.audit.StartFlight("serve", shared.cfg.Role)
	defer shared.audit.FinishFlight(flightID)

	go observeAudit(subj.Subscribe(serveBuffer), flightID)
	if desktop {
		go observeNotify(subj.Subscribe(serveBuffer), notifySink{desktop: true})
	}
	return tui.Run(views.NewServeView(name, subj.Subscribe(serveBuffer)))
}

func runServe(ctx context.Context, name string, interval time.Duration, bell, desktop bool, tray *daemon.Tray) error {
	cctx, cancel := context.WithCancel(ctx)
	defer cancel()

	state, closeState := openServeState()
	defer closeState()

	subj, err := serveObservable(cctx, name, interval, state)
	if err != nil {
		return err
	}
	defer subj.Close()

	closeSock := serveSocket(cctx, subj)
	defer closeSock()

	flightID := shared.audit.StartFlight("serve", shared.cfg.Role)
	defer shared.audit.FinishFlight(flightID)

	go observeAudit(subj.Subscribe(serveBuffer), flightID)

	sink := notifySink{bell: bell, desktop: desktop, terminal: tray == nil, tray: tray}
	observeNotifyLoop(cctx, subj.Subscribe(serveBuffer), sink)
	return nil
}

func runServeTray(parent context.Context, name string, interval time.Duration, bell, desktop bool) error {
	ctx, cancel := context.WithCancel(parent)
	defer cancel()
	errCh := make(chan error, 1)
	ready := make(chan struct{})
	var tray *daemon.Tray
	tray = daemon.NewTray(daemon.TrayConfig{
		Title:   "munin",
		Tooltip: "munin",
		Icons:   daemon.DefaultStateIcons(),
		OnQuit:  cancel,
		OnReady: func() {
			close(ready)
			go func() {
				tray.SetState(daemon.StateRunning)
				errCh <- runServe(ctx, name, interval, bell, desktop, tray)
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

func stateForEvent(ev signals.Event) daemon.State {
	if ev.Section.Err != nil {
		return daemon.StateError
	}
	for _, it := range ev.Section.Items {
		switch strings.ToLower(it.Kind) {
		case "mention", "review-requested", "review_requested", "assigned", "alert", "incident", "warn", "warning":
			return daemon.StateWarn
		}
	}
	return daemon.StateNotify
}

func serveDaemon(name string, interval time.Duration, bell, desktop bool, theme string, userService bool) (*daemon.Service, error) {
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
	return daemon.NewService(daemon.ServiceConfig{
		Name:        daemonName,
		DisplayName: "munin",
		Description: "munin realtime signal watcher",
		Arguments:   args,
		UserService: userService,
	}, func(ctx context.Context) error {
		return runServe(ctx, name, interval, bell, desktop, nil)
	})
}

func newServeControlCmds() []*cobra.Command {
	var system bool
	install := &cobra.Command{
		Use:               "install [flight]",
		Short:             "Install munin serve as a system daemon (systemd user unit / launchd agent / Windows service)",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeFlightNames,
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := resolveServeFlight(args)
			if err != nil {
				return err
			}
			svc, err := serveDaemon(name, configServeInterval(), shared.cfg.Daemon.Bell, shared.cfg.Daemon.Desktop, configServeTheme(), !system)
			if err != nil {
				return err
			}
			if err := svc.Install(); err != nil {
				return errs.Wrap(errs.KindInternal, err, "installing service")
			}
			fmt.Fprintf(cmd.OutOrStdout(), "installed munin service (%s) to watch flight %q\n", svc.Platform(), name)
			fmt.Fprintln(cmd.OutOrStdout(), "start it with: munin serve start")
			return nil
		},
	}
	install.Flags().BoolVar(&system, "system", false, "install a system-wide daemon (default: per-user)")

	ctl := func(use, short string, action func(*daemon.Service) error) *cobra.Command {
		var sys bool
		c := &cobra.Command{
			Use:   use,
			Short: short,
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, _ []string) error {
				svc, err := serveDaemon(defaultFlightName(), configServeInterval(), shared.cfg.Daemon.Bell, shared.cfg.Daemon.Desktop, configServeTheme(), !sys)
				if err != nil {
					return err
				}
				if err := action(svc); err != nil {
					return errs.Wrapf(errs.KindInternal, err, "%s service", use)
				}
				return nil
			},
		}
		c.Flags().BoolVar(&sys, "system", false, "target the system-wide daemon")
		return c
	}

	attach := &cobra.Command{
		Use:   "attach",
		Short: "Attach a TUI to a running munin daemon and watch its live notifications",
		Long: "Connects to a munin daemon already watching a flight (a foreground `munin serve`,\n" +
			"a `--tui` session, or the installed service) over its local socket and shows the\n" +
			"same live notification inbox. Multiple attach clients can watch one daemon at once.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runServeAttach(cmd.Context())
		},
	}

	uninstall := ctl("uninstall", "Remove the installed munin daemon", func(s *daemon.Service) error { return s.Uninstall() })
	start := ctl("start", "Start the installed munin daemon", func(s *daemon.Service) error { return s.Start() })
	stop := ctl("stop", "Stop the running munin daemon", func(s *daemon.Service) error { return s.Stop() })
	restart := ctl("restart", "Restart the munin daemon", func(s *daemon.Service) error { return s.Restart() })

	var statusSys bool
	status := &cobra.Command{
		Use:   "status",
		Short: "Show the munin daemon status",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			svc, err := serveDaemon(defaultFlightName(), configServeInterval(), shared.cfg.Daemon.Bell, shared.cfg.Daemon.Desktop, configServeTheme(), !statusSys)
			if err != nil {
				return err
			}
			st, err := svc.Status()
			if err != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "munin daemon: %s (%v)\n", st, err)
				return nil
			}
			fmt.Fprintf(cmd.OutOrStdout(), "munin daemon: %s\n", st)
			return nil
		},
	}
	status.Flags().BoolVar(&statusSys, "system", false, "target the system-wide daemon")

	return []*cobra.Command{install, uninstall, start, stop, restart, status, attach}
}

func activeJobs(flight string, queries []string, interval time.Duration, state *active.State) []activeJob {
	var jobs []activeJob
	for _, name := range queries {
		q, ok := shared.directives.Queries[name]
		if !ok {
			verbosef("serve: unknown query %q in flight %q", name, flight)
			continue
		}
		hs, err := buildActiveSignal(q.Signal, activeParams(q.Params, interval), state)
		if err != nil {
			if errors.Is(err, errNoActiveSignal) {
				verbosef("serve: query %q signal %q has no realtime support (skipping)", name, q.Signal)
			} else {
				warnf("serve: query %q: %v (skipping)", name, err)
			}
			continue
		}
		resolved, err := shared.directives.Resolve(q)
		if err != nil {
			warnf("serve: query %q: %v (skipping)", name, err)
			continue
		}
		compiled, err := filter.CompileAll(resolved)
		if err != nil {
			warnf("serve: query %q: %v (skipping)", name, err)
			continue
		}
		jobs = append(jobs, activeJob{label: name, src: hs, filters: compiled})
	}
	return jobs
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

func latencyLabel(d time.Duration) string {
	if d <= 0 {
		return "(push/realtime)"
	}
	return "(~" + d.String() + " poll)"
}

func buildActiveSignal(name string, params map[string]string, state *active.State) (signals.ActiveSignal, error) {
	switch name {
	case "demo":
		return demo.Signal{}, nil
	case "slack":
		return buildActiveSlack(params)
	case "github":
		return buildActiveGithub(params, state)
	case "calendar":
		return buildActiveCalendar(params, state)
	case "tasks":
		return buildActiveTasks(params, state)
	default:
		return nil, errNoActiveSignal
	}
}
