package plugins

import (
	"github.com/codyconfer/munin/external/plugins/calendar"
	"github.com/codyconfer/munin/external/plugins/demo"
	"github.com/codyconfer/munin/external/plugins/docs"
	"github.com/codyconfer/munin/external/plugins/drive"
	"github.com/codyconfer/munin/external/plugins/gmail"
	"github.com/codyconfer/munin/external/plugins/google"
	"github.com/codyconfer/munin/external/plugins/slack"
	"github.com/codyconfer/munin/external/plugins/tasks"
	"github.com/codyconfer/munin/plugin"
)

func Register() {
	plugin.Guarded(google.PluginID, google.Register)
	plugin.Guarded(calendar.PluginID, calendar.Register)
	plugin.Guarded(gmail.PluginID, gmail.Register)
	plugin.Guarded(docs.PluginID, docs.Register)
	plugin.Guarded(drive.PluginID, drive.Register)
	plugin.Guarded(tasks.PluginID, tasks.Register)
	plugin.Guarded(slack.PluginID, slack.Register)
	plugin.Guarded(demo.PluginID, demo.Register)
}
