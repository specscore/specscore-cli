package plan

import "testing"

// TestTaskRollup_AllComplete covers cli/plan/info#ac:info-returns-task-rollup:
// a plan with 8 tasks all `complete` reports total:8, complete:8, and the other
// per-status buckets each 0.
func TestTaskRollup_AllComplete(t *testing.T) {
	dir := t.TempDir()
	body := `# Plan: AllComplete

**Source Feature:** foo

## Tasks

### Task 1: A
**Verifies:** foo#ac:a
**Status:** complete

### Task 2: B
**Verifies:** foo#ac:b
**Status:** complete

### Task 3: C
**Verifies:** foo#ac:c
**Status:** complete

### Task 4: D
**Verifies:** foo#ac:d
**Status:** complete

### Task 5: E
**Verifies:** foo#ac:e
**Status:** complete

### Task 6: F
**Verifies:** foo#ac:f
**Status:** complete

### Task 7: G
**Verifies:** foo#ac:g
**Status:** complete

### Task 8: H
**Verifies:** foo#ac:h
**Status:** complete
`
	p, err := Parse(writePlan(t, dir, "all-complete", body))
	if err != nil {
		t.Fatal(err)
	}
	got := p.TaskRollup()
	want := Rollup{Total: 8, Complete: 8, InProgress: 0, Planning: 0, Queued: 0, Blocked: 0}
	if got != want {
		t.Fatalf("rollup = %+v, want %+v", got, want)
	}
}

func TestTaskRollup_Empty(t *testing.T) {
	dir := t.TempDir()
	body := `# Plan: Empty

**Source Feature:** foo

## Tasks
`
	p, err := Parse(writePlan(t, dir, "empty", body))
	if err != nil {
		t.Fatal(err)
	}
	got := p.TaskRollup()
	want := Rollup{}
	if got != want {
		t.Fatalf("rollup = %+v, want %+v", got, want)
	}
}

func TestTaskRollup_Mixed(t *testing.T) {
	dir := t.TempDir()
	body := `# Plan: Mixed

**Source Feature:** foo

## Tasks

### Task 1: A
**Verifies:** foo#ac:a
**Status:** complete

### Task 2: B
**Verifies:** foo#ac:b
**Status:** in_progress

### Task 3: C
**Verifies:** foo#ac:c
**Status:** planning

### Task 4: D
**Verifies:** foo#ac:d
**Status:** queued

### Task 5: E
**Verifies:** foo#ac:e
**Status:** blocked
`
	p, err := Parse(writePlan(t, dir, "mixed", body))
	if err != nil {
		t.Fatal(err)
	}
	got := p.TaskRollup()
	want := Rollup{Total: 5, Complete: 1, InProgress: 1, Planning: 1, Queued: 1, Blocked: 1}
	if got != want {
		t.Fatalf("rollup = %+v, want %+v", got, want)
	}
}

// TestDeriveExecutionBand covers the canonical plan#req:status-rollup
// precedence (Failed > Executing > Blocked > Implemented) plus the
// INDETERMINATE cases (no tasks, any task still pre-execution: planning/queued).
func TestDeriveExecutionBand(t *testing.T) {
	mk := func(statuses ...TaskStatus) *Plan {
		p := &Plan{}
		for _, s := range statuses {
			p.Tasks = append(p.Tasks, Task{Status: s})
		}
		return p
	}
	cases := []struct {
		name     string
		plan     *Plan
		wantBand string
		wantOK   bool
	}{
		{"no-tasks", mk(), "", false},
		{"all-planning", mk(StatusPlanning, StatusPlanning), "", false},
		{"all-queued", mk(StatusQueued, StatusQueued), "", false},
		{"planning-and-queued", mk(StatusPlanning, StatusQueued), "", false},
		{"some-planning-rest-complete", mk(StatusComplete, StatusPlanning), "", false},
		{"all-complete", mk(StatusComplete, StatusComplete), "Implemented", true},
		{"in-progress", mk(StatusComplete, StatusInProgress, StatusPlanning), "Executing", true},
		{"blocked-only", mk(StatusComplete, StatusBlocked), "Blocked", true},
		{"blocked-with-planning", mk(StatusBlocked, StatusPlanning), "Blocked", true},
		{"failed-wins-over-inprogress", mk(StatusFailed, StatusInProgress), "Failed", true},
		{"aborted-counts-as-failed", mk(StatusAborted, StatusComplete), "Failed", true},
		{"in-progress-wins-over-blocked", mk(StatusInProgress, StatusBlocked), "Executing", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			band, ok := tc.plan.DeriveExecutionBand()
			if band != tc.wantBand || ok != tc.wantOK {
				t.Fatalf("DeriveExecutionBand() = (%q, %v), want (%q, %v)",
					band, ok, tc.wantBand, tc.wantOK)
			}
		})
	}
}

// TestImplementationEvidence_TwoTasks verifies
// implementation-commit-provenance#ac:plan-evidence-rolls-up: a plan with two
// provenance-carrying tasks derives the SET of both refs, ordered by task
// number, with no plan-level evidence hand-authored.
func TestImplementationEvidence_TwoTasks(t *testing.T) {
	dir := t.TempDir()
	body := `# Plan: Evidence

**Source Feature:** foo

## Tasks

### Task 2: B
**Status:** complete
**Implemented-by:** abc222

### Task 1: A
**Status:** complete
**Implemented-by:** abc111
`
	p, err := Parse(writePlan(t, dir, "evidence", body))
	if err != nil {
		t.Fatal(err)
	}
	got := p.ImplementationEvidence()
	want := []EvidenceEntry{
		{Task: 1, Ref: "abc111"},
		{Task: 2, Ref: "abc222"},
	}
	if len(got) != len(want) {
		t.Fatalf("evidence = %+v, want %+v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("evidence[%d] = %+v, want %+v", i, got[i], want[i])
		}
	}
}

// TestImplementationEvidence_Dedup confirms duplicate refs collapse to a SET,
// keeping the lowest task number's entry.
func TestImplementationEvidence_Dedup(t *testing.T) {
	dir := t.TempDir()
	body := `# Plan: Dedup

**Source Feature:** foo

## Tasks

### Task 1: A
**Id:** alpha
**Status:** complete
**Implemented-by:** same

### Task 2: B
**Status:** complete
**Implemented-by:** same
`
	p, err := Parse(writePlan(t, dir, "dedup", body))
	if err != nil {
		t.Fatal(err)
	}
	got := p.ImplementationEvidence()
	want := []EvidenceEntry{{Task: 1, Id: "alpha", Ref: "same"}}
	if len(got) != 1 || got[0] != want[0] {
		t.Fatalf("evidence = %+v, want %+v", got, want)
	}
}

// TestImplementationEvidence_None: tasks without provenance contribute nothing,
// yielding an empty (non-nil) set.
func TestImplementationEvidence_None(t *testing.T) {
	dir := t.TempDir()
	body := `# Plan: None

**Source Feature:** foo

## Tasks

### Task 1: A
**Status:** complete
`
	p, err := Parse(writePlan(t, dir, "none", body))
	if err != nil {
		t.Fatal(err)
	}
	got := p.ImplementationEvidence()
	if len(got) != 0 {
		t.Fatalf("evidence = %+v, want empty", got)
	}
}

// TestImplementationEvidence_SnapshotsStayDistinct verifies
// implementation-commit-provenance#ac:snapshots-stay-distinct: a plan carrying
// BOTH a `## Snapshots` table Git Hash and task implementation provenance keeps
// the two as distinct records — the snapshot hash never appears in the derived
// evidence set.
func TestImplementationEvidence_SnapshotsStayDistinct(t *testing.T) {
	dir := t.TempDir()
	const snapshotHash = "deadbeefspecstate"
	body := `# Plan: Distinct

**Source Feature:** foo

## Snapshots

| Date | Git Hash |
| ---- | -------- |
| 2026-06-25 | ` + snapshotHash + ` |

## Tasks

### Task 1: A
**Status:** complete
**Implemented-by:** codecommit111
`
	p, err := Parse(writePlan(t, dir, "distinct", body))
	if err != nil {
		t.Fatal(err)
	}
	got := p.ImplementationEvidence()
	if len(got) != 1 || got[0].Ref != "codecommit111" {
		t.Fatalf("evidence = %+v, want single codecommit111 entry", got)
	}
	for _, e := range got {
		if e.Ref == snapshotHash {
			t.Fatalf("snapshot Git Hash %q leaked into implementation evidence", snapshotHash)
		}
	}
}
