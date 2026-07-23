package cmd

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"golang.org/x/term"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/log"
	"github.com/codyconfer/sisyphus"
	sconfig "github.com/codyconfer/sisyphus/config"
	"github.com/codyconfer/sisyphus/redact"
)

func loadConfigAndDirectives() (*config.Config, *config.Directives, *sisyphus.Manager, error) {
	home, err := config.Home(flagHome)
	if err != nil {
		return nil, nil, nil, err
	}
	gs := config.LoadGlobalSettings()
	res := &reconcileResolver{home: home, preferDB: gs.PreferDB, interactive: term.IsTerminal(int(os.Stdin.Fd()))}

	var mgr *sisyphus.Manager
	if m, err := sisyphus.Open(home, sisyphus.Options{Mode: sisyphus.ModeBoth}); err != nil {
		verbosef("store DB unavailable: %v; reading from files", err)
	} else {
		mgr = m
	}

	cfg, err := reconcileConfig(home, mgr, res)
	if err != nil {
		return nil, nil, nil, err
	}

	var directives *config.Directives
	if mgr == nil {
		directives, err = config.LoadDirectivesFromFiles(home)
	} else {
		var q, f, fl, r []byte
		if q, err = reconcileCollection(mgr, res, home, config.DirQueries); err == nil {
			if f, err = reconcileCollection(mgr, res, home, config.DirFilters); err == nil {
				if fl, err = reconcileCollection(mgr, res, home, config.DirFlights); err == nil {
					if r, err = reconcileCollection(mgr, res, home, config.DirRoles); err == nil {
						directives, err = config.NewDirectives(q, f, fl, r)
					}
				}
			}
		}
	}
	if err != nil {
		return nil, nil, nil, err
	}
	return cfg, directives, mgr, nil
}

func reconcileConfig(home string, mgr *sisyphus.Manager, res sisyphus.Resolver) (*config.Config, error) {
	if flagConfigFile != "" {
		raw, format, err := sconfig.ReadFileAt(flagConfigFile)
		if err != nil {
			return nil, err
		}
		verbosef("using session config file %s (not persisted)", flagConfigFile)
		return config.ParseConfig(home, raw, format)
	}
	if mgr == nil {
		return config.Load(flagHome)
	}
	_, raw, format, err := config.ReadConfigFile(flagHome)
	if err != nil {
		return nil, err
	}
	eff, effFormat, err := mgr.Reconcile("config", raw, format, len(raw) > 0, res)
	if err != nil {
		return nil, err
	}
	return config.ParseConfig(home, eff, effFormat)
}

func reconcileCollection(mgr *sisyphus.Manager, res sisyphus.Resolver, home, name string) ([]byte, error) {
	blob, has, err := sconfig.SerializeDir(filepath.Join(home, name))
	if err != nil {
		return nil, err
	}
	eff, _, err := mgr.Reconcile(name, blob, "collection", has, res)
	return eff, err
}

type reconcileResolver struct {
	home        string
	preferDB    bool
	interactive bool
}

func (r *reconcileResolver) Resolve(rec sisyphus.Reconciliation) (sisyphus.Action, error) {
	if !rec.HasDB {
		return sisyphus.ActionImport, nil
	}
	if !r.interactive {
		if r.preferDB && rec.HasDB {
			warnf("%s differs from DuckDB; using DuckDB (prefer_duckdb)", rec.Name)
			return sisyphus.ActionUseDB, nil
		}
		warnf("%s differs from DuckDB; using the file for this session (not persisted)", rec.Name)
		return sisyphus.ActionUseFile, nil
	}

	out := os.Stderr
	in := bufio.NewReader(os.Stdin)
	for {
		fmt.Fprintf(out, "\n%q differs from the stored (DuckDB) version.\n", rec.Name)
		fmt.Fprintln(out, "  [i] import file (overwrite DuckDB)   [s] use file this session")
		fmt.Fprintln(out, "  [d] use DuckDB (ignore file)         [p] print file   [x] delete file")
		fmt.Fprint(out, "choose [i/s/d/p/x]: ")
		line, _ := in.ReadString('\n')
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "i", "import":
			return sisyphus.ActionImport, nil
		case "s", "session":
			return sisyphus.ActionUseFile, nil
		case "d", "db", "duckdb", "":
			return sisyphus.ActionUseDB, nil
		case "p", "print":
			printReconcileFile(out, rec)
		case "x", "delete":
			if err := deleteDirectiveFiles(r.home, rec.Name); err != nil {
				fmt.Fprintf(out, "delete failed: %v\n", err)
				continue
			}
			fmt.Fprintf(out, "deleted %s file(s); using DuckDB.\n", rec.Name)
			return sisyphus.ActionUseDB, nil
		default:
			fmt.Fprintln(out, "unrecognized choice")
		}
	}
}

func printReconcileFile(out *os.File, rec sisyphus.Reconciliation) {
	if rec.FileFormat == "collection" {
		var files map[string]string
		if json.Unmarshal(rec.FileContent, &files) == nil {
			names := make([]string, 0, len(files))
			for n := range files {
				names = append(names, n)
			}
			sort.Strings(names)
			for _, n := range names {
				fmt.Fprintf(out, "\n── %s ──\n%s\n", n, files[n])
			}
			return
		}
	}
	if rec.Name == "config" {
		fmt.Fprintf(out, "\n%s\n", redact.Config(rec.FileContent, rec.FileFormat))
		return
	}
	fmt.Fprintf(out, "\n%s\n", rec.FileContent)
}

func deleteDirectiveFiles(home, name string) error {
	if name == "config" {
		_, err := sconfig.RemoveFiles(home, "config", nil)
		return err
	}
	_, err := sconfig.ClearDir(filepath.Join(home, name), nil)
	return err
}

func warnf(format string, args ...any) {
	log.Warnf(format, args...)
}
