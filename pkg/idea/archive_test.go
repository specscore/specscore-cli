package idea

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// readArchivedIdea returns the file contents at spec/ideas/archived/<slug>.md.
func readArchivedIdea(t *testing.T, root, slug string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "spec", "ideas", "archived", slug+".md"))
	if err != nil {
		t.Fatalf("read archived idea: %v", err)
	}
	return string(b)
}

// AC: archive-happy-path — `idea archive` sets **Archived:** true (keeping
// the terminal **Status:**) and relocates the file to archived/.
func TestArchive_HappyPath(t *testing.T) {
	root := stageIdeaTree(t, "foo", "Rejected")

	result, err := Archive(ArchiveOptions{
		SpecRoot:     root,
		Slug:         "foo",
		PostMutation: noopLint,
	})
	if err != nil {
		t.Fatalf("Archive: %v", err)
	}
	if result.Slug != "foo" {
		t.Errorf("result.Slug = %q; want foo", result.Slug)
	}
	// Active file MUST be gone.
	if _, err := os.Stat(filepath.Join(root, "spec", "ideas", "foo.md")); !os.IsNotExist(err) {
		t.Errorf("active file should not exist after archive: err=%v", err)
	}
	// Archived file MUST exist, keep its terminal status, and carry the flag.
	body := readArchivedIdea(t, root, "foo")
	if !strings.Contains(body, "**Status:** Rejected") {
		t.Errorf("archived file must keep terminal status:\n%s", body)
	}
	if !strings.Contains(body, "**Archived:** true") {
		t.Errorf("archived file missing **Archived:** true:\n%s", body)
	}
}

// The optional note is written as **Archive Note:** when supplied.
func TestArchive_WithNote(t *testing.T) {
	root := stageIdeaTree(t, "foo", "Stale")

	if _, err := Archive(ArchiveOptions{
		SpecRoot:     root,
		Slug:         "foo",
		Note:         "abandoned after the v2 pivot",
		PostMutation: noopLint,
	}); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	body := readArchivedIdea(t, root, "foo")
	if !strings.Contains(body, "**Archive Note:** abandoned after the v2 pivot") {
		t.Errorf("archived file missing **Archive Note:**:\n%s", body)
	}
}

// First archive into a project without spec/ideas/archived/ materializes the
// lint-clean index stub and the directory.
func TestArchive_CreatesArchivedIndexStub(t *testing.T) {
	root := t.TempDir()
	ideasDir := filepath.Join(root, "spec", "ideas")
	if err := os.MkdirAll(ideasDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body, err := Scaffold(ScaffoldOptions{Slug: "foo", Status: "Stale"})
	if err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ideasDir, "foo.md"), body, 0o644); err != nil {
		t.Fatalf("write idea: %v", err)
	}

	if _, err := Archive(ArchiveOptions{
		SpecRoot:     root,
		Slug:         "foo",
		PostMutation: noopLint,
	}); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	stub := filepath.Join(ideasDir, "archived", "README.md")
	if _, err := os.Stat(stub); err != nil {
		t.Errorf("archived index stub not created: %v", err)
	}
}

// AC: archive-collision — a stale archived file aborts the move (exit 1)
// and leaves the active file untouched.
func TestArchive_Collision(t *testing.T) {
	root := stageIdeaTree(t, "foo", "Stale")
	stalePath := filepath.Join(root, "spec", "ideas", "archived", "foo.md")
	staleBody := "# Stale archived idea — must remain untouched.\n"
	if err := os.WriteFile(stalePath, []byte(staleBody), 0o644); err != nil {
		t.Fatalf("write stale: %v", err)
	}

	_, err := Archive(ArchiveOptions{
		SpecRoot:     root,
		Slug:         "foo",
		PostMutation: noopLint,
	})
	assertExitCode(t, err, exitcode.Conflict)
	if !strings.Contains(err.Error(), stalePath) {
		t.Errorf("error message missing collision target %q: %v", stalePath, err)
	}
	// Active file MUST still exist (no move occurred).
	if _, err := os.Stat(filepath.Join(root, "spec", "ideas", "foo.md")); err != nil {
		t.Errorf("active file should remain after collision: %v", err)
	}
	// Stale archived file MUST be untouched.
	if got := readArchivedIdea(t, root, "foo"); got != staleBody {
		t.Errorf("stale archived file mutated; got:\n%s", got)
	}
}

// AC: archive-lint-failure-rolls-back — a post-move lint failure restores the
// file at the active path and leaves nothing in archived/.
func TestArchive_LintFailureRollsBack(t *testing.T) {
	root := stageIdeaTree(t, "foo", "Stale")
	before := readIdea(t, root, "foo")
	archivedPath := filepath.Join(root, "spec", "ideas", "archived", "foo.md")

	_, err := Archive(ArchiveOptions{
		SpecRoot:     root,
		Slug:         "foo",
		PostMutation: failingLint(exitcode.UnexpectedErrorf("lint boom")),
	})
	assertExitCode(t, err, exitcode.Unexpected)

	after := readIdea(t, root, "foo")
	if after != before {
		t.Errorf("rollback not byte-identical.\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if _, statErr := os.Stat(archivedPath); !os.IsNotExist(statErr) {
		t.Errorf("archived file should not exist after rollback: err=%v", statErr)
	}
}

// AC: archive-slug-not-found — a missing active file exits 3.
func TestArchive_SlugNotFound(t *testing.T) {
	root := stageIdeaTree(t, "foo", "Stale")
	_, err := Archive(ArchiveOptions{
		SpecRoot:     root,
		Slug:         "nonexistent",
		PostMutation: noopLint,
	})
	assertExitCode(t, err, exitcode.NotFound)
}

// stageIdeaNoArchivedDir writes a single Idea file but does NOT create
// spec/ideas/archived/, so the first Archive materializes the index stub
// (stubCreated == true) — exercising the removeStub cleanup on error paths.
func stageIdeaNoArchivedDir(t *testing.T, slug, status string) string {
	t.Helper()
	root := t.TempDir()
	ideasDir := filepath.Join(root, "spec", "ideas")
	if err := os.MkdirAll(ideasDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	body, err := Scaffold(ScaffoldOptions{Slug: slug, Status: status})
	if err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ideasDir, slug+".md"), body, 0o644); err != nil {
		t.Fatalf("write idea: %v", err)
	}
	return root
}

// Injected non-ENOENT stat error on the collision check surfaces exit 10.
// Staged WITHOUT a pre-existing archived/ dir so the stub is created first,
// exercising the removeStub cleanup on this error path.
func TestArchive_StatNonENOENT_Injected(t *testing.T) {
	root := stageIdeaNoArchivedDir(t, "foo", "Stale")

	old := osStatFn
	osStatFn = func(name string) (os.FileInfo, error) {
		if strings.HasSuffix(filepath.ToSlash(name), "archived/foo.md") {
			return nil, fmt.Errorf("injected stat error: not ENOENT")
		}
		return os.Stat(name)
	}
	t.Cleanup(func() { osStatFn = old })

	_, err := Archive(ArchiveOptions{
		SpecRoot:     root,
		Slug:         "foo",
		PostMutation: noopLint,
	})
	assertExitCode(t, err, exitcode.Unexpected)
	if !strings.Contains(err.Error(), "stat archive target") {
		t.Errorf("expected 'stat archive target' in error, got: %v", err)
	}
	// The stub README must have been cleaned up (we created it).
	if _, serr := os.Stat(filepath.Join(root, "spec", "ideas", "archived", "README.md")); !os.IsNotExist(serr) {
		t.Errorf("stub README should have been removed on error: %v", serr)
	}
}

// A non-ENOENT read error on the active path surfaces exit 10. Achieved by
// making the active file a directory entry that ReadFile cannot read as a
// file (a directory at the path).
func TestArchive_ReadActiveNonENOENT(t *testing.T) {
	root := t.TempDir()
	ideasDir := filepath.Join(root, "spec", "ideas")
	if err := os.MkdirAll(filepath.Join(ideasDir, "foo.md"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := Archive(ArchiveOptions{SpecRoot: root, Slug: "foo", PostMutation: noopLint})
	assertExitCode(t, err, exitcode.Unexpected)
	if !strings.Contains(err.Error(), "reading") {
		t.Errorf("expected 'reading' in error, got: %v", err)
	}
}

// A failure removing the active file after the archived copy is written
// surfaces exit 10 and removes the just-written archived copy. Achieved by
// making the ideas/ dir read-only so os.Remove(active) fails.
func TestArchive_RemoveActiveError(t *testing.T) {
	root := stageIdeaTree(t, "foo", "Stale")
	ideasDir := filepath.Join(root, "spec", "ideas")
	if err := os.Chmod(ideasDir, 0o555); err != nil {
		t.Skip("cannot change permissions")
	}
	t.Cleanup(func() { _ = os.Chmod(ideasDir, 0o755) })

	_, err := Archive(ArchiveOptions{SpecRoot: root, Slug: "foo", PostMutation: noopLint})
	if err == nil {
		t.Skip("remove did not fail on this platform")
	}
	assertExitCode(t, err, exitcode.Unexpected)
}

// Validation guards.
func TestArchive_Guards(t *testing.T) {
	if _, err := Archive(ArchiveOptions{Slug: "x", PostMutation: noopLint}); err == nil {
		t.Error("expected error for missing SpecRoot")
	}
	if _, err := Archive(ArchiveOptions{SpecRoot: "/r", PostMutation: noopLint}); err == nil {
		t.Error("expected error for missing Slug")
	}
	if _, err := Archive(ArchiveOptions{SpecRoot: "/r", Slug: "x"}); err == nil {
		t.Error("expected error for nil PostMutation")
	}
}

// AC: unarchive-happy-path — `idea unarchive` clears the **Archived:** axis
// and relocates back to the active path, keeping **Status:**.
func TestUnarchive_HappyPath(t *testing.T) {
	root := stageIdeaTree(t, "foo", "Stale")
	// First archive it (with a note, so unarchive must drop both lines).
	if _, err := Archive(ArchiveOptions{
		SpecRoot:     root,
		Slug:         "foo",
		Note:         "parked",
		PostMutation: noopLint,
	}); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	result, err := Unarchive(UnarchiveOptions{
		SpecRoot:     root,
		Slug:         "foo",
		PostMutation: noopLint,
	})
	if err != nil {
		t.Fatalf("Unarchive: %v", err)
	}
	if result.Slug != "foo" {
		t.Errorf("result.Slug = %q; want foo", result.Slug)
	}
	// Archived file MUST be gone.
	if _, err := os.Stat(filepath.Join(root, "spec", "ideas", "archived", "foo.md")); !os.IsNotExist(err) {
		t.Errorf("archived file should not exist after unarchive: err=%v", err)
	}
	// Active file back, status preserved, flag + note cleared.
	body := readIdea(t, root, "foo")
	if !strings.Contains(body, "**Status:** Stale") {
		t.Errorf("active file must keep status:\n%s", body)
	}
	if strings.Contains(body, "**Archived:**") {
		t.Errorf("**Archived:** line should be removed:\n%s", body)
	}
	if strings.Contains(body, "**Archive Note:**") {
		t.Errorf("**Archive Note:** line should be removed:\n%s", body)
	}
}

// AC: unarchive-collision — a pre-existing active file aborts (exit 1).
func TestUnarchive_Collision(t *testing.T) {
	root := stageIdeaTree(t, "foo", "Stale")
	if _, err := Archive(ArchiveOptions{
		SpecRoot:     root,
		Slug:         "foo",
		PostMutation: noopLint,
	}); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	// Recreate an active file at the same slug.
	body, _ := Scaffold(ScaffoldOptions{Slug: "foo", Status: "Draft"})
	if err := os.WriteFile(filepath.Join(root, "spec", "ideas", "foo.md"), body, 0o644); err != nil {
		t.Fatalf("write active: %v", err)
	}

	_, err := Unarchive(UnarchiveOptions{
		SpecRoot:     root,
		Slug:         "foo",
		PostMutation: noopLint,
	})
	assertExitCode(t, err, exitcode.Conflict)
	// Archived file MUST still exist.
	if _, err := os.Stat(filepath.Join(root, "spec", "ideas", "archived", "foo.md")); err != nil {
		t.Errorf("archived file should remain after collision: %v", err)
	}
}

// AC: unarchive-slug-not-found — missing archived file exits 3.
func TestUnarchive_SlugNotFound(t *testing.T) {
	root := stageIdeaTree(t, "foo", "Stale")
	_, err := Unarchive(UnarchiveOptions{
		SpecRoot:     root,
		Slug:         "nonexistent",
		PostMutation: noopLint,
	})
	assertExitCode(t, err, exitcode.NotFound)
}

// AC: unarchive-lint-failure-rolls-back — a post-move lint failure restores
// the file at the archived path.
func TestUnarchive_LintFailureRollsBack(t *testing.T) {
	root := stageIdeaTree(t, "foo", "Stale")
	if _, err := Archive(ArchiveOptions{
		SpecRoot:     root,
		Slug:         "foo",
		PostMutation: noopLint,
	}); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	before := readArchivedIdea(t, root, "foo")

	_, err := Unarchive(UnarchiveOptions{
		SpecRoot:     root,
		Slug:         "foo",
		PostMutation: failingLint(exitcode.UnexpectedErrorf("lint boom")),
	})
	assertExitCode(t, err, exitcode.Unexpected)

	after := readArchivedIdea(t, root, "foo")
	if after != before {
		t.Errorf("rollback not byte-identical.\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if _, statErr := os.Stat(filepath.Join(root, "spec", "ideas", "foo.md")); !os.IsNotExist(statErr) {
		t.Errorf("active file should not exist after rollback: err=%v", statErr)
	}
}

// Injected non-ENOENT stat error on the unarchive collision check → exit 10.
func TestUnarchive_StatNonENOENT_Injected(t *testing.T) {
	root := stageIdeaTree(t, "foo", "Stale")
	if _, err := Archive(ArchiveOptions{
		SpecRoot:     root,
		Slug:         "foo",
		PostMutation: noopLint,
	}); err != nil {
		t.Fatalf("Archive: %v", err)
	}

	old := osStatFn
	osStatFn = func(name string) (os.FileInfo, error) {
		if strings.HasSuffix(filepath.ToSlash(name), "ideas/foo.md") {
			return nil, fmt.Errorf("injected stat error: not ENOENT")
		}
		return os.Stat(name)
	}
	t.Cleanup(func() { osStatFn = old })

	_, err := Unarchive(UnarchiveOptions{
		SpecRoot:     root,
		Slug:         "foo",
		PostMutation: noopLint,
	})
	assertExitCode(t, err, exitcode.Unexpected)
	if !strings.Contains(err.Error(), "stat unarchive target") {
		t.Errorf("expected 'stat unarchive target' in error, got: %v", err)
	}
}

// A non-ENOENT read error on the archived path surfaces exit 10 (archived
// path is a directory, not a readable file).
func TestUnarchive_ReadArchivedNonENOENT(t *testing.T) {
	root := t.TempDir()
	archivedDir := filepath.Join(root, "spec", "ideas", "archived")
	if err := os.MkdirAll(filepath.Join(archivedDir, "foo.md"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := Unarchive(UnarchiveOptions{SpecRoot: root, Slug: "foo", PostMutation: noopLint})
	assertExitCode(t, err, exitcode.Unexpected)
	if !strings.Contains(err.Error(), "reading") {
		t.Errorf("expected 'reading' in error, got: %v", err)
	}
}

// A failure removing the archived file after the active copy is written
// surfaces exit 10. Achieved by making the archived/ dir read-only so
// os.Remove(archived) fails (the active write target is the parent ideas/
// dir, which stays writable).
func TestUnarchive_RemoveArchivedError(t *testing.T) {
	root := stageIdeaTree(t, "foo", "Stale")
	if _, err := Archive(ArchiveOptions{SpecRoot: root, Slug: "foo", PostMutation: noopLint}); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	archivedDir := filepath.Join(root, "spec", "ideas", "archived")
	if err := os.Chmod(archivedDir, 0o555); err != nil {
		t.Skip("cannot change permissions")
	}
	t.Cleanup(func() { _ = os.Chmod(archivedDir, 0o755) })

	_, err := Unarchive(UnarchiveOptions{SpecRoot: root, Slug: "foo", PostMutation: noopLint})
	if err == nil {
		t.Skip("remove did not fail on this platform")
	}
	assertExitCode(t, err, exitcode.Unexpected)
}

// A failure writing the active file surfaces exit 10. The archived file lives
// at spec/ideas/archived/foo.md; making the ideas/ dir read-only makes the
// write of ideas/foo.md fail while leaving archived/ readable.
func TestUnarchive_WriteActiveError(t *testing.T) {
	root := stageIdeaTree(t, "foo", "Stale")
	if _, err := Archive(ArchiveOptions{SpecRoot: root, Slug: "foo", PostMutation: noopLint}); err != nil {
		t.Fatalf("Archive: %v", err)
	}
	ideasDir := filepath.Join(root, "spec", "ideas")
	if err := os.Chmod(ideasDir, 0o555); err != nil {
		t.Skip("cannot change permissions")
	}
	t.Cleanup(func() { _ = os.Chmod(ideasDir, 0o755) })

	_, err := Unarchive(UnarchiveOptions{SpecRoot: root, Slug: "foo", PostMutation: noopLint})
	if err == nil {
		t.Skip("write did not fail on this platform")
	}
	assertExitCode(t, err, exitcode.Unexpected)
	if !strings.Contains(err.Error(), "writing") {
		t.Errorf("expected 'writing' in error, got: %v", err)
	}
}

// EnsureArchivedIndexStub: a WriteFile failure for the stub README (dir
// exists but is read-only) surfaces exit 10.
func TestEnsureArchivedIndexStub_WriteError(t *testing.T) {
	root := t.TempDir()
	archivedDir := filepath.Join(root, "spec", "ideas", "archived")
	if err := os.MkdirAll(archivedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// No README; make the dir read-only so the stub WriteFile fails.
	if err := os.Chmod(archivedDir, 0o555); err != nil {
		t.Skip("cannot change permissions")
	}
	t.Cleanup(func() { _ = os.Chmod(archivedDir, 0o755) })

	_, err := EnsureArchivedIndexStub(root)
	if err == nil {
		t.Skip("write did not fail on this platform")
	}
	assertExitCode(t, err, exitcode.Unexpected)
}

// Validation guards.
func TestUnarchive_Guards(t *testing.T) {
	if _, err := Unarchive(UnarchiveOptions{Slug: "x", PostMutation: noopLint}); err == nil {
		t.Error("expected error for missing SpecRoot")
	}
	if _, err := Unarchive(UnarchiveOptions{SpecRoot: "/r", PostMutation: noopLint}); err == nil {
		t.Error("expected error for missing Slug")
	}
	if _, err := Unarchive(UnarchiveOptions{SpecRoot: "/r", Slug: "x"}); err == nil {
		t.Error("expected error for nil PostMutation")
	}
}

// Archive surfaces the archived-index-stub mkdir failure (a file blocking the
// archived/ directory) as exit 10.
func TestArchive_StubMkdirError(t *testing.T) {
	root := t.TempDir()
	ideasDir := filepath.Join(root, "spec", "ideas")
	if err := os.MkdirAll(ideasDir, 0o755); err != nil {
		t.Fatal(err)
	}
	body, _ := Scaffold(ScaffoldOptions{Slug: "mkdir-err", Status: "Stale"})
	if err := os.WriteFile(filepath.Join(ideasDir, "mkdir-err.md"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	// Place a file where the archived/ directory should be → MkdirAll fails.
	if err := os.WriteFile(filepath.Join(ideasDir, "archived"), []byte("file"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := Archive(ArchiveOptions{SpecRoot: root, Slug: "mkdir-err", PostMutation: noopLint})
	assertExitCode(t, err, exitcode.Unexpected)
	// Active file must remain untouched (failure before any move).
	if _, serr := os.Stat(filepath.Join(ideasDir, "mkdir-err.md")); serr != nil {
		t.Errorf("active file should remain after stub-mkdir failure: %v", serr)
	}
}

// Archive surfaces a WriteFile failure for the archived idea (archived dir is
// read-only after the stub is created) as exit 10, leaving the active file.
func TestArchive_WriteError(t *testing.T) {
	root := t.TempDir()
	ideasDir := filepath.Join(root, "spec", "ideas")
	archivedDir := filepath.Join(ideasDir, "archived")
	if err := os.MkdirAll(archivedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Pre-create the stub README so EnsureArchivedIndexStub is a no-op, then
	// make the dir read-only so the archived-idea WriteFile fails.
	if err := os.WriteFile(filepath.Join(archivedDir, "README.md"), []byte("# Archived\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	body, _ := Scaffold(ScaffoldOptions{Slug: "write-err", Status: "Stale"})
	if err := os.WriteFile(filepath.Join(ideasDir, "write-err.md"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(archivedDir, 0o555); err != nil {
		t.Skip("cannot change permissions")
	}
	t.Cleanup(func() { _ = os.Chmod(archivedDir, 0o755) })

	_, err := Archive(ArchiveOptions{SpecRoot: root, Slug: "write-err", PostMutation: noopLint})
	assertExitCode(t, err, exitcode.Unexpected)
	if _, serr := os.Stat(filepath.Join(ideasDir, "write-err.md")); serr != nil {
		t.Errorf("active file should remain after write failure: %v", serr)
	}
}

// EnsureArchivedIndexStub: a non-ENOENT stat error on the README path is
// surfaced as exit 10. Exercised via a read-only archived/ dir so the README
// stat fails with permission denied.
func TestEnsureArchivedIndexStub_StatError(t *testing.T) {
	root := t.TempDir()
	archivedDir := filepath.Join(root, "spec", "ideas", "archived")
	if err := os.MkdirAll(archivedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// No README present; make the dir unsearchable so os.Stat(README) returns
	// a non-ENOENT error.
	if err := os.Chmod(archivedDir, 0o000); err != nil {
		t.Skip("cannot change permissions")
	}
	t.Cleanup(func() { _ = os.Chmod(archivedDir, 0o755) })

	_, err := EnsureArchivedIndexStub(root)
	if err == nil {
		// Some platforms (e.g. running as root) ignore the perm bits.
		return
	}
	assertExitCode(t, err, exitcode.Unexpected)
}

// setArchivedHeader edge cases: a malformed file with no **Status:** line
// prepends the flag; archived=false on content without the lines is a no-op.
func TestSetArchivedHeader_EdgeCases(t *testing.T) {
	// No Status line: flag prepended.
	got := setArchivedHeader([]byte("# Idea: X\n\n## Body\n"), true, "")
	if !strings.HasPrefix(string(got), "**Archived:** true\n") {
		t.Errorf("expected flag prepended when no Status line:\n%s", got)
	}
	// archived=false with no existing flag: content unchanged.
	in := "# Idea: X\n\n**Status:** Draft\n\n## Body\n"
	if out := string(setArchivedHeader([]byte(in), false, "")); out != in {
		t.Errorf("clearing absent flag should be a no-op.\nin:\n%s\nout:\n%s", in, out)
	}
	// No trailing newline preserved.
	noNL := "# Idea: X\n\n**Status:** Draft"
	if out := string(setArchivedHeader([]byte(noNL), true, "")); strings.HasSuffix(out, "\n") {
		t.Errorf("trailing-newline shape not preserved:\n%q", out)
	}
}
