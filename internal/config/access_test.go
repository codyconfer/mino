package config

import "testing"

func testRoles() map[string]RoleDef {
	return map[string]RoleDef{
		"triage": {
			Name:    "triage",
			Flights: []string{"triage"},
			Queries: []string{"incidents", "loki-errors", "no-bots"},
		},
	}
}

func TestNewAccessScopesToRoleLists(t *testing.T) {
	a := NewAccess("triage", testRoles())
	if a.Role != "triage" {
		t.Errorf("role = %q, want triage", a.Role)
	}

	if !a.FlightVisible("triage") {
		t.Error("flight named by the role should be visible")
	}
	if a.FlightVisible("default") {
		t.Error("flight not named by the role should be hidden")
	}
	if !a.QueryVisible("incidents") || !a.QueryVisible("loki-errors") {
		t.Error("queries named by the role should be visible")
	}
	if a.QueryVisible("today") {
		t.Error("query not named by the role should be hidden")
	}
	if !a.QueryVisible("no-bots") {
		t.Error("filter-only documents are scoped by the same queries list")
	}
}

func TestNewAccessNoActiveRoleShowsAll(t *testing.T) {
	a := NewAccess("", testRoles())
	if !a.FlightVisible("anything") || !a.QueryVisible("anything") {
		t.Error("with no active role, everything should be visible")
	}
}

func TestNewAccessUnknownRoleShowsNothing(t *testing.T) {
	a := NewAccess("ghost", testRoles())
	if a.Role != "ghost" {
		t.Errorf("role = %q, want ghost", a.Role)
	}
	if a.FlightVisible("triage") || a.QueryVisible("incidents") {
		t.Error("an undefined active role should surface nothing")
	}
}
