package plan

// Rollup counts a plan's tasks by their parsed task **Status:** value.
type Rollup struct {
	Total      int `yaml:"total" json:"total"`
	Done       int `yaml:"done" json:"done"`
	InProgress int `yaml:"in-progress" json:"in-progress"`
	Pending    int `yaml:"pending" json:"pending"`
	Blocked    int `yaml:"blocked" json:"blocked"`
}

// TaskRollup tallies p.Tasks by status. Total is len(p.Tasks); each
// per-status count is 0 when no task holds that status.
func (p *Plan) TaskRollup() Rollup {
	r := Rollup{Total: len(p.Tasks)}
	for _, t := range p.Tasks {
		switch t.Status {
		case StatusDone:
			r.Done++
		case StatusInProgress:
			r.InProgress++
		case StatusPending:
			r.Pending++
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
// pending so the set cannot resolve to a single band. The returned string, when
// ok, is the canonical Title-Case Plan status ("Failed"/"Executing"/"Blocked"/
// "Implemented"). This reads task status only; it never writes it.
func (p *Plan) DeriveExecutionBand() (string, bool) {
	if len(p.Tasks) == 0 {
		return "", false
	}
	var failed, inProgress, blocked, done int
	for _, t := range p.Tasks {
		switch t.Status {
		case StatusFailed, StatusAborted:
			failed++
		case StatusInProgress:
			inProgress++
		case StatusBlocked:
			blocked++
		case StatusDone:
			done++
		}
	}
	switch {
	case failed > 0:
		return "Failed", true
	case inProgress > 0:
		return "Executing", true
	case blocked > 0:
		// Blocked wins over a residual pending count: with nothing in progress
		// or failed, a blocked task is the most-advanced signal in the set.
		return "Blocked", true
	case done == len(p.Tasks):
		return "Implemented", true
	default:
		// No tasks failed/in-progress/blocked and not all done — i.e. at least
		// one task is still pending. The set cannot resolve to a single band.
		return "", false
	}
}
