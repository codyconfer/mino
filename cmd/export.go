package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"

	sconfig "github.com/codyconfer/sisyphus/config"
	"github.com/codyconfer/sisyphus/configdb"
	"github.com/codyconfer/sisyphus/redact"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
)

func storeDB() (*configdb.Store, error) {
	if shared.mgr == nil {
		return nil, errs.New(errs.KindStore, "store DB unavailable")
	}
	db := shared.mgr.DB()
	if db == nil {
		return nil, errs.New(errs.KindStore, "store DB unavailable")
	}
	return db, nil
}

const storeConfig = "config"

func collectionDirectives() []string {
	return []string{config.DirQueries, config.DirFilters, config.DirFlights, config.DirRoles}
}

func validDirectives() []string {
	return append([]string{storeConfig}, append(collectionDirectives(), "all")...)
}

func validateDirectiveArg(name string) error {
	for _, s := range validDirectives() {
		if s == name {
			return nil
		}
	}
	return errs.Newf(errs.KindUsage, "unknown directive %q: want one of %v", name, validDirectives())
}

func newExportCmd() *cobra.Command {
	var out string
	var includeSecrets bool
	c := &cobra.Command{
		Use:   "export <directive>",
		Short: "Write a directive's current version from the DuckDB store to files",
		Long: "Materializes the current version of a directive held in the DuckDB store back\n" +
			"onto disk. <directive> is one of: config, queries, filters, flights, roles, all.\n" +
			"config is written as config.yaml/config.json; each collection is written as\n" +
			"individual files under <out>/<directive>/. Defaults to the munin home directory.\n" +
			"Secret values in config are masked unless --include-secrets is set.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runExport(cmd, args[0], out, includeSecrets)
		},
	}
	c.Flags().StringVar(&out, "out", "", "output directory (default: munin home)")
	c.Flags().BoolVar(&includeSecrets, "include-secrets", false, "write secret values in cleartext (default: masked; cleartext is not safe to share)")
	return c
}

func runExport(cmd *cobra.Command, directive, out string, includeSecrets bool) error {
	if err := validateDirectiveArg(directive); err != nil {
		return err
	}
	db, err := storeDB()
	if err != nil {
		return err
	}
	if out == "" {
		out = shared.cfg.Home
	}

	switch directive {
	case "all":
		if err := exportConfig(cmd, db, out, false, includeSecrets); err != nil {
			return err
		}
		for _, name := range collectionDirectives() {
			if err := exportCollection(cmd, db, out, name, false); err != nil {
				return err
			}
		}
	case storeConfig:
		return exportConfig(cmd, db, out, true, includeSecrets)
	default:
		return exportCollection(cmd, db, out, directive, true)
	}
	return nil
}

func exportConfig(cmd *cobra.Command, db *configdb.Store, out string, single, includeSecrets bool) error {
	v, ok, err := db.Current(storeConfig)
	if err != nil {
		return errs.Wrap(errs.KindStore, err, "reading config from store")
	}
	if !ok {
		if single {
			return errs.New(errs.KindStore, "no current version for config in the store").
				WithHint("run `munin import config` first")
		}
		fmt.Fprintln(cmd.OutOrStdout(), "notice: no config version in store, skipping")
		return nil
	}
	content := v.Content
	if includeSecrets {
		fmt.Fprintln(cmd.ErrOrStderr(), "warning: exported config contains secret values in cleartext")
	} else {
		content = redact.Config([]byte(v.Content), v.Format)
	}
	path, err := sconfig.WriteConfigFile(out, []byte(content), v.Format)
	if err != nil {
		return err
	}
	fmt.Fprintf(cmd.OutOrStdout(), "wrote %s\n", path)
	return nil
}

func exportCollection(cmd *cobra.Command, db *configdb.Store, out, name string, single bool) error {
	v, ok, err := db.Current(name)
	if err != nil {
		return errs.Wrapf(errs.KindStore, err, "reading %s from store", name)
	}
	if !ok {
		if single {
			return errs.Newf(errs.KindStore, "no current version for %s in the store", name).
				WithHint("run `munin import %s` first", name)
		}
		fmt.Fprintf(cmd.OutOrStdout(), "notice: no %s version in store, skipping\n", name)
		return nil
	}
	dir := filepath.Join(out, name)
	names, err := sconfig.WriteCollection(dir, []byte(v.Content))
	if err != nil {
		return errs.Wrapf(errs.KindInternal, err, "writing %s", name)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "wrote %d file(s) to %s: %v\n", len(names), dir, names)
	return nil
}

func newImportCmd() *cobra.Command {
	c := &cobra.Command{
		Use:   "import <directive>",
		Short: "Persist on-disk files into the DuckDB store as the current version",
		Long: "Reads directive files from the munin home directory and writes them into the\n" +
			"DuckDB store as a new current version (archiving any prior version).\n" +
			"<directive> is one of: config, queries, filters, flights, roles, all.",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runImport(cmd, args[0])
		},
	}
	return c
}

func runImport(cmd *cobra.Command, directive string) error {
	if err := validateDirectiveArg(directive); err != nil {
		return err
	}
	db, err := storeDB()
	if err != nil {
		return err
	}
	home := shared.cfg.Home

	switch directive {
	case "all":
		if err := importConfig(cmd, db, home); err != nil {
			return err
		}
		for _, name := range collectionDirectives() {
			if err := importCollection(cmd, db, home, name, false); err != nil {
				return err
			}
		}
	case storeConfig:
		return importConfig(cmd, db, home)
	default:
		return importCollection(cmd, db, home, directive, true)
	}
	return nil
}

func importConfig(cmd *cobra.Command, db *configdb.Store, home string) error {
	_, raw, format, err := config.ReadConfigFile(home)
	if err != nil {
		return errs.Wrap(errs.KindConfig, err, "reading config file")
	}
	if len(raw) == 0 {
		return errs.Newf(errs.KindConfig, "no config file found in %s", home).
			WithHint("expected config.yaml, config.yml, or config.json; run `munin install` to create one")
	}
	if err := db.Import(storeConfig, raw, format); err != nil {
		return errs.Wrap(errs.KindStore, err, "importing config")
	}
	fmt.Fprintf(cmd.OutOrStdout(), "imported config (%s, %d bytes)\n", format, len(raw))
	return nil
}

func importCollection(cmd *cobra.Command, db *configdb.Store, home, name string, required bool) error {
	blob, has, err := sconfig.SerializeDir(filepath.Join(home, name))
	if err != nil {
		return errs.Wrapf(errs.KindConfig, err, "reading %s files", name)
	}
	if !has {
		if required {
			return errs.Newf(errs.KindConfig, "no %s files found in %s", name, filepath.Join(home, name))
		}
		fmt.Fprintf(cmd.OutOrStdout(), "notice: no %s files, skipping\n", name)
		return nil
	}
	if err := db.Import(name, blob, "collection"); err != nil {
		return errs.Wrapf(errs.KindStore, err, "importing %s", name)
	}
	fmt.Fprintf(cmd.OutOrStdout(), "imported %s (%d bytes)\n", name, len(blob))
	return nil
}
