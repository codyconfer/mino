// Package signals re-exports public plugin SDK types for host packages.
package signals

import "github.com/codyconfer/munin/plugin"

type Item = plugin.Item
type Section = plugin.Section
type Event = plugin.Event
type Signal = plugin.Query
type ActiveSignal = plugin.Stream
type Scheduled = plugin.Scheduled
