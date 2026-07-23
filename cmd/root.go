package cmd

import (
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"golang.org/x/term"

	"github.com/codyconfer/sisyphus"

	"github.com/codyconfer/munin/internal/audit"
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/log"
	"github.com/codyconfer/munin/internal/token"
)

type app struct {
	cfg        *config.Config
	directives *config.Directives
	audit      *audit.Store
	tokens     *token.Store
	mgr        *sisyphus.Manager
}

var (
	flagOutput     string
	flagHome       string
	flagConfigFile string
	flagRole       string
	flagTimeout    string
	flagVerbose    bool
)

var shared app

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "munin",
		Short: "Aggregate GitHub, Google, and Slack activity into one view",
		Long: "munin (Odin's raven of memory) pulls information from GitHub, Google\n" +
			"Docs, Calendar, Gmail, and Slack and prints it in a nicely formatted way.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			log.SetVerbose(flagVerbose)
			if home, err := config.Home(flagHome); err == nil {
				_ = os.Chmod(home, 0o700)
			}
			interactive := term.IsTerminal(int(os.Stdin.Fd()))
			cfg, directives, mgr, err := config.LoadConfigAndDirectives(flagHome, flagConfigFile, interactive, os.Stdin, os.Stderr)
			if err != nil {
				return err
			}
			if flagOutput != "" {
				cfg.Output = flagOutput
			}
			if flagRole != "" {
				cfg.Role = flagRole
			}
			if flagTimeout != "" {
				cfg.Timeout = flagTimeout
			}
			shared.cfg = cfg
			shared.directives = directives
			shared.mgr = mgr
			openTokens()
			openAudit()
			return enforceOnboarding(cmd)
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

func Shutdown() {
	if shared.audit != nil {
		_ = shared.audit.Close()
	}
	if shared.tokens != nil {
		_ = shared.tokens.Close()
	}
	if shared.mgr != nil {
		_ = shared.mgr.Close()
	}
}

func openTokens() {
	ts, err := token.Open(filepath.Join(shared.cfg.Home, "tokens.duckdb"))
	if err != nil {
		verbosef("token store unavailable: %v", err)
		return
	}
	shared.tokens = ts
}

func openAudit() {
	if !shared.cfg.Audit.Enabled {
		return
	}
	path := shared.cfg.Audit.Path
	if path == "" {
		path = filepath.Join(shared.cfg.Home, "audit.duckdb")
	}
	st, err := audit.Open(path)
	if err != nil {
		verbosef("audit disabled: %v", err)
		return
	}
	shared.audit = st
}
