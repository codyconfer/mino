package signals

import (
	"testing"

	"github.com/codyconfer/viewkit/glyph"
)

func TestClassifyItemUsesState(t *testing.T) {
	cases := []struct {
		name string
		item Item
		want glyph.Severity
	}{
		{"open pr", Item{Kind: "pr", Meta: map[string]string{"state": "open"}}, glyph.SeverityNeutral},
		{"merged pr", Item{Kind: "pr", Meta: map[string]string{"state": "merged"}}, glyph.SeverityPositive},
		{"closed pr", Item{Kind: "pr", Meta: map[string]string{"state": "closed"}}, glyph.SeverityNegative},
		{"open issue", Item{Kind: "issue", Meta: map[string]string{"state": "open"}}, glyph.SeverityNeutral},
		{"closed issue", Item{Kind: "issue", Meta: map[string]string{"state": "closed"}}, glyph.SeverityPositive},
		{"uppercase state", Item{Kind: "pr", Meta: map[string]string{"state": "MERGED"}}, glyph.SeverityPositive},
		{"draft open pr", Item{Kind: "pr", Meta: map[string]string{"state": "open", "draft": "true"}}, glyph.SeverityNeutral},
		{"workflow success", Item{Kind: "workflow", Meta: map[string]string{"status": "completed", "conclusion": "success"}}, glyph.SeverityPositive},
		{"workflow failure", Item{Kind: "workflow", Meta: map[string]string{"status": "completed", "conclusion": "failure"}}, glyph.SeverityNegative},
		{"workflow running", Item{Kind: "workflow", Meta: map[string]string{"status": "in_progress"}}, glyph.SeverityWarning},
		{"argocd synced", Item{Kind: "application", Meta: map[string]string{"state": "synced"}}, glyph.SeverityPositive},
		{"argocd degraded", Item{Kind: "application", Meta: map[string]string{"state": "degraded"}}, glyph.SeverityNegative},
		{"argocd missing", Item{Kind: "application", Meta: map[string]string{"state": "missing"}}, glyph.SeverityNegative},
		{"argocd failed", Item{Kind: "application", Meta: map[string]string{"state": "failed"}}, glyph.SeverityNegative},
		{"argocd progressing", Item{Kind: "application", Meta: map[string]string{"state": "progressing"}}, glyph.SeverityWarning},
		{"argocd in progress", Item{Kind: "application", Meta: map[string]string{"state": "in progress"}}, glyph.SeverityWarning},
		{"argocd out of sync", Item{Kind: "application", Meta: map[string]string{"state": "out of sync"}}, glyph.SeverityWarning},
		{"argocd suspended", Item{Kind: "application", Meta: map[string]string{"state": "suspended"}}, glyph.SeverityNeutral},
		{"argocd unknown", Item{Kind: "application", Meta: map[string]string{"state": "unknown"}}, glyph.SeverityNeutral},
		{"opened mr", Item{Kind: "mr", Meta: map[string]string{"state": "opened"}}, glyph.SeverityNeutral},
		{"merged mr", Item{Kind: "mr", Meta: map[string]string{"state": "merged"}}, glyph.SeverityPositive},
		{"closed mr", Item{Kind: "mr", Meta: map[string]string{"state": "closed"}}, glyph.SeverityNegative},
		{"opened issue", Item{Kind: "issue", Meta: map[string]string{"state": "opened"}}, glyph.SeverityNeutral},
		{"failed pipeline", Item{Kind: "pipeline", Meta: map[string]string{"status": "failed"}}, glyph.SeverityNegative},
		{"running pipeline", Item{Kind: "pipeline", Meta: map[string]string{"status": "running"}}, glyph.SeverityWarning},
		{"manual pipeline", Item{Kind: "pipeline", Meta: map[string]string{"status": "manual"}}, glyph.SeverityWarning},
		{"successful pipeline", Item{Kind: "pipeline", Meta: map[string]string{"status": "success"}}, glyph.SeverityPositive},
		{"canceled pipeline", Item{Kind: "pipeline", Meta: map[string]string{"status": "canceled"}}, glyph.SeverityNeutral},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ClassifyItem(c.item); got != c.want {
				t.Errorf("ClassifyItem = %v, want %v", got, c.want)
			}
		})
	}
}

func TestClassifyItemFallsBackToKind(t *testing.T) {
	cases := []struct {
		name string
		item Item
		want glyph.Severity
	}{
		{"no meta", Item{Kind: "pr"}, glyph.SeverityNeutral},
		{"unknown state", Item{Kind: "pr", Meta: map[string]string{"state": "weird"}}, glyph.SeverityNeutral},
		{"mention kind", Item{Kind: "mention"}, glyph.SeverityWarning},
		{"failed kind", Item{Kind: "failed"}, glyph.SeverityNegative},
		{"approved kind", Item{Kind: "approved"}, glyph.SeverityPositive},
		{"unrecognized state keeps the kind", Item{Kind: "failed", Meta: map[string]string{"state": "weird"}}, glyph.SeverityNegative},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ClassifyItem(c.item); got != c.want {
				t.Errorf("ClassifyItem = %v, want %v", got, c.want)
			}
		})
	}
}

func TestClassifyStateCoversDeployVocabulary(t *testing.T) {
	cases := []struct {
		state string
		want  glyph.Severity
	}{
		{"SUCCESS", glyph.SeverityPositive},
		{"Succeeded", glyph.SeverityPositive},
		{"Synced", glyph.SeverityPositive},
		{"Healthy", glyph.SeverityPositive},
		{"Failure", glyph.SeverityNegative},
		{"Failed", glyph.SeverityNegative},
		{"Error", glyph.SeverityNegative},
		{"Degraded", glyph.SeverityNegative},
		{"Missing", glyph.SeverityNegative},
		{"in progress", glyph.SeverityWarning},
		{"Progressing", glyph.SeverityWarning},
		{"Running", glyph.SeverityWarning},
		{"Terminating", glyph.SeverityWarning},
		{"out of sync", glyph.SeverityWarning},
		{"OUT_OF_SYNC", glyph.SeverityWarning},
		{"Suspended", glyph.SeverityNeutral},
		{"Unknown", glyph.SeverityNeutral},
		{"", glyph.SeverityNeutral},
	}
	for _, c := range cases {
		t.Run(c.state, func(t *testing.T) {
			if got := ClassifyState(c.state); got != c.want {
				t.Errorf("ClassifyState(%q) = %v, want %v; a deploy panel that cannot colour its own "+
					"states tells an on-call engineer nothing at a glance", c.state, got, c.want)
			}
		})
	}
}
