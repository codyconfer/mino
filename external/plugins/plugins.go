package plugins

import (
	"github.com/codyconfer/mino/external/plugins/argocd"
	"github.com/codyconfer/mino/external/plugins/calendar"
	"github.com/codyconfer/mino/external/plugins/demo"
	"github.com/codyconfer/mino/external/plugins/docs"
	"github.com/codyconfer/mino/external/plugins/drive"
	"github.com/codyconfer/mino/external/plugins/example"
	"github.com/codyconfer/mino/external/plugins/gcx"
	"github.com/codyconfer/mino/external/plugins/gmail"
	"github.com/codyconfer/mino/external/plugins/google"
	"github.com/codyconfer/mino/external/plugins/kubectl"
	"github.com/codyconfer/mino/external/plugins/ollama"
	"github.com/codyconfer/mino/external/plugins/slack"
	"github.com/codyconfer/mino/external/plugins/tasks"
	"github.com/codyconfer/mino/plugin"
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
	plugin.Guarded(gcx.PluginID, gcx.Register)
	plugin.Guarded(kubectl.PluginID, kubectl.Register)
	plugin.Guarded(ollama.PluginID, ollama.Register)
	plugin.Guarded(argocd.PluginID, argocd.Register)
	plugin.Guarded(example.PluginID, example.Register)
}
