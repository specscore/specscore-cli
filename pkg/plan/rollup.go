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
