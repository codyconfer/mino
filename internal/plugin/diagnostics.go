package plugin

import pub "github.com/codyconfer/mino/plugin"

type Diagnostic = pub.Diagnostic

// Diagnostics returns every contribution the registry skipped or degraded. A
// skipped contribution is absent from the registry, so `plugins enable`,
// `plugins disable` and `plugins install` all answer "unknown plugin" for it:
// listings are the only place a user can be told it exists at all.
func Diagnostics() []Diagnostic { return pub.Diagnostics() }

func DiagnosticsFor(pluginID string) []Diagnostic { return pub.DiagnosticsFor(pluginID) }

func HasDiagnostics() bool { return pub.HasDiagnostics() }

func ResetDiagnostics() { pub.ResetDiagnostics() }

func NoteDiagnostic(pluginID string, kind Kind, ref, message string) {
	pub.NoteDiagnostic(pluginID, kind, ref, message)
}

// DiagnosticLines renders one line per diagnostic, ready to append to a
// `plugins list` listing.
func DiagnosticLines() []string {
	diags := pub.Diagnostics()
	out := make([]string, 0, len(diags))
	for _, d := range diags {
		out = append(out, d.String())
	}
	return out
}
