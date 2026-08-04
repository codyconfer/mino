package role

import (
	"os"
	"strings"

	sconfig "github.com/codyconfer/sisyphus/config"

	"github.com/codyconfer/mino/internal/config"
)

const activeRoleFile = "active-role"

func TakeLegacyActive(home string) (string, bool) {
	if home == "" {
		return "", false
	}
	path := config.DataPath(home, activeRoleFile)
	b, err := os.ReadFile(path)
	if err != nil {
		return "", false
	}
	_ = sconfig.RemoveItem(path)
	return strings.TrimSpace(string(b)), true
}
