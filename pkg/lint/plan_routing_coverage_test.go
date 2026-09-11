package lint

// This file closes out statement-coverage gaps left by the Plan repository
// routing feature (external plansDir support: effectivePlansDir, the
// plansDir-aware adherence-footer walkers, plan-index-sync's fix()
// tolerance for a not-yet-materialized index, and plan-rules' fixers). Each
// test targets one specific branch resolve_test.go/lint_test.go's existing
// behavioral focus didn't already happen to exercise.
//
// Three near-identical "re-parse under lock" error branches in
// plan_rules.go (fixNoSourceLines, fixLegacyTaskStatusesInDir, fixP007InDir)
// share the planFixParseBytesFn seam (an injectable wrapper over
// plan.ParseBytes) below: plan.Discover already parses each file
// successfully once before these fixers re-parse it under
// TransformArtifact's lock, so in real operation that re-parse can only
// fail on a genuine concurrent rewrite — not something a deterministic test
// should force by racing. The seam lets each branch still get a
// deterministic test.

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/gofrs/flock"
	"github.com/specscore/specscore-cli/pkg/plan"
)

// --- plan_path.go / plan_index.go ------------------------------------------

func TestEffectivePlansDirOverride(t *testing.T) {
	if got := effectivePlansDir("/spec", "/external/plans"); got != "/external/plans" {
		t.Fatalf("effectivePlansDir override = %q, want /external/plans", got)
	}
}

func TestPlanIndexCheckerFixToleratesMissingIndexFile(t *testing.T) {
	plansDir := t.TempDir()
	// plansDir exists but has no README.md yet — plan new materializes it
	// separately; a fixer running before that must not hard-fail.
	c := newPlanIndexChecker(plansDir)
	if err := c.fix("/unused-spec-root"); err != nil {
		t.Fatalf("fix() = %v, want nil for a not-yet-materialized index", err)
	}
}

// --- readme_exists.go: external plansDir walk -------------------------------

func TestReadmeExistsCheckerPlansDirWalkStatError(t *testing.T) {
	specRoot := t.TempDir()
	writeFile(t, filepath.Join(specRoot, "README.md"), "# Root")
	plansDir := filepath.Join(t.TempDir(), "does-not-exist")
	c := newReadmeExistsChecker(plansDir)
	if _, err := c.check(specRoot); err == nil {
		t.Fatal("expected the plansDir walk's stat error to propagate")
	}
}

func TestReadmeExistsCheckerPlansDirDetectsMissingSkipsDotFindsFiles(t *testing.T) {
	specRoot := t.TempDir()
	writeFile(t, filepath.Join(specRoot, "README.md"), "# Root")

	plansDir := t.TempDir()
	writeFile(t, filepath.Join(plansDir, "README.md"), "# Plans")
	mkdir(t, filepath.Join(plansDir, "with-readme"))
	writeFile(t, filepath.Join(plansDir, "with-readme", "README.md"), "# Plan")
	mkdir(t, filepath.Join(plansDir, "without-readme"))
	mkdir(t, filepath.Join(plansDir, ".hidden"))
	writeFile(t, filepath.Join(plansDir, "loose-file.md"), "# Not a directory")

	c := newReadmeExistsChecker(plansDir)
	violations, err := c.check(specRoot)
	if err != nil {
		t.Fatal(err)
	}
	var gotOne bool
	for _, v := range violations {
		if v.File == filepath.Join("plans", "without-readme") {
			gotOne = true
		}
		if v.File == filepath.Join("plans", ".hidden") {
			t.Fatalf("dot-directory must be skipped, got violation: %+v", v)
		}
	}
	if !gotOne {
		t.Fatalf("expected a violation for plans/without-readme, got %+v", violations)
	}
}

func TestReadmeExistsCheckerSkipsSpecRootPlansWhenExternallyRouted(t *testing.T) {
	specRoot := t.TempDir()
	writeFile(t, filepath.Join(specRoot, "README.md"), "# Root")
	// specRoot's own "plans" tree has no README anywhere — if it were
	// checked directly (the no-routing default) this would produce
	// violations. With external routing configured, the checker must skip
	// it entirely: the authoritative Plans namespace is plansDir instead,
	// walked separately.
	mkdir(t, filepath.Join(specRoot, "plans", "nested"))

	plansDir := t.TempDir()
	writeFile(t, filepath.Join(plansDir, "README.md"), "# Plans")

	c := newReadmeExistsChecker(plansDir)
	violations, err := c.check(specRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range violations {
		if v.File == "plans" || strings.HasPrefix(filepath.ToSlash(v.File), "plans/") {
			t.Fatalf("specRoot's own plans tree must be skipped when externally routed, got %+v", v)
		}
	}
}

// --- adherence_footer.go: plansDir-aware walkers ----------------------------

func TestWalkPlansIndexDirRejectsSymlink(t *testing.T) {
	plansDir := t.TempDir()
	target := filepath.Join(t.TempDir(), "index-target.md")
	writeFile(t, target, "# Plans")
	if err := os.Symlink(target, filepath.Join(plansDir, "README.md")); err != nil {
		t.Fatal(err)
	}
	if err := walkPlansIndexDir(plansDir, func(string, []byte) {}); err == nil || !strings.Contains(err.Error(), "must not be a symbolic link") {
		t.Fatalf("err = %v", err)
	}
}

func TestWalkFlatPlanFilesDirRejectsUnreadableFile(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission checks do not apply while running as root")
	}
	plansDir := t.TempDir()
	blocked := filepath.Join(plansDir, "blocked.md")
	writeFile(t, blocked, "# Plan: Blocked\n")
	if err := os.Chmod(blocked, 0o000); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(blocked, 0o644) }()
	if err := walkFlatPlanFilesDir(plansDir, func(string, []byte) {}); err == nil {
		t.Fatal("expected Parse's read error to propagate")
	}
}

func TestWalkPlanReadmesDirRejectsSymlink(t *testing.T) {
	plansDir := t.TempDir()
	target := filepath.Join(t.TempDir(), "plan-target.md")
	writeFile(t, target, "# Plan: Target\n")
	nested := filepath.Join(plansDir, "child")
	mkdir(t, nested)
	if err := os.Symlink(target, filepath.Join(nested, "README.md")); err != nil {
		t.Fatal(err)
	}
	if err := walkPlanReadmesDir(plansDir, func(string, []byte) {}); err == nil || !strings.Contains(err.Error(), "plan must not be a symbolic link") {
		t.Fatalf("err = %v", err)
	}
}

func TestWalkTaskReadmesDirRejectsSymlink(t *testing.T) {
	plansDir := t.TempDir()
	target := filepath.Join(t.TempDir(), "task-target.md")
	writeFile(t, target, "# Task 1: Target\n")
	nested := filepath.Join(plansDir, "auth", "tasks", "1")
	mkdir(t, nested)
	if err := os.Symlink(target, filepath.Join(nested, "README.md")); err != nil {
		t.Fatal(err)
	}
	if err := walkTaskReadmesDir(plansDir, func(string, []byte) {}); err == nil || !strings.Contains(err.Error(), "plan task must not be a symbolic link") {
		t.Fatalf("err = %v", err)
	}
}

func TestWalkDocTargetPlanOwnedRouting(t *testing.T) {
	plansDir := t.TempDir()
	if err := walkDocTarget(docTypeTarget{planOwned: "index"}, "unused", plansDir, false, func(string, []byte) {}); err != nil {
		t.Fatalf("index routing: %v", err)
	}
	if err := walkDocTarget(docTypeTarget{planOwned: "task"}, "unused", plansDir, false, func(string, []byte) {}); err != nil {
		t.Fatalf("task routing: %v", err)
	}
	if err := walkDocTarget(docTypeTarget{planOwned: "bogus"}, "unused", plansDir, false, func(string, []byte) {}); err == nil || !strings.Contains(err.Error(), "unknown plan-owned document type") {
		t.Fatalf("unknown routing err = %v", err)
	}
}

// AC: skip-plan-owned — when skipPlanOwned is true, a plan-owned target is a
// complete no-op (fn is never invoked, no error), regardless of plansDir;
// this is what a configured-but-broken Plan route relies on to never read
// spec/plans (finding 1 / repo-config#req:plan-route-required). A
// non-plan-owned target is unaffected — skipPlanOwned only ever gates
// target.planOwned != "" targets.
func TestWalkDocTargetSkipPlanOwned(t *testing.T) {
	plansDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(plansDir, "README.md"), []byte("plans index"), 0o644); err != nil {
		t.Fatal(err)
	}
	called := false
	if err := walkDocTarget(docTypeTarget{planOwned: "index"}, "unused", plansDir, true, func(string, []byte) { called = true }); err != nil {
		t.Fatalf("skip-plan-owned index: %v", err)
	}
	if called {
		t.Fatal("skipPlanOwned=true must never invoke fn for a plan-owned target")
	}

	specRoot := t.TempDir()
	nonPlanCalled := false
	nonPlan := docTypeTarget{walk: func(string, func(string, []byte)) error { nonPlanCalled = true; return nil }}
	if err := walkDocTarget(nonPlan, specRoot, plansDir, true, func(string, []byte) {}); err != nil {
		t.Fatalf("skip-plan-owned non-plan target: %v", err)
	}
	if !nonPlanCalled {
		t.Fatal("skipPlanOwned=true must not affect a non-plan-owned target")
	}
}

// --- plan_rules.go: real lock contention (deterministic, not racy) --------

// holdArtifactLock acquires the exact flock lifecycle.TransformArtifact uses
// for planPath and returns a release func. Acquiring it first, synchronously,
// before calling the fixer under test makes the fixer's lock-contention
// error deterministic rather than dependent on any timing race.
func holdArtifactLock(t *testing.T, planPath string) func() {
	t.Helper()
	lockPath := filepath.Join(filepath.Dir(planPath), "."+filepath.Base(planPath)+".lifecycle-transaction.lock")
	fl := flock.New(lockPath)
	locked, err := fl.TryLock()
	if err != nil {
		t.Fatal(err)
	}
	if !locked {
		t.Fatal("failed to acquire the artifact lock for the test setup itself")
	}
	return func() { _ = fl.Unlock() }
}

func TestFixLegacyTaskStatusesInDirLockContentionPropagates(t *testing.T) {
	plansDir := t.TempDir()
	planPath := filepath.Join(plansDir, "auth.md")
	writeFile(t, planPath, "# Plan: Auth\n")
	release := holdArtifactLock(t, planPath)
	defer release()
	if err := fixLegacyTaskStatusesInDir(plansDir); err == nil || !strings.Contains(err.Error(), "fixing") {
		t.Fatalf("err = %v", err)
	}
}

func TestFixP007InDirLockContentionPropagates(t *testing.T) {
	plansDir := t.TempDir()
	planPath := filepath.Join(plansDir, "auth.md")
	writeFile(t, planPath, "# Plan: Auth\n")
	release := holdArtifactLock(t, planPath)
	defer release()
	if err := fixP007InDir(plansDir); err == nil || !strings.Contains(err.Error(), "fixing") {
		t.Fatalf("err = %v", err)
	}
}

// withFailingPlanFixParseBytes overrides planFixParseBytesFn to fail for the
// duration of the test, restoring the real plan.ParseBytes afterward.
func withFailingPlanFixParseBytes(t *testing.T) {
	t.Helper()
	orig := planFixParseBytesFn
	t.Cleanup(func() { planFixParseBytesFn = orig })
	planFixParseBytesFn = func(string, []byte) (*plan.Plan, error) {
		return nil, errors.New("injected re-parse failure")
	}
}

func TestFixNoSourceLinesReparseErrorPropagates(t *testing.T) {
	plansDir := t.TempDir()
	planPath := filepath.Join(plansDir, "auth.md")
	writeFile(t, planPath, "# Plan: Auth\n")
	withFailingPlanFixParseBytes(t)
	c := &planRulesChecker{plansDir: plansDir}
	err := c.fixNoSourceLines("")
	if err == nil || !strings.Contains(err.Error(), "injected re-parse failure") {
		t.Fatalf("err = %v", err)
	}
}

func TestFixLegacyTaskStatusesInDirReparseErrorPropagates(t *testing.T) {
	plansDir := t.TempDir()
	planPath := filepath.Join(plansDir, "auth.md")
	writeFile(t, planPath, "# Plan: Auth\n")
	withFailingPlanFixParseBytes(t)
	if err := fixLegacyTaskStatusesInDir(plansDir); err == nil || !strings.Contains(err.Error(), "injected re-parse failure") {
		t.Fatalf("err = %v", err)
	}
}

func TestFixP007InDirReparseErrorPropagates(t *testing.T) {
	plansDir := t.TempDir()
	planPath := filepath.Join(plansDir, "auth.md")
	writeFile(t, planPath, "# Plan: Auth\n")
	withFailingPlanFixParseBytes(t)
	if err := fixP007InDir(plansDir); err == nil || !strings.Contains(err.Error(), "injected re-parse failure") {
		t.Fatalf("err = %v", err)
	}
}
