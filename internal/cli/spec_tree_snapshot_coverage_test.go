//go:build darwin || linux

package cli

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestRetainedLifecycleLockBranches(t *testing.T) {
	t.Run("native lock and release", func(t *testing.T) {
		resetSpecTreeSnapshotSeams(t)
		root := t.TempDir()
		lockPath, lockFile, err := acquireLifecycleLock(root)
		if err != nil {
			t.Fatal(err)
		}
		if _, _, err := acquireLifecycleLock(root); err == nil || !strings.Contains(err.Error(), "another SpecScore lifecycle transaction") {
			t.Fatalf("second acquire error = %v", err)
		}
		if err := releaseLifecycleLockFile(lockPath, lockFile); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("native lock errors", func(t *testing.T) {
		resetSpecTreeSnapshotSeams(t)
		root := t.TempDir()
		transactionLockFile = func(*os.File) error { return errors.New("lock failed") }
		if _, _, err := acquireLifecycleLock(root); err == nil || !strings.Contains(err.Error(), "lock failed") {
			t.Fatalf("acquire error = %v", err)
		}

		file, err := os.CreateTemp(root, "closed-lock")
		if err != nil {
			t.Fatal(err)
		}
		if err := file.Close(); err != nil {
			t.Fatal(err)
		}
		if err := acquireLifecycleFileLock(file); err == nil {
			t.Fatal("locking a closed descriptor succeeded")
		}
		if err := releaseLifecycleLockedFile(file.Name(), file); err == nil {
			t.Fatal("releasing a closed descriptor succeeded")
		}
	})

	t.Run("open and inspection failures", func(t *testing.T) {
		resetSpecTreeSnapshotSeams(t)
		root := t.TempDir()
		transactionOpenFile = func(string, int, os.FileMode) (*os.File, error) { return nil, errors.New("open failed") }
		if _, _, err := acquireLifecycleLock(root); err == nil || !strings.Contains(err.Error(), "open failed") {
			t.Fatalf("missing-lock acquire error = %v", err)
		}
		transactionLstat = func(string) (os.FileInfo, error) { return nil, errors.New("inspection failed") }
		if _, _, err := acquireLifecycleLock(root); err == nil || !strings.Contains(err.Error(), "inspection failed") {
			t.Fatalf("inspection error = %v", err)
		}
	})

	t.Run("legacy lock shapes", func(t *testing.T) {
		resetSpecTreeSnapshotSeams(t)
		root := t.TempDir()
		lockPath := filepath.Join(root, ".specscore-lifecycle.lock")
		if got := lifecycleLockOwnerPath(lockPath); got != filepath.Join(lockPath, "owner") {
			t.Fatalf("owner path = %q", got)
		}

		regular := filepath.Join(root, "regular")
		if err := os.WriteFile(regular, []byte("lock"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := legacyLifecycleLockInfo(regular); err == nil || !strings.Contains(err.Error(), "not a directory") {
			t.Fatalf("regular lock error = %v", err)
		}
		symlink := filepath.Join(root, "symlink")
		if err := os.Symlink(regular, symlink); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		if _, err := legacyLifecycleLockInfo(symlink); err == nil || !strings.Contains(err.Error(), "symlinked") {
			t.Fatalf("symlink lock error = %v", err)
		}

		if err := os.Mkdir(lockPath, 0o700); err != nil {
			t.Fatal(err)
		}
		if stale, err := lifecycleLegacyLockIsStale(lockPath); err != nil || stale {
			t.Fatalf("ownerless legacy lock = stale %v, err %v", stale, err)
		}
		ownerPath := lifecycleLockOwnerPath(lockPath)
		if err := os.Symlink(regular, ownerPath); err != nil {
			t.Fatal(err)
		}
		if _, err := lifecycleLegacyLockIsStale(lockPath); err == nil || !strings.Contains(err.Error(), "non-regular") {
			t.Fatalf("symlink owner error = %v", err)
		}
		if err := os.Remove(ownerPath); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(ownerPath, []byte("invalid\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if stale, err := lifecycleLegacyLockIsStale(lockPath); err != nil || !stale {
			t.Fatalf("invalid owner = stale %v, err %v", stale, err)
		}
		if err := os.WriteFile(ownerPath, []byte("123\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		transactionProcessAlive = func(pid int) bool { return pid == 123 }
		if stale, err := lifecycleLegacyLockIsStale(lockPath); err != nil || stale {
			t.Fatalf("live owner = stale %v, err %v", stale, err)
		}
		transactionProcessAlive = func(int) bool { return false }
		if stale, err := lifecycleLegacyLockIsStale(lockPath); err != nil || !stale {
			t.Fatalf("dead owner = stale %v, err %v", stale, err)
		}

		transactionOpenFile = func(string, int, os.FileMode) (*os.File, error) { return nil, errors.New("directory lock") }
		if _, _, err := acquireLifecycleLock(root); err == nil || !strings.Contains(err.Error(), "manual recovery") {
			t.Fatalf("stale legacy acquire error = %v", err)
		}
		transactionProcessAlive = func(int) bool { return true }
		if _, _, err := acquireLifecycleLock(root); err == nil || !strings.Contains(err.Error(), "another SpecScore") {
			t.Fatalf("active legacy acquire error = %v", err)
		}
	})

	t.Run("legacy owner inspection and read failures", func(t *testing.T) {
		resetSpecTreeSnapshotSeams(t)
		root := t.TempDir()
		lockPath := filepath.Join(root, ".specscore-lifecycle.lock")
		if err := os.Mkdir(lockPath, 0o700); err != nil {
			t.Fatal(err)
		}
		ownerPath := lifecycleLockOwnerPath(lockPath)
		if err := os.WriteFile(ownerPath, []byte("1\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		originalLstat := transactionLstat
		transactionLstat = func(path string) (os.FileInfo, error) {
			if path == ownerPath {
				return nil, errors.New("owner stat failed")
			}
			return originalLstat(path)
		}
		if _, err := lifecycleLegacyLockIsStale(lockPath); err == nil || !strings.Contains(err.Error(), "owner stat failed") {
			t.Fatalf("owner stat error = %v", err)
		}
		transactionLstat = originalLstat
		transactionReadFile = func(string) ([]byte, error) { return nil, errors.New("owner read failed") }
		if _, err := lifecycleLegacyLockIsStale(lockPath); err == nil || !strings.Contains(err.Error(), "owner read failed") {
			t.Fatalf("owner read error = %v", err)
		}
	})

	t.Run("legacy stale inspection failure during acquire", func(t *testing.T) {
		resetSpecTreeSnapshotSeams(t)
		root := t.TempDir()
		lockPath := filepath.Join(root, ".specscore-lifecycle.lock")
		if err := os.Mkdir(lockPath, 0o700); err != nil {
			t.Fatal(err)
		}
		originalLstat := transactionLstat
		calls := 0
		transactionLstat = func(path string) (os.FileInfo, error) {
			if path == lockPath {
				calls++
				if calls == 2 {
					return nil, errors.New("stale inspection failed")
				}
			}
			return originalLstat(path)
		}
		if _, err := lifecycleLegacyLockIsStale(filepath.Join(root, "missing")); err == nil {
			t.Fatal("missing legacy lock accepted")
		}
		transactionOpenFile = func(string, int, os.FileMode) (*os.File, error) { return nil, errors.New("directory lock") }
		if _, _, err := acquireLifecycleLock(root); err == nil || !strings.Contains(err.Error(), "stale inspection failed") {
			t.Fatalf("stale inspection error = %v", err)
		}
	})

	t.Run("close seam failure", func(t *testing.T) {
		resetSpecTreeSnapshotSeams(t)
		file, err := os.CreateTemp(t.TempDir(), "lock")
		if err != nil {
			t.Fatal(err)
		}
		transactionCloseFile = func(*os.File) error { return errors.New("close failed") }
		t.Cleanup(func() { _ = file.Close() })
		if err := releaseLifecycleLockFile(file.Name(), file); err == nil || !strings.Contains(err.Error(), "close failed") {
			t.Fatalf("release error = %v", err)
		}
	})

	if !defaultProcessAlive(os.Getpid()) {
		t.Fatal("current process reported dead")
	}
	if defaultProcessAlive(99999999) {
		t.Fatal("impossible process reported alive")
	}
}

func TestRetainedSpecTreeSnapshotUtilityBranches(t *testing.T) {
	if err := validateSpecTreeSnapshot(specTreeSnapshot{}); err == nil {
		t.Fatal("rootless snapshot accepted")
	}
	if err := validateSpecTreeSnapshot(rootSnapshot(nil, "../escape")); err == nil {
		t.Fatal("invalid directory path accepted")
	}
	invalidFile := rootSnapshot(map[string]string{"../escape": "x"})
	if err := validateSpecTreeSnapshot(invalidFile); err == nil {
		t.Fatal("invalid file path accepted")
	}
	if got := unionSnapshotPaths([]string{"b", "a"}, []string{"a", "c"}); strings.Join(got, ",") != "a,b,c" {
		t.Fatalf("union = %v", got)
	}
	before := rootSnapshot(map[string]string{"a": "one"})
	after := rootSnapshot(map[string]string{"a": "one"}, "new")
	if got := changedSnapshotDirectories(before, after); len(got) != 1 || got[0] != "new" {
		t.Fatalf("changed directories = %v", got)
	}
	if specTreeSnapshotsEqual(before, after) {
		t.Fatal("different snapshot lengths compare equal")
	}
	changedFile := rootSnapshot(map[string]string{"a": "two"})
	if specTreeSnapshotsEqual(before, changedFile) {
		t.Fatal("different file content compares equal")
	}
	changedRoot := rootSnapshot(map[string]string{"a": "one"})
	root := changedRoot.directories["."]
	root.mode = os.ModeDir | 0o700
	changedRoot.directories["."] = root
	if specTreeSnapshotsEqual(before, changedRoot) {
		t.Fatal("different directory metadata compares equal")
	}

	t.Run("staging helpers", func(t *testing.T) {
		resetSpecTreeSnapshotSeams(t)
		root := t.TempDir()
		if _, err := openStagedSpecTreeSnapshot(root, specTreeSnapshot{}); err == nil {
			t.Fatal("rootless staged snapshot accepted")
		}
		transactionMkdirTemp = func(string, string) (string, error) { return "", errors.New("mkdir failed") }
		if _, err := openStagedSpecTreeSnapshot(root, rootSnapshot(nil)); err == nil || !strings.Contains(err.Error(), "mkdir failed") {
			t.Fatalf("stage mkdir error = %v", err)
		}
	})

	t.Run("stage path and materialize failures", func(t *testing.T) {
		resetSpecTreeSnapshotSeams(t)
		root := t.TempDir()
		stagePath, err := stageSpecTreeSnapshot(root, rootSnapshot(nil))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.RemoveAll(stagePath); err != nil {
			t.Fatal(err)
		}
		transactionMkdirTemp = func(string, string) (string, error) { return "", errors.New("stage open failed") }
		if _, err := stageSpecTreeSnapshot(root, rootSnapshot(nil)); err == nil || !strings.Contains(err.Error(), "stage open failed") {
			t.Fatalf("stage helper error = %v", err)
		}
		fileRoot := filepath.Join(root, "file-root")
		if err := os.WriteFile(fileRoot, []byte("x"), 0o600); err != nil {
			t.Fatal(err)
		}
		if err := materializeSpecSnapshot(fileRoot, rootSnapshot(nil)); err == nil {
			t.Fatal("materialization through file root succeeded")
		}
	})
}

func TestRetainedDescriptorSnapshotBranches(t *testing.T) {
	if _, err := snapshotSpecTreeNoFollow(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing snapshot root accepted")
	}
	stage, err := openStagedSpecTreeNoFollow(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = closeStagedSpecTree(stage) })
	if same, err := stagedSpecTreeMatchesPath(stage); err != nil || !same {
		t.Fatalf("stage path match = %v, %v", same, err)
	}
	if same, err := stagedSpecTreePublishedAt(stage, stage.path); err != nil || !same {
		t.Fatalf("published path match = %v, %v", same, err)
	}

	t.Run("descriptor read failure", func(t *testing.T) {
		resetSpecTreeSnapshotSeams(t)
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("body"), 0o600); err != nil {
			t.Fatal(err)
		}
		original := transactionReadSnapshotFile
		t.Cleanup(func() { transactionReadSnapshotFile = original })
		transactionReadSnapshotFile = func(io.Reader) ([]byte, error) { return []byte("partial"), errors.New("read failed") }
		if _, err := snapshotSpecTreeNoFollow(root); err == nil || !strings.Contains(err.Error(), "read failed") {
			t.Fatalf("snapshot read error = %v", err)
		}
	})

	t.Run("platform flag capture failure", func(t *testing.T) {
		resetSnapshotNoFollowSeams(t)
		file, err := os.CreateTemp(t.TempDir(), "entry")
		if err != nil {
			t.Fatal(err)
		}
		defer file.Close()
		info, err := file.Stat()
		if err != nil {
			t.Fatal(err)
		}
		snapshotPlatformFlagsForEntry = func(int, os.FileInfo) (uint32, error) { return 0, errors.New("flags failed") }
		if _, err := captureSnapshotEntryMetadata(file, info); err == nil || !strings.Contains(err.Error(), "flags failed") {
			t.Fatalf("platform flags error = %v", err)
		}
	})
}
