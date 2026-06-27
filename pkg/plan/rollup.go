package plan

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
