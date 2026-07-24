package build

import "testing"

func TestResolveWriteTarget(t *testing.T) {
	if _, err := ResolveWriteTarget("task list", "tasks.list", "", ""); err == nil {
		t.Error("expected error when no writable target is configured")
	}

	if got, err := ResolveWriteTarget("task list", "tasks.list", "My Tasks", ""); err != nil || got != "My Tasks" {
		t.Errorf("default target = %q, %v", got, err)
	}

	if got, err := ResolveWriteTarget("task list", "tasks.list", "My Tasks", "my tasks"); err != nil || got != "My Tasks" {
		t.Errorf("matching target = %q, %v", got, err)
	}

	if _, err := ResolveWriteTarget("directory", "drive.dir", "Inbox", "Someone Else's Folder"); err == nil {
		t.Error("expected other targets to be rejected as read-only")
	}
}
