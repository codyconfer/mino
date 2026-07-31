package tasks

import (
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/codyconfer/munin/cmd"
	"github.com/codyconfer/munin/external/plugins/internal/googleauth"
	"github.com/codyconfer/munin/external/plugins/internal/params"
	"github.com/codyconfer/munin/external/plugins/internal/stream"
	"github.com/codyconfer/munin/plugin"
)

const (
	PluginID   = "external.tasks"
	SignalName = "tasks"
)

func Register() {
	plugin.RegisterSignal(plugin.Descriptor{
		ID:           PluginID,
		Kind:         plugin.KindSignal,
		Signal:       SignalName,
		Capabilities: []plugin.Capability{plugin.CapQuery, plugin.CapStream, plugin.CapAction, plugin.CapCacheable},
	}, plugin.Builders{
		Query:  BuildQuery,
		Stream: BuildStream,
	})
	cmd.RegisterCommand(newTasksCmd)
}

func settings(bc plugin.BuildContext) (googleauth.Auth, map[string]any, error) {
	ga, err := googleauth.FromBuildContext(bc)
	if err != nil {
		return googleauth.Auth{}, nil, err
	}
	return ga, plugin.SettingsOf(bc, SignalName), nil
}

func BuildQuery(bc plugin.BuildContext) (plugin.Query, error) {
	ga, s, err := settings(bc)
	if err != nil {
		return nil, err
	}
	return New(ga, plugin.SettingList(s, "lists"), plugin.SettingBool(s, "show_completed", false),
		params.Int(bc.Params(), "max", plugin.SettingInt(s, "max", 100))), nil
}

func BuildStream(bc plugin.BuildContext) (plugin.Stream, error) {
	ga, s, err := settings(bc)
	if err != nil {
		return nil, err
	}
	interval, err := params.PollInterval(bc.Params(), SignalName, 60*time.Second)
	if err != nil {
		return nil, err
	}
	return NewActive(ga, plugin.SettingList(s, "lists"), plugin.SettingBool(s, "show_completed", false),
		interval, stream.StateOf(bc)), nil
}

func newTasksCmd() *cobra.Command {
	parent := cmd.SignalCmd(SignalName, "Google Tasks (read any list; create tasks only in the configured list)")

	var notes, due, list string
	add := &cobra.Command{
		Use:   "add <title>",
		Short: "Create a task in the writable list (plugins.tasks.list)",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(c *cobra.Command, args []string) error {
			return addTask(c, strings.Join(args, " "), notes, due, list)
		},
	}
	add.Flags().StringVar(&notes, "notes", "", "task notes/body")
	add.Flags().StringVar(&due, "due", "", "due date (YYYY-MM-DD or RFC3339)")
	add.Flags().StringVar(&list, "list", "", "target list; must be the configured writable list")
	parent.AddCommand(add)
	return parent
}

func addTask(c *cobra.Command, title, notes, due, list string) error {
	host := cmd.Host()
	configured := plugin.Setting(host.Settings(SignalName), "list", "")
	target, err := cmd.ResolveWriteTarget("task list", "plugins.tasks.list", configured, list)
	if err != nil {
		return err
	}
	started := time.Now()
	item, err := CreateTask(c.Context(), googleauth.FromHost(host), target, title, notes, due)
	if err != nil {
		return err
	}
	sections := []plugin.Section{{Signal: SignalName, Title: "Created task in " + target, Items: []plugin.Item{item}}}
	cmd.RecordAction("tasks add", started, time.Now(), sections)
	return cmd.EmitSections(c, SignalName, sections)
}
