package flight

import "testing"

func TestQueryDisplayPrefersTitle(t *testing.T) {
	if got := (Query{Label: "my-open-prs", Title: "Open pull requests"}).Display(); got != "Open pull requests" {
		t.Errorf("Display = %q, want the title", got)
	}
	if got := (Query{Label: "my-open-prs"}).Display(); got != "my-open-prs" {
		t.Errorf("Display without a title = %q, want the label", got)
	}
}
