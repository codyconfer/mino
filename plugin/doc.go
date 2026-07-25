// Package plugin is the public compile-time plugin SDK.
//
// Overlay binaries register contributions from app.Options.RegisterPlugins
// without importing munin/internal. Stock munin plugins may keep using
// internal/plugin, which re-exports this surface for host code.
//
// Contribution kinds are routed and verified:
//
//	KindSignal  — Register / RegisterSignal + builders
//	KindAction  — RegisterAction (companion Ref "signal/name")
//	KindContext — RegisterContext
//	KindView    — RegisterView (viewkit/deck)
//	KindTheme   — RegisterTheme (viewkit/theme)
//	KindFilter  — RegisterFilter (YAML rules) / RegisterFilterEngine (Go logic)
//
// Status-strip chips use RegisterStatusContribution (not a Kind; host Collect).
//
// Typical overlay registration:
//
//	plugin.RegisterSignal(plugin.Descriptor{
//		ID: "team.example", Kind: plugin.KindSignal, Signal: "example",
//		Capabilities: []plugin.Capability{plugin.CapQuery},
//	}, plugin.Builders{
//		Query: func(plugin.BuildContext) (plugin.Query, error) {
//			return mySignal{}, nil
//		},
//	})
//	plugin.RegisterSeeds("team.example", []plugin.FileSeed{{
//		RelPath: "queries/example.yaml",
//		Content: []byte("name: example\nsignal: example\n"),
//	}})
package plugin
