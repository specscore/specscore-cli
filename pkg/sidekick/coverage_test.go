package sidekick

// Coverage tests for the seed-kind change-status primitives: error and edge
// branches in seed.go (ResolveSeedPath / ReadFrontmatterStatus /
// RewriteFrontmatterStatus / line helpers) and relocate.go (Relocate rollback
// permutations / addSidekickSeedType / writeBack). I/O error branches that are
// unreachable via the filesystem alone use the package's statFn / mkdirAllFn /
// renameFn seams (mirroring pkg/idea's osStatFn convention).

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validSeed = "---\ncaptured_by: user\nstatus: queued\n---\n# A seed\n"

func writeSeedFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// --- ResolveSeedPath ---

func TestResolveSeedPath_EmptyArgs(t *testing.T) {
	if _, err := ResolveSeedPath("", "x"); err == nil {
		t.Fatal("expected error for empty specRoot")
	}
	if _, err := ResolveSeedPath(t.TempDir(), ""); err == nil {
		t.Fatal("expected error for empty slug")
	}
}

func TestResolveSeedPath_StatNonNotExist(t *testing.T) {
	root := t.TempDir()
	// Make the seeds path a FILE so stat of seeds/<slug>.md is ENOTDIR
	// (an error that is not os.IsNotExist).
	seeds := filepath.Join(root, "spec", "ideas", "seeds")
	if err := os.MkdirAll(filepath.Dir(seeds), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(seeds, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveSeedPath(root, "foo"); err == nil {
		t.Fatal("expected ENOTDIR-class error")
	}
}

// --- ReadFrontmatterStatus ---

func TestReadFrontmatterStatus(t *testing.T) {
	dir := t.TempDir()
	if _, err := ReadFrontmatterStatus(filepath.Join(dir, "nope.md")); err == nil {
		t.Fatal("expected ReadFile error")
	}

	noStatus := filepath.Join(dir, "no-status.md")
	writeSeedFile(t, noStatus, "---\ncaptured_by: user\n---\n# x\n")
	if _, err := ReadFrontmatterStatus(noStatus); err != ErrFrontmatterStatusNotFound {
		t.Fatalf("want ErrFrontmatterStatusNotFound, got %v", err)
	}

	ok := filepath.Join(dir, "ok.md")
	writeSeedFile(t, ok, validSeed)
	got, err := ReadFrontmatterStatus(ok)
	if err != nil || got != "queued" {
		t.Fatalf("want queued, got %q err %v", got, err)
	}
}

// --- RewriteFrontmatterStatus ---

func TestRewriteFrontmatterStatus_ReadError(t *testing.T) {
	if _, err := RewriteFrontmatterStatus(filepath.Join(t.TempDir(), "nope.md"), "Implemented"); err == nil {
		t.Fatal("expected ReadFile error")
	}
}

func TestRewriteFrontmatterStatus_StatSeamError(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "s.md")
	writeSeedFile(t, p, validSeed)
	defer func() { statFn = os.Stat }()
	statFn = func(string) (fs.FileInfo, error) { return nil, errors.New("stat boom") }
	if _, err := RewriteFrontmatterStatus(p, "Implemented"); err == nil {
		t.Fatal("expected stat-seam error")
	}
}

func TestRewriteFrontmatterStatus_WriteError(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "s.md")
	writeSeedFile(t, p, validSeed)
	if err := os.Chmod(p, 0o400); err != nil { // read-only → WriteFile fails
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(p, 0o644) }()
	if _, err := RewriteFrontmatterStatus(p, "Implemented"); err == nil {
		t.Fatal("expected WriteFile error on read-only file")
	}
}

func TestRewriteFrontmatterStatus_CRLFAndNoTrailingNewline(t *testing.T) {
	dir := t.TempDir()
	// CRLF terminators + no trailing newline on the last line.
	p := filepath.Join(dir, "crlf.md")
	writeSeedFile(t, p, "---\r\nstatus: queued\r\n---\r\n# title")
	orig, err := RewriteFrontmatterStatus(p, "Implemented")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(orig, "queued") {
		t.Fatalf("original line should carry old value, got %q", orig)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "status: Implemented\r\n") {
		t.Fatalf("CRLF not preserved: %q", string(b))
	}
}

func TestReadFrontmatterStatus_EmptyAndNoFenceAndClosingFence(t *testing.T) {
	dir := t.TempDir()
	empty := filepath.Join(dir, "empty.md")
	writeSeedFile(t, empty, "")
	if _, err := ReadFrontmatterStatus(empty); err != ErrFrontmatterStatusNotFound {
		t.Fatalf("empty: want ErrFrontmatterStatusNotFound, got %v", err)
	}
	noFence := filepath.Join(dir, "nofence.md")
	writeSeedFile(t, noFence, "# just a title\nbody\n")
	if _, err := ReadFrontmatterStatus(noFence); err != ErrFrontmatterStatusNotFound {
		t.Fatalf("nofence: want ErrFrontmatterStatusNotFound, got %v", err)
	}
	closed := filepath.Join(dir, "closed.md")
	writeSeedFile(t, closed, "---\ncaptured_by: user\n---\n# body\n")
	if _, err := ReadFrontmatterStatus(closed); err != ErrFrontmatterStatusNotFound {
		t.Fatalf("closed-fence: want ErrFrontmatterStatusNotFound, got %v", err)
	}
}

// --- addSidekickSeedType / frontmatterKeyOf / writeBack (direct) ---

func TestAddSidekickSeedType_Branches(t *testing.T) {
	dir := t.TempDir()

	// Empty file → no frontmatter.
	empty := filepath.Join(dir, "empty.md")
	writeSeedFile(t, empty, "")
	if err := addSidekickSeedType(empty); err != ErrFrontmatterStatusNotFound {
		t.Fatalf("empty: want ErrFrontmatterStatusNotFound, got %v", err)
	}

	// No opening fence.
	noFence := filepath.Join(dir, "nofence.md")
	writeSeedFile(t, noFence, "# title\n")
	if err := addSidekickSeedType(noFence); err != ErrFrontmatterStatusNotFound {
		t.Fatalf("nofence: want ErrFrontmatterStatusNotFound, got %v", err)
	}

	// Existing type: key is rewritten in place; a no-colon line is skipped.
	existing := filepath.Join(dir, "existing.md")
	writeSeedFile(t, existing, "---\nnocolon line\ntype: old\nstatus: queued\n---\n# t\n")
	if err := addSidekickSeedType(existing); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(existing)
	if !strings.Contains(string(b), "type: sidekick-seed") || strings.Contains(string(b), "type: old") {
		t.Fatalf("type not rewritten: %q", string(b))
	}
}

func TestFrontmatterKeyOf_NoColon(t *testing.T) {
	if got := frontmatterKeyOf("no colon here"); got != "" {
		t.Fatalf("want empty key, got %q", got)
	}
}

func TestWriteBack_StatError(t *testing.T) {
	// Nonexistent path → statFn(real os.Stat) fails.
	if err := writeBack(filepath.Join(t.TempDir(), "nope.md"), []string{"x\n"}); err == nil {
		t.Fatal("expected stat error for nonexistent path")
	}
}

// --- rollback (direct) ---

func TestRollback_WriteError(t *testing.T) {
	roDir := t.TempDir()
	if err := os.Chmod(roDir, 0o500); err != nil { // no write → WriteFile fails
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(roDir, 0o755) }()
	opts := RelocateOptions{SeedPath: filepath.Join(roDir, "x.md")}
	if err := rollback(opts, []byte("data"), errors.New("cause")); err == nil {
		t.Fatal("expected rollback WriteFile error")
	}
}

func TestRollback_HookError(t *testing.T) {
	dir := t.TempDir()
	opts := RelocateOptions{
		SeedPath:     filepath.Join(dir, "x.md"),
		RollbackHook: func() error { return errors.New("hook boom") },
	}
	if err := rollback(opts, []byte("data"), errors.New("cause")); err == nil {
		t.Fatal("expected rollback hook error")
	}
}

// --- Relocate rollback permutations ---

func seedOpts(t *testing.T) (root, seedPath string, opts RelocateOptions) {
	t.Helper()
	root = t.TempDir()
	seedPath = filepath.Join(root, "spec", "ideas", "seeds", "foo.md")
	writeSeedFile(t, seedPath, validSeed)
	return root, seedPath, RelocateOptions{
		SpecRoot:  root,
		Slug:      "foo",
		SeedPath:  seedPath,
		NewStatus: SeedImplemented,
	}
}

func TestRelocate_CollisionStatNonNotExist(t *testing.T) {
	_, _, opts := seedOpts(t)
	defer func() { statFn = os.Stat }()
	statFn = func(string) (fs.FileInfo, error) { return nil, errors.New("stat boom") }
	if err := Relocate(opts); err == nil {
		t.Fatal("expected unexpected-stat error on collision check")
	}
}

func TestRelocate_SeedReadError(t *testing.T) {
	root := t.TempDir()
	opts := RelocateOptions{
		SpecRoot: root, Slug: "foo",
		SeedPath:  filepath.Join(root, "spec", "ideas", "seeds", "foo.md"), // absent
		NewStatus: SeedImplemented,
	}
	if err := Relocate(opts); err == nil {
		t.Fatal("expected ReadFile error for missing seed")
	}
}

func TestRelocate_RewriteError(t *testing.T) {
	root := t.TempDir()
	seedPath := filepath.Join(root, "spec", "ideas", "seeds", "foo.md")
	writeSeedFile(t, seedPath, "---\ncaptured_by: user\n---\n# no status\n") // no status line
	opts := RelocateOptions{SpecRoot: root, Slug: "foo", SeedPath: seedPath, NewStatus: SeedImplemented}
	if err := Relocate(opts); err == nil {
		t.Fatal("expected rewrite error (no status line)")
	}
}

func TestRelocate_AddTypeError(t *testing.T) {
	_, _, opts := seedOpts(t)
	defer func() { statFn = os.Stat }()
	calls := 0
	statFn = func(name string) (fs.FileInfo, error) {
		calls++
		switch calls {
		case 1:
			return nil, os.ErrNotExist // collision check: proceed
		case 3:
			return nil, errors.New("writeBack stat boom") // addSidekickSeedType → writeBack
		default:
			return os.Stat(name) // call 2: Rewrite's stat
		}
	}
	if err := Relocate(opts); err == nil {
		t.Fatal("expected add-type error")
	}
}

func TestRelocate_MkdirAllError(t *testing.T) {
	_, _, opts := seedOpts(t)
	defer func() { mkdirAllFn = os.MkdirAll }()
	mkdirAllFn = func(string, os.FileMode) error { return errors.New("mkdir boom") }
	if err := Relocate(opts); err == nil {
		t.Fatal("expected mkdir error")
	}
}

func TestRelocate_RenameError(t *testing.T) {
	_, _, opts := seedOpts(t)
	defer func() { renameFn = os.Rename }()
	renameFn = func(string, string) error { return errors.New("rename boom") }
	if err := Relocate(opts); err == nil {
		t.Fatal("expected rename error")
	}
}

func TestRelocate_PostMoveHookRollback(t *testing.T) {
	_, _, opts := seedOpts(t)
	opts.afterMoveHook = func() error { return errors.New("lint boom") }
	if err := Relocate(opts); err == nil {
		t.Fatal("expected post-move hook error with rollback")
	}
	// Seed restored to seeds/ with original status.
	b, err := os.ReadFile(opts.SeedPath)
	if err != nil || !strings.Contains(string(b), "status: queued") {
		t.Fatalf("seed not restored: %q err %v", string(b), err)
	}
}

func TestRelocate_PostMoveHookAndRenameBackFail(t *testing.T) {
	_, _, opts := seedOpts(t)
	defer func() { renameFn = os.Rename }()
	rcalls := 0
	renameFn = func(o, n string) error {
		rcalls++
		if rcalls == 1 {
			return os.Rename(o, n) // forward move succeeds
		}
		return errors.New("rename-back boom") // rollback move-back fails
	}
	opts.afterMoveHook = func() error { return errors.New("lint boom") }
	if err := Relocate(opts); err == nil {
		t.Fatal("expected post-move + rename-back failure")
	}
}

func TestAddSidekickSeedType_ReadErrorAndNoTermFence(t *testing.T) {
	if err := addSidekickSeedType(filepath.Join(t.TempDir(), "nope.md")); err == nil {
		t.Fatal("expected ReadFile error")
	}
	// A lone "---" with no terminator: the insert path defaults openTerm to "\n".
	p := filepath.Join(t.TempDir(), "fence.md")
	writeSeedFile(t, p, "---")
	if err := addSidekickSeedType(p); err != nil {
		t.Fatal(err)
	}
	if b, _ := os.ReadFile(p); !strings.Contains(string(b), "type: sidekick-seed") {
		t.Fatalf("type not inserted: %q", string(b))
	}
}

func TestReadFrontmatterStatus_OpenFenceNoCloseNoNewline(t *testing.T) {
	// Open fence + non-status key + no closing fence + no final newline:
	// exercises splitTerminator's no-terminator branch and the loop-end -1.
	p := filepath.Join(t.TempDir(), "open.md")
	writeSeedFile(t, p, "---\nfoo: bar")
	if _, err := ReadFrontmatterStatus(p); err != ErrFrontmatterStatusNotFound {
		t.Fatalf("want ErrFrontmatterStatusNotFound, got %v", err)
	}
}
