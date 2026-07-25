package plugin

import (
	"context"

	pub "github.com/codyconfer/munin/plugin"
)

type Context = pub.Context
type ContextProvider = pub.ContextProvider

// RegisterContextProvider registers a tool context provider without KindContext.
func RegisterContextProvider(p ContextProvider) { pub.RegisterContextProvider(p) }

// RegisterContext registers a ContextProvider and KindContext companion.
func RegisterContext(parentID string, p ContextProvider) { pub.RegisterContext(parentID, p) }

// HasContextProvider reports whether tool has a registered ContextProvider.
func HasContextProvider(tool string) bool { return pub.HasContextProvider(tool) }

// ContextTools returns registered context tool names sorted.
func ContextTools() []string { return pub.ContextTools() }

// SwitchContext switches the named tool to name and records the selection.
func SwitchContext(ctx context.Context, tool, name string) error {
	return pub.SwitchContext(ctx, tool, name)
}

// CurrentContext returns the active context for tool.
func CurrentContext(ctx context.Context, tool string) (Context, bool) {
	return pub.CurrentContext(ctx, tool)
}

// ListContexts returns last-known contexts for all registered tools.
func ListContexts(ctx context.Context) []Context { return pub.ListContexts(ctx) }

// ApplyRoleContexts runs role context bindings (tool → name).
func ApplyRoleContexts(ctx context.Context, bindings map[string]string) error {
	return pub.ApplyRoleContexts(ctx, bindings)
}
