package plugin

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"sync"
)

// Context is a per-tool active selection (ADR-9).
type Context struct {
	Tool string
	Name string
}

// ContextProvider is implemented by plugins that own a switchable context.
type ContextProvider interface {
	Tool() string
	Switch(ctx context.Context, name string) error
	// Current is optional; when absent the host renders "as of last switch".
	Current(ctx context.Context) (name string, ok bool, err error)
}

var (
	ctxMu      sync.RWMutex
	providers  = map[string]ContextProvider{}
	lastSwitch = map[string]string{} // tool → name
)

// RegisterContextProvider registers a tool context provider.
func RegisterContextProvider(p ContextProvider) {
	if p == nil || p.Tool() == "" {
		return
	}
	ctxMu.Lock()
	providers[p.Tool()] = p
	ctxMu.Unlock()
}

// SwitchContext switches the named tool to name and records the selection.
func SwitchContext(ctx context.Context, tool, name string) error {
	ctxMu.RLock()
	p, ok := providers[tool]
	ctxMu.RUnlock()
	if !ok {
		return errUnknownTool(tool)
	}
	if err := p.Switch(ctx, name); err != nil {
		return err
	}
	ctxMu.Lock()
	lastSwitch[tool] = name
	ctxMu.Unlock()
	return nil
}

// CurrentContext returns the active context for tool. Prefers Current() probe;
// falls back to last switch (R4 drift mitigation).
func CurrentContext(ctx context.Context, tool string) (Context, bool) {
	ctxMu.RLock()
	p, ok := providers[tool]
	last := lastSwitch[tool]
	ctxMu.RUnlock()
	if !ok {
		return Context{}, false
	}
	if name, pokeOK, err := p.Current(ctx); err == nil && pokeOK {
		return Context{Tool: tool, Name: name}, true
	}
	if last != "" {
		return Context{Tool: tool, Name: last}, true
	}
	return Context{Tool: tool}, true
}

// ListContexts returns last-known contexts for all registered tools.
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

// ApplyRoleContexts runs role context bindings (tool → name) in stable tool
// order. Failures are collected so one bad tool does not skip the rest;
// a joined error is returned when any binding fails.
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
