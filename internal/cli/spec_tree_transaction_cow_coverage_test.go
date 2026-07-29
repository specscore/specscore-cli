package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

func resetSpecTreeTransactionSeams(t *testing.T) {
	t.Helper()
	read, openFile, closeFile, lstat := transactionReadFile, transactionOpenFile, transactionCloseFile, transactionLstat
	mkdirTemp, chmod, removeAll := transactionMkdirTemp, transactionChmod, transactionRemoveAll
	scratchMkdir, scratchWrite := transactionScratchMkdir, transactionScratchWrite
	snapshot, publish, lockFile, alive := transactionSnapshot, transactionPublishTree, transactionLockFile, transactionProcessAlive
	t.Cleanup(func() {
		transactionReadFile, transactionOpenFile, transactionCloseFile, transactionLstat = read, openFile, closeFile, lstat
		transactionMkdirTemp, transactionChmod, transactionRemoveAll = mkdirTemp, chmod, removeAll
		transactionScratchMkdir, transactionScratchWrite = scratchMkdir, scratchWrite
		transactionSnapshot, transactionPublishTree, transactionLockFile, transactionProcessAlive = snapshot, publish, lockFile, alive
	})
}

func rootSnapshot(files map[string]string, dirs ...string) specTreeSnapshot {
	directories := map[string]os.FileMode{".": os.ModeDir | 0o755}
	for _, dir := range dirs {
		directories[dir] = os.ModeDir | 0o755
	}
	result := specTreeSnapshot{directories: directories, files: map[string]specTreeFile{}}
	for name, content := range files {
		result.files[name] = specTreeFile{content: []byte(content), mode: 0o644}
	}
	return result
}

func TestSpecTreeTransaction_FailClosedCoverage(t *testing.T) {
	t.Run("begin and legacy lock classifications", func(t *testing.T) {
		if _, err := beginSpecTreeTransaction(t.TempDir(), ".."); err == nil {
			t.Fatal("invalid owned path was accepted")
		}

		t.Run("generic file lock error", func(t *testing.T) {
			resetSpecTreeTransactionSeams(t)
			root := t.TempDir()
			transactionLockFile = func(*os.File) error { return errors.New("lock syscall failed") }
			if _, _, err := acquireLifecycleLock(root); err == nil || !contains(err, "lock syscall failed") {
				t.Fatalf("acquire error = %v", err)
			}
		})
		t.Run("held file lock", func(t *testing.T) {
			resetSpecTreeTransactionSeams(t)
			transactionLockFile = func(*os.File) error { return errLifecycleLockHeld }
			if _, _, err := acquireLifecycleLock(t.TempDir()); err == nil || !contains(err, "another SpecScore lifecycle transaction") {
				t.Fatalf("acquire error = %v", err)
			}
		})
		t.Run("open failure is not mistaken for legacy directory", func(t *testing.T) {
			resetSpecTreeTransactionSeams(t)
			transactionOpenFile = func(string, int, os.FileMode) (*os.File, error) { return nil, errors.New("open denied") }
			if _, _, err := acquireLifecycleLock(t.TempDir()); err == nil || !contains(err, "open denied") {
				t.Fatalf("acquire error = %v", err)
			}
		})
		t.Run("legacy inspection and owner variants", func(t *testing.T) {
			root := t.TempDir()
			lock := filepath.Join(root, ".specscore-lifecycle.lock")
			if err := os.Mkdir(lock, 0o700); err != nil {
				t.Fatal(err)
			}
			if stale, err := lifecycleLegacyLockIsStale(lock); err != nil || stale {
				t.Fatalf("ownerless stale = %v, %v", stale, err)
			}
			owner := lifecycleLockOwnerPath(lock)
			if err := os.Mkdir(owner, 0o700); err != nil {
				t.Fatal(err)
			}
			if _, err := lifecycleLegacyLockIsStale(lock); err == nil || !contains(err, "non-regular") {
				t.Fatalf("nonregular owner error = %v", err)
			}
			if err := os.Remove(owner); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(owner, []byte("bad\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			if stale, err := lifecycleLegacyLockIsStale(lock); err != nil || !stale {
				t.Fatalf("malformed stale = %v, %v", stale, err)
			}
			if err := os.WriteFile(owner, []byte("42\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			resetSpecTreeTransactionSeams(t)
			transactionReadFile = func(string) ([]byte, error) { return nil, errors.New("owner read failed") }
			if _, err := lifecycleLegacyLockIsStale(lock); err == nil || !contains(err, "owner read failed") {
				t.Fatalf("owner read error = %v", err)
			}
		})
	})

	t.Run("snapshot staging and validation failures", func(t *testing.T) {
		invalid := rootSnapshot(nil)
		invalid.files["../escape.md"] = specTreeFile{content: []byte("x"), mode: 0o644}
		if _, err := stageSpecTreeSnapshot(filepath.Join(t.TempDir(), "spec"), invalid); err == nil || !contains(err, "invalid snapshot path") {
			t.Fatalf("invalid stage error = %v", err)
		}
		if err := validateSpecTreeSnapshot(specTreeSnapshot{}); err == nil || !contains(err, "missing") {
			t.Fatalf("missing root validation error = %v", err)
		}
		for _, path := range []string{".", "nested/../file.md", "/absolute.md"} {
			if err := validateSnapshotRelativePath(path); err == nil {
				t.Fatalf("invalid relative path accepted: %q", path)
			}
		}
		resetSpecTreeTransactionSeams(t)
		root := t.TempDir()
		snapshot := rootSnapshot(map[string]string{"file.md": "body"})
		transactionMkdirTemp = func(string, string) (string, error) { return "", errors.New("stage temp failed") }
		if _, err := stageSpecTreeSnapshot(filepath.Join(root, "spec"), snapshot); err == nil || !contains(err, "stage temp failed") {
			t.Fatalf("temp stage error = %v", err)
		}
		transactionMkdirTemp = os.MkdirTemp
		transactionChmod = func(string, os.FileMode) error { return errors.New("stage chmod failed") }
		if _, err := stageSpecTreeSnapshot(filepath.Join(root, "spec"), snapshot); err == nil || !contains(err, "stage chmod failed") {
			t.Fatalf("chmod stage error = %v", err)
		}
		transactionChmod = os.Chmod
		transactionScratchMkdir = func(path string, mode os.FileMode) error {
			if filepath.Base(path) == "." { // retained for clarity; actual root is allowed below.
				return os.MkdirAll(path, mode)
			}
			return errors.New("scratch mkdir failed")
		}
		if _, err := stageSpecTreeSnapshot(filepath.Join(root, "spec"), snapshot); err == nil || !contains(err, "scratch mkdir failed") {
			t.Fatalf("materialize stage error = %v", err)
		}
	})

	t.Run("lifecycle hook mismatch, capture and finish failures", func(t *testing.T) {
		root := t.TempDir()
		if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("before"), 0o644); err != nil {
			t.Fatal(err)
		}
		transaction, err := beginSpecTreeTransaction(root)
		if err != nil {
			t.Fatal(err)
		}
		defer func() { _ = transaction.release() }()
		resetSpecTreeTransactionSeams(t)
		transactionPublishTree = func(specRoot, _ string) (string, error) {
			return "recovery", os.WriteFile(filepath.Join(specRoot, "README.md"), []byte("raw successor"), 0o644)
		}
		err = transaction.postMutationHookWithLint(func(clone string) error {
			return os.WriteFile(filepath.Join(clone, "README.md"), []byte("lint output"), 0o644)
		})()
		if err == nil || !contains(err, "while applying lint manifest") {
			t.Fatalf("mismatch hook error = %v", err)
		}

		transaction = &specTreeTransaction{specRoot: root}
		transactionSnapshot = func(string) (specTreeSnapshot, error) { return specTreeSnapshot{}, errors.New("capture failed") }
		if err := transaction.captureLifecycleMutationState(); err == nil || !lifecycle.IsDeferredRollback(err) || !contains(err, "capture failed") {
			t.Fatalf("capture error = %v", err)
		}
		if err := transaction.finish(errors.New("action failed")); err == nil || !contains(err, "ownership could not be established") {
			t.Fatalf("finish missing state error = %v", err)
		}

		baseline := rootSnapshot(nil)
		transaction = &specTreeTransaction{specRoot: root, snapshot: baseline, preLintSnapshot: &baseline, postLintSnapshot: &baseline, postMutationStarted: true}
		transactionSnapshot = func(string) (specTreeSnapshot, error) {
			return specTreeSnapshot{}, errors.New("restore snapshot failed")
		}
		if err := transaction.finish(errors.New("action failed")); err == nil || !contains(err, "could not restore") {
			t.Fatalf("finish restore error = %v", err)
		}
	})

	t.Run("rollback conflict, stage, publication and helper branches", func(t *testing.T) {
		root := t.TempDir()
		before := rootSnapshot(map[string]string{"tracked.md": "before"})
		post := rootSnapshot(map[string]string{"tracked.md": "lint"})
		transaction := &specTreeTransaction{specRoot: root, snapshot: before, postLintSnapshot: &post}
		resetSpecTreeTransactionSeams(t)
		transactionSnapshot = func(string) (specTreeSnapshot, error) {
			return rootSnapshot(map[string]string{"tracked.md": "raw"}), nil
		}
		if err := transaction.restoreLintMutations(); err == nil || !contains(err, "tracked.md") {
			t.Fatalf("file conflict error = %v", err)
		}

		before = rootSnapshot(nil)
		post = rootSnapshot(nil, "generated")
		transaction = &specTreeTransaction{specRoot: root, snapshot: before, postLintSnapshot: &post}
		transactionSnapshot = func(string) (specTreeSnapshot, error) { return rootSnapshot(nil, "generated", "generated/raw"), nil }
		if err := transaction.restoreLintMutations(); err == nil || !contains(err, "generated/") {
			t.Fatalf("directory conflict error = %v", err)
		}

		actualRoot := t.TempDir()
		if err := os.WriteFile(filepath.Join(actualRoot, "README.md"), []byte("lint"), 0o644); err != nil {
			t.Fatal(err)
		}
		before, err := snapshotSpecTreeForTransaction(actualRoot)
		if err != nil {
			t.Fatal(err)
		}
		before.files["README.md"] = specTreeFile{content: []byte("before"), mode: 0o644}
		post, err = snapshotSpecTreeForTransaction(actualRoot)
		if err != nil {
			t.Fatal(err)
		}
		transaction = &specTreeTransaction{specRoot: actualRoot, snapshot: before, postLintSnapshot: &post}
		transactionSnapshot = snapshotSpecTreeForTransaction
		transactionPublishTree = func(string, string) (string, error) { return "", errors.New("publish failed") }
		if err := transaction.restoreLintMutations(); err == nil || !contains(err, "publish failed") {
			t.Fatalf("publication error = %v", err)
		}
		transactionPublishTree = publishSpecTreeNoReplace
		if err := transaction.restoreLintMutations(); err != nil {
			t.Fatalf("copy-on-write rollback: %v", err)
		}

		if !snapshotDirectoryHasExternalDescendant(rootSnapshot(map[string]string{"nested/raw.md": "raw"}, "nested"), rootSnapshot(nil, "nested"), "nested") {
			t.Fatal("external file descendant was missed")
		}
		if !snapshotDirectoryHasExternalDescendant(rootSnapshot(nil, "nested", "nested/raw"), rootSnapshot(nil, "nested"), "nested") {
			t.Fatal("external directory descendant was missed")
		}
	})

	t.Run("remaining classifier, rollback and compatibility branches", func(t *testing.T) {
		t.Run("legacy classifier errors and active owner", func(t *testing.T) {
			root := t.TempDir()
			lock := filepath.Join(root, ".specscore-lifecycle.lock")
			if err := os.WriteFile(lock, []byte("not a directory"), 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := legacyLifecycleLockInfo(lock); err == nil || !contains(err, "not a directory") {
				t.Fatalf("legacy file error = %v", err)
			}
			if err := os.Remove(lock); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(root, lock); err == nil {
				if _, err := legacyLifecycleLockInfo(lock); err == nil || !contains(err, "symlinked") {
					t.Fatalf("legacy symlink error = %v", err)
				}
				if err := os.Remove(lock); err != nil {
					t.Fatal(err)
				}
			}
			if err := os.Mkdir(lock, 0o700); err != nil {
				t.Fatal(err)
			}
			owner := lifecycleLockOwnerPath(lock)
			if err := os.WriteFile(owner, []byte("42\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			resetSpecTreeTransactionSeams(t)
			originalLstat := transactionLstat
			transactionLstat = func(path string) (os.FileInfo, error) {
				if path == owner {
					return nil, errors.New("owner inspection failed")
				}
				return originalLstat(path)
			}
			if _, err := lifecycleLegacyLockIsStale(lock); err == nil || !contains(err, "owner inspection failed") {
				t.Fatalf("owner inspection error = %v", err)
			}
			transactionLstat = originalLstat
			transactionProcessAlive = func(int) bool { return true }
			if _, _, err := acquireLifecycleLock(root); err == nil || !contains(err, "another SpecScore lifecycle transaction") {
				t.Fatalf("active legacy acquire error = %v", err)
			}
			transactionLstat = func(path string) (os.FileInfo, error) {
				if path == owner {
					return nil, errors.New("legacy stale inspection failed")
				}
				return originalLstat(path)
			}
			if _, _, err := acquireLifecycleLock(root); err == nil || !contains(err, "legacy stale inspection failed") {
				t.Fatalf("legacy stale acquire error = %v", err)
			}
		})

		t.Run("materialize parent, directory ownership and no-op rollback paths", func(t *testing.T) {
			resetSpecTreeTransactionSeams(t)
			stage := t.TempDir()
			calls := 0
			transactionScratchMkdir = func(path string, mode os.FileMode) error {
				calls++
				if calls == 1 {
					return os.MkdirAll(path, mode)
				}
				return errors.New("file parent failed")
			}
			if err := materializeSpecSnapshot(stage, rootSnapshot(map[string]string{"file.md": "body"})); err == nil || !contains(err, "file parent failed") {
				t.Fatalf("materialize parent error = %v", err)
			}
			transactionScratchMkdir = os.MkdirAll
			invalidDirectory := rootSnapshot(nil)
			invalidDirectory.directories["../escape"] = os.ModeDir | 0o755
			if err := validateSpecTreeSnapshot(invalidDirectory); err == nil || !contains(err, "invalid snapshot path") {
				t.Fatalf("directory validation error = %v", err)
			}
			baseline, changed := rootSnapshot(nil), rootSnapshot(nil, "new")
			if paths := (&specTreeTransaction{snapshot: baseline}).unownedPreLintPaths(changed); len(paths) != 1 || paths[0] != "new/" {
				t.Fatalf("unowned directory paths = %v", paths)
			}
			if paths := changedSnapshotPaths(baseline, changed); len(paths) != 1 || paths[0] != "new/" {
				t.Fatalf("changed directory paths = %v", paths)
			}

			for _, tc := range []struct {
				name    string
				before  specTreeSnapshot
				post    specTreeSnapshot
				current specTreeSnapshot
			}{
				{"file", rootSnapshot(map[string]string{"tracked.md": "before"}), rootSnapshot(map[string]string{"tracked.md": "lint"}), rootSnapshot(map[string]string{"tracked.md": "before"})},
				{"directory", rootSnapshot(nil), rootSnapshot(nil, "generated"), rootSnapshot(nil)},
			} {
				t.Run(tc.name, func(t *testing.T) {
					resetSpecTreeTransactionSeams(t)
					root := t.TempDir()
					transaction := &specTreeTransaction{specRoot: root, snapshot: tc.before, postLintSnapshot: &tc.post}
					transactionSnapshot = func(string) (specTreeSnapshot, error) { return tc.current, nil }
					if err := transaction.restoreLintMutations(); err != nil {
						t.Fatalf("no-op rollback = %v", err)
					}
				})
			}
		})

		t.Run("rollback stage and compatibility publication failures", func(t *testing.T) {
			resetSpecTreeTransactionSeams(t)
			root := t.TempDir()
			baseline := rootSnapshot(nil)
			transaction := &specTreeTransaction{specRoot: root, snapshot: baseline, postLintSnapshot: &baseline}
			transactionSnapshot = func(string) (specTreeSnapshot, error) { return baseline, nil }
			transactionMkdirTemp = func(string, string) (string, error) { return "", errors.New("rollback stage failed") }
			if err := transaction.restoreLintMutations(); err == nil || !contains(err, "rollback stage failed") {
				t.Fatalf("rollback stage error = %v", err)
			}
			if err := applySpecSnapshotDiff(root, baseline, baseline); err == nil || !contains(err, "rollback stage failed") {
				t.Fatalf("compatibility stage error = %v", err)
			}
			transactionMkdirTemp = os.MkdirTemp
			transactionSnapshot = func(string) (specTreeSnapshot, error) {
				return specTreeSnapshot{}, errors.New("manifest snapshot failed")
			}
			if err := applySpecSnapshotDiff(root, baseline, baseline); err == nil || !contains(err, "manifest snapshot failed") {
				t.Fatalf("compatibility snapshot error = %v", err)
			}
			transactionSnapshot = func(string) (specTreeSnapshot, error) { return rootSnapshot(map[string]string{"raw.md": "raw"}), nil }
			if err := applySpecSnapshotDiff(root, baseline, baseline); err == nil || !contains(err, "raw.md") {
				t.Fatalf("compatibility conflict error = %v", err)
			}
			transactionSnapshot = func(string) (specTreeSnapshot, error) { return baseline, nil }
			transactionPublishTree = func(string, string) (string, error) { return "", errors.New("manifest publish failed") }
			if err := applySpecSnapshotDiff(root, baseline, baseline); err == nil || !contains(err, "manifest publish failed") {
				t.Fatalf("compatibility publish error = %v", err)
			}
			invalid := rootSnapshot(nil)
			invalid.files["../escape.md"] = specTreeFile{content: []byte("bad"), mode: 0o644}
			if err := invalid.restore(root); err == nil || !contains(err, "invalid snapshot path") {
				t.Fatalf("restore stage error = %v", err)
			}
			valid := rootSnapshot(nil)
			transactionPublishTree = func(string, string) (string, error) { return "", errors.New("restore publish failed") }
			if err := valid.restore(root); err == nil || !contains(err, "restore publish failed") {
				t.Fatalf("restore publication error = %v", err)
			}
		})
	})
}

// Each lifecycle verb must acquire the full-tree transaction before it asks
// its domain package to mutate anything. Force the snapshot step to fail after
// every verb's own validation/resolution work so that a future refactor cannot
// accidentally turn a transaction-start failure into a partial mutation.
func TestLifecycleVerbs_AbortWhenTransactionSnapshotFails(t *testing.T) {
	forceSnapshotFailure := func(t *testing.T) {
		t.Helper()
		resetSpecTreeTransactionSeams(t)
		transactionSnapshot = func(string) (specTreeSnapshot, error) {
			return specTreeSnapshot{}, errors.New("forced transaction snapshot failure")
		}
	}
	requireSnapshotFailure := func(t *testing.T, err error) {
		t.Helper()
		if err == nil || !contains(err, "forced transaction snapshot failure") {
			t.Fatalf("command error = %v, want transaction snapshot failure", err)
		}
	}

	t.Run("idea change-status archive and unarchive", func(t *testing.T) {
		for _, invocation := range [][]string{
			{"change-status", "foo", "--to=approved"},
			{"archive", "foo"},
			{"unarchive", "foo"},
		} {
			invocation := invocation
			t.Run(invocation[0], func(t *testing.T) {
				root := setupSpecRoot(t)
				withCwd(t, root)
				forceSnapshotFailure(t)
				_, _, err := runIdea(t, invocation...)
				requireSnapshotFailure(t, err)
			})
		}
	})

	t.Run("issue change-status", func(t *testing.T) {
		root := setupIssueSpecRoot(t)
		withCwd(t, root)
		forceSnapshotFailure(t)
		_, _, err := runIssue(t, "change-status", "foo", "--to=investigating")
		requireSnapshotFailure(t, err)
	})

	t.Run("lesson change-status", func(t *testing.T) {
		setupLessonsSpec(t)
		forceSnapshotFailure(t)
		_, _, err := runLesson(t, "change-status", "foo", "--to=stated")
		requireSnapshotFailure(t, err)
	})

	t.Run("plan reconcile", func(t *testing.T) {
		setupPlansSpec(t)
		forceSnapshotFailure(t)
		_, _, err := runPlan(t, "reconcile", "foo", "--tasks=complete", "--note=recorded directly")
		requireSnapshotFailure(t, err)
	})

	t.Run("sidekick change-status", func(t *testing.T) {
		stageQueuedSeed(t, "foo")
		forceSnapshotFailure(t)
		_, _, err := runSidekick(t, "change-status", "foo", "--to=implemented")
		requireSnapshotFailure(t, err)
	})
}

func TestReleaseLifecycleLockedFile_CloseFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "lock")
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	originalRemove, originalClose := transactionRemove, transactionCloseFile
	transactionRemove = os.Remove
	transactionCloseFile = func(*os.File) error { return errors.New("forced close failure") }
	t.Cleanup(func() {
		transactionRemove, transactionCloseFile = originalRemove, originalClose
		_ = file.Close()
	})
	if err := releaseLifecycleLockedFile(path, file); err == nil || !contains(err, "forced close failure") {
		t.Fatalf("release error = %v, want close failure", err)
	}
}

func TestStagedPublicationConflict_RetainsAllStagedRecoveryForms(t *testing.T) {
	for _, tc := range []struct {
		name string
		run  func(t *testing.T, root string, baseline specTreeSnapshot) error
	}{
		{
			name: "transaction rollback",
			run: func(t *testing.T, root string, baseline specTreeSnapshot) error {
				transaction := &specTreeTransaction{specRoot: root, snapshot: baseline, postLintSnapshot: &baseline}
				return transaction.restoreLintMutations()
			},
		},
		{
			name: "manifest compatibility helper",
			run: func(_ *testing.T, root string, baseline specTreeSnapshot) error {
				return applySpecSnapshotDiff(root, baseline, baseline)
			},
		},
		{
			name: "snapshot restore",
			run: func(_ *testing.T, root string, baseline specTreeSnapshot) error {
				return baseline.restore(root)
			},
		},
	} {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			resetSpecTreeTransactionSeams(t)
			root := filepath.Join(t.TempDir(), "spec")
			if err := os.Mkdir(root, 0o755); err != nil {
				t.Fatal(err)
			}
			baseline := rootSnapshot(map[string]string{"README.md": "before"})
			transactionSnapshot = func(string) (specTreeSnapshot, error) { return baseline, nil }
			transactionPublishTree = func(string, string) (string, error) {
				return filepath.Join(filepath.Dir(root), "recovery"), errors.New("raw successor")
			}
			if err := tc.run(t, root, baseline); err == nil || !contains(err, "retained staged") {
				t.Fatalf("publication error = %v, want retained stage", err)
			}
			entries, err := os.ReadDir(filepath.Dir(root))
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				if strings.HasPrefix(entry.Name(), ".specscore-lint-stage-") {
					return
				}
			}
			t.Fatal("publish conflict discarded the staged recovery tree")
		})
	}
}
