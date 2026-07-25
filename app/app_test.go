package app

import (
	"context"
	"errors"
	"testing"
	"testing/fstest"

	"github.com/codyconfer/munin/app/defaults"
	"github.com/codyconfer/munin/internal/app/onboard"
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
		"queries/my-open-prs.yaml",
		"queries/demo.yaml",
		"queries/demo-reviews.yaml",
		"queries/notify-smoke.yaml",
		"filters/demo.yaml",
		"flights/default.yaml",
		"flights/demo.yaml",
		"flights/notify-smoke.yaml",
		"demo.yaml",
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
