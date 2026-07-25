package plugin_test

import (
	"context"
	"testing"

	"github.com/codyconfer/munin/plugin"
)

type testQuery struct{ name string }

func (q testQuery) Name() string { return q.name }
func (q testQuery) Fetch(context.Context) ([]plugin.Section, error) {
	return []plugin.Section{{Signal: q.name, Title: q.name}}, nil
}

type testBuildCtx struct {
	params       map[string]string
	home, role   string
	access, scp  string
	tokenService string
}

func (c testBuildCtx) Params() map[string]string { return c.params }
func (c testBuildCtx) Home() string              { return c.home }
func (c testBuildCtx) Role() string              { return c.role }
func (c testBuildCtx) GetToken(_ context.Context, service string) (string, string, bool, error) {
	if service != c.tokenService {
		return "", "", false, nil
	}
	return c.access, c.scp, c.access != "", nil
}

func TestRegisterSignalWiresBuilders(t *testing.T) {
	const id = "test.public.sdk"
	const signal = "publicsdk"
	if _, ok := plugin.Lookup(id); ok {
		t.Skip("already registered")
	}
	plugin.RegisterSignal(plugin.Descriptor{
		ID:           id,
		Kind:         plugin.KindSignal,
		Signal:       signal,
		Capabilities: []plugin.Capability{plugin.CapQuery},
	}, plugin.Builders{
		Query: func(bc plugin.BuildContext) (plugin.Query, error) {
			return testQuery{name: bc.Home() + ":" + signal}, nil
		},
	})
	if !plugin.HasBuilder(signal) {
		t.Fatal("expected query builder")
	}
	if !plugin.BuilderSignals()[signal] {
		t.Fatal("expected BuilderSignals entry")
	}
	q, err := plugin.BuildQuery(signal, testBuildCtx{home: "h", role: "r"})
	if err != nil {
		t.Fatal(err)
	}
	if q.Name() != "h:"+signal {
		t.Fatalf("name = %q", q.Name())
	}
}

func TestTokenSourceOptional(t *testing.T) {
	bc := testBuildCtx{access: "tok", scp: "irm", tokenService: "gcx"}
	ts, ok := any(bc).(plugin.TokenSource)
	if !ok {
		t.Fatal("expected TokenSource")
	}
	tok, scope, present, err := ts.GetToken(context.Background(), "gcx")
	if err != nil || !present || tok != "tok" || scope != "irm" {
		t.Fatalf("GetToken = %q %q %v %v", tok, scope, present, err)
	}
}
