package build

import (
	"strings"
	"testing"

	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/signals"
)

func TestActiveBuildersRefuseAHotPollRate(t *testing.T) {
	cfg := config.Defaults()
	params := map[string]string{"interval": "100ms"}
	cases := []struct {
		name  string
		build func() (signals.ActiveSignal, error)
	}{
		{"calendar", func() (signals.ActiveSignal, error) { return buildActiveCalendar(params, cfg, nil, nil) }},
		{"tasks", func() (signals.ActiveSignal, error) { return buildActiveTasks(params, cfg, nil, nil) }},
	}
	for _, c := range cases {
		src, err := c.build()
		if err == nil {
			t.Errorf("%s active builder accepted interval=100ms and returned %v; the floor has to hold at every "+
				"builder, because a query param reaches them without passing the CLI flag", c.name, src)
			continue
		}
		if !strings.Contains(err.Error(), "poll interval") {
			t.Errorf("%s active builder error = %q; want it to name the poll interval so the user can find the "+
				"offending query param", c.name, err)
		}
	}
}
