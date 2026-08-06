package kubectl

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/mino/external/plugins/internal/stream"
	"github.com/codyconfer/mino/plugin"
)

func TestMain(m *testing.M) {
	Register()
	os.Exit(m.Run())
}

func fixture(t *testing.T, name string) []byte {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

type fakeRunner struct {
	mu    sync.Mutex
	calls [][]string

	byResource map[string][]byte
	errs       map[string]error
}

func (f *fakeRunner) set(resource string, raw []byte) {
	f.mu.Lock()
	f.byResource[resource] = raw
	f.mu.Unlock()
}

func newFakeRunner(t *testing.T) *fakeRunner {
	t.Helper()
	return &fakeRunner{
		byResource: map[string][]byte{
			"pods":            fixture(t, "pods.json"),
			"events":          fixture(t, "events.json"),
			"nodes":           fixture(t, "nodes.json"),
			workloadResources: fixture(t, "workloads.json"),
		},
		errs: map[string]error{},
	}
}

func (f *fakeRunner) Run(_ context.Context, args ...string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, slices.Clone(args))

	resource := resourceOf(args)
	if err, ok := f.errs[resource]; ok {
		return nil, err
	}
	if raw, ok := f.byResource[resource]; ok {
		return raw, nil
	}
	return nil, errors.New("fakeRunner: no fixture for " + strings.Join(args, " "))
}

func (f *fakeRunner) argv() [][]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

func (f *fakeRunner) argvFor(resource string) []string {
	for _, call := range f.argv() {
		if resourceOf(call) == resource {
			return call
		}
	}
	return nil
}

func resourceOf(args []string) string {
	for i, a := range args {
		if a == "get" && i+1 < len(args) {
			return args[i+1]
		}
	}
	return ""
}

func titles(items []plugin.Item) []string {
	out := make([]string, 0, len(items))
	for _, it := range items {
		out = append(out, it.Title)
	}
	return out
}

func sectionTitles(secs []plugin.Section) []string {
	out := make([]string, 0, len(secs))
	for _, s := range secs {
		out = append(out, s.Title)
	}
	return out
}

func sectionByTitle(t *testing.T, secs []plugin.Section, title string) plugin.Section {
	t.Helper()
	for _, s := range secs {
		if s.Title == title {
			return s
		}
	}
	t.Fatalf("no %q section in %v", title, sectionTitles(secs))
	return plugin.Section{}
}

func itemsByTitle(items []plugin.Item) map[string]plugin.Item {
	out := make(map[string]plugin.Item, len(items))
	for _, it := range items {
		out[it.Title] = it
	}
	return out
}

var fixtureNow = time.Date(2026, 8, 5, 12, 0, 0, 0, time.UTC)

func testOptions() options {
	return options{
		Collectors:       DefaultCollectors(),
		Since:            DefaultSince,
		Limit:            DefaultLimit,
		RestartThreshold: DefaultRestartThreshold,
		Now:              func() time.Time { return fixtureNow },
	}
}

func TestMapPodsJSONKeepsOnlyUnhealthyPods(t *testing.T) {
	sec, err := mapPodsJSON(fixture(t, "pods.json"), DefaultRestartThreshold, DefaultLimit)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"payments/checkout-7d9f8b6c4-2xk9p",
		"ops/evicted-worker-2ttlx",
		"payments/ledger-5c4d8f7b9-qq4mt",
		"storefront/api-6b7c9d5f4-hh8rn",
	}
	if got := titles(sec.Items); !slices.Equal(got, want) {
		t.Fatalf("pods = %v\nwant %v", got, want)
	}
	if sec.Meta["scanned"] != "7" {
		t.Fatalf("scanned = %q, want 7", sec.Meta["scanned"])
	}

	byTitle := itemsByTitle(sec.Items)
	for _, c := range []struct{ title, subtitle string }{
		{"payments/checkout-7d9f8b6c4-2xk9p", "CrashLoopBackOff"},
		{"payments/ledger-5c4d8f7b9-qq4mt", "ImagePullBackOff"},
		{"ops/evicted-worker-2ttlx", "Evicted"},
		{"storefront/api-6b7c9d5f4-hh8rn", "Restarting"},
	} {
		if got := byTitle[c.title].Subtitle; got != c.subtitle {
			t.Errorf("%s subtitle = %q, want %q", c.title, got, c.subtitle)
		}
	}
	if got := byTitle["ops/evicted-worker-2ttlx"].Meta["severity"]; got != sevCritical {
		t.Errorf("Failed severity = %q, want %q", got, sevCritical)
	}
}

func TestMapPodsJSONSkipsSucceededCreatingAndHealthy(t *testing.T) {
	sec, err := mapPodsJSON(fixture(t, "pods.json"), DefaultRestartThreshold, DefaultLimit)
	if err != nil {
		t.Fatal(err)
	}
	for _, unwanted := range []string{
		"ops/nightly-backup-29011500-fj7cz",
		"ops/migrate-once-8kkm2",
		"storefront/web-849cbb7f56-lp2vd",
	} {
		if slices.Contains(titles(sec.Items), unwanted) {
			t.Errorf("%s should not be reported", unwanted)
		}
	}
}

func TestMapPodsJSONRestartThresholdIsHonoured(t *testing.T) {
	sec, err := mapPodsJSON(fixture(t, "pods.json"), 20, DefaultLimit)
	if err != nil {
		t.Fatal(err)
	}
	if slices.Contains(titles(sec.Items), "storefront/api-6b7c9d5f4-hh8rn") {
		t.Error("7 restarts is below a threshold of 20 and must not be reported")
	}
}

func TestMapPodsJSONLimitTruncates(t *testing.T) {
	sec, err := mapPodsJSON(fixture(t, "pods.json"), DefaultRestartThreshold, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(sec.Items) != 2 {
		t.Fatalf("len = %d, want 2", len(sec.Items))
	}
}

func TestMapEventsJSONWindowsAndDropsNormal(t *testing.T) {
	sec, err := mapEventsJSON(fixture(t, "events.json"), time.Hour, DefaultLimit, fixtureNow)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"payments/pod/checkout-7d9f8b6c4-2xk9p",
		"payments/pod/ledger-5c4d8f7b9-qq4mt",
		"node/ip-10-0-3-77",
	}
	if got := titles(sec.Items); !slices.Equal(got, want) {
		t.Fatalf("events = %v\nwant %v", got, want)
	}
	if sec.Meta["window"] != "1h0m0s" {
		t.Errorf("window = %q", sec.Meta["window"])
	}
	if got := sec.Items[0].Body; !strings.Contains(got, "(×47)") {
		t.Errorf("repeat count missing from body %q", got)
	}
}

func TestMapEventsJSONWiderWindowPicksUpTheStaleEvent(t *testing.T) {
	sec, err := mapEventsJSON(fixture(t, "events.json"), 24*time.Hour, DefaultLimit, fixtureNow)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(titles(sec.Items), "storefront/pod/web-849cbb7f56-lp2vd") {
		t.Fatalf("stale event missing from %v", titles(sec.Items))
	}
}

func TestMapNodesJSONReportsUnreadyPressureAndCordon(t *testing.T) {
	sec, err := mapNodesJSON(fixture(t, "nodes.json"), DefaultLimit)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"ip-10-0-3-77", "ip-10-0-2-31"}
	if got := titles(sec.Items); !slices.Equal(got, want) {
		t.Fatalf("nodes = %v\nwant %v", got, want)
	}
	if got := sec.Items[0].Subtitle; got != "Ready=Unknown (NodeStatusUnknown)" {
		t.Errorf("unready subtitle = %q", got)
	}
	if got := sec.Items[0].Meta["severity"]; got != sevCritical {
		t.Errorf("unready severity = %q, want %q", got, sevCritical)
	}
	if got := sec.Items[1].Body; got != "DiskPressure · cordoned" {
		t.Errorf("pressure body = %q", got)
	}
}

func TestMapWorkloadsJSONReportsDegradedAndStuckRollouts(t *testing.T) {
	sec, err := mapWorkloadsJSON(fixture(t, "workloads.json"), DefaultLimit)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"payments/checkout",
		"storefront/web",
		"storefront/api",
		"monitoring/node-exporter",
	}
	if got := titles(sec.Items); !slices.Equal(got, want) {
		t.Fatalf("workloads = %v\nwant %v", got, want)
	}
	byTitle := itemsByTitle(sec.Items)
	for _, c := range []struct{ title, subtitle string }{
		{"payments/checkout", "deployment · no ready replicas"},
		{"storefront/api", "deployment · degraded"},
		{"storefront/web", "deployment · rollout in progress"},
		{"monitoring/node-exporter", "daemonset · degraded"},
	} {
		if got := byTitle[c.title].Subtitle; got != c.subtitle {
			t.Errorf("%s subtitle = %q, want %q", c.title, got, c.subtitle)
		}
	}
	if got := byTitle["monitoring/node-exporter"].Body; got != "2/3 ready · 3/3 updated" {
		t.Errorf("daemonset body = %q", got)
	}
}

func TestMapWorkloadsJSONIgnoresScaledToZeroAndHealthy(t *testing.T) {
	sec, err := mapWorkloadsJSON(fixture(t, "workloads.json"), DefaultLimit)
	if err != nil {
		t.Fatal(err)
	}
	for _, unwanted := range []string{"ops/batch-runner", "payments/ledger-db", "monitoring/fluent-bit"} {
		if slices.Contains(titles(sec.Items), unwanted) {
			t.Errorf("%s should not be reported", unwanted)
		}
	}
}

func TestFetchRunsEveryCollectorInOrder(t *testing.T) {
	r := newFakeRunner(t)
	secs, err := NewSignal(r, scope{allNamespaces: true}, testOptions()).Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := []string{titlePods, titleEvents, titleNodes, titleWorkloads}
	if got := sectionTitles(secs); !slices.Equal(got, want) {
		t.Fatalf("sections = %v\nwant %v", got, want)
	}
	for _, s := range secs {
		if s.Err != nil {
			t.Errorf("%s: %v", s.Title, s.Err)
		}
		if s.Signal != SignalName {
			t.Errorf("%s: signal = %q", s.Title, s.Signal)
		}
	}
}

func TestFetchInjectsTheContextIntoEveryCall(t *testing.T) {
	r := newFakeRunner(t)
	s := scope{context: "prod-us-east", allNamespaces: true, timeout: DefaultTimeout}
	if _, err := NewSignal(r, s, testOptions()).Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	calls := r.argv()
	if len(calls) != 4 {
		t.Fatalf("calls = %d, want 4", len(calls))
	}
	for _, call := range calls {
		if i := slices.Index(call, "--context"); i < 0 || i+1 >= len(call) || call[i+1] != "prod-us-east" {
			t.Errorf("--context prod-us-east missing from %v", call)
		}
		if !slices.Contains(call, "--request-timeout") {
			t.Errorf("--request-timeout missing from %v", call)
		}
	}
}

func TestFetchNeverMutatesKubeconfig(t *testing.T) {
	r := newFakeRunner(t)
	s := scope{context: "staging", allNamespaces: true}
	if _, err := NewSignal(r, s, testOptions()).Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, call := range r.argv() {
		joined := strings.Join(call, " ")
		for _, forbidden := range []string{"use-context", "set-context", "set-cluster", "set-credentials", "delete-context"} {
			if strings.Contains(joined, forbidden) {
				t.Fatalf("kubectl invocation must be read-only, got %q", joined)
			}
		}
		if !slices.Contains(call, "get") {
			t.Fatalf("non-get invocation %q", joined)
		}
	}
}

func TestFetchNamespaceScoping(t *testing.T) {
	t.Run("named namespace", func(t *testing.T) {
		r := newFakeRunner(t)
		if _, err := NewSignal(r, scope{namespace: "payments"}, testOptions()).Fetch(context.Background()); err != nil {
			t.Fatal(err)
		}
		pods := r.argvFor("pods")
		if i := slices.Index(pods, "--namespace"); i < 0 || pods[i+1] != "payments" {
			t.Errorf("--namespace payments missing from %v", pods)
		}
		if slices.Contains(pods, "--all-namespaces") {
			t.Errorf("--all-namespaces must not accompany a named namespace: %v", pods)
		}
	})

	t.Run("all namespaces", func(t *testing.T) {
		r := newFakeRunner(t)
		if _, err := NewSignal(r, scope{allNamespaces: true}, testOptions()).Fetch(context.Background()); err != nil {
			t.Fatal(err)
		}
		if pods := r.argvFor("pods"); !slices.Contains(pods, "--all-namespaces") {
			t.Errorf("--all-namespaces missing from %v", pods)
		}
	})

	t.Run("nodes are cluster scoped", func(t *testing.T) {
		r := newFakeRunner(t)
		if _, err := NewSignal(r, scope{namespace: "payments"}, testOptions()).Fetch(context.Background()); err != nil {
			t.Fatal(err)
		}
		nodes := r.argvFor("nodes")
		if slices.Contains(nodes, "--namespace") || slices.Contains(nodes, "--all-namespaces") {
			t.Errorf("nodes must not carry a namespace selection: %v", nodes)
		}
	})
}

func TestFetchPutsGlobalFlagsBeforeGetAndSelectorsAfter(t *testing.T) {
	r := newFakeRunner(t)
	s := scope{context: "prod", allNamespaces: true, timeout: DefaultTimeout}
	if _, err := NewSignal(r, s, testOptions()).Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	for _, call := range r.argv() {
		get := slices.Index(call, "get")
		if get < 0 {
			t.Fatalf("no subcommand in %v", call)
		}
		for _, global := range []string{"--context", "--request-timeout"} {
			if i := slices.Index(call, global); i > get {
				t.Errorf("%s must precede the subcommand in %v", global, call)
			}
		}
		for _, selector := range []string{"--all-namespaces", "--namespace"} {
			if i := slices.Index(call, selector); i >= 0 && i < get {
				t.Errorf("%s must follow the subcommand in %v", selector, call)
			}
		}
	}

	named := newFakeRunner(t)
	if _, err := NewSignal(named, scope{namespace: "payments"}, testOptions()).Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	pods := named.argvFor("pods")
	if i, get := slices.Index(pods, "--namespace"), slices.Index(pods, "get"); i < get {
		t.Errorf("--namespace must follow the subcommand in %v", pods)
	}
}

func TestFetchEventsAskForWarningsOnly(t *testing.T) {
	r := newFakeRunner(t)
	if _, err := NewSignal(r, scope{allNamespaces: true}, testOptions()).Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	events := r.argvFor("events")
	if i := slices.Index(events, "--field-selector"); i < 0 || events[i+1] != "type=Warning" {
		t.Errorf("warning field-selector missing from %v", events)
	}
}

func TestFetchSurvivesAFailingCollector(t *testing.T) {
	r := newFakeRunner(t)
	boom := errors.New("the server could not find the requested resource")
	r.errs["pods"] = boom

	secs, err := NewSignal(r, scope{allNamespaces: true}, testOptions()).Fetch(context.Background())
	if err != nil {
		t.Fatalf("a failing collector must not fail the run: %v", err)
	}
	if pods := sectionByTitle(t, secs, titlePods); !errors.Is(pods.Err, boom) {
		t.Fatalf("pods.Err = %v, want %v", pods.Err, boom)
	}
	for _, title := range []string{titleEvents, titleNodes, titleWorkloads} {
		sec := sectionByTitle(t, secs, title)
		if sec.Err != nil {
			t.Errorf("%s must still succeed, got %v", title, sec.Err)
		}
		if len(sec.Items) == 0 {
			t.Errorf("%s returned nothing", title)
		}
	}
}

func TestFetchReportsAMissingBinary(t *testing.T) {
	opts := testOptions()
	opts.Binary = "mino-kubectl-does-not-exist"
	_, err := NewSignal(nil, scope{}, opts).Fetch(context.Background())
	if err == nil {
		t.Fatal("want an error when the binary is absent")
	}
	if !strings.Contains(err.Error(), "hint:") {
		t.Errorf("error should carry a hint, got %q", err)
	}
}

func TestFetchReportsAnUnknownCollector(t *testing.T) {
	r := newFakeRunner(t)
	opts := testOptions()
	opts.Collectors = []string{"pods", "nonsense"}

	secs, err := NewSignal(r, scope{allNamespaces: true}, opts).Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if bad := sectionByTitle(t, secs, "nonsense"); bad.Err == nil {
		t.Fatal("unknown collector must report an error section")
	}
}

func TestContextCollectorReportsTheSelection(t *testing.T) {
	r := newFakeRunner(t)
	opts := testOptions()
	opts.Collectors = []string{CollectorContext}

	secs, err := NewSignal(r, scope{context: "staging", namespace: "payments"}, opts).Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	sec := sectionByTitle(t, secs, titleContext)
	if len(sec.Items) != 1 {
		t.Fatalf("items = %d, want 1", len(sec.Items))
	}
	if got := sec.Items[0].Body; got != "staging" {
		t.Errorf("body = %q, want staging", got)
	}
	if got := sec.Items[0].Kind; got != "context" {
		t.Errorf("kind = %q", got)
	}
	if len(r.argv()) != 0 {
		t.Errorf("an explicit context needs no probe, got %v", r.argv())
	}
}

func TestParseCollectors(t *testing.T) {
	cases := []struct {
		in   string
		want []string
	}{
		{"", DefaultCollectors()},
		{"   ", DefaultCollectors()},
		{"nodes,pods", []string{"pods", "nodes"}},
		{"pods, pods ,events", []string{"pods", "events"}},
		{"PODS", []string{"pods"}},
		{"pods,bogus", []string{"pods", "bogus"}},
	}
	for _, c := range cases {
		if got := parseCollectors(c.in); !slices.Equal(got, c.want) {
			t.Errorf("parseCollectors(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

type fakeBuildContext struct {
	params   map[string]string
	settings map[string]any
}

func (f fakeBuildContext) Params() map[string]string           { return f.params }
func (f fakeBuildContext) Home() string                        { return "" }
func (f fakeBuildContext) Role() string                        { return "" }
func (f fakeBuildContext) Credentials() plugin.CredentialStore { return nil }

func (f fakeBuildContext) Settings(namespace string) map[string]any {
	if namespace != SignalName {
		return nil
	}
	return f.settings
}

func withSelection(t *testing.T, name string) {
	t.Helper()
	prev := shared.selected()
	if err := shared.Switch(context.Background(), name); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = shared.Switch(context.Background(), prev) })
}

func TestResolveContextPrecedence(t *testing.T) {
	t.Run("param wins", func(t *testing.T) {
		withSelection(t, "from-switch")
		s, _ := resolve(fakeBuildContext{
			params:   map[string]string{"context": "from-param"},
			settings: map[string]any{"context": "from-settings"},
		})
		if s.context != "from-param" {
			t.Fatalf("context = %q", s.context)
		}
	})

	t.Run("in-process selection beats settings", func(t *testing.T) {
		withSelection(t, "from-switch")
		s, _ := resolve(fakeBuildContext{
			params:   map[string]string{},
			settings: map[string]any{"context": "from-settings"},
		})
		if s.context != "from-switch" {
			t.Fatalf("context = %q", s.context)
		}
	})

	t.Run("settings are the last explicit source", func(t *testing.T) {
		withSelection(t, "")
		s, _ := resolve(fakeBuildContext{
			params:   map[string]string{},
			settings: map[string]any{"context": "from-settings"},
		})
		if s.context != "from-settings" {
			t.Fatalf("context = %q", s.context)
		}
	})

	t.Run("nothing set defers to kubeconfig", func(t *testing.T) {
		withSelection(t, "")
		s, _ := resolve(fakeBuildContext{params: map[string]string{}})
		if s.context != "" {
			t.Fatalf("context = %q, want empty so kubectl reads the kubeconfig", s.context)
		}
	})
}

func TestResolveOptionsFromSettingsAndParams(t *testing.T) {
	withSelection(t, "")
	s, opts := resolve(fakeBuildContext{
		params: map[string]string{"limit": "7", "since": "15m"},
		settings: map[string]any{
			"binary":            "/opt/bin/kubectl",
			"kinds":             "pods,nodes",
			"limit":             "50",
			"since":             "6h",
			"restart_threshold": 9,
			"timeout":           "45s",
			"namespace":         "payments",
		},
	})

	if opts.Binary != "/opt/bin/kubectl" {
		t.Errorf("binary = %q", opts.Binary)
	}
	if !slices.Equal(opts.Collectors, []string{"pods", "nodes"}) {
		t.Errorf("collectors = %v", opts.Collectors)
	}
	if opts.Limit != 7 {
		t.Errorf("limit = %d, want the param to win", opts.Limit)
	}
	if opts.Since != 15*time.Minute {
		t.Errorf("since = %s, want the param to win", opts.Since)
	}
	if opts.RestartThreshold != 9 {
		t.Errorf("restarts = %d", opts.RestartThreshold)
	}
	if opts.Timeout != 45*time.Second {
		t.Errorf("timeout = %s", opts.Timeout)
	}
	if s.namespace != "payments" || s.allNamespaces {
		t.Errorf("scope = %+v", s)
	}
}

func TestResolveDefaultsToAllNamespaces(t *testing.T) {
	withSelection(t, "")
	s, opts := resolve(fakeBuildContext{params: map[string]string{}})
	if !s.allNamespaces || s.namespace != "" {
		t.Errorf("scope = %+v, want all namespaces", s)
	}
	if !slices.Equal(opts.Collectors, DefaultCollectors()) {
		t.Errorf("collectors = %v", opts.Collectors)
	}
	if opts.Limit != DefaultLimit || opts.Since != DefaultSince {
		t.Errorf("opts = %+v", opts)
	}
}

const newlyBrokenPod = `{"items":[{
  "metadata": {"name": "newly-broken-abc12", "namespace": "payments", "creationTimestamp": "2026-08-05T11:59:00Z"},
  "spec": {"nodeName": "ip-10-0-1-14"},
  "status": {
    "phase": "Running",
    "conditions": [{"type": "Ready", "status": "False"}],
    "containerStatuses": [{"name": "c", "ready": false, "restartCount": 1,
      "state": {"waiting": {"reason": "CrashLoopBackOff", "message": "back-off"}}}]
  }
}]}`

func TestStreamEmitsOnlyNewProblemsAndOnlyOnce(t *testing.T) {
	r := newFakeRunner(t)
	src := NewActive(NewSignal(r, scope{allNamespaces: true}, testOptions()), 20*time.Millisecond, stream.NewState(nil))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ch, err := src.Stream(ctx)
	if err != nil {
		t.Fatal(err)
	}

	select {
	case ev := <-ch:
		t.Fatalf("baseline poll must emit nothing, got %v", titles(ev.Section.Items))
	case <-time.After(200 * time.Millisecond):
	}

	r.set("pods", []byte(newlyBrokenPod))

	select {
	case ev, ok := <-ch:
		if !ok {
			t.Fatal("stream closed before the new problem arrived")
		}
		if ev.Section.Err != nil {
			t.Fatalf("emission errored: %v", ev.Section.Err)
		}
		if got := titles(ev.Section.Items); !slices.Equal(got, []string{"payments/newly-broken-abc12"}) {
			t.Fatalf("emitted %v, want only the new pod", got)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the new problem was never emitted")
	}

	select {
	case ev := <-ch:
		t.Fatalf("an unchanged problem must not be re-emitted, got %v", titles(ev.Section.Items))
	case <-time.After(200 * time.Millisecond):
	}
}

func TestStreamReportsATotallyUnreachableCluster(t *testing.T) {
	r := newFakeRunner(t)
	down := errors.New("dial tcp: connection refused")
	for _, resource := range []string{"pods", "events", "nodes", workloadResources} {
		r.errs[resource] = down
	}
	src := NewActive(NewSignal(r, scope{allNamespaces: true}, testOptions()), 20*time.Millisecond, stream.NewState(nil))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	ch, err := src.Stream(ctx)
	if err != nil {
		t.Fatal(err)
	}
	ev, ok := <-ch
	if !ok {
		t.Fatal("stream closed without emitting")
	}
	if ev.Section.Err == nil {
		t.Fatal("every collector failed; the emission must carry the error")
	}
}

func TestItemKeyDistinguishesEventRepeats(t *testing.T) {
	base := plugin.Item{Kind: "event", Meta: map[string]string{"object": "Pod/a", "reason": "BackOff", "count": "3"}}
	repeat := plugin.Item{Kind: "event", Meta: map[string]string{"object": "Pod/a", "reason": "BackOff", "count": "4"}}
	if itemKey(base) == itemKey(repeat) {
		t.Error("a growing repeat count is new news and must produce a new key")
	}

	pod := plugin.Item{Kind: "pod", Title: "ns/p", Meta: map[string]string{"reason": "CrashLoopBackOff"}}
	same := plugin.Item{Kind: "pod", Title: "ns/p", Meta: map[string]string{"reason": "CrashLoopBackOff"}}
	if itemKey(pod) != itemKey(same) {
		t.Error("an unchanged pod problem must keep its key")
	}
}

func TestRegistration(t *testing.T) {
	d, ok := plugin.Lookup(PluginID)
	if !ok || d.Signal != SignalName {
		t.Fatalf("lookup = %+v ok=%v", d, ok)
	}
	for _, capability := range []plugin.Capability{plugin.CapQuery, plugin.CapStream, plugin.CapCacheable} {
		if !slices.Contains(d.Capabilities, capability) {
			t.Errorf("missing capability %q", capability)
		}
	}
	if slices.Contains(d.Capabilities, plugin.CapAction) {
		t.Error("the kubectl signal is read-only and must not advertise CapAction")
	}
	if !slices.Contains(d.SettingsNamespaces, SignalName) {
		t.Errorf("settings namespaces = %v", d.SettingsNamespaces)
	}
	if _, ok := glyph.Named(GlyphID); !ok {
		t.Fatal("glyph missing")
	}
	if len(plugin.ActionsFor(SignalName)) != 0 {
		t.Error("the kubectl signal must register no actions")
	}
}

func TestRegisteredQueryParams(t *testing.T) {
	specs := plugin.QueryParams(SignalName)
	if len(specs) == 0 {
		t.Fatal("no query params registered")
	}
	keys := make([]string, 0, len(specs))
	for _, s := range specs {
		keys = append(keys, s.Key)
	}
	for _, want := range []string{"context", "namespace", "what", "since", "limit", "restarts", "interval"} {
		if !slices.Contains(keys, want) {
			t.Errorf("param %q not registered; have %v", want, keys)
		}
	}
}

func TestSwitchContextReachesTheArgv(t *testing.T) {
	withSelection(t, "")
	want := "mino-shared-provider-ctx"
	if err := plugin.SwitchContext(context.Background(), ContextTool, want); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = shared.Switch(context.Background(), "") })

	if got := shared.selected(); got != want {
		t.Fatalf("selected = %q, want %q", got, want)
	}
	if s, _ := resolve(fakeBuildContext{params: map[string]string{}}); s.context != want {
		t.Fatalf("a switched context must reach the argv, got %q", s.context)
	}
}

func TestSwitchDoesNotRequireKubectlBinary(t *testing.T) {
	withSelection(t, "")
	if err := shared.Switch(context.Background(), "definitely-not-a-real-context-xyz"); err != nil {
		t.Fatalf("Switch = %v", err)
	}
	if shared.selected() != "definitely-not-a-real-context-xyz" {
		t.Fatalf("selected = %q", shared.selected())
	}
}

type contextFixture struct {
	Context string `json:"context"`
	Signal  string `json:"signal"`
	Title   string `json:"title"`
	Kind    string `json:"kind"`
}

func TestFixtureContextDrivesFetchAndStatus(t *testing.T) {
	var fx contextFixture
	if err := json.Unmarshal(fixture(t, "context.json"), &fx); err != nil {
		t.Fatal(err)
	}
	withSelection(t, fx.Context)
	shared.grade(0)
	t.Cleanup(func() { shared.grade(0) })

	opts := testOptions()
	opts.Collectors = []string{CollectorContext}
	s, _ := resolve(fakeBuildContext{params: map[string]string{}})

	secs, err := NewSignal(newFakeRunner(t), s, opts).Fetch(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(secs) != 1 || len(secs[0].Items) != 1 {
		t.Fatalf("sections = %#v", secs)
	}
	if secs[0].Signal != fx.Signal {
		t.Errorf("signal = %q, want %q", secs[0].Signal, fx.Signal)
	}
	item := secs[0].Items[0]
	if item.Kind != fx.Kind || item.Body != fx.Context {
		t.Fatalf("item = %#v want kind=%q body=%q", item, fx.Kind, fx.Context)
	}

	contrib := StatusContribution()
	if contrib.Info == nil || contrib.Info() != ContextTool {
		t.Fatalf("Info want %q", ContextTool)
	}
	if g, sev := contrib.Status(); g != glyph.StatusOK() || sev != glyph.SeverityPositive {
		t.Fatalf("healthy status = %q sev=%v", g, sev)
	}
}

func TestStatusContributionWarnsWhenTheClusterIsUnhealthy(t *testing.T) {
	withSelection(t, "some-context")
	shared.grade(3)
	t.Cleanup(func() { shared.grade(0) })

	if g, sev := StatusContribution().Status(); g != glyph.StatusWarn() || sev != glyph.SeverityNegative {
		t.Fatalf("unhealthy status = %q sev=%v", g, sev)
	}
}

func TestFetchGradesTheStatusChip(t *testing.T) {
	withSelection(t, "graded")
	shared.grade(0)
	t.Cleanup(func() { shared.grade(0) })

	if _, err := NewSignal(newFakeRunner(t), scope{allNamespaces: true}, testOptions()).Fetch(context.Background()); err != nil {
		t.Fatal(err)
	}
	unhealthy, graded := shared.verdict()
	if !graded || unhealthy == 0 {
		t.Fatalf("verdict = %d graded=%v, want the fixture problems counted", unhealthy, graded)
	}
}

func TestSeedDirectives(t *testing.T) {
	for _, d := range []struct{ name, body string }{
		{"kubectl-context", ExampleDirective},
		{"kubectl-health", HealthDirective},
	} {
		for _, line := range []string{"name: " + d.name, "type: query", "signal: " + SignalName} {
			if !strings.Contains(d.body, line) {
				t.Errorf("%s: %q missing from\n%s", d.name, line, d.body)
			}
		}
	}
}
