package cmd

import (
	"os"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/codyconfer/mino/internal/app"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/log"
)

var (
	flagOutput     string
	flagHome       string
	flagConfigFile string
	flagRole       string
	flagTimeout    string
	flagCacheTTL   string
	flagNoCache    bool
	flagRefresh    bool
	flagReconcile  string
	flagVerbose    bool
)

const annoReconcile = "mino_reconcile"

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
		Use:   "mino",
		Short: "Aggregate GitHub, Google, and Slack activity into one view",
		Long: "mino (the myna — the bird that repeats what it hears) pulls information from GitHub, Google\n" +
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
			thin := thinMode(cmd)
			completing := isCompletion(cmd)
			a, err := app.Load(app.Options{
				Home:        flagHome,
				ConfigFile:  flagConfigFile,
				Output:      flagOutput,
				Role:        flagRole,
				Timeout:     flagTimeout,
				CacheTTL:    flagCacheTTL,
				NoCache:     flagNoCache,
				Refresh:     flagRefresh,
				Reconcile:   policy,
				Verbose:     flagVerbose,
				Thin:        thin,
				Completion:  completing,
				Interactive: term.IsTerminal(os.Stdin.Fd()),
				In:          os.Stdin,
				Out:         os.Stderr,
			})
			if err != nil {
				stopLaunchLoading()
				return err
			}
			shared = a
			if completing {
				return nil
			}
			routeLogs(gateMode(cmd), thin)
			if err := gate(cmd); err != nil {
				stopLaunchLoading()
				return err
			}
			return nil
		},
	}

	pf := root.PersistentFlags()
	pf.StringVarP(&flagOutput, "output", "o", "", "output format: terminal or json")
	pf.StringVar(&flagHome, "home", "", "config directory (default ~/.mino)")
	pf.StringVar(&flagHome, "dir", "", "alias for --home")
	_ = pf.MarkHidden("dir")
	pf.StringVar(&flagConfigFile, "config", "", "use this config file for this session only (not persisted)")
	pf.StringVar(&flagRole, "role", "", "active role: scope visible flights/queries/filters")
	pf.StringVar(&flagTimeout, "timeout", "", "per-signal fetch timeout (e.g. 30s, 2m)")
	pf.StringVar(&flagCacheTTL, "cache-ttl", "", "how long cached signal results stay fresh (e.g. 60s, 5m; 0 disables)")
	pf.BoolVar(&flagNoCache, "no-cache", false, "bypass cached signal results and store nothing")
	pf.BoolVar(&flagRefresh, "refresh", false, "ignore cached signal results but store the fresh ones")
	pf.StringVar(&flagReconcile, "reconcile", "", "staged config changes: prompt, apply, session, or ignore")
	pf.BoolVarP(&flagVerbose, "verbose", "v", false, "verbose logging to stderr")
	bindRootCompletions(root)

	root.AddCommand(
		newFlyCmd(),
		newListCmd(),
		newQueryCmd(),
		newFilterCmd(),
		newFormatterCmd(),
		newRoleCmd(),
		newHistoryCmd(),
		newShowCmd(),
		newCacheCmd(),
		newConfigCmd(),
		newBackupCmd(),
		newRestoreCmd(),
		newVerifyCmd(),
		newOnboardCmd(),
		newInstallCmd(),
		newCleanCmd(),
		newNukeCmd(),
		newDeckCmd(),
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
		newServeCmd(),
		newPaneCmd(),
	)
	root.AddCommand(registered()...)
	return root
}

func Root() *cobra.Command { return newRootCmd() }

func Shutdown() {
	shared.Shutdown()
	log.CloseFileSink()
}
