package cmd

import (
	"github.com/codyconfer/munin/internal/config"
	"github.com/codyconfer/munin/internal/errs"
)

func access() config.Access {
	return config.NewAccess(shared.cfg.Role, shared.directives.Roles)
}

func visibleQueryNames() []string {
	a := access()
	var out []string
	for _, n := range shared.directives.QueryNames() {
		if a.QueryVisible(n) {
			out = append(out, n)
		}
	}
	return out
}

func visibleFilterNames() []string {
	a := access()
	var out []string
	for _, n := range shared.directives.FilterNames() {
		if a.FilterVisible(n) {
			out = append(out, n)
		}
	}
	return out
}

func visibleFlightNames() []string {
	a := access()
	var out []string
	for _, n := range shared.directives.FlightNames() {
		if a.FlightVisible(n) {
			out = append(out, n)
		}
	}
	return out
}

func notInRoleError(kind, name string) error {
	return errs.Newf(errs.KindUsage, "%s %q is not available in role %q", kind, name, shared.cfg.Role)
}
