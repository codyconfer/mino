package config

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"time"

	"github.com/codyconfer/sisyphus"
	sconfig "github.com/codyconfer/sisyphus/config"
	"github.com/codyconfer/sisyphus/redact"

	"github.com/codyconfer/mino/internal/errs"
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

func PrintCurrentConfig(w io.Writer, db *sisyphus.ConfigStore) error {
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
	fmt.Fprintln(w, redact.Document([]byte(cur.Content), string(cur.Format)))
	return nil
}

func PrintConfigHistory(w io.Writer, db *sisyphus.ConfigStore) error {
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

func Export(w io.Writer, db *sisyphus.ConfigStore, out, liveHome, directive string, includeSecrets bool) error {
	directive, err := ResolveDirectiveArg(directive)
	if err != nil {
		return err
	}
	if out == "" {
		out = liveHome
	}
	switch directive {
	case "all":
		if err := exportConfig(w, db, out, liveHome, false, includeSecrets); err != nil {
			return err
		}
		return exportDirectives(w, db, out, false)
	case ConfigDirective:
		return exportConfig(w, db, out, liveHome, true, includeSecrets)
	default:
		return exportDirectives(w, db, out, true)
	}
}

func SamePath(a, b string) bool {
	return resolvePath(a) == resolvePath(b)
}

func resolvePath(p string) string {
	if abs, err := filepath.Abs(p); err == nil {
		p = abs
	} else {
		p = filepath.Clean(p)
	}
	if real, err := filepath.EvalSymlinks(p); err == nil {
		p = real
	}
	if runtime.GOOS == "windows" {
		return strings.ToLower(p)
	}
	return p
}

func exportConfig(w io.Writer, db *sisyphus.ConfigStore, out, liveHome string, single, includeSecrets bool) error {
	v, ok, err := db.Current(context.Background(), ConfigDirective)
	if err != nil {
		return errs.Wrap(errs.KindStore, err, "reading config from store")
	}
	if !ok {
		if single {
			return errs.New(errs.KindStore, "no current version for config in the store").
				WithHint("run `mino import config` first")
		}
		fmt.Fprintln(w, "notice: no config version in store, skipping")
		return nil
	}
	content := v.Content
	if includeSecrets {
		fmt.Fprintln(w, "warning: exported config contains secret values in cleartext")
	} else {
		if SamePath(out, liveHome) {
			return errs.Newf(errs.KindUsage,
				"refusing to overwrite the live config in %s with a secret-masked copy", out).
				WithHint("masked exports are for sharing only: pass --out <other-dir> to write the masked copy elsewhere, " +
					"or --include-secrets to materialize the real config back into the mino home")
		}
		content = redact.Document([]byte(v.Content), string(v.Format))
		fmt.Fprintf(w, "warning: secret values are replaced with %q and comments and key order are lost; this copy is for sharing, not a working config\n", redact.Mask)
	}
	path, err := sconfig.WriteConfigFile(out, []byte(content), v.Format)
	if err != nil {
		return err
	}
	fmt.Fprintf(w, "wrote %s\n", path)
	return nil
}

func exportDirectives(w io.Writer, db *sisyphus.ConfigStore, out string, single bool) error {
	v, ok, err := db.Current(context.Background(), DirectivesDirective)
	if err != nil {
		return errs.Wrap(errs.KindStore, err, "reading directives from store")
	}
	if !ok {
		if single {
			return errs.New(errs.KindStore, "no current version for directives in the store").
				WithHint("run `mino import directives` first")
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

func ExportAllToFiles(db *sisyphus.ConfigStore, home string) ([]string, error) {
	var plan *directivePlan
	cur, hasDirectives, err := db.Current(context.Background(), DirectivesDirective)
	if err != nil {
		return nil, err
	}
	if hasDirectives {
		if plan, err = planDirectives(home, []byte(cur.Content)); err != nil {
			return nil, err
		}
	}
	var written []string
	if cfg, ok, cerr := db.Current(context.Background(), ConfigDirective); cerr != nil {
		return nil, cerr
	} else if ok {
		path, werr := sconfig.WriteConfigFile(home, []byte(cfg.Content), cfg.Format)
		if werr != nil {
			return nil, werr
		}
		written = append(written, path)
	}
	if !hasDirectives {
		return written, nil
	}
	names, err := plan.apply()
	if err != nil {
		return nil, err
	}
	for _, rel := range names {
		written = append(written, filepath.Join(home, filepath.FromSlash(rel)))
	}
	return written, nil
}

func Import(w io.Writer, db *sisyphus.ConfigStore, home, directive string) error {
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

func importConfig(w io.Writer, db *sisyphus.ConfigStore, home string, required bool) error {
	_, raw, format, err := ReadConfigFile(home)
	if err != nil {
		return errs.Wrap(errs.KindConfig, err, "reading config file")
	}
	if len(raw) == 0 {
		if required {
			return errs.Newf(errs.KindConfig, "no config file found in %s", home).
				WithHint("expected config.yaml, config.yml, or config.json; run `mino install` to create one")
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

func importDirectives(w io.Writer, db *sisyphus.ConfigStore, home string, required bool) error {
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
