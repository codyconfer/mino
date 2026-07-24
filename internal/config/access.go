package config

// Access scopes which flights/queries/filters are visible for a role.
type Access struct {
	Role    string
	all     bool
	flights map[string]bool
	queries map[string]bool
	filters map[string]bool
}

func NewAccess(role string, roles map[string]RoleDef) Access {
	if role == "" {
		return Access{all: true}
	}
	rd, ok := roles[role]
	if !ok {
		return Access{Role: role}
	}
	return Access{
		Role:    role,
		flights: toSet(rd.Flights),
		queries: toSet(rd.Queries),
		filters: toSet(rd.Filters),
	}
}

func (a Access) FlightVisible(name string) bool { return a.all || a.flights[name] }
func (a Access) QueryVisible(name string) bool  { return a.all || a.queries[name] }
func (a Access) FilterVisible(name string) bool { return a.all || a.filters[name] }

func toSet(xs []string) map[string]bool {
	if len(xs) == 0 {
		return nil
	}
	m := make(map[string]bool, len(xs))
	for _, x := range xs {
		m[x] = true
	}
	return m
}
