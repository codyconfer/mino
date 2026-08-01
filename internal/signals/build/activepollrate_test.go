package build

import (
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/config"
)

func TestActiveBuildersRefuseAHotPollRate(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "gho_test")
	cfg := config.Defaults()
	params := map[string]string{"interval": "100ms"}

	src, err := buildActiveGithub(params, cfg, nil, nil)
	if err == nil {
		t.Fatalf("github active builder accepted interval=100ms and returned %v; the floor has to hold at every "+
			"builder, because a query param reaches them without passing the CLI flag", src)
	}
	if !strings.Contains(err.Error(), "poll interval") {
		t.Fatalf("github active builder error = %q; want it to name the poll interval so the user can find the "+
			"offending query param", err)
	}
}
