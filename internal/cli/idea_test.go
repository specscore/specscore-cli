package cli

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/idea"
	"github.com/specscore/specscore-cli/pkg/lint"
	"github.com/specscore/specscore-cli/pkg/projectdef"
)

// setupSpecRoot stages a temp spec repo with empty indexes and returns the
// repo root (the parent of `spec/`). The test's CWD is set to root so that
// FindSpecRepoRoot picks it up.
func setupSpecRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	specDir := filepath.Join(root, "spec")
	featDir := filepath.Join(specDir, "features")
	if err := os.MkdirAll(featDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	ideasDir := filepath.Join(specDir, "ideas")
	if err := os.MkdirAll(filepath.Join(ideasDir, "archived"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	idx := "# Ideas\n\n## Index\n\n| Idea | Status | Date | Owner | Promotes To |\n|------|--------|------|-------|-------------|\n\n_No active ideas yet._\n\n## Open Questions\n\nNone at this time.\n"
	_ = os.WriteFile(filepath.Join(ideasDir, "README.md"), []byte(idx), 0o644)
	arch := "# Archived\n\n_No archived ideas yet._\n\n## Open Questions\n\nNone at this time.\n"
	_ = os.WriteFile(filepath.Join(ideasDir, "archived", "README.md"), []byte(arch), 0o644)
	migrateTree(t, root)
	return root
}

func withCwd(t *testing.T, dir string) {
	t.Helper()
	old, _ := os.Getwd()
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

func runIdea(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := ideaCommand()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func TestIdeaNew_BareInvocationLintClean(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)

	_, _, err := runIdea(t, "new", "demo-bare")
	if err != nil {
		t.Fatalf("command failed: %v", err)
	}
	path := filepath.Join(root, "spec", "ideas", "demo-bare.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected idea file at %s: %v", path, err)
	}
	// Index should now list demo-bare.
	idx, _ := os.ReadFile(filepath.Join(root, "spec", "ideas", "README.md"))
	if !strings.Contains(string(idx), "[demo-bare](demo-bare.md)") {
		t.Errorf("index not updated: %s", idx)
	}
}

func TestIdeaNew_FlagsInject(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)

	_, _, err := runIdea(t, "new", "demo-flag",
		"--title", "Demo Flag",
		"--owner", "alice",
		"--hmw", "How might we flag this?",
		"--not-doing", "thing one — reason",
	)
	if err != nil {
		t.Fatalf("command failed: %v", err)
	}
	body, _ := os.ReadFile(filepath.Join(root, "spec", "ideas", "demo-flag.md"))
	s := string(body)
	for _, want := range []string{
		"# Idea: Demo Flag",
		"**Owner:** alice",
		"How might we flag this?",
		"- thing one — reason",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in generated body:\n%s", want, s)
		}
	}
}

func TestIdeaNew_DuplicateRefused(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)

	if _, _, err := runIdea(t, "new", "dup"); err != nil {
		t.Fatalf("first run: %v", err)
	}
	_, _, err := runIdea(t, "new", "dup")
	if err == nil {
		t.Fatal("expected second run to fail without --force")
	}
	if !strings.Contains(err.Error(), "already exists") {
		t.Errorf("unexpected error: %v", err)
	}

	// With --force, succeeds.
	if _, _, err := runIdea(t, "new", "dup", "--force", "--title", "After Force"); err != nil {
		t.Fatalf("--force run: %v", err)
	}
	body, _ := os.ReadFile(filepath.Join(root, "spec", "ideas", "dup.md"))
	if !strings.Contains(string(body), "# Idea: After Force") {
		t.Errorf("expected overwrite, got:\n%s", body)
	}
}

func TestIdeaNew_InvalidSlug(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)

	_, _, err := runIdea(t, "new", "BadSlug")
	if err == nil {
		t.Fatal("expected error for invalid slug")
	}
}

func TestIdeaNew_Interactive(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)

	cmd := ideaCommand()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	// Provide: title, owner, hmw, then blank-through the rest, then a
	// single not-doing bullet, then blank to end.
	cmd.SetIn(strings.NewReader("Interactive Demo\nzoe\nHow might we interact?\n\n\n\nthing interactive — reason\n\n"))
	cmd.SetArgs([]string{"new", "demo-inter", "-i"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("interactive run failed: %v\nerr: %s", err, errOut.String())
	}
	body, _ := os.ReadFile(filepath.Join(root, "spec", "ideas", "demo-inter.md"))
	s := string(body)
	if !strings.Contains(s, "# Idea: Interactive Demo") {
		t.Errorf("title not captured from interactive input:\n%s", s)
	}
	if !strings.Contains(s, "**Owner:** zoe") {
		t.Errorf("owner not captured:\n%s", s)
	}
	if !strings.Contains(s, "How might we interact?") {
		t.Errorf("hmw not captured:\n%s", s)
	}
	if !strings.Contains(s, "- thing interactive — reason") {
		t.Errorf("not-doing not captured:\n%s", s)
	}
}

func TestIdeaNew_IndexRowInserted(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)

	if _, _, err := runIdea(t, "new", "idx-check", "--owner", "pat"); err != nil {
		t.Fatalf("run: %v", err)
	}
	idx, _ := os.ReadFile(filepath.Join(root, "spec", "ideas", "README.md"))
	s := string(idx)
	if !strings.Contains(s, "[idx-check](idx-check.md)") {
		t.Errorf("missing idea link in index:\n%s", s)
	}
	if !strings.Contains(s, "pat") {
		t.Errorf("missing owner in index:\n%s", s)
	}
}

// AC: lint-clean-after-bare-project — `idea new` in a project that has
// specscore.yaml but no spec/ tree MUST materialize spec/README.md and
// spec/ideas/README.md, then leave the tree lint-clean modulo any
// violations inside the newly created Idea file.
func TestIdeaNew_BareProjectMaterializesAncestorIndexes(t *testing.T) {
	root := t.TempDir()
	// Only specscore.yaml exists — no spec/ tree.
	if err := projectdef.WriteSpecConfig(root, projectdef.SpecConfig{}); err != nil {
		t.Fatalf("write specscore.yaml: %v", err)
	}
	withCwd(t, root)

	if _, _, err := runIdea(t, "new", "first-idea"); err != nil {
		t.Fatalf("idea new: %v", err)
	}

	// Both ancestor indexes MUST exist with their canonical headings.
	specReadme, err := os.ReadFile(filepath.Join(root, "spec", "README.md"))
	if err != nil {
		t.Fatalf("spec/README.md missing: %v", err)
	}
	if !strings.Contains(string(specReadme), "# Specifications") {
		t.Errorf("spec/README.md missing canonical heading:\n%s", specReadme)
	}
	ideasReadme, err := os.ReadFile(filepath.Join(root, "spec", "ideas", "README.md"))
	if err != nil {
		t.Fatalf("spec/ideas/README.md missing: %v", err)
	}
	for _, want := range []string{"# Ideas", "## Index", "## Open Questions", "ideas-index-specification"} {
		if !strings.Contains(string(ideasReadme), want) {
			t.Errorf("spec/ideas/README.md missing %q:\n%s", want, ideasReadme)
		}
	}
	// Idea file itself.
	if _, err := os.Stat(filepath.Join(root, "spec", "ideas", "first-idea.md")); err != nil {
		t.Fatalf("idea file missing: %v", err)
	}

	// Lint MUST be clean for everything outside spec/ideas/first-idea.md.
	// We run lint directly (not via the CLI) to avoid coupling to spec lint
	// CLI changes in this test.
	violations, err := lint.Lint(lint.Options{SpecRoot: filepath.Join(root, "spec")})
	if err != nil {
		t.Fatalf("lint: %v", err)
	}
	for _, v := range violations {
		if v.Severity != "error" {
			continue
		}
		// Allow error-severity violations only inside the new idea file.
		if v.File == filepath.Join("ideas", "first-idea.md") {
			continue
		}
		t.Errorf("unexpected error-severity violation outside the new idea: %s:%d [%s] %s", v.File, v.Line, v.Rule, v.Message)
	}
}

// Idempotence: running idea new twice with different slugs in a bare
// project MUST NOT clobber the indexes created on the first run.
func TestIdeaNew_AncestorIndexesIdempotent(t *testing.T) {
	root := t.TempDir()
	if err := projectdef.WriteSpecConfig(root, projectdef.SpecConfig{}); err != nil {
		t.Fatalf("write specscore.yaml: %v", err)
	}
	withCwd(t, root)

	if _, _, err := runIdea(t, "new", "alpha"); err != nil {
		t.Fatalf("first idea new: %v", err)
	}
	specBefore, _ := os.ReadFile(filepath.Join(root, "spec", "README.md"))

	if _, _, err := runIdea(t, "new", "beta"); err != nil {
		t.Fatalf("second idea new: %v", err)
	}
	specAfter, _ := os.ReadFile(filepath.Join(root, "spec", "README.md"))

	if string(specBefore) != string(specAfter) {
		t.Errorf("spec/README.md mutated by second idea new:\nbefore=%q\nafter=%q", specBefore, specAfter)
	}
}

// =====================================================================
// idea change-status CLI tests
// =====================================================================
//
// Per-AC mapping (matches spec/features/cli/idea/change-status/README.md):
//
//   TestIdeaChangeStatus_DraftToApprovedHappyPath_CLI -> draft-to-approved-happy-path
//   TestIdeaChangeStatus_ArchiveHappyPath_CLI         -> archive-from-approved-happy-path
//   TestIdeaChangeStatus_MissingSlugRejected_CLI      -> missing-slug-rejected
//   TestIdeaChangeStatus_MissingToFlagRejected_CLI    -> missing-to-flag-rejected
//   TestIdeaChangeStatus_UnrecognizedToValueRejected_CLI -> unrecognized-to-value-rejected
//   TestIdeaChangeStatus_SlugNotFound_CLI             -> slug-not-found
//
// Logic-level coverage of the remaining ACs lives in
// pkg/idea/transitions_test.go.

// stageActiveIdea creates a lint-clean spec tree at root and writes a
// single Idea file at spec/ideas/<slug>.md with the given status. The
// tree is materialized via `specscore idea new` plus a hand-written
// spec/features/README.md so that running `spec lint` across the whole
// tree (which the verb's post-mutation hook does) finds no error-
// severity violations. The status line is then patched in place and a
// final lint --fix syncs the indexes. A subsequent CLI run won't trip
// the post-mutation rollback on pre-existing violations.
//
// extraHeader is injected into the Idea's header block (just after the
// Status line, BEFORE the first `## ` section) so the Idea parser
// picks it up as a header field — used by archive tests to add the
// **Archive Reason:** field that idea-archive-reason requires for
// Status: Archived.
func stageActiveIdea(t *testing.T, slug, status string, extraHeader string) string {
	t.Helper()
	root := setupSpecRoot(t)
	withCwd(t, root)
	// Bootstrap via `idea new` so spec/README.md and spec/ideas/README.md
	// land via the same template the production CLI uses.
	if _, _, err := runIdea(t, "new", slug, "--owner", "tester"); err != nil {
		t.Fatalf("idea new: %v", err)
	}
	// idea new does NOT materialize spec/features/README.md (that's
	// init's job). Hand-write a minimal lint-clean Features index so
	// lint passes across the whole tree.
	featuresReadme := "# Features\n\n## Index\n\n| Feature | Status |\n|---------|--------|\n\n_No features yet._\n\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(
		filepath.Join(root, "spec", "features", "README.md"),
		[]byte(featuresReadme), 0o644); err != nil {
		t.Fatalf("write features README: %v", err)
	}
	// Patch the status line and (optionally) inject extra header fields
	// directly after it, so they land in the header block (above the
	// first `## ` section) where the Idea parser scans for fields.
	path := filepath.Join(root, "spec", "ideas", slug+".md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read idea: %v", err)
	}
	newStatusLine := "**Status:** " + status
	if extraHeader != "" {
		newStatusLine += "\n" + strings.TrimRight(extraHeader, "\n")
	}
	patched := strings.Replace(string(raw),
		"**Status:** Draft",
		newStatusLine, 1)
	if err := os.WriteFile(path, []byte(patched), 0o644); err != nil {
		t.Fatalf("write patched idea: %v", err)
	}
	migrateTree(t, root)
	// Sync indexes via lint --fix so the active index reflects the
	// patched status.
	if _, err := lint.Lint(lint.Options{
		SpecRoot: filepath.Join(root, "spec"),
		Fix:      true,
	}); err != nil {
		t.Fatalf("initial lint --fix: %v", err)
	}
	// Verify no error-severity lint violations exist before our test
	// runs — otherwise the post-mutation rollback would fire on
	// pre-existing tree errors and the test wouldn't actually exercise
	// the verb under test.
	vs, err := lint.Lint(lint.Options{SpecRoot: filepath.Join(root, "spec")})
	if err != nil {
		t.Fatalf("verify lint: %v", err)
	}
	for _, v := range vs {
		if v.Severity == "error" {
			t.Fatalf("pre-existing lint error in test fixture: %s:%d [%s] %s",
				v.File, v.Line, v.Rule, v.Message)
		}
	}
	return root
}

// AC: draft-to-approved-happy-path (CLI level — exercises the full
// cobra → idea.ChangeStatus → lint --fix path).
func TestIdeaChangeStatus_DraftToApprovedHappyPath_CLI(t *testing.T) {
	root := stageActiveIdea(t, "foo", "Draft", "")

	stdout, stderr, err := runIdea(t, "change-status", "foo", "--to=approved")
	if err != nil {
		t.Fatalf("change-status: %v (stderr=%s)", err, stderr)
	}
	// Stdout MUST be exactly "<slug>: <from> → <to>\n" — nothing else.
	want := "foo: Draft → Approved\n"
	if stdout != want {
		t.Errorf("stdout = %q; want %q", stdout, want)
	}
	body, _ := os.ReadFile(filepath.Join(root, "spec", "ideas", "foo.md"))
	if !strings.Contains(string(body), "**Status:** Approved") {
		t.Errorf("status not rewritten:\n%s", body)
	}
	// Index row MUST reflect the new status.
	idx, _ := os.ReadFile(filepath.Join(root, "spec", "ideas", "README.md"))
	if !strings.Contains(string(idx), "Approved") {
		t.Errorf("index not synced:\n%s", idx)
	}
}

// AC: archive-happy-path (CLI level — exercises the full cobra →
// idea.Archive → lint --fix path). Archival is orthogonal to status: the
// archived file keeps its terminal **Status:** and gains **Archived:** true.
func TestIdeaArchive_HappyPath_CLI(t *testing.T) {
	root := stageActiveIdea(t, "foo", "Stale", "")

	stdout, stderr, err := runIdea(t, "archive", "foo", "--note", "superseded by follow-up idea")
	if err != nil {
		t.Fatalf("archive: %v (stderr=%s)", err, stderr)
	}
	activePath := filepath.Join(root, "spec", "ideas", "foo.md")
	archivedPath := filepath.Join(root, "spec", "ideas", "archived", "foo.md")
	// macOS resolves t.TempDir() through /private; assert on the structure
	// rather than the exact OS-resolved absolute path.
	if !strings.HasPrefix(stdout, "foo: ") ||
		!strings.Contains(stdout, "ideas/foo.md → ") ||
		!strings.Contains(stdout, "ideas/archived/foo.md") {
		t.Errorf("stdout = %q; want 'foo: <active> → <archived>'", stdout)
	}
	// File moved.
	if _, err := os.Stat(activePath); !os.IsNotExist(err) {
		t.Errorf("active file should be gone: err=%v", err)
	}
	body, err := os.ReadFile(archivedPath)
	if err != nil {
		t.Fatalf("archived file missing: %v", err)
	}
	if !strings.Contains(string(body), "**Status:** Stale") {
		t.Errorf("archived file must keep terminal status:\n%s", body)
	}
	if !strings.Contains(string(body), "**Archived:** true") {
		t.Errorf("archived file missing **Archived:** true:\n%s", body)
	}
}

// AC: unarchive-happy-path (CLI level).
func TestIdeaUnarchive_HappyPath_CLI(t *testing.T) {
	root := stageActiveIdea(t, "foo", "Stale", "")
	if _, stderr, err := runIdea(t, "archive", "foo"); err != nil {
		t.Fatalf("archive: %v (stderr=%s)", err, stderr)
	}

	stdout, stderr, err := runIdea(t, "unarchive", "foo")
	if err != nil {
		t.Fatalf("unarchive: %v (stderr=%s)", err, stderr)
	}
	activePath := filepath.Join(root, "spec", "ideas", "foo.md")
	archivedPath := filepath.Join(root, "spec", "ideas", "archived", "foo.md")
	if !strings.HasPrefix(stdout, "foo: ") ||
		!strings.Contains(stdout, "ideas/archived/foo.md → ") ||
		!strings.HasSuffix(strings.TrimSpace(stdout), "ideas/foo.md") {
		t.Errorf("stdout = %q; want 'foo: <archived> → <active>'", stdout)
	}
	if _, err := os.Stat(archivedPath); !os.IsNotExist(err) {
		t.Errorf("archived file should be gone: err=%v", err)
	}
	body, err := os.ReadFile(activePath)
	if err != nil {
		t.Fatalf("active file missing: %v", err)
	}
	if strings.Contains(string(body), "**Archived:**") {
		t.Errorf("**Archived:** line should be cleared:\n%s", body)
	}
}

// An invalid slug is rejected (exit 2) by both archive and unarchive before
// any filesystem work.
func TestIdeaArchiveUnarchive_InvalidSlug_CLI(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)

	for _, verb := range []string{"archive", "unarchive"} {
		_, _, err := runIdea(t, verb, "Bad_Slug")
		if err == nil {
			t.Fatalf("%s: expected error for invalid slug", verb)
		}
		type exitCoder interface{ ExitCode() int }
		var ec exitCoder
		if !errors.As(err, &ec) || ec.ExitCode() != 2 {
			t.Errorf("%s: want exit 2 for invalid slug, got %v", verb, err)
		}
	}
}

// A missing target (no active file to archive / no archived file to
// unarchive) surfaces the underlying NotFound (exit 3).
func TestIdeaArchiveUnarchive_NotFound_CLI(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)

	for _, verb := range []string{"archive", "unarchive"} {
		_, _, err := runIdea(t, verb, "ghost")
		if err == nil {
			t.Fatalf("%s: expected error for missing target", verb)
		}
		type exitCoder interface{ ExitCode() int }
		var ec exitCoder
		if !errors.As(err, &ec) || ec.ExitCode() != 3 {
			t.Errorf("%s: want exit 3 for missing target, got %v", verb, err)
		}
	}
}

// When the project root cannot be resolved (no spec repo at the given
// --project path), both verbs surface the resolveSpecRoot error.
func TestIdeaArchiveUnarchive_UnresolvableProject_CLI(t *testing.T) {
	// A bare temp dir with no specscore.yaml / spec tree and no git repo.
	bare := t.TempDir()
	for _, verb := range []string{"archive", "unarchive"} {
		_, _, err := runIdea(t, verb, "foo", "--project", bare)
		if err == nil {
			t.Fatalf("%s: expected error for unresolvable project root", verb)
		}
	}
}

// AC: missing-slug-rejected — no positional argument. Cobra's
// ExactArgs(1) rejects this and returns an error; the CLI's Fatal
// helper maps that to exit 2.
func TestIdeaChangeStatus_MissingSlugRejected_CLI(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)

	_, stderr, err := runIdea(t, "change-status", "--to=approved")
	if err == nil {
		t.Fatal("expected error for missing positional")
	}
	// Cobra prints a usage error to stderr; we just confirm SOME
	// rejection happened. Exit-code mapping for ExactArgs is exit 2
	// at the CLI top-level (cobra's default).
	if stderr == "" && err.Error() == "" {
		t.Errorf("expected non-empty error/stderr; got err=%v stderr=%q", err, stderr)
	}
}

// AC: missing-to-flag-rejected.
func TestIdeaChangeStatus_MissingToFlagRejected_CLI(t *testing.T) {
	root := stageActiveIdea(t, "foo", "Draft", "")
	_ = root

	_, stderr, err := runIdea(t, "change-status", "foo")
	if err == nil {
		t.Fatal("expected error for missing --to flag")
	}
	// Cobra's MarkFlagRequired error message names the missing flag.
	combined := err.Error() + stderr
	if !strings.Contains(combined, "to") {
		t.Errorf("expected error/stderr to name `to`; got err=%v stderr=%q", err, stderr)
	}
}

// AC: unrecognized-to-value-rejected (CLI level — exit 2 BEFORE
// state-machine check).
func TestIdeaChangeStatus_UnrecognizedToValueRejected_CLI(t *testing.T) {
	root := stageActiveIdea(t, "foo", "Draft", "")
	_ = root

	_, _, err := runIdea(t, "change-status", "foo", "--to=banana")
	if err == nil {
		t.Fatal("expected error for --to=banana")
	}
	type exitCoder interface{ ExitCode() int }
	var ec exitCoder
	if !errors.As(err, &ec) {
		t.Fatalf("error does not carry ExitCode: %v", err)
	}
	if got := ec.ExitCode(); got != 2 {
		t.Errorf("exit code = %d; want 2", got)
	}
	if !strings.Contains(err.Error(), "banana") {
		t.Errorf("error message missing %q: %v", "banana", err)
	}
}

// AC: slug-not-found (CLI level).
func TestIdeaChangeStatus_SlugNotFound_CLI(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)
	// Pre-existing archived file does NOT satisfy active lookup.
	if err := os.WriteFile(
		filepath.Join(root, "spec", "ideas", "archived", "nonexistent.md"),
		[]byte("# x\n**Status:** Archived\n"), 0o644); err != nil {
		t.Fatalf("write archived: %v", err)
	}

	_, _, err := runIdea(t, "change-status", "nonexistent", "--to=approved")
	if err == nil {
		t.Fatal("expected error for missing active file")
	}
	type exitCoder interface{ ExitCode() int }
	var ec exitCoder
	if !errors.As(err, &ec) {
		t.Fatalf("error does not carry ExitCode: %v", err)
	}
	if got := ec.ExitCode(); got != 3 {
		t.Errorf("exit code = %d; want 3", got)
	}
}

// Ensure the exported idea.Scaffold still produces a valid file for a
// hand-chosen set of options.
func TestIdeaNew_ScaffoldExported(t *testing.T) {
	body, err := idea.Scaffold(idea.ScaffoldOptions{Slug: "export-check"})
	if err != nil {
		t.Fatalf("Scaffold: %v", err)
	}
	if !strings.Contains(string(body), "# Idea: Export Check") {
		t.Errorf("unexpected title: %s", body)
	}
}

// =====================================================================
// idea new --type=change-request tests
// =====================================================================

// setupSpecRootWithFeature stages a spec tree that includes a feature
// directory at spec/features/<featureSlug>/ with a minimal README.md.
func setupSpecRootWithFeature(t *testing.T, featureSlug string) string {
	t.Helper()
	root := setupSpecRoot(t)
	featDir := filepath.Join(root, "spec", "features", featureSlug)
	if err := os.MkdirAll(featDir, 0o755); err != nil {
		t.Fatalf("mkdir feature: %v", err)
	}
	// Write a minimal feature README so the directory is non-empty.
	readme := "# Feature: " + featureSlug + "\n\n**Status:** Approved\n"
	if err := os.WriteFile(filepath.Join(featDir, "README.md"), []byte(readme), 0o644); err != nil {
		t.Fatalf("write feature README: %v", err)
	}
	// Write a features index so lint passes.
	featuresReadme := "# Features\n\n## Index\n\n| Feature | Status |\n|---------|--------|\n\n_No features yet._\n\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(
		filepath.Join(root, "spec", "features", "README.md"),
		[]byte(featuresReadme), 0o644); err != nil {
		t.Fatalf("write features README: %v", err)
	}
	migrateTree(t, root)
	return root
}

// Test that --type=change-request --targets=<feature> creates a proposal
// at the correct path with the Proposal title prefix.
func TestIdeaNew_ChangeRequestCreatesProposal(t *testing.T) {
	root := setupSpecRootWithFeature(t, "auth")
	withCwd(t, root)

	stdout, _, err := runIdea(t, "new", "add-mfa",
		"--type", "change-request",
		"--targets", "auth",
		"--title", "Add MFA Support",
	)
	if err != nil {
		t.Fatalf("command failed: %v", err)
	}

	expectedPath := filepath.Join(root, "spec", "features", "auth", "proposals", "add-mfa.md")
	if !strings.Contains(stdout, expectedPath) {
		t.Errorf("stdout %q does not contain expected path %q", stdout, expectedPath)
	}
	if _, err := os.Stat(expectedPath); err != nil {
		t.Fatalf("proposal file not created at %s: %v", expectedPath, err)
	}
	body, _ := os.ReadFile(expectedPath)
	s := string(body)
	for _, want := range []string{
		"# Proposal: Add MFA Support",
		"**Type:** change-request",
		"**Targets:** auth",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %q in generated body:\n%s", want, s)
		}
	}
	// Must NOT contain "# Idea:".
	if strings.Contains(s, "# Idea:") {
		t.Errorf("proposal should not use Idea prefix:\n%s", s)
	}
}

func TestIdeaNew_ChangeRequestIsFullTreeLintClean(t *testing.T) {
	root := t.TempDir()
	withCwd(t, root)

	if _, errOut, err := runInitCmd(t, nil,
		"--project", root,
		"--host", "github.com",
		"--org", "acme",
		"--repo", "widget",
	); err != nil {
		t.Fatalf("init failed: %v\nstderr=%s", err, errOut)
	}
	if _, errOut, err := runFeature(t, "new",
		"--title", "Authentication",
		"--slug", "auth",
		"--description", "Authentication capability",
	); err != nil {
		t.Fatalf("feature new failed: %v\nstderr=%s", err, errOut)
	}
	if _, errOut, err := runIdea(t, "new", "add-mfa",
		"--type", "change-request",
		"--targets", "auth",
		"--title", "Add MFA",
	); err != nil {
		t.Fatalf("change-request creation failed: %v\nstderr=%s", err, errOut)
	}

	violations, err := lint.Lint(lint.Options{SpecRoot: filepath.Join(root, "spec")})
	if err != nil {
		t.Fatalf("full lint failed: %v", err)
	}
	for _, violation := range violations {
		if violation.Severity == "error" {
			t.Errorf("new change-request tree is not lint-clean: %s:%d [%s] %s",
				violation.File, violation.Line, violation.Rule, violation.Message)
		}
	}
}

// Test that --type without --targets fails.
func TestIdeaNew_ChangeRequestRequiresTargets(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)

	_, _, err := runIdea(t, "new", "orphan", "--type", "change-request")
	if err == nil {
		t.Fatal("expected error when --type=change-request without --targets")
	}
	if !strings.Contains(err.Error(), "--targets") {
		t.Errorf("error should mention --targets: %v", err)
	}
}

// Test that --targets without --type=change-request fails.
func TestIdeaNew_TargetsRequiresChangeRequest(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)

	_, _, err := runIdea(t, "new", "bad", "--targets", "auth")
	if err == nil {
		t.Fatal("expected error when --targets without --type=change-request")
	}
	if !strings.Contains(err.Error(), "change-request") {
		t.Errorf("error should mention change-request: %v", err)
	}
}

// Test error when the target feature directory does not exist.
func TestIdeaNew_ChangeRequestTargetNotFound(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)

	_, _, err := runIdea(t, "new", "fix-it",
		"--type", "change-request",
		"--targets", "nonexistent-feature",
	)
	if err == nil {
		t.Fatal("expected error for nonexistent feature directory")
	}
	type exitCoder interface{ ExitCode() int }
	var ec exitCoder
	if !errors.As(err, &ec) {
		t.Fatalf("error does not carry ExitCode: %v", err)
	}
	if got := ec.ExitCode(); got != 3 {
		t.Errorf("exit code = %d; want 3 (NotFound)", got)
	}
}

// Test that --phase is pre-populated in the header.
func TestIdeaNew_PhaseFlag(t *testing.T) {
	root := setupSpecRootWithFeature(t, "billing")
	withCwd(t, root)

	_, _, err := runIdea(t, "new", "add-invoices",
		"--type", "change-request",
		"--targets", "billing",
		"--phase", "discovery",
	)
	if err != nil {
		t.Fatalf("command failed: %v", err)
	}
	body, _ := os.ReadFile(filepath.Join(root, "spec", "features", "billing", "proposals", "add-invoices.md"))
	if !strings.Contains(string(body), "**Phase:** discovery") {
		t.Errorf("phase not found in body:\n%s", body)
	}
}

// Test that idea new without --type still creates at spec/ideas/.
func TestIdeaNew_DefaultTypeCreatesAtIdeasDir(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)

	_, _, err := runIdea(t, "new", "vanilla")
	if err != nil {
		t.Fatalf("command failed: %v", err)
	}
	path := filepath.Join(root, "spec", "ideas", "vanilla.md")
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected idea at %s: %v", path, err)
	}
	body, _ := os.ReadFile(path)
	s := string(body)
	if !strings.Contains(s, "# Idea: Vanilla") {
		t.Errorf("expected Idea prefix, got:\n%s", s)
	}
	// Should NOT contain Type or Targets fields.
	if strings.Contains(s, "**Type:**") {
		t.Errorf("default idea should not have Type field:\n%s", s)
	}
	if strings.Contains(s, "**Targets:**") {
		t.Errorf("default idea should not have Targets field:\n%s", s)
	}
}

// Test invalid --type value.
func TestIdeaNew_InvalidType(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)

	_, _, err := runIdea(t, "new", "bad-type", "--type", "banana")
	if err == nil {
		t.Fatal("expected error for invalid --type")
	}
	if !strings.Contains(err.Error(), "banana") {
		t.Errorf("error should mention invalid type: %v", err)
	}
}
