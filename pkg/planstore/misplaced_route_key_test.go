package planstore

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

// AC: err-no-route-detectable — Resolve wraps ErrNoRoute (via %w) only for
// "nothing configured at all", so a caller can distinguish that shape from
// every other resolution failure via errors.Is.
func TestErrNoRoute_IsDetectable(t *testing.T) {
	source := t.TempDir()
	initRepo(t, source, "git@github.com:acme/app.git")
	t.Setenv("HOME", t.TempDir())

	_, err := Resolve(source, ReadOnly)
	if err == nil {
		t.Fatal("expected an error with no route configured")
	}
	if !errors.Is(err, ErrNoRoute) {
		t.Fatalf("errors.Is(err, ErrNoRoute) = false; err = %v", err)
	}
	if errors.Is(err, ErrRouteUnresolved) {
		t.Fatalf("no-route-at-all must not also match ErrRouteUnresolved: %v", err)
	}
	if !strings.Contains(err.Error(), "no plans repository is configured") {
		t.Fatalf("error message changed unexpectedly: %v", err)
	}
}

// AC: misplaced-plan-repos-in-committed-file — finding 5: plan_repos
// declared in the committed project file (wrong layer; it belongs in
// organization or user config) is a pointed error, not a silent fall-through
// to the generic no-route message. This is also NOT ErrNoRoute — a
// misplaced key means something WAS configured, just in the wrong place.
func TestResolve_MisplacedPlanReposInCommittedFile(t *testing.T) {
	source := t.TempDir()
	initRepo(t, source, "git@github.com:acme/app.git")
	t.Setenv("HOME", t.TempDir())
	write(t, filepath.Join(source, RepoConfigFile), "plan_repos:\n  acme/hub: [acme/app]\n")

	_, err := Resolve(source, ReadOnly)
	if err == nil {
		t.Fatal("expected an error for a misplaced plan_repos key")
	}
	if errors.Is(err, ErrNoRoute) {
		t.Fatalf("a misplaced key must not be reported as ErrNoRoute: %v", err)
	}
	if !errors.Is(err, ErrRouteUnresolved) {
		t.Fatalf("a misplaced key must be reported as ErrRouteUnresolved: %v", err)
	}
	if !strings.Contains(err.Error(), "plan_repos") || !strings.Contains(err.Error(), "plans_repo") {
		t.Fatalf("error should name plan_repos and point at plans_repo as the fix: %v", err)
	}
}

// AC: misplaced-plan-repos-in-local-file — same misplacement, in
// specscore.local.yaml instead of the committed file.
func TestResolve_MisplacedPlanReposInLocalFile(t *testing.T) {
	source := t.TempDir()
	initRepo(t, source, "git@github.com:acme/app.git")
	t.Setenv("HOME", t.TempDir())
	write(t, filepath.Join(source, LocalConfigFile), "plan_repos:\n  acme/hub: [acme/app]\n")

	_, err := Resolve(source, ReadOnly)
	if err == nil {
		t.Fatal("expected an error for a misplaced plan_repos key")
	}
	if !strings.Contains(err.Error(), "plan_repos") {
		t.Fatalf("error should name plan_repos: %v", err)
	}
}

// AC: misplaced-plans-repo-in-org-config — finding 5: plans_repo declared in
// organization config (wrong layer; it belongs in specscore.local.yaml or
// the committed project file) is a pointed error.
func TestResolve_MisplacedPlansRepoInOrgConfig(t *testing.T) {
	projects := t.TempDir()
	source := filepath.Join(projects, "acme", "app")
	initRepo(t, source, "git@github.com:acme/app.git")
	t.Setenv("HOME", t.TempDir())
	write(t, filepath.Join(projects, "acme", OrgConfigFile), "plans_repo: acme/hub\n")

	_, err := Resolve(source, ReadOnly)
	if err == nil {
		t.Fatal("expected an error for a misplaced plans_repo key")
	}
	if errors.Is(err, ErrNoRoute) {
		t.Fatalf("a misplaced key must not be reported as ErrNoRoute: %v", err)
	}
	if !strings.Contains(err.Error(), "plans_repo") || !strings.Contains(err.Error(), "plan_repos") {
		t.Fatalf("error should name plans_repo and point at plan_repos as the fix: %v", err)
	}
}

// AC: misplaced-plans-repo-in-user-config — same misplacement, in
// ~/.specscore.yaml instead of organization config.
func TestResolve_MisplacedPlansRepoInUserConfig(t *testing.T) {
	source := t.TempDir()
	initRepo(t, source, "git@github.com:acme/app.git")
	home := t.TempDir()
	t.Setenv("HOME", home)
	write(t, filepath.Join(home, UserConfigFile), "plans_repo: acme/hub\n")

	_, err := Resolve(source, ReadOnly)
	if err == nil {
		t.Fatal("expected an error for a misplaced plans_repo key")
	}
	if !strings.Contains(err.Error(), "plans_repo") {
		t.Fatalf("error should name plans_repo: %v", err)
	}
}

// AC: correctly-placed-route-not-shadowed — a correctly-placed route (here,
// a committed self-route plans_repo, the same shape specscore/specscore's
// own specscore.yaml uses) resolves normally and never reaches the
// misplaced-key diagnostic, even when an unrelated layer also happens to
// carry a stray key of the wrong kind elsewhere is not being tested here;
// this only proves the happy path is untouched by the new check.
func TestResolve_CorrectlyPlacedCommittedPlansRepoStillResolves(t *testing.T) {
	source := t.TempDir()
	initRepo(t, source, "git@github.com:specscore/specscore.git")
	t.Setenv("HOME", t.TempDir())
	write(t, filepath.Join(source, RepoConfigFile), "plans_repo: specscore/specscore\n")

	got, err := Resolve(source, ReadOnly)
	if err != nil {
		t.Fatalf("expected the committed self-route to resolve cleanly: %v", err)
	}
	if got.External {
		t.Fatalf("a self-route must resolve same-repo, not external: %#v", got)
	}
}
