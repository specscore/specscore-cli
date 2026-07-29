package cli

import (
	"errors"
	"io"
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
		if err == nil || !strings.Contains(err.Error(), "opening spec tree without following links") {
			t.Fatalf("error = %v, want wrapped snapshot failure", err)
		}
	})

	t.Run("read failure", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("body"), 0o644); err != nil {
			t.Fatal(err)
		}
		originalRead := transactionReadSnapshotFile
		transactionReadSnapshotFile = func(io.Reader) ([]byte, error) { return nil, errors.New("forced read failure") }
		t.Cleanup(func() { transactionReadSnapshotFile = originalRead })

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
		if _, err := os.Lstat(link); !os.IsNotExist(err) {
			t.Fatalf("live symlink should be retained only in the recovery tree, err=%v", err)
		}
		if recovery := findLifecycleRecoveryTree(t, filepath.Dir(root)); recovery == "" {
			t.Fatal("copy-on-write restore did not retain the excluded link in recovery")
		} else if info, err := os.Lstat(filepath.Join(recovery, "link")); err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("excluded symlink not preserved for manual recovery: info=%v err=%v", info, err)
		}
	})
}

func TestSpecTreeSnapshotRestore_PreservesSymlinkReplacementInRecovery(t *testing.T) {
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
	if err := snapshot.restore(root); err != nil {
		t.Fatalf("restore error = %v", err)
	}
	if got, err := os.ReadFile(external); err != nil || string(got) != "external bytes\n" {
		t.Fatalf("rollback followed replacement symlink: %q, %v", got, err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "original\n" {
		t.Fatalf("copy-on-write restore did not publish original file: %q, %v", got, err)
	}
	recovery := findLifecycleRecoveryTree(t, filepath.Dir(root))
	if recovery == "" {
		t.Fatal("copy-on-write restore did not preserve swapped tree")
	}
	if info, err := os.Lstat(filepath.Join(recovery, "nested", "tracked.md")); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("swapped symlink is not retained in recovery: info=%v err=%v", info, err)
	}
}

func TestTransactionManifestApplication(t *testing.T) {
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
		originalSnapshot := transactionSnapshot
		transactionSnapshot = func(string) (specTreeSnapshot, error) {
			return specTreeSnapshot{}, errors.New("forced snapshot failure")
		}
		t.Cleanup(func() { transactionSnapshot = originalSnapshot })

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

	t.Run("stale legacy owner requires manual recovery", func(t *testing.T) {
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

		if _, err := beginSpecTreeTransaction(specRoot); err == nil || !strings.Contains(err.Error(), "requires manual recovery") {
			t.Fatalf("stale legacy lock was reclaimed unsafely: %v", err)
		}
		if _, err := os.Stat(lockPath); err != nil {
			t.Fatalf("manual-recovery lock was removed: %v", err)
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

func TestSpecTreeTransaction_RollbackPreservesRawFilesystemChanges(t *testing.T) {
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
		if got, readErr := os.ReadFile(managedPath); readErr != nil || string(got) != "lint changed\n" {
			t.Fatalf("managed lint path = %q, %v; unsafe rollback should preserve it beside raw work", got, readErr)
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
	err = transaction.postMutationHookWithLint(func(string) error {
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
		if _, _, err := acquireLifecycleLock(root); err == nil || !strings.Contains(err.Error(), "requires manual recovery") {
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
		if err := releaseLegacyLifecycleLock(lock); err == nil || !strings.Contains(err.Error(), "requires manual recovery") {
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
		originalPublish := transactionPublishTree
		transactionPublishTree = func(string, string) (string, error) { return "", errors.New("manifest publication failed") }
		t.Cleanup(func() { transactionPublishTree = originalPublish })
		err = transaction.postMutationHookWithLint(func(clone string) error {
			return os.WriteFile(filepath.Join(clone, "README.md"), []byte("lint"), 0o644)
		})()
		if err == nil || !strings.Contains(err.Error(), "atomically publishing isolated lint tree") {
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
				run := func(clone string) error {
					return os.WriteFile(filepath.Join(clone, "README.md"), []byte("lint"), 0o644)
				}
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
	err = transaction.postMutationHookWithLint(func(string) error {
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
