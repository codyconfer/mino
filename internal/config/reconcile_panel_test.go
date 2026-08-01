package config

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/ansi"
	"github.com/codyconfer/sisyphus"
	"github.com/codyconfer/sisyphus/configdb"
	"github.com/codyconfer/viewkit/theme"
	"github.com/codyconfer/viewkit/ui"

	"github.com/codyconfer/mino/internal/render/glyph"
)

func collectionBlob(t *testing.T, files map[string]string) []byte {
	t.Helper()
	b, err := json.Marshal(files)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

func TestReconcileChoiceFor(t *testing.T) {
	tests := map[string]reconcileChoice{
		"":         defaultReconcileChoice,
		"  ":       defaultReconcileChoice,
		"a":        choiceApply,
		"APPLY":    choiceApply,
		"import":   choiceApply,
		"s":        choiceSession,
		"session":  choiceSession,
		"i":        choiceIgnore,
		"ignore":   choiceIgnore,
		"d":        choiceDiscard,
		"discard":  choiceDiscard,
		"e":        choiceEdit,
		"edit":     choiceEdit,
		"editor":   choiceEdit,
		"open":     choiceEdit,
		"p":        choiceUnknown,
		"preview":  choiceUnknown,
		"whatever": choiceUnknown,
	}
	for in, want := range tests {
		if got := reconcileChoiceFor(in); got != want {
			t.Errorf("reconcileChoiceFor(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestChangedSummary(t *testing.T) {
	rec := sisyphus.Reconciliation{
		Name:       DirectivesDirective,
		FileFormat: "collection",
		FileContent: collectionBlob(t, map[string]string{
			"kept.yaml":           "same",
			"queries/edited.yaml": "new body",
			"team/gh/added.yaml":  "brand new",
		}),
		DB: configdb.Snapshot{
			Hash:   "0123456789abcdef",
			Format: "collection",
			Content: string(collectionBlob(t, map[string]string{
				"kept.yaml":           "same",
				"queries/edited.yaml": "old body",
				"team/removed.yaml":   "gone",
			})),
		},
	}
	got := changedSummary(theme.Default(), rec, 200)
	for _, want := range []string{"+team/gh/added", "queries/edited", "-team/removed"} {
		if !strings.Contains(got, want) {
			t.Errorf("changedSummary = %q, missing %q", got, want)
		}
	}
	if strings.Contains(got, "kept") {
		t.Errorf("changedSummary = %q, should not list unchanged files", got)
	}
	th := theme.Default()
	warn := lipgloss.NewStyle().Foreground(th.NotifWarning.GetForeground())
	if !strings.Contains(got, th.Can.Render("+team/gh/added")) {
		t.Errorf("added file should be green (Can):\n%s", got)
	}
	if !strings.Contains(got, warn.Render("queries/edited")) {
		t.Errorf("changed file should be yellow (Warning):\n%s", got)
	}
	if !strings.Contains(got, th.Cant.Render("-team/removed")) {
		t.Errorf("deleted file should be red (Cant):\n%s", got)
	}
}

func TestChangedSummaryTruncates(t *testing.T) {
	staged := map[string]string{}
	for _, n := range []string{"aaaaaaaa", "bbbbbbbb", "cccccccc", "dddddddd", "eeeeeeee"} {
		staged[n+".yaml"] = "x"
	}
	rec := sisyphus.Reconciliation{
		Name:        DirectivesDirective,
		FileFormat:  "collection",
		FileContent: collectionBlob(t, staged),
		DB:          configdb.Snapshot{Hash: "0123456789abcdef", Format: "collection", Content: string(collectionBlob(t, map[string]string{}))},
	}
	got := changedSummary(theme.Default(), rec, 24)
	if !strings.Contains(got, "more") {
		t.Errorf("changedSummary = %q, want a truncation marker", got)
	}
}

func TestRenderReconcilePanelMentionsEveryChoice(t *testing.T) {
	rec := sisyphus.Reconciliation{
		Name:        DirectivesDirective,
		FileFormat:  "collection",
		FileContent: collectionBlob(t, map[string]string{"queries/a.yaml": "one"}),
		DB: configdb.Snapshot{
			Hash:    "abcdef0123456789",
			Format:  "collection",
			Content: string(collectionBlob(t, map[string]string{"queries/a.yaml": "two"})),
			At:      time.Date(2026, 7, 24, 13, 52, 0, 0, time.UTC),
		},
	}
	out := renderReconcilePanel(ui.Default(), os.Stderr, rec)
	for _, want := range []string{"new config changes staged", "apply changes", "use this session", "ignore staged", "discard changes", "open in editor", "one choice applies to all"} {
		if !strings.Contains(out, want) {
			t.Errorf("panel missing %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "preview") || strings.Contains(out, "print") {
		t.Errorf("panel still mentions preview/print:\n%s", out)
	}
	plain := ansi.Strip(out)
	if title := glyph.Lead(glyph.Warn()) + "new config changes staged"; !strings.Contains(plain, title) {
		t.Errorf("title line missing spaced glyph+text %q:\n%s", title, plain)
	}
}

func TestRenderReconcileBatchPanelListsAllDirectives(t *testing.T) {
	recs := []sisyphus.Reconciliation{
		{
			Name:        ConfigDirective,
			FileFormat:  "yaml",
			FileContent: []byte("output: json\n"),
			DB:          configdb.Snapshot{Hash: "aaaaaaaaaaaaaaaa", Format: "yaml", Content: "output: text\n"},
		},
		{
			Name:        DirectivesDirective,
			FileFormat:  "collection",
			FileContent: collectionBlob(t, map[string]string{"queries/a.yaml": "one", "team/b.yaml": "x"}),
			DB: configdb.Snapshot{
				Hash:    "bbbbbbbbbbbbbbbb",
				Format:  "collection",
				Content: string(collectionBlob(t, map[string]string{"queries/a.yaml": "two", "team/b.yaml": "y"})),
			},
		},
	}
	out := renderReconcileBatchPanel(ui.Default(), os.Stderr, recs)
	for _, want := range []string{"config, directives", "queries/a", "team/b", "write all staged files"} {
		if !strings.Contains(out, want) {
			t.Errorf("batch panel missing %q:\n%s", want, out)
		}
	}
	for _, gone := range []string{DirQueries + ",", DirFlights + ",", KindRoles + ","} {
		if strings.Contains(out, gone) {
			t.Errorf("batch panel still lists a per-collection row %q:\n%s", gone, out)
		}
	}
}

func TestParseReconcilePolicy(t *testing.T) {
	tests := map[string]ReconcilePolicy{
		"":        ReconcilePrompt,
		"prompt":  ReconcilePrompt,
		"apply":   ReconcileApply,
		"session": ReconcileSession,
		"ignore":  ReconcileIgnore,
		"DuckDB":  ReconcileIgnore,
	}
	for in, want := range tests {
		got, err := ParseReconcilePolicy(in)
		if err != nil {
			t.Errorf("ParseReconcilePolicy(%q): %v", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseReconcilePolicy(%q) = %v, want %v", in, got, want)
		}
	}
	if _, err := ParseReconcilePolicy("nope"); err == nil {
		t.Error("ParseReconcilePolicy(\"nope\") = nil error, want usage error")
	}
}
