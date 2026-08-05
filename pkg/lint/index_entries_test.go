package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestIndexEntriesFix_ExcludesReservedBenchmarkSubtree is a regression test
// for the fix()/check() asymmetry: check() already excludes the reserved
// `benchmark` subtree (a per-feature benchmark-tooling directory that is not
// itself a spec artifact) from the set of children a parent index must list,
// but fix()'s child-collection loop did not — so a full-tree `spec lint
// --fix` (as run internally by `feature new`/`feature change-status`, and by
// this repo's own `lesson new`) would insert a phantom `benchmark` row into
// the parent index, which check() would then immediately flag as pointing at
// a non-existent (excluded) directory. Left uncaught, this made a routine
// `--fix` pass silently corrupt an unrelated file elsewhere in the tree.
func TestIndexEntriesFix_ExcludesReservedBenchmarkSubtree(t *testing.T) {
	root := t.TempDir()
	parentDir := filepath.Join(root, "features", "parent")
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	parentReadme := filepath.Join(parentDir, "README.md")
	if err := os.WriteFile(parentReadme, []byte("# Feature: Parent\n\n## Summary\n\nx\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// The reserved subtree: a benchmark-tooling directory that legitimately
	// carries its own README.md (mirroring spec/features/cli/studio/answers/benchmark/)
	// but MUST NOT be treated as a child spec artifact.
	benchmarkDir := filepath.Join(parentDir, reservedFeatureSubtree)
	if err := os.MkdirAll(benchmarkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(benchmarkDir, "README.md"), []byte("# Benchmark\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	// A genuine child feature, to prove the fixer still does its real job.
	childDir := filepath.Join(parentDir, "child-a")
	if err := os.MkdirAll(childDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(childDir, "README.md"), []byte("# Feature: Child A\n\n**Status:** Draft\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := newIndexEntriesChecker()
	f, ok := c.(fixer)
	if !ok {
		t.Fatal("indexEntriesChecker does not implement fixer")
	}
	if err := f.fix(root); err != nil {
		t.Fatalf("fix: %v", err)
	}

	got, err := os.ReadFile(parentReadme)
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	if strings.Contains(s, reservedFeatureSubtree) {
		t.Errorf("fix() inserted a row for the reserved %q subtree:\n%s", reservedFeatureSubtree, s)
	}
	if !strings.Contains(s, "child-a") {
		t.Errorf("fix() should still insert a row for the genuine child-a directory:\n%s", s)
	}

	// A subsequent check() must report no violations: the fixed index and the
	// reserved-subtree exclusion agree.
	violations, err := c.check(root)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	for _, v := range violations {
		t.Errorf("unexpected violation after fix: %+v", v)
	}
}

func TestIndexEntries_ExcludesReservedProposalsSubtree(t *testing.T) {
	root := t.TempDir()
	parentDir := filepath.Join(root, "features", "parent")
	if err := os.MkdirAll(filepath.Join(parentDir, proposalsFeatureSubtree), 0o755); err != nil {
		t.Fatal(err)
	}
	parentReadme := filepath.Join(parentDir, "README.md")
	if err := os.WriteFile(parentReadme, []byte("# Feature: Parent\n\n## Summary\n\nx\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := newIndexEntriesChecker()
	if err := c.(fixer).fix(root); err != nil {
		t.Fatalf("fix: %v", err)
	}
	got, err := os.ReadFile(parentReadme)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), proposalsFeatureSubtree) {
		t.Errorf("proposals container must not be inserted as a child Feature:\n%s", got)
	}
	violations, err := c.check(root)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	for _, violation := range violations {
		if strings.Contains(violation.Message, proposalsFeatureSubtree) {
			t.Errorf("proposals container must not be checked as a child Feature: %+v", violation)
		}
	}
}

// TestIndexEntriesFix_DropsStaleBenchmarkRow covers the companion case: an
// index that ALREADY carries a phantom row pointing at the reserved
// subtree (e.g. written before this fix, or by a build of the CLI that
// predates it) must have that row removed by fix()'s phantom-row-deletion
// phase, not treated as legitimately mentioned.
func TestIndexEntriesFix_DropsStaleBenchmarkRow(t *testing.T) {
	root := t.TempDir()
	parentDir := filepath.Join(root, "features", "parent")
	if err := os.MkdirAll(parentDir, 0o755); err != nil {
		t.Fatal(err)
	}
	parentReadme := filepath.Join(parentDir, "README.md")
	stale := "# Feature: Parent\n\n## Summary\n\nx\n\n## Contents\n\n| Child | Description |\n|---|---|\n| [benchmark](benchmark/README.md) | TODO: Add description. |\n"
	if err := os.WriteFile(parentReadme, []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}
	benchmarkDir := filepath.Join(parentDir, reservedFeatureSubtree)
	if err := os.MkdirAll(benchmarkDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(benchmarkDir, "README.md"), []byte("# Benchmark\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	c := newIndexEntriesChecker()
	f, ok := c.(fixer)
	if !ok {
		t.Fatal("indexEntriesChecker does not implement fixer")
	}
	if err := f.fix(root); err != nil {
		t.Fatalf("fix: %v", err)
	}

	got, err := os.ReadFile(parentReadme)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), reservedFeatureSubtree) {
		t.Errorf("fix() should have dropped the stale phantom benchmark row:\n%s", got)
	}
}
