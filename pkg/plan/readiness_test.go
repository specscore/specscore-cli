package plan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readinessPlanBody(status, prerequisite, taskStatus string) string {
	prerequisiteLine := ""
	if prerequisite != "" {
		prerequisiteLine = "**Prerequisite Plans:** " + prerequisite + "\n"
	}
	tasks := ""
	if taskStatus != "" {
		tasks = "\n## Tasks\n\n### Task 1: Work\n\n**Status:** " + taskStatus + "\n"
	}
	return "# Plan: Test\n\n**Status:** " + status + "\n" + prerequisiteLine + tasks
}

func writeReadinessPlan(t *testing.T, plansDir, slug, status, prerequisite, taskStatus string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(plansDir, slug+".md"), []byte(readinessPlanBody(status, prerequisite, taskStatus)), 0o644); err != nil {
		t.Fatalf("write plan %s: %v", slug, err)
	}
}

func TestPrerequisiteReadiness_AllImplemented(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// The recorded status deliberately stays Approved: readiness relies on the
	// derived task rollup, not this field.
	writeReadinessPlan(t, plansDir, "foundation", "Approved", "", "complete")
	writeReadinessPlan(t, plansDir, "integration", "Approved", "", "complete")
	writeReadinessPlan(t, plansDir, "delivery", "Approved", "foundation, integration", "queued")

	p, err := Parse(filepath.Join(plansDir, "delivery.md"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.PrerequisiteReadiness(plansDir)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Ready || len(got.Unmet) != 0 {
		t.Errorf("readiness = %+v, want ready", got)
	}
}

func TestPrerequisiteReadiness_ReportsEveryUnmetSlugAndStatus(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeReadinessPlan(t, plansDir, "foundation", "Approved", "", "queued")
	writeReadinessPlan(t, plansDir, "integration", "Executing", "", "in_progress")
	writeReadinessPlan(t, plansDir, "delivery", "Approved", "foundation, integration", "queued")

	p, err := Parse(filepath.Join(plansDir, "delivery.md"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.PrerequisiteReadiness(plansDir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Ready || len(got.Unmet) != 2 {
		t.Fatalf("readiness = %+v, want two unmet prerequisites", got)
	}
	if got.Unmet[0] != (UnmetPrerequisite{Slug: "foundation", Status: "Approved"}) {
		t.Errorf("first unmet = %+v", got.Unmet[0])
	}
	if got.Unmet[1] != (UnmetPrerequisite{Slug: "integration", Status: "Executing", DerivedStatus: "Executing"}) {
		t.Errorf("second unmet = %+v", got.Unmet[1])
	}
	for _, want := range []string{"foundation", "Approved", "integration", "Executing"} {
		if !strings.Contains(got.UnmetMessage(), want) {
			t.Errorf("UnmetMessage %q missing %q", got.UnmetMessage(), want)
		}
	}
}

func TestPrerequisiteReadiness_UsesUnsetForPrerequisiteWithoutRecordedStatus(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeReadinessPlan(t, plansDir, "foundation", "", "", "queued")
	writeReadinessPlan(t, plansDir, "delivery", "Approved", "foundation", "queued")
	p, err := Parse(filepath.Join(plansDir, "delivery.md"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.PrerequisiteReadiness(plansDir)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Unmet) != 1 || got.Unmet[0].Status != "unset" {
		t.Errorf("readiness = %+v, want prerequisite status unset", got)
	}
}

func TestPrerequisiteReadiness_NoPrerequisitesIsReady(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeReadinessPlan(t, plansDir, "delivery", "Approved", "", "queued")
	p, err := Parse(filepath.Join(plansDir, "delivery.md"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.PrerequisiteReadiness(plansDir)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Ready || len(got.Unmet) != 0 {
		t.Errorf("readiness = %+v, want ready", got)
	}
}

func TestPrerequisiteReadiness_MalformedDeclarationIsNeverReady(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeReadinessPlan(t, plansDir, "delivery", "Approved", "foundation,", "queued")
	p, err := Parse(filepath.Join(plansDir, "delivery.md"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.PrerequisiteReadiness(plansDir)
	if err != nil {
		t.Fatal(err)
	}
	if got.Ready || len(got.Unmet) == 0 || got.Unmet[0].Status != "invalid" {
		t.Errorf("readiness = %+v, want malformed declaration to be unready", got)
	}
}

func TestPrerequisiteReadiness_DirectoryFormPrerequisite(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "spec", "plans")
	dir := filepath.Join(plansDir, "foundation")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(readinessPlanBody("Approved", "", "complete")), 0o644); err != nil {
		t.Fatal(err)
	}
	writeReadinessPlan(t, plansDir, "delivery", "Approved", "foundation", "queued")
	p, err := Parse(filepath.Join(plansDir, "delivery.md"))
	if err != nil {
		t.Fatal(err)
	}
	got, err := p.PrerequisiteReadiness(plansDir)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Ready {
		t.Errorf("directory-form prerequisite readiness = %+v, want ready", got)
	}
}

func TestMalformedPrerequisiteDeclaration(t *testing.T) {
	tests := []struct {
		name string
		plan Plan
		want string
	}{
		{"absent", Plan{}, ""},
		{"em-dash", Plan{PrerequisiteLine: 1, PrerequisiteRaw: "—"}, ""},
		{"empty", Plan{PrerequisiteLine: 1, PrerequisiteRaw: " "}, "empty"},
		{"invalid", Plan{PrerequisiteLine: 1, PrerequisiteRaw: "Bad_Slug"}, "invalid slug"},
		{"duplicate", Plan{PrerequisiteLine: 1, PrerequisiteRaw: "alpha, alpha"}, "duplicate slug"},
		{"self", Plan{Slug: "alpha", PrerequisiteLine: 1, PrerequisiteRaw: "alpha"}, "self reference"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := malformedPrerequisiteDeclaration(&tt.plan); got != tt.want {
				t.Errorf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestPlanReadiness_ResolvesAndReportsReadError(t *testing.T) {
	root := t.TempDir()
	plansDir := filepath.Join(root, "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeReadinessPlan(t, plansDir, "delivery", "Approved", "", "queued")
	got, err := PlanReadiness(root, "delivery")
	if err != nil || !got.Ready {
		t.Fatalf("PlanReadiness = %+v, %v", got, err)
	}

	writeReadinessPlan(t, plansDir, "blocked", "Approved", "", "queued")
	originalParse := parseReadinessPlan
	parseReadinessPlan = func(string) (*Plan, error) { return nil, os.ErrPermission }
	t.Cleanup(func() { parseReadinessPlan = originalParse })
	if _, err := PlanReadiness(root, "blocked"); err == nil {
		t.Fatal("expected parse error")
	}
	parseReadinessPlan = originalParse
	if _, err := PlanReadiness(root, "missing"); err == nil {
		t.Fatal("expected missing plan error")
	}
}

func TestPrerequisiteReadiness_PrerequisiteParseFailure(t *testing.T) {
	plansDir := filepath.Join(t.TempDir(), "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeReadinessPlan(t, plansDir, "foundation", "Approved", "", "complete")
	writeReadinessPlan(t, plansDir, "delivery", "Approved", "foundation", "queued")
	p, err := Parse(filepath.Join(plansDir, "delivery.md"))
	if err != nil {
		t.Fatal(err)
	}
	originalParse := parseReadinessPlan
	parseReadinessPlan = func(string) (*Plan, error) { return nil, os.ErrPermission }
	t.Cleanup(func() { parseReadinessPlan = originalParse })
	if _, err := p.PrerequisiteReadiness(plansDir); err == nil {
		t.Fatal("expected prerequisite parse error")
	}
}
