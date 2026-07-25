package role

import (
	"os"
	"strings"

	"github.com/codyconfer/munin/internal/config"
)

const activeRoleFile = "active-role"

// LoadActive returns the last role for which enter hooks completed a transition
// (empty when none). Used so exit hooks can run across CLI invocations.
func LoadActive(home string) string {
	if home == "" {
		return ""
	}
	b, err := os.ReadFile(config.DataPath(home, activeRoleFile))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// SaveActive persists the role currently considered "entered" for hooks.
func SaveActive(home, role string) error {
	if home == "" {
		return nil
	}
	path := config.DataPath(home, activeRoleFile)
	if role == "" {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	return os.WriteFile(path, []byte(role+"\n"), 0o600)
}
