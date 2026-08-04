package views

import (
	"path/filepath"
	"strconv"
	"strings"

	vkdeck "github.com/codyconfer/viewkit/deck"
	"github.com/codyconfer/viewkit/forms"
	"github.com/codyconfer/viewkit/keys"

	"github.com/codyconfer/mino/internal/app/suggest"
	"github.com/codyconfer/mino/internal/app/verify"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/signals"
)

type configField struct {
	key     string
	kind    forms.FieldKind
	options []string
	sugg    forms.Suggester
	num     bool
	get     func(*config.Config) any
	set     func(*config.Config, any)
}

var configFields = []configField{
	{
		key: "output", kind: forms.FieldSelect, options: []string{"terminal", "json"},
		get: func(c *config.Config) any { return c.Output },
		set: func(c *config.Config, v any) { c.Output = configStr(v) },
	},
	{
		key: "audit.enabled", kind: forms.FieldToggle,
		get: func(c *config.Config) any { return c.Audit.Enabled },
		set: func(c *config.Config, v any) { c.Audit.Enabled = configBool(v) },
	},
	{
		key: "timeout", kind: forms.FieldText, sugg: suggest.DurationValues(),
		get: func(c *config.Config) any { return c.Timeout },
		set: func(c *config.Config, v any) { c.Timeout = configStr(v) },
	},
	{
		key: "cache.ttl", kind: forms.FieldText, sugg: suggest.DurationValues(),
		get: func(c *config.Config) any { return c.Cache.TTL },
		set: func(c *config.Config, v any) { c.Cache.TTL = configStr(v) },
	},
	{
		key: "backup.destination", kind: forms.FieldSelect, options: []string{"local", "gdrive"},
		get: func(c *config.Config) any { return c.Backup.Destination },
		set: func(c *config.Config, v any) { c.Backup.Destination = configStr(v) },
	},
	{
		key: "backup.keep", kind: forms.FieldText, num: true,
		get: func(c *config.Config) any { return c.Backup.Keep },
		set: func(c *config.Config, v any) { c.Backup.Keep = configInt(v) },
	},
	{
		key: "daemon.interval", kind: forms.FieldText, sugg: suggest.DurationValues(),
		get: func(c *config.Config) any { return c.Daemon.Interval },
		set: func(c *config.Config, v any) { c.Daemon.Interval = configStr(v) },
	},
	{
		key: "daemon.bell", kind: forms.FieldToggle,
		get: func(c *config.Config) any { return c.Daemon.Bell },
		set: func(c *config.Config, v any) { c.Daemon.Bell = configBool(v) },
	},
	{
		key: "daemon.desktop", kind: forms.FieldToggle,
		get: func(c *config.Config) any { return c.Daemon.Desktop },
		set: func(c *config.Config, v any) { c.Daemon.Desktop = configBool(v) },
	},
	{
		key: "daemon.theme", kind: forms.FieldSelect, options: []string{"dark", "light"},
		get: func(c *config.Config) any { return c.Daemon.Theme },
		set: func(c *config.Config, v any) { c.Daemon.Theme = configStr(v) },
	},
}

func configStr(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case int:
		return strconv.Itoa(t)
	case bool:
		return strconv.FormatBool(t)
	}
	return ""
}

func configBool(v any) bool {
	on, _ := v.(bool)
	return on
}

func configInt(v any) int {
	switch t := v.(type) {
	case int:
		return t
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(t))
		if err != nil {
			return 0
		}
		return n
	}
	return 0
}

func (f configField) field(c *config.Config) forms.Field {
	fd := forms.Field{Key: f.key, Label: f.key, Kind: f.kind}
	switch f.kind {
	case forms.FieldSelect:
		fd.Options = forms.SelectFirst(f.options, configStr(f.get(c)))
	case forms.FieldToggle:
		fd.On = configBool(f.get(c))
	default:
		fd.Text = configStr(f.get(c))
		fd.Suggest = f.sugg
	}
	return fd
}

func (f configField) value(vals map[string]any) any {
	switch {
	case f.kind == forms.FieldToggle:
		return forms.Bool(vals, f.key)
	case f.num:
		return forms.Int(vals, f.key)
	}
	return forms.Str(vals, f.key)
}

type configView struct {
	*editorShell

	kit  *Kit
	base *config.Config
}

func (kit *Kit) Config() vkdeck.View {
	base := kit.d.App.Cfg
	if base == nil {
		base = config.Defaults()
	}
	v := &configView{kit: kit, base: base}
	v.editorShell = newEditorShell(v, nil, kit.scope().Keys)
	return v
}

func (v *configView) editorKind() string { return "config" }

func (v *configView) editorTitle() string { return "config" }

func (v *configView) editorSync() bool { return false }

func (v *configView) editorCtx() []keys.Hint {
	ctx := v.kit.setvCtx()
	if name := v.editorSavedName(); name != "" {
		ctx = append(ctx, keys.Hint{Key: "file", Label: name})
	}
	return ctx
}

func (v *configView) editorSavedName() string {
	path, err := config.ConfigFilePath(v.kit.setvHome())
	if err != nil {
		return ""
	}
	return filepath.Base(path)
}

func (v *configView) editorFields(map[string]any) []forms.Field {
	fields := make([]forms.Field, 0, len(configFields)+4)
	for _, f := range configFields {
		fields = append(fields, f.field(v.base))
	}
	return append(fields, sectionFields(v.base)...)
}

func (v *configView) editorSummary() string {
	parts := []string{"output=" + v.Value("output")}
	if t := v.Value("timeout"); t != "" {
		parts = append(parts, "timeout="+t)
	}
	if name := v.editorSavedName(); name != "" {
		parts = append(parts, name)
	} else {
		parts = append(parts, "no config file yet")
	}
	return strings.Join(parts, "  ")
}

func (v *configView) dotted() map[string]any {
	vals := v.Form().Values()
	set := make(map[string]any, len(configFields))
	for _, f := range configFields {
		set[f.key] = f.value(vals)
	}
	sectionValues(vals, set)
	return set
}

func (v *configView) merged() *config.Config {
	c := *v.base
	set := v.dotted()
	for _, f := range configFields {
		if val, ok := set[f.key]; ok {
			f.set(&c, val)
		}
	}
	return &c
}

func (v *configView) editorValue() (any, error) {
	return configNest(v.dotted()), nil
}

func configNest(set map[string]any) map[string]any {
	out := make(map[string]any, len(set))
	for key, val := range set {
		parts := strings.Split(key, ".")
		node := out
		for _, p := range parts[:len(parts)-1] {
			child, ok := node[p].(map[string]any)
			if !ok {
				child = map[string]any{}
				node[p] = child
			}
			node = child
		}
		node[parts[len(parts)-1]] = val
	}
	return out
}

func (v *configView) findings() []Finding {
	return verify.Config(v.merged(), v.directives(), v.kit.d.App.Role())
}

func (v *configView) directives() *config.Directives {
	if d := v.kit.d.App.Dirs(); d != nil {
		return d
	}
	return &config.Directives{}
}

func (v *configView) editorFindingList(any) []Finding { return v.findings() }

func (v *configView) editorVerify(any) Finding {
	bad := 0
	for _, f := range v.findings() {
		if !f.OK {
			bad++
		}
	}
	if bad == 0 {
		return Finding{Name: "config", OK: true}
	}
	return Finding{Name: "config", Msg: strconv.Itoa(bad) + " to fix"}
}

func (v *configView) editorRun() (string, func() []signals.Section, error) {
	cfg, dirs, role := v.merged(), v.directives(), v.kit.d.App.Role()
	return "config", func() []signals.Section {
		return []signals.Section{configFindingSection(verify.Config(cfg, dirs, role))}
	}, nil
}

func configFindingSection(findings []Finding) signals.Section {
	items := make([]signals.Item, 0, len(findings))
	for _, f := range findings {
		it := signals.Item{Kind: configFindingKind(f), Title: f.Name}
		if !f.OK {
			it.Subtitle = f.Msg
		}
		items = append(items, it)
	}
	return signals.Section{Signal: "config", Title: "verify: config", Items: items}
}

func configFindingKind(f Finding) string {
	switch {
	case f.OK:
		return "success"
	case f.Warn:
		return "warn"
	}
	return "error"
}

func (v *configView) editorPersist(any) (string, error) {
	path, err := config.SetValues(v.kit.setvHome(), v.dotted())
	if err != nil {
		return "", err
	}
	return "wrote " + path, nil
}

func (v *configView) editorRemove() (string, error) {
	removed := setvDeleteConfigFiles(v.kit.setvHome())
	if len(removed) == 0 {
		return "", errs.New(errs.KindUsage, "no config file found")
	}
	return "removed:\n" + strings.Join(removed, "\n"), nil
}
