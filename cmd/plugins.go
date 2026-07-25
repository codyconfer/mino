package cmd

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/plugin/scaffold"
	"github.com/codyconfer/munin/internal/signals/build"
)

func newPluginsCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plugins",
		Short: "List, enable/disable, scaffold, and install config seeds for compile-time plugins",
		Long: `Plugins are Go packages linked into this binary at compile time.
There is no runtime .so / plugin.Open loading.

  list                     — registered plugins and enablement (CLI discovery)
  enable / disable         — runtime activation via disabled_plugins (keeps installed)
  install / uninstall      — add/remove from installed_plugins plus provision or remove
                           example directive seeds into the munin home; disable alone
                           does not uninstall. Overlays may register seeds via
                           munin/plugin.RegisterSeeds
  scaffold                 — generate an overlay-friendly plugin package from the
                           canonical template (public munin/plugin SDK)

Use munin install for the home scaffold, and munin daemon install for the OS service.`,
	}
	cmd.AddCommand(
		newPluginsListCmd(),
		newPluginsEnableCmd(),
		newPluginsDisableCmd(),
		newPluginsInstallCmd(),
		newPluginsUninstallCmd(),
		newPluginsScaffoldCmd(),
	)
	return cmd
}

func newPluginsListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List registered plugins",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			_ = build.KnownSignals()
			plugin.LoadEnabled()
			out := cmd.OutOrStdout()
			for _, row := range plugin.ListEnabled() {
				d, _ := plugin.Lookup(row.ID)
				state := "enabled"
				if !row.Enabled {
					state = "disabled"
				}
				caps := make([]string, len(d.Capabilities))
				for i, c := range d.Capabilities {
					caps[i] = string(c)
				}
				fmt.Fprintf(out, "%-18s %-8s kind=%s signal=%s caps=[%s]\n",
					row.ID, state, d.Kind, d.Signal, strings.Join(caps, ","))
			}
			return nil
		},
	}
}

func newPluginsEnableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "enable <plugin-id>",
		Short: "Enable a registered plugin",
		Long:  "Removes plugin-id from disabled_plugins. Does not write example directives; use install for that.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = build.KnownSignals()
			if err := plugin.SetEnabled(args[0], true); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "enabled %s\n", args[0])
			return nil
		},
	}
}

func newPluginsDisableCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "disable <plugin-id>",
		Short: "Disable a registered plugin",
		Long:  "Adds plugin-id to disabled_plugins. Does not remove example directives; use uninstall for that.",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = build.KnownSignals()
			if err := plugin.SetEnabled(args[0], false); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "disabled %s\n", args[0])
			return nil
		},
	}
}

func newPluginsInstallCmd() *cobra.Command {
	var force bool
	c := &cobra.Command{
		Use:   "install <plugin-id>",
		Short: "Install a plugin (managed set), enable it, and provision seeds",
		Long: `Add a compile-time registered plugin to installed_plugins, enable it, and
write its example queries/flights (and similar) into the munin home directory.

This is config/seed install only — it does not download or dynamically load
plugin binaries. Unknown ids (not linked into this binary) are rejected.

Seeds match examples/ for stock plugins; overlays register extras with
github.com/codyconfer/munin/plugin.RegisterSeeds. Existing files are left
unchanged unless --force.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = build.KnownSignals()
			home, err := config.Home(flagHome)
			if err != nil {
				return err
			}
			res, err := plugin.Install(home, args[0], plugin.InstallOptions{Force: force})
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "installed %s (enabled", res.PluginID)
			if n := len(plugin.SeedsFor(res.PluginID)); n == 0 {
				fmt.Fprintf(out, "; no example seeds for this plugin)\n")
			} else {
				fmt.Fprintf(out, "; wrote %d, skipped %d)\n", len(res.Written), len(res.Skipped))
			}
			for _, p := range res.Written {
				fmt.Fprintf(out, "  wrote %s\n", p)
			}
			for _, p := range res.Skipped {
				fmt.Fprintf(out, "  skipped %s\n", p)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&force, "force", false, "overwrite existing seed files")
	return c
}

func newPluginsUninstallCmd() *cobra.Command {
	var keepSeeds, force bool
	c := &cobra.Command{
		Use:   "uninstall <plugin-id>",
		Short: "Uninstall a plugin (remove from managed set) and matching seeds",
		Long: `Remove a compile-time registered plugin from installed_plugins, disable it,
and remove example directive seeds that still match the catalog. Modified
seeds are kept unless --force. Pass --keep-seeds to uninstall without
removing seed files (still removes from the managed/installed set).

Disable alone keeps the plugin installed/listed; uninstall is what drops it.
Compile-linked plugin code remains in the binary.`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			_ = build.KnownSignals()
			home, err := config.Home(flagHome)
			if err != nil {
				return err
			}
			res, err := plugin.Uninstall(home, args[0], plugin.UninstallOptions{
				KeepSeeds: keepSeeds,
				Force:     force,
			})
			if err != nil {
				return err
			}
			out := cmd.OutOrStdout()
			fmt.Fprintf(out, "uninstalled %s (disabled; removed %d, kept %d)\n",
				res.PluginID, len(res.Removed), len(res.Kept))
			for _, p := range res.Removed {
				fmt.Fprintf(out, "  removed %s\n", p)
			}
			for _, p := range res.Kept {
				fmt.Fprintf(out, "  kept %s\n", p)
			}
			return nil
		},
	}
	c.Flags().BoolVar(&keepSeeds, "keep-seeds", false, "uninstall without removing example directive files")
	c.Flags().BoolVar(&force, "force", false, "remove seed files even if modified")
	return c
}

func newPluginsScaffoldCmd() *cobra.Command {
	var dir, signal, pkg string
	var force bool
	c := &cobra.Command{
		Use:   "scaffold <plugin-id>",
		Short: "Generate an overlay-friendly plugin package",
		Annotations: map[string]string{
			annoSkipOnboarding: "true",
			annoSkipAppLoad:    "true",
		},
		Long: `Write a CapQuery plugin package into --dir using the public munin/plugin SDK.

The generated package includes:
  plugin.go       — RegisterSignal, glyph, RegisterContext, RegisterFilterEngine
  plugin_test.go  — fixture registration test
  queries/*.yaml  — example query seed referencing the filter engine

Import the package from app.Options.RegisterPlugins in an overlay binary.
This does not link the plugin into the running munin binary — compile-time
registration is still required.

Example:
  munin plugins scaffold team.example --dir ./plugins/example`,
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := strings.TrimSpace(args[0])
			if id == "" {
				return errs.Newf(errs.KindUsage, "plugin id required")
			}
			out := strings.TrimSpace(dir)
			if out == "" {
				base := id
				if i := strings.LastIndex(id, "."); i >= 0 {
					base = id[i+1:]
				}
				out = filepath.Join(".", base)
			}
			res, err := scaffold.Generate(scaffold.GenerateOptions{
				ID:      id,
				Dir:     out,
				Signal:  signal,
				Package: pkg,
				Force:   force,
			})
			if err != nil {
				return errs.Wrap(errs.KindUsage, err, "scaffold")
			}
			fmt.Fprintf(cmd.OutOrStdout(), "scaffolded %s → %s\n", id, res.Dir)
			for _, p := range res.Written {
				fmt.Fprintf(cmd.OutOrStdout(), "  wrote %s\n", p)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Next: call %s.Register() from app.Options.RegisterPlugins\n",
				filepath.Base(res.Dir))
			return nil
		},
	}
	c.Flags().StringVar(&dir, "dir", "", "output directory (default: ./<id-last-segment>)")
	c.Flags().StringVar(&signal, "signal", "", "config signal name (default: id last segment)")
	c.Flags().StringVar(&pkg, "package", "", "Go package name (default: sanitized signal)")
	c.Flags().BoolVar(&force, "force", false, "overwrite existing generated files")
	return c
}
