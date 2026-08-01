package backup

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const Prefix = "mino-backup-"

const suffix = ".tar.enc"

func PruneLocal(dir string, keep int) []string {
	if keep <= 0 {
		return nil
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	var files []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), Prefix) && strings.HasSuffix(e.Name(), suffix) {
			files = append(files, e.Name())
		}
	}
	sort.Sort(sort.Reverse(sort.StringSlice(files)))
	var deleted []string
	for i := keep; i < len(files); i++ {
		if os.Remove(filepath.Join(dir, files[i])) == nil {
			deleted = append(deleted, files[i])
		}
	}
	return deleted
}
