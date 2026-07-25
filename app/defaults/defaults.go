// Package defaults demonstrates the go:embed seed layout for distribution
// overlays. Overlay repos copy this pattern with their own YAML.
//
// Expected layout inside the embedded FS:
//
//	config.yaml
//	*.yaml         (roles, one per file, next to config.yaml)
//	queries/*.yaml
//	filters/*.yaml
//	flights/*.yaml
package defaults

import "embed"

// FS holds stock seed files. Overlays typically define their own embed.FS and
// pass it as app.Options.Defaults rather than importing this package.
//
//go:embed *.yaml queries filters flights
var FS embed.FS
