package plan

import "sort"

// EvidenceEntry is one task's implementation-commit reference, surfaced as a
// distinct record from any plan-level Snapshots Git Hash.
type EvidenceEntry struct {
	Task int    `yaml:"task" json:"task"`                 // task number carrying the ref
	Id   string `yaml:"id,omitempty" json:"id,omitempty"` // task **Id:** when present
	Ref  string `yaml:"ref" json:"ref"`                   // the `**Implemented-by:**` value
}

// ImplementationEvidence derives the deduplicated SET of implementation-commit
// refs carried by p's tasks (via each task's `**Implemented-by:**` field), in a
// stable order by task number. It is query-only: it reads task provenance and
// never reads or writes any plan-body/frontmatter "evidence" field, nor the
// `## Snapshots` Git Hash (which records spec-document state, a distinct axis).
// Tasks without provenance — or with an empty ref — contribute nothing, so a
// plan with no provenance yields an empty slice.
func (p *Plan) ImplementationEvidence() []EvidenceEntry {
	tasks := make([]Task, len(p.Tasks))
	copy(tasks, p.Tasks)
	sort.SliceStable(tasks, func(i, j int) bool { return tasks[i].Number < tasks[j].Number })

	out := []EvidenceEntry{}
	seen := map[string]bool{}
	for _, t := range tasks {
		ref := t.ImplementationCommit
		if ref == "" || seen[ref] {
			continue
		}
		seen[ref] = true
		out = append(out, EvidenceEntry{Task: t.Number, Id: t.Id, Ref: ref})
	}
	return out
}

// Rollup counts a plan's tasks by their parsed task **Status:** value.
type Rollup struct {
	Total      int `yaml:"total" json:"total"`
	Complete   int `yaml:"complete" json:"complete"`
	InProgress int `yaml:"in_progress" json:"in_progress"`
	Planning   int `yaml:"planning" json:"planning"`
	Queued     int `yaml:"queued" json:"queued"`
	Blocked    int `yaml:"blocked" json:"blocked"`
}

// TaskRollup tallies p.Tasks by status. Total is len(p.Tasks); each
// per-status count is 0 when no task holds that status.
func (p *Plan) TaskRollup() Rollup {
	r := Rollup{Total: len(p.Tasks)}
	for _, t := range p.Tasks {
		switch t.Status {
		case StatusComplete:
			r.Complete++
		case StatusInProgress:
			r.InProgress++
		case StatusPlanning:
			r.Planning++
		case StatusQueued:
			r.Queued++
		case StatusBlocked:
			r.Blocked++
		}
	}
	return r
}

// DeriveExecutionBand computes the plan's execution-band status from the rollup
// of its task statuses, per the canonical plan#req:status-rollup precedence
// (Failed > Executing > Blocked > Implemented). It returns ("", false) when the
// rollup is INDETERMINATE — there are no tasks, or at least one task is still
// pre-execution (planning/queued) so the set cannot resolve to a single band.
// The returned string, when
// ok, is the canonical Title-Case Plan status ("Failed"/"Executing"/"Blocked"/
// "Implemented"). This reads task status only; it never writes it.
func (p *Plan) DeriveExecutionBand() (string, bool) {
	if len(p.Tasks) == 0 {
		return "", false
	}
	var failed, inProgress, blocked, complete int
	for _, t := range p.Tasks {
		switch t.Status {
		case StatusFailed, StatusAborted:
			failed++
		case StatusInProgress:
			inProgress++
		case StatusBlocked:
			blocked++
		case StatusComplete:
			complete++
		}
	}
	switch {
	case failed > 0:
		return "Failed", true
	case inProgress > 0:
		return "Executing", true
	case blocked > 0:
		// Blocked wins over a residual pre-execution count: with nothing in
		// progress or failed, a blocked task is the most-advanced signal in the
		// set.
		return "Blocked", true
	case complete == len(p.Tasks):
		return "Implemented", true
	default:
		// No tasks failed/in-progress/blocked and not all complete — i.e. at
		// least one task is still planning or queued. The set cannot resolve to
		// a single band.
		return "", false
	}
}
