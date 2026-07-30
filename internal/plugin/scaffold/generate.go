package scaffold

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"
)

type GenerateOptions struct {
	ID      string
	Dir     string
	Signal  string
	Package string
	Module  string
	Force   bool
}

type GenerateResult struct {
	Dir     string
	Written []string
}

var (
	identRE  = regexp.MustCompile(`^[a-z][a-z0-9_]*$`)
	signalRE = regexp.MustCompile(`^[a-z][a-z0-9_-]*$`)
)

// maxNameLen keeps generated names inside every filesystem's per-component
// limit (255 on ext4/APFS/NTFS). Without a cap a long-but-valid signal passes
// validation and only fails at os.WriteFile, after the other files are written.
const maxNameLen = 64

func Generate(opts GenerateOptions) (GenerateResult, error) {
	id := strings.TrimSpace(opts.ID)
	if id == "" {
		return GenerateResult{}, fmt.Errorf("scaffold: plugin id required")
	}
	if strings.ContainsAny(id, " \t\n/") || strings.HasPrefix(id, ".") || strings.HasSuffix(id, ".") {
		return GenerateResult{}, fmt.Errorf("scaffold: invalid plugin id %q", id)
	}
	if len(id) > maxNameLen {
		return GenerateResult{}, fmt.Errorf("scaffold: plugin id is %d characters; keep it to %d or fewer", len(id), maxNameLen)
	}
	dir := strings.TrimSpace(opts.Dir)
	if dir == "" {
		return GenerateResult{}, fmt.Errorf("scaffold: output dir required")
	}
	dir, err := filepath.Abs(dir)
	if err != nil {
		return GenerateResult{}, err
	}

	signal := strings.TrimSpace(opts.Signal)
	if signal == "" {
		signal = defaultSignal(id)
	}
	if !signalRE.MatchString(signal) {
		return GenerateResult{}, fmt.Errorf("scaffold: invalid signal name %q (want %s)", signal, signalRE)
	}
	if len(signal) > maxNameLen {
		return GenerateResult{}, fmt.Errorf("scaffold: signal name is %d characters; keep it to %d or fewer", len(signal), maxNameLen)
	}
	pkg := strings.TrimSpace(opts.Package)
	if pkg == "" {
		pkg = sanitizeIdent(signal)
	}
	if !identRE.MatchString(pkg) {
		return GenerateResult{}, fmt.Errorf("scaffold: invalid package name %q", pkg)
	}
	if len(pkg) > maxNameLen {
		return GenerateResult{}, fmt.Errorf("scaffold: package name is %d characters; keep it to %d or fewer", len(pkg), maxNameLen)
	}

	files := map[string]string{
		"plugin.go":      renderPluginGo(pkg, id, signal),
		"plugin_test.go": renderPluginTest(pkg, signal),
		filepath.Join("queries", signal+"-ping.yaml"): renderQueryYAML(signal),
	}

	rels := make([]string, 0, len(files))
	for rel := range files {
		rels = append(rels, rel)
	}
	sort.Strings(rels)
	paths := make([]string, len(rels))
	for i, rel := range rels {
		p, err := resolveWithin(dir, rel)
		if err != nil {
			return GenerateResult{}, err
		}
		paths[i] = p
	}

	// Anything this call creates is rolled back if a later write fails, so a
	// half-written scaffold is never left behind for the user to clean up.
	var created []string
	fail := func(err error) (GenerateResult, error) {
		for i := len(created) - 1; i >= 0; i-- {
			_ = os.Remove(created[i])
		}
		return GenerateResult{}, err
	}
	if _, err := os.Stat(dir); os.IsNotExist(err) {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return GenerateResult{}, err
		}
		created = append(created, dir)
	} else if err := os.MkdirAll(dir, 0o755); err != nil {
		return GenerateResult{}, err
	}
	var written []string
	for i, rel := range rels {
		body := files[rel]
		path := paths[i]
		parent := filepath.Dir(path)
		if _, err := os.Stat(parent); os.IsNotExist(err) {
			if err := os.MkdirAll(parent, 0o755); err != nil {
				return fail(err)
			}
			created = append(created, parent)
		}
		switch _, err := os.Stat(path); {
		case err == nil && !opts.Force:
			return fail(fmt.Errorf("scaffold: %s exists (pass --force to overwrite)", path))
		case err == nil:
		case os.IsNotExist(err):
			created = append(created, path)
		default:
			return fail(err)
		}
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			return fail(err)
		}
		written = append(written, rel)
	}
	return GenerateResult{Dir: dir, Written: written}, nil
}

func resolveWithin(dir, rel string) (string, error) {
	if rel == "" || filepath.IsAbs(rel) || os.IsPathSeparator(rel[0]) || filepath.VolumeName(rel) != "" {
		return "", fmt.Errorf("scaffold: refusing to write %q outside %s", rel, dir)
	}
	path := filepath.Join(dir, rel)
	inside, err := filepath.Rel(dir, path)
	if err != nil {
		return "", fmt.Errorf("scaffold: refusing to write %q outside %s", rel, dir)
	}
	if inside == ".." || strings.HasPrefix(inside, ".."+string(filepath.Separator)) || filepath.IsAbs(inside) {
		return "", fmt.Errorf("scaffold: refusing to write %q outside %s", rel, dir)
	}
	return path, nil
}

func defaultSignal(id string) string {
	parts := strings.Split(id, ".")
	return sanitizeIdent(parts[len(parts)-1])
}

func sanitizeIdent(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	for i, r := range s {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			if i == 0 {
				b.WriteByte('p')
			}
			b.WriteRune(r)
		case r == '_' || r == '-' || r == '.':
			if b.Len() > 0 {
				b.WriteByte('_')
			}
		default:
			if unicode.IsLetter(r) {
				b.WriteRune(unicode.ToLower(r))
			}
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "plugin"
	}
	if out[0] >= '0' && out[0] <= '9' {
		return "p" + out
	}
	return out
}

func renderPluginGo(pkg, id, signal string) string {
	filterName := signal + "-clean"
	return fmt.Sprintf(`// Code generated by munin plugins scaffold; edit as needed.
package %s

import (
	"context"
	"strings"

	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/munin/plugin"
)

const (
	PluginID    = %q
	SignalName  = %q
	GlyphID     = %q
	ContextTool = %q
)

// Register installs this plugin's contributions. Call from
// app.Options.RegisterPlugins in an overlay binary.
func Register() {
	if _, ok := plugin.Lookup(PluginID); ok {
		return
	}
	plugin.RegisterSignal(plugin.Descriptor{
		ID:           PluginID,
		Kind:         plugin.KindSignal,
		Signal:       SignalName,
		Capabilities: []plugin.Capability{plugin.CapQuery},
	}, plugin.Builders{
		Query: func(plugin.BuildContext) (plugin.Query, error) {
			return Signal{}, nil
		},
	})
	glyph.Register(GlyphID, glyph.Variants{Nerd: "", Uni: "◇", ASCII: "pl"})
	plugin.RegisterContext(PluginID, &provider{})

	// Example KindFilter engine — reference from a query with filters: [%s]
	if !plugin.HasFilter(%q) {
		plugin.RegisterFilterEngine(PluginID, %q, func(items []plugin.Item) []plugin.Item {
			out := make([]plugin.Item, 0, len(items))
			for _, it := range items {
				if strings.TrimSpace(it.Title) == "" {
					continue
				}
				out = append(out, it)
			}
			return out
		})
	}
}

// Signal is a minimal Query capability implementation.
type Signal struct{}

func (Signal) Name() string { return SignalName }

func (Signal) Fetch(context.Context) ([]plugin.Section, error) {
	return []plugin.Section{{
		Signal: SignalName,
		Title:  SignalName,
		Items: []plugin.Item{{
			Kind:  SignalName,
			Title: SignalName + " plugin is alive",
		}},
	}}, nil
}

type provider struct{ current string }

func (p *provider) Tool() string { return ContextTool }

func (p *provider) Switch(_ context.Context, name string) error {
	p.current = name
	return nil
}

func (p *provider) Current(context.Context) (string, bool, error) {
	if p.current == "" {
		return "", false, nil
	}
	return p.current, true, nil
}
`, pkg, id, signal, id, signal, filterName, filterName, filterName)
}

func renderPluginTest(pkg, _ string) string {
	return fmt.Sprintf(`package %s

import (
	"context"
	"testing"

	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/munin/plugin"
)

func TestRegister(t *testing.T) {
	Register()
	d, ok := plugin.Lookup(PluginID)
	if !ok || d.Signal != SignalName {
		t.Fatalf("lookup = %%+v ok=%%v", d, ok)
	}
	if _, ok := glyph.Lookup(GlyphID); !ok {
		t.Fatal("glyph not registered")
	}
	if !plugin.HasFilterEngine(SignalName + "-clean") {
		t.Fatal("expected filter engine")
	}
	secs, err := Signal{}.Fetch(context.Background())
	if err != nil || len(secs) != 1 {
		t.Fatalf("Fetch = %%v err=%%v", secs, err)
	}
	if err := plugin.SwitchContext(context.Background(), ContextTool, "demo"); err != nil {
		t.Fatal(err)
	}
}
`, pkg)
}

func renderQueryYAML(signal string) string {
	return fmt.Sprintf(`# Example query seed — copy anywhere under the munin home, or RegisterSeeds.
name: %s-ping
type: query
signal: %s
params: {}
filters: [%s-clean]
`, signal, signal, signal)
}
