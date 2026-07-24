package cmd

import (
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/codyconfer/sisyphus/daemon"

	mdaemon "github.com/codyconfer/munin/internal/app/daemon"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/render"
	"github.com/codyconfer/munin/internal/render/icons"
)

func serveServer() *mdaemon.Server {
	return mdaemon.NewServer(shared)
}

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
				bell = shared.Cfg.Daemon.Bell
			}
			if !f.Changed("desktop") {
				desktop = shared.Cfg.Daemon.Desktop
			}
			if !f.Changed("tray") {
				tray = shared.Cfg.Daemon.Tray
			}
			if !f.Changed("theme") {
				theme = configServeTheme()
			}
			srv := serveServer()
			if tuiMode {
				if events, ok := srv.Dial(cmd.Context()); ok {
					verbosef("serve: attaching to running daemon at %s", srv.SocketPath())
					err := srv.WatchAttached(events)
					fmt.Fprintln(cmd.ErrOrStderr(), "serve: shutting down")
					return err
				}
			}
			name, err := resolveServeFlight(args)
			if err != nil {
				return err
			}
			icons.LoadStateIcons(shared.Cfg.Home, theme)
			if tuiMode {
				err := srv.RunTUI(cmd.Context(), name, interval, desktop)
				fmt.Fprintln(cmd.ErrOrStderr(), "serve: shutting down")
				return err
			}
			if tray {
				return srv.RunTray(cmd.Context(), name, interval, bell, desktop)
			}
			if daemon.Interactive() {
				err := srv.Run(cmd.Context(), name, interval, bell, desktop, nil)
				fmt.Fprintln(cmd.ErrOrStderr(), "serve: shutting down")
				return err
			}
			svc, err := srv.Service(name, interval, bell, desktop, theme, true)
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
	if d, err := time.ParseDuration(shared.Cfg.Daemon.Interval); err == nil && d > 0 {
		return d
	}
	return 60 * time.Second
}

func configServeTheme() string {
	if t := shared.Cfg.Daemon.Theme; t != "" {
		return t
	}
	return "dark"
}

func resolveServeFlight(args []string) (string, error) {
	name := defaultFlightName()
	if len(args) == 1 {
		name = args[0]
	}
	if _, ok := shared.Directives.Flights[name]; !ok {
		return "", errs.Newf(errs.KindUsage, "no flight named %q%s", name, availableFlightSuffix()).
			WithHint("run `munin fly` with no argument to list available flights")
	}
	if !access().FlightVisible(name) {
		return "", notInRoleError("flight", name)
	}
	return name, nil
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
			svc, err := serveServer().Service(name, configServeInterval(), shared.Cfg.Daemon.Bell, shared.Cfg.Daemon.Desktop, configServeTheme(), !system)
			if err != nil {
				return err
			}
			if err := svc.Install(); err != nil {
				return errs.Wrap(errs.KindInternal, err, "installing service")
			}
			fmt.Fprintln(cmd.OutOrStdout(), render.Success(fmt.Sprintf("installed munin service (%s) to watch flight %q", svc.Platform(), name)))
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
				svc, err := serveServer().Service(defaultFlightName(), configServeInterval(), shared.Cfg.Daemon.Bell, shared.Cfg.Daemon.Desktop, configServeTheme(), !sys)
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
			return serveServer().Attach(cmd.Context())
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
			svc, err := serveServer().Service(defaultFlightName(), configServeInterval(), shared.Cfg.Daemon.Bell, shared.Cfg.Daemon.Desktop, configServeTheme(), !statusSys)
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
