package gcx

import (
	"reflect"
	"strings"
	"testing"
)

func TestMapIncidentsJSONAcceptsBothEnvelopes(t *testing.T) {
	member := `{"incidentID":"incident-1","title":"t","status":"active","severity":"high","incidentURL":"https://x/1"}`
	legacy, err := MapIncidentsJSON([]byte(`{"incidents":[` + member + `]}`))
	if err != nil {
		t.Fatal(err)
	}
	previews, err := MapIncidentsJSON([]byte(`{"incidentPreviews":[` + member + `]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(legacy, previews) {
		t.Fatalf("envelope spelling changed the section:\n%#v\n%#v", legacy, previews)
	}
}

func TestNormalizeSeverityFallback(t *testing.T) {
	got := incidentWire{IncidentID: "i", SeverityLabel: "critical"}.normalize("")
	if got.Severity != "critical" {
		t.Fatalf("severity = %q", got.Severity)
	}
	got = incidentWire{IncidentID: "i", Severity: "high", SeverityLabel: "critical"}.normalize("")
	if got.Severity != "high" {
		t.Fatalf("severity field should win: %q", got.Severity)
	}
}

func TestNormalizeSynthesizesURL(t *testing.T) {
	got := incidentWire{IncidentID: "incident-9"}.normalize("myorg.grafana.net")
	want := "https://myorg.grafana.net/a/grafana-irm-app/incidents/incident-9"
	if got.URL != want {
		t.Fatalf("URL = %q want %q", got.URL, want)
	}
	got = incidentWire{IncidentID: "incident-9", IncidentURL: "https://given/x"}.normalize("myorg.grafana.net")
	if got.URL != "https://given/x" {
		t.Fatalf("wire URL should win: %q", got.URL)
	}
	bare := incidentWire{IncidentID: "incident-9"}
	if bare.normalize("").URL != "" {
		t.Fatal("no stack means no synthesized URL")
	}
}

func TestNormalizeParsesCreatedTime(t *testing.T) {
	got := incidentWire{CreatedTime: "2026-07-24T15:04:05Z"}.normalize("")
	if got.Created.IsZero() || got.Created.Year() != 2026 {
		t.Fatalf("created = %v", got.Created)
	}
	bad := incidentWire{CreatedTime: "not-a-time"}
	if !bad.normalize("").Created.IsZero() {
		t.Fatal("unparseable createdTime should stay zero")
	}
}

func TestMapIncidentsJSONMalformed(t *testing.T) {
	_, err := MapIncidentsJSON([]byte("{"))
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "gcx incidents fixture") {
		t.Fatalf("error = %v", err)
	}
}

func TestSectionFromIncidentsEmpty(t *testing.T) {
	sec := sectionFromIncidents(nil)
	if sec.Signal != SignalName || sec.Title != "incidents" {
		t.Fatalf("section = %#v", sec)
	}
	if sec.Items == nil || len(sec.Items) != 0 {
		t.Fatalf("items = %#v", sec.Items)
	}
}

func TestSectionMarksDrills(t *testing.T) {
	sec := sectionFromIncidents([]incident{{ID: "i", Drill: true}, {ID: "j"}})
	if sec.Items[0].Meta["drill"] != "true" {
		t.Fatalf("drill meta = %#v", sec.Items[0].Meta)
	}
	if _, ok := sec.Items[1].Meta["drill"]; ok {
		t.Fatal("non-drill should carry no drill meta")
	}
	if sec.Items[0].Title != "i" {
		t.Fatalf("empty title should fall back to the id: %q", sec.Items[0].Title)
	}
}
