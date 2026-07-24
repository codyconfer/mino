package plugin

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type boomProvider struct{ tool string }

func (p boomProvider) Tool() string { return p.tool }
func (p boomProvider) Switch(context.Context, string) error {
	return errors.New("boom")
}
func (p boomProvider) Current(context.Context) (string, bool, error) {
	return "", false, nil
}

type okProvider struct {
	tool string
	cur  string
}

func (p *okProvider) Tool() string { return p.tool }
func (p *okProvider) Switch(_ context.Context, name string) error {
	p.cur = name
	return nil
}
func (p *okProvider) Current(context.Context) (string, bool, error) {
	if p.cur == "" {
		return "", false, nil
	}
	return p.cur, true, nil
}

func TestApplyRoleContextsContinuesAndJoins(t *testing.T) {
	// Isolate providers for this test.
	ctxMu.Lock()
	prevP, prevL := providers, lastSwitch
	providers = map[string]ContextProvider{}
	lastSwitch = map[string]string{}
	ctxMu.Unlock()
	t.Cleanup(func() {
		ctxMu.Lock()
		providers, lastSwitch = prevP, prevL
		ctxMu.Unlock()
	})

	ok := &okProvider{tool: "alpha"}
	RegisterContextProvider(ok)
	RegisterContextProvider(boomProvider{tool: "beta"})

	err := ApplyRoleContexts(context.Background(), map[string]string{
		"beta":  "x",
		"alpha": "prod",
		"zeta":  "nope", // unknown
	})
	if err == nil {
		t.Fatal("expected joined errors")
	}
	msg := err.Error()
	if !strings.Contains(msg, "beta") || !strings.Contains(msg, "zeta") {
		t.Fatalf("error missing tools: %v", err)
	}
	if ok.cur != "prod" {
		t.Fatalf("alpha should still switch despite other failures; cur=%q", ok.cur)
	}
}
