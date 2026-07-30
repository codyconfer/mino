package config

import (
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"

	"github.com/codyconfer/sisyphus"
	sconfig "github.com/codyconfer/sisyphus/config"
	"gopkg.in/yaml.v3"

	"github.com/codyconfer/munin/internal/errs"
	"github.com/codyconfer/munin/internal/log"
)

var collectionExts = []string{".yaml", ".yml", ".json"}

var (
	miscasedMu   sync.Mutex
	miscasedSeen = map[string]bool{}
)

func DirectiveFiles(home string) ([]string, error) {
	var out []string
	err := filepath.WalkDir(home, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		rel, relErr := filepath.Rel(home, p)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		if rel == "." {
			return nil
		}
		if d.IsDir() {
			if skipDir(d.Name()) {
				return fs.SkipDir
			}
			return nil
		}
		if strings.HasPrefix(d.Name(), ".") || !hasCollectionExt(d.Name()) {
			return nil
		}
		if reservedRoot(rel) {
			warnMiscasedConfig(home, rel)
			return nil
		}
		out = append(out, rel)
		return nil
	})
	if err != nil {
		return nil, errs.Wrapf(errs.KindConfig, err, "reading %s", home)
	}
	sort.Strings(out)
	return out, nil
}

func skipDir(name string) bool {
	return strings.HasPrefix(name, ".") || name == DirLogs || name == DirReports
}

func reservedRoot(rel string) bool {
	return !strings.Contains(rel, "/") && isReservedConfigName(rel)
}

func isReservedConfigName(name string) bool {
	for _, reserved := range ConfigBasenames() {
		if strings.EqualFold(name, reserved) {
			return true
		}
	}
	return false
}

func warnMiscasedConfig(home, rel string) {
	canonical := strings.ToLower(rel)
	if rel == canonical {
		return
	}
	path := filepath.Join(home, rel)
	miscasedMu.Lock()
	defer miscasedMu.Unlock()
	if miscasedSeen[path] {
		return
	}
	miscasedSeen[path] = true
	log.Warnf("%s differs only in case from %s: munin reads it as the config file on case-insensitive filesystems "+
		"and ignores it everywhere else, and either way it is not a directive; rename it to %s", path, canonical, canonical)
}

func SerializeDirectives(home string) ([]byte, bool, error) {
	rels, err := DirectiveFiles(home)
	if err != nil {
		return nil, false, err
	}
	if len(rels) == 0 {
		return nil, false, nil
	}
	c := make(map[string]string, len(rels))
	for _, rel := range rels {
		p := filepath.Join(home, filepath.FromSlash(rel))
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, false, errs.Wrapf(errs.KindConfig, err, "reading %s", p)
		}
		c[rel] = string(data)
	}
	blob, err := json.Marshal(c)
	if err != nil {
		return nil, false, errs.Wrap(errs.KindConfig, err, "encoding directives")
	}
	return blob, true, nil
}

func WriteDirectives(home string, blob []byte) ([]string, error) {
	files, err := decodeCollection(blob)
	if err != nil {
		return nil, err
	}
	rels := sortedKeys(files)
	targets := make([]string, len(rels))
	for i, rel := range rels {
		if err := checkDirectiveRel(rel); err != nil {
			return nil, err
		}
		target, err := directivePath(home, rel)
		if err != nil {
			return nil, err
		}
		targets[i] = target
	}
	var undo []func()
	written := make([]string, 0, len(rels))
	for i, rel := range rels {
		target := targets[i]
		prev, existed, err := sconfig.ReadRaw(target)
		if err != nil {
			return nil, rollbackDirectives(undo, errs.Wrapf(errs.KindConfig, err, "reading %s", target))
		}
		if err := sconfig.EnsureDir(filepath.Dir(target)); err != nil {
			return nil, rollbackDirectives(undo, errs.Wrapf(errs.KindConfig, err, "creating parent of %s", rel))
		}
		if _, err := writeItem(target, []byte(files[rel])); err != nil {
			return nil, rollbackDirectives(undo, errs.Wrapf(errs.KindConfig, err, "writing %s", target))
		}
		undo = append(undo, undoDirectiveWrite(target, prev, existed))
		written = append(written, rel)
	}
	return written, nil
}

func writeItem(target string, data []byte) (string, error) {
	return sconfig.WriteItem(filepath.Dir(target), filepath.Base(target), data)
}

func undoDirectiveWrite(target string, prev []byte, existed bool) func() {
	if !existed {
		return func() { _ = os.Remove(target) }
	}
	return func() { _, _ = writeItem(target, prev) }
}

func rollbackDirectives(undo []func(), cause *errs.Error) error {
	for i := len(undo) - 1; i >= 0; i-- {
		undo[i]()
	}
	if len(undo) == 0 {
		return cause
	}
	return cause.WithHint("nothing was applied: the %d file(s) written before the failure were rolled back", len(undo))
}

func checkDirectiveRel(rel string) error {
	clean := path.Clean(strings.ReplaceAll(rel, `\`, "/"))
	if reservedRoot(clean) {
		return errs.Newf(errs.KindConfig, "directive file %q collides with the config file", rel)
	}
	if !hasCollectionExt(clean) {
		return errs.Newf(errs.KindConfig, "directive file %q must end in .yaml, .yml, or .json", rel)
	}
	segs := strings.Split(clean, "/")
	for _, seg := range segs[:len(segs)-1] {
		if skipDir(seg) {
			return errs.Newf(errs.KindConfig, "directive file %q lives in reserved directory %q", rel, seg)
		}
	}
	if strings.HasPrefix(segs[len(segs)-1], ".") {
		return errs.Newf(errs.KindConfig, "directive file %q is hidden", rel)
	}
	return nil
}

func directivePath(home, rel string) (string, error) {
	target, err := sconfig.JoinUnder(home, filepath.FromSlash(rel))
	if err != nil {
		return "", errs.Wrapf(errs.KindConfig, err, "resolving %s", rel)
	}
	return target, nil
}

func ClearDirectives(home string) ([]string, error) {
	rels, err := DirectiveFiles(home)
	if err != nil {
		return nil, err
	}
	var removed []string
	for _, rel := range rels {
		p := filepath.Join(home, filepath.FromSlash(rel))
		if err := os.Remove(p); err != nil {
			return removed, errs.Wrapf(errs.KindConfig, err, "removing %s", p)
		}
		removed = append(removed, p)
	}
	return removed, nil
}

func DefaultDirectivePath(kind DirectiveType, name string) string {
	switch kind {
	case TypeFlight:
		return path.Join(DirFlights, name+".yaml")
	case TypeFormatter:
		return path.Join(DirFormatters, name+".yaml")
	case TypeRole:
		return name + ".yaml"
	}
	return path.Join(DirQueries, name+".yaml")
}

func SaveDirective(mgr *sisyphus.Manager, home, rel string, kind DirectiveType, name string, doc any) (string, bool, error) {
	derived := strings.TrimSpace(rel) == ""
	if derived {
		rel = DefaultDirectivePath(kind, name)
	}
	if err := checkDirectiveRel(rel); err != nil {
		return "", false, err
	}
	data, err := yaml.Marshal(stampType(doc, kind))
	if err != nil {
		return "", false, errs.Wrapf(errs.KindConfig, err, "encoding %s %q", typeLabel(kind), name)
	}
	target, err := directivePath(home, rel)
	if err != nil {
		return "", false, err
	}
	if derived {
		if err := checkDerivedTarget(target, rel, kind, name); err != nil {
			return "", false, err
		}
	}
	if err := sconfig.EnsureDir(filepath.Dir(target)); err != nil {
		return "", false, errs.Wrapf(errs.KindConfig, err, "creating parent of %s", rel)
	}
	if _, err := writeItem(target, data); err != nil {
		return "", false, errs.Wrapf(errs.KindConfig, err, "writing %s", target)
	}
	stored, err := SyncDirectives(mgr, home)
	return target, stored, err
}

func checkDerivedTarget(target, rel string, kind DirectiveType, name string) error {
	fi, err := os.Lstat(target)
	switch {
	case err == nil:
	case os.IsNotExist(err):
		return nil
	default:
		return errs.Wrapf(errs.KindConfig, err, "inspecting %s", target)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		return errs.Newf(errs.KindConfig, "%s is a symlink", rel).
			WithHint("munin replaces directive files in place and will not write through a link; remove it, or save %s %q to a path of your own",
				typeLabel(kind), name)
	}
	raw, ok, err := sconfig.ReadRaw(target)
	if err != nil {
		return errs.Wrapf(errs.KindConfig, err, "reading %s", target)
	}
	if !ok {
		return nil
	}
	docs, err := decodeDocs[directiveDoc](rel, raw)
	if err != nil {
		return errs.Wrapf(errs.KindConfig, err, "%s already exists and does not parse", rel).
			WithHint("fix or remove that file, or save %s %q to a path of your own", typeLabel(kind), name)
	}
	if len(docs) == 0 || (len(docs) == 1 && sameDirective(docs[0], rel, kind, name)) {
		return nil
	}
	return errs.Newf(errs.KindConfig, "%s already holds %s", rel, describeDirectiveDocs(docs)).
		WithHint("writing %s %q there would drop that content; give it a file of its own, or add it to %s by hand",
			typeLabel(kind), name, rel)
}

func sameDirective(doc directiveDoc, rel string, kind DirectiveType, name string) bool {
	have := doc.Name
	if have == "" {
		have = baseName(rel)
	}
	return have == name && sameDirectiveKind(doc.Type, kind)
}

func sameDirectiveKind(have, want DirectiveType) bool {
	if have == TypeAuto || want == TypeAuto || have == want {
		return true
	}
	return queryish(have) && queryish(want)
}

func queryish(k DirectiveType) bool { return k == TypeQuery || k == TypeFilter }

func describeDirectiveDocs(docs []directiveDoc) string {
	names := make([]string, 0, len(docs))
	for _, d := range docs {
		if d.Name == "" {
			names = append(names, "an unnamed "+typeLabel(d.Type))
			continue
		}
		names = append(names, typeLabel(d.Type)+" "+strconv.Quote(d.Name))
	}
	count := strconv.Itoa(len(docs)) + " documents"
	if len(docs) == 1 {
		count = "1 document"
	}
	return count + ": " + strings.Join(names, ", ")
}

func stampType(doc any, kind DirectiveType) any {
	switch d := doc.(type) {
	case Query:
		d.Type = kind
		return d
	case Flight:
		d.Type = kind
		return d
	case RoleDef:
		d.Type = kind
		return d
	case FormatterDef:
		d.Type = kind
		return d
	}
	return doc
}

func RemoveDirective(home, rel string) ([]string, error) {
	if strings.TrimSpace(rel) == "" {
		return nil, nil
	}
	if err := checkDirectiveRel(rel); err != nil {
		return nil, err
	}
	target, err := directivePath(home, rel)
	if err != nil {
		return nil, err
	}
	if err := os.Remove(target); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, errs.Wrapf(errs.KindConfig, err, "removing %s", target)
	}
	return []string{target}, nil
}

func SyncDirectives(mgr *sisyphus.Manager, home string) (bool, error) {
	if mgr == nil {
		return false, nil
	}
	blob, has, err := SerializeDirectives(home)
	if err != nil {
		return false, err
	}
	if !has {
		blob = []byte("{}")
	}
	if err := mgr.Import(context.Background(), DirectivesDirective, blob, "collection"); err != nil {
		return false, errs.Wrap(errs.KindStore, err, "importing directives into the store")
	}
	return true, nil
}

func hasCollectionExt(name string) bool {
	return slices.Contains(collectionExts, strings.ToLower(filepath.Ext(name)))
}
