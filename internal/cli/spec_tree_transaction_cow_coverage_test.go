package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

func resetSpecTreeTransactionSeams(t *testing.T) {
	t.Helper()
	read, openFile, closeFile, lstat := transactionReadFile, transactionOpenFile, transactionCloseFile, transactionLstat
	mkdirTemp, removeAll := transactionMkdirTemp, transactionRemoveAll
	snapshot, snapshotStaged := transactionSnapshot, transactionSnapshotStaged
	stageMatches, stagePublished := transactionStageMatchesPath, transactionStagePublishedAt
	closeStaged := transactionCloseStagedTree
	publish, lockFile, alive := transactionPublishTree, transactionLockFile, transactionProcessAlive
	platform := transactionPlatformSupportsSecureMutation
	t.Cleanup(func() {
		transactionReadFile, transactionOpenFile, transactionCloseFile, transactionLstat = read, openFile, closeFile, lstat
		transactionMkdirTemp, transactionRemoveAll = mkdirTemp, removeAll
		transactionSnapshot, transactionSnapshotStaged = snapshot, snapshotStaged
		transactionStageMatchesPath, transactionStagePublishedAt = stageMatches, stagePublished
		transactionCloseStagedTree = closeStaged
		transactionPublishTree, transactionLockFile, transactionProcessAlive = publish, lockFile, alive
		transactionPlatformSupportsSecureMutation = platform
	})
}

func rootSnapshot(files map[string]string, dirs ...string) specTreeSnapshot {
	directories := map[string]specTreeDirectory{".": {mode: os.ModeDir | 0o755}}
	for _, dir := range dirs {
		directories[dir] = specTreeDirectory{mode: os.ModeDir | 0o755}
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
		if err == nil || !contains(err, "staged lint tree was replaced during publication") {
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
			if err := materializeSpecSnapshot(stage, rootSnapshot(map[string]string{"file.md": "body"})); err != nil {
				t.Fatalf("descriptor materialization error = %v", err)
			}
			invalidDirectory := rootSnapshot(nil)
			invalidDirectory.directories["../escape"] = specTreeDirectory{mode: os.ModeDir | 0o755}
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
	originalClose := transactionCloseFile
	transactionCloseFile = func(*os.File) error { return errors.New("forced close failure") }
	t.Cleanup(func() {
		transactionCloseFile = originalClose
		_ = file.Close()
	})
	if err := releaseLifecycleLockedFile(path, file); err == nil || !contains(err, "forced close failure") {
		t.Fatalf("release error = %v, want close failure", err)
	}
}

func TestLifecycleTransaction_LegacyPlatformAdapterPreservesMutationFlow(t *testing.T) {
	resetSpecTreeTransactionSeams(t)
	transactionPlatformSupportsSecureMutation = func() bool { return false }
	root := t.TempDir()
	transaction, err := beginSpecTreeTransaction(root, "ideas/foo.md")
	if err != nil {
		t.Fatal(err)
	}
	if !transaction.passthrough || transaction.lockFile != nil {
		t.Fatalf("unsupported-platform transaction = %#v, want unlocked passthrough", transaction)
	}
	if err := transaction.captureLifecycleMutationState(); err != nil {
		t.Fatalf("passthrough MutationReady = %v", err)
	}
	ran := false
	if err := transaction.postMutationHookWithLint(func(path string) error {
		ran = path == root
		return nil
	})(); err != nil {
		t.Fatalf("passthrough PostMutation = %v", err)
	}
	if !ran {
		t.Fatal("passthrough PostMutation did not retain the historical live spec root")
	}
	if err := transaction.finish(errors.New("action failure")); err == nil || err.Error() != "action failure" {
		t.Fatalf("passthrough finish = %v, want original action error", err)
	}
}

func TestLifecycleTransaction_RecoveryRetentionCapFailsBeforeMutation(t *testing.T) {
	project := t.TempDir()
	specRoot := filepath.Join(project, "spec")
	if err := os.Mkdir(specRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < maxRetainedLifecycleTrees; i++ {
		if _, err := os.MkdirTemp(project, ".specscore-lint-stage-"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := beginSpecTreeTransaction(specRoot); err == nil || !contains(err, "retention limit") {
		t.Fatalf("begin error = %v, want retention-cap conflict", err)
	}
	if _, err := os.Stat(filepath.Join(project, ".specscore-lifecycle.lock")); !os.IsNotExist(err) {
		t.Fatalf("retention cap created a lifecycle lock: %v", err)
	}
}

func TestLifecycleTransaction_ReleaseAndRetentionErrorBranches(t *testing.T) {
	t.Run("retention inspection failure", func(t *testing.T) {
		if err := ensureLifecycleRecoveryCapacity(filepath.Join(t.TempDir(), "missing", "spec")); err == nil || !contains(err, "inspecting retained") {
			t.Fatalf("retention inspection error = %v", err)
		}
	})
	t.Run("release errors retain command error", func(t *testing.T) {
		resetSpecTreeTransactionSeams(t)
		file, err := os.CreateTemp(t.TempDir(), "lock")
		if err != nil {
			t.Fatal(err)
		}
		transactionCloseFile = func(*os.File) error { return errors.New("close failed") }
		transaction := &specTreeTransaction{lockPath: file.Name(), lockFile: file}
		if err := transaction.release(); err == nil || !contains(err, "close failed") {
			t.Fatalf("release error = %v", err)
		}
		actionErr := exitcode.InvalidStateError("action failed")
		result := error(actionErr)
		releaseSpecTreeTransaction(transaction, &result)
		if !contains(result, "close failed") || !errors.Is(result, actionErr) {
			t.Fatalf("combined release result = %v", result)
		}
		if lifecycleErr, ok := result.(*lifecycleReleaseError); !ok || lifecycleErr.Unwrap() == nil || lifecycleErr.Error() == "" {
			t.Fatalf("release wrapper = %#v", result)
		}
		_ = file.Close()
	})
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
			if err := tc.run(t, root, baseline); err == nil || !contains(err, "retained") {
				t.Fatalf("publication error = %v, want retained recovery evidence", err)
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

func TestPostMutationHookWithLint_DefersAllPublicationUncertainties(t *testing.T) {
	changed := rootSnapshot(map[string]string{"README.md": "lint"})
	newTransaction := func(t *testing.T) (*specTreeTransaction, specTreeSnapshot, string) {
		t.Helper()
		root := filepath.Join(t.TempDir(), "spec")
		if err := os.Mkdir(root, 0o755); err != nil {
			t.Fatal(err)
		}
		baseline := rootSnapshot(map[string]string{"README.md": "before"})
		return &specTreeTransaction{specRoot: root, snapshot: baseline}, baseline, filepath.Join(filepath.Dir(root), "recovery")
	}
	configure := func(t *testing.T, transaction *specTreeTransaction, baseline specTreeSnapshot, recovery string) {
		t.Helper()
		resetSpecTreeTransactionSeams(t)
		transactionSnapshot = func(string) (specTreeSnapshot, error) { return baseline, nil }
		transactionSnapshotStaged = func(*stagedSpecTree) (specTreeSnapshot, error) { return changed, nil }
		transactionStageMatchesPath = func(*stagedSpecTree) (bool, error) { return true, nil }
		transactionStagePublishedAt = func(*stagedSpecTree, string) (bool, error) { return true, nil }
		transactionPublishTree = func(string, string) (string, error) { return recovery, nil }
	}
	run := func(string) error { return nil }
	assertDeferred := func(t *testing.T, err error, want string) {
		t.Helper()
		if err == nil || !lifecycle.IsDeferredRollback(err) || !contains(err, want) {
			t.Fatalf("hook error = %v, want deferred %q", err, want)
		}
	}

	t.Run("isolated lint snapshot fails", func(t *testing.T) {
		transaction, baseline, recovery := newTransaction(t)
		configure(t, transaction, baseline, recovery)
		transactionSnapshotStaged = func(*stagedSpecTree) (specTreeSnapshot, error) {
			return specTreeSnapshot{}, errors.New("clone snapshot failed")
		}
		assertDeferred(t, transaction.postMutationHookWithLint(run)(), "clone snapshot failed")
	})
	t.Run("stage identity error", func(t *testing.T) {
		transaction, baseline, recovery := newTransaction(t)
		configure(t, transaction, baseline, recovery)
		transactionStageMatchesPath = func(*stagedSpecTree) (bool, error) { return false, errors.New("stage identity failed") }
		assertDeferred(t, transaction.postMutationHookWithLint(run)(), "stage identity failed")
	})
	t.Run("stage identity mismatch", func(t *testing.T) {
		transaction, baseline, recovery := newTransaction(t)
		configure(t, transaction, baseline, recovery)
		transactionStageMatchesPath = func(*stagedSpecTree) (bool, error) { return false, nil }
		assertDeferred(t, transaction.postMutationHookWithLint(run)(), "staged lint tree was replaced")
	})
	t.Run("publish retains stage on post-claim error", func(t *testing.T) {
		transaction, baseline, recovery := newTransaction(t)
		configure(t, transaction, baseline, recovery)
		transactionPublishTree = func(string, string) (string, error) { return recovery, errors.New("claim successor exists") }
		assertDeferred(t, transaction.postMutationHookWithLint(run)(), "retained staged lint tree")
		if got := transaction.recoveryPaths; len(got) != 2 || got[0] != recovery {
			t.Fatalf("recovery paths = %v", got)
		}
	})
	t.Run("published identity error", func(t *testing.T) {
		transaction, baseline, recovery := newTransaction(t)
		configure(t, transaction, baseline, recovery)
		transactionStagePublishedAt = func(*stagedSpecTree, string) (bool, error) { return false, errors.New("published identity failed") }
		assertDeferred(t, transaction.postMutationHookWithLint(run)(), "published identity failed")
	})
	t.Run("published identity mismatch", func(t *testing.T) {
		transaction, baseline, recovery := newTransaction(t)
		configure(t, transaction, baseline, recovery)
		transactionStagePublishedAt = func(*stagedSpecTree, string) (bool, error) { return false, nil }
		assertDeferred(t, transaction.postMutationHookWithLint(run)(), "replaced during publication")
	})
	t.Run("prior live snapshot fails", func(t *testing.T) {
		transaction, baseline, recovery := newTransaction(t)
		configure(t, transaction, baseline, recovery)
		transactionSnapshot = func(path string) (specTreeSnapshot, error) {
			if path == recovery {
				return specTreeSnapshot{}, errors.New("prior snapshot failed")
			}
			return baseline, nil
		}
		assertDeferred(t, transaction.postMutationHookWithLint(run)(), "prior snapshot failed")
	})
	t.Run("prior live snapshot does not match", func(t *testing.T) {
		transaction, baseline, recovery := newTransaction(t)
		configure(t, transaction, baseline, recovery)
		transactionSnapshot = func(path string) (specTreeSnapshot, error) {
			if path == recovery {
				return changed, nil
			}
			return baseline, nil
		}
		assertDeferred(t, transaction.postMutationHookWithLint(run)(), "raw changes raced")
	})
	t.Run("final published snapshot fails", func(t *testing.T) {
		transaction, baseline, recovery := newTransaction(t)
		configure(t, transaction, baseline, recovery)
		rootCalls := 0
		transactionSnapshot = func(path string) (specTreeSnapshot, error) {
			if path == transaction.specRoot {
				rootCalls++
				if rootCalls == 3 {
					return specTreeSnapshot{}, errors.New("final snapshot failed")
				}
			}
			return baseline, nil
		}
		assertDeferred(t, transaction.postMutationHookWithLint(run)(), "final snapshot failed")
	})
	t.Run("final published snapshot differs from lint output", func(t *testing.T) {
		transaction, baseline, recovery := newTransaction(t)
		configure(t, transaction, baseline, recovery)
		rootCalls := 0
		transactionSnapshot = func(path string) (specTreeSnapshot, error) {
			if path == transaction.specRoot {
				rootCalls++
				if rootCalls == 3 {
					return rootSnapshot(map[string]string{"README.md": "raw"}), nil
				}
			}
			return baseline, nil
		}
		assertDeferred(t, transaction.postMutationHookWithLint(run)(), "while applying lint manifest")
	})
}

func TestPublishSnapshotFromHeldStage_DefersAllPublicationUncertainties(t *testing.T) {
	newState := func(t *testing.T) (string, specTreeSnapshot, string) {
		t.Helper()
		root := filepath.Join(t.TempDir(), "spec")
		if err := os.Mkdir(root, 0o755); err != nil {
			t.Fatal(err)
		}
		return root, rootSnapshot(map[string]string{"README.md": "before"}), filepath.Join(filepath.Dir(root), "recovery")
	}
	configure := func(t *testing.T, baseline specTreeSnapshot, recovery string) {
		t.Helper()
		resetSpecTreeTransactionSeams(t)
		transactionSnapshot = func(string) (specTreeSnapshot, error) { return baseline, nil }
		transactionStageMatchesPath = func(*stagedSpecTree) (bool, error) { return true, nil }
		transactionStagePublishedAt = func(*stagedSpecTree, string) (bool, error) { return true, nil }
		transactionPublishTree = func(string, string) (string, error) { return recovery, nil }
	}
	t.Run("stage identity error", func(t *testing.T) {
		root, baseline, recovery := newState(t)
		configure(t, baseline, recovery)
		transactionStageMatchesPath = func(*stagedSpecTree) (bool, error) { return false, errors.New("rollback stage identity failed") }
		if _, err := publishSnapshotFromHeldStage(root, baseline, baseline); err == nil || !contains(err, "rollback stage identity failed") {
			t.Fatalf("publish error = %v", err)
		}
	})
	t.Run("stage identity mismatch", func(t *testing.T) {
		root, baseline, recovery := newState(t)
		configure(t, baseline, recovery)
		transactionStageMatchesPath = func(*stagedSpecTree) (bool, error) { return false, nil }
		if _, err := publishSnapshotFromHeldStage(root, baseline, baseline); err == nil || !contains(err, "staged rollback tree was replaced") {
			t.Fatalf("publish error = %v", err)
		}
	})
	t.Run("published identity error", func(t *testing.T) {
		root, baseline, recovery := newState(t)
		configure(t, baseline, recovery)
		transactionStagePublishedAt = func(*stagedSpecTree, string) (bool, error) {
			return false, errors.New("rollback published identity failed")
		}
		if _, err := publishSnapshotFromHeldStage(root, baseline, baseline); err == nil || !contains(err, "rollback published identity failed") {
			t.Fatalf("publish error = %v", err)
		}
	})
	t.Run("published identity mismatch", func(t *testing.T) {
		root, baseline, recovery := newState(t)
		configure(t, baseline, recovery)
		transactionStagePublishedAt = func(*stagedSpecTree, string) (bool, error) { return false, nil }
		if _, err := publishSnapshotFromHeldStage(root, baseline, baseline); err == nil || !contains(err, "staged rollback tree was replaced") {
			t.Fatalf("publish error = %v", err)
		}
	})
	t.Run("prior live snapshot fails", func(t *testing.T) {
		root, baseline, recovery := newState(t)
		configure(t, baseline, recovery)
		transactionSnapshot = func(path string) (specTreeSnapshot, error) {
			if path == recovery {
				return specTreeSnapshot{}, errors.New("rollback prior snapshot failed")
			}
			return baseline, nil
		}
		if _, err := publishSnapshotFromHeldStage(root, baseline, baseline); err == nil || !contains(err, "rollback prior snapshot failed") {
			t.Fatalf("publish error = %v", err)
		}
	})
	t.Run("prior live snapshot differs", func(t *testing.T) {
		root, baseline, recovery := newState(t)
		configure(t, baseline, recovery)
		transactionSnapshot = func(path string) (specTreeSnapshot, error) {
			if path == recovery {
				return rootSnapshot(map[string]string{"README.md": "raw"}), nil
			}
			return baseline, nil
		}
		if _, err := publishSnapshotFromHeldStage(root, baseline, baseline); err == nil || !contains(err, "raw changes raced rollback") {
			t.Fatalf("publish error = %v", err)
		}
	})
}

func TestSpecTreeTransaction_ReleaseAndDirectoryComparisonBranches(t *testing.T) {
	resetSpecTreeTransactionSeams(t)
	file, err := os.CreateTemp(t.TempDir(), "lock")
	if err != nil {
		t.Fatal(err)
	}
	transactionCloseFile = func(*os.File) error { return errors.New("release failed") }
	result := error(nil)
	releaseSpecTreeTransaction(&specTreeTransaction{lockPath: file.Name(), lockFile: file}, &result)
	if result == nil || !contains(result, "release failed") {
		t.Fatalf("release result = %v", result)
	}
	_ = file.Close()

	left := rootSnapshot(nil)
	right := rootSnapshot(nil)
	directory := right.directories["."]
	directory.mode = os.ModeDir | 0o700
	right.directories["."] = directory
	if specTreeSnapshotsEqual(left, right) {
		t.Fatal("directory metadata difference was treated as equal")
	}
}
