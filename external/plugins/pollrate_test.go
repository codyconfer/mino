package plugins_test

import (
	"strings"
	"testing"

	"github.com/codyconfer/munin/external/plugins/calendar"
	"github.com/codyconfer/munin/external/plugins/tasks"
	"github.com/codyconfer/munin/plugin"
)

type hotContext struct{}

func (hotContext) Params() map[string]string { return map[string]string{"interval": "100ms"} }

func (hotContext) Home() string { return "" }

func (hotContext) Role() string { return "" }

func (hotContext) Settings(string) map[string]any { return nil }

func (hotContext) Credentials() plugin.CredentialStore { return nil }

func TestStreamBuildersRefuseAHotPollRate(t *testing.T) {
	cases := []struct {
		name  string
		build func(plugin.BuildContext) (plugin.Stream, error)
	}{
		{"calendar", calendar.BuildStream},
		{"tasks", tasks.BuildStream},
	}
	for _, c := range cases {
		src, err := c.build(hotContext{})
		if err == nil {
			t.Errorf("%s stream builder accepted interval=100ms and returned %v; the floor has to hold at every "+
				"builder, because a query param reaches them without passing the CLI flag", c.name, src)
			continue
		}
		if !strings.Contains(err.Error(), "poll interval") {
			t.Errorf("%s stream builder error = %q; want it to name the poll interval so the user can find the "+
				"offending query param", c.name, err)
		}
	}
}
