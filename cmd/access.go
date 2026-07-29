package cmd

import "github.com/codyconfer/munin/internal/config"

func access() config.Access { return shared.Access() }

func visibleQueryNames() []string { return shared.VisibleQueries() }

func visibleFilterNames() []string { return shared.VisibleFilters() }

func visibleFlightNames() []string { return shared.VisibleFlights() }

func visibleFormatterNames() []string { return shared.VisibleFormatters() }

func notInRoleError(kind, name string) error { return shared.NotInRoleError(kind, name) }
