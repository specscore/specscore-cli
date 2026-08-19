package lifecycle

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type failingArtifactLock struct{ err error }

func (l failingArtifactLock) TryLock() (bool, error) { return false, l.err }
func (l failingArtifactLock) Unlock() error          { return nil }

func TestTransformArtifact(t *testing.T) {
	p := filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(p, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := TransformArtifact(p, func(before []byte) ([]byte, error) {
		if string(before) != "before" {
			t.Fatalf("before=%q", before)
		}
		return []byte("after"), nil
	}); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(p); string(got) != "after" {
		t.Fatalf("got %q", got)
	}
}

func TestTransformArtifact_WriteFailureLeavesOriginal(t *testing.T) {
	p := filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(p, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	ops := defaultArtifactTransactionOps()
	ops.renameNoReplace = func(string, string) error { return errors.New("fenced") }
	if err := transformArtifactWithOps(p, ops, func([]byte) ([]byte, error) { return []byte("after"), nil }); err == nil {
		t.Fatal("expected write failure")
	}
	if got, _ := os.ReadFile(p); string(got) != "before" {
		t.Fatalf("original lost: %q", got)
	}
}

func TestTransformArtifact_ReadFailure(t *testing.T) {
	err := transformTo(filepath.Join(t.TempDir(), "missing.md"), []byte("after"))
	if err == nil {
		t.Fatal("expected read failure")
	}
}

func TestTransformArtifact_ConcurrentWriterIsFenced(t *testing.T) {
	p := filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(p, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	lock, err := acquireArtifactLock(p)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Unlock() }()
	if err := transformTo(p, []byte("after")); !errors.Is(err, ErrConcurrentMutation) {
		t.Fatalf("err=%v", err)
	}
}

func TestTransformArtifact_ReleasesLock(t *testing.T) {
	p := filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(p, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := transformTo(p, []byte("after")); err != nil {
		t.Fatal(err)
	}
	lock, err := acquireArtifactLock(p)
	if err != nil {
		t.Fatalf("lock was not released: %v", err)
	}
	if err := lock.Unlock(); err != nil {
		t.Fatal(err)
	}
}

func TestTransformArtifact_LockFaultLeavesOriginal(t *testing.T) {
	p := filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(p, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	ops := defaultArtifactTransactionOps()
	ops.newLock = func(string) artifactLock { return failingArtifactLock{err: errors.New("lock fault")} }
	if err := transformArtifactWithOps(p, ops, func([]byte) ([]byte, error) { return []byte("after"), nil }); err == nil {
		t.Fatal("expected lock fault")
	}
	if got, _ := os.ReadFile(p); string(got) != "before" {
		t.Fatalf("lock fault wrote %q", got)
	}
}

func TestWithArtifactTransaction_IsBoundedAndReleases(t *testing.T) {
	p := filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(p, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	lock, err := acquireArtifactLock(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := WithArtifactTransaction(p, func(*ArtifactTransaction) error { return nil }); !errors.Is(err, ErrConcurrentMutation) {
		t.Fatalf("contention=%v", err)
	}
	if err := lock.Unlock(); err != nil {
		t.Fatal(err)
	}
	if err := WithArtifactTransaction(p, func(*ArtifactTransaction) error { return nil }); err != nil {
		t.Fatalf("release=%v", err)
	}
}

// TestTransformArtifact_ForeignEditBeforeRenameIsDetected simulates a
// non-cooperating writer that mutates the artifact after WithArtifactTransaction
// captured its initial "before" snapshot but before writeFileAtomicExpectedWithOps
// takes its own pinned identity read. Injecting at ops.openIdentity (rather than
// racing a real second goroutine) makes the interleaving deterministic while still
// exercising the real on-disk compare.
func TestTransformArtifact_ForeignEditBeforeRenameIsDetected(t *testing.T) {
	p := filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(p, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	ops := defaultArtifactTransactionOps()
	real := ops.openIdentity
	ops.openIdentity = func(path string) (artifactIdentityFile, error) {
		if err := os.WriteFile(path, []byte("foreign"), 0o644); err != nil {
			return nil, err
		}
		return real(path)
	}
	err := transformArtifactWithOps(p, ops, func([]byte) ([]byte, error) { return []byte("after"), nil })
	if !errors.Is(err, ErrConcurrentMutation) {
		t.Fatalf("err=%v", err)
	}
	if got, _ := os.ReadFile(p); string(got) != "foreign" {
		t.Fatalf("got=%q", got)
	}
}

func TestArtifactTransaction_CommitOnlyOnce(t *testing.T) {
	p := filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(p, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WithArtifactTransaction(p, func(tx *ArtifactTransaction) error {
		if err := tx.Commit([]byte("after")); err != nil {
			return err
		}
		if err := tx.Commit([]byte("twice")); err == nil {
			t.Fatal("expected second commit refusal")
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
}

func TestWithArtifactTransaction_PostCommitCallbackFailureIsTyped(t *testing.T) {
	p := filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(p, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("post commit failed")
	err := WithArtifactTransaction(p, func(tx *ArtifactTransaction) error {
		if err := tx.Commit([]byte("after")); err != nil {
			return err
		}
		return boom
	})
	var committed *CommittedMutationError
	if !errors.As(err, &committed) || !errors.Is(err, boom) || committed.Phase != "post-commit transaction work" {
		t.Fatalf("err=%T %v", err, err)
	}
}

func TestCommittedMutationErrorAndNoOpCommit(t *testing.T) {
	boom := errors.New("boom")
	err := CommittedError("task.md", "fence", boom)
	var committed *CommittedMutationError
	if !errors.As(err, &committed) || !errors.Is(err, boom) || !strings.Contains(err.Error(), "task.md") || !strings.Contains(err.Error(), "fence") {
		t.Fatalf("committed error identity/text lost: %v", err)
	}
	if CommittedError("task.md", "fence", nil) != nil {
		t.Fatal("nil cause must not manufacture committed state")
	}
	tx := &ArtifactTransaction{before: []byte("same")}
	if err := tx.Commit([]byte("same")); err != nil || !tx.committed {
		t.Fatalf("no-op commit: committed=%v err=%v", tx.committed, err)
	}
}

func TestWriteFileAtomicExpected_MissingArtifact(t *testing.T) {
	t.Run("missing-file", func(t *testing.T) {
		err := writeFileAtomicExpected(filepath.Join(t.TempDir(), "missing.md"), nil, []byte("after"))
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("missing-directory", func(t *testing.T) {
		err := writeFileAtomicExpected(filepath.Join(t.TempDir(), "no-such-dir", "missing.md"), nil, []byte("after"))
		if !errors.Is(err, os.ErrNotExist) {
			t.Fatalf("err=%v", err)
		}
	})
}

// TestWriteFileAtomicExpectedWithOps_RefusesWhenArtifactChanged proves the
// core CAS refusal: writeFileAtomicExpectedWithOps must refuse to replace an
// artifact whose current bytes no longer match what the caller expected, and
// must leave the artifact byte-for-byte untouched when it refuses.
func TestWriteFileAtomicExpectedWithOps_RefusesWhenArtifactChanged(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "artifact.md")
	if err := os.WriteFile(dst, []byte("current"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := writeFileAtomicExpectedWithOps(dst, []byte("stale expectation"), []byte("after"), defaultArtifactTransactionOps())
	if !errors.Is(err, ErrConcurrentMutation) {
		t.Fatalf("err=%v", err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != "current" {
		t.Fatalf("file changed on refused CAS: %q", got)
	}
}

func TestWriteFileAtomicExpectedWithOps_IdentityReadFailure(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "artifact.md")
	if err := os.WriteFile(dst, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("identity read failed")
	ops := defaultArtifactTransactionOps()
	real := ops.openIdentity
	ops.openIdentity = func(path string) (artifactIdentityFile, error) {
		f, err := real(path)
		if err != nil {
			return nil, err
		}
		return identityFault{artifactIdentityFile: f, readErr: boom}, nil
	}
	if err := writeFileAtomicExpectedWithOps(dst, []byte("before"), []byte("after"), ops); !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
	if got, _ := os.ReadFile(dst); string(got) != "before" {
		t.Fatalf("file changed: %q", got)
	}
}

func TestWriteFileAtomicExpectedWithOps_IdentityStatFailure(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "artifact.md")
	if err := os.WriteFile(dst, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("identity stat failed")
	ops := defaultArtifactTransactionOps()
	real := ops.openIdentity
	ops.openIdentity = func(path string) (artifactIdentityFile, error) {
		f, err := real(path)
		if err != nil {
			return nil, err
		}
		return identityFault{artifactIdentityFile: f, statErr: boom}, nil
	}
	if err := writeFileAtomicExpectedWithOps(dst, []byte("before"), []byte("after"), ops); !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
	if got, _ := os.ReadFile(dst); string(got) != "before" {
		t.Fatalf("file changed: %q", got)
	}
}

func TestWriteFileAtomicExpectedWithOps_IdentityCloseFailure(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "artifact.md")
	if err := os.WriteFile(dst, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("identity close failed")
	ops := defaultArtifactTransactionOps()
	real := ops.openIdentity
	ops.openIdentity = func(path string) (artifactIdentityFile, error) {
		f, err := real(path)
		if err != nil {
			return nil, err
		}
		return identityFault{artifactIdentityFile: f, closeErr: boom}, nil
	}
	if err := writeFileAtomicExpectedWithOps(dst, []byte("before"), []byte("after"), ops); !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
	if got, _ := os.ReadFile(dst); string(got) != "before" {
		t.Fatalf("file changed: %q", got)
	}
}

// TestWriteFileAtomicExpectedWithOps_ReopenQuarantineIdentityFailure simulates a
// filesystem fault (not a race) immediately after the artifact was quarantined:
// re-opening it to verify identity fails outright. The preimage must still be
// restored to dst rather than left stranded at the quarantine path.
func TestWriteFileAtomicExpectedWithOps_ReopenQuarantineIdentityFailure(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "artifact.md")
	before := []byte("before")
	if err := os.WriteFile(dst, before, 0o644); err != nil {
		t.Fatal(err)
	}
	quarantine := dst + ".lifecycle-cas-quarantine"

	boom := errors.New("reopen quarantine identity failed")
	ops := defaultArtifactTransactionOps()
	real := ops.openIdentity
	calls := 0
	ops.openIdentity = func(path string) (artifactIdentityFile, error) {
		calls++
		if calls == 2 {
			return nil, boom
		}
		return real(path)
	}

	err := writeFileAtomicExpectedWithOps(dst, before, []byte("after"), ops)
	if !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
	if _, statErr := os.Stat(quarantine); !os.IsNotExist(statErr) {
		t.Fatalf("quarantine copy left behind: %v", statErr)
	}
	got, readErr := os.ReadFile(dst)
	if readErr != nil || !bytes.Equal(got, before) {
		t.Fatalf("dst not restored: got=%q err=%v", got, readErr)
	}
}

// TestWriteFileAtomicExpectedWithOps_ForeignReplacementDuringQuarantineIsDetected
// is the identity check's reason for existing: a decoy file with byte-identical
// content to the quarantined copy, but a distinct inode, stands in for a
// non-cooperating writer that unlinked and recreated dst with the same bytes in
// the narrow window between the initial pin and the quarantine rename. A
// bytes-only compare (as the original writeFileAtomicExpected performed alone)
// would miss this entirely; os.SameFile must not.
func TestWriteFileAtomicExpectedWithOps_ForeignReplacementDuringQuarantineIsDetected(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "artifact.md")
	before := []byte("before")
	if err := os.WriteFile(dst, before, 0o644); err != nil {
		t.Fatal(err)
	}
	decoy := filepath.Join(dir, "decoy.md")
	if err := os.WriteFile(decoy, before, 0o644); err != nil {
		t.Fatal(err)
	}
	quarantine := dst + ".lifecycle-cas-quarantine"

	ops := defaultArtifactTransactionOps()
	real := ops.openIdentity
	calls := 0
	ops.openIdentity = func(path string) (artifactIdentityFile, error) {
		calls++
		if calls == 2 {
			return real(decoy)
		}
		return real(path)
	}

	err := writeFileAtomicExpectedWithOps(dst, before, []byte("after"), ops)
	if !errors.Is(err, ErrConcurrentMutation) {
		t.Fatalf("err=%v", err)
	}
	if _, statErr := os.Stat(quarantine); !os.IsNotExist(statErr) {
		t.Fatalf("quarantine copy left behind: %v", statErr)
	}
	got, readErr := os.ReadFile(dst)
	if readErr != nil || !bytes.Equal(got, before) {
		t.Fatalf("dst not restored: got=%q err=%v", got, readErr)
	}
}

// TestWriteFileAtomicExpectedWithOps_InstallFailureRestoresPreimage covers the
// recoverable half of an install failure: the new content could not be
// installed, but the verified original was successfully moved back to dst, so
// the caller gets back a plain error rather than RecoveryRequiredError.
func TestWriteFileAtomicExpectedWithOps_InstallFailureRestoresPreimage(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "artifact.md")
	before := []byte("before")
	if err := os.WriteFile(dst, before, 0o644); err != nil {
		t.Fatal(err)
	}
	quarantine := dst + ".lifecycle-cas-quarantine"

	boom := errors.New("install failed transiently")
	ops := defaultArtifactTransactionOps()
	calls := 0
	ops.renameNoReplace = func(oldpath, newpath string) error {
		if newpath == dst {
			calls++
			if calls == 1 {
				return boom
			}
		}
		return artifactRenameNoReplace(oldpath, newpath)
	}

	err := writeFileAtomicExpectedWithOps(dst, before, []byte("after"), ops)
	if !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
	var recovery *RecoveryRequiredError
	if errors.As(err, &recovery) {
		t.Fatalf("expected a plain propagated error once restore succeeded, got RecoveryRequiredError: %v", err)
	}
	if _, statErr := os.Stat(quarantine); !os.IsNotExist(statErr) {
		t.Fatalf("quarantine copy left behind: %v", statErr)
	}
	got, readErr := os.ReadFile(dst)
	if readErr != nil || !bytes.Equal(got, before) {
		t.Fatalf("dst not restored: got=%q err=%v", got, readErr)
	}
}

// TestWriteFileAtomicExpectedWithOps_InstallFailureWithBlockedRestoreRequiresRecovery
// covers the fail-closed half: installing the new content failed AND restoring
// the quarantined original also failed (a third writer now occupies dst).
// Neither copy may be silently discarded, so both paths must be surfaced
// together via RecoveryRequiredError.
func TestWriteFileAtomicExpectedWithOps_InstallFailureWithBlockedRestoreRequiresRecovery(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "artifact.md")
	before := []byte("before")
	if err := os.WriteFile(dst, before, 0o644); err != nil {
		t.Fatal(err)
	}
	quarantine := dst + ".lifecycle-cas-quarantine"

	boom := errors.New("install blocked")
	ops := defaultArtifactTransactionOps()
	ops.renameNoReplace = func(oldpath, newpath string) error {
		if newpath == dst {
			return boom
		}
		return artifactRenameNoReplace(oldpath, newpath)
	}

	err := writeFileAtomicExpectedWithOps(dst, before, []byte("after"), ops)
	var recovery *RecoveryRequiredError
	if !errors.As(err, &recovery) {
		t.Fatalf("err=%T %v", err, err)
	}
	if recovery.Path != dst || recovery.RecoveryPath != quarantine {
		t.Fatalf("recovery paths: path=%q recoveryPath=%q", recovery.Path, recovery.RecoveryPath)
	}
	if !errors.Is(err, boom) {
		t.Fatalf("install failure not preserved: %v", err)
	}
	if !strings.Contains(err.Error(), dst) || !strings.Contains(err.Error(), quarantine) {
		t.Fatalf("error text must name both paths for manual recovery: %v", err)
	}
	// Neither copy was silently discarded: the verified preimage remains at
	// quarantine for manual recovery.
	got, readErr := os.ReadFile(quarantine)
	if readErr != nil || !bytes.Equal(got, before) {
		t.Fatalf("quarantine copy lost: got=%q err=%v", got, readErr)
	}
}

func TestWriteFileAtomicExpectedWithOps_DirectoryOpenFailureIsCommitted(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "artifact.md")
	if err := os.WriteFile(dst, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("dir open failed")
	ops := defaultArtifactTransactionOps()
	ops.openDir = func(string) (artifactTransactionFile, error) { return nil, boom }

	err := writeFileAtomicExpectedWithOps(dst, []byte("before"), []byte("after"), ops)
	var committed *CommittedMutationError
	if !errors.As(err, &committed) || !errors.Is(err, boom) || committed.Phase != "opening artifact directory for durable sync" {
		t.Fatalf("err=%T %v", err, err)
	}
	if got, _ := os.ReadFile(dst); string(got) != "after" {
		t.Fatalf("committed content lost: %q", got)
	}
}

func TestWriteFileAtomicExpectedWithOps_DirectorySyncFailureIsCommitted(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "artifact.md")
	if err := os.WriteFile(dst, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("dir sync failed")
	ops := defaultArtifactTransactionOps()
	realOpenDir := ops.openDir
	ops.openDir = func(path string) (artifactTransactionFile, error) {
		f, err := realOpenDir(path)
		if err != nil {
			return nil, err
		}
		return txFault{artifactTransactionFile: f, syncErr: boom}, nil
	}

	err := writeFileAtomicExpectedWithOps(dst, []byte("before"), []byte("after"), ops)
	var committed *CommittedMutationError
	if !errors.As(err, &committed) || !errors.Is(err, boom) || committed.Phase != "syncing artifact directory" {
		t.Fatalf("err=%T %v", err, err)
	}
	if got, _ := os.ReadFile(dst); string(got) != "after" {
		t.Fatalf("committed content lost: %q", got)
	}
}

func TestWriteFileAtomicExpectedWithOps_DirectoryCloseFailureIsCommitted(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "artifact.md")
	if err := os.WriteFile(dst, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("dir close failed")
	ops := defaultArtifactTransactionOps()
	realOpenDir := ops.openDir
	ops.openDir = func(path string) (artifactTransactionFile, error) {
		f, err := realOpenDir(path)
		if err != nil {
			return nil, err
		}
		return txFault{artifactTransactionFile: f, closeErr: boom}, nil
	}

	err := writeFileAtomicExpectedWithOps(dst, []byte("before"), []byte("after"), ops)
	var committed *CommittedMutationError
	if !errors.As(err, &committed) || !errors.Is(err, boom) || committed.Phase != "closing artifact directory" {
		t.Fatalf("err=%T %v", err, err)
	}
	if got, _ := os.ReadFile(dst); string(got) != "after" {
		t.Fatalf("committed content lost: %q", got)
	}
}

func TestWriteFileAtomicExpectedWithOps_QuarantineRemovalFailureIsCommitted(t *testing.T) {
	dir := t.TempDir()
	dst := filepath.Join(dir, "artifact.md")
	if err := os.WriteFile(dst, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	quarantine := dst + ".lifecycle-cas-quarantine"
	boom := errors.New("remove quarantine failed")
	ops := defaultArtifactTransactionOps()
	ops.remove = func(path string) error {
		if path == quarantine {
			return boom
		}
		return os.Remove(path)
	}

	err := writeFileAtomicExpectedWithOps(dst, []byte("before"), []byte("after"), ops)
	var committed *CommittedMutationError
	if !errors.As(err, &committed) || !errors.Is(err, boom) || committed.Phase != "removing artifact quarantine copy" {
		t.Fatalf("err=%T %v", err, err)
	}
	if got, _ := os.ReadFile(dst); string(got) != "after" {
		t.Fatalf("committed content lost: %q", got)
	}
	if _, statErr := os.Stat(quarantine); statErr != nil {
		t.Fatalf("quarantine copy should remain when its removal fails: %v", statErr)
	}
}

func TestOpenArtifactIdentityWithRoot_CloseFaultIsPropagated(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "artifact.md")
	if err := os.WriteFile(path, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("root close failed")
	openRoot := func(d string) (artifactRoot, error) {
		root, err := os.OpenRoot(d)
		if err != nil {
			return nil, err
		}
		return closeFaultRoot{root: root, err: boom}, nil
	}
	if _, err := openArtifactIdentityWithRoot(path, openRoot); !errors.Is(err, boom) {
		t.Fatalf("err=%v", err)
	}
}

type closeFaultRoot struct {
	root *os.Root
	err  error
}

func (r closeFaultRoot) OpenFile(name string, flag int, perm os.FileMode) (*os.File, error) {
	return r.root.OpenFile(name, flag, perm)
}

func (r closeFaultRoot) Close() error {
	realErr := r.root.Close()
	if r.err != nil {
		return r.err
	}
	return realErr
}

func TestBaseOf(t *testing.T) {
	for _, tc := range []struct{ path, want string }{
		{"/a/b/c.md", "c.md"},
		{`C:\a\b.md`, "b.md"},
		{"c.md", "c.md"},
	} {
		if got := baseOf(tc.path); got != tc.want {
			t.Errorf("baseOf(%q)=%q want %q", tc.path, got, tc.want)
		}
	}
}

func TestRewrite_LockFaultLeavesArtifactUntouched(t *testing.T) {
	p := filepath.Join(t.TempDir(), "task.md")
	before := []byte("**Status:** Draft\n")
	if err := os.WriteFile(p, before, 0o644); err != nil {
		t.Fatal(err)
	}
	ops := defaultArtifactTransactionOps()
	ops.newLock = func(string) artifactLock { return failingArtifactLock{err: errors.New("lock fault")} }
	err := transformArtifactWithOps(p, ops, func(original []byte) ([]byte, error) {
		updated, _, rewriteErr := RewriteBytes(original, IdeaApproved)
		return updated, rewriteErr
	})
	if err == nil {
		t.Fatal("expected lock fault")
	}
	if got, _ := os.ReadFile(p); !bytes.Equal(got, before) {
		t.Fatalf("lock fault wrote %q", got)
	}
}
