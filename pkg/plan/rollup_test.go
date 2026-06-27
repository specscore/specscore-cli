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
