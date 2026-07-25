package plugin

import pub "github.com/codyconfer/munin/plugin"

type BuildContext = pub.BuildContext
type TokenSource = pub.TokenSource
type QueryFunc = pub.QueryFunc
type StreamFunc = pub.StreamFunc
type Builders = pub.Builders

// RegisterBuilders associates Query/Stream constructors with a config signal.
func RegisterBuilders(signal string, b Builders) { pub.RegisterBuilders(signal, b) }

// RegisterSignal registers a descriptor and its builders together.
func RegisterSignal(d Descriptor, b Builders) { pub.RegisterSignal(d, b) }

// LookupBuilders returns builders for signal.
func LookupBuilders(signal string) (Builders, bool) { return pub.LookupBuilders(signal) }

// HasBuilder reports whether signal has a Query builder.
func HasBuilder(signal string) bool { return pub.HasBuilder(signal) }

// HasStreamBuilder reports whether signal has a Stream builder.
func HasStreamBuilder(signal string) bool { return pub.HasStreamBuilder(signal) }

// BuilderSignals returns signal names with any registered builders.
func BuilderSignals() map[string]bool { return pub.BuilderSignals() }

// BuildQuery constructs the Query for signal using bc.
func BuildQuery(signal string, bc BuildContext) (Query, error) {
	return pub.BuildQuery(signal, bc)
}

// BuildStream constructs the Stream for signal using bc.
func BuildStream(signal string, bc BuildContext) (Stream, error) {
	return pub.BuildStream(signal, bc)
}
