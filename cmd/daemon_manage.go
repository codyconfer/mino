package cmd

import (
	"fmt"
	"os"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/codyconfer/sisyphus/daemon/service"

	"github.com/codyconfer/munin/internal/deck"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/render"
)

func newDaemonCmd() *cobra.Command {
	var system, yes bool
	c := &cobra.Command{
		Use:   "daemon [flight]",
		Short: "Install and start munin as an OS service (idempotent)",
		Long: "Ensures munin runs as a background OS service (systemd user unit on Linux,\n" +
			"launchd agent on macOS, Windows service): installs it if not present (after a\n" +
			"confirmation), then starts it if not already running. Running again is a no-op.\n" +
			"The service watches the given flight (or the role default) and logs through the\n" +
			"OS logging facility. Manage it with the install/uninstall/start/stop/restart/\n" +
			"status/attach subcommands.",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeFlightNames,
		Annotations:       map[string]string{annoGateMode: modeDaemon},
		RunE: func(cmd *cobra.Command, args []string) error {
			name, err := resolveServeFlight(args)
			if err != nil {
				return err
			}
			svc, err := serveServer().Service(name, configServeInterval(), shared.Cfg.Daemon.Bell, shared.Cfg.Daemon.Desktop, configServeTheme(), !system)
			if err != nil {
				return err
			}
			w := cmd.ErrOrStderr()
			st, sErr := svc.Status()
			if sErr != nil {
				if !yes {
					ok, err := confirmDaemonInstall()
					if err != nil {
						return err
					}
					if !ok {
						return errs.New(errs.KindUsage, "aborted")
					}
				}
				if err := svc.Install(); err != nil {
					return errs.Wrap(errs.KindInternal, err, "installing service")
				}
				if err := svc.Start(); err != nil {
					return errs.Wrap(errs.KindInternal, err, "starting service")
				}
				fmt.Fprintln(w, render.Success(fmt.Sprintf("installed and started munin daemon (%s) watching flight %q", svc.Platform(), name)))
				return nil
			}
			if st != "running" {
				if err := svc.Start(); err != nil {
					return errs.Wrap(errs.KindInternal, err, "starting service")
				}
				fmt.Fprintln(w, render.Success(fmt.Sprintf("started munin daemon watching flight %q", name)))
				return nil
			}
			fmt.Fprintln(w, "munin daemon already running")
			return nil
		},
	}
	c.Flags().BoolVar(&system, "system", false, "target a system-wide daemon (default: per-user)")
	c.Flags().BoolVar(&yes, "yes", false, "skip the install confirmation prompt")
	c.AddCommand(newDaemonControlCmds()...)
	return c
}

func confirmDaemonInstall() (bool, error) {
	if !term.IsTerminal(os.Stdout.Fd()) {
		return false, errs.New(errs.KindUsage, "refusing to install the daemon without --yes (no terminal for confirmation)").
			WithHint("pass --yes to install non-interactively")
	}
	return deck.Confirm("Install munin daemon?",
		"Install munin as an OS service (systemd/launchd/Windows) and start it?",
		"Install", "Cancel")
}

func newDaemonControlCmds() []*cobra.Command {
	var system bool
	install := &cobra.Command{
		Use:               "install [flight]",
		Short:             "Install munin as a system daemon (systemd user unit / launchd agent / Windows service)",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: completeFlightNames,
		Annotations:       map[string]string{annoGateMode: modeDaemon},
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
			fmt.Fprintln(cmd.OutOrStdout(), "start it with: munin daemon start")
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
			Annotations: map[string]string{annoGateMode: modeDaemon},
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
			"a `munin deck` session, or the installed service) over its local socket and shows\n" +
			"the same live notification inbox. Multiple attach clients can watch one daemon.",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{annoGateMode: modeDaemon},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return serveServer().Attach(cmd.Context())
		},
	}

	uninstall := ctl("uninstall", "Remove the installed munin daemon", func(s *service.Service) error { return s.Uninstall() })
	start := ctl("start", "Start the installed munin daemon", func(s *service.Service) error { return s.Start() })
	stop := ctl("stop", "Stop the running munin daemon", func(s *service.Service) error { return s.Stop() })
	restart := ctl("restart", "Restart the munin daemon", func(s *service.Service) error { return s.Restart() })

	var statusSys bool
	status := &cobra.Command{
		Use:         "status",
		Short:       "Show the munin daemon status",
		Args:        cobra.NoArgs,
		Annotations: map[string]string{annoGateMode: modeDaemon},
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
