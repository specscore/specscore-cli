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

// TestSnapshotSpecTreeNoFollow_RejectsEntrySwappedToSymlink exercises the
// exact race that a filepath.Walk followed by os.ReadFile could lose: a name
// is enumerated, then an editor swaps it for a link before it is opened. The
// descriptor-relative O_NOFOLLOW open must skip the new link and never read
// or modify its target.
func TestSnapshotSpecTreeNoFollow_RejectsEntrySwappedToSymlink(t *testing.T) {
	root := t.TempDir()
	tracked := filepath.Join(root, "tracked.md")
	if err := os.WriteFile(tracked, []byte("tracked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	external := filepath.Join(t.TempDir(), "external.md")
	if err := os.WriteFile(external, []byte("outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	original := transactionBeforeSnapshotOpenat
	transactionBeforeSnapshotOpenat = func(_ string, rel, name string) {
		if rel != "." || name != "tracked.md" {
			return
		}
		if err := os.Remove(tracked); err != nil {
			t.Fatalf("remove tracked entry: %v", err)
		}
		if err := os.Symlink(external, tracked); err != nil {
			t.Fatalf("replace tracked entry with symlink: %v", err)
		}
	}
	t.Cleanup(func() { transactionBeforeSnapshotOpenat = original })

	snapshot, err := snapshotSpecTreeForTransaction(root)
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	if _, found := snapshot.files["tracked.md"]; found {
		t.Fatal("snapshot accepted a symlink swapped after directory enumeration")
	}
	if got, err := os.ReadFile(external); err != nil || string(got) != "outside\n" {
		t.Fatalf("snapshot followed or changed external target: %q, %v", got, err)
	}
}

// TestPublishSpecTreeNoReplace_PreservesRawSuccessor places a raw directory
// at spec/ in the only possible gap between claiming the old tree and
// publishing the staged result. The second no-replace rename must fail rather
// than overwrite that successor; both the old tree and staged tree remain
// available for recovery.
func TestPublishSpecTreeNoReplace_PreservesRawSuccessor(t *testing.T) {
	project := t.TempDir()
	specRoot := filepath.Join(project, "spec")
	if err := os.Mkdir(specRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specRoot, "before.md"), []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	stageRoot, err := os.MkdirTemp(project, ".specscore-lint-stage-")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stageRoot, "after.md"), []byte("after\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	original := transactionAfterRecoveryClaim
	transactionAfterRecoveryClaim = func() {
		if err := os.Mkdir(specRoot, 0o755); err != nil {
			t.Fatalf("create raw successor: %v", err)
		}
		if err := os.WriteFile(filepath.Join(specRoot, "raw.md"), []byte("raw writer\n"), 0o644); err != nil {
			t.Fatalf("write raw successor: %v", err)
		}
	}
	t.Cleanup(func() { transactionAfterRecoveryClaim = original })

	recoveryPath, err := publishSpecTreeNoReplace(specRoot, stageRoot)
	if err == nil || !strings.Contains(err.Error(), "without replacing a successor") {
		t.Fatalf("publish error = %v, want no-replace conflict", err)
	}
	if recoveryPath == "" {
		t.Fatal("publish did not report the preserved recovery tree")
	}
	if got, readErr := os.ReadFile(filepath.Join(specRoot, "raw.md")); readErr != nil || string(got) != "raw writer\n" {
		t.Fatalf("raw successor was overwritten: %q, %v", got, readErr)
	}
	if got, readErr := os.ReadFile(filepath.Join(recoveryPath, "before.md")); readErr != nil || string(got) != "before\n" {
		t.Fatalf("pre-publication tree was not retained: %q, %v", got, readErr)
	}
	if got, readErr := os.ReadFile(filepath.Join(stageRoot, "after.md")); readErr != nil || string(got) != "after\n" {
		t.Fatalf("staged tree was lost after no-replace conflict: %q, %v", got, readErr)
	}
}

// The lifecycle hook must not clean up a staged tree after the publisher has
// moved the old tree aside but refused to replace a raw successor. Both trees
// are necessary to reconcile the interrupted publication.
func TestPostMutationHook_PublishConflictRetainsBothCandidateTrees(t *testing.T) {
	project := t.TempDir()
	specRoot := filepath.Join(project, "spec")
	if err := os.Mkdir(specRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specRoot, "README.md"), []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	transaction, err := beginSpecTreeTransaction(specRoot)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = transaction.release() })

	original := transactionAfterRecoveryClaim
	transactionAfterRecoveryClaim = func() {
		if err := os.Mkdir(specRoot, 0o755); err != nil {
			t.Fatalf("create raw successor: %v", err)
		}
		if err := os.WriteFile(filepath.Join(specRoot, "raw.md"), []byte("raw writer\n"), 0o644); err != nil {
			t.Fatalf("write raw successor: %v", err)
		}
	}
	t.Cleanup(func() { transactionAfterRecoveryClaim = original })

	err = transaction.postMutationHookWithLint(func(stageRoot string) error {
		return os.WriteFile(filepath.Join(stageRoot, "README.md"), []byte("lint output\n"), 0o644)
	})()
	if err == nil || !strings.Contains(err.Error(), "retained staged lint tree") {
		t.Fatalf("hook error = %v, want preserved stage", err)
	}
	if got, readErr := os.ReadFile(filepath.Join(specRoot, "raw.md")); readErr != nil || string(got) != "raw writer\n" {
		t.Fatalf("raw successor = %q, %v", got, readErr)
	}
	if recovery := findLifecycleRecoveryTree(t, project); recovery == "" {
		t.Fatal("old spec tree was not retained for recovery")
	} else if got, readErr := os.ReadFile(filepath.Join(recovery, "README.md")); readErr != nil || string(got) != "before\n" {
		t.Fatalf("recovery tree = %q, %v", got, readErr)
	}
	for _, entry := range mustReadDir(t, project) {
		if strings.HasPrefix(entry.Name(), ".specscore-lint-stage-") {
			if got, readErr := os.ReadFile(filepath.Join(project, entry.Name(), "README.md")); readErr != nil || string(got) != "lint output\n" {
				t.Fatalf("staged lint tree = %q, %v", got, readErr)
			}
			return
		}
	}
	t.Fatal("staged lint tree was discarded after publication conflict")
}

func mustReadDir(t *testing.T, path string) []os.DirEntry {
	t.Helper()
	entries, err := os.ReadDir(path)
	if err != nil {
		t.Fatal(err)
	}
	return entries
}

// TestAcquireLifecycleLock_StaleLegacyDirectoryRequiresManualRecovery proves
// the legacy Lstat/read/revalidate path no longer unlinks a stale directory:
// the pathname could have become a successor between those operations.
func TestAcquireLifecycleLock_StaleLegacyDirectoryRequiresManualRecovery(t *testing.T) {
	project := t.TempDir()
	lockPath := filepath.Join(project, ".specscore-lifecycle.lock")
	if err := os.Mkdir(lockPath, 0o700); err != nil {
		t.Fatal(err)
	}
	ownerPath := lifecycleLockOwnerPath(lockPath)
	if err := os.WriteFile(ownerPath, []byte("12345\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	originalAlive := transactionProcessAlive
	transactionProcessAlive = func(int) bool { return false }
	t.Cleanup(func() { transactionProcessAlive = originalAlive })

	_, _, err := acquireLifecycleLock(project)
	if err == nil || !strings.Contains(err.Error(), "requires manual recovery") {
		t.Fatalf("acquire error = %v, want manual legacy recovery", err)
	}
	if info, statErr := os.Stat(lockPath); statErr != nil || !info.IsDir() {
		t.Fatalf("stale legacy lock was removed: info=%v err=%v", info, statErr)
	}
	if got, readErr := os.ReadFile(ownerPath); readErr != nil || string(got) != "12345\n" {
		t.Fatalf("legacy lock owner was changed: %q, %v", got, readErr)
	}
}

func TestSnapshotSpecTreeNoFollow_DescriptorReadFailure(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "README.md"), []byte("body"), 0o644); err != nil {
		t.Fatal(err)
	}
	original := transactionReadSnapshotFile
	transactionReadSnapshotFile = func(io.Reader) ([]byte, error) { return nil, errors.New("descriptor read failed") }
	t.Cleanup(func() { transactionReadSnapshotFile = original })
	if _, err := snapshotSpecTreeForTransaction(root); err == nil || !strings.Contains(err.Error(), "descriptor read failed") {
		t.Fatalf("snapshot error = %v, want descriptor read failure", err)
	}
}
