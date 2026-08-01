package role

import (
	"os"
	"strings"

	sconfig "github.com/codyconfer/sisyphus/config"

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
	if role == "" {
		return sconfig.RemoveItem(config.DataPath(home, activeRoleFile))
	}
	_, err := sconfig.WriteItem(config.DataDir(home), activeRoleFile, []byte(role+"\n"))
	return err
}
