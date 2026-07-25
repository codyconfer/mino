package cmd

import (
	"os"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/app"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/log"
)

var (
	flagOutput     string
	flagHome       string
	flagConfigFile string
	flagRole       string
	flagTimeout    string
	flagReconcile  string
	flagVerbose    bool
)

const annoReconcile = "munin_reconcile"

func reconcilePolicyFor(cmd *cobra.Command) (config.ReconcilePolicy, error) {
	if flagReconcile != "" {
		return config.ParseReconcilePolicy(flagReconcile)
	}
	for c := cmd; c != nil; c = c.Parent() {
		if v, ok := c.Annotations[annoReconcile]; ok && v != "" {
			return config.ParseReconcilePolicy(v)
		}
	}
	return config.ReconcilePrompt, nil
}

var shared *app.App

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "munin",
		Short: "Aggregate GitHub, Google, and Slack activity into one view",
		Long: "munin (Odin's raven of memory) pulls information from GitHub, Google\n" +
			"Docs, Calendar, Gmail, and Slack and prints it in a nicely formatted way.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			if skipsAppLoad(cmd) {
				return nil
			}
			policy, err := reconcilePolicyFor(cmd)
			if err != nil {
				return err
			}
			if wantsLaunchLoading(cmd) {
				startLaunchLoading()
			}
			a, err := app.Load(app.Options{
				Home:        flagHome,
				ConfigFile:  flagConfigFile,
				Output:      flagOutput,
				Role:        flagRole,
				Timeout:     flagTimeout,
				Reconcile:   policy,
				Verbose:     flagVerbose,
				Interactive: term.IsTerminal(os.Stdin.Fd()),
				In:          os.Stdin,
				Out:         os.Stderr,
			})
			if err != nil {
				stopLaunchLoading()
				return err
			}
			shared = a
			routeLogs(gateMode(cmd))
			if err := gate(cmd); err != nil {
				stopLaunchLoading()
				return err
			}
			return nil
		},
	}

	pf := root.PersistentFlags()
	pf.StringVarP(&flagOutput, "output", "o", "", "output format: terminal or json")
	pf.StringVar(&flagHome, "home", "", "config directory (default ~/.munin)")
	pf.StringVar(&flagHome, "dir", "", "alias for --home")
	_ = pf.MarkHidden("dir")
	pf.StringVar(&flagConfigFile, "config", "", "use this config file for this session only (not persisted)")
	pf.StringVar(&flagRole, "role", "", "active role: scope visible flights/queries/filters")
	pf.StringVar(&flagTimeout, "timeout", "", "per-signal fetch timeout (e.g. 30s, 2m)")
	pf.StringVar(&flagReconcile, "reconcile", "", "staged config changes: prompt, apply, session, or ignore")
	pf.BoolVarP(&flagVerbose, "verbose", "v", false, "verbose logging to stderr")

	root.AddCommand(
		newFlyCmd(),
		newQueryCmd(),
		newFilterCmd(),
		newRoleCmd(),
		newHistoryCmd(),
		newConfigCmd(),
		newBackupCmd(),
		newRestoreCmd(),
		newVerifyCmd(),
		newOnboardCmd(),
		newInstallCmd(),
		newCleanCmd(),
		newNukeCmd(),
		newDeckCmd(),
		newServeCmd(),
		newDaemonCmd(),
		newSettingsCmd(),
		newExportCmd(),
		newImportCmd(),
		newLoginCmd(),
		newPluginsCmd(),
		newActionCmd(),
		newContextCmd(),
		newNotesCmd(),
		newVersionCmd(),
		newGithubCmd(),
		newCalendarCmd(),
		newGmailCmd(),
		newDocsCmd(),
		newDriveCmd(),
		newTasksCmd(),
		newSlackCmd(),
	)
	return root
}

func Root() *cobra.Command { return newRootCmd() }

func Shutdown() {
	shared.Shutdown()
	shared = nil
	log.CloseFileSink()
}
