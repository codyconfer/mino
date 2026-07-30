package plugin

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

type Context struct {
	Tool string
	Name string
}

type ContextProvider interface {
	Tool() string
	Switch(ctx context.Context, name string) error
	Current(ctx context.Context) (name string, ok bool, err error)
}

var (
	ctxMu      sync.RWMutex
	providers  = map[string]ContextProvider{}
	lastSwitch = map[string]string{}
)

// RegisterContextProvider installs a live provider for its tool. Like every
// other registration site it is first-write-wins: the incumbent keeps the tool
// and the later provider is skipped with a diagnostic, so a plugin can never
// take over a tool another plugin already owns.
func RegisterContextProvider(p ContextProvider) {
	if p == nil {
		return
	}
	tool := p.Tool()
	if tool == "" {
		return
	}
	registerContextProvider("", tool, p)
}

func registerContextProvider(ownerID, tool string, p ContextProvider) bool {
	ctxMu.Lock()
	_, dup := providers[tool]
	if !dup {
		providers[tool] = p
	}
	ctxMu.Unlock()
	if dup {
		noteDiagnosticf(ownerID, KindContext, tool,
			"context tool %q already has a live provider (%s); later provider skipped",
			tool, contextOwner(tool))
		return false
	}
	return true
}

func contextOwner(tool string) string {
	if d, ok := ByKind(KindContext, tool); ok {
		return d.ID
	}
	return "registered by an earlier caller"
}

func RegisterContext(parentID string, p ContextProvider) {
	if parentID == "" {
		noteDiagnostic(Diagnostic{
			Kind:    KindContext,
			Message: "RegisterContext requires a parent plugin id; context provider skipped",
		})
		return
	}
	if p == nil {
		noteDiagnosticf(parentID, KindContext, "",
			"RegisterContext requires a non-nil ContextProvider; context provider skipped")
		return
	}
	tool := p.Tool()
	if tool == "" {
		noteDiagnosticf(parentID, KindContext, "",
			"RegisterContext requires a provider whose Tool() is non-empty; context provider skipped")
		return
	}
	cid := parentID + "/context/" + tool
	if _, ok := Lookup(cid); ok {
		return
	}
	if prev, ok := ByKind(KindContext, tool); ok {
		noteDiagnosticf(parentID, KindContext, tool,
			"context tool %q is already owned by %q; later context provider skipped", tool, prev.ID)
		return
	}
	if !registerDescriptor(Descriptor{
		ID:     cid,
		Kind:   KindContext,
		Ref:    tool,
		Parent: parentID,
	}) {
		return
	}
	registerContextProvider(cid, tool, p)
}

func HasContextProvider(tool string) bool {
	ctxMu.RLock()
	defer ctxMu.RUnlock()
	_, ok := providers[tool]
	return ok
}

func ContextTools() []string {
	ctxMu.RLock()
	defer ctxMu.RUnlock()
	out := make([]string, 0, len(providers))
	for t := range providers {
		out = append(out, t)
	}
	sort.Strings(out)
	return out
}

func ResetContextProvidersForTest() {
	ctxMu.Lock()
	providers = map[string]ContextProvider{}
	lastSwitch = map[string]string{}
	ctxMu.Unlock()
}

// providerFor resolves the live provider for tool. A provider whose owning
// plugin is disabled is not usable: disabling a plugin must revoke it, not just
// hide it from listings.
func providerFor(tool string) (ContextProvider, error) {
	ctxMu.RLock()
	p, ok := providers[tool]
	ctxMu.RUnlock()
	if !ok {
		return nil, errUnknownTool(tool)
	}
	if d, ok := ByKind(KindContext, tool); ok && !pluginEnabled(d.ID) {
		return nil, &disabledToolError{tool: tool, owner: OwnerID(d)}
	}
	return p, nil
}

func SwitchContext(ctx context.Context, tool, name string) error {
	p, err := providerFor(tool)
	if err != nil {
		return err
	}
	if err := p.Switch(ctx, name); err != nil {
		return err
	}
	ctxMu.Lock()
	lastSwitch[tool] = name
	ctxMu.Unlock()
	return nil
}

func CurrentContext(ctx context.Context, tool string) (Context, bool) {
	p, err := providerFor(tool)
	if err != nil {
		return Context{}, false
	}
	ctxMu.RLock()
	last := lastSwitch[tool]
	ctxMu.RUnlock()
	if name, pokeOK, err := p.Current(ctx); err == nil && pokeOK {
		return Context{Tool: tool, Name: name}, true
	}
	if last != "" {
		return Context{Tool: tool, Name: last}, true
	}
	return Context{Tool: tool}, true
}

func ListContexts(ctx context.Context) []Context {
	ctxMu.RLock()
	tools := make([]string, 0, len(providers))
	for t := range providers {
		tools = append(tools, t)
	}
	ctxMu.RUnlock()
	sort.Strings(tools)
	var out []Context
	for _, t := range tools {
		if c, ok := CurrentContext(ctx, t); ok {
			out = append(out, c)
		}
	}
	return out
}

func ApplyRoleContexts(ctx context.Context, bindings map[string]string) error {
	if len(bindings) == 0 {
		return nil
	}
	tools := make([]string, 0, len(bindings))
	for tool := range bindings {
		tools = append(tools, tool)
	}
	sort.Strings(tools)
	var errs []error
	for _, tool := range tools {
		name := bindings[tool]
		if err := SwitchContext(ctx, tool, name); err != nil {
			errs = append(errs, fmt.Errorf("role context %s→%s: %w", tool, name, err))
		}
	}
	return errors.Join(errs...)
}

func errUnknownTool(tool string) error {
	return &unknownToolError{tool: tool}
}

type unknownToolError struct{ tool string }

func (e *unknownToolError) Error() string {
	return "unknown context tool " + e.tool
}

type disabledToolError struct{ tool, owner string }

func (e *disabledToolError) Error() string {
	return "context tool " + e.tool + " is provided by disabled plugin " + e.owner
}
