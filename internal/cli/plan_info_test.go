package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// writePlanRaw writes the given content verbatim to plansDir/<slug>.md.
func writePlanRaw(t *testing.T, plansDir, slug, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(plansDir, slug+".md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write plan %s: %v", slug, err)
	}
}

// TestPlanInfo_ReturnsMetadata verifies cli/plan/info#ac:info-returns-metadata.
func TestPlanInfo_ReturnsMetadata(t *testing.T) {
	plansDir := setupPlansSpec(t)

	writePlanWithSource(t, plansDir, "cli-rules", "Completed", "cli/rules")

	stdout, _, err := runPlan(t, "info", "cli-rules")
	if err != nil {
		t.Fatalf("plan info cli-rules: %v", err)
	}
	if !strings.Contains(stdout, "slug: cli-rules") {
		t.Errorf("yaml missing slug: %s", stdout)
	}
	if !strings.Contains(stdout, "status: Completed") {
		t.Errorf("yaml missing status: %s", stdout)
	}
	if !strings.Contains(stdout, "source_feature: cli/rules") {
		t.Errorf("yaml missing source_feature: %s", stdout)
	}
}

// TestPlanInfo_ReturnsTaskRollup verifies cli/plan/info#ac:info-returns-task-rollup.
func TestPlanInfo_ReturnsTaskRollup(t *testing.T) {
	plansDir := setupPlansSpec(t)

	var b strings.Builder
	b.WriteString("# Plan: eight-done\n\n**Status:** Implementing\n\n## Tasks\n\n")
	for i := 1; i <= 8; i++ {
		b.WriteString("### Task ")
		b.WriteString(itoa(i))
		b.WriteString(": work\n\n**Status:** done\n\nBody.\n\n")
	}
	writePlanRaw(t, plansDir, "eight-done", b.String())

	stdout, _, err := runPlan(t, "info", "eight-done")
	if err != nil {
		t.Fatalf("plan info eight-done: %v", err)
	}
	for _, want := range []string{
		"total: 8",
		"done: 8",
		"in-progress: 0",
		"pending: 0",
		"blocked: 0",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("rollup missing %q in: %s", want, stdout)
		}
	}
}

// TestPlanInfo_NotFoundExits3 verifies cli/plan/info#ac:not-found-exits-3.
func TestPlanInfo_NotFoundExits3(t *testing.T) {
	setupPlansSpec(t)

	stdout, _, err := runPlan(t, "info", "does-not-exist")
	if err == nil {
		t.Fatal("expected error for missing plan, got nil")
	}
	if got := exitCodeOfErr(err); got != exitcode.NotFound {
		t.Errorf("exit code = %d, want %d (NotFound)", got, exitcode.NotFound)
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("error should name the missing slug, got: %v", err)
	}
	if stdout != "" {
		t.Errorf("expected no partial stdout, got: %q", stdout)
	}
}

// TestPlanInfo_MissingSlugExits2 — no positional argument is exit 2.
func TestPlanInfo_MissingSlugExits2(t *testing.T) {
	setupPlansSpec(t)

	_, _, err := runPlan(t, "info")
	if err == nil {
		t.Fatal("expected error for missing slug, got nil")
	}
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit code = %d, want %d (InvalidArgs)", got, exitcode.InvalidArgs)
	}
}

// TestPlanInfo_AbsentStatusExitsZero guards the no-omitempty requirement:
// a plan without a **Status:** line still emits a status key and exits 0.
func TestPlanInfo_AbsentStatusExitsZero(t *testing.T) {
	plansDir := setupPlansSpec(t)

	writePlanRaw(t, plansDir, "no-status",
		"# Plan: no-status\n\n## Tasks\n\n### Task 1: x\n\nBody.\n")

	stdout, _, err := runPlan(t, "info", "no-status")
	if err != nil {
		t.Fatalf("plan info no-status: %v (expected exit 0)", err)
	}
	if !strings.Contains(stdout, "status:") {
		t.Errorf("expected status key present even when empty, got: %s", stdout)
	}
}

// TestPlanInfo_InvalidFormatExits2 — a bad --format value is rejected (exit 2).
func TestPlanInfo_InvalidFormatExits2(t *testing.T) {
	plansDir := setupPlansSpec(t)
	writePlanInDir(t, plansDir, "any-plan", "Draft")

	_, _, err := runPlan(t, "info", "any-plan", "--format", "csv")
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit code = %d, want %d (InvalidArgs)", got, exitcode.InvalidArgs)
	}
}

// itoa is a tiny local helper to avoid importing strconv in the test.
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
