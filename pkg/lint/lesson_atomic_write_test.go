package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/specscore/specscore-cli/pkg/lesson"
)

func TestWriteLintFileCoordinatesLessonArtifactsAndIndex(t *testing.T) {
	root := t.TempDir()
	specRoot := filepath.Join(root, "spec")
	if err := upsertLessonIndexRowUnlocked(specRoot, nil); err == nil {
		t.Fatal("unlocked nil Lesson index upsert was accepted")
	}
	lessonsDir := filepath.Join(specRoot, "lessons")
	for _, path := range []string{
		filepath.Join(lessonsDir, "README.md"),
		filepath.Join(lessonsDir, "canonical", "README.md"),
		filepath.Join(lessonsDir, "legacy.md"),
	} {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("before\n"), 0o640); err != nil {
			t.Fatal(err)
		}
		if err := writeLintFile(specRoot, path, []byte("before\n"), []byte("after\n"), 0o600); err != nil {
			t.Fatalf("rewrite %s: %v", path, err)
		}
		got, err := os.ReadFile(path)
		if err != nil || string(got) != "after\n" {
			t.Fatalf("rewrite %s = %q, %v", path, got, err)
		}
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm() != 0o640 {
			t.Fatalf("rewrite %s mode = %v, %v", path, info.Mode().Perm(), err)
		}
	}

	other := filepath.Join(specRoot, "research", "note.md")
	if err := os.MkdirAll(filepath.Dir(other), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeLintFile(specRoot, other, nil, []byte("new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(other); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("ordinary lint write mode = %v, %v", info.Mode().Perm(), err)
	}
}

func TestWriteLintFileSharesLifecycleLock(t *testing.T) {
	root := t.TempDir()
	specRoot := filepath.Join(root, "spec")
	path := filepath.Join(specRoot, "lessons", "locked", "README.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("before\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	locked := make(chan struct{})
	release := make(chan struct{})
	holder := make(chan error, 1)
	go func() {
		holder <- lesson.WithMutationLock(root, "locked", func() error {
			close(locked)
			<-release
			return os.WriteFile(path, []byte("lifecycle\n"), 0o644)
		})
	}()
	<-locked
	writer := make(chan error, 1)
	go func() { writer <- writeLintFile(specRoot, path, []byte("before\n"), []byte("after\n"), 0o644) }()
	select {
	case err := <-writer:
		t.Fatalf("lint writer escaped lifecycle lock: %v", err)
	case <-time.After(100 * time.Millisecond):
	}
	close(release)
	if err := <-holder; err != nil {
		t.Fatal(err)
	}
	if err := <-writer; err == nil || !strings.Contains(err.Error(), "stale Lesson rewrite") {
		t.Fatalf("stale lint rewrite = %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "lifecycle\n" {
		t.Fatalf("lifecycle bytes after stale lint writer = %q, %v", got, err)
	}
}
