package cli

// This file reproduces the three rows of the PR #199 adversarial
// re-review's finding 1: a config layer that EXISTS but fails to read or
// parse (malformed YAML) must be classified exactly like a route that
// resolved to something broken (planstore.ErrRouteUnresolved) — never
// silently treated as "nothing configured" and fall back to the local
// spec/plans tree, which may be exactly the stale artifact routing was
// configured to route away from.
//
//   - Row 1: a malformed user ~/.specscore.yaml. No route is committed
//     anywhere else, so before the fix this looked exactly like "nothing
//     configured at all".
//   - Row 2: a malformed organization .specscore.yaml (anchored beside the
//     canonical clone), while the project's OWN committed specscore.yaml
//     carries a perfectly valid plans_repo. That valid route must never
//     even be reached — the org layer is read first.
//   - Row 3: a malformed committed specscore.yaml itself.
//
// Each row is exercised three ways: read-only `spec lint`, `spec lint
// --fix`, and `feature info` — matching the brief's required
// post-fix behaviour for all three.

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/planstore"
	"github.com/specscore/specscore-cli/pkg/projectdef"
)

// setupSpecLintMalformedLayerProject builds the same fixture shape as
// setupBrokenPlanRouteProject (spec_lint_plan_route_test.go) — a git repo
// with a stale local spec/plans/stale/README.md carrying a P-002 violation —
// with an isolated HOME. committedPlansRepo controls whether the project's
// OWN committed specscore.yaml carries `plans_repo: acme/plans-hub` (row 2
// needs it to prove that valid route is never reached; rows 1 and 3 don't).
// placeMalformedLayer writes (or, for row 3, overwrites) the one config
// layer this row's malformed YAML lives in.
func setupSpecLintMalformedLayerProject(t *testing.T, committedPlansRepo bool, placeMalformedLayer func(root, home string)) string {
	t.Helper()
	root := t.TempDir()
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"remote", "add", "origin", "git@github.com:acme/source.git"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	config := projectdef.SchemaHeader + "\n\nproject:\n  host: github.com\n  org: acme\n  repo: source\n"
	if committedPlansRepo {
		config += "plans_repo: acme/plans-hub\n"
	}
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
	placeMalformedLayer(root, home)
	return root
}

// setupFeatureInfoMalformedLayerProject mirrors
// setupFeatureSpecWithBrokenPlanRoute (feature_info_plan_route_test.go) but
// for a malformed (rather than missing-checkout) layer — see
// setupSpecLintMalformedLayerProject's doc comment for the row semantics.
func setupFeatureInfoMalformedLayerProject(t *testing.T, committedPlansRepo bool, placeMalformedLayer func(root, home string)) string {
	t.Helper()
	root := setupFeatureSpec(t, "Approved")
	home := t.TempDir()
	t.Setenv("HOME", home)
	for _, args := range [][]string{
		{"init", "-b", "main"},
		{"remote", "add", "origin", "git@github.com:acme/source.git"},
	} {
		if out, err := exec.Command("git", append([]string{"-C", root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	config := projectdef.SchemaHeader + "\n\nproject:\n  host: github.com\n  org: acme\n  repo: source\n"
	if committedPlansRepo {
		config += "plans_repo: acme/plans-hub\n"
	}
	if err := os.WriteFile(filepath.Join(root, "specscore.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	planDir := filepath.Join(root, "spec", "plans", "auth-rollout")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatal(err)
	}
	planBody := "# Plan: Auth Rollout\n\n**Status:** Draft\n**Features:**\n- [Auth](../../features/auth/README.md)\n\n## Tasks\n\n### Task 1: Do\n\n**Status:** planning\n\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(filepath.Join(planDir, "README.md"), []byte(planBody), 0o644); err != nil {
		t.Fatal(err)
	}
	placeMalformedLayer(root, home)
	return root
}

// --- Row 1: malformed user ~/.specscore.yaml --------------------------------

func writeMalformedUserLayer(t *testing.T) func(root, home string) {
	return func(root, home string) {
		t.Helper()
		body := "plan_repos:\n  acme/plans-hub: [acme/source\n"
		if err := os.WriteFile(filepath.Join(home, planstore.UserConfigFile), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSpecLint_MalformedUserLayer_SkipsLocalPlansAndReportsRouteError(t *testing.T) {
	root := setupSpecLintMalformedLayerProject(t, false, writeMalformedUserLayer(t))
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
		t.Fatalf("exit code = %d, want %d (Conflict); err = %v", got, exitcode.Conflict, err)
	}
	if !strings.Contains(stdout, "plan-route-unresolved") {
		t.Errorf("stdout missing plan-route-unresolved finding; got: %q", stdout)
	}
	if strings.Contains(stdout, "P-002") {
		t.Errorf("stdout must not report the stale local plan's P-002 violation once the user layer is malformed; got: %q", stdout)
	}
	if strings.Contains(stdout, "not-a-real-source") {
		t.Errorf("stdout must not mention the stale local plan's content at all; got: %q", stdout)
	}

	after, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("read-only lint must never write to spec/plans once the user layer is malformed")
	}
}

func TestSpecLint_MalformedUserLayer_FixFailsClosedBeforeAnyWrite(t *testing.T) {
	root := setupSpecLintMalformedLayerProject(t, false, writeMalformedUserLayer(t))
	before := snapshotTree(t, root)

	_, stderr, err := runSpecLintCmd(t, "--project", root, "--fix")
	if err == nil {
		t.Fatalf("expected --fix to fail closed, got nil error")
	}
	if got := exitCodeOf(err); got != exitcode.InvalidState {
		t.Fatalf("exit code = %d, want %d (InvalidState); err = %v", got, exitcode.InvalidState, err)
	}
	combined := err.Error() + stderr
	if !strings.Contains(combined, "parse config") && !strings.Contains(combined, "specscore.yaml") {
		t.Errorf("error should name the broken user layer; err=%v stderr=%q", err, stderr)
	}

	after := snapshotTree(t, root)
	assertTreeUnchanged(t, before, after)
}

func TestFeatureInfo_MalformedUserLayer_ReportsPlansUnavailable(t *testing.T) {
	setupFeatureInfoMalformedLayerProject(t, false, writeMalformedUserLayer(t))

	out, stderr, err := runFeature(t, "info", "auth")
	if err != nil {
		t.Fatalf("feature info must still succeed when only Plan routing is broken: %v", err)
	}
	if strings.Contains(out, "auth-rollout") {
		t.Errorf("stdout must not surface the stale local plan's back-reference; got: %q", out)
	}
	if strings.Contains(out, "plans:") {
		t.Errorf("stdout must not carry a plans back-reference key when the user layer is malformed; got: %q", out)
	}
	if !strings.Contains(stderr, "Plan back-references unavailable") {
		t.Errorf("stderr should report Plan back-references unavailable; got: %q", stderr)
	}
}

// --- Row 2: malformed organization .specscore.yaml, valid committed route --

func writeMalformedOrgLayer(t *testing.T) func(root, home string) {
	return func(root, _ string) {
		t.Helper()
		orgPath, err := planstore.OrgConfigPath(root)
		if err != nil {
			t.Fatalf("computing org config path: %v", err)
		}
		body := "plan_repos:\n  acme/plans-hub: [acme/source\n"
		if err := os.WriteFile(orgPath, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSpecLint_MalformedOrgLayerWithValidCommittedRoute_SkipsLocalPlansAndReportsRouteError(t *testing.T) {
	root := setupSpecLintMalformedLayerProject(t, true, writeMalformedOrgLayer(t))
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
		t.Fatalf("exit code = %d, want %d (Conflict); err = %v", got, exitcode.Conflict, err)
	}
	if !strings.Contains(stdout, "plan-route-unresolved") {
		t.Errorf("stdout missing plan-route-unresolved finding; got: %q", stdout)
	}
	if strings.Contains(stdout, "P-002") {
		t.Errorf("stdout must not report the stale local plan's P-002 violation; the project's own valid committed plans_repo must never even be reached once the org layer ahead of it is malformed; got: %q", stdout)
	}

	after, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("read-only lint must never write to spec/plans once the org layer is malformed")
	}
}

func TestSpecLint_MalformedOrgLayerWithValidCommittedRoute_FixFailsClosedBeforeAnyWrite(t *testing.T) {
	root := setupSpecLintMalformedLayerProject(t, true, writeMalformedOrgLayer(t))
	before := snapshotTree(t, root)

	_, stderr, err := runSpecLintCmd(t, "--project", root, "--fix")
	if err == nil {
		t.Fatalf("expected --fix to fail closed, got nil error")
	}
	if got := exitCodeOf(err); got != exitcode.InvalidState {
		t.Fatalf("exit code = %d, want %d (InvalidState); err = %v", got, exitcode.InvalidState, err)
	}
	_ = stderr

	after := snapshotTree(t, root)
	assertTreeUnchanged(t, before, after)
}

func TestFeatureInfo_MalformedOrgLayerWithValidCommittedRoute_ReportsPlansUnavailable(t *testing.T) {
	setupFeatureInfoMalformedLayerProject(t, true, writeMalformedOrgLayer(t))

	out, stderr, err := runFeature(t, "info", "auth")
	if err != nil {
		t.Fatalf("feature info must still succeed when only Plan routing is broken: %v", err)
	}
	if strings.Contains(out, "auth-rollout") {
		t.Errorf("stdout must not surface the stale local plan's back-reference (the project's own valid committed plans_repo must never be reached); got: %q", out)
	}
	if strings.Contains(out, "plans:") {
		t.Errorf("stdout must not carry a plans back-reference key when the org layer is malformed; got: %q", out)
	}
	if !strings.Contains(stderr, "Plan back-references unavailable") {
		t.Errorf("stderr should report Plan back-references unavailable; got: %q", stderr)
	}
}

// --- Row 3: malformed committed specscore.yaml ------------------------------

func writeMalformedCommittedLayer(t *testing.T) func(root, home string) {
	return func(root, _ string) {
		t.Helper()
		body := "plans_repo: [unterminated\n"
		if err := os.WriteFile(filepath.Join(root, "specscore.yaml"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestSpecLint_MalformedCommittedLayer_SkipsLocalPlansAndReportsRouteError(t *testing.T) {
	root := setupSpecLintMalformedLayerProject(t, false, writeMalformedCommittedLayer(t))
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
		t.Fatalf("exit code = %d, want %d (Conflict); err = %v", got, exitcode.Conflict, err)
	}
	if !strings.Contains(stdout, "plan-route-unresolved") {
		t.Errorf("stdout missing plan-route-unresolved finding; got: %q", stdout)
	}
	if strings.Contains(stdout, "P-002") {
		t.Errorf("stdout must not report the stale local plan's P-002 violation once the committed layer is malformed; got: %q", stdout)
	}

	after, err := os.ReadFile(planPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Error("read-only lint must never write to spec/plans once the committed layer is malformed")
	}
}

func TestSpecLint_MalformedCommittedLayer_FixFailsClosedBeforeAnyWrite(t *testing.T) {
	root := setupSpecLintMalformedLayerProject(t, false, writeMalformedCommittedLayer(t))
	before := snapshotTree(t, root)

	_, _, err := runSpecLintCmd(t, "--project", root, "--fix")
	if err == nil {
		t.Fatalf("expected --fix to fail closed, got nil error")
	}
	if got := exitCodeOf(err); got != exitcode.InvalidState {
		t.Fatalf("exit code = %d, want %d (InvalidState); err = %v", got, exitcode.InvalidState, err)
	}

	after := snapshotTree(t, root)
	assertTreeUnchanged(t, before, after)
}

func TestFeatureInfo_MalformedCommittedLayer_ReportsPlansUnavailable(t *testing.T) {
	setupFeatureInfoMalformedLayerProject(t, false, writeMalformedCommittedLayer(t))

	out, stderr, err := runFeature(t, "info", "auth")
	if err != nil {
		t.Fatalf("feature info must still succeed when only Plan routing is broken: %v", err)
	}
	if strings.Contains(out, "auth-rollout") {
		t.Errorf("stdout must not surface the stale local plan's back-reference; got: %q", out)
	}
	if strings.Contains(out, "plans:") {
		t.Errorf("stdout must not carry a plans back-reference key when the committed layer is malformed; got: %q", out)
	}
	if !strings.Contains(stderr, "Plan back-references unavailable") {
		t.Errorf("stderr should report Plan back-references unavailable; got: %q", stderr)
	}
}

// --- shared snapshot helpers -------------------------------------------------

// snapshotTree captures every non-.git file's content under root, keyed by
// its root-relative path, for a before/after --fix-must-not-write assertion.
func snapshotTree(t *testing.T, root string) map[string]string {
	t.Helper()
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

func assertTreeUnchanged(t *testing.T, before, after map[string]string) {
	t.Helper()
	if len(before) != len(after) {
		t.Fatalf("file count changed under a failed --fix: before=%d after=%d", len(before), len(after))
	}
	for rel, content := range before {
		if after[rel] != content {
			t.Errorf("file %s changed under a failed --fix", rel)
		}
	}
}
