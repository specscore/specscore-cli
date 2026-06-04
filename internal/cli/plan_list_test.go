package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
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

// writePlanWithSource writes a single-file plan with a status and a
// Source Feature field.
func writePlanWithSource(t *testing.T, plansDir, slug, status, sourceFeature string) {
	t.Helper()
	content := "# Plan: " + slug + "\n\n**Status:** " + status +
		"\n\n**Source Feature:** " + sourceFeature +
		"\n\n## Tasks\n\n### Task 1: x\n\nBody.\n"
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

// TestPlanList_FieldsReturnsYAML verifies cli/plan/list#ac:fields-returns-yaml.
func TestPlanList_FieldsReturnsYAML(t *testing.T) {
	plansDir := setupPlansSpec(t)

	writePlanWithSource(t, plansDir, "fielded-plan", "Approved", "cli/plan/list")

	stdout, _, err := runPlan(t, "list", "--fields", "status,source-feature")
	if err != nil {
		t.Fatalf("plan list --fields: %v", err)
	}
	if !strings.Contains(stdout, "slug: fielded-plan") {
		t.Errorf("yaml missing slug key: %s", stdout)
	}
	if !strings.Contains(stdout, "status: Approved") {
		t.Errorf("yaml missing status key: %s", stdout)
	}
	if !strings.Contains(stdout, "source-feature: cli/plan/list") {
		t.Errorf("yaml missing source-feature key: %s", stdout)
	}
}

// TestPlanList_FieldsUpgradesTextToYAML verifies the text->yaml auto-upgrade
// of cli/plan/list#ac:fields-returns-yaml.
func TestPlanList_FieldsUpgradesTextToYAML(t *testing.T) {
	plansDir := setupPlansSpec(t)

	writePlanInDir(t, plansDir, "upgraded-plan", "Draft")

	stdout, _, err := runPlan(t, "list", "--format", "text", "--fields", "status")
	if err != nil {
		t.Fatalf("plan list --format text --fields status: %v", err)
	}
	if !strings.HasPrefix(strings.TrimSpace(stdout), "- ") {
		t.Errorf("expected YAML list output, got: %s", stdout)
	}
	if !strings.Contains(stdout, "status: Draft") {
		t.Errorf("expected status key in upgraded output, got: %s", stdout)
	}
}

// TestPlanList_UnknownFieldExitsTwo verifies cli/plan/list#ac:unknown-field-exits-2.
func TestPlanList_UnknownFieldExitsTwo(t *testing.T) {
	setupPlansSpec(t)

	_, _, err := runPlan(t, "list", "--fields", "bogus")
	if err == nil {
		t.Fatal("expected error for unknown field")
	}
	if got := exitCodeOf(err); got != exitcode.InvalidArgs {
		t.Errorf("exit code = %d, want %d (InvalidArgs)", got, exitcode.InvalidArgs)
	}
	if !strings.Contains(err.Error(), "bogus") {
		t.Errorf("error should name offending field: %v", err)
	}
}

// TestPlanList_FieldsEmptyValueKeyPresent guards the no-omitempty requirement:
// a plan with an empty status still emits a status key.
func TestPlanList_FieldsEmptyValueKeyPresent(t *testing.T) {
	plansDir := setupPlansSpec(t)

	writePlanInDir(t, plansDir, "no-status-plan", "")

	stdout, _, err := runPlan(t, "list", "--fields", "status")
	if err != nil {
		t.Fatalf("plan list --fields status: %v", err)
	}
	if !strings.Contains(stdout, "status:") {
		t.Errorf("expected status key present even when empty, got: %s", stdout)
	}
}
