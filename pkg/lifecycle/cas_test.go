package lifecycle

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

type failingCASLock struct{ err error }

func (l failingCASLock) TryLock() (bool, error) { return false, l.err }
func (l failingCASLock) Unlock() error          { return nil }

func TestCompareAndSwap(t *testing.T) {
	p := filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(p, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CompareAndSwap(p, []byte("before"), []byte("after")); err != nil {
		t.Fatal(err)
	}
	if got, _ := os.ReadFile(p); string(got) != "after" {
		t.Fatalf("got %q", got)
	}
	if err := CompareAndSwap(p, []byte("before"), []byte("lost")); !errors.Is(err, ErrConcurrentMutation) {
		t.Fatalf("err=%v", err)
	}
}

func TestCompareAndSwap_WriteFailureLeavesOriginal(t *testing.T) {
	p := filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(p, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	orig := osRename
	osRename = func(string, string) error { return errors.New("fenced") }
	t.Cleanup(func() { osRename = orig })
	if err := CompareAndSwap(p, []byte("before"), []byte("after")); err == nil {
		t.Fatal("expected write failure")
	}
	if got, _ := os.ReadFile(p); string(got) != "before" {
		t.Fatalf("original lost: %q", got)
	}
}

func TestCompareAndSwap_ReadFailure(t *testing.T) {
	err := CompareAndSwap(filepath.Join(t.TempDir(), "missing.md"), []byte("before"), []byte("after"))
	if err == nil {
		t.Fatal("expected read failure")
	}
}

func TestCompareAndSwap_ConcurrentWriterIsFenced(t *testing.T) {
	p := filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(p, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	lock, err := acquireCASLock(p)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = lock.Unlock() }()
	if err := CompareAndSwap(p, []byte("before"), []byte("after")); !errors.Is(err, ErrConcurrentMutation) {
		t.Fatalf("err=%v", err)
	}
}

func TestCompareAndSwap_ReleasesLock(t *testing.T) {
	p := filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(p, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := CompareAndSwap(p, []byte("before"), []byte("after")); err != nil {
		t.Fatal(err)
	}
	lock, err := acquireCASLock(p)
	if err != nil {
		t.Fatalf("lock was not released: %v", err)
	}
	if err := lock.Unlock(); err != nil {
		t.Fatal(err)
	}
}

func TestCompareAndSwap_LockFaultLeavesOriginal(t *testing.T) {
	p := filepath.Join(t.TempDir(), "task.md")
	if err := os.WriteFile(p, []byte("before"), 0o644); err != nil {
		t.Fatal(err)
	}
	orig := newCASLock
	newCASLock = func(string) casLock { return failingCASLock{err: errors.New("lock fault")} }
	t.Cleanup(func() { newCASLock = orig })
	if err := CompareAndSwap(p, []byte("before"), []byte("after")); err == nil {
		t.Fatal("expected lock fault")
	}
	if got, _ := os.ReadFile(p); string(got) != "before" {
		t.Fatalf("lock fault wrote %q", got)
	}
}

func TestWithArtifactMutationLock_IsBoundedAndReleases(t *testing.T) {
	p := filepath.Join(t.TempDir(), "task.md")
	lock, err := acquireCASLock(p)
	if err != nil {
		t.Fatal(err)
	}
	if err := WithArtifactMutationLock(p, func() error { return nil }); !errors.Is(err, ErrConcurrentMutation) {
		t.Fatalf("contention=%v", err)
	}
	if err := lock.Unlock(); err != nil {
		t.Fatal(err)
	}
	if err := WithArtifactMutationLock(p, func() error { return nil }); err != nil {
		t.Fatalf("release=%v", err)
	}
}
