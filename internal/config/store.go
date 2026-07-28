package config

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"time"

	sconfig "github.com/codyconfer/sisyphus/config"
	"github.com/codyconfer/sisyphus/configdb"
	"github.com/codyconfer/sisyphus/redact"

	"github.com/codyconfer/munin/internal/errs"
)

const ConfigDirective = "config"

func CollectionDirectives() []string {
	return []string{DirQueries, DirFlights, KindRoles}
}

func ValidDirectives() []string {
	return append([]string{ConfigDirective}, append(CollectionDirectives(), "all")...)
}

func ValidateDirectiveArg(name string) error {
	for _, s := range ValidDirectives() {
		if s == name {
			return nil
		}
	}
	return errs.Newf(errs.KindUsage, "unknown directive %q: want one of %v", name, ValidDirectives())
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
	if err := ValidateDirectiveArg(directive); err != nil {
		return err
	}
	switch directive {
	case "all":
		if err := exportConfig(w, db, home, false, includeSecrets); err != nil {
			return err
		}
		for _, name := range CollectionDirectives() {
			if err := exportCollection(w, db, home, name, false); err != nil {
				return err
			}
		}
	case ConfigDirective:
		return exportConfig(w, db, home, true, includeSecrets)
	default:
		return exportCollection(w, db, home, directive, true)
	}
	return nil
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

func exportCollection(w io.Writer, db *configdb.Store, out, name string, single bool) error {
	v, ok, err := db.Current(context.Background(), name)
	if err != nil {
		return errs.Wrapf(errs.KindStore, err, "reading %s from store", name)
	}
	if !ok {
		if single {
			return errs.Newf(errs.KindStore, "no current version for %s in the store", name).
				WithHint("run `munin import %s` first", name)
		}
		fmt.Fprintf(w, "notice: no %s version in store, skipping\n", name)
		return nil
	}
	dir := CollectionDir(out, name)
	names, err := WriteCollection(out, name, []byte(v.Content))
	if err != nil {
		return errs.Wrapf(errs.KindInternal, err, "writing %s", name)
	}
	fmt.Fprintf(w, "wrote %d file(s) to %s: %v\n", len(names), dir, names)
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
	for _, name := range CollectionDirectives() {
		cur, ok, err := db.Current(context.Background(), name)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		dir := CollectionDir(home, name)
		names, err := WriteCollection(home, name, []byte(cur.Content))
		if err != nil {
			return nil, err
		}
		for _, fn := range names {
			written = append(written, filepath.Join(dir, fn))
		}
	}
	return written, nil
}

func Import(w io.Writer, db *configdb.Store, home, directive string) error {
	if err := ValidateDirectiveArg(directive); err != nil {
		return err
	}
	switch directive {
	case "all":
		if err := importConfig(w, db, home, false); err != nil {
			return err
		}
		for _, name := range CollectionDirectives() {
			if err := importCollection(w, db, home, name, false); err != nil {
				return err
			}
		}
	case ConfigDirective:
		return importConfig(w, db, home, true)
	default:
		return importCollection(w, db, home, directive, true)
	}
	return nil
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

func importCollection(w io.Writer, db *configdb.Store, home, name string, required bool) error {
	blob, has, err := SerializeCollection(home, name)
	if err != nil {
		return errs.Wrapf(errs.KindConfig, err, "reading %s files", name)
	}
	if !has {
		if required {
			return errs.Newf(errs.KindConfig, "no %s files found in %s", name, CollectionDir(home, name))
		}
		fmt.Fprintf(w, "notice: no %s files, skipping\n", name)
		return nil
	}
	if err := db.Import(context.Background(), name, blob, "collection"); err != nil {
		return errs.Wrapf(errs.KindStore, err, "importing %s", name)
	}
	fmt.Fprintf(w, "imported %s (%d bytes)\n", name, len(blob))
	return nil
}
