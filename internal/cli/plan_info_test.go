package cli

import (
	"encoding/json"
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

func TestPlanInfo_ReturnsPrerequisitePlans(t *testing.T) {
	plansDir := setupPlansSpec(t)
	writePlanRaw(t, plansDir, "delivery", "# Plan: Delivery\n\n**Status:** Draft\n**Prerequisite Plans:** foundation, integration\n")

	stdout, _, err := runPlan(t, "info", "delivery")
	if err != nil {
		t.Fatalf("plan info delivery: %v", err)
	}
	if !strings.Contains(stdout, "prerequisite_plans:\n  - foundation\n  - integration") {
		t.Errorf("yaml missing prerequisite plans: %s", stdout)
	}

	text, _, err := runPlan(t, "info", "delivery", "--format", "text")
	if err != nil {
		t.Fatalf("plan info delivery --format text: %v", err)
	}
	if !strings.Contains(text, "Prerequisite plans: foundation, integration") {
		t.Errorf("text missing prerequisite plans: %s", text)
	}
}

// TestPlanInfo_AbsentPrerequisitePlansIsAnEmptyCollection verifies that an
// absent header is stable for both machine and human consumers.
func TestPlanInfo_AbsentPrerequisitePlansIsAnEmptyCollection(t *testing.T) {
	plansDir := setupPlansSpec(t)
	writePlanRaw(t, plansDir, "delivery", "# Plan: Delivery\n\n**Status:** Draft\n")

	jsonOutput, _, err := runPlan(t, "info", "delivery", "--format", "json")
	if err != nil {
		t.Fatalf("plan info delivery --format json: %v", err)
	}
	var doc map[string]json.RawMessage
	if err := json.Unmarshal([]byte(jsonOutput), &doc); err != nil {
		t.Fatalf("decode JSON output: %v\n%s", err, jsonOutput)
	}
	if got := string(doc["prerequisite_plans"]); got != "[]" {
		t.Errorf("prerequisite_plans = %s, want []", got)
	}

	text, _, err := runPlan(t, "info", "delivery", "--format", "text")
	if err != nil {
		t.Fatalf("plan info delivery --format text: %v", err)
	}
	if !strings.Contains(text, "Prerequisite plans: none") {
		t.Errorf("text missing absent-prerequisites rendering: %s", text)
	}
}

// TestPlanInfo_ReturnsTaskRollup verifies cli/plan/info#ac:info-returns-task-rollup.
func TestPlanInfo_ReturnsTaskRollup(t *testing.T) {
	plansDir := setupPlansSpec(t)

	var b strings.Builder
	b.WriteString("# Plan: eight-complete\n\n**Status:** Implementing\n\n## Tasks\n\n")
	for i := 1; i <= 8; i++ {
		b.WriteString("### Task ")
		b.WriteString(itoa(i))
		b.WriteString(": work\n\n**Status:** complete\n\nBody.\n\n")
	}
	writePlanRaw(t, plansDir, "eight-complete", b.String())

	stdout, _, err := runPlan(t, "info", "eight-complete")
	if err != nil {
		t.Fatalf("plan info eight-complete: %v", err)
	}
	for _, want := range []string{
		"total: 8",
		"complete: 8",
		"in_progress: 0",
		"planning: 0",
		"queued: 0",
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

// TestPlanInfo_ImplementationEvidence verifies
// implementation-commit-provenance#ac:plan-evidence-rolls-up: `plan info`
// surfaces the derived SET of the plan's tasks' `**Implemented-by:**` refs.
func TestPlanInfo_ImplementationEvidence(t *testing.T) {
	plansDir := setupPlansSpec(t)

	writePlanRaw(t, plansDir, "evi",
		"# Plan: evi\n\n**Status:** Implementing\n\n## Tasks\n\n"+
			"### Task 1: a\n\n**Status:** complete\n**Implemented-by:** ref-one\n\n"+
			"### Task 2: b\n\n**Status:** complete\n**Implemented-by:** ref-two\n")

	stdout, _, err := runPlan(t, "info", "evi")
	if err != nil {
		t.Fatalf("plan info evi: %v", err)
	}
	for _, want := range []string{"implementation_evidence:", "ref-one", "ref-two"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("evidence output missing %q in: %s", want, stdout)
		}
	}
}

// TestPlanInfo_ImplementationEvidence_None: a plan with no task provenance still
// emits the evidence key (present-but-empty), exiting 0.
func TestPlanInfo_ImplementationEvidence_None(t *testing.T) {
	plansDir := setupPlansSpec(t)

	writePlanRaw(t, plansDir, "no-evi",
		"# Plan: no-evi\n\n**Status:** Implementing\n\n## Tasks\n\n### Task 1: a\n\n**Status:** complete\n")

	stdout, _, err := runPlan(t, "info", "no-evi")
	if err != nil {
		t.Fatalf("plan info no-evi: %v", err)
	}
	if !strings.Contains(stdout, "implementation_evidence: []") {
		t.Errorf("expected empty evidence set, got: %s", stdout)
	}
}

// TestPlanInfo_SnapshotsStayDistinct verifies
// implementation-commit-provenance#ac:snapshots-stay-distinct: a plan with both
// a `## Snapshots` Git Hash and task provenance surfaces them as distinct
// records — the snapshot hash is not pulled into the evidence list.
func TestPlanInfo_SnapshotsStayDistinct(t *testing.T) {
	plansDir := setupPlansSpec(t)

	const snapHash = "specstatehash999"
	writePlanRaw(t, plansDir, "both",
		"# Plan: both\n\n**Status:** Implementing\n\n"+
			"## Snapshots\n\n| Date | Git Hash |\n| - | - |\n| 2026-06-25 | "+snapHash+" |\n\n"+
			"## Tasks\n\n### Task 1: a\n\n**Status:** complete\n**Implemented-by:** codehash111\n")

	stdout, _, err := runPlan(t, "info", "both")
	if err != nil {
		t.Fatalf("plan info both: %v", err)
	}
	if !strings.Contains(stdout, "codehash111") {
		t.Errorf("expected implementation ref in evidence, got: %s", stdout)
	}
	// The snapshot Git Hash must not appear in the evidence section. Since
	// `plan info` does not surface Snapshots at all, it must not appear anywhere.
	if strings.Contains(stdout, snapHash) {
		t.Errorf("snapshot Git Hash leaked into plan info output: %s", stdout)
	}
}

// TestPlanInfo_ImplementationEvidence_Text covers the text format branch.
func TestPlanInfo_ImplementationEvidence_Text(t *testing.T) {
	plansDir := setupPlansSpec(t)

	writePlanRaw(t, plansDir, "evi-text",
		"# Plan: evi-text\n\n**Status:** Implementing\n\n## Tasks\n\n"+
			"### Task 1: a\n\n**Status:** complete\n**Implemented-by:** ref-one\n")

	stdout, _, err := runPlan(t, "info", "evi-text", "--format", "text")
	if err != nil {
		t.Fatalf("plan info evi-text: %v", err)
	}
	if !strings.Contains(stdout, "Implementation evidence:") || !strings.Contains(stdout, "task 1: ref-one") {
		t.Errorf("text evidence missing, got: %s", stdout)
	}
}

// TestPlanInfo_ImplementationEvidence_TextNone covers the "none" text branch.
func TestPlanInfo_ImplementationEvidence_TextNone(t *testing.T) {
	plansDir := setupPlansSpec(t)

	writePlanRaw(t, plansDir, "no-evi-text",
		"# Plan: no-evi-text\n\n**Status:** Implementing\n\n## Tasks\n\n### Task 1: a\n\n**Status:** complete\n")

	stdout, _, err := runPlan(t, "info", "no-evi-text", "--format", "text")
	if err != nil {
		t.Fatalf("plan info no-evi-text: %v", err)
	}
	if !strings.Contains(stdout, "Implementation evidence: none") {
		t.Errorf("expected 'none' evidence line, got: %s", stdout)
	}
}
