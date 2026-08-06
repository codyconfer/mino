package gcx

import (
	"strings"
	"testing"

	"github.com/codyconfer/mino/plugin"
)

func TestBuildQueryStatusViewStaysOffline(t *testing.T) {
	clearEnv(t)
	pinStack(t, "")
	q, err := BuildQuery(newFakeBC(map[string]string{"view": ViewStatus}, nil))
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := q.(Signal); !ok {
		t.Fatalf("query = %T want Signal", q)
	}
}

func TestBuildQueryDefaultsToIncidents(t *testing.T) {
	clearEnv(t)
	pinStack(t, "myorg.grafana.net")
	bc := newFakeBC(nil, nil)
	bc.store[TokenKey] = plugin.Credential{AccessToken: "glsa_x"}

	q, err := BuildQuery(bc)
	if err != nil {
		t.Fatal(err)
	}
	inc, ok := q.(Incidents)
	if !ok {
		t.Fatalf("query = %T want Incidents", q)
	}
	if inc.Client.Stack != "myorg.grafana.net" || inc.Client.Token != "glsa_x" {
		t.Fatalf("client = %#v", inc.Client)
	}
	if inc.Query.Status != StatusActive || inc.Query.Limit != defaultLimit || inc.Query.IncludeDrills {
		t.Fatalf("query = %#v", inc.Query)
	}
}

func TestBuildQueryUnknownView(t *testing.T) {
	clearEnv(t)
	_, err := BuildQuery(newFakeBC(map[string]string{"view": "bogus"}, nil))
	if err == nil {
		t.Fatal("expected an error")
	}
	hint := hintOf(err)
	if !strings.Contains(hint, ViewIncidents) || !strings.Contains(hint, ViewStatus) {
		t.Fatalf("hint = %q", hint)
	}
}

func TestBuildQueryIncidentsNeedsAStack(t *testing.T) {
	clearEnv(t)
	pinStack(t, "")
	bc := newFakeBC(map[string]string{"view": ViewIncidents}, nil)
	bc.store[TokenKey] = plugin.Credential{AccessToken: "glsa_x"}
	_, err := BuildQuery(bc)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(hintOf(err), "plugins.gcx.stack") {
		t.Fatalf("hint = %q", hintOf(err))
	}
}

func TestBuildQueryIncidentsNeedsAToken(t *testing.T) {
	clearEnv(t)
	pinStack(t, "myorg.grafana.net")
	_, err := BuildQuery(newFakeBC(map[string]string{"view": ViewIncidents}, nil))
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(hintOf(err), "mino login gcx") {
		t.Fatalf("hint = %q", hintOf(err))
	}
}

func TestBuildQueryParamsBeatSettings(t *testing.T) {
	clearEnv(t)
	pinStack(t, "from-context.grafana.net")
	settings := map[string]any{
		"stack":  "from-setting.grafana.net",
		"limit":  5,
		"status": "resolved",
		"drills": true,
	}
	bc := newFakeBC(map[string]string{
		"stack":  "from-param.grafana.net",
		"limit":  "7",
		"status": StatusActive,
		"drills": "false",
	}, settings)
	bc.store[TokenKey] = plugin.Credential{AccessToken: "glsa_x"}

	q, err := BuildQuery(bc)
	if err != nil {
		t.Fatal(err)
	}
	inc := q.(Incidents)
	if inc.Client.Stack != "from-param.grafana.net" {
		t.Fatalf("stack = %q", inc.Client.Stack)
	}
	if inc.Query.Limit != 7 || inc.Query.Status != StatusActive || inc.Query.IncludeDrills {
		t.Fatalf("query = %#v", inc.Query)
	}
}

func TestBuildQuerySettingsWhenNoParams(t *testing.T) {
	clearEnv(t)
	pinStack(t, "")
	settings := map[string]any{"stack": "from-setting.grafana.net", "limit": 50, "drills": true}
	bc := newFakeBC(nil, settings)
	bc.store[TokenKey] = plugin.Credential{AccessToken: "glsa_x"}

	q, err := BuildQuery(bc)
	if err != nil {
		t.Fatal(err)
	}
	inc := q.(Incidents)
	if inc.Client.Stack != "from-setting.grafana.net" || inc.Query.Limit != 50 || !inc.Query.IncludeDrills {
		t.Fatalf("built = %#v %#v", inc.Client, inc.Query)
	}
}

func TestIncidentQueryClampsLimit(t *testing.T) {
	cases := []struct {
		param string
		want  int
	}{
		{"9999", maxLimit},
		{"0", defaultLimit},
		{"-3", defaultLimit},
		{"", defaultLimit},
		{"garbage", defaultLimit},
	}
	for _, tc := range cases {
		got := incidentQueryFrom(map[string]string{"limit": tc.param}, nil).Limit
		if got != tc.want {
			t.Fatalf("limit %q = %d want %d", tc.param, got, tc.want)
		}
	}
}

func TestBuildQueryStatusViewNeedsNoHost(t *testing.T) {
	clearEnv(t)
	pinStack(t, "")
	q, err := BuildQuery(bareBuildContext{})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := q.(Signal); !ok {
		t.Fatalf("query = %T", q)
	}
}

type bareBuildContext struct{}

func (bareBuildContext) Params() map[string]string { return map[string]string{"view": ViewStatus} }
func (bareBuildContext) Home() string              { return "" }
func (bareBuildContext) Role() string              { return "" }

func TestBuildQueryIncidentsRejectsBareBuildContext(t *testing.T) {
	clearEnv(t)
	_, err := BuildQuery(bareIncidentsContext{})
	if err == nil {
		t.Fatal("expected an error without a mino host")
	}
}

type bareIncidentsContext struct{}

func (bareIncidentsContext) Params() map[string]string { return map[string]string{} }
func (bareIncidentsContext) Home() string              { return "" }
func (bareIncidentsContext) Role() string              { return "" }

func TestSplitList(t *testing.T) {
	if got := splitList(" a , ,b "); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Fatalf("splitList = %#v", got)
	}
	if splitList("") != nil {
		t.Fatal("empty input should produce nil")
	}
}
