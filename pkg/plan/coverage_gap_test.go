package plan

// This file closes out statement-coverage gaps in path.go, discover.go,
// index.go and reconcile.go left after the Plan repository routing feature
// widened these packages to understand both the canonical directory form and
// the legacy flat form. Each test targets one specific error branch that
// resolve_test.go/path_external_test.go/reconcile_test.go's behavioral focus
// didn't already happen to exercise.
//
// A few branches are provably unreachable given the caller's own invariants
// (e.g. a Rel() call against a path that was just built via Join() from the
// same base) and are simplified in place at the call site rather than
// tested. TOCTOU-only defenses (e.g. an ancestor directory or plansDir
// itself being swapped out from under a call mid-flight) sit behind
// injectable seams (discoverWalkDirFn, validateResolvedPlanPathFn) so they
// still get a deterministic test instead of a genuinely racy, CI-flaky one.

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// --- ValidateID / PathForID --------------------------------------------

func TestValidateIDRejectsEmptyAndMalformed(t *testing.T) {
	cases := []string{"", "a\\b", "/leading", "trailing/"}
	for _, id := range cases {
		if err := ValidateID(id); err == nil {
			t.Errorf("ValidateID(%q) = nil, want error", id)
		}
	}
}

func TestPathForIDPropagatesInvalidID(t *testing.T) {
	if _, err := PathForID(t.TempDir(), ""); err == nil {
		t.Fatal("expected error for an empty plan ID")
	}
}

// --- ValidateWritePath ---------------------------------------------------

func TestValidateWritePathPlansDirMissing(t *testing.T) {
	if err := ValidateWritePath(filepath.Join(t.TempDir(), "does-not-exist"), "/tmp/x"); err == nil || !strings.Contains(err.Error(), "resolve plans directory") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateWritePathAncestorPermissionDenied(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission checks do not apply while running as root")
	}
	plansDir := t.TempDir()
	restricted := filepath.Join(plansDir, "restricted")
	if err := os.Mkdir(restricted, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(restricted, 0o000); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(restricted, 0o755) }()
	target := filepath.Join(restricted, "child", "README.md")
	if err := ValidateWritePath(plansDir, target); err == nil {
		t.Fatal("expected a permission error")
	}
}

func TestValidateWritePathAcceptsPathWithinPlansDir(t *testing.T) {
	plansDir := t.TempDir()
	target, err := PathForID(plansDir, "new-plan")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ValidateWritePath(plansDir, target); err != nil {
		t.Fatalf("ValidateWritePath = %v, want nil", err)
	}
}

func TestValidateWritePathAncestorSymlinkResolutionFails(t *testing.T) {
	plansDir := t.TempDir()
	if err := os.Symlink(filepath.Join(plansDir, "nowhere"), filepath.Join(plansDir, "brokenlink")); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(plansDir, "brokenlink", "child", "README.md")
	if err := ValidateWritePath(plansDir, target); err == nil {
		t.Fatal("expected an EvalSymlinks error resolving the broken ancestor")
	}
}

// --- ResolveFile / validateResolvedPlanPath -------------------------------

func TestResolveFileInvalidID(t *testing.T) {
	if _, err := ResolveFile(t.TempDir(), ""); err == nil {
		t.Fatal("expected error for an empty plan ID")
	}
}

func TestResolveFileCanonicalAncestorSymlinkEscapes(t *testing.T) {
	plansDir := t.TempDir()
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "README.md"), []byte("# Plan: Outside\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// "escaped" is a directory-shaped symlink pointing entirely outside
	// plansDir; os.Lstat on the canonical README.md path follows it as an
	// intermediate component and lands on a real, non-symlink file, so
	// regularFileExists accepts it — only the later EvalSymlinks-based
	// containment check catches the escape.
	if err := os.Symlink(outside, filepath.Join(plansDir, "escaped")); err != nil {
		t.Fatal(err)
	}
	if _, err := ResolveFile(plansDir, "escaped"); err == nil || !strings.Contains(err.Error(), "escapes plans directory") {
		t.Fatalf("err = %v", err)
	}
}

// TestResolveFileFlatValidateReparseErrorPropagates covers ResolveFile's
// flat-form validateResolvedPlanPathFn call: unlike the canonical branch
// above (an intermediate directory segment CAN be a symlink), there is no
// intermediate path component for a flat plan file, so in real operation
// this can only fail via a genuine concurrent mutation — not something a
// deterministic, non-racy test can force by construction.
func TestResolveFileFlatValidateReparseErrorPropagates(t *testing.T) {
	plansDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(plansDir, "flat.md"), []byte("# Plan: Flat\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	orig := validateResolvedPlanPathFn
	t.Cleanup(func() { validateResolvedPlanPathFn = orig })
	validateResolvedPlanPathFn = func(string, string) error {
		return errors.New("injected validate failure")
	}
	if _, err := ResolveFile(plansDir, "flat"); err == nil || !strings.Contains(err.Error(), "injected validate failure") {
		t.Fatalf("err = %v", err)
	}
}

func TestValidateResolvedPlanPathDirectUnitCases(t *testing.T) {
	t.Run("plans dir missing", func(t *testing.T) {
		missing := filepath.Join(t.TempDir(), "gone")
		if err := validateResolvedPlanPath(missing, filepath.Join(missing, "x.md")); err == nil || !strings.Contains(err.Error(), "resolve plans directory") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("path missing", func(t *testing.T) {
		plansDir := t.TempDir()
		if err := validateResolvedPlanPath(plansDir, filepath.Join(plansDir, "missing.md")); err == nil || !strings.Contains(err.Error(), "resolve plan path") {
			t.Fatalf("err = %v", err)
		}
	})
}

// --- planIDFromPath --------------------------------------------------------

func TestPlanIDFromPathRejectsNonMatchingShapes(t *testing.T) {
	plansDir := t.TempDir()
	cases := []string{
		filepath.Join(plansDir, "sub", "notes.md"),  // nested, not README.md
		filepath.Join(plansDir, "notes.txt"),        // top-level, wrong extension
		filepath.Join(plansDir, "sub", "deep", "x"), // nested, no extension
	}
	for _, path := range cases {
		if id, ok := planIDFromPath(plansDir, path); ok {
			t.Errorf("planIDFromPath(%q) = (%q, true), want ok=false", path, id)
		}
	}
}

func TestPlanIDFromPathRejectsPathOutsideOrEqualToPlansDir(t *testing.T) {
	plansDir := t.TempDir()
	cases := []string{
		plansDir, // rel == "."
		filepath.Join(t.TempDir(), "elsewhere.md"), // rel has a ".." prefix
	}
	for _, path := range cases {
		if id, ok := planIDFromPath(plansDir, path); ok {
			t.Errorf("planIDFromPath(%q) = (%q, true), want ok=false", path, id)
		}
	}
}

// --- Discover --------------------------------------------------------------

func TestDiscoverPlansDirStatPermissionDenied(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission checks do not apply while running as root")
	}
	parent := t.TempDir()
	if err := os.Chmod(parent, 0o000); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(parent, 0o755) }()
	if _, err := Discover(filepath.Join(parent, "plans")); err == nil {
		t.Fatal("expected a permission error")
	}
}

func TestDiscoverSkipsDotDirectories(t *testing.T) {
	plansDir := t.TempDir()
	hidden := filepath.Join(plansDir, ".git", "objects")
	if err := os.MkdirAll(hidden, 0o755); err != nil {
		t.Fatal(err)
	}
	// A Plan-shaped file inside the dot-directory must never surface: proof
	// that filepath.SkipDir actually fired rather than merely not matching.
	if err := os.WriteFile(filepath.Join(hidden, "README.md"), []byte("# Plan: Hidden\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plansDir, "visible.md"), []byte("# Plan: Visible\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	plans, err := Discover(plansDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(plans) != 1 || plans[0].Slug != "visible" {
		t.Fatalf("plans = %#v, want exactly [visible]", plans)
	}
}

func TestDiscoverRejectsAmbiguousFlatAndDirectoryForms(t *testing.T) {
	plansDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(plansDir, "dup.md"), []byte("# Plan: Flat\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(plansDir, "dup"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plansDir, "dup", "README.md"), []byte("# Plan: Directory\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Discover(plansDir); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("err = %v", err)
	}
}

func TestDiscoverPropagatesParseError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission checks do not apply while running as root")
	}
	plansDir := t.TempDir()
	blocked := filepath.Join(plansDir, "blocked.md")
	if err := os.WriteFile(blocked, []byte("# Plan: Blocked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(blocked, 0o000); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(blocked, 0o644) }()
	if _, err := Discover(plansDir); err == nil {
		t.Fatal("expected Parse's read error to propagate")
	}
}

// TestDiscoverPropagatesRealWalkErrorFromSubdirectory covers Discover's
// walkErr fallthrough ("return walkErr") for a real, non-tolerated error: a
// nested directory that WalkDir cannot even ReadDir (unlike the
// plansDir-itself-removed TOCTOU tolerance, this is a deterministic,
// reproducible permission failure on a genuinely present subdirectory).
func TestDiscoverPropagatesRealWalkErrorFromSubdirectory(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission checks do not apply while running as root")
	}
	plansDir := t.TempDir()
	blocked := filepath.Join(plansDir, "blocked")
	if err := os.MkdirAll(blocked, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(blocked, "README.md"), []byte("# Plan: Blocked\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(blocked, 0o000); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(blocked, 0o755) }()
	if _, err := Discover(plansDir); err == nil {
		t.Fatal("expected the unreadable subdirectory's walk error to propagate")
	}
}

// TestDiscoverToleratesPlansDirRemovedBetweenStatAndWalk covers the
// TOCTOU-tolerance branch: os.Stat(plansDir) above succeeded, but
// filepath.WalkDir's own first Lstat(plansDir) can still observe the
// directory gone if a concurrent process removed it in that (usually
// microseconds-wide) window. discoverWalkDirFn stands in for
// filepath.WalkDir so the test can invoke Discover's real callback with
// that exact walkErr deterministically, instead of racing a real deletion.
func TestDiscoverToleratesPlansDirRemovedBetweenStatAndWalk(t *testing.T) {
	plansDir := t.TempDir()
	orig := discoverWalkDirFn
	t.Cleanup(func() { discoverWalkDirFn = orig })
	discoverWalkDirFn = func(root string, fn fs.WalkDirFunc) error {
		return fn(root, nil, fs.ErrNotExist)
	}
	plans, err := Discover(plansDir)
	if err != nil {
		t.Fatalf("err = %v, want the not-exist walkErr tolerated", err)
	}
	if len(plans) != 0 {
		t.Fatalf("plans = %#v, want none", plans)
	}
}

// TestDiscoverValidateResolvedPathReparseErrorPropagates covers Discover's
// defense-in-depth validateResolvedPlanPathFn call: in real operation it can
// only fail if plansDir is swapped out from under an in-progress walk, not
// something a deterministic, non-racy test can force by construction — so
// the seam is overridden directly instead.
func TestDiscoverValidateResolvedPathReparseErrorPropagates(t *testing.T) {
	plansDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(plansDir, "visible.md"), []byte("# Plan: Visible\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	orig := validateResolvedPlanPathFn
	t.Cleanup(func() { validateResolvedPlanPathFn = orig })
	validateResolvedPlanPathFn = func(string, string) error {
		return errors.New("injected validate failure")
	}
	if _, err := Discover(plansDir); err == nil || !strings.Contains(err.Error(), "injected validate failure") {
		t.Fatalf("err = %v", err)
	}
}

// --- reconcile.go ---------------------------------------------------------

func TestReconcilePlansDirExplicitOverride(t *testing.T) {
	if got := reconcilePlansDir(ReconcileOptions{PlansDir: "/explicit/plans"}); got != "/explicit/plans" {
		t.Fatalf("reconcilePlansDir = %q, want explicit override", got)
	}
}

func TestPreviewReconcileInvalidSlugPropagatesPathForIDError(t *testing.T) {
	if _, err := PreviewReconcile(ReconcileOptions{
		SpecRoot: t.TempDir(),
		Slug:     "Not A Valid Slug!",
		Note:     "reason",
	}); err == nil || !strings.Contains(err.Error(), "invalid plan ID") {
		t.Fatalf("err = %v", err)
	}
}

func TestReconcileInvalidSlugPropagatesPathForIDError(t *testing.T) {
	if _, err := Reconcile(ReconcileOptions{
		SpecRoot: t.TempDir(),
		Slug:     "Not A Valid Slug!",
		Note:     "reason",
		PostMutation: func() error {
			t.Fatal("PostMutation must not run when the slug itself is invalid")
			return nil
		},
	}); err == nil || !strings.Contains(err.Error(), "invalid plan ID") {
		t.Fatalf("err = %v", err)
	}
}

func TestPreviewReconcileReadFilePermissionDenied(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission checks do not apply while running as root")
	}
	root := t.TempDir()
	path := filepath.Join(root, "spec", "plans", "auth.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(reconcilePlanBody("Draft", "planning")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.Chmod(path, 0o644) }()
	if _, err := PreviewReconcile(ReconcileOptions{
		SpecRoot: root,
		Slug:     "auth",
		Note:     "reason",
	}); err == nil || !strings.Contains(err.Error(), "reading plan") {
		t.Fatalf("err = %v", err)
	}
}
