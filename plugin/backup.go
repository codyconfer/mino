package plugin

import (
	"context"
	"sort"
	"sync"
)

const KindBackup Kind = "backup"

type BackupDestination interface {
	Name() string
	Upload(ctx context.Context, name string, data []byte, contentType string) (Item, error)
	Prune(ctx context.Context, prefix string, keep int) ([]string, error)
}

type BackupDestinationFunc func(h Host) (BackupDestination, error)

type backupEntry struct {
	pluginID string
	open     BackupDestinationFunc
}

var (
	backupMu       sync.RWMutex
	backupByName   = map[string]backupEntry{}
	reservedBackup = map[string]bool{"local": true}
)

func RegisterBackupDestination(pluginID, name string, open BackupDestinationFunc) {
	noteRegistrationCheckpoint(pluginID)
	if name == "" {
		noteDiagnosticf(pluginID, KindBackup, "",
			"RegisterBackupDestination requires a destination name; destination skipped")
		return
	}
	if open == nil {
		noteDiagnosticf(pluginID, KindBackup, name,
			"backup destination %q supplied no open func; destination skipped", name)
		return
	}
	if reservedBackup[name] {
		noteDiagnosticf(pluginID, KindBackup, name,
			"backup destination %q is reserved by the host; destination skipped", name)
		return
	}
	backupMu.Lock()
	incumbent, dup := backupByName[name]
	if !dup {
		backupByName[name] = backupEntry{pluginID: pluginID, open: open}
	}
	backupMu.Unlock()
	if dup {
		owner := incumbent.pluginID
		if owner == "" {
			owner = "an earlier registration"
		}
		noteDiagnosticf(pluginID, KindBackup, name,
			"backup destination %q is already registered by %q; later destination skipped", name, owner)
	}
}

func LookupBackupDestination(name string) (BackupDestinationFunc, bool) {
	backupMu.RLock()
	defer backupMu.RUnlock()
	entry, ok := backupByName[name]
	if !ok {
		return nil, false
	}
	return entry.open, true
}

func BackupDestinationOwner(name string) (string, bool) {
	backupMu.RLock()
	defer backupMu.RUnlock()
	entry, ok := backupByName[name]
	if !ok {
		return "", false
	}
	return entry.pluginID, true
}

func BackupDestinations() []string {
	backupMu.RLock()
	out := make([]string, 0, len(backupByName))
	for name := range backupByName {
		out = append(out, name)
	}
	backupMu.RUnlock()
	sort.Strings(out)
	return out
}

func ResetBackupDestinations() {
	backupMu.Lock()
	backupByName = map[string]backupEntry{}
	backupMu.Unlock()
}
