package cmd

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/codyconfer/mino/internal/app"
	"github.com/codyconfer/mino/internal/app/flight"
	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/signals"
)

type flightTestSignal struct {
	name  string
	items []string
	err   error
	boom  any
}

func (f flightTestSignal) Name() string { return f.name }

func (f flightTestSignal) Fetch(context.Context) ([]signals.Section, error) {
	if f.boom != nil {
		panic(f.boom)
	}
	if f.err != nil {
		return nil, f.err
	}
	items := make([]signals.Item, 0, len(f.items))
	for _, title := range f.items {
		items = append(items, signals.Item{Kind: "test", Title: title})
	}
	return []signals.Section{{Signal: f.name, Title: f.name, Items: items}}, nil
}

func useFlightTestApp(t *testing.T) {
	t.Helper()
	orig := shared
	t.Cleanup(func() { shared = orig })
	shared = &app.App{
		Cfg:        &config.Config{Home: t.TempDir(), Output: "json", Timeout: "5s", DefaultRole: "test"},
		Directives: &config.Directives{},
	}
	closeSharedDBs(t)
}

func TestRunQueriesWithReturnsErrorWhenEveryQueryFails(t *testing.T) {
	useFlightTestApp(t)
	authErr := errors.New("gh auth login: exit status 4")
	queries := []query{
		{Label: "github", Src: flightTestSignal{name: "github", err: authErr}},
		{Label: "gmail", Src: flightTestSignal{name: "gmail", err: authErr}},
	}

	var out bytes.Buffer
	err := runQueriesWith(context.Background(), &out, io.Discard, "default", queries, 0, runOpts{})
	if err == nil {
		t.Fatal("runQueriesWith with every query failing = nil, want a non-zero-exit error")
	}
	if !errors.Is(err, flight.ErrAllQueriesFailed) {
		t.Errorf("err = %v, want it to wrap flight.ErrAllQueriesFailed", err)
	}
	if !strings.Contains(out.String(), "exit status 4") {
		t.Errorf("output = %q, want the per-source errors still rendered in band", out.String())
	}
}

func TestRunQueriesWithPartialFailureSucceeds(t *testing.T) {
	useFlightTestApp(t)
	queries := []query{
		{Label: "github", Src: flightTestSignal{name: "github", err: errors.New("exit status 4")}},
		{Label: "gmail", Src: flightTestSignal{name: "gmail", items: []string{"one"}}},
	}

	var out bytes.Buffer
	if err := runQueriesWith(context.Background(), &out, io.Discard, "default", queries, 0, runOpts{}); err != nil {
		t.Fatalf("partial failure = %v, want nil so degraded runs still exit 0", err)
	}
	if !strings.Contains(out.String(), "exit status 4") {
		t.Errorf("output = %q, want the failing source rendered in band", out.String())
	}
	if !strings.Contains(out.String(), "one") {
		t.Errorf("output = %q, want the healthy source's item", out.String())
	}
}

func TestRunQueriesWithHealthyRunSucceeds(t *testing.T) {
	useFlightTestApp(t)
	queries := []query{{Label: "gmail", Src: flightTestSignal{name: "gmail", items: []string{"one"}}}}
	if err := runQueriesWith(context.Background(), io.Discard, io.Discard, "default", queries, 0, runOpts{}); err != nil {
		t.Fatalf("healthy run = %v, want nil", err)
	}
}

func TestRunQueriesWithFormatterReportsTotalFailure(t *testing.T) {
	useFlightTestApp(t)
	o := runOpts{
		formatter: config.FormatterDef{Name: "digest", Template: "{{ .Kind }}\n"},
		active:    true,
		kind:      "flight",
	}
	queries := []query{{Label: "github", Src: flightTestSignal{name: "github", err: errors.New("exit status 4")}}}

	var out bytes.Buffer
	err := runQueriesWith(context.Background(), &out, io.Discard, "default", queries, 0, o)
	if err == nil {
		t.Fatal("formatter run with every query failing = nil, want an error")
	}
	if !errors.Is(err, flight.ErrAllQueriesFailed) {
		t.Errorf("err = %v, want it to wrap flight.ErrAllQueriesFailed", err)
	}
	if !strings.Contains(out.String(), "flight") {
		t.Errorf("output = %q, want the report still delivered", out.String())
	}
}

func TestRunQueriesWithSurvivesPluginPanic(t *testing.T) {
	useFlightTestApp(t)
	queries := []query{
		{Label: "bad", Src: flightTestSignal{name: "panicker", boom: "assignment to entry in nil map"}},
		{Label: "gmail", Src: flightTestSignal{name: "gmail", items: []string{"one"}}},
	}

	var out bytes.Buffer
	if err := runQueriesWith(context.Background(), &out, io.Discard, "default", queries, 0, runOpts{}); err != nil {
		t.Fatalf("panicking plugin alongside a healthy one = %v, want nil", err)
	}
	if !strings.Contains(out.String(), "panicker") {
		t.Errorf("output = %q, want the panicking signal named", out.String())
	}
}
