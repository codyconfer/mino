package ui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/bubbles/spinner"
)

func newTestModel(labels ...string) model {
	m := model{
		panels: make([]panel, len(labels)),
		spin:   spinner.New(),
		left:   len(labels),
	}
	for i, l := range labels {
		m.panels[i].label = l
	}
	return m
}

func TestPanelsReplaceOnDone(t *testing.T) {
	m := newTestModel("alpha", "beta")

	v := m.View()
	if !strings.Contains(v, "alpha") || !strings.Contains(v, "beta") || !strings.Contains(v, "loading") {
		t.Fatalf("initial view should show both labels loading:\n%s", v)
	}

	next, _ := m.Update(doneMsg{idx: 0, content: "ALPHA-CONTENT"})
	m = next.(model)
	if m.left != 1 {
		t.Fatalf("left = %d, want 1", m.left)
	}
	v = m.View()
	if !strings.Contains(v, "ALPHA-CONTENT") {
		t.Errorf("view should show completed content:\n%s", v)
	}
	if !strings.Contains(v, "beta") || !strings.Contains(v, "loading") {
		t.Errorf("second panel should still be loading:\n%s", v)
	}

	next, cmd := m.Update(doneMsg{idx: 1, content: "BETA-CONTENT"})
	m = next.(model)
	if m.left != 0 {
		t.Fatalf("left = %d, want 0", m.left)
	}
	if cmd == nil {
		t.Fatal("expected a quit command once all panels are done")
	}
	v = m.View()
	if !strings.Contains(v, "ALPHA-CONTENT") || !strings.Contains(v, "BETA-CONTENT") {
		t.Errorf("final view should show all content:\n%s", v)
	}
	if strings.Contains(v, "loading") {
		t.Errorf("final view should have no loading panels:\n%s", v)
	}
}

func TestDuplicateDoneIsIgnored(t *testing.T) {
	m := newTestModel("only")
	next, _ := m.Update(doneMsg{idx: 0, content: "X"})
	m = next.(model)

	next, cmd := m.Update(doneMsg{idx: 0, content: "X"})
	m = next.(model)
	if m.left != 0 {
		t.Fatalf("left = %d, want 0 (no underflow)", m.left)
	}
	_ = cmd
}
