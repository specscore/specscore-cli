package plan

import "testing"

// TestTaskRollup_AllDone covers cli/plan/info#ac:info-returns-task-rollup:
// a plan with 8 tasks all `done` reports total:8, done:8, and the other
// per-status buckets each 0.
func TestTaskRollup_AllDone(t *testing.T) {
	dir := t.TempDir()
	body := `# Plan: AllDone

**Source Feature:** foo

## Tasks

### Task 1: A
**Verifies:** foo#ac:a
**Status:** done

### Task 2: B
**Verifies:** foo#ac:b
**Status:** done

### Task 3: C
**Verifies:** foo#ac:c
**Status:** done

### Task 4: D
**Verifies:** foo#ac:d
**Status:** done

### Task 5: E
**Verifies:** foo#ac:e
**Status:** done

### Task 6: F
**Verifies:** foo#ac:f
**Status:** done

### Task 7: G
**Verifies:** foo#ac:g
**Status:** done

### Task 8: H
**Verifies:** foo#ac:h
**Status:** done
`
	p, err := Parse(writePlan(t, dir, "all-done", body))
	if err != nil {
		t.Fatal(err)
	}
	got := p.TaskRollup()
	want := Rollup{Total: 8, Done: 8, InProgress: 0, Pending: 0, Blocked: 0}
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
**Status:** done

### Task 2: B
**Verifies:** foo#ac:b
**Status:** in-progress

### Task 3: C
**Verifies:** foo#ac:c
**Status:** pending

### Task 4: D
**Verifies:** foo#ac:d
**Status:** blocked
`
	p, err := Parse(writePlan(t, dir, "mixed", body))
	if err != nil {
		t.Fatal(err)
	}
	got := p.TaskRollup()
	want := Rollup{Total: 4, Done: 1, InProgress: 1, Pending: 1, Blocked: 1}
	if got != want {
		t.Fatalf("rollup = %+v, want %+v", got, want)
	}
}

// TestDeriveExecutionBand covers the canonical plan#req:status-rollup
// precedence (Failed > Executing > Blocked > Implemented) plus the
// INDETERMINATE cases (no tasks, any task still pending).
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
		{"all-pending", mk(StatusPending, StatusPending), "", false},
		{"some-pending-rest-done", mk(StatusDone, StatusPending), "", false},
		{"all-done", mk(StatusDone, StatusDone), "Implemented", true},
		{"in-progress", mk(StatusDone, StatusInProgress, StatusPending), "Executing", true},
		{"blocked-only", mk(StatusDone, StatusBlocked), "Blocked", true},
		{"blocked-with-pending", mk(StatusBlocked, StatusPending), "Blocked", true},
		{"failed-wins-over-inprogress", mk(StatusFailed, StatusInProgress), "Failed", true},
		{"aborted-counts-as-failed", mk(StatusAborted, StatusDone), "Failed", true},
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
