// Package defaults demonstrates the go:embed seed layout for distribution
// overlays (ADR-8). Overlay repos copy this pattern with their own YAML.
//
// Expected layout inside the embedded FS:
//
//	config.yaml
//	queries/*.yaml
//	filters/*.yaml
//	flights/*.yaml
//	roles/*.yaml
package defaults

import "embed"

// FS holds stock seed files. Overlays typically define their own embed.FS and
// pass it as app.Options.Defaults rather than importing this package.
//
//go:embed config.yaml queries filters flights roles
var FS embed.FS
