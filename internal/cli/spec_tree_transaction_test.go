package cli

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/issue"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
	"github.com/specscore/specscore-cli/pkg/lint"
)

func TestSpecTreeSnapshotRestore_RestoresExactFilesAndRemovesNewFiles(t *testing.T) {
	root := t.TempDir()
	originalPath := filepath.Join(root, "nested", "original.md")
	if err := os.MkdirAll(filepath.Dir(originalPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(originalPath, []byte("original bytes\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	emptyOriginalDirectory := filepath.Join(root, "empty-original-directory")
	if err := os.MkdirAll(emptyOriginalDirectory, 0o755); err != nil {
		t.Fatal(err)
	}

	before, err := snapshotSpecTreeForTransaction(root)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if err := os.RemoveAll(filepath.Join(root, "nested")); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(emptyOriginalDirectory); err != nil {
		t.Fatal(err)
	}
	createdPath := filepath.Join(root, "lint-created", "nested", "index.md")
	if err := os.MkdirAll(filepath.Dir(createdPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(createdPath, []byte("created by lint\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := before.restore(root); err != nil {
		t.Fatalf("restore: %v", err)
	}
	after, err := snapshotSpecTreeForTransaction(root)
	if err != nil {
		t.Fatalf("snapshot after restore: %v", err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("restored snapshot differs\nwant: %#v\n got: %#v", before, after)
	}
	if _, err := os.Stat(createdPath); !os.IsNotExist(err) {
		t.Fatalf("lint-created file remains after restore: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(createdPath)); !os.IsNotExist(err) {
		t.Fatalf("lint-created directory remains after restore: %v", err)
	}
	if info, err := os.Stat(emptyOriginalDirectory); err != nil || !info.IsDir() {
		t.Fatalf("original empty directory was not recreated: info=%v err=%v", info, err)
	}
}

func TestSpecTreeSnapshotReadAndWalkFailures(t *testing.T) {
	t.Run("walk failure", func(t *testing.T) {
		_, err := snapshotSpecTreeForTransaction(filepath.Join(t.TempDir(), "missing"))
		if err == nil || !strings.Contains(err.Error(), "snapshotting spec tree") {
			t.Fatalf("error = %v, want wrapped snapshot failure", err)
		}
	})

	t.Run("read failure", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("body"), 0o644); err != nil {
			t.Fatal(err)
		}
		originalRead := transactionReadFile
		transactionReadFile = func(string) ([]byte, error) { return nil, errors.New("forced read failure") }
		t.Cleanup(func() { transactionReadFile = originalRead })

		_, err := snapshotSpecTreeForTransaction(root)
		if err == nil || !strings.Contains(err.Error(), "forced read failure") {
			t.Fatalf("error = %v, want forced read failure", err)
		}
	})

	t.Run("non-regular files are excluded", func(t *testing.T) {
		root := t.TempDir()
		link := filepath.Join(root, "link")
		if err := os.Symlink("not-followed", link); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		snapshot, err := snapshotSpecTreeForTransaction(root)
		if err != nil {
			t.Fatal(err)
		}
		if len(snapshot.files) != 0 {
			t.Fatalf("non-regular link entered file snapshot: %#v", snapshot.files)
		}
		if err := snapshot.restore(root); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(link); err != nil {
			t.Fatalf("non-regular link should remain outside transaction state: %v", err)
		}
	})
}

func TestSpecTreeSnapshotRestore_RefusesSymlinkReplacement(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "nested", "tracked.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot, err := snapshotSpecTreeForTransaction(root)
	if err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(t.TempDir(), "external.md")
	if err := os.WriteFile(external, []byte("external bytes\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(external, path); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	if err := snapshot.restore(root); err == nil || !strings.Contains(err.Error(), "symlink") {
		t.Fatalf("restore error = %v, want symlink refusal", err)
	}
	if got, err := os.ReadFile(external); err != nil || string(got) != "external bytes\n" {
		t.Fatalf("rollback followed replacement symlink: %q, %v", got, err)
	}
}

func TestTransactionSafePathAndManifestApplication(t *testing.T) {
	t.Run("safe path rejects unsafe roots and components", func(t *testing.T) {
		root := t.TempDir()
		if _, err := transactionSafePath(filepath.Join(root, "missing"), "file.md"); err == nil {
			t.Fatal("missing root accepted")
		}
		if _, err := transactionSafePath(root, "../outside.md"); err == nil {
			t.Fatal("parent escape accepted")
		}
		component := filepath.Join(root, "component")
		if err := os.WriteFile(component, []byte("not a dir"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := transactionSafePath(root, filepath.Join("component", "child.md")); err == nil {
			t.Fatal("regular path component accepted")
		}
		link := filepath.Join(root, "link")
		if err := os.Symlink(t.TempDir(), link); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		if _, err := transactionSafePath(root, filepath.Join("link", "child.md")); err == nil {
			t.Fatal("symlink component accepted")
		}
	})

	t.Run("manifest adds rewrites and removes exact paths", func(t *testing.T) {
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, "old"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, "old", "remove.md"), []byte("remove"), 0o644); err != nil {
			t.Fatal(err)
		}
		before, err := snapshotSpecTreeForTransaction(root)
		if err != nil {
			t.Fatal(err)
		}
		after := specTreeSnapshot{
			directories: map[string]os.FileMode{".": before.directories["."], "new": os.ModeDir | 0o700},
			files:       map[string]specTreeFile{"new/index.md": {content: []byte("new"), mode: 0o600}},
		}
		if err := applySpecSnapshotDiff(root, before, after); err != nil {
			t.Fatal(err)
		}
		got, err := snapshotSpecTreeForTransaction(root)
		if err != nil || !reflect.DeepEqual(got, after) {
			t.Fatalf("manifest result = %#v, %v; want %#v", got, err, after)
		}
	})

	t.Run("materialize creates an exact private copy", func(t *testing.T) {
		root := t.TempDir()
		source := filepath.Join(root, "source")
		if err := os.MkdirAll(filepath.Join(source, "nested"), 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(source, "nested", "file.md"), []byte("source"), 0o600); err != nil {
			t.Fatal(err)
		}
		snapshot, err := snapshotSpecTreeForTransaction(source)
		if err != nil {
			t.Fatal(err)
		}
		destination := filepath.Join(root, "destination")
		if err := os.MkdirAll(destination, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := materializeSpecSnapshot(destination, snapshot); err != nil {
			t.Fatal(err)
		}
		got, err := snapshotSpecTreeForTransaction(destination)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got.files, snapshot.files) || !reflect.DeepEqual(got.directories["nested"], snapshot.directories["nested"]) {
			t.Fatalf("materialized = %#v; want nested/files from %#v", got, snapshot)
		}
	})

	t.Run("manifest and materialization surface filesystem failures", func(t *testing.T) {
		root := t.TempDir()
		fileRoot := filepath.Join(root, "file-root")
		if err := os.WriteFile(fileRoot, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := materializeSpecSnapshot(fileRoot, specTreeSnapshot{directories: map[string]os.FileMode{"nested": os.ModeDir | 0o755}, files: map[string]specTreeFile{}}); err == nil {
			t.Fatal("materialization through a file root succeeded")
		}
		if err := applySpecSnapshotDiff(root, specTreeSnapshot{directories: map[string]os.FileMode{".": os.ModeDir | 0o755}, files: map[string]specTreeFile{}}, specTreeSnapshot{directories: map[string]os.FileMode{".": os.ModeDir | 0o755}, files: map[string]specTreeFile{"../escape.md": {content: []byte("x"), mode: 0o644}}}); err == nil {
			t.Fatal("manifest path escape succeeded")
		}
	})
}

func TestSpecTreeTransaction_ErrorSeamsFailClosed(t *testing.T) {
	t.Run("invalid owned path prevents transaction", func(t *testing.T) {
		if _, err := beginSpecTreeTransaction(t.TempDir(), ".."); err == nil {
			t.Fatal("invalid owned path succeeded")
		}
	})
	t.Run("isolated lint pre-snapshot failure", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("body"), 0o644); err != nil {
			t.Fatal(err)
		}
		snapshot, err := snapshotSpecTreeForTransaction(root)
		if err != nil {
			t.Fatal(err)
		}
		originalRead := transactionReadFile
		transactionReadFile = func(string) ([]byte, error) { return nil, errors.New("read failed") }
		t.Cleanup(func() { transactionReadFile = originalRead })
		transaction := &specTreeTransaction{specRoot: root, snapshot: snapshot}
		if err := transaction.postMutationHookWithLint(func(string) error { return nil })(); err == nil || !strings.Contains(err.Error(), "read failed") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestSpecTreeTransaction_LockExcludesConcurrentLifecycleCommands(t *testing.T) {
	root := stagePlan(t, "auth", "Draft")
	specRoot := filepath.Join(root, "spec")
	before, err := snapshotSpecTreeForTransaction(specRoot)
	if err != nil {
		t.Fatal(err)
	}
	transaction, err := beginSpecTreeTransaction(specRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = transaction.release() })

	_, _, err = runPlan(t, "change-status", "auth", "--to=in review")
	if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Fatalf("concurrent command exit = %d, want %d; err=%v", got, exitcode.Unexpected, err)
	}
	after, err := snapshotSpecTreeForTransaction(specRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("rejected concurrent lifecycle command mutated the spec tree\nwant: %#v\n got: %#v", before, after)
	}
	if err := transaction.release(); err != nil {
		t.Fatal(err)
	}
	if next, err := beginSpecTreeTransaction(specRoot); err != nil {
		t.Fatalf("lock was not released for the next transaction: %v", err)
	} else if err := next.release(); err != nil {
		t.Fatal(err)
	}
}

func TestSpecTreeTransaction_LockFailuresAreReportedAndCleanedUp(t *testing.T) {
	t.Run("acquire", func(t *testing.T) {
		specRoot := filepath.Join(t.TempDir(), "spec")
		if err := os.MkdirAll(specRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		originalOpenFile := transactionOpenFile
		transactionOpenFile = func(string, int, os.FileMode) (*os.File, error) {
			return nil, errors.New("forced lock acquire failure")
		}
		t.Cleanup(func() { transactionOpenFile = originalOpenFile })

		if _, err := beginSpecTreeTransaction(specRoot); err == nil || !strings.Contains(err.Error(), "forced lock acquire failure") {
			t.Fatalf("error = %v, want lock acquire failure", err)
		}
	})

	t.Run("snapshot failure releases lock", func(t *testing.T) {
		root := t.TempDir()
		specRoot := filepath.Join(root, "spec")
		if err := os.MkdirAll(specRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(specRoot, "README.md"), []byte("body"), 0o644); err != nil {
			t.Fatal(err)
		}
		originalRead := transactionReadFile
		transactionReadFile = func(string) ([]byte, error) { return nil, errors.New("forced snapshot failure") }
		t.Cleanup(func() { transactionReadFile = originalRead })

		if _, err := beginSpecTreeTransaction(specRoot); err == nil || !strings.Contains(err.Error(), "forced snapshot failure") {
			t.Fatalf("error = %v, want snapshot failure", err)
		}
		if _, err := os.Stat(filepath.Join(root, ".specscore-lifecycle.lock")); !os.IsNotExist(err) {
			t.Fatalf("snapshot failure left lifecycle lock behind: %v", err)
		}
	})

	t.Run("release", func(t *testing.T) {
		specRoot := filepath.Join(t.TempDir(), "spec")
		if err := os.MkdirAll(specRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		transaction, err := beginSpecTreeTransaction(specRoot)
		if err != nil {
			t.Fatal(err)
		}
		originalRemove := transactionRemove
		transactionRemove = func(string) error { return errors.New("forced lock release failure") }
		t.Cleanup(func() { transactionRemove = originalRemove })

		if err := transaction.release(); err == nil || !strings.Contains(err.Error(), "forced lock release failure") {
			t.Fatalf("error = %v, want lock release failure", err)
		}
	})

	t.Run("stale owner is reclaimed", func(t *testing.T) {
		root := t.TempDir()
		specRoot := filepath.Join(root, "spec")
		if err := os.MkdirAll(specRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		lockPath := filepath.Join(root, ".specscore-lifecycle.lock")
		if err := os.Mkdir(lockPath, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(lifecycleLockOwnerPath(lockPath), []byte("999999\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		originalProcessAlive := transactionProcessAlive
		transactionProcessAlive = func(pid int) bool {
			if pid != 999999 {
				t.Fatalf("stale lock checked unexpected pid %d", pid)
			}
			return false
		}
		t.Cleanup(func() { transactionProcessAlive = originalProcessAlive })

		transaction, err := beginSpecTreeTransaction(specRoot)
		if err != nil {
			t.Fatalf("stale lock prevented transaction: %v", err)
		}
		if err := transaction.release(); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("stale file from a crashed transaction is not a blocker", func(t *testing.T) {
		root := t.TempDir()
		specRoot := filepath.Join(root, "spec")
		if err := os.MkdirAll(specRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		lockPath := filepath.Join(root, ".specscore-lifecycle.lock")
		if err := os.WriteFile(lockPath, []byte("abandoned lock metadata\n"), 0o600); err != nil {
			t.Fatal(err)
		}

		transaction, err := beginSpecTreeTransaction(specRoot)
		if err != nil {
			t.Fatalf("stale lock file prevented transaction: %v", err)
		}
		if err := transaction.release(); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
			t.Fatalf("released lock file remains: %v", err)
		}
	})
}

func TestLifecycleLockRecoveryPaths(t *testing.T) {
	newLegacyLock := func(t *testing.T, owner string) (string, string) {
		t.Helper()
		root := t.TempDir()
		lockPath := filepath.Join(root, ".specscore-lifecycle.lock")
		if err := os.Mkdir(lockPath, 0o700); err != nil {
			t.Fatal(err)
		}
		if owner != "" {
			if err := os.WriteFile(lifecycleLockOwnerPath(lockPath), []byte(owner), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		return root, lockPath
	}

	t.Run("file lock failure other than contention", func(t *testing.T) {
		root := t.TempDir()
		originalLock := transactionLockFile
		transactionLockFile = func(*os.File) error { return errors.New("forced file-lock failure") }
		t.Cleanup(func() { transactionLockFile = originalLock })

		_, _, err := acquireLifecycleLock(root)
		if err == nil || !strings.Contains(err.Error(), "forced file-lock failure") {
			t.Fatalf("error = %v, want file-lock failure", err)
		}
	})

	t.Run("legacy lock and owner symlinks or non-directories are refused", func(t *testing.T) {
		root := t.TempDir()
		lockPath := filepath.Join(root, ".specscore-lifecycle.lock")
		if err := os.WriteFile(lockPath, []byte("not a directory"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := legacyLifecycleLockInfo(lockPath); err == nil || !strings.Contains(err.Error(), "not a directory") {
			t.Fatalf("non-directory lock error = %v", err)
		}
		if err := os.Remove(lockPath); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(root, lockPath); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		if _, err := legacyLifecycleLockInfo(lockPath); err == nil || !strings.Contains(err.Error(), "symlinked") {
			t.Fatalf("symlink lock error = %v", err)
		}
		if err := os.Remove(lockPath); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(lockPath, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(root, "missing"), lifecycleLockOwnerPath(lockPath)); err != nil {
			t.Skipf("owner symlink unavailable: %v", err)
		}
		if _, err := lifecycleLegacyLockIsStale(lockPath); err == nil || !strings.Contains(err.Error(), "non-regular") {
			t.Fatalf("symlink owner error = %v", err)
		}
	})

	t.Run("legacy owner read failure", func(t *testing.T) {
		root, _ := newLegacyLock(t, "42\n")
		originalRead := transactionReadFile
		transactionReadFile = func(string) ([]byte, error) { return nil, errors.New("forced owner read failure") }
		t.Cleanup(func() { transactionReadFile = originalRead })
		if _, _, err := acquireLifecycleLock(root); err == nil || !strings.Contains(err.Error(), "forced owner read failure") {
			t.Fatalf("error = %v, want owner read failure", err)
		}
	})

	t.Run("active legacy owner is not removed", func(t *testing.T) {
		root, _ := newLegacyLock(t, "42\n")
		originalProcessAlive := transactionProcessAlive
		transactionProcessAlive = func(int) bool { return true }
		t.Cleanup(func() { transactionProcessAlive = originalProcessAlive })
		if _, _, err := acquireLifecycleLock(root); err == nil || !strings.Contains(err.Error(), "another SpecScore lifecycle transaction") {
			t.Fatalf("error = %v, want active transaction", err)
		}
	})

	t.Run("stale legacy removal failure is reported", func(t *testing.T) {
		root, _ := newLegacyLock(t, "invalid\n")
		originalRemove := transactionRemove
		transactionRemove = func(string) error { return errors.New("forced stale lock removal failure") }
		t.Cleanup(func() { transactionRemove = originalRemove })
		if _, _, err := acquireLifecycleLock(root); err == nil || !strings.Contains(err.Error(), "forced stale lock removal failure") {
			t.Fatalf("error = %v, want stale lock removal failure", err)
		}
	})

	t.Run("legacy lock recreated twice is reported", func(t *testing.T) {
		root, lockPath := newLegacyLock(t, "999999\n")
		originalProcessAlive := transactionProcessAlive
		transactionProcessAlive = func(int) bool { return false }
		t.Cleanup(func() { transactionProcessAlive = originalProcessAlive })
		originalOpen := transactionOpenFile
		calls := 0
		transactionOpenFile = func(string, int, os.FileMode) (*os.File, error) {
			calls++
			if calls == 2 {
				if err := os.Mkdir(lockPath, 0o700); err != nil {
					t.Fatal(err)
				}
			}
			return nil, &os.PathError{Op: "open", Path: lockPath, Err: os.ErrExist}
		}
		t.Cleanup(func() { transactionOpenFile = originalOpen })
		if _, _, err := acquireLifecycleLock(root); err == nil || !strings.Contains(err.Error(), "another SpecScore lifecycle transaction") {
			t.Fatalf("error = %v, want recreated active-lock error", err)
		}
	})

	t.Run("legacy stale forms and release failures", func(t *testing.T) {
		_, missingOwnerLock := newLegacyLock(t, "")
		if stale, err := lifecycleLegacyLockIsStale(missingOwnerLock); err != nil || stale {
			t.Fatalf("missing owner stale = %v, %v; want false, nil", stale, err)
		}
		_, malformedOwnerLock := newLegacyLock(t, "not-a-pid\n")
		if stale, err := lifecycleLegacyLockIsStale(malformedOwnerLock); err != nil || !stale {
			t.Fatalf("malformed owner stale = %v, %v; want true, nil", stale, err)
		}

		_, ownerFailureLock := newLegacyLock(t, "42\n")
		originalRemove := transactionRemove
		transactionRemove = func(string) error { return errors.New("forced owner removal failure") }
		if err := releaseLegacyLifecycleLock(ownerFailureLock); err == nil || !strings.Contains(err.Error(), "forced owner removal failure") {
			t.Fatalf("error = %v, want owner removal failure", err)
		}
		transactionRemove = func(path string) error {
			if filepath.Base(path) == "owner" {
				return originalRemove(path)
			}
			return errors.New("forced lock directory removal failure")
		}
		if err := releaseLegacyLifecycleLock(ownerFailureLock); err == nil || !strings.Contains(err.Error(), "forced lock directory removal failure") {
			t.Fatalf("error = %v, want lock directory removal failure", err)
		}
		transactionRemove = originalRemove
	})

	t.Run("legacy recovery refuses ownerless and active locks", func(t *testing.T) {
		_, ownerless := newLegacyLock(t, "")
		if err := releaseLegacyLifecycleLock(ownerless); err == nil || !strings.Contains(err.Error(), "ownerless") {
			t.Fatalf("ownerless release error = %v", err)
		}
		_, active := newLegacyLock(t, "42\n")
		originalAlive := transactionProcessAlive
		transactionProcessAlive = func(int) bool { return true }
		t.Cleanup(func() { transactionProcessAlive = originalAlive })
		if err := releaseLegacyLifecycleLock(active); err == nil || !strings.Contains(err.Error(), "active") {
			t.Fatalf("active release error = %v", err)
		}
	})

	t.Run("close failure is returned after unlink", func(t *testing.T) {
		root := t.TempDir()
		lockPath := filepath.Join(root, ".specscore-lifecycle.lock")
		lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		originalClose := transactionCloseFile
		transactionCloseFile = func(*os.File) error { return errors.New("forced close failure") }
		t.Cleanup(func() {
			transactionCloseFile = originalClose
			_ = lockFile.Close()
		})
		if err := releaseLifecycleLockFile(lockPath, lockFile); err == nil || !strings.Contains(err.Error(), "forced close failure") {
			t.Fatalf("error = %v, want close failure", err)
		}
	})

	t.Run("unlink failure still closes file", func(t *testing.T) {
		root := t.TempDir()
		lockPath := filepath.Join(root, ".specscore-lifecycle.lock")
		lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		originalRemove := transactionRemove
		originalClose := transactionCloseFile
		closed := false
		transactionRemove = func(string) error { return errors.New("forced unlink failure") }
		transactionCloseFile = func(*os.File) error {
			closed = true
			return nil
		}
		t.Cleanup(func() {
			transactionRemove = originalRemove
			transactionCloseFile = originalClose
			_ = lockFile.Close()
		})
		if err := releaseLifecycleLockFile(lockPath, lockFile); err == nil || !strings.Contains(err.Error(), "forced unlink failure") {
			t.Fatalf("error = %v, want unlink failure", err)
		}
		if !closed {
			t.Fatal("release did not close the lock file after unlink failure")
		}
	})

	t.Run("stale recovery refuses a replaced owner immediately before removal", func(t *testing.T) {
		_, lockPath := newLegacyLock(t, "999999\n")
		ownerPath := lifecycleLockOwnerPath(lockPath)
		originalAlive := transactionProcessAlive
		transactionProcessAlive = func(pid int) bool { return pid == 42 }
		t.Cleanup(func() { transactionProcessAlive = originalAlive })
		originalLstat := transactionLstat
		ownerChecks := 0
		transactionLstat = func(path string) (os.FileInfo, error) {
			if path == ownerPath {
				ownerChecks++
				// releaseLegacyLifecycleLock checks this identity once after
				// stale detection and once immediately before removing it.
				if ownerChecks == 3 {
					if err := os.Remove(ownerPath); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(ownerPath, []byte("42\n"), 0o600); err != nil {
						t.Fatal(err)
					}
				}
			}
			return originalLstat(path)
		}
		t.Cleanup(func() { transactionLstat = originalLstat })
		if err := releaseLegacyLifecycleLock(lockPath); err == nil || !strings.Contains(err.Error(), "owner was replaced") {
			t.Fatalf("release error = %v, want replaced-owner refusal", err)
		}
		if got, err := os.ReadFile(ownerPath); err != nil || string(got) != "42\n" {
			t.Fatalf("replacement owner was removed: %q, %v", got, err)
		}
	})

	t.Run("normal file lock release removes the pathname", func(t *testing.T) {
		root := t.TempDir()
		lockPath := filepath.Join(root, ".specscore-lifecycle.lock")
		lockFile, err := os.OpenFile(lockPath, os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		if err := releaseLifecycleLockFile(lockPath, lockFile); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Lstat(lockPath); !os.IsNotExist(err) {
			t.Fatalf("released lock remains: %v", err)
		}
	})
}

func TestSpecTreeTransaction_RollbackPreservesRawFilesystemChanges(t *testing.T) {
	t.Run("opaque hook cannot overwrite a competing command-source edit", func(t *testing.T) {
		root := t.TempDir()
		specRoot := filepath.Join(root, "spec")
		source := filepath.Join(specRoot, "plans", "auth.md")
		if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(source, []byte("before\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		transaction, err := beginSpecTreeTransaction(specRoot)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = transaction.release() })
		err = transaction.postMutationHookWith(func() error {
			if err := os.WriteFile(source, []byte("external command-source edit\n"), 0o644); err != nil {
				return err
			}
			return errors.New("verification failed")
		})()
		err = transaction.finish(err)
		if err == nil || !strings.Contains(err.Error(), "opaque post-mutation hook") {
			t.Fatalf("finish error = %v, want opaque ownership recovery", err)
		}
		if got, readErr := os.ReadFile(source); readErr != nil || string(got) != "external command-source edit\n" {
			t.Fatalf("competing source edit was overwritten: %q, %v", got, readErr)
		}
	})

	t.Run("isolated lint manifest preserves a raw command-source edit during lint", func(t *testing.T) {
		root := t.TempDir()
		specRoot := filepath.Join(root, "spec")
		source := filepath.Join(specRoot, "plans", "auth.md")
		if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(source, []byte("before\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		transaction, err := beginSpecTreeTransaction(specRoot)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = transaction.release() })
		err = transaction.postMutationHookWithLint(func(clone string) error {
			if err := os.WriteFile(filepath.Join(clone, "plans", "index.md"), []byte("lint manifest\n"), 0o644); err != nil {
				return err
			}
			return os.WriteFile(source, []byte("external command-source edit\n"), 0o644)
		})()
		err = transaction.finish(err)
		if err == nil || !strings.Contains(err.Error(), "rollback preserved unowned changes") {
			t.Fatalf("finish error = %v, want concurrent recovery", err)
		}
		if got, readErr := os.ReadFile(source); readErr != nil || string(got) != "external command-source edit\n" {
			t.Fatalf("competing source edit was overwritten: %q, %v", got, readErr)
		}
	})

	t.Run("unrelated raw file survives managed rollback", func(t *testing.T) {
		root := t.TempDir()
		specRoot := filepath.Join(root, "spec")
		managedPath := filepath.Join(specRoot, "README.md")
		if err := os.MkdirAll(specRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(managedPath, []byte("before\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		transaction, err := beginSpecTreeTransaction(specRoot)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = transaction.release() })

		err = transaction.postMutationHookWithLint(func(clone string) error {
			return os.WriteFile(filepath.Join(clone, "README.md"), []byte("lint changed\n"), 0o644)
		})()
		if err != nil {
			t.Fatal(err)
		}
		rawPath := filepath.Join(specRoot, "raw-writer.md")
		if err := os.WriteFile(rawPath, []byte("raw writer data\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		err = transaction.finish(errors.New("verification failed"))
		if err == nil || !strings.Contains(err.Error(), "verification failed") {
			t.Fatalf("finish error = %v, want original verification failure", err)
		}
		if got, readErr := os.ReadFile(managedPath); readErr != nil || string(got) != "before\n" {
			t.Fatalf("managed lint path = %q, %v; want original bytes", got, readErr)
		}
		if got, readErr := os.ReadFile(rawPath); readErr != nil || string(got) != "raw writer data\n" {
			t.Fatalf("raw file was changed or deleted: %q, %v", got, readErr)
		}
	})

	t.Run("competing raw edit of lint-created file is preserved and reported", func(t *testing.T) {
		root := t.TempDir()
		specRoot := filepath.Join(root, "spec")
		if err := os.MkdirAll(specRoot, 0o755); err != nil {
			t.Fatal(err)
		}
		transaction, err := beginSpecTreeTransaction(specRoot)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = transaction.release() })
		createdPath := filepath.Join(specRoot, "generated.md")

		err = transaction.postMutationHookWithLint(func(clone string) error {
			return os.WriteFile(filepath.Join(clone, "generated.md"), []byte("lint-created\n"), 0o644)
		})()
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(createdPath, []byte("raw writer replacement\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		err = transaction.finish(errors.New("verification failed"))
		if err == nil || !strings.Contains(err.Error(), "rollback preserved concurrent spec changes") || !strings.Contains(err.Error(), "generated.md") {
			t.Fatalf("finish error = %v, want actionable concurrent-change recovery error", err)
		}
		if got, readErr := os.ReadFile(createdPath); readErr != nil || string(got) != "raw writer replacement\n" {
			t.Fatalf("competing raw file was changed or deleted: %q, %v", got, readErr)
		}
	})
}

func TestSpecTreeTransaction_ExternalPreLintMutationIsPreserved(t *testing.T) {
	root := t.TempDir()
	specRoot := filepath.Join(root, "spec")
	ownedPath := filepath.Join(specRoot, "plans", "auth.md")
	externalPath := filepath.Join(specRoot, "README.md")
	if err := os.MkdirAll(filepath.Dir(ownedPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ownedPath, []byte("Draft\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(externalPath, []byte("index before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	transaction, err := beginSpecTreeTransaction(specRoot, filepath.Join("plans", "auth.md"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = transaction.release() })

	// Simulate the lifecycle package's authorized source rewrite, followed by a
	// non-cooperating editor changing an unrelated file before the lifecycle
	// package invokes its post-mutation hook.
	if err := os.WriteFile(ownedPath, []byte("In Review\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := transaction.captureLifecycleMutationState(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(externalPath, []byte("external editor update\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCalled := false
	err = transaction.postMutationHookWith(func() error {
		runCalled = true
		return errors.New("verification failed")
	})()
	if err == nil || !lifecycle.IsDeferredRollback(err) || !strings.Contains(err.Error(), "README.md") || strings.Contains(err.Error(), "plans/auth.md") {
		t.Fatalf("hook error = %v, want deferred unowned README.md recovery error", err)
	}
	if runCalled {
		t.Fatal("lint ran despite an unowned pre-lint mutation")
	}
	err = transaction.finish(err)
	if err == nil || !strings.Contains(err.Error(), "rollback preserved unowned changes") || !strings.Contains(err.Error(), "README.md") {
		t.Fatalf("finish error = %v, want actionable unowned-change recovery error", err)
	}
	if got, readErr := os.ReadFile(externalPath); readErr != nil || string(got) != "external editor update\n" {
		t.Fatalf("external pre-lint edit was overwritten: %q, %v", got, readErr)
	}
	if got, readErr := os.ReadFile(ownedPath); readErr != nil || string(got) != "In Review\n" {
		t.Fatalf("authorized lifecycle mutation should be left for manual recovery: %q, %v", got, readErr)
	}
}

func TestLifecycleOwnedPaths(t *testing.T) {
	owned, err := normalizeLifecycleOwnedPaths([]string{filepath.Join("plans", "auth.md")})
	if err != nil || !owned[filepath.Join("plans", "auth.md")] {
		t.Fatalf("normalize valid path = %#v, %v", owned, err)
	}
	for _, invalid := range []string{"", ".", "..", filepath.Join("..", "plans", "auth.md"), filepath.Join(string(os.PathSeparator), "tmp", "auth.md")} {
		if _, err := normalizeLifecycleOwnedPaths([]string{invalid}); err == nil {
			t.Fatalf("normalizeLifecycleOwnedPaths(%q) succeeded", invalid)
		}
	}

	specRoot := filepath.Join(t.TempDir(), "spec")
	inside := filepath.Join(specRoot, "plans", "auth.md")
	if got, err := relativeLifecycleOwnedPath(specRoot, inside); err != nil || got != filepath.Join("plans", "auth.md") {
		t.Fatalf("relativeLifecycleOwnedPath(inside) = %q, %v", got, err)
	}
	if _, err := relativeLifecycleOwnedPath(specRoot, filepath.Join(filepath.Dir(specRoot), "README.md")); err == nil {
		t.Fatal("relativeLifecycleOwnedPath(outside spec root) succeeded")
	}
	originalRel := transactionRel
	transactionRel = func(string, string) (string, error) { return "", errors.New("forced relative path failure") }
	t.Cleanup(func() { transactionRel = originalRel })
	if _, err := relativeLifecycleOwnedPath(specRoot, inside); err == nil || !strings.Contains(err.Error(), "forced relative path failure") {
		t.Fatalf("relativeLifecycleOwnedPath error = %v, want forced failure", err)
	}
}

// These are defensive recovery paths rather than normal user journeys. Keep
// their fault injection next to the transaction tests: every error leaves the
// live spec tree untouched or explicitly reports that manual recovery is needed.
func TestSpecTreeTransaction_DefensiveRecoveryBranches(t *testing.T) {
	t.Run("non-not-exist inspection failure is retained", func(t *testing.T) {
		root := t.TempDir()
		originalOpen, originalLstat := transactionOpenFile, transactionLstat
		transactionOpenFile = func(string, int, os.FileMode) (*os.File, error) { return nil, errors.New("open denied") }
		transactionLstat = func(string) (os.FileInfo, error) { return nil, errors.New("inspection denied") }
		t.Cleanup(func() { transactionOpenFile, transactionLstat = originalOpen, originalLstat })
		if _, _, err := acquireLifecycleLock(root); err == nil || !strings.Contains(err.Error(), "inspection denied") {
			t.Fatalf("acquire error = %v", err)
		}
	})

	t.Run("unlink and close failures are both retained", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "lock")
		file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			t.Fatal(err)
		}
		originalRemove, originalClose := transactionRemove, transactionCloseFile
		transactionRemove = func(string) error { return errors.New("unlink failed") }
		transactionCloseFile = func(*os.File) error { return errors.New("close failed") }
		t.Cleanup(func() { transactionRemove, transactionCloseFile = originalRemove, originalClose; _ = file.Close() })
		if err := releaseLifecycleLockFile(path, file); err == nil || !strings.Contains(err.Error(), "unlink failed") || !strings.Contains(err.Error(), "close failed") {
			t.Fatalf("release error = %v", err)
		}
	})

	t.Run("legacy lock recreation after both retries is surfaced", func(t *testing.T) {
		root := t.TempDir()
		lock := filepath.Join(root, ".specscore-lifecycle.lock")
		makeStale := func() {
			if err := os.Mkdir(lock, 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(lifecycleLockOwnerPath(lock), []byte("bad\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		makeStale()
		originalOpen := transactionOpenFile
		calls := 0
		transactionOpenFile = func(string, int, os.FileMode) (*os.File, error) {
			calls++
			if calls == 2 {
				makeStale()
			}
			return nil, &os.PathError{Op: "open", Path: lock, Err: os.ErrExist}
		}
		t.Cleanup(func() { transactionOpenFile = originalOpen })
		if _, _, err := acquireLifecycleLock(root); err == nil || !strings.Contains(err.Error(), "recreated concurrently") {
			t.Fatalf("acquire error = %v", err)
		}
	})

	t.Run("legacy inspection and revalidation failures are fail closed", func(t *testing.T) {
		root := t.TempDir()
		lock := filepath.Join(root, ".specscore-lifecycle.lock")
		if err := os.Mkdir(lock, 0o700); err != nil {
			t.Fatal(err)
		}
		owner := lifecycleLockOwnerPath(lock)
		if err := os.WriteFile(owner, []byte("bad\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		originalLstat := transactionLstat
		calls := 0
		transactionLstat = func(path string) (os.FileInfo, error) {
			calls++
			if calls == 1 {
				return nil, errors.New("initial inspection failure")
			}
			return originalLstat(path)
		}
		if _, err := lifecycleLegacyLockIsStale(lock); err == nil || !strings.Contains(err.Error(), "initial inspection failure") {
			t.Fatalf("stale error = %v", err)
		}
		transactionLstat = originalLstat
		calls = 0
		transactionLstat = func(path string) (os.FileInfo, error) {
			calls++
			if path == owner && calls >= 3 {
				return nil, errors.New("revalidation failure")
			}
			return originalLstat(path)
		}
		t.Cleanup(func() { transactionLstat = originalLstat })
		if err := releaseLegacyLifecycleLock(lock); err == nil || !strings.Contains(err.Error(), "revalidation failure") {
			t.Fatalf("release error = %v", err)
		}
	})

	t.Run("isolated hook detects pre and during lint writers", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("before"), 0o644); err != nil {
			t.Fatal(err)
		}
		transaction, err := beginSpecTreeTransaction(root)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = transaction.release() })
		if err := os.WriteFile(filepath.Join(root, "external.md"), []byte("writer"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := transaction.postMutationHookWithLint(func(string) error { t.Fatal("lint should not run"); return nil })(); err == nil || !lifecycle.IsDeferredRollback(err) {
			t.Fatalf("pre-lint error = %v", err)
		}
		if err := transaction.release(); err != nil {
			t.Fatal(err)
		}

		root = t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("before"), 0o644); err != nil {
			t.Fatal(err)
		}
		transaction, err = beginSpecTreeTransaction(root)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = transaction.release() })
		err = transaction.postMutationHookWithLint(func(clone string) error {
			if err := os.WriteFile(filepath.Join(clone, "README.md"), []byte("lint"), 0o644); err != nil {
				return err
			}
			return os.WriteFile(filepath.Join(root, "during.md"), []byte("writer"), 0o644)
		})()
		if err == nil || !strings.Contains(err.Error(), "during lint") {
			t.Fatalf("during-lint error = %v", err)
		}
	})

	t.Run("failed lint and failed manifest preserve recovery evidence", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("before"), 0o644); err != nil {
			t.Fatal(err)
		}
		transaction, err := beginSpecTreeTransaction(root)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = transaction.release() })
		err = transaction.postMutationHookWithLint(func(string) error {
			if err := os.WriteFile(filepath.Join(root, "during-error.md"), []byte("writer"), 0o644); err != nil {
				return err
			}
			return errors.New("lint failed")
		})()
		if err == nil || !strings.Contains(err.Error(), "lint failed") || len(transaction.preLintUnownedPaths) == 0 {
			t.Fatalf("failed lint = %v, paths=%v", err, transaction.preLintUnownedPaths)
		}
		if err := transaction.release(); err != nil {
			t.Fatal(err)
		}

		root = t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("before"), 0o644); err != nil {
			t.Fatal(err)
		}
		transaction, err = beginSpecTreeTransaction(root)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = transaction.release() })
		originalWrite := transactionWriteFile
		transactionWriteFile = func(string, []byte, os.FileMode) error { return errors.New("manifest write failed") }
		t.Cleanup(func() { transactionWriteFile = originalWrite })
		err = transaction.postMutationHookWithLint(func(clone string) error {
			return os.WriteFile(filepath.Join(clone, "README.md"), []byte("lint"), 0o644)
		})()
		if err == nil || !strings.Contains(err.Error(), "applying lint mutation manifest") {
			t.Fatalf("manifest error = %v", err)
		}
	})

	t.Run("isolated lint setup failures defer rollback without touching live tree", func(t *testing.T) {
		newTransaction := func(t *testing.T) *specTreeTransaction {
			t.Helper()
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("before"), 0o644); err != nil {
				t.Fatal(err)
			}
			transaction, err := beginSpecTreeTransaction(root)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = transaction.release() })
			return transaction
		}
		t.Run("temp directory", func(t *testing.T) {
			transaction := newTransaction(t)
			original := transactionMkdirTemp
			transactionMkdirTemp = func(string, string) (string, error) { return "", errors.New("temp failed") }
			t.Cleanup(func() { transactionMkdirTemp = original })
			if err := transaction.postMutationHookWithLint(func(string) error { return nil })(); err == nil || !strings.Contains(err.Error(), "creating isolated lint tree") {
				t.Fatalf("error = %v", err)
			}
		})
		t.Run("root permissions", func(t *testing.T) {
			transaction := newTransaction(t)
			original := transactionChmod
			transactionChmod = func(string, os.FileMode) error { return errors.New("chmod failed") }
			t.Cleanup(func() { transactionChmod = original })
			if err := transaction.postMutationHookWithLint(func(string) error { return nil })(); err == nil || !strings.Contains(err.Error(), "preparing isolated lint tree") {
				t.Fatalf("error = %v", err)
			}
		})
		t.Run("materialize directory and file", func(t *testing.T) {
			transaction := newTransaction(t)
			originalMkdir := transactionScratchMkdir
			transactionScratchMkdir = func(string, os.FileMode) error { return errors.New("scratch mkdir failed") }
			if err := transaction.postMutationHookWithLint(func(string) error { return nil })(); err == nil || !strings.Contains(err.Error(), "materializing isolated lint tree") {
				t.Fatalf("mkdir error = %v", err)
			}
			if err := transaction.release(); err != nil {
				t.Fatal(err)
			}
			transactionScratchMkdir = originalMkdir
			transaction = newTransaction(t)
			originalWrite := transactionScratchWrite
			transactionScratchWrite = func(string, []byte, os.FileMode) error { return errors.New("scratch write failed") }
			t.Cleanup(func() { transactionScratchMkdir, transactionScratchWrite = originalMkdir, originalWrite })
			if err := transaction.postMutationHookWithLint(func(string) error { return nil })(); err == nil || !strings.Contains(err.Error(), "materializing isolated lint tree") {
				t.Fatalf("write error = %v", err)
			}
		})
	})

	t.Run("snapshot failures at every isolated lint boundary defer rollback", func(t *testing.T) {
		newTransaction := func(t *testing.T) *specTreeTransaction {
			t.Helper()
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("before"), 0o644); err != nil {
				t.Fatal(err)
			}
			transaction, err := beginSpecTreeTransaction(root)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = transaction.release() })
			return transaction
		}
		for _, tc := range []struct {
			name   string
			failOn int
			want   string
		}{
			{"before lint", 1, "before lint --fix"},
			{"after lint failure", 2, "after isolated lint failure"},
			{"isolated output", 2, "isolated lint output"},
			{"before manifest", 3, "before applying lint manifest"},
			{"after manifest", 4, "after lint manifest"},
		} {
			t.Run(tc.name, func(t *testing.T) {
				transaction := newTransaction(t)
				original := transactionSnapshot
				calls := 0
				transactionSnapshot = func(root string) (specTreeSnapshot, error) {
					calls++
					if calls == tc.failOn {
						return specTreeSnapshot{}, errors.New("forced snapshot failure")
					}
					return snapshotSpecTreeForTransaction(root)
				}
				t.Cleanup(func() { transactionSnapshot = original })
				run := func(string) error { return nil }
				if tc.name == "after lint failure" {
					run = func(string) error { return errors.New("lint failed") }
				}
				err := transaction.postMutationHookWithLint(run)()
				if err == nil || !strings.Contains(err.Error(), tc.want) {
					t.Fatalf("error = %v, want %q", err, tc.want)
				}
			})
		}
	})
}

func TestApplySpecSnapshotDiff_DefensiveFailures(t *testing.T) {
	root := t.TempDir()
	base := specTreeSnapshot{directories: map[string]os.FileMode{".": os.ModeDir | 0o755}, files: map[string]specTreeFile{}}
	t.Run("directory and parent creation failures", func(t *testing.T) {
		original := transactionMkdirAll
		transactionMkdirAll = func(string, os.FileMode) error { return errors.New("mkdir failed") }
		t.Cleanup(func() { transactionMkdirAll = original })
		after := specTreeSnapshot{directories: map[string]os.FileMode{".": os.ModeDir | 0o755, "new": os.ModeDir | 0o755}, files: map[string]specTreeFile{}}
		if err := applySpecSnapshotDiff(root, base, after); err == nil || !strings.Contains(err.Error(), "mkdir failed") {
			t.Fatalf("directory error = %v", err)
		}
		transactionMkdirAll = original
		after = specTreeSnapshot{directories: base.directories, files: map[string]specTreeFile{"new.md": {content: []byte("new"), mode: 0o644}}}
		transactionMkdirAll = func(string, os.FileMode) error { return errors.New("parent mkdir failed") }
		if err := applySpecSnapshotDiff(root, base, after); err == nil || !strings.Contains(err.Error(), "parent mkdir failed") {
			t.Fatalf("parent error = %v", err)
		}
	})
	t.Run("write and removal failures", func(t *testing.T) {
		originalWrite := transactionWriteFile
		transactionWriteFile = func(string, []byte, os.FileMode) error { return errors.New("write failed") }
		t.Cleanup(func() { transactionWriteFile = originalWrite })
		after := specTreeSnapshot{directories: base.directories, files: map[string]specTreeFile{"new.md": {content: []byte("new"), mode: 0o644}}}
		if err := applySpecSnapshotDiff(root, base, after); err == nil || !strings.Contains(err.Error(), "write failed") {
			t.Fatalf("write error = %v", err)
		}
		transactionWriteFile = originalWrite
		originalRemove := transactionRemove
		transactionRemove = func(string) error { return errors.New("remove failed") }
		t.Cleanup(func() { transactionRemove = originalRemove })
		before := specTreeSnapshot{directories: base.directories, files: map[string]specTreeFile{"gone.md": {content: []byte("gone"), mode: 0o644}}}
		if err := applySpecSnapshotDiff(root, before, base); err == nil || !strings.Contains(err.Error(), "remove failed") {
			t.Fatalf("file removal error = %v", err)
		}
		before = specTreeSnapshot{directories: map[string]os.FileMode{".": os.ModeDir | 0o755, "gone": os.ModeDir | 0o755}, files: map[string]specTreeFile{}}
		if err := applySpecSnapshotDiff(root, before, base); err == nil || !strings.Contains(err.Error(), "remove failed") {
			t.Fatalf("directory removal error = %v", err)
		}
	})
	t.Run("unsafe manifest paths are rejected", func(t *testing.T) {
		for _, snapshot := range []specTreeSnapshot{
			{directories: map[string]os.FileMode{".": os.ModeDir | 0o755, "../escape": os.ModeDir | 0o755}, files: map[string]specTreeFile{}},
			{directories: base.directories, files: map[string]specTreeFile{"../escape.md": {content: []byte("x"), mode: 0o644}}},
		} {
			if err := applySpecSnapshotDiff(root, base, snapshot); err == nil {
				t.Fatal("unsafe manifest accepted")
			}
		}
	})
}

func TestSpecTreeTransaction_RemainingDefensiveBranches(t *testing.T) {
	t.Run("scratch file parent and write failures", func(t *testing.T) {
		root := t.TempDir()
		snapshot := specTreeSnapshot{directories: map[string]os.FileMode{}, files: map[string]specTreeFile{"file.md": {content: []byte("x"), mode: 0o644}}}
		originalMkdir, originalWrite := transactionScratchMkdir, transactionScratchWrite
		transactionScratchMkdir = func(string, os.FileMode) error { return errors.New("file parent failed") }
		if err := materializeSpecSnapshot(root, snapshot); err == nil || !strings.Contains(err.Error(), "file parent failed") {
			t.Fatalf("parent error = %v", err)
		}
		transactionScratchMkdir = originalMkdir
		transactionScratchWrite = func(string, []byte, os.FileMode) error { return errors.New("file write failed") }
		t.Cleanup(func() { transactionScratchMkdir, transactionScratchWrite = originalMkdir, originalWrite })
		if err := materializeSpecSnapshot(root, snapshot); err == nil || !strings.Contains(err.Error(), "file write failed") {
			t.Fatalf("write error = %v", err)
		}
	})

	t.Run("manifest apply postcondition detects a changed live snapshot", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("before"), 0o644); err != nil {
			t.Fatal(err)
		}
		transaction, err := beginSpecTreeTransaction(root)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = transaction.release() })
		original := transactionSnapshot
		calls := 0
		transactionSnapshot = func(path string) (specTreeSnapshot, error) {
			calls++
			snapshot, err := snapshotSpecTreeForTransaction(path)
			if calls == 4 {
				snapshot.files["unexpected.md"] = specTreeFile{content: []byte("race"), mode: 0o644}
			}
			return snapshot, err
		}
		t.Cleanup(func() { transactionSnapshot = original })
		err = transaction.postMutationHookWithLint(func(clone string) error {
			return os.WriteFile(filepath.Join(clone, "README.md"), []byte("lint"), 0o644)
		})()
		if err == nil || !strings.Contains(err.Error(), "while applying lint manifest") {
			t.Fatalf("error = %v", err)
		}
	})

	t.Run("finish and change helpers cover manual recovery states", func(t *testing.T) {
		base := specTreeSnapshot{directories: map[string]os.FileMode{".": os.ModeDir | 0o755}, files: map[string]specTreeFile{}}
		changed := specTreeSnapshot{directories: map[string]os.FileMode{".": os.ModeDir | 0o755, "new": os.ModeDir | 0o755}, files: map[string]specTreeFile{}}
		if got := changedSnapshotPaths(base, changed); !reflect.DeepEqual(got, []string{"new/"}) {
			t.Fatalf("paths = %v", got)
		}
		transaction := &specTreeTransaction{postMutationStarted: true, preLintSnapshot: &base, postLintSnapshot: &changed, opaquePostMutation: true}
		if err := transaction.finish(errors.New("action failed")); err == nil || !strings.Contains(err.Error(), "opaque post-mutation hook") {
			t.Fatalf("finish = %v", err)
		}
		transaction = &specTreeTransaction{snapshot: base}
		if got := transaction.unownedPreLintPaths(changed); !reflect.DeepEqual(got, []string{"new/"}) {
			t.Fatalf("unowned = %v", got)
		}
	})

	t.Run("restore reports snapshot and filesystem errors", func(t *testing.T) {
		root := t.TempDir()
		base := specTreeSnapshot{directories: map[string]os.FileMode{".": os.ModeDir | 0o755}, files: map[string]specTreeFile{"tracked.md": {content: []byte("before"), mode: 0o644}}}
		post := specTreeSnapshot{directories: base.directories, files: map[string]specTreeFile{"tracked.md": {content: []byte("lint"), mode: 0o644}}}
		transaction := &specTreeTransaction{specRoot: root, snapshot: base, postLintSnapshot: &post}
		originalSnapshot := transactionSnapshot
		transactionSnapshot = func(string) (specTreeSnapshot, error) {
			return specTreeSnapshot{}, errors.New("current snapshot failed")
		}
		if err := transaction.restoreLintMutations(); err == nil || !strings.Contains(err.Error(), "current snapshot failed") {
			t.Fatalf("snapshot error = %v", err)
		}
		transactionSnapshot = originalSnapshot
		if err := os.WriteFile(filepath.Join(root, "tracked.md"), []byte("lint"), 0o644); err != nil {
			t.Fatal(err)
		}
		originalMkdir := transactionMkdirAll
		transactionMkdirAll = func(string, os.FileMode) error { return errors.New("restore mkdir failed") }
		t.Cleanup(func() { transactionSnapshot, transactionMkdirAll = originalSnapshot, originalMkdir })
		if err := transaction.restoreLintMutations(); err == nil || !strings.Contains(err.Error(), "restore mkdir failed") {
			t.Fatalf("restore error = %v", err)
		}
	})
}

func TestReleaseLegacyLifecycleLock_RevalidationFailures(t *testing.T) {
	newLock := func(t *testing.T) string {
		t.Helper()
		lock := filepath.Join(t.TempDir(), ".specscore-lifecycle.lock")
		if err := os.Mkdir(lock, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(lifecycleLockOwnerPath(lock), []byte("bad\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		return lock
	}
	t.Run("initial and stale inspection failures", func(t *testing.T) {
		if err := releaseLegacyLifecycleLock(filepath.Join(t.TempDir(), "missing")); err == nil {
			t.Fatal("missing lock accepted")
		}
		lock := newLock(t)
		originalRead := transactionReadFile
		transactionReadFile = func(string) ([]byte, error) { return nil, errors.New("stale inspection failed") }
		t.Cleanup(func() { transactionReadFile = originalRead })
		if err := releaseLegacyLifecycleLock(lock); err == nil || !strings.Contains(err.Error(), "stale inspection failed") {
			t.Fatalf("stale error = %v", err)
		}
	})

	for _, tc := range []struct {
		name   string
		mutate func(t *testing.T, lock string, original func(string) (os.FileInfo, error)) func(string) (os.FileInfo, error)
		want   string
	}{
		{
			name: "owner revalidation lstat error",
			want: "revalidating lifecycle transaction lock owner",
			mutate: func(t *testing.T, lock string, original func(string) (os.FileInfo, error)) func(string) (os.FileInfo, error) {
				owner, calls := lifecycleLockOwnerPath(lock), 0
				return func(path string) (os.FileInfo, error) {
					if path == owner {
						calls++
						if calls == 2 {
							return nil, errors.New("owner revalidate failed")
						}
					}
					return original(path)
				}
			},
		},
		{
			name: "owner becomes nonregular",
			want: "refusing changed lifecycle transaction lock owner",
			mutate: func(t *testing.T, lock string, original func(string) (os.FileInfo, error)) func(string) (os.FileInfo, error) {
				owner, calls := lifecycleLockOwnerPath(lock), 0
				return func(path string) (os.FileInfo, error) {
					if path == owner {
						calls++
						if calls == 2 {
							if err := os.Remove(owner); err != nil {
								t.Fatal(err)
							}
							if err := os.Mkdir(owner, 0o700); err != nil {
								t.Fatal(err)
							}
						}
					}
					return original(path)
				}
			},
		},
		{
			name: "lock revalidation lstat error",
			want: "lock revalidation failed",
			mutate: func(t *testing.T, lock string, original func(string) (os.FileInfo, error)) func(string) (os.FileInfo, error) {
				calls := 0
				return func(path string) (os.FileInfo, error) {
					if path == lock {
						calls++
						if calls == 3 {
							return nil, errors.New("lock revalidation failed")
						}
					}
					return original(path)
				}
			},
		},
		{
			name: "lock identity changes before removal",
			want: "lock was replaced before stale-lock recovery",
			mutate: func(t *testing.T, lock string, original func(string) (os.FileInfo, error)) func(string) (os.FileInfo, error) {
				calls := 0
				return func(path string) (os.FileInfo, error) {
					if path == lock {
						calls++
						if calls == 3 {
							if err := os.RemoveAll(lock); err != nil {
								t.Fatal(err)
							}
							if err := os.Mkdir(lock, 0o700); err != nil {
								t.Fatal(err)
							}
						}
					}
					return original(path)
				}
			},
		},
		{
			name: "immediate owner revalidation error",
			want: "immediately before removal",
			mutate: func(t *testing.T, lock string, original func(string) (os.FileInfo, error)) func(string) (os.FileInfo, error) {
				owner, calls := lifecycleLockOwnerPath(lock), 0
				return func(path string) (os.FileInfo, error) {
					if path == owner {
						calls++
						if calls == 3 {
							return nil, errors.New("immediate owner failed")
						}
					}
					return original(path)
				}
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lock := newLock(t)
			original := transactionLstat
			transactionLstat = tc.mutate(t, lock, original)
			t.Cleanup(func() { transactionLstat = original })
			if err := releaseLegacyLifecycleLock(lock); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}

	for _, tc := range []struct {
		name string
		want string
	}{
		{"missing after owner removal", "no such file or directory"},
		{"replaced after owner removal", "replaced during stale-lock recovery"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			lock := newLock(t)
			original := transactionRemove
			calls := 0
			transactionRemove = func(path string) error {
				calls++
				if calls != 1 {
					return original(path)
				}
				if err := original(path); err != nil {
					return err
				}
				if err := os.Remove(lock); err != nil {
					return err
				}
				if tc.name == "replaced after owner removal" {
					return os.Mkdir(lock, 0o700)
				}
				return nil
			}
			t.Cleanup(func() { transactionRemove = original })
			if err := releaseLegacyLifecycleLock(lock); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
		})
	}
}

func TestSpecTreeTransaction_RestoreFailClosedBranches(t *testing.T) {
	t.Run("safe path inspection errors", func(t *testing.T) {
		root := t.TempDir()
		original := transactionLstat
		transactionLstat = func(string) (os.FileInfo, error) { return nil, errors.New("lstat failed") }
		t.Cleanup(func() { transactionLstat = original })
		if _, err := transactionSafePath(root, "file.md"); err == nil || !strings.Contains(err.Error(), "lstat failed") {
			t.Fatalf("root error = %v", err)
		}
		transactionLstat = original
		if err := os.WriteFile(filepath.Join(root, "file.md"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		transactionLstat = func(path string) (os.FileInfo, error) {
			if path == filepath.Join(root, "file.md") {
				return nil, errors.New("component lstat failed")
			}
			return original(path)
		}
		if _, err := transactionSafePath(root, "file.md"); err == nil || !strings.Contains(err.Error(), "component lstat failed") {
			t.Fatalf("component error = %v", err)
		}
		transactionLstat = original
		fileRoot := filepath.Join(t.TempDir(), "not-a-directory")
		if err := os.WriteFile(fileRoot, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := transactionSafePath(fileRoot, "file.md"); err == nil || !strings.Contains(err.Error(), "non-directory") {
			t.Fatalf("file root error = %v", err)
		}
	})

	newRestore := func(t *testing.T, initial, post specTreeSnapshot) *specTreeTransaction {
		t.Helper()
		root := t.TempDir()
		return &specTreeTransaction{specRoot: root, snapshot: initial, postLintSnapshot: &post}
	}
	rootDirs := map[string]os.FileMode{".": os.ModeDir | 0o755}
	t.Run("restoring a directory and source file rejects invalid paths", func(t *testing.T) {
		initial := specTreeSnapshot{directories: map[string]os.FileMode{".": os.ModeDir | 0o755, "../bad": os.ModeDir | 0o755}, files: map[string]specTreeFile{}}
		post := specTreeSnapshot{directories: rootDirs, files: map[string]specTreeFile{}}
		if err := newRestore(t, initial, post).restoreLintMutations(); err == nil || !strings.Contains(err.Error(), "recreate pre-transition directory") {
			t.Fatalf("directory error = %v", err)
		}
		initial = specTreeSnapshot{directories: rootDirs, files: map[string]specTreeFile{"../bad.md": {content: []byte("x"), mode: 0o644}}}
		post = specTreeSnapshot{directories: rootDirs, files: map[string]specTreeFile{}}
		if err := newRestore(t, initial, post).restoreLintMutations(); err == nil || !strings.Contains(err.Error(), "restore pre-transition file") {
			t.Fatalf("file error = %v", err)
		}
	})

	t.Run("removing lint-created paths rejects inspection errors", func(t *testing.T) {
		initial := specTreeSnapshot{directories: rootDirs, files: map[string]specTreeFile{}}
		post := specTreeSnapshot{directories: rootDirs, files: map[string]specTreeFile{"created.md": {content: []byte("lint"), mode: 0o644}}}
		transaction := newRestore(t, initial, post)
		if err := os.WriteFile(filepath.Join(transaction.specRoot, "created.md"), []byte("lint"), 0o644); err != nil {
			t.Fatal(err)
		}
		original := transactionLstat
		transactionLstat = func(path string) (os.FileInfo, error) {
			if path == filepath.Join(transaction.specRoot, "created.md") {
				return nil, errors.New("remove file inspect failed")
			}
			return original(path)
		}
		t.Cleanup(func() { transactionLstat = original })
		if err := transaction.restoreLintMutations(); err == nil || !strings.Contains(err.Error(), "remove lint-created file") {
			t.Fatalf("file removal error = %v", err)
		}
		transactionLstat = original
		initial = specTreeSnapshot{directories: rootDirs, files: map[string]specTreeFile{}}
		post = specTreeSnapshot{directories: map[string]os.FileMode{".": os.ModeDir | 0o755, "created": os.ModeDir | 0o755}, files: map[string]specTreeFile{}}
		transaction = newRestore(t, initial, post)
		if err := os.Mkdir(filepath.Join(transaction.specRoot, "created"), 0o755); err != nil {
			t.Fatal(err)
		}
		transactionLstat = func(path string) (os.FileInfo, error) {
			if path == filepath.Join(transaction.specRoot, "created") {
				return nil, errors.New("remove directory inspect failed")
			}
			return original(path)
		}
		if err := transaction.restoreLintMutations(); err == nil || !strings.Contains(err.Error(), "remove lint-created directory") {
			t.Fatalf("directory removal error = %v", err)
		}
	})

	t.Run("finish reports a non-concurrent restore failure", func(t *testing.T) {
		base := specTreeSnapshot{directories: rootDirs, files: map[string]specTreeFile{}}
		post := specTreeSnapshot{directories: rootDirs, files: map[string]specTreeFile{"created.md": {content: []byte("lint"), mode: 0o644}}}
		transaction := newRestore(t, base, post)
		transaction.postMutationStarted, transaction.preLintSnapshot = true, &base
		original := transactionSnapshot
		transactionSnapshot = func(string) (specTreeSnapshot, error) {
			return specTreeSnapshot{}, errors.New("restore snapshot failed")
		}
		t.Cleanup(func() { transactionSnapshot = original })
		if err := transaction.finish(errors.New("action failed")); err == nil || !strings.Contains(err.Error(), "could not restore") {
			t.Fatalf("finish error = %v", err)
		}
	})

	t.Run("already-restored source is left alone", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "tracked.md"), []byte("before"), 0o644); err != nil {
			t.Fatal(err)
		}
		initial := specTreeSnapshot{directories: rootDirs, files: map[string]specTreeFile{"tracked.md": {content: []byte("before"), mode: 0o644}}}
		post := specTreeSnapshot{directories: rootDirs, files: map[string]specTreeFile{"tracked.md": {content: []byte("lint"), mode: 0o644}}}
		transaction := &specTreeTransaction{specRoot: root, snapshot: initial, postLintSnapshot: &post}
		if err := transaction.restoreLintMutations(); err != nil {
			t.Fatalf("restore = %v", err)
		}
	})
}

func TestSnapshotRestoreAndManifestRemovalSafety(t *testing.T) {
	root := t.TempDir()
	base := specTreeSnapshot{directories: map[string]os.FileMode{".": os.ModeDir | 0o755}, files: map[string]specTreeFile{}}
	t.Run("manifest refuses unsafe removed files and directories", func(t *testing.T) {
		before := specTreeSnapshot{directories: base.directories, files: map[string]specTreeFile{"../gone.md": {content: []byte("x"), mode: 0o644}}}
		if err := applySpecSnapshotDiff(root, before, base); err == nil {
			t.Fatal("unsafe removed file accepted")
		}
		before = specTreeSnapshot{directories: map[string]os.FileMode{".": os.ModeDir | 0o755, "../gone": os.ModeDir | 0o755}, files: map[string]specTreeFile{}}
		if err := applySpecSnapshotDiff(root, before, base); err == nil {
			t.Fatal("unsafe removed directory accepted")
		}
	})
	t.Run("restore refuses unsafe writes and deletes", func(t *testing.T) {
		original := transactionLstat
		file := filepath.Join(root, "extra.md")
		if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
		transactionLstat = func(path string) (os.FileInfo, error) {
			if path == file {
				return nil, errors.New("extra file inspect failed")
			}
			return original(path)
		}
		if err := base.restore(root); err == nil || !strings.Contains(err.Error(), "extra file inspect failed") {
			t.Fatalf("file restore error = %v", err)
		}
		transactionLstat = original
		if err := os.Remove(file); err != nil {
			t.Fatal(err)
		}
		dir := filepath.Join(root, "extra")
		if err := os.Mkdir(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		transactionLstat = func(path string) (os.FileInfo, error) {
			if path == dir {
				return nil, errors.New("extra dir inspect failed")
			}
			return original(path)
		}
		if err := base.restore(root); err == nil || !strings.Contains(err.Error(), "extra dir inspect failed") {
			t.Fatalf("directory restore error = %v", err)
		}
		transactionLstat = original
		snapshot := specTreeSnapshot{directories: map[string]os.FileMode{".": os.ModeDir | 0o755, "../bad": os.ModeDir | 0o755}, files: map[string]specTreeFile{}}
		t.Cleanup(func() { transactionLstat = original })
		if err := snapshot.restore(root); err == nil || !strings.Contains(err.Error(), "recreate snapshot directory") {
			t.Fatalf("snapshot directory error = %v", err)
		}
	})
}

func TestLifecycleCommandAdapterDefensiveSeams(t *testing.T) {
	t.Run("issue discovery failure precedes mutation", func(t *testing.T) {
		root := setupIssueSpecRoot(t)
		withCwd(t, root)
		original := issueDiscoverAll
		issueDiscoverAll = func(string) ([]issue.Discovered, error) { return nil, errors.New("issue discovery failed") }
		t.Cleanup(func() { issueDiscoverAll = original })
		_, _, err := runIssue(t, "change-status", "missing", "--to=investigating")
		if err == nil || !strings.Contains(err.Error(), "issue discovery failed") {
			t.Fatalf("issue error = %v", err)
		}
	})

	for _, tc := range []struct {
		name string
		run  func(*testing.T) error
	}{
		{
			name: "plan owned path failure",
			run: func(t *testing.T) error {
				stagePlan(t, "auth", "Draft")
				_, _, err := runPlan(t, "change-status", "auth", "--to=in review")
				return err
			},
		},
		{
			name: "sidekick owned path failure",
			run: func(t *testing.T) error {
				root := setupSpecRoot(t)
				withCwd(t, root)
				if _, _, err := runSidekick(t, "new", "queued seed"); err != nil {
					return err
				}
				_, _, err := runSidekick(t, "change-status", "queued-seed", "--to=implemented")
				return err
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			original := transactionRel
			transactionRel = func(string, string) (string, error) { return "", errors.New("owned path failed") }
			t.Cleanup(func() { transactionRel = original })
			if err := tc.run(t); err == nil || !strings.Contains(err.Error(), "owned path failed") {
				t.Fatalf("adapter error = %v", err)
			}
		})
	}
}

func TestSpecTreeTransaction_ExternalOwnedPreLintMutationIsPreserved(t *testing.T) {
	root := t.TempDir()
	specRoot := filepath.Join(root, "spec")
	ownedPath := filepath.Join(specRoot, "plans", "auth.md")
	if err := os.MkdirAll(filepath.Dir(ownedPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ownedPath, []byte("Draft\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	transaction, err := beginSpecTreeTransaction(specRoot, filepath.Join("plans", "auth.md"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = transaction.release() })

	// Capture the exact bytes written by the lifecycle operation. An external
	// writer then changes that same, otherwise-authorized path before lint gets
	// control. A path allowlist alone could not distinguish this from our own
	// status rewrite.
	if err := os.WriteFile(ownedPath, []byte("In Review\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := transaction.captureLifecycleMutationState(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ownedPath, []byte("External editor change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCalled := false
	err = transaction.postMutationHookWith(func() error {
		runCalled = true
		return errors.New("verification failed")
	})()
	if err == nil || !lifecycle.IsDeferredRollback(err) || !strings.Contains(err.Error(), "plans/auth.md") {
		t.Fatalf("hook error = %v, want deferred owned-artifact recovery error", err)
	}
	if runCalled {
		t.Fatal("lint ran despite an external edit of the lifecycle artifact")
	}
	err = transaction.finish(err)
	if err == nil || !strings.Contains(err.Error(), "rollback preserved unowned changes") || !strings.Contains(err.Error(), "plans/auth.md") {
		t.Fatalf("finish error = %v, want actionable owned-artifact recovery error", err)
	}
	if got, readErr := os.ReadFile(ownedPath); readErr != nil || string(got) != "External editor change\n" {
		t.Fatalf("external edit of lifecycle artifact was overwritten: %q, %v", got, readErr)
	}
}

func TestSpecTreeTransaction_RecoveryFailurePaths(t *testing.T) {
	t.Run("capture mutation state failure defers rollback", func(t *testing.T) {
		specRoot := t.TempDir()
		if err := os.WriteFile(filepath.Join(specRoot, "README.md"), []byte("body"), 0o644); err != nil {
			t.Fatal(err)
		}
		transaction, err := beginSpecTreeTransaction(specRoot)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = transaction.release() })
		originalRead := transactionReadFile
		transactionReadFile = func(string) ([]byte, error) { return nil, errors.New("forced mutation-state snapshot failure") }
		t.Cleanup(func() { transactionReadFile = originalRead })

		err = transaction.captureLifecycleMutationState()
		if err == nil || !lifecycle.IsDeferredRollback(err) || !strings.Contains(err.Error(), "forced mutation-state snapshot failure") {
			t.Fatalf("capture error = %v, want deferred snapshot failure", err)
		}
		if got := transaction.finish(err); got == nil || !strings.Contains(got.Error(), "ownership could not be established") {
			t.Fatalf("finish error = %v, want manual recovery error", got)
		}
	})

	t.Run("pre-lint snapshot failure does not start rollback", func(t *testing.T) {
		specRoot := t.TempDir()
		if err := os.WriteFile(filepath.Join(specRoot, "README.md"), []byte("body"), 0o644); err != nil {
			t.Fatal(err)
		}
		transaction, err := beginSpecTreeTransaction(specRoot)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = transaction.release() })
		originalRead := transactionReadFile
		transactionReadFile = func(string) ([]byte, error) { return nil, errors.New("forced pre-lint snapshot failure") }
		t.Cleanup(func() { transactionReadFile = originalRead })

		err = transaction.postMutationHookWith(func() error { return nil })()
		if err == nil || !strings.Contains(err.Error(), "forced pre-lint snapshot failure") {
			t.Fatalf("hook error = %v, want pre-lint snapshot failure", err)
		}
		if got := transaction.finish(err); got != err {
			t.Fatalf("finish changed pre-lint failure: got %v want %v", got, err)
		}
	})

	t.Run("post-lint snapshot failure defers inner rollback and preserves tree", func(t *testing.T) {
		specRoot := t.TempDir()
		if err := os.WriteFile(filepath.Join(specRoot, "README.md"), []byte("body"), 0o644); err != nil {
			t.Fatal(err)
		}
		transaction, err := beginSpecTreeTransaction(specRoot)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = transaction.release() })
		originalRead := transactionReadFile
		reads := 0
		transactionReadFile = func(path string) ([]byte, error) {
			reads++
			if reads > 1 {
				return nil, errors.New("forced post-lint snapshot failure")
			}
			return originalRead(path)
		}
		t.Cleanup(func() { transactionReadFile = originalRead })

		err = transaction.postMutationHookWith(func() error { return errors.New("verification failed") })()
		if err == nil || !lifecycle.IsDeferredRollback(err) || !strings.Contains(err.Error(), "forced post-lint snapshot failure") {
			t.Fatalf("hook error = %v, want deferred post-lint snapshot failure", err)
		}
		err = transaction.finish(err)
		if err == nil || !strings.Contains(err.Error(), "ownership could not be established") {
			t.Fatalf("finish error = %v, want recovery guidance", err)
		}
	})

	t.Run("current snapshot failure is reported", func(t *testing.T) {
		specRoot := t.TempDir()
		if err := os.WriteFile(filepath.Join(specRoot, "README.md"), []byte("body"), 0o644); err != nil {
			t.Fatal(err)
		}
		snapshot, err := snapshotSpecTreeForTransaction(specRoot)
		if err != nil {
			t.Fatal(err)
		}
		transaction := &specTreeTransaction{specRoot: specRoot, snapshot: snapshot, postLintSnapshot: &snapshot}
		originalRead := transactionReadFile
		transactionReadFile = func(string) ([]byte, error) { return nil, errors.New("forced current snapshot failure") }
		t.Cleanup(func() { transactionReadFile = originalRead })
		if err := transaction.restoreLintMutations(); err == nil || !strings.Contains(err.Error(), "forced current snapshot failure") {
			t.Fatalf("restore error = %v, want current snapshot failure", err)
		}
	})

	t.Run("restore recreates removed paths and reports I/O failures", func(t *testing.T) {
		newTransaction := func(t *testing.T) (*specTreeTransaction, string) {
			t.Helper()
			specRoot := t.TempDir()
			initial := specTreeSnapshot{
				files: map[string]specTreeFile{
					"nested/original.md": {content: []byte("original\n"), mode: 0o640},
				},
				directories: map[string]os.FileMode{".": 0o755, "nested": 0o755},
			}
			post := specTreeSnapshot{files: map[string]specTreeFile{}, directories: map[string]os.FileMode{".": 0o755}}
			return &specTreeTransaction{specRoot: specRoot, snapshot: initial, postLintSnapshot: &post}, specRoot
		}

		t.Run("success", func(t *testing.T) {
			transaction, specRoot := newTransaction(t)
			if err := transaction.restoreLintMutations(); err != nil {
				t.Fatal(err)
			}
			if got, err := os.ReadFile(filepath.Join(specRoot, "nested", "original.md")); err != nil || string(got) != "original\n" {
				t.Fatalf("restored file = %q, %v", got, err)
			}
		})

		t.Run("directory creation failure", func(t *testing.T) {
			transaction, _ := newTransaction(t)
			originalMkdirAll := transactionMkdirAll
			transactionMkdirAll = func(string, os.FileMode) error { return errors.New("forced directory creation failure") }
			t.Cleanup(func() { transactionMkdirAll = originalMkdirAll })
			if err := transaction.restoreLintMutations(); err == nil || !strings.Contains(err.Error(), "forced directory creation failure") {
				t.Fatalf("restore error = %v, want directory creation failure", err)
			}
		})

		t.Run("file write failure", func(t *testing.T) {
			transaction, _ := newTransaction(t)
			originalWrite := transactionWriteFile
			transactionWriteFile = func(string, []byte, os.FileMode) error { return errors.New("forced file write failure") }
			t.Cleanup(func() { transactionWriteFile = originalWrite })
			if err := transaction.restoreLintMutations(); err == nil || !strings.Contains(err.Error(), "forced file write failure") {
				t.Fatalf("restore error = %v, want file write failure", err)
			}
		})
	})

	t.Run("removing transaction-created paths reports I/O failures", func(t *testing.T) {
		newTransaction := func(t *testing.T, nested bool) (*specTreeTransaction, string) {
			t.Helper()
			specRoot := t.TempDir()
			path := filepath.Join(specRoot, "generated.md")
			if nested {
				path = filepath.Join(specRoot, "generated", "README.md")
				if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.WriteFile(path, []byte("generated\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			initial := specTreeSnapshot{files: map[string]specTreeFile{}, directories: map[string]os.FileMode{".": 0o755}}
			post := specTreeSnapshot{files: map[string]specTreeFile{}, directories: map[string]os.FileMode{".": 0o755}}
			if nested {
				post.files["generated/README.md"] = specTreeFile{content: []byte("generated\n"), mode: 0o644}
				post.directories["generated"] = os.ModeDir | 0o755
			} else {
				post.files["generated.md"] = specTreeFile{content: []byte("generated\n"), mode: 0o644}
			}
			return &specTreeTransaction{specRoot: specRoot, snapshot: initial, postLintSnapshot: &post}, path
		}

		t.Run("file", func(t *testing.T) {
			transaction, _ := newTransaction(t, false)
			originalRemove := transactionRemove
			transactionRemove = func(string) error { return errors.New("forced created-file removal failure") }
			t.Cleanup(func() { transactionRemove = originalRemove })
			if err := transaction.restoreLintMutations(); err == nil || !strings.Contains(err.Error(), "forced created-file removal failure") {
				t.Fatalf("restore error = %v, want created-file removal failure", err)
			}
		})

		t.Run("directory", func(t *testing.T) {
			transaction, path := newTransaction(t, true)
			originalRemove := transactionRemove
			transactionRemove = func(target string) error {
				if target == path {
					return originalRemove(target)
				}
				return errors.New("forced created-directory removal failure")
			}
			t.Cleanup(func() { transactionRemove = originalRemove })
			if err := transaction.restoreLintMutations(); err == nil || !strings.Contains(err.Error(), "forced created-directory removal failure") {
				t.Fatalf("restore error = %v, want created-directory removal failure", err)
			}
		})
	})

	t.Run("external directory descendants are detected", func(t *testing.T) {
		expected := specTreeSnapshot{files: map[string]specTreeFile{}, directories: map[string]os.FileMode{".": 0o755, "generated": 0o755}}
		current := specTreeSnapshot{files: map[string]specTreeFile{}, directories: map[string]os.FileMode{".": 0o755, "generated": 0o755, "generated/raw": 0o755}}
		if !snapshotDirectoryHasExternalDescendant(current, expected, "generated") {
			t.Fatal("external directory descendant was not detected")
		}
		current.files["generated/raw.md"] = specTreeFile{content: []byte("raw"), mode: 0o644}
		if !snapshotDirectoryHasExternalDescendant(current, expected, "generated") {
			t.Fatal("external file descendant was not detected")
		}
	})

	t.Run("directory compare and restoration edge cases", func(t *testing.T) {
		mode755 := os.ModeDir | 0o755
		mode700 := os.ModeDir | 0o700

		t.Run("already-restored directory is left alone", func(t *testing.T) {
			specRoot := t.TempDir()
			if err := os.Mkdir(filepath.Join(specRoot, "original"), 0o755); err != nil {
				t.Fatal(err)
			}
			initial := specTreeSnapshot{files: map[string]specTreeFile{}, directories: map[string]os.FileMode{".": 0o700, "original": mode755}}
			post := specTreeSnapshot{files: map[string]specTreeFile{}, directories: map[string]os.FileMode{".": 0o700}}
			transaction := &specTreeTransaction{specRoot: specRoot, snapshot: initial, postLintSnapshot: &post}
			if err := transaction.restoreLintMutations(); err != nil {
				t.Fatal(err)
			}
		})

		t.Run("modified directory conflicts", func(t *testing.T) {
			specRoot := t.TempDir()
			if err := os.Mkdir(filepath.Join(specRoot, "generated"), 0o700); err != nil {
				t.Fatal(err)
			}
			initial := specTreeSnapshot{files: map[string]specTreeFile{}, directories: map[string]os.FileMode{".": 0o700}}
			post := specTreeSnapshot{files: map[string]specTreeFile{}, directories: map[string]os.FileMode{".": 0o700, "generated": mode755}}
			transaction := &specTreeTransaction{specRoot: specRoot, snapshot: initial, postLintSnapshot: &post}
			if err := transaction.restoreLintMutations(); err == nil || !strings.Contains(err.Error(), "generated/") {
				t.Fatalf("restore error = %v, want generated directory conflict", err)
			}
		})

		t.Run("post-state directory is retained", func(t *testing.T) {
			specRoot := t.TempDir()
			if err := os.Mkdir(filepath.Join(specRoot, "mode-change"), 0o700); err != nil {
				t.Fatal(err)
			}
			initial := specTreeSnapshot{files: map[string]specTreeFile{}, directories: map[string]os.FileMode{".": 0o700, "mode-change": mode755}}
			post := specTreeSnapshot{files: map[string]specTreeFile{}, directories: map[string]os.FileMode{".": 0o700, "mode-change": mode700}}
			transaction := &specTreeTransaction{specRoot: specRoot, snapshot: initial, postLintSnapshot: &post}
			if err := transaction.restoreLintMutations(); err != nil {
				t.Fatal(err)
			}
		})

		t.Run("sorts nested created directories before removing", func(t *testing.T) {
			specRoot := t.TempDir()
			if err := os.MkdirAll(filepath.Join(specRoot, "generated", "nested"), 0o755); err != nil {
				t.Fatal(err)
			}
			initial := specTreeSnapshot{files: map[string]specTreeFile{}, directories: map[string]os.FileMode{".": 0o700}}
			post := specTreeSnapshot{files: map[string]specTreeFile{}, directories: map[string]os.FileMode{
				".":                0o700,
				"generated":        mode755,
				"generated/nested": mode755,
			}}
			transaction := &specTreeTransaction{specRoot: specRoot, snapshot: initial, postLintSnapshot: &post}
			if err := transaction.restoreLintMutations(); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(filepath.Join(specRoot, "generated")); !os.IsNotExist(err) {
				t.Fatalf("created directory remains after rollback: %v", err)
			}
		})
	})

	t.Run("file parent creation failure is reported", func(t *testing.T) {
		specRoot := t.TempDir()
		initial := specTreeSnapshot{
			files:       map[string]specTreeFile{"original.md": {content: []byte("original"), mode: 0o644}},
			directories: map[string]os.FileMode{".": 0o700},
		}
		post := specTreeSnapshot{files: map[string]specTreeFile{}, directories: map[string]os.FileMode{".": 0o700}}
		transaction := &specTreeTransaction{specRoot: specRoot, snapshot: initial, postLintSnapshot: &post}
		originalMkdirAll := transactionMkdirAll
		transactionMkdirAll = func(string, os.FileMode) error { return errors.New("forced file parent creation failure") }
		t.Cleanup(func() { transactionMkdirAll = originalMkdirAll })
		if err := transaction.restoreLintMutations(); err == nil || !strings.Contains(err.Error(), "forced file parent creation failure") {
			t.Fatalf("restore error = %v, want file parent creation failure", err)
		}
	})
}

func TestSpecTreeSnapshotRestore_LeavesExistingSnapshotFileUntouched(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "existing.md")
	if err := os.WriteFile(path, []byte("existing\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	snapshot, err := snapshotSpecTreeForTransaction(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := snapshot.restore(root); err != nil {
		t.Fatal(err)
	}
}

func TestSidekickNoteRollback(t *testing.T) {
	path := filepath.Join(t.TempDir(), "seed.md")
	if err := os.WriteFile(path, []byte("original\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("with note\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := (sidekickNoteRollback{seedPath: path, original: []byte("original\n"), written: true}).restore(); err != nil {
		t.Fatal(err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "original\n" {
		t.Fatalf("restored note body = %q, %v", got, err)
	}
	if err := (sidekickNoteRollback{seedPath: path}).restore(); err != nil {
		t.Fatalf("no-note rollback = %v, want nil", err)
	}
}

func TestDefaultProcessAlive(t *testing.T) {
	if !defaultProcessAlive(os.Getpid()) {
		t.Fatal("current process was not reported alive")
	}
}

func TestReleaseSpecTreeTransaction_PreservesActionError(t *testing.T) {
	root := t.TempDir()
	specRoot := filepath.Join(root, "spec")
	if err := os.MkdirAll(specRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	transaction, err := beginSpecTreeTransaction(specRoot)
	if err != nil {
		t.Fatal(err)
	}
	originalRemove := transactionRemove
	transactionRemove = func(string) error { return errors.New("forced lock release failure") }
	t.Cleanup(func() { transactionRemove = originalRemove })

	actionErr := exitcode.InvalidStateError("the lifecycle action failed")
	resultErr := error(actionErr)
	releaseSpecTreeTransaction(transaction, &resultErr)
	if !errors.Is(resultErr, actionErr) {
		t.Fatalf("release error did not retain action error: %v", resultErr)
	}
	if got := exitCodeOfErr(resultErr); got != exitcode.InvalidState {
		t.Fatalf("exit code = %d, want primary action code %d; err=%v", got, exitcode.InvalidState, resultErr)
	}
	if !strings.Contains(resultErr.Error(), "forced lock release failure") {
		t.Fatalf("release error was not surfaced: %v", resultErr)
	}
}

func TestPlanChangeStatus_LockReleaseFailureIsReturned_CLI(t *testing.T) {
	root := stagePlan(t, "auth", "Draft")
	originalRemove := transactionRemove
	transactionRemove = func(path string) error {
		if filepath.Base(path) == ".specscore-lifecycle.lock" {
			return errors.New("forced lock release failure")
		}
		return originalRemove(path)
	}

	_, _, err := runPlan(t, "change-status", "auth", "--to=in review")
	transactionRemove = originalRemove
	if err == nil || !strings.Contains(err.Error(), "forced lock release failure") {
		t.Fatalf("command error = %v, want surfaced lock release failure", err)
	}
	if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Fatalf("exit = %d, want %d; err=%v", got, exitcode.Unexpected, err)
	}
	body, readErr := os.ReadFile(filepath.Join(root, "spec", "plans", "auth.md"))
	if readErr != nil || !strings.Contains(string(body), "**Status:** In Review") {
		t.Fatalf("successful action was rolled back after release failure: body=%q err=%v", body, readErr)
	}
	if cleanupErr := os.RemoveAll(filepath.Join(root, ".specscore-lifecycle.lock")); cleanupErr != nil {
		t.Fatalf("clean up intentionally retained lock: %v", cleanupErr)
	}
}

func TestSpecTreeRestoreIOFailures(t *testing.T) {
	t.Run("walk failure", func(t *testing.T) {
		err := (specTreeSnapshot{files: map[string]specTreeFile{}}).restore(filepath.Join(t.TempDir(), "missing"))
		if err == nil || !strings.Contains(err.Error(), "removing lint-created files") {
			t.Fatalf("error = %v, want wrapped walk failure", err)
		}
	})

	t.Run("remove created file", func(t *testing.T) {
		root := t.TempDir()
		createdPath := filepath.Join(root, "created.md")
		if err := os.WriteFile(createdPath, []byte("created"), 0o644); err != nil {
			t.Fatal(err)
		}
		originalRemove := transactionRemove
		transactionRemove = func(string) error { return errors.New("forced remove failure") }
		t.Cleanup(func() { transactionRemove = originalRemove })

		err := (specTreeSnapshot{files: map[string]specTreeFile{}}).restore(root)
		if err == nil || !strings.Contains(err.Error(), "forced remove failure") {
			t.Fatalf("error = %v, want forced remove failure", err)
		}
	})

	t.Run("remove created directory", func(t *testing.T) {
		root := t.TempDir()
		createdPath := filepath.Join(root, "created", "child.md")
		if err := os.MkdirAll(filepath.Dir(createdPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(createdPath, []byte("created"), 0o644); err != nil {
			t.Fatal(err)
		}
		originalRemove := transactionRemove
		transactionRemove = func(path string) error {
			if path == createdPath {
				return os.Remove(path)
			}
			return errors.New("forced directory remove failure")
		}
		t.Cleanup(func() { transactionRemove = originalRemove })

		err := (specTreeSnapshot{files: map[string]specTreeFile{}}).restore(root)
		if err == nil || !strings.Contains(err.Error(), "forced directory remove failure") {
			t.Fatalf("error = %v, want forced directory remove failure", err)
		}
	})

	t.Run("recreate directory", func(t *testing.T) {
		root := t.TempDir()
		originalMkdirAll := transactionMkdirAll
		transactionMkdirAll = func(string, os.FileMode) error { return errors.New("forced mkdir failure") }
		t.Cleanup(func() { transactionMkdirAll = originalMkdirAll })

		err := (specTreeSnapshot{directories: map[string]os.FileMode{
			"nested": 0o755,
		}}).restore(root)
		if err == nil || !strings.Contains(err.Error(), "forced mkdir failure") {
			t.Fatalf("error = %v, want forced mkdir failure", err)
		}
	})

	t.Run("recreate file parent directory", func(t *testing.T) {
		root := t.TempDir()
		originalMkdirAll := transactionMkdirAll
		transactionMkdirAll = func(string, os.FileMode) error { return errors.New("forced file parent mkdir failure") }
		t.Cleanup(func() { transactionMkdirAll = originalMkdirAll })

		err := (specTreeSnapshot{files: map[string]specTreeFile{
			"nested/original.md": {content: []byte("original"), mode: 0o644},
		}}).restore(root)
		if err == nil || !strings.Contains(err.Error(), "forced file parent mkdir failure") {
			t.Fatalf("error = %v, want forced file parent mkdir failure", err)
		}
	})

	t.Run("rewrite file", func(t *testing.T) {
		root := t.TempDir()
		originalWrite := transactionWriteFile
		transactionWriteFile = func(string, []byte, os.FileMode) error { return errors.New("forced write failure") }
		t.Cleanup(func() { transactionWriteFile = originalWrite })

		err := (specTreeSnapshot{files: map[string]specTreeFile{
			"original.md": {content: []byte("original"), mode: 0o644},
		}}).restore(root)
		if err == nil || !strings.Contains(err.Error(), "forced write failure") {
			t.Fatalf("error = %v, want forced write failure", err)
		}
	})
}

func TestSpecTreeTransaction_RestoresAllLintMutationsAfterVerificationFailure(t *testing.T) {
	root := t.TempDir()
	specRoot := filepath.Join(root, "spec")
	target := filepath.Join(specRoot, "plans", "auth.md")
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("target before lint\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	transaction, err := beginSpecTreeTransaction(specRoot)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	before := transaction.snapshot

	originalLint := lintLintFn
	lintLintFn = func(opts lint.Options) ([]lint.Violation, error) {
		if opts.Fix {
			lintTarget := filepath.Join(opts.SpecRoot, "plans", "auth.md")
			if err := os.WriteFile(lintTarget, []byte("target changed by lint\n"), 0o644); err != nil {
				return nil, err
			}
			return nil, os.WriteFile(filepath.Join(opts.SpecRoot, "plans", "README.md"), []byte("index added by lint\n"), 0o644)
		}
		return []lint.Violation{{File: "plans/auth.md", Line: 1, Rule: "forced", Severity: "error", Message: "verify failure"}}, nil
	}
	t.Cleanup(func() { lintLintFn = originalLint })

	err = transaction.postMutationHook()()
	err = transaction.finish(err)
	if err == nil || !strings.Contains(err.Error(), "verify failure") {
		t.Fatalf("error = %v, want verification failure", err)
	}
	after, err := snapshotSpecTreeForTransaction(specRoot)
	if err != nil {
		t.Fatalf("snapshot after hook: %v", err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("lint mutations were not restored\nwant: %#v\n got: %#v", before, after)
	}
}

func TestLintPostMutationHook_SnapshotAndRollbackFailuresAreReported(t *testing.T) {
	t.Run("snapshot", func(t *testing.T) {
		specRoot := t.TempDir()
		if err := os.WriteFile(filepath.Join(specRoot, "README.md"), []byte("body"), 0o644); err != nil {
			t.Fatal(err)
		}
		originalRead := transactionReadFile
		transactionReadFile = func(string) ([]byte, error) { return nil, errors.New("forced snapshot failure") }
		t.Cleanup(func() { transactionReadFile = originalRead })

		_, err := beginSpecTreeTransaction(specRoot)
		if err == nil || !strings.Contains(err.Error(), "forced snapshot failure") {
			t.Fatalf("error = %v, want snapshot failure", err)
		}
	})

	t.Run("rollback", func(t *testing.T) {
		specRoot := t.TempDir()
		target := filepath.Join(specRoot, "README.md")
		if err := os.WriteFile(target, []byte("before"), 0o644); err != nil {
			t.Fatal(err)
		}
		originalLint := lintLintFn
		lintLintFn = func(opts lint.Options) ([]lint.Violation, error) {
			if opts.Fix {
				return nil, os.WriteFile(filepath.Join(opts.SpecRoot, "README.md"), []byte("changed"), 0o644)
			}
			return nil, errors.New("forced verification failure")
		}
		t.Cleanup(func() { lintLintFn = originalLint })
		originalWrite := transactionWriteFile
		transactionWriteFile = func(string, []byte, os.FileMode) error { return errors.New("forced restore failure") }
		t.Cleanup(func() { transactionWriteFile = originalWrite })

		transaction, err := beginSpecTreeTransaction(specRoot)
		if err != nil {
			t.Fatal(err)
		}
		err = transaction.postMutationHook()()
		err = transaction.finish(err)
		if err == nil || !strings.Contains(err.Error(), "forced verification failure") {
			t.Fatalf("error = %v, want verification failure", err)
		}
		if got, readErr := os.ReadFile(target); readErr != nil || string(got) != "before" {
			t.Fatalf("isolated lint changed live tree despite verification failure: %q, %v", got, readErr)
		}
	})
}

func TestIdeaChangeStatus_LintVerificationFailureRestoresPreexistingMirrorDrift_CLI(t *testing.T) {
	root := stageActiveIdea(t, "foo", "Draft", "")
	specRoot := filepath.Join(root, "spec")
	ideaPath := filepath.Join(specRoot, "ideas", "foo.md")
	ideaBody, err := os.ReadFile(ideaPath)
	if err != nil {
		t.Fatal(err)
	}
	driftedBody := strings.Replace(string(ideaBody), "status: Draft", "status: Approved", 1)
	if driftedBody == string(ideaBody) {
		t.Fatal("fixture Idea lacks a frontmatter status mirror to drift")
	}
	if err := os.WriteFile(ideaPath, []byte(driftedBody), 0o644); err != nil {
		t.Fatal(err)
	}
	before, err := snapshotSpecTreeForTransaction(specRoot)
	if err != nil {
		t.Fatal(err)
	}

	originalLint := lintLintFn
	lintLintFn = func(opts lint.Options) ([]lint.Violation, error) {
		if opts.Fix {
			return lint.Lint(opts)
		}
		return []lint.Violation{{File: "ideas/foo.md", Line: 1, Rule: "forced", Severity: "error", Message: "late verification failure"}}, nil
	}
	t.Cleanup(func() { lintLintFn = originalLint })

	_, _, err = runIdea(t, "change-status", "foo", "--to=approved")
	if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Fatalf("exit = %d, want %d; err=%v", got, exitcode.Unexpected, err)
	}
	after, err := snapshotSpecTreeForTransaction(specRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("failed Idea transition left mutations behind\nwant: %#v\n got: %#v", before, after)
	}
	if got, err := os.ReadFile(ideaPath); err != nil || string(got) != driftedBody {
		t.Fatalf("failed transition did not restore pre-existing Idea mirror drift\nwant:\n%s\ngot:\n%s\nerr=%v", driftedBody, got, err)
	}
}

func TestIssueChangeStatus_LintVerificationFailureRestoresFullSpecTree_CLI(t *testing.T) {
	root := setupIssueSpecRoot(t)
	withCwd(t, root)
	writeIssueFixture(t, root, "timeout-bug", "open", "high", "")
	specRoot := filepath.Join(root, "spec")
	before, err := snapshotSpecTreeForTransaction(specRoot)
	if err != nil {
		t.Fatal(err)
	}

	originalLint := lintLintFn
	lintLintFn = func(opts lint.Options) ([]lint.Violation, error) {
		if opts.Fix {
			indexPath := filepath.Join(opts.SpecRoot, "issues", "generated", "README.md")
			if err := os.MkdirAll(filepath.Dir(indexPath), 0o755); err != nil {
				return nil, err
			}
			return nil, os.WriteFile(indexPath, []byte("lint-created issue index\n"), 0o644)
		}
		return []lint.Violation{{File: "issues/timeout-bug.md", Line: 1, Rule: "forced", Severity: "error", Message: "late issue verification failure"}}, nil
	}
	t.Cleanup(func() { lintLintFn = originalLint })

	_, _, err = runIssue(t, "change-status", "timeout-bug", "--to=investigating")
	if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Fatalf("exit = %d, want %d; err=%v", got, exitcode.Unexpected, err)
	}
	after, err := snapshotSpecTreeForTransaction(specRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("failed Issue transition left lint mutations behind\nwant: %#v\n got: %#v", before, after)
	}
}

func TestSidekickChangeStatus_LintVerificationFailureRestoresCreatedDirectory_CLI(t *testing.T) {
	root := stageQueuedSeed(t, "foo")
	specRoot := filepath.Join(root, "spec")
	archivedDirectory := filepath.Join(specRoot, "ideas", "archived")
	if err := os.RemoveAll(archivedDirectory); err != nil {
		t.Fatal(err)
	}
	before, err := snapshotSpecTreeForTransaction(specRoot)
	if err != nil {
		t.Fatal(err)
	}

	originalLint := lintLintFn
	lintLintFn = func(opts lint.Options) ([]lint.Violation, error) {
		if opts.Fix {
			return nil, nil
		}
		return []lint.Violation{{File: "ideas/seeds/foo.md", Line: 1, Rule: "forced", Severity: "error", Message: "late sidekick verification failure"}}, nil
	}
	t.Cleanup(func() { lintLintFn = originalLint })

	_, _, err = runSidekick(t, "change-status", "foo", "--to=implemented")
	if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Fatalf("exit = %d, want %d; err=%v", got, exitcode.Unexpected, err)
	}
	after, err := snapshotSpecTreeForTransaction(specRoot)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(after, before) {
		t.Fatalf("failed Sidekick transition left a created directory or other mutation behind\nwant: %#v\n got: %#v", before, after)
	}
	if _, err := os.Stat(archivedDirectory); !os.IsNotExist(err) {
		t.Fatalf("created archived directory remains after failed Sidekick transition: %v", err)
	}
}

// Every lifecycle CLI command must stop before its first domain mutation when
// the pre-transition snapshot cannot be read. Keep this table at the command
// seam: it covers the adapter branches that install the full-tree transaction.
func TestLifecycleCommands_AbortBeforeMutationWhenTransactionSnapshotFails_CLI(t *testing.T) {
	forceSnapshotFailure := func(t *testing.T) {
		t.Helper()
		originalRead := transactionReadFile
		transactionReadFile = func(string) ([]byte, error) { return nil, errors.New("forced pre-transition snapshot failure") }
		t.Cleanup(func() { transactionReadFile = originalRead })
	}
	expectUnexpected := func(t *testing.T, err error) {
		t.Helper()
		if got := exitCodeOfErr(err); got != exitcode.Unexpected {
			t.Fatalf("exit = %d, want %d; err=%v", got, exitcode.Unexpected, err)
		}
	}

	t.Run("Plan change-status", func(t *testing.T) {
		stagePlan(t, "auth", "Draft")
		forceSnapshotFailure(t)
		_, _, err := runPlan(t, "change-status", "auth", "--to=in review")
		expectUnexpected(t, err)
	})
	t.Run("Plan reconcile", func(t *testing.T) {
		stageReconcilablePlan(t, "auth", "Draft", "planning")
		forceSnapshotFailure(t)
		_, _, err := runPlan(t, "reconcile", "auth", "--tasks=complete", "--note", "record the direct delivery")
		expectUnexpected(t, err)
	})
	t.Run("Idea change-status", func(t *testing.T) {
		stageActiveIdea(t, "foo", "Draft", "")
		forceSnapshotFailure(t)
		_, _, err := runIdea(t, "change-status", "foo", "--to=approved")
		expectUnexpected(t, err)
	})
	t.Run("Idea archive", func(t *testing.T) {
		stageActiveIdea(t, "foo", "Stale", "")
		forceSnapshotFailure(t)
		_, _, err := runIdea(t, "archive", "foo")
		expectUnexpected(t, err)
	})
	t.Run("Idea unarchive", func(t *testing.T) {
		stageActiveIdea(t, "foo", "Stale", "")
		if _, _, err := runIdea(t, "archive", "foo"); err != nil {
			t.Fatalf("stage archive: %v", err)
		}
		forceSnapshotFailure(t)
		_, _, err := runIdea(t, "unarchive", "foo")
		expectUnexpected(t, err)
	})
	t.Run("Issue change-status", func(t *testing.T) {
		root := setupIssueSpecRoot(t)
		withCwd(t, root)
		writeIssueFixture(t, root, "timeout-bug", "open", "high", "")
		forceSnapshotFailure(t)
		_, _, err := runIssue(t, "change-status", "timeout-bug", "--to=investigating")
		expectUnexpected(t, err)
	})
	t.Run("Lesson change-status", func(t *testing.T) {
		stageLesson(t, "kinder-fake", "Recorded")
		forceSnapshotFailure(t)
		_, _, err := runLesson(t, "change-status", "kinder-fake", "--to=stated")
		expectUnexpected(t, err)
	})
	t.Run("Sidekick change-status", func(t *testing.T) {
		stageQueuedSeed(t, "foo")
		forceSnapshotFailure(t)
		_, _, err := runSidekick(t, "change-status", "foo", "--to=implemented")
		expectUnexpected(t, err)
	})
}
