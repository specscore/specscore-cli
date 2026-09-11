package cli

// TestSpecLint_PlanOwnedBrokenRouteGuard is finding 2's guard test: it runs
// spec lint's FULL checker registry (pkg/lint.newLinter registers every
// checker unconditionally; nothing here hand-picks a subset via --rules)
// against a fully lint-clean project (setupLintCleanProject — the same
// zero-violation fixture dozens of other tests already rely on) with Plan
// routing configured but broken, PLUS a stale local spec/plans/README.md
// that is simultaneously missing its Open Questions section (oq-section),
// its required `format:` frontmatter field (format-field), and its
// adherence footer (adherence-footer) — exactly the composition finding 2
// named. Before this PR's fix, oq-section and format-field walked the
// physical spec/plans tree unconditionally (ignoring routeBroken/plansDir
// entirely), so this stale file would have produced two extra findings
// alongside plan-route-unresolved.
//
// The guard is deliberately NOT scoped to a curated list of "plan-owned"
// rule names: a future checker that reads spec/plans without being wired
// through newLinter's registerPlanOwned (pkg/lint/linter.go) — the ONE
// place that now decides "Plan-owned" — fails this test the moment it
// produces a stray finding against the stale file, regardless of how it got
// registered.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/projectdef"
)

// staleUnadornedPlansIndex deliberately carries none of: an Open Questions
// section, a `format:` frontmatter field, or an adherence footer.
const staleUnadornedPlansIndex = "# Plans\n\nStale local plans index.\n"

func setupPlanOwnedGuardProject(t *testing.T) string {
	t.Helper()
	root := setupLintCleanProject(t)
	t.Setenv("HOME", t.TempDir())
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"remote", "add", "origin", "git@github.com:acme/source.git"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	// Route Plans to an external repository with no matching repo_checkouts
	// entry anywhere — planstore.ErrRouteUnresolved, not ErrNoRoute.
	// setupLintCleanProject's own specscore.yaml (via
	// projectdef.WriteSpecConfig(root, projectdef.SpecConfig{})) marshals
	// the zero-value SpecConfig as a bare top-level flow mapping "{}";
	// appending a plans_repo line after that flow-mapping document is
	// silently ignored by yaml.Unmarshal (confirmed empirically — it is NOT
	// a YAML error, just dropped), so this replaces the file outright with
	// a single valid block-style document instead.
	cfgPath := filepath.Join(root, "specscore.yaml")
	config := projectdef.SchemaHeader + "\n\nproject:\n  host: github.com\n  org: acme\n  repo: source\nplans_repo: acme/plans-hub\n"
	if err := os.WriteFile(cfgPath, []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}

	planDir := filepath.Join(root, "spec", "plans")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(planDir, "README.md"), []byte(staleUnadornedPlansIndex), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestSpecLint_PlanOwnedBrokenRouteGuard_ReadOnly(t *testing.T) {
	root := setupPlanOwnedGuardProject(t)
	planPath := filepath.Join(root, "spec", "plans", "README.md")
	before, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}

	stdout, _, err := runSpecLintCmd(t, "--project", root)
	if err == nil {
		t.Fatalf("expected the route-error finding, got nil error")
	}
	if got := exitCodeOf(err); got != exitcode.Conflict {
		t.Fatalf("exit code = %d, want %d (Conflict); err = %v", got, exitcode.Conflict, err)
	}
	assertOnlyPlanRouteUnresolvedFinding(t, stdout)

	after, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("read-only lint must never write to spec/plans once routing is broken")
	}
}

func TestSpecLint_PlanOwnedBrokenRouteGuard_Fix(t *testing.T) {
	root := setupPlanOwnedGuardProject(t)
	planPath := filepath.Join(root, "spec", "plans", "README.md")
	before, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}

	_, stderr, err := runSpecLintCmd(t, "--project", root, "--fix")
	if err == nil {
		t.Fatalf("expected --fix to fail closed, got nil error")
	}
	if got := exitCodeOf(err); got != exitcode.InvalidState {
		t.Fatalf("exit code = %d, want %d (InvalidState); err = %v", got, exitcode.InvalidState, err)
	}
	if !strings.Contains(err.Error()+stderr, "acme/plans-hub") {
		t.Errorf("error should name the unresolved route; err=%v stderr=%q", err, stderr)
	}

	after, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("--fix must leave the stale plan file byte-identical once routing is broken")
	}
}

// assertOnlyPlanRouteUnresolvedFinding fails the test unless the text-format
// `spec lint` output names exactly one rule, plan-route-unresolved. It scans
// every "[error]"/"[warning]"/"[info]" severity marker (the text renderer's
// per-violation line shape) rather than assuming a specific line count, so
// it stays correct if the renderer's surrounding formatting changes.
func assertOnlyPlanRouteUnresolvedFinding(t *testing.T, stdout string) {
	t.Helper()
	foundRouteError := false
	for _, line := range strings.Split(stdout, "\n") {
		if !strings.Contains(line, "[error]") && !strings.Contains(line, "[warning]") && !strings.Contains(line, "[info]") {
			continue
		}
		if strings.Contains(line, "plan-route-unresolved") {
			foundRouteError = true
			continue
		}
		t.Errorf("unexpected finding once Plan routing is broken (every Plan-owned checker must be a no-op): %s", line)
	}
	if !foundRouteError {
		t.Errorf("expected a plan-route-unresolved finding; got: %q", stdout)
	}
}
