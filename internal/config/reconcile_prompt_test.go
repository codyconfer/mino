package config

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/codyconfer/sisyphus"
	"github.com/codyconfer/sisyphus/configdb"
)

func stagedDirectives(t *testing.T) sisyphus.Reconciliation {
	t.Helper()
	return sisyphus.Reconciliation{
		Name:        DirectivesDirective,
		FileFormat:  "collection",
		FileContent: collectionBlob(t, map[string]string{"queries/a.yaml": "name: a\n"}),
		HasDB:       true,
		DB: configdb.Version{
			Hash:    "0123456789abcdef",
			Format:  "collection",
			Content: string(collectionBlob(t, map[string]string{"queries/a.yaml": "name: b\n"})),
		},
	}
}

func TestReconcilePromptChoices(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  sisyphus.Action
		echo  string
	}{
		{"apply", "a", sisyphus.ActionImport, ""},
		{"session", "s", sisyphus.ActionUseFile, ""},
		{"ignore", "i", sisyphus.ActionUseDB, ""},
		{"enter defaults to session", "\n", sisyphus.ActionUseFile, ""},
		{"return defaults to session", "\r", sisyphus.ActionUseFile, ""},
		{"eof defaults to session", "", sisyphus.ActionUseFile, ""},
		{"unrecognized re-asks", "zzi", sisyphus.ActionUseDB, "unrecognized choice"},
		{"edit re-asks", "ea", sisyphus.ActionImport, "opened"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			home := t.TempDir()
			prev := openConfigEditor
			t.Cleanup(func() { openConfigEditor = prev })
			openConfigEditor = func(path string) error {
				if path != home {
					t.Fatalf("openConfigEditor path = %q, want %q", path, home)
				}
				return nil
			}
			var out bytes.Buffer
			r := &Resolver{home: home, interactive: true, in: strings.NewReader(tt.input), out: &out}
			got, err := r.Resolve(stagedDirectives(t))
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if got != tt.want {
				t.Fatalf("Resolve = %v, want %v", got, tt.want)
			}
			if !strings.Contains(out.String(), "new config changes staged") {
				t.Errorf("prompt did not render the panel:\n%s", out.String())
			}
			if tt.echo != "" && !strings.Contains(out.String(), tt.echo) {
				t.Errorf("output missing %q:\n%s", tt.echo, out.String())
			}
		})
	}
}

func TestReconcilePromptDiscardRequiresConfirmation(t *testing.T) {
	home := t.TempDir()
	dir := filepath.Join(home, DirQueries, "gh")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, "a.yaml")
	if err := os.WriteFile(path, []byte("name: a\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	var out bytes.Buffer
	r := &Resolver{home: home, interactive: true, in: strings.NewReader("dns"), out: &out}
	got, err := r.Resolve(stagedDirectives(t))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != sisyphus.ActionUseFile {
		t.Fatalf("Resolve = %v, want ActionUseFile after declining discard", got)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("declined discard still removed %s: %v", path, err)
	}

	out.Reset()
	r = &Resolver{home: home, interactive: true, in: strings.NewReader("dy"), out: &out}
	got, err = r.Resolve(stagedDirectives(t))
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if got != sisyphus.ActionUseDB {
		t.Fatalf("Resolve = %v, want ActionUseDB after discarding", got)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("discard left %s in place (err=%v)", path, err)
	}
}

func TestReconcilePromptAllListsEveryDirectiveOnce(t *testing.T) {
	recs := []sisyphus.Reconciliation{
		{
			Name:        ConfigDirective,
			FileFormat:  "yaml",
			FileContent: []byte("output: json\n"),
			HasDB:       true,
			DB:          configdb.Version{Hash: "fedcba9876543210", Format: "yaml", Content: "output: text\n"},
		},
		stagedDirectives(t),
	}
	var out bytes.Buffer
	r := &Resolver{interactive: true, in: strings.NewReader("a"), out: &out}
	got, err := r.promptAll(recs)
	if err != nil {
		t.Fatalf("promptAll: %v", err)
	}
	if got != sisyphus.ActionImport {
		t.Fatalf("promptAll = %v, want ActionImport", got)
	}
	text := out.String()
	if strings.Count(text, "new config changes staged") != 1 {
		t.Fatalf("want exactly one staged panel, got:\n%s", text)
	}
	for _, want := range []string{ConfigDirective, DirectivesDirective, "queries/a", "press"} {
		if !strings.Contains(text, want) {
			t.Errorf("batch prompt missing %q:\n%s", want, text)
		}
	}
}

func TestReconcilePolicySkipsPrompt(t *testing.T) {
	tests := []struct {
		policy ReconcilePolicy
		want   sisyphus.Action
	}{
		{ReconcileApply, sisyphus.ActionImport},
		{ReconcileSession, sisyphus.ActionUseFile},
		{ReconcileIgnore, sisyphus.ActionUseDB},
	}
	for _, tt := range tests {
		var out bytes.Buffer
		r := &Resolver{policy: tt.policy, interactive: true, in: strings.NewReader(""), out: &out}
		got, err := r.Resolve(stagedDirectives(t))
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		if got != tt.want {
			t.Errorf("policy %v: Resolve = %v, want %v", tt.policy, got, tt.want)
		}
		if out.Len() != 0 {
			t.Errorf("policy %v prompted anyway:\n%s", tt.policy, out.String())
		}
	}
}
