package render

import (
	"testing"

	"github.com/codyconfer/mino/internal/signals"
)

func TestItemInProgressHonorsThePluginMetaKey(t *testing.T) {
	cases := []struct {
		name string
		item signals.Item
		want bool
	}{
		{"plugin flag on a non-workflow kind", signals.Item{Kind: "application", Meta: map[string]string{"in_progress": "true"}}, true},
		{"plugin flag wins over a set conclusion", signals.Item{Kind: "workflow", Meta: map[string]string{"in_progress": "true", "conclusion": "failure"}}, true},
		{"flag absent leaves a plain item settled", signals.Item{Kind: "application", Meta: map[string]string{"state": "synced"}}, false},
		{"flag must be the literal true", signals.Item{Kind: "application", Meta: map[string]string{"in_progress": "yes"}}, false},
		{"workflow status path still works", signals.Item{Kind: "workflow", Meta: map[string]string{"status": "in_progress"}}, true},
		{"workflow with a conclusion is settled", signals.Item{Kind: "workflow", Meta: map[string]string{"status": "in_progress", "conclusion": "failure"}}, false},
		{"no meta at all", signals.Item{Kind: "workflow"}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ItemInProgress(c.item); got != c.want {
				t.Errorf("ItemInProgress = %v, want %v; this gate drives both the row spinner and the "+
					"detail re-poll, so getting it wrong either freezes a live deploy or polls forever", got, c.want)
			}
		})
	}
}

func TestSectionsHaveInProgressSeesPluginItems(t *testing.T) {
	sections := []signals.Section{{
		Signal: "argocd",
		Items: []signals.Item{
			{Kind: "application", Meta: map[string]string{"state": "synced"}},
			{Kind: "application", Meta: map[string]string{"in_progress": "true"}},
		},
	}}
	if !SectionsHaveInProgress(sections) {
		t.Error("SectionsHaveInProgress = false; the deck would stop animating while an app is still syncing")
	}
}

func TestWorkflowSectionOptsInViaStateRows(t *testing.T) {
	cases := []struct {
		name    string
		section signals.DetailSection
		want    bool
	}{
		{"state_rows opt-in", signals.DetailSection{Title: "resources", Meta: map[string]string{"state_rows": "true"}}, true},
		{"run_id still works", signals.DetailSection{Title: "jobs", Meta: map[string]string{"run_id": "7"}}, true},
		{"title prefix still works", signals.DetailSection{Title: "workflow steps"}, true},
		{"plain section stays plain", signals.DetailSection{Title: "resources"}, false},
		{"state_rows must be the literal true", signals.DetailSection{Title: "resources", Meta: map[string]string{"state_rows": "1"}}, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := workflowSection(c.section); got != c.want {
				t.Errorf("workflowSection = %v, want %v; without the opt-in a plugin has to fake run_id "+
					"to get per-row state cues", got, c.want)
			}
		})
	}
}
