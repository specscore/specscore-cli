package cli

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lint"
	"github.com/specscore/specscore-cli/pkg/plan"
)

// testPlanTemplate mirrors the published new/plan.md gallery template: a bare
// skeleton carrying the artifact-frontmatter-convention frontmatter and
// placeholder tokens. A GALLERY-MARKER comment proves a fetched (not embedded)
// template was written.
const testPlanTemplate = `---
format: https://specscore.md/plan-specification
status: Draft
---

# Plan: <Plan Name>

**Status:** Draft
**Source Feature:** <feature-slug>
**Date:** YYYY-MM-DD
**Owner:** <your-handle>
**Supersedes:** —

## Summary

<!-- GALLERY-MARKER -->

## Approach

<!-- gallery -->

## Tasks

### Task 1: <task name>

**Verifies:** <feature-slug>#ac:<ac-slug>
**Depends-On:** —
**Status:** pending

<!-- 1–3 sentences describing what this task implements. -->

### Task 2: <task name>

**Verifies:** <feature-slug>#ac:<ac-slug>
**Depends-On:** 1
**Status:** pending

<!-- 1–3 sentences describing what this task implements. -->

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
`

func readPlan(t *testing.T, root, slug string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "spec", "plans", slug+".md"))
	if err != nil {
		t.Fatalf("reading plan %s: %v", slug, err)
	}
	return string(b)
}

// AC: scaffold-emits-planning (unify-task-status-vocabulary) — the embedded
// (offline) `plan new` scaffold names `planning` as the canonical starting
// task status and never emits the legacy `pending` token: no `### Task N:`
// block carries `**Status:** pending`.
func TestPlanNew_EmbeddedTaskStatusVocabulary(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)

	if _, _, err := runPlan(t, "new", "vocab-plan", "--feature", "some-feature"); err != nil {
		t.Fatalf("plan new: %v", err)
	}
	s := readPlan(t, root, "vocab-plan")
	if strings.Contains(s, "pending") {
		t.Errorf("scaffold must not emit the legacy `pending` task status:\n%s", s)
	}
	if !strings.Contains(s, "planning") {
		t.Errorf("scaffold must name the canonical `planning` task status:\n%s", s)
	}
}

// AC: scaffold-emits-frontmatter — the embedded (offline) scaffold carries
// format:/status: frontmatter mirroring the body, and the footer matches.
func TestPlanNew_EmbeddedEmitsFrontmatter(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)

	_, stderr, err := runPlan(t, "new", "my-plan", "--feature", "some-feature", "--owner", "alex")
	if err != nil {
		t.Fatalf("plan new: %v", err)
	}
	if !strings.Contains(stderr, "used built-in template") {
		t.Errorf("expected offline fallback warning, got %q", stderr)
	}
	s := readPlan(t, root, "my-plan")
	for _, want := range []string{
		"---\nformat: https://specscore.md/plan-specification\nstatus: Draft\n---",
		"# Plan: My Plan",
		"**Status:** Draft",
		"**Source Feature:** some-feature",
		"**Owner:** alex",
		"*This document follows the https://specscore.md/plan-specification*",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("scaffold missing %q:\n%s", want, s)
		}
	}
}

func TestPlanNew_IdeaSourceLine(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)
	if _, _, err := runPlan(t, "new", "idea-plan", "--idea", "some-idea"); err != nil {
		t.Fatalf("plan new: %v", err)
	}
	s := readPlan(t, root, "idea-plan")
	if !strings.Contains(s, "**Source:** idea:some-idea") {
		t.Errorf("idea source line missing:\n%s", s)
	}
	if strings.Contains(s, "**Source Feature:**") {
		t.Errorf("idea-sourced plan must not carry a Source Feature line:\n%s", s)
	}
}

func TestPlanNew_SourceLess(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)
	if _, _, err := runPlan(t, "new", "loose-plan"); err != nil {
		t.Fatalf("source-less plan new: %v", err)
	}
	s := readPlan(t, root, "loose-plan")
	if !strings.Contains(s, "**Source:** none") {
		t.Errorf("source-less plan missing `**Source:** none` line:\n%s", s)
	}
	if strings.Contains(s, "**Source Feature:**") || strings.Contains(s, "idea:") {
		t.Errorf("source-less plan must not carry a Feature or Idea source line:\n%s", s)
	}
}

func TestPlanNew_BothSourcesRejected(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)
	_, _, err := runPlan(t, "new", "p", "--feature", "f", "--idea", "i")
	if err == nil {
		t.Fatal("expected error when both --feature and --idea are passed")
	}
	if got := exitCodeOf(err); got != exitcode.InvalidArgs {
		t.Errorf("exit = %d, want %d", got, exitcode.InvalidArgs)
	}
}

func TestPlanNew_InvalidSlug(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)
	_, _, err := runPlan(t, "new", "Bad_Slug", "--feature", "f")
	if err == nil {
		t.Fatal("expected error for invalid slug")
	}
	if got := exitCodeOf(err); got != exitcode.InvalidArgs {
		t.Errorf("exit = %d, want %d", got, exitcode.InvalidArgs)
	}
	if _, statErr := os.Stat(filepath.Join(root, "spec", "plans", "Bad_Slug.md")); statErr == nil {
		t.Error("no file should be created for an invalid slug")
	}
}

func TestPlanNew_ParentEmitted(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)
	if _, _, err := runPlan(t, "new", "sub", "--feature", "f", "--parent", "specscore:cross-repo-master"); err != nil {
		t.Fatalf("plan new --parent: %v", err)
	}
	s := readPlan(t, root, "sub")
	if !strings.Contains(s, "**Parent:** specscore:cross-repo-master") {
		t.Errorf("scaffold missing Parent line:\n%s", s)
	}
}

func TestPlanNew_ParentEmptyExits2(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)
	_, _, err := runPlan(t, "new", "p", "--feature", "f", "--parent", "")
	if err == nil {
		t.Fatal("expected error for empty --parent")
	}
	if got := exitCodeOf(err); got != exitcode.InvalidArgs {
		t.Errorf("exit = %d, want %d", got, exitcode.InvalidArgs)
	}
	if _, statErr := os.Stat(filepath.Join(root, "spec", "plans", "p.md")); statErr == nil {
		t.Error("no file should be created when --parent is empty")
	}
}

func TestPlanNew_Collision(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)
	if _, _, err := runPlan(t, "new", "dup", "--feature", "f"); err != nil {
		t.Fatalf("first plan new: %v", err)
	}
	// Mark the file so we can prove --force overwrote vs left-untouched.
	path := filepath.Join(root, "spec", "plans", "dup.md")
	orig := readPlan(t, root, "dup")

	_, _, err := runPlan(t, "new", "dup", "--feature", "f")
	if err == nil {
		t.Fatal("expected conflict on second run")
	}
	if got := exitCodeOf(err); got != exitcode.Conflict {
		t.Errorf("exit = %d, want %d (Conflict)", got, exitcode.Conflict)
	}
	if cur := readPlan(t, root, "dup"); cur != orig {
		t.Error("conflicting run must leave the existing file untouched")
	}

	if _, _, err := runPlan(t, "new", "dup", "--feature", "other", "--force"); err != nil {
		t.Fatalf("--force run failed: %v", err)
	}
	if !strings.Contains(readPlan(t, root, "dup"), "**Source Feature:** other") {
		t.Error("--force should overwrite with the new source")
	}
	_ = path
}

// AC: ancestor-indexes-materialized — a project with no spec/plans tree gets
// spec/README.md and spec/plans/README.md created; re-running preserves the
// pre-existing index content while synchronizing its derived Plan rows.
func TestPlanNew_AncestorIndexesMaterialized(t *testing.T) {
	root := setupSpecRoot(t) // has spec/features + spec/ideas, but no spec/README.md or spec/plans
	withCwd(t, root)

	if _, _, err := runPlan(t, "new", "p1", "--feature", "f"); err != nil {
		t.Fatalf("plan new: %v", err)
	}
	for _, rel := range []string{"spec/README.md", "spec/plans/README.md", "spec/plans/p1.md"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Errorf("expected %s to exist: %v", rel, err)
		}
	}
	// Preserve hand-authored index content while adding the new derived row.
	idxPath := filepath.Join(root, "spec", "plans", "README.md")
	sentinel := "<!-- sentinel -->\n"
	cur, _ := os.ReadFile(idxPath)
	_ = os.WriteFile(idxPath, append([]byte(sentinel), cur...), 0o644)

	if _, _, err := runPlan(t, "new", "p2", "--feature", "f"); err != nil {
		t.Fatalf("second plan new: %v", err)
	}
	after, _ := os.ReadFile(idxPath)
	if !strings.HasPrefix(string(after), sentinel) {
		t.Error("plan new must preserve hand-authored plans-index content")
	}
	for _, slug := range []string{"p1", "p2"} {
		if !strings.Contains(string(after), "["+slug+"]("+slug+".md)") {
			t.Errorf("plans index missing derived row for %q:\n%s", slug, after)
		}
	}
}

func TestPlanNew_SyncsPlansIndex(t *testing.T) {
	root := setupLintCleanProject(t)
	if _, _, err := runPlan(t, "new", "indexed-plan", "--project", root, "--owner", "alex"); err != nil {
		t.Fatalf("plan new: %v", err)
	}
	index, err := os.ReadFile(filepath.Join(root, "spec", "plans", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), "| [indexed-plan](indexed-plan.md) | Draft | none |") {
		t.Errorf("plan new must synchronize the plans index:\n%s", index)
	}
}

// AC: fetches-published-template — a bare scaffold pulls <base>/new/plan.md and
// substitutes the known fields.
func TestPlanNew_FetchesPublishedTemplate(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(testPlanTemplate))
	}))
	defer srv.Close()
	t.Setenv("SPECSCORE_TEMPLATE_BASE_URL", srv.URL)

	root := setupSpecRoot(t)
	withCwd(t, root)

	_, stderr, err := runPlan(t, "new", "fetched", "--feature", "alpha", "--owner", "alex", "--title", "Fetched Plan")
	if err != nil {
		t.Fatalf("plan new: %v (stderr=%s)", err, stderr)
	}
	if gotPath != "/new/plan.md" {
		t.Errorf("fetched path = %q, want /new/plan.md", gotPath)
	}
	if stderr != "" {
		t.Errorf("unexpected fallback warning on successful fetch: %q", stderr)
	}
	s := readPlan(t, root, "fetched")
	if !strings.Contains(s, "<!-- GALLERY-MARKER -->") {
		t.Errorf("expected fetched gallery template, got:\n%s", s)
	}
	for _, want := range []string{"# Plan: Fetched Plan", "**Source Feature:** alpha", "**Owner:** alex"} {
		if !strings.Contains(s, want) {
			t.Errorf("missing substituted %q:\n%s", want, s)
		}
	}
	for _, ph := range []string{"<Plan Name>", "YYYY-MM-DD", "<your-handle>", "<feature-slug>"} {
		if strings.Contains(s, ph) {
			t.Errorf("placeholder %q was not substituted:\n%s", ph, s)
		}
	}
}

// AC: scaffolded-plan-is-lint-clean — the successful gallery-fetch path must
// be held to the same lint contract as the embedded fallback. In particular,
// a fresh plan must not carry a legacy task status which `spec lint` rejects.
func TestPlanNew_FetchedTemplateIsLintClean(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(testPlanTemplate))
	}))
	defer srv.Close()
	t.Setenv("SPECSCORE_TEMPLATE_BASE_URL", srv.URL)

	root := setupLintCleanProject(t)
	if _, _, err := runPlan(t, "new", "fetched-clean", "--project", root); err != nil {
		t.Fatalf("plan new: %v", err)
	}
	generated := readPlan(t, root, "fetched-clean")
	if strings.Contains(generated, "**Status:** pending") {
		t.Errorf("fetched plan must not retain legacy pending task status:\n%s", generated)
	}
	if !strings.Contains(generated, "**Status:** planning") {
		t.Errorf("fetched plan must use canonical planning task status:\n%s", generated)
	}

	_, stderr, err := runSpecLintCmd(t, "--project", root)
	if err != nil {
		t.Fatalf("fetched plan must pass spec lint: %v\nstderr:\n%s", err, stderr)
	}
}

// The idea-source rewrite of the fetched template turns the default Source
// Feature line into the idea form.
func TestPlanNew_FetchIdeaSourceRewrite(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(testPlanTemplate))
	}))
	defer srv.Close()
	t.Setenv("SPECSCORE_TEMPLATE_BASE_URL", srv.URL)

	root := setupSpecRoot(t)
	withCwd(t, root)
	if _, _, err := runPlan(t, "new", "fi", "--idea", "my-idea"); err != nil {
		t.Fatalf("plan new: %v", err)
	}
	s := readPlan(t, root, "fi")
	if !strings.Contains(s, "**Source:** idea:my-idea") {
		t.Errorf("fetched template idea-source rewrite failed:\n%s", s)
	}
	if strings.Contains(s, "**Source Feature:**") {
		t.Errorf("feature source line should be gone:\n%s", s)
	}
}

func TestPlanNew_TitleOwnerDefaults(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)
	t.Setenv("USER", "envuser")
	if _, _, err := runPlan(t, "new", "defaults-plan", "--feature", "f"); err != nil {
		t.Fatalf("plan new: %v", err)
	}
	s := readPlan(t, root, "defaults-plan")
	if !strings.Contains(s, "# Plan: Defaults Plan") {
		t.Errorf("title not defaulted from slug:\n%s", s)
	}
	if !strings.Contains(s, "**Owner:** envuser") {
		t.Errorf("owner not defaulted from $USER:\n%s", s)
	}
}

// AC: scaffolded-plan-is-lint-clean / ancestor-indexes-materialized — on a
// lint-clean project, plan new introduces no error-severity violations outside
// the scaffolded plan file (the plan file itself may carry P-002 for an
// unresolved source Feature, which the AC explicitly tolerates).
func TestPlanNew_LintCleanOutsideFile(t *testing.T) {
	root := setupLintCleanProject(t)
	if _, _, err := runPlan(t, "new", "clean-plan", "--feature", "nonexistent", "--project", root); err != nil {
		t.Fatalf("plan new: %v", err)
	}
	specSub := filepath.Join(root, "spec")
	violations, err := lint.Lint(lint.Options{SpecRoot: specSub})
	if err != nil {
		t.Fatal(err)
	}
	planRel := filepath.Join("plans", "clean-plan.md")
	for _, v := range violations {
		if v.Severity == "error" && v.File != planRel {
			t.Errorf("unexpected error-severity violation outside the plan file: %s:%d [%s] %s",
				v.File, v.Line, v.Rule, v.Message)
		}
	}
}

func TestPlanNew_ResolveSpecRootError(t *testing.T) {
	// A directory with no spec/ tree → FindSpecRepoRoot fails.
	dir := t.TempDir()
	withCwd(t, dir)
	_, _, err := runPlan(t, "new", "p", "--feature", "f")
	if err == nil {
		t.Fatal("expected error when no spec root can be resolved")
	}
}

func TestPlanNew_AncestorIndexError(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)
	// Make spec/ read-only so materializing spec/plans/README.md fails.
	specDir := filepath.Join(root, "spec")
	if err := os.Chmod(specDir, 0o555); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(specDir, 0o755) }()

	_, _, err := runPlan(t, "new", "p", "--feature", "f")
	if err == nil {
		t.Fatal("expected error materializing ancestor indexes")
	}
	if got := exitCodeOf(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d (Unexpected)", got, exitcode.Unexpected)
	}
}

func TestPlanNew_WriteError(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)
	// Pre-create both ancestor indexes so ensurePlanAncestorIndexes is a no-op,
	// then make spec/plans/ read-only so the plan-file write fails.
	plansDir := filepath.Join(root, "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFileT(t, filepath.Join(root, "spec", "README.md"), "# spec\n")
	writeFileT(t, filepath.Join(plansDir, "README.md"), "# plans\n")
	if err := os.Chmod(plansDir, 0o555); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(plansDir, 0o755) }()

	_, _, err := runPlan(t, "new", "p", "--feature", "f")
	if err == nil {
		t.Fatal("expected write error")
	}
	if got := exitCodeOf(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d (Unexpected)", got, exitcode.Unexpected)
	}
}

func TestPlanNew_ScaffoldError(t *testing.T) {
	// Force the embedded scaffolder (offline) to fail.
	orig := planScaffoldFn
	t.Cleanup(func() { planScaffoldFn = orig })
	planScaffoldFn = func(plan.ScaffoldOptions) ([]byte, error) {
		return nil, errors.New("forced scaffold failure")
	}
	root := setupSpecRoot(t)
	withCwd(t, root)
	_, _, err := runPlan(t, "new", "boom", "--feature", "f")
	if err == nil {
		t.Fatal("expected scaffold error")
	}
	if got := exitCodeOf(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d (Unexpected)", got, exitcode.Unexpected)
	}
}

func TestPlanNew_SyncIndexError(t *testing.T) {
	original := planSyncIndexFn
	planSyncIndexFn = func(string) (bool, error) { return false, errors.New("sync boom") }
	t.Cleanup(func() { planSyncIndexFn = original })
	root := setupSpecRoot(t)
	withCwd(t, root)
	_, _, err := runPlan(t, "new", "sync-fail", "--feature", "f")
	if got := exitCodeOf(err); got != exitcode.Unexpected {
		t.Fatalf("exit = %d, want %d; err=%v", got, exitcode.Unexpected, err)
	}
}
