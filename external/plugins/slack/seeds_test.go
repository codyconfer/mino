package slack

import (
	"strings"
	"testing"
)

func TestSeedFiles(t *testing.T) {
	seeds := seedFiles()
	if len(seeds) == 0 {
		t.Fatal("no seeds embedded; `mino plugins install external.slack` would write nothing")
	}
	names := map[string]bool{}
	for _, s := range seeds {
		if !strings.HasPrefix(s.RelPath, "queries/") {
			t.Errorf("RelPath = %q, want it under queries/", s.RelPath)
		}
		if strings.Contains(s.RelPath, "seeds/") {
			t.Errorf("RelPath = %q, still carries the embed prefix", s.RelPath)
		}
		body := string(s.Content)
		if !strings.Contains(body, "signal: slack") {
			t.Errorf("%s does not declare `signal: slack`", s.RelPath)
		}
		if !strings.Contains(body, "name:") {
			t.Errorf("%s has no name", s.RelPath)
		}
		names[s.RelPath] = true
	}
	for _, want := range []string{
		"queries/slack-standup.yaml",
		"queries/slack-mentions.yaml",
		"queries/slack-catchup.yaml",
	} {
		if !names[want] {
			t.Errorf("missing seed %s", want)
		}
	}
}
