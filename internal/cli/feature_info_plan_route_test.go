package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/projectdef"
)

// setupFeatureSpecWithBrokenPlanRoute layers a broken-route configuration
// (committed plans_repo, no repo_checkouts anywhere, isolated HOME) onto the
// lint-clean feature fixture setupFeatureSpec builds, plus a stale local
// Plan that links back to the "auth" feature via its **Features:** section
// (pkg/feature.planReferencesFeature's match convention) — the exact
// back-reference finding 2 says must NOT surface from a stale local tree
// once routing is configured but broken.
func setupFeatureSpecWithBrokenPlanRoute(t *testing.T) string {
	t.Helper()
	root := setupFeatureSpec(t, "Approved")
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
	planDir := filepath.Join(root, "spec", "plans", "auth-rollout")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatal(err)
	}
	planBody := "# Plan: Auth Rollout\n\n**Status:** Draft\n**Features:**\n- [Auth](../../features/auth/README.md)\n\n## Tasks\n\n### Task 1: Do\n\n**Status:** planning\n\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(filepath.Join(planDir, "README.md"), []byte(planBody), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// AC: broken-route-plans-unavailable — finding 2: `feature info` against a
// repo with a committed plans_repo and no matching repo_checkouts must
// report the Plans back-reference as unavailable (never populated from the
// stale local tree) and must report the resolution error on stderr, while
// the rest of the feature info still succeeds (Feature operations are
// independent of Plan routing).
func TestFeatureInfo_PlanRouteBroken_ReportsPlansUnavailable(t *testing.T) {
	setupFeatureSpecWithBrokenPlanRoute(t)

	out, stderr, err := runFeature(t, "info", "auth")
	if err != nil {
		t.Fatalf("feature info must still succeed when only Plan routing is broken: %v", err)
	}
	if strings.Contains(out, "auth-rollout") {
		t.Errorf("stdout must not surface the stale local plan's back-reference; got: %q", out)
	}
	if strings.Contains(out, "plans:") {
		t.Errorf("stdout must not carry a plans back-reference key when routing is broken (Plans field is omitempty); got: %q", out)
	}
	if !strings.Contains(stderr, "Plan back-references unavailable") {
		t.Errorf("stderr should report Plan back-references unavailable; got: %q", stderr)
	}
	if !strings.Contains(stderr, "acme/plans-hub") {
		t.Errorf("stderr should name the unresolved route acme/plans-hub; got: %q", stderr)
	}
}

// AC: no-route-plans-default — with no plans_repo/plan_repos configured at
// all, `feature info` keeps searching the local spec/plans tree exactly as
// before Plan routing existed: the linked plan surfaces normally and no
// stderr noise is printed.
func TestFeatureInfo_NoRouteConfigured_KeepsLocalPlansDefault(t *testing.T) {
	root := setupFeatureSpec(t, "Approved")
	planDir := filepath.Join(root, "spec", "plans", "auth-rollout")
	if err := os.MkdirAll(planDir, 0o755); err != nil {
		t.Fatal(err)
	}
	planBody := "# Plan: Auth Rollout\n\n**Status:** Draft\n**Features:**\n- [Auth](../../features/auth/README.md)\n\n## Tasks\n\n### Task 1: Do\n\n**Status:** planning\n\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(filepath.Join(planDir, "README.md"), []byte(planBody), 0o644); err != nil {
		t.Fatal(err)
	}

	out, stderr, err := runFeature(t, "info", "auth")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "auth-rollout") {
		t.Errorf("expected the local plan back-reference under the no-route default; got: %q", out)
	}
	if strings.Contains(stderr, "Plan back-references unavailable") {
		t.Errorf("no-route default must not print the unavailable-back-references warning; got: %q", stderr)
	}
}
