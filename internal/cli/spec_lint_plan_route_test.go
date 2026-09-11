package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/projectdef"
)

// stalePlanWithBadSourceCLI is a Plan artifact carrying a P-002 violation
// (unrecognized **Source:** value) — the reviewer's reproduction of finding
// 1: a stale local spec/plans artifact left behind once Plan routing moved
// authoritative storage elsewhere. It must be linted under the same-repo
// default and never linted once routing is configured but broken.
const stalePlanWithBadSourceCLI = "# Plan: Stale\n\n**Status:** Draft\n**Source:** not-a-real-source\n\n## Tasks\n\n### Task 1: Do\n\n**Status:** planning\n\n## Open Questions\n\nNone at this time.\n"

// setupBrokenPlanRouteProject configures root as a git repo whose committed
// specscore.yaml routes Plans to an external repository (acme/plans-hub)
// with NO repo_checkouts entry anywhere — the exact "route configured but
// unresolved" shape finding 1 covers (planstore.ErrRouteUnresolved, not
// ErrNoRoute) — plus a stale local spec/plans/stale/README.md carrying a
// violation that must never surface once routing is configured. HOME is
// isolated so no real ~/.specscore.yaml can supply the missing checkout.
func setupBrokenPlanRouteProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	t.Setenv("HOME", t.TempDir())
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"remote", "add", "origin", "git@github.com:acme/source.git"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	config := projectdef.SchemaHeader + "\n\nproject:\n  host: github.com\n  org: acme\n  repo: source\nplans_repo: acme/plans-hub\n"
	if err := os.WriteFile(filepath.Join(root, "specscore.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "spec", "features"), 0o755); err != nil {
		t.Fatal(err)
	}
	featuresReadme := "# Features\n\n## Index\n\n| Feature | Status | Description |\n|---------|--------|-------------|\n\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(filepath.Join(root, "spec", "features", "README.md"), []byte(featuresReadme), 0o644); err != nil {
		t.Fatal(err)
	}
	specReadme := "# Specifications\n\nTest tree.\n\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(filepath.Join(root, "spec", "README.md"), []byte(specReadme), 0o644); err != nil {
		t.Fatal(err)
	}
	planDir := filepath.Join(root, "spec", "plans", "stale")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(planDir, "README.md"), []byte(stalePlanWithBadSourceCLI), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// AC: broken-route-skips-local-plans — finding 1's core reproduction:
// read-only `spec lint` against a repo with a committed plans_repo and no
// matching repo_checkouts must report the route error and must NOT report
// the stale local plan's P-002 violation, and must not touch the file.
func TestSpecLint_PlanRouteBroken_SkipsLocalPlansAndReportsRouteError(t *testing.T) {
	root := setupBrokenPlanRouteProject(t)
	planPath := filepath.Join(root, "spec", "plans", "stale", "README.md")
	before, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}

	stdout, _, err := runSpecLintCmd(t, "--project", root)
	if err == nil {
		t.Fatalf("expected lint to report the route-error violation, got nil error")
	}
	if got := exitCodeOf(err); got != exitcode.Conflict {
		t.Fatalf("exit code = %d, want %d (Conflict, violations found); err = %v", got, exitcode.Conflict, err)
	}
	if !strings.Contains(stdout, "plan-route-unresolved") {
		t.Errorf("stdout missing plan-route-unresolved finding; got: %q", stdout)
	}
	if !strings.Contains(stdout, "acme/plans-hub") {
		t.Errorf("stdout should name the unresolved route acme/plans-hub; got: %q", stdout)
	}
	if strings.Contains(stdout, "P-002") {
		t.Errorf("stdout must not report the stale local plan's P-002 violation once routing is broken; got: %q", stdout)
	}
	if strings.Contains(stdout, "not-a-real-source") {
		t.Errorf("stdout must not mention the stale local plan's content at all; got: %q", stdout)
	}

	after, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("read-only lint must never write to spec/plans once routing is broken")
	}
}

// AC: broken-route-fix-fails-closed — `spec lint --fix` against the same
// broken-route project must exit non-zero before writing anything, naming
// the route and how to fix it, and must leave every file byte-identical.
func TestSpecLint_PlanRouteBroken_FixFailsClosedBeforeAnyWrite(t *testing.T) {
	root := setupBrokenPlanRouteProject(t)
	snapshot := func() map[string]string {
		files := map[string]string{}
		_ = filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				return err
			}
			if info.IsDir() {
				return nil
			}
			if strings.Contains(filepath.ToSlash(path), "/.git/") {
				return nil
			}
			b, rerr := os.ReadFile(path)
			if rerr != nil {
				return rerr
			}
			rel, _ := filepath.Rel(root, path)
			files[rel] = string(b)
			return nil
		})
		return files
	}
	before := snapshot()

	_, stderr, err := runSpecLintCmd(t, "--project", root, "--fix")
	if err == nil {
		t.Fatalf("expected --fix to fail closed, got nil error")
	}
	if got := exitCodeOf(err); got != exitcode.InvalidState {
		t.Fatalf("exit code = %d, want %d (InvalidState); err = %v", got, exitcode.InvalidState, err)
	}
	combined := err.Error() + stderr
	if !strings.Contains(combined, "acme/plans-hub") {
		t.Errorf("error should name the unresolved route; err=%v stderr=%q", err, stderr)
	}
	if !strings.Contains(combined, "repo_checkouts") {
		t.Errorf("error should point at repo_checkouts as the fix; err=%v", err)
	}

	after := snapshot()
	if len(before) != len(after) {
		t.Fatalf("file count changed under a failed --fix: before=%d after=%d", len(before), len(after))
	}
	for rel, content := range before {
		if after[rel] != content {
			t.Errorf("file %s changed under a failed --fix", rel)
		}
	}
}

// AC: no-route-keeps-local-default — with no plans_repo/plan_repos
// configured anywhere (planstore.ErrNoRoute), spec lint keeps linting the
// local spec/plans tree exactly as it did before Plan routing existed: the
// stale plan's P-002 violation surfaces normally, and no plan-route-unresolved
// finding appears.
func TestSpecLint_NoRouteConfigured_KeepsLocalPlansDefault(t *testing.T) {
	root := t.TempDir()
	writeValidSpecscoreYAML(t, root) // no plans_repo/plan_repos anywhere
	if err := os.MkdirAll(filepath.Join(root, "spec", "features"), 0o755); err != nil {
		t.Fatal(err)
	}
	featuresReadme := "# Features\n\n## Index\n\n| Feature | Status | Description |\n|---------|--------|-------------|\n\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(filepath.Join(root, "spec", "features", "README.md"), []byte(featuresReadme), 0o644); err != nil {
		t.Fatal(err)
	}
	specReadme := "# Specifications\n\nTest tree.\n\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(filepath.Join(root, "spec", "README.md"), []byte(specReadme), 0o644); err != nil {
		t.Fatal(err)
	}
	planDir := filepath.Join(root, "spec", "plans", "stale")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(planDir, "README.md"), []byte(stalePlanWithBadSourceCLI), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := runSpecLintCmd(t, "--project", root)
	if err == nil {
		t.Fatalf("expected the local P-002 violation to be reported under the no-route default")
	}
	if !strings.Contains(stdout, "P-002") {
		t.Errorf("expected local spec/plans P-002 violation under the no-route default; got %q", stdout)
	}
	if strings.Contains(stdout, "plan-route-unresolved") {
		t.Errorf("no-route default must not emit plan-route-unresolved; got %q", stdout)
	}
}

// AC: resolved-route-lints-normally — a self-route that resolves cleanly
// (committed plans_repo naming this repository itself) lints the resolved
// namespace exactly as before this finding — the stale plan's violation
// still surfaces (it IS the resolved namespace here), and no
// plan-route-unresolved finding appears.
func TestSpecLint_ResolvedRoute_LintsResolvedNamespaceNormally(t *testing.T) {
	root := t.TempDir()
	configureSameRepoPlans(t, root)
	if err := os.MkdirAll(filepath.Join(root, "spec", "features"), 0o755); err != nil {
		t.Fatal(err)
	}
	featuresReadme := "# Features\n\n## Index\n\n| Feature | Status | Description |\n|---------|--------|-------------|\n\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(filepath.Join(root, "spec", "features", "README.md"), []byte(featuresReadme), 0o644); err != nil {
		t.Fatal(err)
	}
	specReadme := "# Specifications\n\nTest tree.\n\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(filepath.Join(root, "spec", "README.md"), []byte(specReadme), 0o644); err != nil {
		t.Fatal(err)
	}
	planDir := filepath.Join(root, "spec", "plans", "stale")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(planDir, "README.md"), []byte(stalePlanWithBadSourceCLI), 0o644); err != nil {
		t.Fatal(err)
	}

	stdout, _, err := runSpecLintCmd(t, "--project", root)
	if err == nil {
		t.Fatalf("expected the resolved-namespace P-002 violation to be reported")
	}
	if !strings.Contains(stdout, "P-002") {
		t.Errorf("expected resolved same-repo spec/plans P-002 violation; got %q", stdout)
	}
	if strings.Contains(stdout, "plan-route-unresolved") {
		t.Errorf("a resolved route must not emit plan-route-unresolved; got %q", stdout)
	}
}
