package slackauth

import (
	"strings"
	"testing"
)

func TestDefaultUserScopesKeepsBaseScopes(t *testing.T) {
	have := map[string]bool{}
	for _, s := range strings.Split(DefaultUserScopes, ",") {
		have[strings.TrimSpace(s)] = true
	}
	for _, s := range strings.Split(BaseUserScopes, ",") {
		s = strings.TrimSpace(s)
		if !have[s] {
			t.Errorf("DefaultUserScopes dropped %q; shrinking the grant breaks channel reads for anyone already logged in", s)
		}
	}
}

func TestDefaultUserScopesCoversNewSurfaces(t *testing.T) {
	for _, s := range []string{"im:history", "im:read", "mpim:history", "mpim:read", "search:read", "users:read"} {
		if !strings.Contains(DefaultUserScopes, s) {
			t.Errorf("DefaultUserScopes is missing %q, so a fresh login cannot use the surface that needs it", s)
		}
	}
}
