package config

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/codyconfer/sisyphus"
	sconfig "github.com/codyconfer/sisyphus/config"
	"gopkg.in/yaml.v3"

	"github.com/codyconfer/munin/internal/errs"
)

var collectionExts = []string{".yaml", ".yml", ".json"}

var reservedHomeFiles = map[string]bool{
	"config.yaml": true,
	"config.yml":  true,
	"config.json": true,
}

func CollectionDir(home, name string) string {
	if name == KindRoles {
		return home
	}
	return filepath.Join(home, name)
}

func SerializeCollection(home, name string) ([]byte, bool, error) {
	if name == KindRoles {
		return serializeRoles(home)
	}
	return sconfig.SerializeDir(filepath.Join(home, name))
}

func WriteCollection(home, name string, blob []byte) ([]string, error) {
	if name == KindRoles {
		if err := checkRoleBlob(blob); err != nil {
			return nil, err
		}
	}
	return sconfig.WriteCollection(CollectionDir(home, name), blob)
}

func WriteCollectionItem(home, name, item string, doc any) (string, error) {
	data, err := yaml.Marshal(doc)
	if err != nil {
		return "", errs.Wrapf(errs.KindConfig, err, "encoding %s %q", name, item)
	}
	return sconfig.WriteItem(CollectionDir(home, name), item+".yaml", data)
}

func SaveCollectionItem(mgr *sisyphus.Manager, home, name, item string, doc any) (path string, stored bool, err error) {
	path, err = WriteCollectionItem(home, name, item, doc)
	if err != nil {
		return "", false, err
	}
	stored, err = SyncCollection(mgr, home, name)
	return path, stored, err
}

func SyncCollection(mgr *sisyphus.Manager, home, name string) (stored bool, err error) {
	if mgr == nil {
		return false, nil
	}
	blob, has, err := SerializeCollection(home, name)
	if err != nil {
		return false, err
	}
	if !has {
		blob = []byte("{}")
	}
	if err := mgr.Import(context.Background(), name, blob, "collection"); err != nil {
		return false, errs.Wrapf(errs.KindStore, err, "importing %s into the store", name)
	}
	return true, nil
}

func ClearCollection(home, name string) ([]string, error) {
	if name == KindRoles {
		return removeRoleFiles(home)
	}
	return sconfig.ClearDir(filepath.Join(home, name), nil)
}

func RemoveCollectionItem(home, name, item string) ([]string, error) {
	if name == KindRoles && reservedRoleName(item) {
		return nil, errs.Newf(errs.KindConfig, "%q is a reserved name at the top of %s", item, home)
	}
	return sconfig.RemoveFiles(CollectionDir(home, name), item, collectionExts)
}

func RoleFiles(home string) []string {
	names, err := roleFileNames(home)
	if err != nil {
		return nil
	}
	return names
}

func roleFileNames(home string) ([]string, error) {
	entries, err := os.ReadDir(home)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errs.Wrapf(errs.KindConfig, err, "reading %s", home)
	}
	var out []string
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || strings.HasPrefix(name, ".") || reservedHomeFiles[name] || !hasCollectionExt(name) {
			continue
		}
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

func serializeRoles(home string) ([]byte, bool, error) {
	names, err := roleFileNames(home)
	if err != nil {
		return nil, false, err
	}
	if len(names) == 0 {
		return nil, false, nil
	}
	c := make(map[string]string, len(names))
	for _, n := range names {
		data, err := os.ReadFile(filepath.Join(home, n))
		if err != nil {
			return nil, false, errs.Wrapf(errs.KindConfig, err, "reading %s", filepath.Join(home, n))
		}
		c[n] = string(data)
	}
	blob, err := json.Marshal(c)
	if err != nil {
		return nil, false, errs.Wrap(errs.KindConfig, err, "encoding roles")
	}
	return blob, true, nil
}

func removeRoleFiles(home string) ([]string, error) {
	names, err := roleFileNames(home)
	if err != nil {
		return nil, err
	}
	var removed []string
	for _, n := range names {
		path := filepath.Join(home, n)
		if err := os.Remove(path); err != nil {
			return removed, errs.Wrapf(errs.KindConfig, err, "removing %s", path)
		}
		removed = append(removed, path)
	}
	return removed, nil
}

func checkRoleBlob(blob []byte) error {
	var files map[string]string
	if err := json.Unmarshal(blob, &files); err != nil {
		return errs.Wrap(errs.KindConfig, err, "decoding roles")
	}
	for name := range files {
		if reservedHomeFiles[name] {
			return errs.Newf(errs.KindConfig, "role file %q collides with the config file", name)
		}
	}
	return nil
}

func reservedRoleName(item string) bool {
	for _, ext := range collectionExts {
		if reservedHomeFiles[item+ext] {
			return true
		}
	}
	return false
}

func hasCollectionExt(name string) bool {
	ext := strings.ToLower(filepath.Ext(name))
	for _, e := range collectionExts {
		if ext == e {
			return true
		}
	}
	return false
}
