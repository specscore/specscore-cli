//go:build windows

package cli

import (
	"os"
	"path/filepath"
	"testing"
)

// TestReleaseLifecycleLockedFile_RetainsPathAfterClose guards the Windows
// successor race. The release path must not call Remove after closing its
// handle: another process can create and lock a successor at that pathname in
// the gap. An unlocked lock file is harmless because every acquisition locks
// the opened handle, so retaining it is the safe behaviour.
func TestReleaseLifecycleLockedFile_RetainsPathAfterClose(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), ".specscore-lifecycle.lock")
	file, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := acquireLifecycleFileLock(file); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	originalRemove := transactionRemove
	transactionRemove = func(string) error {
		t.Fatal("Windows lock release attempted pathname unlink after close")
		return nil
	}
	t.Cleanup(func() { transactionRemove = originalRemove })

	if err := releaseLifecycleLockedFile(lockPath, file); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(lockPath); err != nil || info.IsDir() {
		t.Fatalf("released Windows lock pathname was not retained: info=%v err=%v", info, err)
	}

	successor, err := os.OpenFile(lockPath, os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := acquireLifecycleFileLock(successor); err != nil {
		_ = successor.Close()
		t.Fatalf("retained unlocked lock file cannot be acquired: %v", err)
	}
	if err := releaseLifecycleLockedFile(lockPath, successor); err != nil {
		t.Fatal(err)
	}
}
