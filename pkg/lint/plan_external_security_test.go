package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExternalPlanFixRejectsSymlinkWithoutTouchingTarget(t *testing.T) {
	specRoot, plansDir := filepath.Join(t.TempDir(), "spec"), t.TempDir()
	if err := os.MkdirAll(specRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.md")
	body := "---\nformat: https://specscore.md/plan-specification\nstatus: Approved\n---\n# Plan: Escape\n**Status:** Approved\n**Source:** none\n"
	if err := os.WriteFile(outside, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(plansDir, "escape.md")); err != nil {
		t.Skip(err)
	}
	_, err := Lint(Options{SpecRoot: specRoot, PlansDir: plansDir, Rules: []string{"status-mirror"}, Fix: true})
	if err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("fix error = %v", err)
	}
	after, readErr := os.ReadFile(outside)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if string(after) != body {
		t.Fatal("external symlink target changed")
	}
}
