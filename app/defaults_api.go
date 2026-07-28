package app

import (
	"io/fs"
	"path"
	"sort"
	"strings"
)

type DefaultFile struct {
	RelPath string
	Data    []byte
}

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
