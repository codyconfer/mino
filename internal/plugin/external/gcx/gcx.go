// Package gcx is Lane C (Grafana Cloud). There is no gcx binary — HTTP APIs only.
//
// Auth assumption (C-0): sealed token store key TokenKey holds a Grafana stack
// service-account token (Bearer glsa_…) for IRM. Context name is the stack host
// (e.g. myorg.grafana.net). See SPIKE.md for the full auth matrix and deferrals.
package gcx

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/munin/internal/auth"
	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/plugin"
	"github.com/codyconfer/munin/internal/signals"
)

const (
	PluginID    = "external.gcx"
	SignalName  = "gcx"
	GlyphID     = "external.gcx"
	ContextTool = "gcx"
	// TokenKey is the sealed store service name for the stack SA token.
	TokenKey = "gcx"
)

// Vertical is the C-0 locked first query surface (fixture-mappable offline).
const Vertical = "irm-incidents"

func init() {
	plugin.Register(plugin.Descriptor{
		ID:           PluginID,
		Kind:         plugin.KindSignal,
		Signal:       SignalName,
		Capabilities: []plugin.Capability{plugin.CapQuery, plugin.CapAction},
	})
	glyph.Register(GlyphID, glyph.Variants{Nerd: "󰡉", Uni: "☁", ASCII: "gx"})
	plugin.RegisterContextProvider(shared)
	for _, a := range KnownActions() {
		name := a.Name()
		plugin.RegisterAction(SignalName, name, a.Run)
	}
}

var shared = &provider{}

type provider struct{ cur string }

func (p *provider) Tool() string { return ContextTool }

func (p *provider) Switch(_ context.Context, name string) error {
	p.cur = name
	return nil
}

func (p *provider) Current(context.Context) (string, bool, error) {
	if p.cur == "" {
		return "", false, nil
	}
	return p.cur, true, nil
}

// TokenLookup is the narrow store face used for auth-status probes (tests inject fakes).
type TokenLookup interface {
	Get(ctx context.Context, service string) (auth.Credential, bool, error)
}

// Signal is the offline-safe status query. Live IRM HTTP is Lane C follow-up.
type Signal struct {
	Tokens TokenLookup
}

// NewSignal wires the sealed token store for auth presence checks.
func NewSignal(tokens TokenLookup) Signal {
	return Signal{Tokens: tokens}
}

func (Signal) Name() string { return SignalName }

func (s Signal) Fetch(ctx context.Context) ([]signals.Section, error) {
	authBody := "sealed key gcx: unset (stack SA token assumed; see SPIKE.md)"
	if s.Tokens != nil {
		if c, ok, err := s.Tokens.Get(ctx, TokenKey); err == nil && ok && c.AccessToken != "" {
			authBody = "sealed key gcx: present"
			if c.Scope != "" {
				authBody += " scope=" + c.Scope
			}
		}
	}
	stack := "(unset)"
	if n, ok, _ := shared.Current(ctx); ok && n != "" {
		stack = n
	}
	return []signals.Section{{
		Signal: SignalName,
		Title:  "gcx",
		Items: []signals.Item{
			{Kind: "auth", Title: "auth", Body: authBody},
			{Kind: "context", Title: "stack", Body: stack},
			{Kind: "info", Title: "vertical", Body: Vertical + " (fixture mapper ready; no live HTTP)"},
		},
	}}, nil
}

// StatusContribution exposes auth/context health for the status strip.
func StatusContribution() glyph.StatusContribution {
	return glyph.StatusContribution{
		BrandGlyph: glyph.ResolveID(GlyphID),
		Info:       func() string { return ContextTool },
		Status: func() (string, glyph.Severity) {
			name, ok, _ := shared.Current(context.Background())
			if !ok || name == "" {
				return glyph.StatusMuted(), glyph.SeverityNeutral
			}
			return glyph.StatusOK(), glyph.SeverityPositive
		},
	}
}

// StubAction reserves CapAction names without a live Grafana client.
type StubAction struct{ NameStr string }

func (a StubAction) Name() string { return a.NameStr }

func (a StubAction) Run(context.Context, map[string]string) error {
	return errs.New(errs.KindConfig, "gcx action stub: no live Grafana Cloud client").
		WithHint("Lane C write path deferred; see SPIKE.md")
}

// KnownActions are the reserved write-side names for Lane C (host CapAction registry).
func KnownActions() []plugin.Action {
	return []plugin.Action{
		StubAction{NameStr: "declare-incident"},
		StubAction{NameStr: "add-activity"},
	}
}

// incidentWire is a minimal IRM-shaped list fixture (subset of incident fields).
type incidentWire struct {
	IncidentID  string `json:"incidentID"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	Severity    string `json:"severity"`
	IncidentURL string `json:"incidentURL"`
}

type incidentsEnvelope struct {
	Incidents []incidentWire `json:"incidents"`
}

// MapIncidentsJSON maps a recorded IRM-ish incident list into a munin section.
// Offline-testable query surface for vertical irm-incidents (no network).
func MapIncidentsJSON(raw []byte) (signals.Section, error) {
	var env incidentsEnvelope
	if err := json.Unmarshal(raw, &env); err != nil {
		return signals.Section{}, fmt.Errorf("gcx incidents fixture: %w", err)
	}
	items := make([]signals.Item, 0, len(env.Incidents))
	for _, inc := range env.Incidents {
		title := inc.Title
		if title == "" {
			title = inc.IncidentID
		}
		items = append(items, signals.Item{
			Kind:     "incident",
			Title:    title,
			Subtitle: inc.Status,
			Body:     inc.Severity,
			URL:      inc.IncidentURL,
			Meta: map[string]string{
				"id":       inc.IncidentID,
				"status":   inc.Status,
				"severity": inc.Severity,
			},
		})
	}
	return signals.Section{
		Signal: SignalName,
		Title:  "incidents",
		Items:  items,
	}, nil
}

// ExampleDirective for Lane D / verify.
const ExampleDirective = `name: gcx-status
signal: gcx
params: {}
`
