package app

import (
	"io/fs"
	"path"
	"sort"
	"strings"
)

// DefaultFile lists one path inside an overlay Defaults FS.
type DefaultFile struct {
	RelPath string
	Data    []byte
}

// ListDefaults walks a Defaults filesystem and returns seed files suitable for
// an InstallSpec. Skips directories and hidden files. RelPath uses forward slashes.
// app.Run registers Options.Defaults so Install/Nuke merge these over stock seeds.
func ListDefaults(defaults fs.FS) ([]DefaultFile, error) {
	if defaults == nil {
		return nil, nil
	}
	var out []DefaultFile
	err := fs.WalkDir(defaults, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		base := path.Base(p)
		if strings.HasPrefix(base, ".") {
			return nil
		}
		b, err := fs.ReadFile(defaults, p)
		if err != nil {
			return err
		}
		out = append(out, DefaultFile{RelPath: path.Clean(p), Data: b})
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(out, func(i, j int) bool { return out[i].RelPath < out[j].RelPath })
	return out, nil
}
