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
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := ClassifyItem(c.item); got != c.want {
				t.Errorf("ClassifyItem = %v, want %v", got, c.want)
			}
		})
	}
}
