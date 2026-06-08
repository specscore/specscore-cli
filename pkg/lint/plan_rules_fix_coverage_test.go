package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/plan"
)

func skipIfRoot(t *testing.T) {
	t.Helper()
	if os.Geteuid() == 0 {
		t.Skip("chmod-based error path requires a non-root user")
	}
}

// Covers Options.fixUnscoped and Options.fixRequested across all branches.
func TestOptions_fixRequested(t *testing.T) {
	if (Options{}).fixRequested("x") {
		t.Error("fix disabled must yield false")
	}
	if !(Options{Fix: true}).fixRequested("x") {
		t.Error("unscoped (no targets) must yield true")
	}
	if !(Options{Fix: true, FixTargets: []string{FixTargetAll}}).fixRequested("x") {
		t.Error("explicit all must yield true")
	}
	if !(Options{Fix: true, FixTargets: []string{"a", "b"}}).fixRequested("b") {
		t.Error("named match must yield true")
	}
	if (Options{Fix: true, FixTargets: []string{"a"}}).fixRequested("z") {
		t.Error("scoped, no match must yield false")
	}
}

// Covers the scoped branch of linter.fix(): the named target (no-source) runs
// while every other fixer is skipped as out of scope.
func TestLintWithResult_ScopedFixRunsOnlyNamedTarget(t *testing.T) {
	specRoot := filepath.Join(t.TempDir(), "spec")
	plansDir := filepath.Join(specRoot, "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	planPath := filepath.Join(plansDir, "p.md")
	body := "# Plan: P\n\n**Status:** Draft\n\n## Tasks\n\n### Task 1: Do\n\n**Status:** pending\n"
	if err := os.WriteFile(planPath, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}

	res, err := LintWithResult(Options{
		SpecRoot:   specRoot,
		Fix:        true,
		FixTargets: []string{FixTargetNoSource},
	})
	if err != nil {
		t.Fatalf("LintWithResult: %v", err)
	}
	data, _ := os.ReadFile(planPath)
	if !strings.Contains(string(data), "**Source:** none") {
		t.Errorf("scoped --fix=no-source should repair the plan:\n%s", data)
	}
	if len(res.Fixed) == 0 {
		t.Error("expected the plan to be reported as fixed")
	}
}

// Covers planRulesChecker.fix() early returns and skip branches: no plans dir,
// subdirectory entries, and README / non-.md files.
func TestPlanRulesFix_SkipBranches(t *testing.T) {
	c := newPlanRulesChecker()
	c.fixNoSource = true

	// No plans dir at all -> early return nil.
	if err := c.fix(t.TempDir()); err != nil {
		t.Fatalf("no plans dir: %v", err)
	}

	root := t.TempDir()
	plansDir := filepath.Join(root, "plans")
	if err := os.MkdirAll(filepath.Join(plansDir, "subdir"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWrite(t, filepath.Join(plansDir, "README.md"), "# index\n")
	mustWrite(t, filepath.Join(plansDir, "notes.txt"), "not a plan\n")
	if err := c.fix(root); err != nil {
		t.Fatalf("skip branches: %v", err)
	}
}

// Covers the os.ReadDir error path in planRulesChecker.fix().
func TestPlanRulesFix_ReadDirError(t *testing.T) {
	skipIfRoot(t)
	root := t.TempDir()
	plansDir := filepath.Join(root, "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(plansDir, 0o000); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(plansDir, 0o755) }()

	c := newPlanRulesChecker()
	c.fixNoSource = true
	if err := c.fix(root); err == nil {
		t.Error("expected an error reading an unreadable plans dir")
	}
}

// Covers the plan.Parse error path in planRulesChecker.fix() (unreadable file).
func TestPlanRulesFix_ParseError(t *testing.T) {
	skipIfRoot(t)
	root := t.TempDir()
	plansDir := filepath.Join(root, "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	bad := filepath.Join(plansDir, "bad.md")
	mustWrite(t, bad, "# Plan: Bad\n")
	if err := os.Chmod(bad, 0o000); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(bad, 0o644) }()

	c := newPlanRulesChecker()
	c.fixNoSource = true
	if err := c.fix(root); err == nil {
		t.Error("expected a parse error for an unreadable plan file")
	}
}

// Covers the insertSourceNone error path inside planRulesChecker.fix(): a
// sourceless plan that is read-only fails when the fixer rewrites it.
func TestPlanRulesFix_InsertWriteError(t *testing.T) {
	skipIfRoot(t)
	root := t.TempDir()
	plansDir := filepath.Join(root, "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(plansDir, "p.md")
	mustWrite(t, p, "# Plan: P\n\n**Status:** Draft\n\n## Tasks\n\n### Task 1: Do\n\n**Status:** pending\n")
	if err := os.Chmod(p, 0o444); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(p, 0o644) }()

	c := newPlanRulesChecker()
	c.fixNoSource = true
	if err := c.fix(root); err == nil {
		t.Error("expected a write error rewriting a read-only plan")
	}
}

// Covers insertSourceNone's ReadFile error path and the anchor-overflow fallback.
func TestInsertSourceNone_EdgeCases(t *testing.T) {
	// ReadFile error: a path that does not exist.
	if err := insertSourceNone(filepath.Join(t.TempDir(), "missing.md"), &plan.Plan{}); err == nil {
		t.Error("expected a read error for a missing file")
	}

	// Anchor past end of file -> fallback to len(lines); still writes cleanly.
	f := filepath.Join(t.TempDir(), "p.md")
	mustWrite(t, f, "# Plan: P\n")
	if err := insertSourceNone(f, &plan.Plan{TitleLine: 99}); err != nil {
		t.Fatalf("anchor fallback: %v", err)
	}
	data, _ := os.ReadFile(f)
	if !strings.Contains(string(data), "**Source:** none") {
		t.Errorf("expected source line appended:\n%s", data)
	}
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
