package lint

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// covSnapshotReadFileError exercises snapshotSpecTree's os.ReadFile error branch
// (and the final non-nil-error return) by making a file under the spec root
// unreadable.
func TestCov_SnapshotSpecTree_ReadFileError(t *testing.T) {
	tmp := t.TempDir()
	bad := filepath.Join(tmp, "unreadable.md")
	if err := os.WriteFile(bad, []byte("data"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(bad, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(bad, 0o644) })

	if _, err := snapshotSpecTree(tmp); err == nil {
		t.Fatal("expected error from snapshotSpecTree on unreadable file")
	}
}

// covSnapshotWalkError exercises snapshotSpecTree's walk-callback err!=nil branch
// by making a subdirectory unreadable so filepath.Walk reports an error for it.
func TestCov_SnapshotSpecTree_WalkError(t *testing.T) {
	tmp := t.TempDir()
	sub := filepath.Join(tmp, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "f.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sub, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(sub, 0o755) })

	if _, err := snapshotSpecTree(tmp); err == nil {
		t.Fatal("expected error from snapshotSpecTree on unreadable subdir")
	}
}

// covSnapshotRelError exercises snapshotSpecTree's filepath.Rel error branch via
// the filepathRelFn seam (otherwise unreachable, since Walk only yields paths
// under specRoot).
func TestCov_SnapshotSpecTree_RelError(t *testing.T) {
	tmp := t.TempDir()
	if err := os.WriteFile(filepath.Join(tmp, "f.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	orig := filepathRelFn
	filepathRelFn = func(basepath, targpath string) (string, error) {
		return "", errors.New("forced rel error")
	}
	t.Cleanup(func() { filepathRelFn = orig })

	if _, err := snapshotSpecTree(tmp); err == nil {
		t.Fatal("expected error from snapshotSpecTree on forced Rel error")
	}
}

// covLintBeforeSnapshotError exercises LintWithResult's before-snapshot error
// path via the snapshotSpecTreeFn seam.
func TestCov_LintWithResult_BeforeSnapshotError(t *testing.T) {
	tmp := t.TempDir()
	writeFeatureReadme(t, tmp, "f", "# Feature: F\n\n**Status:** Draft\n\n## Summary\nx\n")

	orig := snapshotSpecTreeFn
	snapshotSpecTreeFn = func(string) (map[string][32]byte, error) {
		return nil, errors.New("forced before snapshot error")
	}
	t.Cleanup(func() { snapshotSpecTreeFn = orig })

	if _, err := LintWithResult(Options{SpecRoot: tmp, Rules: []string{"adherence-footer"}, Fix: true}); err == nil {
		t.Fatal("expected before-snapshot error")
	}
}

// covLintAfterSnapshotError exercises LintWithResult's after-snapshot error path:
// the first snapshot call succeeds, the second fails.
func TestCov_LintWithResult_AfterSnapshotError(t *testing.T) {
	tmp := t.TempDir()
	writeFeatureReadme(t, tmp, "f", "# Feature: F\n\n**Status:** Draft\n\n## Summary\nx\n")

	orig := snapshotSpecTreeFn
	calls := 0
	snapshotSpecTreeFn = func(string) (map[string][32]byte, error) {
		calls++
		if calls == 1 {
			return map[string][32]byte{}, nil
		}
		return nil, errors.New("forced after snapshot error")
	}
	t.Cleanup(func() { snapshotSpecTreeFn = orig })

	if _, err := LintWithResult(Options{SpecRoot: tmp, Rules: []string{"adherence-footer"}, Fix: true}); err == nil {
		t.Fatal("expected after-snapshot error")
	}
}

// covImmutabilityParseError exercises checkDecisionImmutability's
// parseDecisionFromContent error branch via the parseDecisionFromContentFn seam.
// The decision must be Accepted and committed so the parse call is reached.
func TestCov_CheckDecisionImmutability_ParseError(t *testing.T) {
	root := setupGitRepo(t, map[string]string{
		"decisions/0001-test.md": acceptedDecisionContent(),
	})

	orig := parseDecisionFromContentFn
	parseDecisionFromContentFn = func(content, relPath string, archived bool) (*parsedDecision, error) {
		return nil, errors.New("forced parse error")
	}
	t.Cleanup(func() { parseDecisionFromContentFn = orig })

	vs, err := checkDecisionImmutability(root)
	if err != nil {
		t.Fatal(err)
	}
	// On parse error the decision is skipped, so no violations.
	if len(vs) != 0 {
		t.Fatalf("expected no violations on parse error, got %v", vs)
	}
}
