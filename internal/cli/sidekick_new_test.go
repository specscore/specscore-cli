package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lint"
)

// runSidekick invokes the `sidekick` cobra command tree in-process and
// captures stdout, stderr, and the returned error.
func runSidekick(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := sidekickCommand()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func readSeed(t *testing.T, root, slug string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "spec", "ideas", "seeds", slug+".md"))
	if err != nil {
		t.Fatalf("reading seed %s: %v", slug, err)
	}
	return string(b)
}

// AC: group-exposes-subcommands
func TestSidekickGroup_NoSubcommand_PrintsHelpExitsZero(t *testing.T) {
	out, _, err := runSidekick(t)
	if err != nil {
		t.Fatalf("expected nil error (exit 0), got %v", err)
	}
	if !strings.Contains(out, "new") {
		t.Errorf("expected help listing the new subcommand, got: %q", out)
	}
}

// AC: scaffolded-seed-is-lint-clean
func TestSidekickNew_LintClean(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)
	// setupSpecRoot leaves spec/features/ without a README; hand-write a
	// minimal Features index, then re-run the migrate engine to backfill the
	// frontmatter every index requires, so the whole tree is clean and the
	// only candidate violation is the new seed file (mirrors stageActiveIdea).
	featuresReadme := "# Features\n\n## Index\n\n| Feature | Status |\n|---------|--------|\n\n_No features yet._\n\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(filepath.Join(root, "spec", "features", "README.md"), []byte(featuresReadme), 0o644); err != nil {
		t.Fatalf("write features README: %v", err)
	}
	migrateTree(t, root)

	if _, _, err := runSidekick(t, "new", "specscore needs a seed verb"); err != nil {
		t.Fatalf("sidekick new: %v", err)
	}
	s := readSeed(t, root, "specscore-needs-a-seed-verb")
	for _, want := range []string{
		"captured_by: user\n",
		"status: queued\n",
		"# specscore needs a seed verb\n",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("seed missing %q:\n%s", want, s)
		}
	}
	for _, gone := range []string{"type:", "slug:"} {
		if strings.Contains(s, gone) {
			t.Errorf("seed must not emit %q:\n%s", gone, s)
		}
	}

	violations, err := lint.Lint(lint.Options{SpecRoot: filepath.Join(root, "spec")})
	if err != nil {
		t.Fatalf("lint: %v", err)
	}
	rel := filepath.Join("ideas", "seeds", "specscore-needs-a-seed-verb.md")
	for _, v := range violations {
		if v.Severity != "error" || v.File == rel {
			continue
		}
		t.Errorf("unexpected error-severity violation outside the new seed: %s:%d [%s] %s", v.File, v.Line, v.Rule, v.Message)
	}
}

// AC: slug-derived-from-one-liner
func TestSidekickNew_SlugDerived(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)

	if _, _, err := runSidekick(t, "new", "Add Batch Mode!"); err != nil {
		t.Fatalf("sidekick new: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "spec", "ideas", "seeds", "add-batch-mode.md")); err != nil {
		t.Errorf("expected seed at add-batch-mode.md: %v", err)
	}
}

// AC: slug-override-used-verbatim
func TestSidekickNew_SlugOverride(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)

	// Valid override: the override names the file; H1 keeps the one-liner.
	// (slug is no longer written to frontmatter — it only drives the filename.)
	if _, _, err := runSidekick(t, "new", "Add batch mode", "--slug", "add-batch-mode-2"); err != nil {
		t.Fatalf("sidekick new --slug: %v", err)
	}
	s := readSeed(t, root, "add-batch-mode-2")
	if !strings.Contains(s, "# Add batch mode\n") {
		t.Errorf("override seed content wrong:\n%s", s)
	}

	// Invalid override exits 2 and writes nothing.
	_, _, err := runSidekick(t, "new", "x", "--slug", "Bad_Slug")
	if got := exitCodeOf(err); got != 2 {
		t.Errorf("invalid --slug: exit = %d, want 2", got)
	}
	if _, statErr := os.Stat(filepath.Join(root, "spec", "ideas", "seeds", "Bad_Slug.md")); statErr == nil {
		t.Error("no file should be created on invalid --slug")
	}
}

// AC: empty-one-liner-rejected
func TestSidekickNew_EmptyOrUnslugableRejected(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)

	for _, arg := range []string{"   ", "!!!"} {
		_, _, err := runSidekick(t, "new", arg)
		if got := exitCodeOf(err); got != 2 {
			t.Errorf("sidekick new %q: exit = %d, want 2", arg, got)
		}
	}
	// No seed directory should have been created from the rejected calls.
	entries, _ := os.ReadDir(filepath.Join(root, "spec", "ideas", "seeds"))
	if len(entries) != 0 {
		t.Errorf("expected no seed files, got %d", len(entries))
	}
}

// AC: existing-file-conflict
func TestSidekickNew_ConflictAndForce(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)

	if _, _, err := runSidekick(t, "new", "dup idea"); err != nil {
		t.Fatalf("first create: %v", err)
	}
	_, _, err := runSidekick(t, "new", "dup idea")
	if got := exitCodeOf(err); got != 1 {
		t.Errorf("second create without --force: exit = %d, want 1", got)
	}
	if _, _, err := runSidekick(t, "new", "dup idea", "--force"); err != nil {
		t.Errorf("create with --force: %v", err)
	}
}

// AC: ancestor-indexes-materialized
func TestSidekickNew_AncestorIndexesMaterialized(t *testing.T) {
	root := t.TempDir()
	// spec/features/ presence makes FindSpecRepoRoot recognize the root,
	// while spec/ideas/ is deliberately absent so materialization runs.
	if err := os.MkdirAll(filepath.Join(root, "spec", "features"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	withCwd(t, root)

	if _, _, err := runSidekick(t, "new", "first seed"); err != nil {
		t.Fatalf("sidekick new: %v", err)
	}
	for _, p := range []string{
		filepath.Join("spec", "README.md"),
		filepath.Join("spec", "ideas", "README.md"),
		filepath.Join("spec", "ideas", "seeds", "first-seed.md"),
	} {
		if _, err := os.Stat(filepath.Join(root, p)); err != nil {
			t.Errorf("expected %s to exist: %v", p, err)
		}
	}

	// Idempotence: the materialized indexes are left untouched on a second run.
	idxPath := filepath.Join(root, "spec", "ideas", "README.md")
	before, _ := os.ReadFile(idxPath)
	if _, _, err := runSidekick(t, "new", "second seed"); err != nil {
		t.Fatalf("second sidekick new: %v", err)
	}
	after, _ := os.ReadFile(idxPath)
	if string(before) != string(after) {
		t.Error("spec/ideas/README.md was modified on the second run; expected idempotent materialization")
	}
}

// REQ: one-liner-length — a one-liner over the cap exits 2 at the CLI boundary.
func TestSidekickNew_OneLinerTooLong(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)
	_, _, err := runSidekick(t, "new", strings.Repeat("a", 501))
	if got := exitCodeOf(err); got != 2 {
		t.Errorf("exit = %d, want 2", got)
	}
}

// REQ: optional-body — an over-cap --body makes Scaffold reject the seed, which
// the CLI surfaces as exit 2 (InvalidArgs) before any file is written.
func TestSidekickNew_BodyTooLong(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)
	_, _, err := runSidekick(t, "new", "valid one liner", "--body", strings.Repeat("a", 3001))
	if got := exitCodeOf(err); got != 2 {
		t.Errorf("exit = %d, want 2", got)
	}
	if _, statErr := os.Stat(filepath.Join(root, "spec", "ideas", "seeds", "valid-one-liner.md")); statErr == nil {
		t.Error("no file should be created when --body exceeds the cap")
	}
}

// REQ: ancestor-indexes-materialized — an unresolvable spec root surfaces an error.
func TestSidekickNew_ResolveSpecRootError(t *testing.T) {
	dir := t.TempDir() // no spec/ tree → FindSpecRepoRoot fails
	withCwd(t, dir)
	_, _, err := runSidekick(t, "new", "valid one liner")
	if err == nil {
		t.Fatal("expected error when no spec root can be resolved")
	}
}

// REQ: ancestor-indexes-materialized — a read-only spec/ makes index materialization fail.
func TestSidekickNew_AncestorIndexError(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)
	specDir := filepath.Join(root, "spec")
	if err := os.Chmod(specDir, 0o555); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(specDir, 0o755) }()

	_, _, err := runSidekick(t, "new", "needs an index")
	if got := exitCodeOf(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d (Unexpected)", got, exitcode.Unexpected)
	}
}

// REQ: ancestor-indexes-materialized — indexes succeed but the seeds dir cannot be created.
func TestSidekickNew_MkdirSeedsError(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)
	// spec/README.md materializes into the writable spec/; spec/ideas/README.md
	// already exists, so ensure succeeds. A read-only spec/ideas/ then blocks
	// MkdirAll(spec/ideas/seeds).
	ideasDir := filepath.Join(root, "spec", "ideas")
	if err := os.Chmod(ideasDir, 0o555); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(ideasDir, 0o755) }()

	_, _, err := runSidekick(t, "new", "blocked seeds dir")
	if got := exitCodeOf(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d (Unexpected)", got, exitcode.Unexpected)
	}
}

// REQ: scaffolds-lint-clean — the final seed write surfaces I/O failures.
func TestSidekickNew_WriteError(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)
	// Pre-create the seeds dir, then make it read-only so the seed-file write
	// fails after ensure + MkdirAll both succeed.
	seedsDir := filepath.Join(root, "spec", "ideas", "seeds")
	if err := os.MkdirAll(seedsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(seedsDir, 0o555); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(seedsDir, 0o755) }()

	_, _, err := runSidekick(t, "new", "unwritable seed")
	if got := exitCodeOf(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d (Unexpected)", got, exitcode.Unexpected)
	}
}
