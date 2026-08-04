package pane

import (
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"github.com/codyconfer/viewkit/theme"

	"github.com/codyconfer/mino/internal/config"
	"github.com/codyconfer/mino/internal/errs"
	"github.com/codyconfer/mino/internal/log"
	"github.com/codyconfer/mino/internal/tmux"
)

const (
	inboxPercent  = 40
	detailPercent = 50
	shellPercent  = 30
	splitDivider  = 1
)

var errClosed = errs.New(errs.KindInternal, "pane manager is closed")

var (
	splitFn  = tmux.Split
	killFn   = tmux.Kill
	existsFn = tmux.Exists
)

func (m *Manager) layoutFor(percent int) (horizontal bool, size int) {
	width, height, ok := tmux.PaneSize(m.own)
	if !ok {
		return false, percent
	}
	if side := width*percent/100 - splitDivider; side >= theme.MinScreenWidth &&
		width-side-splitDivider >= theme.MinScreenWidth {
		return true, side
	}
	return false, max(height*percent/100, 1)
}

type tracked struct {
	id       tmux.PaneID
	snapshot string
}

type Manager struct {
	home   string
	flight string
	role   string
	self   string
	own    tmux.PaneID
	env    []string

	mu     sync.Mutex
	panes  []tracked
	seq    int
	closed bool
}

func NewManager(home, flight, role string) (*Manager, error) {
	if !tmux.Inside() {
		return nil, errs.New(errs.KindInternal, "pane manager requires a tmux session")
	}
	self, err := os.Executable()
	if err != nil {
		return nil, errs.Wrap(errs.KindInternal, err, "locate mino binary")
	}
	return &Manager{
		home:   home,
		flight: flight,
		role:   role,
		self:   self,
		own:    tmux.SelfPane(),
		env:    []string{OwnerEnv + "=" + strconv.Itoa(os.Getpid())},
	}, nil
}

func (m *Manager) OpenInbox() error {
	argv := []string{m.self, "pane", "inbox"}
	if m.flight != "" {
		argv = append(argv, m.flight)
	}
	argv = m.withHome(argv)
	horizontal, size := m.layoutFor(inboxPercent)
	return m.split(tmux.SplitOpts{
		Target:     m.own,
		Horizontal: horizontal,
		Size:       size,
		Title:      "mino inbox",
		Env:        m.env,
		Argv:       argv,
	}, "")
}

func (m *Manager) OpenSnapshot(s Snapshot) error {
	m.mu.Lock()
	m.seq++
	id := strconv.Itoa(os.Getpid()) + "-" + strconv.Itoa(m.seq)
	m.mu.Unlock()

	path := SnapshotPath(m.home, id)
	if err := WriteSnapshot(path, s); err != nil {
		return err
	}
	title := s.Title
	if title == "" {
		title = "mino pane"
	}
	horizontal, size := m.layoutFor(detailPercent)
	err := m.split(tmux.SplitOpts{
		Target:     m.own,
		Horizontal: horizontal,
		Size:       size,
		Title:      title,
		Env:        m.env,
		Argv:       m.withHome([]string{m.self, "pane", "view", path}),
	}, path)
	if err != nil {
		_ = os.Remove(path)
	}
	return err
}

func (m *Manager) OpenShell() error {
	sh := os.Getenv("SHELL")
	if sh == "" {
		sh = "/bin/sh"
	}
	_, height, ok := tmux.PaneSize(m.own)
	size := shellPercent
	if ok {
		size = max(height*shellPercent/100, 1)
	}
	return m.split(tmux.SplitOpts{
		Target:     m.own,
		Horizontal: false,
		Size:       size,
		Title:      "shell",
		Argv:       []string{sh},
	}, "")
}

func (m *Manager) Refresh(id tmux.PaneID, s Snapshot) error {
	m.mu.Lock()
	var path string
	for _, p := range m.panes {
		if p.id == id {
			path = p.snapshot
			break
		}
	}
	m.mu.Unlock()
	if path == "" {
		return errs.Newf(errs.KindInternal, "pane %s has no snapshot", id)
	}
	return WriteSnapshot(path, s)
}

func (m *Manager) CloseLast() error {
	if m == nil {
		return nil
	}
	m.prune()
	m.mu.Lock()
	if len(m.panes) == 0 {
		m.mu.Unlock()
		return nil
	}
	last := m.panes[len(m.panes)-1]
	m.panes = m.panes[:len(m.panes)-1]
	m.mu.Unlock()
	return m.discard(last)
}

func (m *Manager) CloseAll() {
	if m == nil {
		return
	}
	m.mu.Lock()
	panes := m.panes
	m.panes = nil
	m.closed = true
	m.mu.Unlock()
	for _, p := range panes {
		if err := m.discard(p); err != nil {
			log.Debugf("pane: close %s: %v", p.id, err)
		}
	}
}

func (m *Manager) Count() int {
	if m == nil {
		return 0
	}
	m.prune()
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.panes)
}

func (m *Manager) split(o tmux.SplitOpts, snapshot string) error {
	if m.isClosed() {
		return errClosed
	}
	m.prune()
	id, err := splitFn(o)
	if err != nil {
		return err
	}
	t := tracked{id: id, snapshot: snapshot}
	m.mu.Lock()
	if m.closed {
		m.mu.Unlock()
		if derr := m.discard(t); derr != nil {
			log.Debugf("pane: discard %s opened after close: %v", t.id, derr)
		}
		return errClosed
	}
	m.panes = append(m.panes, t)
	m.mu.Unlock()
	return nil
}

func (m *Manager) isClosed() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.closed
}

func (m *Manager) discard(p tracked) error {
	if p.snapshot != "" {
		_ = os.Remove(p.snapshot)
	}
	return killFn(p.id)
}

func (m *Manager) prune() {
	m.mu.Lock()
	kept := m.panes[:0]
	var gone []tracked
	for _, p := range m.panes {
		if existsFn(p.id) {
			kept = append(kept, p)
			continue
		}
		gone = append(gone, p)
	}
	m.panes = kept
	m.mu.Unlock()
	for _, p := range gone {
		if p.snapshot != "" {
			_ = os.Remove(p.snapshot)
		}
	}
}

func (m *Manager) withHome(argv []string) []string {
	if m.home != "" {
		argv = append(argv, "--home", m.home)
	}
	if m.role != "" {
		argv = append(argv, "--role", m.role)
	}
	return argv
}

func CleanupSnapshots(home string) {
	dir := config.PanesDir(home)
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}
