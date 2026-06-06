package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// writeUnmigratedFeature drops a frontmatter-less feature README into a project.
func writeUnmigratedFeature(t *testing.T, root, slug string) string {
	t.Helper()
	dir := filepath.Join(root, "spec", "features", slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "README.md")
	body := "# Feature: " + slug + "\n\n**Status:** Draft\n\n## Summary\n\nx\n\n---\n*This document follows the https://specscore.md/feature-specification*\n"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestSpecMigrate_MigratesAndReports(t *testing.T) {
	root := setupLintCleanProject(t)
	p := writeUnmigratedFeature(t, root, "auth")

	stdout, _, err := runSpec(t, "migrate", "--project", root)
	if err != nil {
		t.Fatalf("spec migrate: %v", err)
	}
	if !strings.Contains(stdout, "Migrated") || !strings.Contains(stdout, "features/auth/README.md") {
		t.Errorf("unexpected stdout:\n%s", stdout)
	}
	got, _ := os.ReadFile(p)
	if !strings.HasPrefix(string(got), "---\nformat: https://specscore.md/feature-specification\nstatus: Draft\n---") {
		t.Errorf("feature not migrated:\n%s", got)
	}
}

func TestSpecMigrate_AlreadyMigrated(t *testing.T) {
	root := setupLintCleanProject(t)
	writeUnmigratedFeature(t, root, "auth")
	if _, _, err := runSpec(t, "migrate", "--project", root); err != nil {
		t.Fatalf("first migrate: %v", err)
	}
	stdout, _, err := runSpec(t, "migrate", "--project", root)
	if err != nil {
		t.Fatalf("second migrate: %v", err)
	}
	if !strings.Contains(stdout, "Already migrated") {
		t.Errorf("expected idempotent message, got:\n%s", stdout)
	}
}

func TestSpecMigrate_NoConfigRoot(t *testing.T) {
	dir := t.TempDir() // no specscore.yaml
	_, _, err := runSpec(t, "migrate", "--project", dir)
	if err == nil {
		t.Fatal("expected error when no specscore.yaml anchors the project")
	}
}

func TestSpecMigrate_Error(t *testing.T) {
	orig := lintMigrateFn
	t.Cleanup(func() { lintMigrateFn = orig })
	lintMigrateFn = func(string) ([]string, error) { return nil, errors.New("boom") }

	root := setupLintCleanProject(t)
	_, _, err := runSpec(t, "migrate", "--project", root)
	if err == nil {
		t.Fatal("expected migrate error")
	}
	if got := exitCodeOf(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d (Unexpected)", got, exitcode.Unexpected)
	}
}
