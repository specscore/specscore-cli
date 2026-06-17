package cli

// Coverage for runSidekickChangeStatus error/rollback branches not exercised by
// the happy-path + exit-code e2e tests: resolveSpecRoot failure, a seed with no
// frontmatter status, a note-write failure, and the note-rollback closure that
// fires when a post-move lint failure rolls back a note-carrying transition.

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/specscore/specscore-cli/pkg/lint"
)

func TestSidekickChangeStatus_ResolveSpecRootError(t *testing.T) {
	stageQueuedSeed(t, "foo")
	orig := osGetwdFn
	defer func() { osGetwdFn = orig }()
	osGetwdFn = func() (string, error) { return "", os.ErrPermission }
	if _, _, err := runSidekick(t, "change-status", "foo", "--to=implemented"); err == nil {
		t.Fatal("expected resolveSpecRoot error")
	}
}

func TestSidekickChangeStatus_ReadStatusError(t *testing.T) {
	root := stageQueuedSeed(t, "foo")
	// Strip the frontmatter status: line so ReadFrontmatterStatus fails (exit 10).
	if err := os.WriteFile(seedPath(root, "foo"),
		[]byte("---\ncaptured_by: user\n---\n# Seed foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := runSidekick(t, "change-status", "foo", "--to=implemented")
	if got := exitCodeOf(err); got != 10 {
		t.Errorf("exit = %d, want 10", got)
	}
}

func TestSidekickChangeStatus_NoteWriteError(t *testing.T) {
	root := stageQueuedSeed(t, "foo")
	seedsDir := filepath.Join(root, "spec", "ideas", "seeds")
	if err := os.Chmod(seedsDir, 0o500); err != nil { // read-only → note temp-write fails
		t.Fatal(err)
	}
	defer os.Chmod(seedsDir, 0o755)
	if _, _, err := runSidekick(t, "change-status", "foo", "--to=implemented", "--note", "shipped"); err == nil {
		t.Fatal("expected note-write error")
	}
}

func TestSidekickChangeStatus_NotePostMoveRollback(t *testing.T) {
	root := stageQueuedSeed(t, "foo")
	before := readFile(t, seedPath(root, "foo"))
	orig := lintLintFn
	defer func() { lintLintFn = orig }()
	lintLintFn = func(lint.Options) ([]lint.Violation, error) {
		return nil, os.ErrPermission // post-move lint --fix fails → Relocate rolls back
	}
	if _, _, err := runSidekick(t, "change-status", "foo", "--to=implemented", "--note", "shipped"); err == nil {
		t.Fatal("expected post-move rollback error")
	}
	// Seed restored byte-identical at seeds/ (note undone by the RollbackHook).
	if after := readFile(t, seedPath(root, "foo")); after != before {
		t.Errorf("seed not restored:\n got: %q\nwant: %q", after, before)
	}
}

func TestSidekickChangeStatus_PostMoveRollbackNoNote(t *testing.T) {
	// Post-move lint failure with NO --note: the RollbackHook closure runs its
	// noteWritten==false branch (return nil).
	root := stageQueuedSeed(t, "foo")
	before := readFile(t, seedPath(root, "foo"))
	orig := lintLintFn
	defer func() { lintLintFn = orig }()
	lintLintFn = func(lint.Options) ([]lint.Violation, error) {
		return nil, os.ErrPermission
	}
	if _, _, err := runSidekick(t, "change-status", "foo", "--to=implemented"); err == nil {
		t.Fatal("expected post-move rollback error")
	}
	if after := readFile(t, seedPath(root, "foo")); after != before {
		t.Errorf("seed not restored:\n got: %q\nwant: %q", after, before)
	}
}
