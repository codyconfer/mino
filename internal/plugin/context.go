package plugin

import (
	"context"

	pub "github.com/codyconfer/mino/plugin"
)

type Context = pub.Context
type ContextProvider = pub.ContextProvider

func RegisterContextProvider(p ContextProvider) { pub.RegisterContextProvider(p) }

func RegisterContext(parentID string, p ContextProvider) { pub.RegisterContext(parentID, p) }

func HasContextProvider(tool string) bool { return pub.HasContextProvider(tool) }

func ContextTools() []string { return pub.ContextTools() }

func SwitchContext(ctx context.Context, tool, name string) error {
	return pub.SwitchContext(ctx, tool, name)
}

func CurrentContext(ctx context.Context, tool string) (Context, bool) {
	return pub.CurrentContext(ctx, tool)
}

func ListContexts(ctx context.Context) []Context { return pub.ListContexts(ctx) }

func ApplyRoleContexts(ctx context.Context, bindings map[string]string) error {
	return pub.ApplyRoleContexts(ctx, bindings)
}
