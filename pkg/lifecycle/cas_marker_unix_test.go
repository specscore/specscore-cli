//go:build unix

package lifecycle

import (
	"path/filepath"
	"syscall"
	"testing"
)

// A FIFO named .git must be rejected without being opened: opening it would
// block until a writer appears and hang every lifecycle mutation below it.
func TestIsGitMarker_NonRegularEntryIsNotAMarker(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".git")
	if err := syscall.Mkfifo(path, 0o644); err != nil {
		t.Fatal(err)
	}
	if isGitMarker(path) {
		t.Fatal("a FIFO .git must not count as a marker")
	}
}
