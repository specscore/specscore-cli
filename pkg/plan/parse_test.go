package plan

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writePlan writes a Plan-shaped file at plansDir/<slug>.md and returns its path.
func writePlan(t *testing.T, plansDir, slug, content string) string {
	t.Helper()
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(plansDir, slug+".md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParse_SourceIdeaAndNone(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "plans")

	idea := `# Plan: Idea Sourced

**Status:** Draft
**Source:** idea:cloud-run-migration

## Summary

x
`
	p, err := Parse(writePlan(t, plansDir, "idea", idea))
	if err != nil {
		t.Fatal(err)
	}
	if p.SourceIdea != "cloud-run-migration" {
		t.Errorf("SourceIdea = %q, want cloud-run-migration", p.SourceIdea)
	}
	if p.SourceFeature != "" || p.SourceNone {
		t.Errorf("idea plan should not set SourceFeature/SourceNone: %+v", p)
	}

	none := `# Plan: Source Less

**Status:** Draft
**Source:** none

## Summary

x
`
	p2, err := Parse(writePlan(t, plansDir, "none", none))
	if err != nil {
		t.Fatal(err)
	}
	if !p2.SourceNone {
		t.Errorf("SourceNone = false, want true")
	}
	if p2.SourceIdea != "" || p2.SourceFeature != "" {
		t.Errorf("source-less plan should set neither Feature nor Idea: %+v", p2)
	}
}

func TestParse_PrerequisitePlansRetainsRawEmptyEntryForLint(t *testing.T) {
	dir := t.TempDir()
	p, err := Parse(writePlan(t, filepath.Join(dir, "plans"), "application", `# Plan: Application

**Prerequisite Plans:** foundation,
`))
	if err != nil {
		t.Fatal(err)
	}
	if p.PrerequisiteRaw != "foundation," {
		t.Fatalf("PrerequisiteRaw = %q, want trailing comma retained", p.PrerequisiteRaw)
	}
	if got, want := strings.Join(p.PrerequisitePlans, ","), "foundation"; got != want {
		t.Fatalf("PrerequisitePlans = %v, want %v", got, want)
	}
	if p.PrerequisiteLine == 0 {
		t.Fatal("PrerequisiteLine must be set for P-009 to report the raw field")
	}
}

func TestParse_PrerequisitePlansRetainsASCIIDashForLint(t *testing.T) {
	dir := t.TempDir()
	p, err := Parse(writePlan(t, filepath.Join(dir, "plans"), "application", `# Plan: Application

**Prerequisite Plans:** -
`))
	if err != nil {
		t.Fatal(err)
	}
	if p.PrerequisiteRaw != "-" {
		t.Fatalf("PrerequisiteRaw = %q, want ASCII dash retained", p.PrerequisiteRaw)
	}
	if got, want := strings.Join(p.PrerequisitePlans, ","), "-"; got != want {
		t.Fatalf("PrerequisitePlans = %q, want %q", got, want)
	}
}

func TestSlugFromPath_DirectoryAndBareReadmeForms(t *testing.T) {
	if got := slugFromPath(filepath.Join("spec", "plans", "auth", "README.md")); got != "auth" {
		t.Errorf("directory-form slug = %q, want auth", got)
	}
	if got := slugFromPath("README.md"); got != "README.md" {
		t.Errorf("bare README slug = %q, want README.md", got)
	}
}

func TestParse_TaskIdField(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "plans")
	body := `# Plan: Sample

**Status:** Draft
**Source Feature:** sample

## Tasks

### Task 1: First task

**Id:** setup
**Status:** in_progress

Body prose.

### Task 2: Second task

**Status:** planning

No id here.
`
	p, err := Parse(writePlan(t, plansDir, "ids", body))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Tasks) != 2 {
		t.Fatalf("got %d tasks", len(p.Tasks))
	}
	t1 := p.Tasks[0]
	if !t1.IdPresent || t1.Id != "setup" {
		t.Fatalf("task1 id present=%v id=%q", t1.IdPresent, t1.Id)
	}
	if t1.IdLine == 0 {
		t.Fatal("task1 IdLine should be set")
	}
	t2 := p.Tasks[1]
	if t2.IdPresent || t2.Id != "" {
		t.Fatalf("task2 id present=%v id=%q", t2.IdPresent, t2.Id)
	}
	if t2.IdLine != 0 {
		t.Fatalf("task2 IdLine should be 0, got %d", t2.IdLine)
	}
}

func TestParse_FullModeMinimal(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "plans")
	body := `# Plan: Sample

**Status:** Draft
**Source Feature:** sample
**Mode:** full

## Summary

Sample plan.

## Tasks

### Task 1: First task

**Verifies:** sample#ac:one
**Status:** planning
**Depends-On:** —

Body prose.

### Task 2: Second task

**Verifies:** sample#ac:two
**Depends-On:** 1

Some prose.
`
	p, err := Parse(writePlan(t, plansDir, "sample", body))
	if err != nil {
		t.Fatal(err)
	}
	if !p.HasPlanTitle {
		t.Fatal("expected HasPlanTitle=true")
	}
	if p.Title != "Sample" {
		t.Fatalf("got title %q", p.Title)
	}
	if p.SourceFeature != "sample" {
		t.Fatalf("got source feature %q", p.SourceFeature)
	}
	if p.Mode != ModeFull || !p.ModeValueValid {
		t.Fatalf("got mode %q valid=%v", p.Mode, p.ModeValueValid)
	}
	if len(p.Tasks) != 2 {
		t.Fatalf("got %d tasks", len(p.Tasks))
	}
	t1 := p.Tasks[0]
	if t1.Number != 1 || t1.Name != "First task" {
		t.Fatalf("task1 = %+v", t1)
	}
	if len(t1.Verifies) != 1 || t1.Verifies[0] != "sample#ac:one" {
		t.Fatalf("task1 verifies = %v", t1.Verifies)
	}
	if t1.Status != StatusPlanning || !t1.StatusValueValid {
		t.Fatalf("task1 status = %v valid=%v", t1.Status, t1.StatusValueValid)
	}
	if len(t1.DependsOn) != 0 || !t1.DependsOnValid {
		t.Fatalf("task1 depends-on = %v valid=%v", t1.DependsOn, t1.DependsOnValid)
	}
	t2 := p.Tasks[1]
	// Status absent → default planning.
	if t2.Status != StatusPlanning {
		t.Fatalf("task2 default status = %v", t2.Status)
	}
	if t2.StatusPresent {
		t.Fatal("task2 status should not be marked present")
	}
	if len(t2.DependsOn) != 1 || t2.DependsOn[0] != 1 {
		t.Fatalf("task2 depends-on = %v", t2.DependsOn)
	}
}

func TestParse_PlanMetadataPopulated(t *testing.T) {
	dir := t.TempDir()
	body := `# Plan: Meta

**Status:** Approved
**Date:** 2026-06-04
**Owner:** someone
**Source Feature:** foo

## Tasks

### Task 1: One

**Verifies:** foo#ac:x
`
	p, err := Parse(writePlan(t, dir, "meta", body))
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != "Approved" {
		t.Fatalf("got status %q", p.Status)
	}
	if p.Date != "2026-06-04" {
		t.Fatalf("got date %q", p.Date)
	}
	if p.Owner != "someone" {
		t.Fatalf("got owner %q", p.Owner)
	}
}

func TestParse_ParentField(t *testing.T) {
	dir := t.TempDir()
	body := `# Plan: Sub

**Status:** Draft
**Source Feature:** foo
**Supersedes:** —
**Parent:** specscore:cross-repo-master

## Tasks

### Task 1: One

**Verifies:** foo#ac:x
`
	p, err := Parse(writePlan(t, dir, "sub", body))
	if err != nil {
		t.Fatal(err)
	}
	if p.Parent != "specscore:cross-repo-master" {
		t.Fatalf("got parent %q", p.Parent)
	}
	if p.ParentLine != 6 {
		t.Fatalf("got parent line %d, want 6", p.ParentLine)
	}
}

func TestParse_CoordinationField(t *testing.T) {
	dir := t.TempDir()
	body := `# Plan: Sub

**Status:** Draft
**Source Feature:** foo
**Supersedes:** —
**Coordination:** specscore/specscore-cli@main

## Tasks

### Task 1: One

**Verifies:** foo#ac:x
`
	p, err := Parse(writePlan(t, dir, "sub", body))
	if err != nil {
		t.Fatal(err)
	}
	if p.Coordination != "specscore/specscore-cli@main" {
		t.Fatalf("got coordination %q", p.Coordination)
	}
	if p.CoordinationLine != 6 {
		t.Fatalf("got coordination line %d, want 6", p.CoordinationLine)
	}
}

func TestParse_CoordinationField_Absent(t *testing.T) {
	dir := t.TempDir()
	body := `# Plan: Sub

**Status:** Draft
**Source Feature:** foo
**Supersedes:** —

## Tasks

### Task 1: One

**Verifies:** foo#ac:x
`
	p, err := Parse(writePlan(t, dir, "sub", body))
	if err != nil {
		t.Fatal(err)
	}
	if p.Coordination != "" || p.CoordinationLine != 0 {
		t.Fatalf("expected absent coordination, got %q at line %d", p.Coordination, p.CoordinationLine)
	}
}

// TestParse_MissingStatusIsEmpty covers cli/plan#ac:missing-status-is-empty:
// a Plan with no `**Status:**` line reports an empty status and Parse succeeds.
func TestParse_MissingStatusIsEmpty(t *testing.T) {
	dir := t.TempDir()
	body := `# Plan: NoStatus

**Source Feature:** foo

## Tasks

### Task 1: One

**Verifies:** foo#ac:x
`
	p, err := Parse(writePlan(t, dir, "no-status", body))
	if err != nil {
		t.Fatal(err)
	}
	if p.Status != "" {
		t.Fatalf("expected empty status, got %q", p.Status)
	}
	if p.StatusLine != 0 {
		t.Fatalf("expected StatusLine=0, got %d", p.StatusLine)
	}
}

func TestParse_ModeAbsentDefaultsFull(t *testing.T) {
	dir := t.TempDir()
	body := `# Plan: NoMode

**Source Feature:** foo

## Tasks

### Task 1: Only

**Verifies:** foo#ac:x
`
	p, err := Parse(writePlan(t, dir, "no-mode", body))
	if err != nil {
		t.Fatal(err)
	}
	if p.Mode != ModeFull {
		t.Fatalf("default mode = %v", p.Mode)
	}
	if p.ModeRawPresent {
		t.Fatal("ModeRawPresent should be false when Mode field absent")
	}
}

func TestParse_StubModeValid(t *testing.T) {
	dir := t.TempDir()
	body := `# Plan: Stubby

**Source Feature:** foo
**Mode:** stub

## Tasks

### Task 1: Pending stub task

**Verifies:** foo#ac:x
**Status:** planning
**Depends-On:** —

<!-- implement: pending -->
`
	p, err := Parse(writePlan(t, dir, "stubby", body))
	if err != nil {
		t.Fatal(err)
	}
	if p.Mode != ModeStub || !p.ModeValueValid {
		t.Fatalf("got mode %q valid=%v", p.Mode, p.ModeValueValid)
	}
	if !p.Tasks[0].HasPlaceholder {
		t.Fatal("expected placeholder detected")
	}
}

func TestParse_InvalidModeValue(t *testing.T) {
	dir := t.TempDir()
	body := `# Plan: BadMode

**Source Feature:** foo
**Mode:** sketch

## Tasks

### Task 1: One

**Verifies:** foo#ac:x
`
	p, err := Parse(writePlan(t, dir, "bad-mode", body))
	if err != nil {
		t.Fatal(err)
	}
	if p.ModeValueValid {
		t.Fatal("expected ModeValueValid=false for 'sketch'")
	}
	if p.ModeRaw != "sketch" {
		t.Fatalf("got raw %q", p.ModeRaw)
	}
	// Default mode preserved so other rules can keep running.
	if p.Mode != ModeFull {
		t.Fatalf("expected default mode preserved, got %v", p.Mode)
	}
}

func TestParse_InvalidStatusValue(t *testing.T) {
	dir := t.TempDir()
	body := `# Plan: Bad

**Source Feature:** foo

## Tasks

### Task 1: Weird

**Verifies:** foo#ac:x
**Status:** waiting
`
	p, err := Parse(writePlan(t, dir, "bad-status", body))
	if err != nil {
		t.Fatal(err)
	}
	t1 := p.Tasks[0]
	if t1.StatusValueValid {
		t.Fatal("expected StatusValueValid=false")
	}
	if t1.StatusRaw != "waiting" {
		t.Fatalf("got raw %q", t1.StatusRaw)
	}
}

// TestParse_FailedAndAbortedStatusesValid covers the extended task-status set:
// `failed` and `aborted` are accepted alongside the original four.
func TestParse_FailedAndAbortedStatusesValid(t *testing.T) {
	dir := t.TempDir()
	body := `# Plan: Extended

**Source Feature:** foo

## Tasks

### Task 1: F
**Verifies:** foo#ac:f
**Status:** failed

### Task 2: A
**Verifies:** foo#ac:a
**Status:** aborted
`
	p, err := Parse(writePlan(t, dir, "ext", body))
	if err != nil {
		t.Fatal(err)
	}
	if p.Tasks[0].Status != StatusFailed || !p.Tasks[0].StatusValueValid {
		t.Fatalf("task1 status=%v valid=%v", p.Tasks[0].Status, p.Tasks[0].StatusValueValid)
	}
	if p.Tasks[1].Status != StatusAborted || !p.Tasks[1].StatusValueValid {
		t.Fatalf("task2 status=%v valid=%v", p.Tasks[1].Status, p.Tasks[1].StatusValueValid)
	}
}

func TestParse_DependsOnVariants(t *testing.T) {
	dir := t.TempDir()
	body := `# Plan: Deps

**Source Feature:** foo

## Tasks

### Task 1: A
**Verifies:** foo#ac:x
**Depends-On:** —

### Task 2: B
**Verifies:** foo#ac:y
**Depends-On:** 1

### Task 3: C
**Verifies:** foo#ac:z
**Depends-On:** 1, 2,

### Task 4: D
**Verifies:** foo#ac:w
**Depends-On:** 1, abc, 3
`
	p, err := Parse(writePlan(t, dir, "deps", body))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Tasks) != 4 {
		t.Fatalf("got %d tasks", len(p.Tasks))
	}
	if got := p.Tasks[0].DependsOn; len(got) != 0 || !p.Tasks[0].DependsOnValid {
		t.Fatalf("t1 deps = %v valid=%v", got, p.Tasks[0].DependsOnValid)
	}
	if got := p.Tasks[1].DependsOn; len(got) != 1 || got[0] != 1 {
		t.Fatalf("t2 deps = %v", got)
	}
	if got := p.Tasks[2].DependsOn; len(got) != 2 || got[0] != 1 || got[1] != 2 {
		t.Fatalf("t3 deps = %v", got)
	}
	if !p.Tasks[2].DependsOnValid {
		t.Fatal("t3 should be valid (trailing comma tolerated)")
	}
	if p.Tasks[3].DependsOnValid {
		t.Fatal("t4 should be invalid (abc token)")
	}
}

func TestParse_DeferredACsParsed(t *testing.T) {
	dir := t.TempDir()
	body := `# Plan: D

**Source Feature:** foo

## Tasks

### Task 1: One

**Verifies:** foo#ac:a

## Deferred AC Coverage

- foo#ac:b — post-MVP
- foo#ac:c — defer per scope cut
`
	p, err := Parse(writePlan(t, dir, "d", body))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.DeferredACs) != 2 {
		t.Fatalf("got %d deferred", len(p.DeferredACs))
	}
	if p.DeferredACs[0].ACID != "foo#ac:b" {
		t.Fatalf("got %q", p.DeferredACs[0].ACID)
	}
}

// TestParse_DeferredACsHyphenatedSlug guards against the separator regex
// truncating an AC slug at the first internal hyphen (e.g. parsing
// `cli/spec/lint#ac:oq-section-missing-flagged` as id `cli/spec/lint#ac:oq`).
func TestParse_DeferredACsHyphenatedSlug(t *testing.T) {
	dir := t.TempDir()
	body := `# Plan: D

**Source Feature:** cli/spec/lint

## Tasks

### Task 1: One

**Verifies:** cli/spec/lint#ac:fix-reports-only-changed-files

## Deferred AC Coverage

- cli/spec/lint#ac:oq-section-missing-flagged — Already implemented and Stable.
- cli/spec/lint#ac:consumer-path-multi-glob-parsed — Already implemented and Stable.
- cli/spec/lint#ac:fix-idempotent
`
	p, err := Parse(writePlan(t, dir, "d", body))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.DeferredACs) != 3 {
		t.Fatalf("got %d deferred", len(p.DeferredACs))
	}
	want := []string{
		"cli/spec/lint#ac:oq-section-missing-flagged",
		"cli/spec/lint#ac:consumer-path-multi-glob-parsed",
		"cli/spec/lint#ac:fix-idempotent",
	}
	for i, w := range want {
		if p.DeferredACs[i].ACID != w {
			t.Errorf("deferred[%d] ACID = %q, want %q", i, p.DeferredACs[i].ACID, w)
		}
	}
	if p.DeferredACs[0].Reason != "Already implemented and Stable." {
		t.Errorf("deferred[0] reason = %q", p.DeferredACs[0].Reason)
	}
}

func TestParse_NotAPlanReturnsHasPlanTitleFalse(t *testing.T) {
	dir := t.TempDir()
	body := `# Random notes

just some markdown
`
	p, err := Parse(writePlan(t, dir, "notes", body))
	if err != nil {
		t.Fatal(err)
	}
	if p.HasPlanTitle {
		t.Fatal("non-plan file should have HasPlanTitle=false")
	}
}

func TestParse_PlaceholderByteExact(t *testing.T) {
	cases := []struct {
		body  string
		match bool
	}{
		{"<!-- implement: pending -->", true},
		{"  <!-- implement: pending -->  ", true}, // surrounding whitespace OK
		{"<!--implement: pending-->", false},
		{"<!-- IMPLEMENT: pending -->", false},
		{"<!-- implement:pending -->", false},
	}
	for _, tc := range cases {
		dir := t.TempDir()
		body := `# Plan: T

**Source Feature:** foo
**Mode:** stub

## Tasks

### Task 1: X
**Verifies:** foo#ac:a
**Status:** planning

` + tc.body + "\n"
		p, err := Parse(writePlan(t, dir, "t", body))
		if err != nil {
			t.Fatal(err)
		}
		got := p.Tasks[0].HasPlaceholder
		if got != tc.match {
			t.Errorf("body=%q want=%v got=%v", tc.body, tc.match, got)
		}
	}
}

func TestIsSingleFilePlanPath(t *testing.T) {
	plansDir := filepath.Join("spec", "plans")
	cases := []struct {
		path string
		want bool
	}{
		{filepath.Join("spec", "plans", "auth.md"), true},
		{filepath.Join("spec", "plans", "auth", "README.md"), false},
		{filepath.Join("spec", "plans", "README.md"), false},
		{filepath.Join("spec", "plans", "auth.txt"), false},
		{filepath.Join("spec", "plans", "nested", "deep.md"), false},
	}
	for _, tc := range cases {
		got := IsSingleFilePlanPath(plansDir, tc.path)
		if got != tc.want {
			t.Errorf("path=%q want=%v got=%v", tc.path, tc.want, got)
		}
	}
}

func TestParse_NonLinearTaskNumbering(t *testing.T) {
	// Parser preserves the heading numbers as-given; P-003 will report
	// the non-linear case.
	dir := t.TempDir()
	body := `# Plan: G

**Source Feature:** foo

## Tasks

### Task 1: A
**Verifies:** foo#ac:a

### Task 3: B
**Verifies:** foo#ac:b

### Task 5: C
**Verifies:** foo#ac:c
`
	p, err := Parse(writePlan(t, dir, "g", body))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Tasks) != 3 {
		t.Fatalf("got %d tasks", len(p.Tasks))
	}
	got := []int{p.Tasks[0].Number, p.Tasks[1].Number, p.Tasks[2].Number}
	if got[0] != 1 || got[1] != 3 || got[2] != 5 {
		t.Fatalf("preserved numbers = %v", got)
	}
}

func TestParse_HeaderFieldsExtractedOnlyFromHeader(t *testing.T) {
	// A `**Source Feature:**` accidentally appearing in body text MUST NOT
	// override the header value.
	dir := t.TempDir()
	body := `# Plan: H

**Source Feature:** real

## Tasks

### Task 1: A
**Verifies:** real#ac:a

This task mentions **Source Feature:** fake in its prose.
`
	p, err := Parse(writePlan(t, dir, "h", body))
	if err != nil {
		t.Fatal(err)
	}
	if p.SourceFeature != "real" {
		t.Fatalf("got %q", p.SourceFeature)
	}
}

func TestParseDependsOn_EmDashSentinel(t *testing.T) {
	deps, ok := parseDependsOn("—")
	if !ok || len(deps) != 0 {
		t.Fatalf("em-dash: deps=%v ok=%v", deps, ok)
	}
	deps, ok = parseDependsOn("")
	if !ok || len(deps) != 0 {
		t.Fatalf("empty: deps=%v ok=%v", deps, ok)
	}
	deps, ok = parseDependsOn("-")
	if !ok || len(deps) != 0 {
		t.Fatalf("ascii-: deps=%v ok=%v", deps, ok)
	}
}

func TestParse_PlaceholderBodyTokenAfterFields(t *testing.T) {
	// Placeholder appears after the task's required body fields.
	body := `# Plan: P

**Source Feature:** foo
**Mode:** stub

## Tasks

### Task 1: Pending
**Verifies:** foo#ac:a
**Status:** planning
**Depends-On:** —

<!-- implement: pending -->
`
	dir := t.TempDir()
	p, err := Parse(writePlan(t, dir, "p", body))
	if err != nil {
		t.Fatal(err)
	}
	if !p.Tasks[0].HasPlaceholder {
		t.Fatal("expected placeholder detected")
	}
	if !strings.Contains(strings.Join(p.Tasks[0].BodyLines, "\n"), PlaceholderBodyToken) {
		t.Fatal("placeholder line missing from body lines")
	}
}

func TestParse_ImplementedByField(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "plans")
	body := `# Plan: Sample

**Status:** Draft
**Source Feature:** sample

## Tasks

### Task 1: First task

**Status:** complete
**Implemented-by:** backstage@a1b2c3d (feature/foo)

### Task 2: Second task

**Status:** planning

No provenance here.
`
	p, err := Parse(writePlan(t, plansDir, "prov", body))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Tasks) != 2 {
		t.Fatalf("got %d tasks", len(p.Tasks))
	}
	t1 := p.Tasks[0]
	if !t1.ImplementedByPresent || t1.ImplementationCommit != "backstage@a1b2c3d (feature/foo)" {
		t.Fatalf("task1 implemented-by present=%v value=%q", t1.ImplementedByPresent, t1.ImplementationCommit)
	}
	if t1.ImplementedByLine == 0 {
		t.Fatal("task1 ImplementedByLine should be set")
	}
	t2 := p.Tasks[1]
	if t2.ImplementedByPresent || t2.ImplementationCommit != "" || t2.ImplementedByLine != 0 {
		t.Fatalf("task2 implemented-by should be absent: present=%v value=%q line=%d", t2.ImplementedByPresent, t2.ImplementationCommit, t2.ImplementedByLine)
	}
}

// TestParse_NoteAndEvidenceFields covers the `**Note:**`/`**Evidence:**`
// optional per-task annotations written by `task change-status
// --note=/--evidence=` (cli/task/change-status). Evidence is a comma-separated
// list; Note is free text. Both are independent of ImplementedBy/
// ImplementationCommit (a syntactically validated code reference — P-008).
func TestParse_NoteAndEvidenceFields(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, "plans")
	body := `# Plan: Sample

**Status:** Draft
**Source Feature:** sample

## Tasks

### Task 1: First task

**Status:** complete
**Implemented-by:** sneat-co/chess@cfabf5e
**Note:** shipped to production, verified live
**Evidence:** cfabf5e, https://chessraiders.com/board/

### Task 2: Second task

**Status:** planning

No note or evidence here.
`
	p, err := Parse(writePlan(t, plansDir, "notes", body))
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Tasks) != 2 {
		t.Fatalf("got %d tasks", len(p.Tasks))
	}
	t1 := p.Tasks[0]
	if !t1.NotePresent || t1.Note != "shipped to production, verified live" {
		t.Fatalf("task1 note present=%v value=%q", t1.NotePresent, t1.Note)
	}
	if t1.NoteLine == 0 {
		t.Fatal("task1 NoteLine should be set")
	}
	wantEvidence := []string{"cfabf5e", "https://chessraiders.com/board/"}
	if !t1.EvidencePresent || len(t1.Evidence) != len(wantEvidence) {
		t.Fatalf("task1 evidence present=%v value=%v", t1.EvidencePresent, t1.Evidence)
	}
	for i, want := range wantEvidence {
		if t1.Evidence[i] != want {
			t.Fatalf("task1 evidence[%d] = %q; want %q", i, t1.Evidence[i], want)
		}
	}
	if t1.EvidenceLine == 0 {
		t.Fatal("task1 EvidenceLine should be set")
	}

	t2 := p.Tasks[1]
	if t2.NotePresent || t2.Note != "" || t2.NoteLine != 0 {
		t.Fatalf("task2 note should be absent: present=%v value=%q line=%d", t2.NotePresent, t2.Note, t2.NoteLine)
	}
	if t2.EvidencePresent || len(t2.Evidence) != 0 || t2.EvidenceLine != 0 {
		t.Fatalf("task2 evidence should be absent: present=%v value=%v line=%d", t2.EvidencePresent, t2.Evidence, t2.EvidenceLine)
	}
}
