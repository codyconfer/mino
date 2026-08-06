// Package gcx is Lane C (Grafana Cloud). There is no gcx binary — HTTP APIs only.
//
// Auth: sealed token store key TokenKey holds a Grafana stack service-account
// token (Bearer glsa_…) for IRM. Context name is the stack host (e.g.
// myorg.grafana.net). See SPIKE.md for the auth matrix, the unverified IRM wire
// contract, and the implementation notes.
package gcx

import (
	"context"

	"github.com/codyconfer/viewkit/glyph"

	"github.com/codyconfer/mino/plugin"
)

const (
	PluginID    = "external.gcx"
	SignalName  = "gcx"
	GlyphID     = "external.gcx"
	ContextTool = "gcx"
	// TokenKey is the sealed store service name for the stack SA token.
	TokenKey = "gcx"
)

// Vertical is the locked first query surface.
const Vertical = "irm-incidents"

type tokenSourceLookup struct{ src plugin.TokenSource }

func (t tokenSourceLookup) Get(ctx context.Context, service string) (accessToken, scope string, ok bool, err error) {
	if t.src == nil {
		return "", "", false, nil
	}
	return t.src.GetToken(ctx, service)
}

func tokenLookupFrom(bc plugin.BuildContext) TokenLookup {
	if ts, ok := bc.(plugin.TokenSource); ok {
		return tokenSourceLookup{src: ts}
	}
	return nil
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
	Get(ctx context.Context, service string) (accessToken, scope string, ok bool, err error)
}

// Signal is the offline-safe status query (view=status).
type Signal struct {
	Tokens TokenLookup
}

// NewSignal wires the sealed token store for auth presence checks.
func NewSignal(tokens TokenLookup) Signal {
	return Signal{Tokens: tokens}
}

func (Signal) Name() string { return SignalName }

func (s Signal) Fetch(ctx context.Context) ([]plugin.Section, error) {
	authBody := "sealed key gcx: unset (stack SA token assumed; see SPIKE.md)"
	if s.Tokens != nil {
		if tok, scope, ok, err := s.Tokens.Get(ctx, TokenKey); err == nil && ok && tok != "" {
			authBody = "sealed key gcx: present"
			if scope != "" {
				authBody += " scope=" + scope
			}
		}
	}
	stack := "(unset)"
	if n, ok, _ := shared.Current(ctx); ok && n != "" {
		stack = n
	}
	return []plugin.Section{{
		Signal: SignalName,
		Title:  "gcx",
		Items: []plugin.Item{
			{Kind: "auth", Title: "auth", Body: authBody},
			{Kind: "context", Title: "stack", Body: stack},
			{Kind: "info", Title: "vertical", Body: Vertical + " (live via view=incidents)"},
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
