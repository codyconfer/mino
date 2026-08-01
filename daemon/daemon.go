//go:build !nodaemon

package daemon

import (
	"fmt"
	"os"
	"time"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/codyconfer/sisyphus/daemon/service"

	"github.com/codyconfer/mino/cmd"
	"github.com/codyconfer/mino/internal/deck"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/render"
)

func newDaemonCmd() *cobra.Command {
	var system, yes bool
	c := &cobra.Command{
		Use:   "daemon [flight]",
		Short: "EXPERIMENTAL: install and start mino as an OS service (idempotent)",
		Long: "EXPERIMENTAL — the OS-service daemon is off by default and only present in\n" +
			"builds made with `-tags daemon`. Its behavior and flags may change.\n\n" +
			"Ensures mino runs as a background OS service (systemd user unit on Linux,\n" +
			"launchd agent on macOS, Windows service): installs it if not present (after a\n" +
			"confirmation), then starts it if not already running. Running again is a no-op.\n" +
			"The service watches the given flight (or the role default) and logs through the\n" +
			"OS logging facility. Set daemon.tray in config for a system-tray icon on the\n" +
			"installed daemon (Linux SNI/AppIndicator, macOS menu bar, Windows notify area).\n" +
			"Manage it with the install/uninstall/start/stop/restart/status/attach subcommands.",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: cmd.CompleteFlights,
		Annotations: map[string]string{
			cmd.AnnoGateMode:      cmd.ModeDaemon,
			cmd.AnnoLaunchLoading: "true",
		},
		RunE: func(c *cobra.Command, args []string) error {
			defer cmd.StopLoading()
			name, err := cmd.ResolveFlight(args)
			if err != nil {
				return err
			}
			svc, err := daemonService(name, !system)
			if err != nil {
				return err
			}
			w := c.ErrOrStderr()
			st, sErr := svc.Status()
			if sErr != nil {
				if !yes {
					cmd.StopLoading()
					ok, err := confirmDaemonInstall()
					if err != nil {
						return err
					}
					if !ok {
						return errs.New(errs.KindUsage, "aborted")
					}
					cmd.StartLoading()
				}
				if err := svc.Install(); err != nil {
					return errs.Wrap(errs.KindInternal, err, "installing service")
				}
				if err := svc.Start(); err != nil {
					return errs.Wrap(errs.KindInternal, err, "starting service")
				}
				cmd.StopLoading()
				fmt.Fprintln(w, render.Success(fmt.Sprintf("installed and started mino daemon (%s) watching flight %q", svc.Platform(), name)))
				return nil
			}
			if st != service.StateRunning {
				if err := svc.Start(); err != nil {
					return errs.Wrap(errs.KindInternal, err, "starting service")
				}
				cmd.StopLoading()
				fmt.Fprintln(w, render.Success(fmt.Sprintf("started mino daemon watching flight %q", name)))
				return nil
			}
			cmd.StopLoading()
			fmt.Fprintln(w, "mino daemon already running")
			return nil
		},
	}
	c.Flags().BoolVar(&system, "system", false, "target a system-wide daemon (default: per-user)")
	c.Flags().BoolVar(&yes, "yes", false, "skip the install confirmation prompt")
	c.AddCommand(newDaemonRunCmd())
	c.AddCommand(newDaemonControlCmds()...)
	return c
}

func newDaemonRunCmd() *cobra.Command {
	var interval time.Duration
	var bell, desktop bool
	var theme string
	c := &cobra.Command{
		Use:         "run [flight]",
		Short:       "Run the daemon watcher (OS service entrypoint)",
		Hidden:      true,
		Args:        cobra.MaximumNArgs(1),
		Annotations: map[string]string{cmd.AnnoGateMode: cmd.ModeDaemon},
		RunE: func(c *cobra.Command, args []string) error {
			f := c.Flags()
			if !f.Changed("interval") {
				interval = cmd.ServeInterval()
			}
			if err := cmd.CheckServeInterval(f.Changed("interval"), interval); err != nil {
				return err
			}
			if !f.Changed("bell") {
				bell = cmd.App().Cfg.Daemon.Bell
			}
			if !f.Changed("desktop") {
				desktop = cmd.App().Cfg.Daemon.Desktop
			}
			if !f.Changed("theme") {
				theme = cmd.ServeTheme()
			}
			name, err := cmd.ResolveFlight(args)
			if err != nil {
				return err
			}
			return watch(c.Context(), options{
				Flight:   name,
				Interval: interval,
				Bell:     bell,
				Desktop:  desktop,
				Tray:     cmd.App().Cfg.Daemon.Tray,
				Theme:    theme,
			})
		},
	}
	c.Flags().DurationVar(&interval, "interval", 60*time.Second, "poll interval floor for polled (non-websocket) signals")
	c.Flags().BoolVar(&bell, "bell", true, "ring the terminal bell on each notification")
	c.Flags().BoolVar(&desktop, "desktop", false, "send OS desktop notifications")
	c.Flags().StringVar(&theme, "theme", "dark", "icon theme for tray/desktop notifications: dark or light")
	return c
}

func confirmDaemonInstall() (bool, error) {
	if !term.IsTerminal(os.Stdout.Fd()) {
		return false, errs.New(errs.KindUsage, "refusing to install the daemon without --yes (no terminal for confirmation)").
			WithHint("pass --yes to install non-interactively")
	}
	return deck.Confirm("Install mino daemon?",
		"Install mino as an OS service (systemd/launchd/Windows) and start it?",
		"Install", "Cancel")
}

func newDaemonControlCmds() []*cobra.Command {
	var system bool
	install := &cobra.Command{
		Use:               "install [flight]",
		Short:             "Install mino as a system daemon (systemd user unit / launchd agent / Windows service)",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: cmd.CompleteFlights,
		Annotations:       map[string]string{cmd.AnnoGateMode: cmd.ModeDaemon},
		RunE: func(c *cobra.Command, args []string) error {
			name, err := cmd.ResolveFlight(args)
			if err != nil {
				return err
			}
			svc, err := daemonService(name, !system)
			if err != nil {
				return err
			}
			if err := svc.Install(); err != nil {
				return errs.Wrap(errs.KindInternal, err, "installing service")
			}
			fmt.Fprintln(c.OutOrStdout(), render.Success(fmt.Sprintf("installed mino service (%s) to watch flight %q", svc.Platform(), name)))
			fmt.Fprintln(c.OutOrStdout(), "start it with: mino daemon start")
			return nil
		},
	}
	install.Flags().BoolVar(&system, "system", false, "install a system-wide daemon (default: per-user)")

	ctl := func(use, short string, action func(*service.Service) error) *cobra.Command {
		var sys bool
		c := &cobra.Command{
			Use:         use,
			Short:       short,
			Args:        cobra.NoArgs,
			Annotations: map[string]string{cmd.AnnoGateMode: cmd.ModeDaemon},
			RunE: func(_ *cobra.Command, _ []string) error {
				svc, err := daemonService(cmd.DefaultFlight(), !sys)
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
		Short: "Attach a TUI to a running mino daemon and watch its live notifications",
		Long: "Connects to a mino daemon already watching a flight (a foreground `mino serve`,\n" +
			"a `mino deck` session, or the installed service) over its local socket and shows\n" +
			"the same live notification inbox. Multiple attach clients can watch one daemon.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{cmd.AnnoGateMode: cmd.ModeDaemon},
		RunE: func(c *cobra.Command, _ []string) error {
			return server().Attach(c.Context())
		},
	}

	uninstall := ctl("uninstall", "Remove the installed mino daemon", func(s *service.Service) error { return s.Uninstall() })
	start := ctl("start", "Start the installed mino daemon", func(s *service.Service) error { return s.Start() })
	stop := ctl("stop", "Stop the running mino daemon", func(s *service.Service) error { return s.Stop() })
	restart := ctl("restart", "Restart the mino daemon", func(s *service.Service) error { return s.Restart() })

	var statusSys bool
	status := &cobra.Command{
		Use:         "status",
		Short:       "Show the mino daemon status",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{cmd.AnnoGateMode: cmd.ModeDaemon},
		RunE: func(c *cobra.Command, _ []string) error {
			svc, err := daemonService(cmd.DefaultFlight(), !statusSys)
			if err != nil {
				return err
			}
			st, err := svc.Status()
			if err != nil {
				fmt.Fprintf(c.OutOrStdout(), "mino daemon: %s (%v)\n", st, err)
				return nil
			}
			fmt.Fprintf(c.OutOrStdout(), "mino daemon: %s\n", st)
			return nil
		},
	}
	status.Flags().BoolVar(&statusSys, "system", false, "target the system-wide daemon")

	return []*cobra.Command{install, uninstall, start, stop, restart, status, attach}
}
