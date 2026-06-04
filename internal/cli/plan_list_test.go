package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writePlanInDir writes a single-file plan at plansDir/<slug>.md with the
// given status.
func writePlanInDir(t *testing.T, plansDir, slug, status string) {
	t.Helper()
	content := "# Plan: " + slug + "\n\n**Status:** " + status + "\n\n## Tasks\n\n### Task 1: x\n\nBody.\n"
	if err := os.WriteFile(filepath.Join(plansDir, slug+".md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write plan %s: %v", slug, err)
	}
}

// TestPlanList_DefaultListingPipeable verifies cli/plan/list#ac:default-listing-pipeable.
func TestPlanList_DefaultListingPipeable(t *testing.T) {
	plansDir := setupPlansSpec(t)

	// Write deliberately out of order.
	writePlanInDir(t, plansDir, "studio-toolbar", "Draft")
	writePlanInDir(t, plansDir, "cli-event", "Draft")
	writePlanInDir(t, plansDir, "cli-rules", "Draft")

	stdout, _, err := runPlan(t, "list")
	if err != nil {
		t.Fatalf("plan list: %v", err)
	}
	// Exactly three lines, sorted, with a single trailing newline (no extra blank).
	want := "cli-event\ncli-rules\nstudio-toolbar\n"
	if stdout != want {
		t.Errorf("stdout = %q, want %q", stdout, want)
	}
}

// TestPlanList_StatusFilterSelects verifies cli/plan/list#ac:status-filter-selects.
func TestPlanList_StatusFilterSelects(t *testing.T) {
	plansDir := setupPlansSpec(t)

	writePlanInDir(t, plansDir, "approved-one", "Approved")
	writePlanInDir(t, plansDir, "completed-one", "Completed")

	// Case-insensitive match.
	stdout, _, err := runPlan(t, "list", "--status", "approved")
	if err != nil {
		t.Fatalf("plan list --status approved: %v", err)
	}
	lines := nonEmptyLines(stdout)
	if len(lines) != 1 || lines[0] != "approved-one" {
		t.Errorf("expected [approved-one], got %v", lines)
	}
}

// TestPlanList_EmptyMatchExitsZero verifies cli/plan/list#ac:empty-match-exits-zero.
func TestPlanList_EmptyMatchExitsZero(t *testing.T) {
	plansDir := setupPlansSpec(t)

	writePlanInDir(t, plansDir, "approved-one", "Approved")

	stdout, _, err := runPlan(t, "list", "--status", "Deprecated")
	if err != nil {
		t.Fatalf("plan list --status Deprecated: %v (expected exit 0)", err)
	}
	if stdout != "" {
		t.Errorf("expected empty stdout, got %q", stdout)
	}
}

// TestPlanList_EmptyProject — no plans lists nothing, exit 0.
func TestPlanList_EmptyProject(t *testing.T) {
	setupPlansSpec(t)

	stdout, _, err := runPlan(t, "list")
	if err != nil {
		t.Fatalf("plan list: %v", err)
	}
	if stdout != "" {
		t.Errorf("expected empty stdout, got %q", stdout)
	}
}

// TestPlanList_InvalidFormat — bad --format is rejected (exit 2).
func TestPlanList_InvalidFormat(t *testing.T) {
	setupPlansSpec(t)

	_, _, err := runPlan(t, "list", "--format", "csv")
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
	if !strings.Contains(err.Error(), "invalid --format") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestPlanList_FormatJSON — json emits {slug, status} entries honoring the filter.
func TestPlanList_FormatJSON(t *testing.T) {
	plansDir := setupPlansSpec(t)

	writePlanInDir(t, plansDir, "approved-one", "Approved")
	writePlanInDir(t, plansDir, "draft-one", "Draft")

	stdout, _, err := runPlan(t, "list", "--format", "json", "--status", "Approved")
	if err != nil {
		t.Fatalf("plan list --format json: %v", err)
	}
	if !strings.Contains(stdout, `"slug": "approved-one"`) {
		t.Errorf("json missing approved-one slug: %s", stdout)
	}
	if !strings.Contains(stdout, `"status": "Approved"`) {
		t.Errorf("json missing status: %s", stdout)
	}
	if strings.Contains(stdout, "draft-one") {
		t.Errorf("json should not include filtered-out draft-one: %s", stdout)
	}
}

// TestPlanList_FormatYAML — yaml output emits slug/status fields.
func TestPlanList_FormatYAML(t *testing.T) {
	plansDir := setupPlansSpec(t)

	writePlanInDir(t, plansDir, "yaml-plan", "Draft")

	stdout, _, err := runPlan(t, "list", "--format", "yaml")
	if err != nil {
		t.Fatalf("plan list --format yaml: %v", err)
	}
	if !strings.Contains(stdout, "slug: yaml-plan") {
		t.Errorf("yaml missing slug: %s", stdout)
	}
	if !strings.Contains(stdout, "status: Draft") {
		t.Errorf("yaml missing status: %s", stdout)
	}
}
