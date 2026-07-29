package plugin

import (
	pub "github.com/codyconfer/munin/plugin"
)

type Kind = pub.Kind

const (
	KindSignal  = pub.KindSignal
	KindFilter  = pub.KindFilter
	KindAction  = pub.KindAction
	KindView    = pub.KindView
	KindTheme   = pub.KindTheme
	KindContext = pub.KindContext
)

type Capability = pub.Capability

const (
	CapQuery     = pub.CapQuery
	CapAction    = pub.CapAction
	CapStream    = pub.CapStream
	CapScheduled = pub.CapScheduled
	CapCacheable = pub.CapCacheable
)

type Query = pub.Query
type Stream = pub.Stream
type Action = pub.Action
type Scheduled = pub.Scheduled
type Descriptor = pub.Descriptor

func KnownKinds() []Kind { return pub.KnownKinds() }

func ValidKind(k Kind) bool { return pub.ValidKind(k) }
