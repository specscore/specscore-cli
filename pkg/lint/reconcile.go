package lint

import (
	"fmt"
	"strings"
)

// Reconciliation is one itemized correction a `--fix` pass performed by
// rewriting a derived index row from its authoritative artifact file.
//
// Principle (see https://github.com/specscore/specscore/blob/main/spec/features/index/README.md#req-file-authoritative-over-index,
// and the general SpecScore rule it documents): an artifact's own file is
// authoritative; any index table that summarizes many artifacts is a derived
// projection. "File wins" is the tool's deterministic default for
// regenerating derived data — it is safe ONLY because the index is derived,
// and it is NOT a claim that the file's value is the one a human would
// actually want. A file can be stale (someone forgot to run `change-status`)
// exactly as often as an index row can be. A --fix pass that silently
// rewrites the index from the file therefore destroys the one signal that
// would let a human or agent notice the file itself was the stale side.
//
// Every fixer that regenerates an index row from an artifact file MUST
// report what it changed via this type instead of rewriting quietly. Nothing
// here changes WHICH side wins — the file still wins mechanically — it only
// makes the correction loud.
type Reconciliation struct {
	// Rule is the lint rule name that performed the reconciliation, e.g.
	// "task-index-row-sync".
	Rule string
	// Artifact identifies the artifact whose file was taken as authoritative,
	// e.g. a task/idea/feature/decision slug.
	Artifact string
	// Changes lists each derived column the fix rewrote, oldest-index-value
	// first. At least one entry is required; an empty Changes is a caller bug.
	Changes []FieldChange
}

// FieldChange names one derived column a reconciliation rewrote, along with
// the value the index carried before the fix and the value taken from the
// authoritative artifact file.
type FieldChange struct {
	Field      string
	IndexValue string
	FileValue  string
}

// String renders one human-readable reconciliation line, e.g.:
//
//	task-index-row-sync: fleet-core-modules-bump: status: index had "planning", file has "queued"
func (r Reconciliation) String() string {
	parts := make([]string, 0, len(r.Changes))
	for _, c := range r.Changes {
		parts = append(parts, fmt.Sprintf("%s: index had %q, file has %q", c.Field, c.IndexValue, c.FileValue))
	}
	return fmt.Sprintf("%s: %s: %s", r.Rule, r.Artifact, strings.Join(parts, "; "))
}

// reconciler is an optional interface a fixer implements when its fix
// rewrites index rows from artifact files. take returns every reconciliation
// performed by the most recent fix() call and clears the fixer's pending
// list, so a checker instance reused across multiple lint runs never
// re-reports a stale batch.
type reconciler interface {
	fixer
	takeReconciliations() []Reconciliation
}
