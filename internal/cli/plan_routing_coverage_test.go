package cli

// This file closes out statement-coverage gaps left by the Plan repository
// routing feature across internal/cli (plan_store.go, plan_coordination.go,
// feature.go, idea.go's lintPostMutationHookWithPlans, plan.go, task.go).
// Each test targets one specific branch the feature's existing test files
// didn't already happen to exercise.
//
// Several branches are NOT covered here; each carries an explanatory comment
// at its call site in the production source. They fall into two classes:
//   - Provably unreachable given an earlier, identical check already
//     succeeded in the same call (e.g. plan.ValidateID(slug) then
//     plan.PathForID(slug) again; a filepath.Rel against a path just built
//     via Join from the same base). These were simplified in place.
//   - Genuine TOCTOU/concurrent-mutation defenses (e.g. a second process
//    creating the exact target file in the window between this process's
//    existence check and its exclusive publish) that only a real race — not
//    a deterministic test — could exercise.

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/gofrs/flock"
	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
	"github.com/specscore/specscore-cli/pkg/lint"
	"github.com/specscore/specscore-cli/pkg/plan"
	"github.com/specscore/specscore-cli/pkg/projectdef"
)

// --- init.go: appendPlanRoutingHint ------------------------------------------

func TestAppendPlanRoutingHintOpenFileError(t *testing.T) {
	if err := appendPlanRoutingHint(filepath.Join(t.TempDir(), "does-not-exist.yaml")); err == nil {
		t.Fatal("expected an error opening a nonexistent file for append")
	}
}

// TestRunInit_AppendPlanRoutingHintFailurePropagates covers runInit's
// error-handling branch around appendPlanRoutingHintFn. In real operation
// that call can only fail via a genuine TOCTOU race (configPath's
// permissions or existence changing between WriteSpecConfig's write and
// this reopen), so the seam is overridden directly instead.
func TestRunInit_AppendPlanRoutingHintFailurePropagates(t *testing.T) {
	root := t.TempDir()
	withCwd(t, root)

	orig := appendPlanRoutingHintFn
	t.Cleanup(func() { appendPlanRoutingHintFn = orig })
	appendPlanRoutingHintFn = func(string) error {
		return errors.New("injected append failure")
	}

	_, _, err := runInitCmd(t, nil, "--project", root)
	if err == nil || !strings.Contains(err.Error(), "injected append failure") {
		t.Fatalf("err = %v", err)
	}
}

// --- plan_store.go: ensurePlansNamespace ------------------------------------

func TestEnsurePlansNamespaceMkdirAllFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission checks do not apply while running as root")
	}
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o555); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(parent, 0o755) }()
	if err := ensurePlansNamespace(filepath.Join(parent, "plans")); err == nil {
		t.Fatal("expected MkdirAll to fail under a read-only parent")
	}
}

func TestEnsurePlansNamespaceStatIndexFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission checks do not apply while running as root")
	}
	plansDir := t.TempDir()
	if err := os.Chmod(plansDir, 0o000); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(plansDir, 0o755) }()
	if err := ensurePlansNamespace(plansDir); err == nil {
		t.Fatal("expected Stat(README.md) to fail without search permission on plansDir")
	}
}

func TestEnsurePlansNamespacePublishFails(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission checks do not apply while running as root")
	}
	plansDir := t.TempDir()
	if err := os.Chmod(plansDir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(plansDir, 0o755) }()
	err := ensurePlansNamespace(plansDir)
	if err == nil || !strings.Contains(err.Error(), "publishing plans namespace index") {
		t.Fatalf("err = %v", err)
	}
}

// --- plan_coordination.go: coordinationCheck --------------------------------

func TestCoordinationCheckUnparseableOriginRemote(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	run("init", "-b", "main")
	run("remote", "add", "origin", "/local/path/not/a/url")
	matched, actual := coordinationCheck(dir, "owner", "repo", "main")
	if matched {
		t.Fatalf("matched = true, want false for an unparseable origin remote")
	}
	if !strings.Contains(actual, "not a recognized owner/repo") {
		t.Fatalf("actual = %q", actual)
	}
}

// --- feature.go: routed feature info -----------------------------------------

func TestFeatureInfo_PopulatesPlansDirWhenRoutingConfigured(t *testing.T) {
	root := setupFeatureSpec(t, "Approved")
	configureSameRepoPlans(t, root)
	// feature info must succeed and populate the Plans back-reference from
	// the resolved store, not just fall back to the no-routing default.
	if _, _, err := runFeature(t, "info", "auth", "--project", root); err != nil {
		t.Fatalf("feature info: %v", err)
	}
}

// --- idea.go: lintPostMutationHookWithPlans failure --------------------------

func TestLintPostMutationHookWithPlansPropagatesLintFailure(t *testing.T) {
	root := setupLintCleanProject(t)
	configureSameRepoPlans(t, root)
	if _, _, err := runPlan(t, "new", "auth", "--project", root, "--owner", "tester"); err != nil {
		t.Fatalf("plan new: %v", err)
	}
	orig := lintLintFn
	lintLintFn = func(lint.Options) ([]lint.Violation, error) {
		return nil, errors.New("boom lint failure")
	}
	t.Cleanup(func() { lintLintFn = orig })
	_, _, err := runPlan(t, "change-status", "auth", "--to=in review", "--project", root)
	if err == nil || !strings.Contains(err.Error(), "running lint --fix") {
		t.Fatalf("err = %v", err)
	}
}

// --- plan.go: runPlanChangeStatus / runPlanNew ------------------------------

func TestPlanNew_EnsurePlansNamespaceFailurePropagates(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission checks do not apply while running as root")
	}
	root := setupLintCleanProject(t)
	configureSameRepoPlans(t, root)
	// spec/README.md (and features/ideas indexes) already exist from
	// setupLintCleanProject, so writeMissingIndex no-ops without needing
	// write access; restricting "spec" itself then makes ensurePlansNamespace's
	// MkdirAll(spec/plans) fail.
	specDir := filepath.Join(root, "spec")
	if err := os.Chmod(specDir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(specDir, 0o755) }()
	_, _, err := runPlan(t, "new", "blocked", "--project", root, "--owner", "tester")
	if err == nil || !strings.Contains(err.Error(), "materializing ancestor indexes") {
		t.Fatalf("err = %v", err)
	}
}

func TestPlanNew_ValidateWritePathAncestorSymlinkEscapes(t *testing.T) {
	root := setupLintCleanProject(t)
	configureSameRepoPlans(t, root)
	plansDir := filepath.Join(root, "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.Symlink(outside, filepath.Join(plansDir, "escaped")); err != nil {
		t.Fatal(err)
	}
	_, _, err := runPlan(t, "new", "escaped", "--project", root, "--owner", "tester")
	if err == nil || !strings.Contains(err.Error(), "escapes") {
		t.Fatalf("err = %v", err)
	}
}

// planStoreArtifactLockPath mirrors lifecycle/cas.go's acquireArtifactLockWithOps
// naming so tests can hold the same real flock before calling a fixer or
// mutation, making its lock-contention error deterministic instead of racy.
func planStoreArtifactLockPath(artifactPath string) string {
	return filepath.Join(filepath.Dir(artifactPath), "."+filepath.Base(artifactPath)+".lifecycle-transaction.lock")
}

func holdCLIArtifactLock(t *testing.T, artifactPath string) func() {
	t.Helper()
	fl := flock.New(planStoreArtifactLockPath(artifactPath))
	locked, err := fl.TryLock()
	if err != nil {
		t.Fatal(err)
	}
	if !locked {
		t.Fatal("failed to acquire the artifact lock for test setup")
	}
	return func() { _ = fl.Unlock() }
}

func TestPlanNew_ForceReplaceLockContentionPropagates(t *testing.T) {
	root := setupLintCleanProject(t)
	configureSameRepoPlans(t, root)
	if _, _, err := runPlan(t, "new", "auth", "--project", root, "--owner", "tester"); err != nil {
		t.Fatalf("plan new: %v", err)
	}
	target := filepath.Join(root, "spec", "plans", "auth", "README.md")
	release := holdCLIArtifactLock(t, target)
	defer release()
	_, _, err := runPlan(t, "new", "auth", "--project", root, "--owner", "tester", "--force")
	if err == nil || !strings.Contains(err.Error(), "replacing") {
		t.Fatalf("err = %v", err)
	}
}

func TestPlanNew_ForcePublishPermissionDenied(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission checks do not apply while running as root")
	}
	root := setupLintCleanProject(t)
	configureSameRepoPlans(t, root)
	// MkdirAll would apply 0o555 to every level it creates, including
	// "spec/plans" itself — which would then block creating "blocked"
	// beneath it in the very same call. Create the writable parent first,
	// then lock down just the leaf directory.
	if err := os.MkdirAll(filepath.Join(root, "spec", "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	slugDir := filepath.Join(root, "spec", "plans", "blocked")
	if err := os.Mkdir(slugDir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(slugDir, 0o755) }()
	_, _, err := runPlan(t, "new", "blocked", "--project", root, "--owner", "tester", "--force")
	if err == nil || !strings.Contains(err.Error(), "publishing") {
		t.Fatalf("err = %v", err)
	}
}

func TestPlanNew_NonForcePublishPermissionDenied(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission checks do not apply while running as root")
	}
	root := setupLintCleanProject(t)
	configureSameRepoPlans(t, root)
	if err := os.MkdirAll(filepath.Join(root, "spec", "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	slugDir := filepath.Join(root, "spec", "plans", "blocked2")
	if err := os.Mkdir(slugDir, 0o555); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(slugDir, 0o755) }()
	_, _, err := runPlan(t, "new", "blocked2", "--project", root, "--owner", "tester")
	if err == nil || !strings.Contains(err.Error(), "publishing") {
		t.Fatalf("err = %v", err)
	}
}

// withFailingPlanNewStat overrides planNewStatFn to fail with a non-nil,
// non-IsNotExist error for any path whose slash-form ends in suffix (a
// relative path fragment), for the duration of the test. Matching by
// suffix rather than exact equality avoids assuming target's exact form —
// store.PlansDir may resolve through a symlink (e.g. macOS's
// /var/folders -> /private/var/folders) to something other than the
// literal root the test constructed.
func withFailingPlanNewStat(t *testing.T, suffix string) {
	t.Helper()
	orig := planNewStatFn
	t.Cleanup(func() { planNewStatFn = orig })
	planNewStatFn = func(path string) (os.FileInfo, error) {
		if strings.HasSuffix(filepath.ToSlash(path), suffix) {
			return nil, errors.New("injected stat failure")
		}
		return orig(path)
	}
}

// TestPlanNew_NonForceStatFindsRacedConflict covers the sibling
// statErr==nil branch of the same planNewStatFn call: plan.ResolveFile just
// established target does not exist, so in real operation this can only
// see it exist now via a genuine concurrent creation — not something a
// deterministic, single-threaded test can force without actually racing a
// second process.
func TestPlanNew_NonForceStatFindsRacedConflict(t *testing.T) {
	root := setupLintCleanProject(t)
	configureSameRepoPlans(t, root)
	orig := planNewStatFn
	t.Cleanup(func() { planNewStatFn = orig })
	planNewStatFn = func(path string) (os.FileInfo, error) {
		if strings.HasSuffix(filepath.ToSlash(path), "spec/plans/blocked5/README.md") {
			return nil, nil
		}
		return orig(path)
	}
	_, _, err := runPlan(t, "new", "blocked5", "--project", root, "--owner", "tester")
	if err == nil || !strings.Contains(err.Error(), "plan already exists") {
		t.Fatalf("err = %v", err)
	}
}

func TestPlanNew_NonForceStatNonNotExistErrorPropagates(t *testing.T) {
	root := setupLintCleanProject(t)
	configureSameRepoPlans(t, root)
	withFailingPlanNewStat(t, "spec/plans/blocked3/README.md")
	_, _, err := runPlan(t, "new", "blocked3", "--project", root, "--owner", "tester")
	if err == nil || !strings.Contains(err.Error(), "checking") {
		t.Fatalf("err = %v", err)
	}
}

func TestPlanNew_ForceStatNonNotExistErrorPropagates(t *testing.T) {
	root := setupLintCleanProject(t)
	configureSameRepoPlans(t, root)
	withFailingPlanNewStat(t, "spec/plans/blocked4/README.md")
	_, _, err := runPlan(t, "new", "blocked4", "--project", root, "--owner", "tester", "--force")
	if err == nil || !strings.Contains(err.Error(), "checking") {
		t.Fatalf("err = %v", err)
	}
}

func TestPlanNew_NonForcePublishFileExclusiveErrExistPropagates(t *testing.T) {
	root := setupLintCleanProject(t)
	configureSameRepoPlans(t, root)
	orig := planNewPublishFileExclusiveFn
	t.Cleanup(func() { planNewPublishFileExclusiveFn = orig })
	planNewPublishFileExclusiveFn = func(string, []byte, os.FileMode) error {
		return os.ErrExist
	}
	_, _, err := runPlan(t, "new", "raced", "--project", root, "--owner", "tester")
	if err == nil || !strings.Contains(err.Error(), "plan already exists") {
		t.Fatalf("err = %v", err)
	}
}

func TestPlanChangeStatus_ValidateSnapshotReparseErrorPropagates(t *testing.T) {
	root := stagePlan(t, "auth", "Draft")
	configureSameRepoPlans(t, root)
	orig := planChangeStatusParseBytesFn
	t.Cleanup(func() { planChangeStatusParseBytesFn = orig })
	planChangeStatusParseBytesFn = func(string, []byte) (*plan.Plan, error) {
		return nil, errors.New("injected re-parse failure")
	}
	_, _, err := runPlan(t, "change-status", "auth", "--to=in review")
	if err == nil || !strings.Contains(err.Error(), "injected re-parse failure") {
		t.Fatalf("err = %v", err)
	}
}

// --- plan_reconcile.go: tree-transaction ResolveFile ------------------------

func TestPlanReconcile_TreeTransactionResolveFileFailurePropagates(t *testing.T) {
	root := stageReconcilablePlan(t, "auth", "Draft", "planning", "planning")
	orig := planReconcileResolveFileFn
	t.Cleanup(func() { planReconcileResolveFileFn = orig })
	planReconcileResolveFileFn = func(string, string) (string, error) {
		return "", errors.New("injected resolve failure")
	}
	_, _, err := runPlan(t, "reconcile", "auth", "--tasks=complete",
		"--note", "delivered outside the tracked flow", "--tree-transaction", "--project", root)
	if err == nil || !strings.Contains(err.Error(), "injected resolve failure") {
		t.Fatalf("err = %v", err)
	}
}

func TestReadSpecConfigBestEffortMalformedYAMLDegradesToZeroValue(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "specscore.yaml"), []byte("not: [valid\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := readSpecConfigBestEffort(root); !reflect.DeepEqual(got, projectdef.SpecConfig{}) {
		t.Fatalf("readSpecConfigBestEffort = %#v, want zero value", got)
	}
}

// --- task.go: runTaskChangeStatusPlanInline / runTaskNew --------------------

func TestTaskChangeStatus_PlanInline_TransformArtifactNotExist(t *testing.T) {
	stagePlanWithTasks(t, "auth", twoTaskPlanBody)
	_, _, err := runTaskWithMutationDeps(t, taskMutationDeps{
		transformArtifact: func(string, func([]byte) ([]byte, error)) error { return os.ErrNotExist },
	}, "change-status", "setup", "--plan", "auth", "--to=complete")
	if exitCodeOfErr(err) != exitcode.NotFound {
		t.Fatalf("exit = %d, want NotFound; err=%v", exitCodeOfErr(err), err)
	}
}

func TestTaskChangeStatus_PlanInline_TransformArtifactTypedError(t *testing.T) {
	stagePlanWithTasks(t, "auth", twoTaskPlanBody)
	typed := exitcode.InvalidArgsError("already-typed failure")
	_, _, err := runTaskWithMutationDeps(t, taskMutationDeps{
		transformArtifact: func(string, func([]byte) ([]byte, error)) error { return typed },
	}, "change-status", "setup", "--plan", "auth", "--to=complete")
	if !errors.Is(err, typed) {
		t.Fatalf("err = %v, want the typed error preserved unwrapped", err)
	}
}

func TestTaskChangeStatus_PlanInline_PostMutationLintFailurePropagates(t *testing.T) {
	stagePlanWithTasks(t, "auth", twoTaskPlanBody)
	orig := lintLintFn
	lintLintFn = func(lint.Options) ([]lint.Violation, error) {
		return nil, errors.New("boom lint failure")
	}
	t.Cleanup(func() { lintLintFn = orig })
	_, _, err := runTask(t, "change-status", "setup", "--plan", "auth", "--to=complete")
	if err == nil || !strings.Contains(err.Error(), "running lint --fix") {
		t.Fatalf("err = %v", err)
	}
}

func TestTaskNew_ArtifactTxConcurrentMutationSurfacesConflict(t *testing.T) {
	root := setupTaskProjectForNew(t)
	withCwd(t, root)
	_, _, err := runTaskWithMutationDeps(t, taskMutationDeps{
		withArtifactTx: func(string, func(*lifecycle.ArtifactTransaction) error) error {
			return lifecycle.ErrConcurrentMutation
		},
	}, "new", "--task=busy", "--title=Busy")
	if exitCodeOfErr(err) != exitcode.Conflict || !strings.Contains(err.Error(), "task board is busy") {
		t.Fatalf("err = %v", err)
	}
}
