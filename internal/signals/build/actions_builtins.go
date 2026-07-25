package build

import (
	"context"
	"fmt"

	"github.com/codyconfer/munin/internal/plugin"
)

func init() {
	plugin.RegisterAction("drive", "mkdir", func(_ context.Context, params map[string]string) error {
		if params["name"] == "" {
			return fmt.Errorf("name param required")
		}
		return fmt.Errorf("drive.mkdir: use `munin drive` helpers; action host reserved (name=%s)", params["name"])
	})
	plugin.RegisterAction("tasks", "add", func(_ context.Context, params map[string]string) error {
		if params["title"] == "" {
			return fmt.Errorf("title param required")
		}
		return fmt.Errorf("tasks.add: use `munin tasks` helpers; action host reserved (title=%s)", params["title"])
	})
}
