package config

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codyconfer/sisyphus"
	"github.com/codyconfer/sisyphus/configdb"
)

func TestReconcileResolver(t *testing.T) {
	withDB := configdb.Snapshot{Hash: "0123456789abcdef"}
	tests := []struct {
		name     string
		resolver Resolver
		rec      sisyphus.Reconciliation
		want     sisyphus.Action
	}{
		{
			name:     "no db version imports even when interactive",
			resolver: Resolver{interactive: true},
			rec:      sisyphus.Reconciliation{Name: "config"},
			want:     sisyphus.ActionImport,
		},
		{
			name:     "no db version imports non-interactive",
			resolver: Resolver{interactive: false},
			rec:      sisyphus.Reconciliation{Name: "config"},
			want:     sisyphus.ActionImport,
		},
		{
			name:     "db conflict non-interactive prefers db",
			resolver: Resolver{interactive: false, preferDB: true},
			rec:      sisyphus.Reconciliation{Name: "config", DB: withDB},
			want:     sisyphus.ActionUseDB,
		},
		{
			name:     "db conflict non-interactive uses file by default",
			resolver: Resolver{interactive: false},
			rec:      sisyphus.Reconciliation{Name: "config", DB: withDB},
			want:     sisyphus.ActionUseFile,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := tt.resolver.Resolve(tt.rec)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if got != tt.want {
				t.Fatalf("Resolve = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestLoadConfigAndDirectivesBatchFlow(t *testing.T) {
	home := t.TempDir()
	write := func(rel, body string) {
		t.Helper()
		path := filepath.Join(home, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	load := func(policy ReconcilePolicy) (*Config, *Directives) {
		t.Helper()
		cfg, dirs, mgr, err := LoadConfigAndDirectives(home, "", policy, false, strings.NewReader(""), io.Discard, nil)
		if err != nil {
			t.Fatalf("LoadConfigAndDirectives: %v", err)
		}
		if mgr == nil {
			t.Skip("config store unavailable")
		}
		mgr.Close()
		return cfg, dirs
	}

	write("config.yaml", "output: json\n")
	write(DirQueries+"/batch-flow.yaml", "type: query\nsignal: github\n")

	// First run: nothing stored yet, so both file-only items import silently.
	cfg, dirs := load(ReconcilePrompt)
	if cfg.Output != "json" {
		t.Fatalf("Output = %q, want json", cfg.Output)
	}
	if _, ok := dirs.Queries["batch-flow"]; !ok {
		t.Fatalf("directives missing batch-flow query: %v", dirs.QueryNames())
	}
	mgr, err := OpenStore(context.Background(), home)
	if err != nil {
		t.Skipf("config store unavailable: %v", err)
	}
	for _, name := range []string{ConfigDirective, DirectivesDirective} {
		if _, ok, err := mgr.Current(context.Background(), name); !ok || err != nil {
			t.Errorf("%s not imported on first load: ok=%v err=%v", name, ok, err)
		}
	}
	mgr.Close()

	// Drift the config file; ReconcileIgnore keeps the stored version.
	write("config.yaml", "output: terminal\n")
	cfg, _ = load(ReconcileIgnore)
	if cfg.Output != "json" {
		t.Errorf("ignore: Output = %q, want stored json", cfg.Output)
	}

	// ReconcileSession uses the file without writing the store.
	cfg, _ = load(ReconcileSession)
	if cfg.Output != "terminal" {
		t.Errorf("session: Output = %q, want terminal from file", cfg.Output)
	}
	cfg, _ = load(ReconcileIgnore)
	if cfg.Output != "json" {
		t.Errorf("session must not write the store: stored Output = %q, want json", cfg.Output)
	}

	// ReconcileApply writes the drifted file to the store.
	cfg, _ = load(ReconcileApply)
	if cfg.Output != "terminal" {
		t.Errorf("apply: Output = %q, want terminal", cfg.Output)
	}
	cfg, _ = load(ReconcileIgnore)
	if cfg.Output != "terminal" {
		t.Errorf("apply must write the store: stored Output = %q, want terminal", cfg.Output)
	}

	// Remove the config file; the stored version silently wins.
	if err := os.Remove(filepath.Join(home, "config.yaml")); err != nil {
		t.Fatal(err)
	}
	cfg, _ = load(ReconcilePrompt)
	if cfg.Output != "terminal" {
		t.Errorf("db-only: Output = %q, want stored terminal", cfg.Output)
	}
}
