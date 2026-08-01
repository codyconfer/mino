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

	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/log"
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
	log.Warnf("%s differs only in case from %s: mino reads it as the config file on case-insensitive filesystems "+
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

type directivePlan struct {
	files   map[string]string
	rels    []string
	targets []string
	skipped []string
}

func WriteDirectives(home string, blob []byte) ([]string, error) {
	plan, err := planDirectives(home, blob)
	if err != nil {
		return nil, err
	}
	return plan.apply()
}

func planDirectives(home string, blob []byte) (*directivePlan, error) {
	files, err := decodeCollection(blob)
	if err != nil {
		return nil, err
	}
	plan := &directivePlan{files: files}
	for _, rel := range sortedKeys(files) {
		if reservedRoot(normalizeRel(rel)) {
			plan.skipped = append(plan.skipped, rel)
			log.Warnf("skipping stored directive %q: %s is the name mino reads as its config file, so it can never be a directive; "+
				"run `mino import directives` to rewrite the stored set from your mino home and drop this row", rel, rel)
			continue
		}
		if err := checkDirectiveRel(rel); err != nil {
			return nil, err
		}
		target, err := directivePath(home, rel)
		if err != nil {
			return nil, err
		}
		plan.rels = append(plan.rels, rel)
		plan.targets = append(plan.targets, target)
	}
	return plan, nil
}

func (p *directivePlan) apply() ([]string, error) {
	var undo []undoStep
	written := make([]string, 0, len(p.rels))
	for i, rel := range p.rels {
		target := p.targets[i]
		step, err := capturePriorEntity(target)
		if err != nil {
			return nil, rollbackDirectives(undo, errs.Wrapf(errs.KindConfig, err, "reading %s", target))
		}
		created, derr := ensureDirTracked(filepath.Dir(target))
		step.dirs = created
		if derr != nil {
			undo = append(undo, step)
			return nil, rollbackDirectives(undo, errs.Wrapf(errs.KindConfig, derr, "creating parent of %s", rel))
		}
		if _, err := writeItem(target, []byte(p.files[rel])); err != nil {
			undo = append(undo, step)
			return nil, rollbackDirectives(undo, errs.Wrapf(errs.KindConfig, err, "writing %s", target))
		}
		undo = append(undo, step)
		written = append(written, rel)
	}
	return written, nil
}

func writeItem(target string, data []byte) (string, error) {
	return sconfig.WriteItem(filepath.Dir(target), filepath.Base(target), data)
}

type priorEntity int

const (
	priorAbsent priorEntity = iota
	priorFile
	priorLink
)

type undoStep struct {
	target string
	prior  priorEntity
	body   []byte
	link   string
	dirs   []string
}

func capturePriorEntity(target string) (undoStep, error) {
	step := undoStep{target: target}
	fi, err := os.Lstat(target)
	switch {
	case err == nil:
	case os.IsNotExist(err):
		return step, nil
	default:
		return step, errs.Wrapf(errs.KindConfig, err, "inspecting %s", target)
	}
	if fi.Mode()&os.ModeSymlink != 0 {
		dest, lerr := os.Readlink(target)
		if lerr != nil {
			return step, errs.Wrapf(errs.KindConfig, lerr, "reading the link %s", target)
		}
		step.prior, step.link = priorLink, dest
		return step, nil
	}
	body, ok, rerr := sconfig.ReadRaw(target)
	if rerr != nil {
		return step, errs.Wrapf(errs.KindConfig, rerr, "reading %s", target)
	}
	if ok {
		step.prior, step.body = priorFile, body
	}
	return step, nil
}

func (u undoStep) restore() error {
	switch u.prior {
	case priorFile:
		if _, err := writeItem(u.target, u.body); err != nil {
			return errs.Wrapf(errs.KindConfig, err, "restoring %s", u.target)
		}
	case priorLink:
		if err := removeIfPresent(u.target); err != nil {
			return err
		}
		if err := os.Symlink(u.link, u.target); err != nil {
			return errs.Wrapf(errs.KindConfig, err, "restoring the link %s", u.target)
		}
	default:
		if err := removeIfPresent(u.target); err != nil {
			return err
		}
	}
	for _, dir := range u.dirs {
		if err := removeIfPresent(dir); err != nil {
			return err
		}
	}
	return nil
}

func removeIfPresent(p string) error {
	if err := os.Remove(p); err != nil && !os.IsNotExist(err) {
		return errs.Wrapf(errs.KindConfig, err, "removing %s", p)
	}
	return nil
}

func ensureDirTracked(dir string) ([]string, error) {
	var created []string
	for p := dir; ; {
		if _, err := os.Lstat(p); err == nil {
			break
		} else if !os.IsNotExist(err) {
			return nil, err
		}
		created = append(created, p)
		parent := filepath.Dir(p)
		if parent == p {
			break
		}
		p = parent
	}
	if err := sconfig.EnsureDir(dir); err != nil {
		return created, err
	}
	return created, nil
}

func rollbackDirectives(undo []undoStep, cause *errs.Error) error {
	if len(undo) == 0 {
		return cause
	}
	var stuck []string
	for i := len(undo) - 1; i >= 0; i-- {
		if err := undo[i].restore(); err != nil {
			stuck = append(stuck, err.Error())
		}
	}
	if len(stuck) > 0 {
		sort.Strings(stuck)
		return cause.WithHint("the apply was only partly undone: %d of the %d change(s) made before the failure are still on disk and have to be put back by hand (%s)",
			len(stuck), len(undo), strings.Join(stuck, "; "))
	}
	return cause.WithHint("nothing was applied: the %d file(s) written before the failure were rolled back", len(undo))
}

func checkDirectiveRel(rel string) error {
	clean := normalizeRel(rel)
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
			WithHint("mino replaces directive files in place and will not write through a link; remove it, or save %s %q to a path of your own",
				typeLabel(kind), name)
	}
	raw, ok, err := sconfig.ReadRaw(target)
	if err != nil {
		return errs.Wrapf(errs.KindConfig, err, "reading %s", target)
	}
	if !ok {
		return nil
	}
	placed, err := decodeDocs[directiveDoc](rel, raw)
	if err != nil {
		return errs.Wrapf(errs.KindConfig, err, "%s already exists and does not parse", rel).
			WithHint("fix or remove that file, or save %s %q to a path of your own", typeLabel(kind), name)
	}
	docs := placedValues(placed)
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
