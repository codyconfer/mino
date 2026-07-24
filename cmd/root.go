package cmd

import (
	"os"

	"github.com/charmbracelet/x/term"
	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/app"
)

var (
	flagOutput     string
	flagHome       string
	flagConfigFile string
	flagRole       string
	flagTimeout    string
	flagVerbose    bool
)

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
			a, err := app.Load(app.Options{
				Home:        flagHome,
				ConfigFile:  flagConfigFile,
				Output:      flagOutput,
				Role:        flagRole,
				Timeout:     flagTimeout,
				Verbose:     flagVerbose,
				Interactive: term.IsTerminal(os.Stdin.Fd()),
				In:          os.Stdin,
				Out:         os.Stderr,
			})
			if err != nil {
				return err
			}
			shared = a
			return requireOnboarding(cmd)
		},
	}

	pf := root.PersistentFlags()
	pf.StringVarP(&flagOutput, "output", "o", "", "output format: terminal or json")
	pf.StringVar(&flagHome, "home", "", "config directory (default ~/.munin)")
	pf.StringVar(&flagConfigFile, "config", "", "use this config file for this session only (not persisted)")
	pf.StringVar(&flagRole, "role", "", "active role: scope visible flights/queries/filters")
	pf.StringVar(&flagTimeout, "timeout", "", "per-signal fetch timeout (e.g. 30s, 2m)")
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
		newTuiCmd(),
		newDaemonCmd(),
		newSettingsCmd(),
		newExportCmd(),
		newImportCmd(),
		newLoginCmd(),
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

func Shutdown() { shared.Shutdown() }
