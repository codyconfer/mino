package app

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/codyconfer/mino/app/defaults"
	"github.com/codyconfer/mino/internal/app/onboard"
	"github.com/codyconfer/mino/plugin"
)

func TestApplyBuildPolicy(t *testing.T) {
	origDomain := onboard.RequiredEmailDomain
	origAllOrNothing := onboard.AllOrNothingAuth
	t.Cleanup(func() {
		onboard.RequiredEmailDomain = origDomain
		onboard.AllOrNothingAuth = origAllOrNothing
	})

	onboard.RequiredEmailDomain = ""
	onboard.AllOrNothingAuth = ""
	applyBuildPolicy(Options{EmailDomain: "example.com", AllOrNothingAuth: true})
	if onboard.RequiredEmailDomain != "example.com" {
		t.Fatalf("RequiredEmailDomain = %q", onboard.RequiredEmailDomain)
	}
	if onboard.AllOrNothingAuth != "true" {
		t.Fatalf("AllOrNothingAuth = %q", onboard.AllOrNothingAuth)
	}
}

func TestRunRequiresCLI(t *testing.T) {
	if err := Run(Options{}); !errors.Is(err, ErrNoCLI) {
		t.Fatalf("got %v, want ErrNoCLI", err)
	}
}

func TestListDefaultsEmbedded(t *testing.T) {
	files, err := ListDefaults(defaults.FS)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) < 4 {
		t.Fatalf("expected seed files, got %d", len(files))
	}
	seen := map[string]bool{}
	for _, f := range files {
		seen[f.RelPath] = true
		if len(f.Data) == 0 {
			t.Errorf("empty file %s", f.RelPath)
		}
	}
	for _, want := range []string{
		"config.yaml",
		"default.yaml",
		"queries/my-open-prs.yaml",
		"queries/sisyphus-open-prs.yaml",
		"queries/sisyphus-ci.yaml",
		"queries/viewkit-open-prs.yaml",
		"queries/viewkit-ci.yaml",
		"queries/mino-open-prs.yaml",
		"queries/mino-ci.yaml",
		"queries/no-bots.yaml",
		"flights/default.yaml",
	} {
		if !seen[want] {
			t.Errorf("missing %s", want)
		}
	}
}

func TestListDefaultsNil(t *testing.T) {
	files, err := ListDefaults(nil)
	if err != nil || files != nil {
		t.Fatalf("got %v %v", files, err)
	}
}

func TestListDefaultsMapFS(t *testing.T) {
	fsys := fstest.MapFS{
		"config.yaml":       &fstest.MapFile{Data: []byte("output: json\n")},
		"queries/ping.yaml": &fstest.MapFile{Data: []byte("name: ping\n")},
		"queries/.ignore":   &fstest.MapFile{Data: []byte("x")},
	}
	files, err := ListDefaults(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if len(files) != 2 {
		t.Fatalf("got %d files: %+v", len(files), files)
	}
}

func TestDeprecatedEnforceAuthAlias(t *testing.T) {
	origDomain := onboard.RequiredEmailDomain
	origAllOrNothing := onboard.AllOrNothingAuth
	t.Cleanup(func() {
		onboard.RequiredEmailDomain = origDomain
		onboard.AllOrNothingAuth = origAllOrNothing
	})

	for _, tc := range []struct {
		name             string
		allOrNothing     bool
		enforceDeprecate bool
		want             string
	}{
		{"neither", false, false, ""},
		{"deprecated only", false, true, "true"},
		{"new only", true, false, "true"},
		{"both", true, true, "true"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			onboard.AllOrNothingAuth = ""
			applyBuildPolicy(Options{
				AllOrNothingAuth: tc.allOrNothing,
				EnforceAuth:      tc.enforceDeprecate,
			})
			if onboard.AllOrNothingAuth != tc.want {
				t.Fatalf("AllOrNothingAuth = %q, want %q", onboard.AllOrNothingAuth, tc.want)
			}
		})
	}
}

func TestReportPluginDiagnostics(t *testing.T) {
	plugin.Register(plugin.Descriptor{ID: "apptest.diag.a", Kind: plugin.KindSignal, Signal: "apptestdiag"})
	plugin.Register(plugin.Descriptor{ID: "apptest.diag.b", Kind: plugin.KindSignal, Signal: "apptestdiag"})

	var buf bytes.Buffer
	ReportPluginDiagnostics(&buf)
	out := buf.String()
	if !strings.Contains(out, "apptest.diag.b") || !strings.Contains(out, "apptestdiag") {
		t.Fatalf("diagnostics output = %q, want it to name the offending plugin and ref", out)
	}
	ReportPluginDiagnostics(nil)
}

func TestRunSurvivesPanickingPluginRegistration(t *testing.T) {
	ran := false
	opts := Options{
		Args:            []string{"--help"},
		RegisterPlugins: func() { panic(`plugin: duplicate signal ref "github" (mino.github and bad.dup)`) },
		CLI: func(context.Context, []string) error {
			ran = true
			return nil
		},
	}
	if err := Run(opts); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	if !ran {
		t.Fatal("CLI never ran: a panicking plugin bricked the binary")
	}
	found := false
	for _, d := range plugin.Diagnostics() {
		if strings.Contains(d.Message, "registration panicked") && strings.Contains(d.Message, "bad.dup") {
			found = true
		}
	}
	if !found {
		t.Fatal("the recovered registration panic was not surfaced as a diagnostic")
	}
}

func TestHooksOrder(t *testing.T) {
	var steps []string
	opts := Options{
		Args: []string{"--help"},
		CLI: func(_ context.Context, args []string) error {
			steps = append(steps, "cli:"+args[0])
			return nil
		},
		RegisterPlugins: func() { steps = append(steps, "register") },
		BeforeRun: func(context.Context) error {
			steps = append(steps, "before")
			return nil
		},
		AfterRun: func(_ context.Context, err error) {
			steps = append(steps, "after")
			if err != nil {
				t.Errorf("unexpected err: %v", err)
			}
		},
	}
	if err := Run(opts); err != nil {
		t.Fatal(err)
	}
	want := []string{"register", "before", "cli:--help", "after"}
	if len(steps) != len(want) {
		t.Fatalf("steps = %v", steps)
	}
	for i := range want {
		if steps[i] != want[i] {
			t.Fatalf("steps = %v, want %v", steps, want)
		}
	}
}
