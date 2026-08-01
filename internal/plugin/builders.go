package plugin

import pub "github.com/codyconfer/mino/plugin"

type BuildContext = pub.BuildContext
type TokenSource = pub.TokenSource
type QueryFunc = pub.QueryFunc
type StreamFunc = pub.StreamFunc
type Builders = pub.Builders
type KV = pub.KV

func KVOf(bc BuildContext) KV { return pub.KVOf(bc) }

func RegisterBuilders(signal string, b Builders) { pub.RegisterBuilders(signal, b) }

func RegisterSignal(d Descriptor, b Builders) { pub.RegisterSignal(d, b) }

func LookupBuilders(signal string) (Builders, bool) { return pub.LookupBuilders(signal) }

func HasBuilder(signal string) bool { return pub.HasBuilder(signal) }

func HasStreamBuilder(signal string) bool { return pub.HasStreamBuilder(signal) }

func BuilderSignals() map[string]bool { return pub.BuilderSignals() }

func BuildQuery(signal string, bc BuildContext) (Query, error) {
	return pub.BuildQuery(signal, bc)
}

func BuildStream(signal string, bc BuildContext) (Stream, error) {
	return pub.BuildStream(signal, bc)
}
