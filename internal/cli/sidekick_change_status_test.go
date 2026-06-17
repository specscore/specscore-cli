package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/lint"
)

// stageQueuedSeed stages a lint-clean temp spec repo carrying a single Queued
// sidekick-seed at spec/ideas/seeds/<slug>.md, with CWD set to the repo root.
// It mirrors stageActiveIdea: bootstrap via the production `sidekick new` verb,
// hand-write a minimal Features index, then migrate + lint --fix so the whole
// tree is clean before the verb under test runs.
func stageQueuedSeed(t *testing.T, slug string) string {
	t.Helper()
	root := setupSpecRoot(t)
	withCwd(t, root)

	if _, _, err := runSidekick(t, "new", "Seed "+slug, "--slug", slug); err != nil {
		t.Fatalf("sidekick new: %v", err)
	}
	featuresReadme := "# Features\n\n## Index\n\n| Feature | Status |\n|---------|--------|\n\n_No features yet._\n\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(
		filepath.Join(root, "spec", "features", "README.md"),
		[]byte(featuresReadme), 0o644); err != nil {
		t.Fatalf("write features README: %v", err)
	}
	migrateTree(t, root)
	if _, err := lint.Lint(lint.Options{SpecRoot: filepath.Join(root, "spec"), Fix: true}); err != nil {
		t.Fatalf("initial lint --fix: %v", err)
	}
	vs, err := lint.Lint(lint.Options{SpecRoot: filepath.Join(root, "spec")})
	if err != nil {
		t.Fatalf("verify lint: %v", err)
	}
	for _, v := range vs {
		if v.Severity == "error" {
			t.Fatalf("pre-existing lint error in fixture: %s:%d [%s] %s", v.File, v.Line, v.Rule, v.Message)
		}
	}
	return root
}

func seedPath(root, slug string) string {
	return filepath.Join(root, "spec", "ideas", "seeds", slug+".md")
}

func archivedPath(root, slug string) string {
	return filepath.Join(root, "spec", "ideas", "archived", slug+".md")
}

func readFile(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatalf("reading %s: %v", p, err)
	}
	return string(b)
}

func lintHasNewErrors(t *testing.T, root string) bool {
	t.Helper()
	vs, err := lint.Lint(lint.Options{SpecRoot: filepath.Join(root, "spec")})
	if err != nil {
		t.Fatalf("lint: %v", err)
	}
	for _, v := range vs {
		if v.Severity == "error" {
			t.Logf("lint error: %s:%d [%s] %s", v.File, v.Line, v.Rule, v.Message)
			return true
		}
	}
	return false
}

// AC: implemented-relocates-and-tags
func TestSidekickChangeStatus_ImplementedRelocatesAndTags(t *testing.T) {
	root := stageQueuedSeed(t, "foo")

	out, _, err := runSidekick(t, "change-status", "foo", "--to=implemented")
	if err != nil {
		t.Fatalf("change-status: %v", err)
	}
	if out != "foo: Queued → Implemented\n" {
		t.Errorf("stdout = %q, want %q", out, "foo: Queued → Implemented\n")
	}
	if _, statErr := os.Stat(seedPath(root, "foo")); statErr == nil {
		t.Error("seed should no longer exist at seeds/foo.md")
	}
	archived := readFile(t, archivedPath(root, "foo"))
	if !strings.Contains(archived, "status: Implemented\n") {
		t.Errorf("archived seed missing status: Implemented:\n%s", archived)
	}
	if !strings.Contains(archived, "type: sidekick-seed\n") {
		t.Errorf("archived seed missing type: sidekick-seed:\n%s", archived)
	}
	if lintHasNewErrors(t, root) {
		t.Error("spec lint reported new violations after change-status")
	}
}

// AC: rejected-with-note-writes-resolution
func TestSidekickChangeStatus_RejectedWithNoteWritesResolution(t *testing.T) {
	root := stageQueuedSeed(t, "bar")

	out, _, err := runSidekick(t, "change-status", "bar", "--to=rejected",
		"--note", "Superseded by the consolidated close skill")
	if err != nil {
		t.Fatalf("change-status: %v", err)
	}
	if out != "bar: Queued → Rejected\n" {
		t.Errorf("stdout = %q, want %q", out, "bar: Queued → Rejected\n")
	}
	if _, statErr := os.Stat(seedPath(root, "bar")); statErr == nil {
		t.Error("seed should no longer exist at seeds/bar.md")
	}
	archived := readFile(t, archivedPath(root, "bar"))
	if !strings.Contains(archived, "status: Rejected\n") {
		t.Errorf("archived seed missing status: Rejected:\n%s", archived)
	}
	if !strings.Contains(archived, "type: sidekick-seed\n") {
		t.Errorf("archived seed missing type: sidekick-seed:\n%s", archived)
	}
	if !strings.Contains(archived, "## Resolution") {
		t.Errorf("archived seed missing ## Resolution section:\n%s", archived)
	}
	if !strings.Contains(archived, "Superseded by the consolidated close skill") {
		t.Errorf("archived seed missing note text:\n%s", archived)
	}
}

// REQ: reason-required-rejected — exit 2 when --to=rejected has no note,
// before any mutation.
func TestSidekickChangeStatus_RejectedWithoutNote_Exit2(t *testing.T) {
	root := stageQueuedSeed(t, "bar")
	before := readFile(t, seedPath(root, "bar"))

	_, _, err := runSidekick(t, "change-status", "bar", "--to=rejected")
	if got := exitCodeOf(err); got != 2 {
		t.Errorf("exit = %d, want 2", got)
	}
	if after := readFile(t, seedPath(root, "bar")); after != before {
		t.Error("seed was mutated despite reason-required rejection")
	}
	if _, statErr := os.Stat(archivedPath(root, "bar")); statErr == nil {
		t.Error("no archived file should be created")
	}
}

// REQ: target-status-flag — an unrecognized --to value exits 2 before mutation.
func TestSidekickChangeStatus_UnrecognizedTarget_Exit2(t *testing.T) {
	root := stageQueuedSeed(t, "foo")
	before := readFile(t, seedPath(root, "foo"))

	_, _, err := runSidekick(t, "change-status", "foo", "--to=stable")
	if got := exitCodeOf(err); got != 2 {
		t.Errorf("exit = %d, want 2", got)
	}
	if after := readFile(t, seedPath(root, "foo")); after != before {
		t.Error("seed was mutated on unrecognized --to")
	}
}

// REQ: seed-slug-resolution — a missing seed exits 3 naming the expected path.
func TestSidekickChangeStatus_SlugNotFound_Exit3(t *testing.T) {
	stageQueuedSeed(t, "foo") // establishes a valid repo + CWD

	_, errOut, err := runSidekick(t, "change-status", "nope", "--to=implemented")
	if got := exitCodeOf(err); got != 3 {
		t.Errorf("exit = %d, want 3", got)
	}
	if !strings.Contains(err.Error()+errOut, filepath.Join("spec", "ideas", "seeds", "nope.md")) {
		t.Errorf("error should name expected path; got %v / %q", err, errOut)
	}
}

// AC: non-queued-source-rejected — a non-Queued source exits 4, seed unchanged.
func TestSidekickChangeStatus_NonQueuedSource_Exit4(t *testing.T) {
	root := stageQueuedSeed(t, "qux")
	// Flip the frontmatter status to a non-Queued value.
	p := seedPath(root, "qux")
	raw := readFile(t, p)
	patched := strings.Replace(raw, "status: queued", "status: Implemented", 1)
	if patched == raw {
		t.Fatal("fixture did not contain status: queued")
	}
	if err := os.WriteFile(p, []byte(patched), 0o644); err != nil {
		t.Fatalf("patch seed: %v", err)
	}

	_, _, err := runSidekick(t, "change-status", "qux", "--to=implemented")
	if got := exitCodeOf(err); got != 4 {
		t.Errorf("exit = %d, want 4", got)
	}
	if after := readFile(t, p); after != patched {
		t.Error("seed was mutated despite non-Queued source")
	}
	if _, statErr := os.Stat(archivedPath(root, "qux")); statErr == nil {
		t.Error("no archived file should be created")
	}
}

// AC: archive-path-collision — exit 1, source seed (status + note) untouched.
func TestSidekickChangeStatus_ArchiveCollision_Exit1(t *testing.T) {
	root := stageQueuedSeed(t, "foo")
	// Pre-occupy the archive destination.
	if err := os.WriteFile(archivedPath(root, "foo"), []byte("# existing\n"), 0o644); err != nil {
		t.Fatalf("seed archive collision: %v", err)
	}
	before := readFile(t, seedPath(root, "foo"))
	archivedBefore := readFile(t, archivedPath(root, "foo"))

	_, _, err := runSidekick(t, "change-status", "foo", "--to=archived")
	if got := exitCodeOf(err); got != 1 {
		t.Errorf("exit = %d, want 1", got)
	}
	if after := readFile(t, seedPath(root, "foo")); after != before {
		t.Errorf("source seed must remain byte-identical on collision:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if !strings.Contains(before, "status: queued") {
		t.Error("source seed should still carry status: queued")
	}
	if archivedAfter := readFile(t, archivedPath(root, "foo")); archivedAfter != archivedBefore {
		t.Error("pre-existing archived file must be untouched")
	}
}

// AC: note-optional-on-implemented — with --note writes Resolution; without it
// writes none.
func TestSidekickChangeStatus_NoteOptionalOnImplemented(t *testing.T) {
	rootWith := stageQueuedSeed(t, "baz")
	if _, _, err := runSidekick(t, "change-status", "baz", "--to=implemented",
		"--note", "Shipped in skills/implement step 4b"); err != nil {
		t.Fatalf("change-status with note: %v", err)
	}
	withNote := readFile(t, archivedPath(rootWith, "baz"))
	if !strings.Contains(withNote, "## Resolution") ||
		!strings.Contains(withNote, "Shipped in skills/implement step 4b") {
		t.Errorf("expected ## Resolution with note text:\n%s", withNote)
	}

	rootNo := stageQueuedSeed(t, "baz2")
	if _, _, err := runSidekick(t, "change-status", "baz2", "--to=implemented"); err != nil {
		t.Fatalf("change-status without note: %v", err)
	}
	noNote := readFile(t, archivedPath(rootNo, "baz2"))
	if strings.Contains(noNote, "## Resolution") {
		t.Errorf("expected no ## Resolution section without --note:\n%s", noNote)
	}
}

// REQ: rollback-includes-relocation — a note appended before a collision must
// be undone so the seed is restored byte-identical.
func TestSidekickChangeStatus_CollisionWithNote_NoteRolledBack(t *testing.T) {
	root := stageQueuedSeed(t, "foo")
	if err := os.WriteFile(archivedPath(root, "foo"), []byte("# existing\n"), 0o644); err != nil {
		t.Fatalf("seed archive collision: %v", err)
	}
	before := readFile(t, seedPath(root, "foo"))

	_, _, err := runSidekick(t, "change-status", "foo", "--to=rejected",
		"--note", "should be rolled back")
	if got := exitCodeOf(err); got != 1 {
		t.Errorf("exit = %d, want 1", got)
	}
	after := readFile(t, seedPath(root, "foo"))
	if after != before {
		t.Errorf("seed must be byte-identical after collision rollback:\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if strings.Contains(after, "## Resolution") {
		t.Error("resolution note must be rolled back on collision")
	}
}
