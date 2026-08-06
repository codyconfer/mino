package gcx

import (
	"context"
	"encoding/json"
	"time"

	"github.com/codyconfer/mino/external/plugins/internal/errx"
	"github.com/codyconfer/mino/plugin"
)

type incident struct {
	ID       string
	Title    string
	Status   string
	Severity string
	URL      string
	Created  time.Time
	Drill    bool
}

type incidentWire struct {
	IncidentID    string `json:"incidentID"`
	Title         string `json:"title"`
	Status        string `json:"status"`
	Severity      string `json:"severity"`
	SeverityLabel string `json:"severityLabel"`
	IncidentURL   string `json:"incidentURL"`
	CreatedTime   string `json:"createdTime"`
	IsDrill       bool   `json:"isDrill"`
}

type incidentsEnvelope struct {
	Incidents        []incidentWire `json:"incidents"`
	IncidentPreviews []incidentWire `json:"incidentPreviews"`
}

func (e incidentsEnvelope) list() []incidentWire {
	if len(e.IncidentPreviews) > 0 {
		return e.IncidentPreviews
	}
	return e.Incidents
}

type incidentEnvelope struct {
	Incident incidentWire `json:"incident"`
}

func (w incidentWire) normalize(stack string) incident {
	severity := w.Severity
	if severity == "" {
		severity = w.SeverityLabel
	}
	url := w.IncidentURL
	if url == "" && stack != "" && w.IncidentID != "" {
		url = "https://" + stack + "/a/grafana-irm-app/incidents/" + w.IncidentID
	}
	created, _ := time.Parse(time.RFC3339, w.CreatedTime)
	return incident{
		ID:       w.IncidentID,
		Title:    w.Title,
		Status:   w.Status,
		Severity: severity,
		URL:      url,
		Created:  created,
		Drill:    w.IsDrill,
	}
}

func normalizeAll(wires []incidentWire, stack string) []incident {
	out := make([]incident, 0, len(wires))
	for _, w := range wires {
		out = append(out, w.normalize(stack))
	}
	return out
}

func sectionFromIncidents(incs []incident) plugin.Section {
	items := make([]plugin.Item, 0, len(incs))
	for _, inc := range incs {
		title := inc.Title
		if title == "" {
			title = inc.ID
		}
		meta := map[string]string{
			"id":       inc.ID,
			"status":   inc.Status,
			"severity": inc.Severity,
		}
		if inc.Drill {
			meta["drill"] = "true"
		}
		items = append(items, plugin.Item{
			Kind:      "incident",
			Title:     title,
			Subtitle:  inc.Status,
			Body:      inc.Severity,
			URL:       inc.URL,
			Timestamp: inc.Created,
			Meta:      meta,
		})
	}
	return plugin.Section{
		Signal: SignalName,
		Title:  "incidents",
		Items:  items,
	}
}

// MapIncidentsJSON maps a recorded IRM incident list into a mino section.
// Offline-testable query surface for vertical irm-incidents (no network).
func MapIncidentsJSON(raw []byte) (plugin.Section, error) {
	var env incidentsEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return plugin.Section{}, errx.Wrap(err, "gcx incidents fixture")
	}
	return sectionFromIncidents(normalizeAll(env.list(), "")), nil
}

// Incidents is the live IRM incident query (view=incidents).
type Incidents struct {
	Client *Client
	Query  IncidentQuery
}

func (Incidents) Name() string { return SignalName }

func (q Incidents) Fetch(ctx context.Context) ([]plugin.Section, error) {
	incs, err := q.Client.QueryIncidents(ctx, q.Query)
	if err != nil {
		return nil, err
	}
	sec := sectionFromIncidents(incs)
	sec.Meta = map[string]string{"stack": q.Client.Stack, "view": ViewIncidents}
	return []plugin.Section{sec}, nil
}
