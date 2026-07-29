package config

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"slices"
	"time"

	sconfig "github.com/codyconfer/sisyphus/config"
	"github.com/codyconfer/sisyphus/configdb"
	"github.com/codyconfer/sisyphus/redact"

	"github.com/codyconfer/munin/internal/errs"
)

const (
	ConfigDirective     = "config"
	DirectivesDirective = "directives"
)

func LegacyDirectiveRows() []string {
	return []string{DirQueries, DirFlights, KindRoles}
}

func ValidDirectives() []string {
	return []string{ConfigDirective, DirectivesDirective, "all"}
}

func ResolveDirectiveArg(name string) (string, error) {
	if slices.Contains(ValidDirectives(), name) {
		return name, nil
	}
	if slices.Contains(LegacyDirectiveRows(), name) {
		return DirectivesDirective, nil
	}
	return "", errs.Newf(errs.KindUsage, "unknown directive %q: want one of %v", name, ValidDirectives())
}

func ValidateDirectiveArg(name string) error {
	_, err := ResolveDirectiveArg(name)
	return err
}

func PrintCurrentConfig(w io.Writer, db *configdb.Store) error {
	cur, ok, err := db.Current(context.Background(), ConfigDirective)
	if err != nil {
		return err
	}
	if !ok {
		fmt.Fprintln(w, "no config stored yet (no config file found and DB is empty)")
		return nil
	}
	fmt.Fprintf(w, "# active config (%s, applied %s, version %s)\n\n",
		cur.Format, cur.At.Format("2006-01-02 15:04:05"), shortHash(cur.Hash))
	fmt.Fprintln(w, redact.Config([]byte(cur.Content), cur.Format))
	return nil
}

func PrintConfigHistory(w io.Writer, db *configdb.Store) error {
	versions, err := db.History(context.Background(), ConfigDirective, 50)
	if err != nil {
		return err
	}
	if len(versions) == 0 {
		fmt.Fprintln(w, "no archived config versions (config has not changed since first import)")
		return nil
	}
	for _, v := range versions {
		fmt.Fprintf(w, "%s  %-4s  %s\n", v.At.Format(time.RFC3339), v.Format, shortHash(v.Hash))
	}
	return nil
}

func shortHash(h string) string {
	if len(h) > 12 {
		return h[:12]
	}
	return h
}

func Export(w io.Writer, db *configdb.Store, home, directive string, includeSecrets bool) error {
	directive, err := ResolveDirectiveArg(directive)
	if err != nil {
		return err
	}
	switch directive {
	case "all":
		if err := exportConfig(w, db, home, false, includeSecrets); err != nil {
			return err
		}
		return exportDirectives(w, db, home, false)
	case ConfigDirective:
		return exportConfig(w, db, home, true, includeSecrets)
	default:
		return exportDirectives(w, db, home, true)
	}
}

func exportConfig(w io.Writer, db *configdb.Store, out string, single, includeSecrets bool) error {
	v, ok, err := db.Current(context.Background(), ConfigDirective)
	if err != nil {
		return errs.Wrap(errs.KindStore, err, "reading config from store")
	}
	if !ok {
		if single {
			return errs.New(errs.KindStore, "no current version for config in the store").
				WithHint("run `munin import config` first")
		}
		fmt.Fprintln(w, "notice: no config version in store, skipping")
		return nil
	}
	content := v.Content
	if includeSecrets {
		fmt.Fprintln(w, "warning: exported config contains secret values in cleartext")
	} else {
		content = redact.Config([]byte(v.Content), v.Format)
	}
	path, err := sconfig.WriteConfigFile(out, []byte(content), v.Format)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "wrote %s\n", path)
	return nil
}

func exportDirectives(w io.Writer, db *configdb.Store, out string, single bool) error {
	v, ok, err := db.Current(context.Background(), DirectivesDirective)
	if err != nil {
		return errs.Wrap(errs.KindStore, err, "reading directives from store")
	}
	if !ok {
		if single {
			return errs.New(errs.KindStore, "no current version for directives in the store").
				WithHint("run `munin import directives` first")
		}
		fmt.Fprintln(w, "notice: no directives version in store, skipping")
		return nil
	}
	names, err := WriteDirectives(out, []byte(v.Content))
	if err != nil {
		return errs.Wrap(errs.KindInternal, err, "writing directives")
	}
	fmt.Fprintf(w, "wrote %d file(s) under %s: %v\n", len(names), out, names)
	return nil
}

func ExportAllToFiles(db *configdb.Store, home string) ([]string, error) {
	var written []string
	if cur, ok, err := db.Current(context.Background(), ConfigDirective); err != nil {
		return nil, err
	} else if ok {
		path, err := sconfig.WriteConfigFile(home, []byte(cur.Content), cur.Format)
		if err != nil {
			return nil, err
		}
		written = append(written, path)
	}
	cur, ok, err := db.Current(context.Background(), DirectivesDirective)
	if err != nil {
		return nil, err
	}
	if !ok {
		return written, nil
	}
	names, err := WriteDirectives(home, []byte(cur.Content))
	if err != nil {
		return nil, err
	}
	for _, rel := range names {
		written = append(written, filepath.Join(home, filepath.FromSlash(rel)))
	}
	return written, nil
}

func Import(w io.Writer, db *configdb.Store, home, directive string) error {
	directive, err := ResolveDirectiveArg(directive)
	if err != nil {
		return err
	}
	switch directive {
	case "all":
		if err := importConfig(w, db, home, false); err != nil {
			return err
		}
		return importDirectives(w, db, home, false)
	case ConfigDirective:
		return importConfig(w, db, home, true)
	default:
		return importDirectives(w, db, home, true)
	}
}

func importConfig(w io.Writer, db *configdb.Store, home string, required bool) error {
	_, raw, format, err := ReadConfigFile(home)
	if err != nil {
		return errs.Wrap(errs.KindConfig, err, "reading config file")
	}
	if len(raw) == 0 {
		if required {
			return errs.Newf(errs.KindConfig, "no config file found in %s", home).
				WithHint("expected config.yaml, config.yml, or config.json; run `munin install` to create one")
		}
		fmt.Fprintln(w, "notice: no config file, skipping")
		return nil
	}
	if err := db.Import(context.Background(), ConfigDirective, raw, format); err != nil {
		return errs.Wrap(errs.KindStore, err, "importing config")
	}
	fmt.Fprintf(w, "imported config (%s, %d bytes)\n", format, len(raw))
	return nil
}

func importDirectives(w io.Writer, db *configdb.Store, home string, required bool) error {
	blob, has, err := SerializeDirectives(home)
	if err != nil {
		return errs.Wrap(errs.KindConfig, err, "reading directive files")
	}
	if !has {
		if required {
			return errs.Newf(errs.KindConfig, "no directive files found under %s", home)
		}
		fmt.Fprintln(w, "notice: no directive files, skipping")
		return nil
	}
	if err := db.Import(context.Background(), DirectivesDirective, blob, "collection"); err != nil {
		return errs.Wrap(errs.KindStore, err, "importing directives")
	}
	fmt.Fprintf(w, "imported directives (%d bytes)\n", len(blob))
	return nil
}
