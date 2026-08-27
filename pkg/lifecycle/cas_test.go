package lifecycle

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type failingArtifactLock struct{ err error }

func (l failingArtifactLock) TryLock() (bool, error) { return false, l.err }
func (l failingArtifactLock) Unlock() error          { return nil }

type configuredArtifactLock struct {
	locked    bool
	tryErr    error
	unlockErr error
}

func (l configuredArtifactLock) TryLock() (bool, error) { return l.locked, l.tryErr }
func (l configuredArtifactLock) Unlock() error          { return l.unlockErr }

func TestArtifactTransactionLock_UnlockErrorsAndReacquireGuard(t *testing.T) {
	if ok, err := (&artifactTransactionLock{}).TryLock(); ok || err == nil {
		t.Fatal("transaction wrapper unexpectedly reacquired")
	}

	t.Run("artifact unlock failure", func(t *testing.T) {
		state := &lifecycleProcessState
		<-state.gate
		state.mu.Lock()
		state.active.Add("artifact")
		state.mu.Unlock()
		lock := &artifactTransactionLock{
			project:      configuredArtifactLock{unlockErr: errors.New("project unlock")},
			artifact:     configuredArtifactLock{unlockErr: errors.New("artifact unlock")},
			artifactPath: "artifact",
			remove:       func(string) error { t.Fatal("removed while artifact unlock failed"); return nil },
			processState: state,
		}
		if err := lock.Unlock(); err == nil || !strings.Contains(err.Error(), "artifact unlock") || !strings.Contains(err.Error(), "project unlock") {
			t.Fatalf("unlock error = %v", err)
		}
		if err := lock.Unlock(); err == nil {
			t.Fatal("second unlock lost the original error")
		}
	})

	t.Run("remove failure", func(t *testing.T) {
		state := &lifecycleProcessState
		<-state.gate
		lock := &artifactTransactionLock{
			project:      configuredArtifactLock{},
			artifact:     configuredArtifactLock{},
			artifactPath: "artifact",
			remove:       func(string) error { return errors.New("remove failed") },
			processState: state,
		}
		if err := lock.Unlock(); err == nil || !strings.Contains(err.Error(), "remove failed") {
			t.Fatalf("unlock error = %v", err)
		}
	})
}

func artifactLockSequence(locks ...artifactLock) func(string) artifactLock {
	i := 0
	return func(string) artifactLock {
		lock := locks[i]
		i++
		return lock
	}
}

func TestAcquireArtifactLockWithOps_FailureBoundaries(t *testing.T) {
	tests := []struct {
		name string
		new  func(string) artifactLock
	}{
		{name: "project error", new: func(string) artifactLock { return configuredArtifactLock{tryErr: errors.New("project")} }},
		{name: "project contention", new: func(string) artifactLock { return configuredArtifactLock{} }},
		{name: "artifact error", new: artifactLockSequence(
			configuredArtifactLock{locked: true},
			configuredArtifactLock{tryErr: errors.New("artifact")},
		)},
		{name: "artifact contention", new: artifactLockSequence(
			configuredArtifactLock{locked: true},
			configuredArtifactLock{},
		)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ops := defaultArtifactTransactionOps()
			ops.newLock = tt.new
			_, err := acquireArtifactLockWithOps(filepath.Join(t.TempDir(), "artifact.md"), ops)
			if err == nil {
				t.Fatal("expected acquisition failure")
			}
		})
	}
}

func TestLifecycleLockRoot_RecognizesProjectAndStagedRoots(t *testing.T) {
	tests := []struct {
		name string
		prep func()
		path func(string) string
	}{
		{name: "spec boundary", path: func(root string) string { return filepath.Join(root, "spec", "features", "auth", "README.md") }},
		{name: "staged transaction", path: func(root string) string {
			return filepath.Join(root, ".specscore-txn-abc", "artifact.md")
		}},
		{name: "config marker", prep: func() {}, path: func(root string) string {
			_ = os.WriteFile(filepath.Join(root, "specscore.yaml"), nil, 0o644)
			return filepath.Join(root, "docs", "artifact.md")
		}},
		{name: "git marker", prep: func() {}, path: func(root string) string {
			_ = os.Mkdir(filepath.Join(root, ".git"), 0o755)
			return filepath.Join(root, "docs", "artifact.md")
		}},
		{name: "directory fallback", path: func(root string) string { return filepath.Join(root, "docs", "artifact.md") }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := t.TempDir()
			if tt.prep != nil {
				tt.prep()
			}
			path := tt.path(root)
			want := root
			switch tt.name {
			case "staged transaction":
				want = filepath.Join(root, ".specscore-txn-abc")
			case "directory fallback":
				want = filepath.Join(root, "docs")
			}
			if got := lifecycleLockRoot(path); got != want {
				t.Fatalf("root = %q, want %q", got, want)
			}
		})
	}
}

func TestLifecycleLockRoot_AbsFailureFallsBackToDirectory(t *testing.T) {
	original := lifecycleLockAbs
	lifecycleLockAbs = func(string) (string, error) { return "", errors.New("abs failed") }
	t.Cleanup(func() { lifecycleLockAbs = original })
	path := filepath.Join(t.TempDir(), "artifact.md")
	if got, want := lifecycleLockRoot(path), filepath.Dir(path); got != want {
		t.Fatalf("root = %q, want %q", got, want)
	}
}

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

func TestTransformArtifact_RemovesLockOnEveryOutcome(t *testing.T) {
	tests := []struct {
		name      string
		transform func([]byte) ([]byte, error)
		configure func(*artifactTransactionOps)
		wantErr   bool
	}{
		{
			name: "success",
			transform: func([]byte) ([]byte, error) {
				return []byte("after"), nil
			},
		},
		{
			name: "transform failure",
			transform: func([]byte) ([]byte, error) {
				return nil, errors.New("transform failed")
			},
			wantErr: true,
		},
		{
			name: "idempotent",
			transform: func(before []byte) ([]byte, error) {
				return before, nil
			},
		},
		{
			name: "write failure",
			transform: func([]byte) ([]byte, error) {
				return []byte("after"), nil
			},
			configure: func(ops *artifactTransactionOps) {
				ops.renameNoReplace = func(string, string) error { return errors.New("write failed") }
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			p := filepath.Join(dir, "task.md")
			if err := os.WriteFile(p, []byte("before"), 0o644); err != nil {
				t.Fatal(err)
			}
			ops := defaultArtifactTransactionOps()
			if tt.configure != nil {
				tt.configure(&ops)
			}

			err := transformArtifactWithOps(p, ops, tt.transform)
			if (err != nil) != tt.wantErr {
				t.Fatalf("error = %v, wantErr=%v", err, tt.wantErr)
			}
			lockPath := filepath.Join(dir, ".task.md.lifecycle-transaction.lock")
			if _, statErr := os.Stat(lockPath); !os.IsNotExist(statErr) {
				t.Fatalf("artifact lock residue: stat=%v", statErr)
			}
		})
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

// TestAcquireArtifactLock_RecoversAfterHolderProcessIsKilled exercises the
// actually-interrupted case, not just a successful run: a real child process
// acquires the per-artifact lock and is then SIGKILLed (no defer, no
// cleanup code of any kind runs in it), simulating an agent process killed
// mid-transaction.
//
// The guarantee under test: acquireArtifactLock's mutual exclusion is a pure
// OS-level flock(2)/LockFileEx advisory lock, scoped to the holder's open
// file description. The kernel releases it unconditionally the instant that
// process's descriptors are closed — which happens on ANY process exit,
// including a SIGKILL that runs no Go code at all. So a later invocation on
// the same artifact must be able to re-acquire the lock and proceed with no
// staleness heuristic in this package and no human deleting the lock file.
//
// The project lock serializes artifact-lock cleanup with the next acquisition,
// so a clean release removes the per-artifact pathname without reopening a
// delete-and-recreate race across concurrent CLI processes. This test only
// proves the lock stops blocking new work after its holder dies; a
// happy-path-only test could not distinguish "recovers automatically" from
// "would hang forever waiting for a lock nobody will ever release".
func TestAcquireArtifactLock_RecoversAfterHolderProcessIsKilled(t *testing.T) {
	if os.Getenv("SPECSCORE_TEST_LOCK_HOLDER") == "1" {
		runLockHolderHelperProcess()
		return
	}

	dir := t.TempDir()
	artifact := filepath.Join(dir, "widget.md")
	if err := os.WriteFile(artifact, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(os.Args[0], "-test.run=^TestAcquireArtifactLock_RecoversAfterHolderProcessIsKilled$")
	cmd.Env = append(os.Environ(),
		"SPECSCORE_TEST_LOCK_HOLDER=1",
		"SPECSCORE_TEST_LOCK_PATH="+artifact,
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatalf("stdout pipe: %v", err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("start lock-holder helper: %v", err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
	})

	ready := bufio.NewReader(stdout)
	line, err := ready.ReadString('\n')
	if err != nil || strings.TrimSpace(line) != "locked" {
		t.Fatalf("lock-holder helper did not report ready: line=%q err=%v stderr=%s", line, err, stderr.String())
	}

	// Contention: the artifact is genuinely locked while the holder is alive.
	if _, err := acquireArtifactLock(artifact); !errors.Is(err, ErrConcurrentMutation) {
		t.Fatalf("expected ErrConcurrentMutation while the holder is alive, got %v", err)
	}

	// Kill it exactly like an OOM-killed or force-stopped agent process: no
	// signal handler, no deferred lock.Unlock(), nothing.
	if err := cmd.Process.Kill(); err != nil {
		t.Fatalf("kill lock-holder helper: %v", err)
	}
	_ = cmd.Wait()

	// The interrupted-run guarantee: a subsequent run recovers on its own.
	lock, err := acquireArtifactLock(artifact)
	if err != nil {
		t.Fatalf("expected the lock to be acquirable after its holder was killed (no human cleanup should be required), got %v", err)
	}
	// The interrupted holder's lock file is exactly where old and new processes
	// both expect to contend on it while this recovery transaction is active.
	lockPath := filepath.Join(dir, ".widget.md.lifecycle-transaction.lock")
	if _, err := os.Stat(lockPath); err != nil {
		t.Fatalf("expected the lock file to exist while recovery holds it, got %v", err)
	}
	if err := lock.Unlock(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("expected recovered artifact lock to be removed after release, got %v", err)
	}
}

// runLockHolderHelperProcess is the child body for
// TestAcquireArtifactLock_RecoversAfterHolderProcessIsKilled. It is only
// entered by the separately executed test binary (guarded by
// SPECSCORE_TEST_LOCK_HOLDER) and never returns on its own: the parent
// SIGKILLs it once it has confirmed the lock is held.
func runLockHolderHelperProcess() {
	path := os.Getenv("SPECSCORE_TEST_LOCK_PATH")
	lock, err := acquireArtifactLock(path)
	if err != nil {
		fmt.Fprintln(os.Stderr, "lock-holder helper: acquire failed:", err)
		os.Exit(1)
	}
	_ = lock
	fmt.Println("locked")
	// Block without ever reaching a deferred Unlock — the parent kills this
	// process. time.Sleep (rather than an empty select{}) avoids tripping
	// the Go runtime's own all-goroutines-asleep deadlock detector.
	time.Sleep(time.Hour)
}
