package node_test

import (
	"errors"
	"syscall"
	"testing"
)

func skipIfNoMount(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	if err := syscall.Mount("tmpfs", dir, "tmpfs", 0, ""); err != nil {
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
			t.Skip("mount not permitted on this system")
		}
		t.Fatalf("mount probe failed: %v", err)
	}
	_ = syscall.Unmount(dir, 0)
}
