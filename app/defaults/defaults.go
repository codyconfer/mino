package defaults

import "embed"

//go:embed *.yaml queries flights formatters
var FS embed.FS
