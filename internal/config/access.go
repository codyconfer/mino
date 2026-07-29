package config

type Access struct {
	Role       string
	all        bool
	flights    map[string]bool
	queries    map[string]bool
	formatters map[string]bool
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
		Role:       role,
		flights:    toSet(rd.Flights),
		queries:    toSet(rd.Queries),
		formatters: toSet(rd.Formatters),
	}
}

func (a Access) FlightVisible(name string) bool    { return a.all || a.flights[name] }
func (a Access) QueryVisible(name string) bool     { return a.all || a.queries[name] }
func (a Access) FormatterVisible(name string) bool { return a.all || a.formatters[name] }

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
