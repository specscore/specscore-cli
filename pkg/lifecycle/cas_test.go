package lifecycle

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
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
	orig := osRename
	osRename = func(string, string) error { return errors.New("fenced") }
	t.Cleanup(func() { osRename = orig })
	if err := transformTo(p, []byte("after")); err == nil {
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
	orig := newArtifactLock
	newArtifactLock = func(string) artifactLock { return failingArtifactLock{err: errors.New("lock fault")} }
	t.Cleanup(func() { newArtifactLock = orig })
	if err := transformTo(p, []byte("after")); err == nil {
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

func TestTransformArtifact_ForeignEditBeforeRenameIsDetected(t *testing.T) {
	p := filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(p, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	orig := osReadBeforeRename
	osReadBeforeRename = func(path string) ([]byte, error) {
		if err := os.WriteFile(path, []byte("foreign"), 0o644); err != nil {
			return nil, err
		}
		return os.ReadFile(path)
	}
	t.Cleanup(func() { osReadBeforeRename = orig })
	if err := transformTo(p, []byte("after")); !errors.Is(err, ErrConcurrentMutation) {
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

func TestTransformArtifact_DirectoryFenceFailureIsCommittedRecoveryRequired(t *testing.T) {
	p := filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(p, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	boom := errors.New("directory open failed")
	orig := osOpenDir
	osOpenDir = func(string) (*os.File, error) { return nil, boom }
	t.Cleanup(func() { osOpenDir = orig })
	err := transformTo(p, []byte("after"))
	var committed *CommittedMutationError
	if !errors.As(err, &committed) || !errors.Is(err, boom) || committed.Phase != "opening artifact directory for durable sync" {
		t.Fatalf("err=%T %v", err, err)
	}
	if got, _ := os.ReadFile(p); string(got) != "after" {
		t.Fatalf("visible commit was lost: %q", got)
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

func TestRewrite_LockFaultLeavesArtifactUntouched(t *testing.T) {
	p := filepath.Join(t.TempDir(), "task.md")
	before := []byte("**Status:** Draft\n")
	if err := os.WriteFile(p, before, 0o644); err != nil {
		t.Fatal(err)
	}
	orig := newArtifactLock
	newArtifactLock = func(string) artifactLock { return failingArtifactLock{err: errors.New("lock fault")} }
	t.Cleanup(func() { newArtifactLock = orig })
	if _, err := Rewrite(p, IdeaApproved); err == nil {
		t.Fatal("expected lock fault")
	}
	if got, _ := os.ReadFile(p); !bytes.Equal(got, before) {
		t.Fatalf("lock fault wrote %q", got)
	}
}
