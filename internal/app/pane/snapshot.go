package pane

import (
	"encoding/json"
	"os"
	"path/filepath"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/signals"
)

type Kind string

const (
	KindSections Kind = "sections"
	KindDetail   Kind = "detail"
)

type Snapshot struct {
	Kind     Kind                `json:"kind"`
	Title    string              `json:"title"`
	Origin   string              `json:"origin,omitempty"`
	Signal   string              `json:"signal,omitempty"`
	Item     *signals.Item       `json:"item,omitempty"`
	Meta     map[string]string   `json:"meta,omitempty"`
	Sections []signals.Section   `json:"sections,omitempty"`
	Detail   *signals.ItemDetail `json:"detail,omitempty"`
}

func SnapshotPath(home, id string) string {
	return filepath.Join(config.PanesDir(home), id+".json")
}

func WriteSnapshot(path string, s Snapshot) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return errs.Wrapf(errs.KindStore, err, "create %s", filepath.Dir(path))
	}
	b, err := json.Marshal(s)
	if err != nil {
		return errs.Wrap(errs.KindInternal, err, "encode pane snapshot")
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return errs.Wrapf(errs.KindStore, err, "write %s", tmp)
	}
	if err := os.Rename(tmp, path); err != nil {
		return errs.Wrapf(errs.KindStore, err, "replace %s", path)
	}
	return nil
}

func ReadSnapshot(path string) (Snapshot, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return Snapshot{}, errs.Wrapf(errs.KindStore, err, "read %s", path)
	}
	var s Snapshot
	if err := json.Unmarshal(b, &s); err != nil {
		return Snapshot{}, errs.Wrapf(errs.KindConfig, err, "decode %s", path)
	}
	return s, nil
}
