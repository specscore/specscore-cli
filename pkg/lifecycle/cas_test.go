package lifecycle

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

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
