package role

import (
	"os"
	"strings"

	"github.com/codyconfer/mino/internal/config"
)

const activeRoleFile = "active-role"

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
