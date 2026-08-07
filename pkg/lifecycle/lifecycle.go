// Package lifecycle hosts a kind-parameterized state machine for SpecScore
// artifact Status transitions. It is the shared implementation layer that
// the per-kind `change-status` CLI verbs consume.
//
// The package is deliberately kind-agnostic: Idea, Feature, and any future
// doc kind plug their own legal-transition matrix into the package's lookup
// tables. Verb-specific logic (archive relocation, feature-id resolution,
// cobra wiring, exit-code mapping) lives in the calling CLI verbs, not here.
//
// Architectural contract implemented by this package (see
// spec/features/cli/lifecycle-transitions/README.md):
//
//   - REQ: state-machine-strictness — Transition rejects any (from, to) pair
//     not declared in the kind's matrix.
//   - REQ: not-idempotent — the matrix MUST NOT contain a self-loop
//     (from == to). The package's init step panics if a self-loop is
//     declared, so corruption is caught at startup rather than runtime.
//   - REQ: status-line-rewrite — Rewrite mutates only the **Status:** line,
//     preserving every other byte (including line endings and trailing
//     whitespace).
//   - REQ: rollback-on-lint-failure — Rollback restores the original
//     **Status:** line byte-for-byte.
package lifecycle

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// Kind names a doc kind that participates in the lifecycle state machine.
type Kind string

const (
	KindIdea     Kind = "idea"
	KindFeature  Kind = "feature"
	KindPlan     Kind = "plan"
	KindTask     Kind = "task"
	KindLesson   Kind = "lesson"
	KindDecision Kind = "decision"
)

// Status is a domain-scoped status value. The set of legal Status values is
// per-Kind and validated by the kind's transition table; callers SHOULD use
// ParseStatus to obtain a canonical Status from a raw flag string.
type Status string

// Idea statuses.
const (
	IdeaDraft        Status = "Draft"
	IdeaInReview     Status = "In Review"
	IdeaApproved     Status = "Approved"
	IdeaSpecifying   Status = "Specifying"
	IdeaSpecified    Status = "Specified"
	IdeaImplementing Status = "Implementing"
	IdeaImplemented  Status = "Implemented"
	IdeaRejected     Status = "Rejected"
	IdeaStale        Status = "Stale"
)

// Feature statuses.
const (
	FeatureDraft        Status = "Draft"
	FeatureInReview     Status = "In Review"
	FeatureApproved     Status = "Approved"
	FeatureImplementing Status = "Implementing"
	FeatureStable       Status = "Stable"
	FeatureAmending     Status = "Amending"
	FeatureRejected     Status = "Rejected"
	FeatureDeprecated   Status = "Deprecated"
)

// Plan statuses. The status models a plan's full lifecycle in three bands
// (prep / execution / disposition); see spec/features/plan/README.md. Only
// the prep band and the dispositions are human-authored — `plan change-status`
// owns those arcs. The execution band (Executing/Blocked/Implemented/Failed)
// is LINT-DERIVED from the task-status rollup (rule P-007) and MUST NOT be
// settable via change-status; those values appear in this matrix only as
// From-states for dispositions.
const (
	PlanDraft       Status = "Draft"
	PlanInReview    Status = "In Review"
	PlanApproved    Status = "Approved"
	PlanExecuting   Status = "Executing"
	PlanBlocked     Status = "Blocked"
	PlanImplemented Status = "Implemented"
	PlanFailed      Status = "Failed"
	PlanRejected    Status = "Rejected"
	PlanWithdrawn   Status = "Withdrawn"
	PlanSuperseded  Status = "Superseded"
	PlanDeprecated  Status = "Deprecated"
)

// Task statuses. A task moves through seven lifecycle states; the legal arcs
// are declared in the KindTask matrix below. The terminal states (Complete,
// Failed, Aborted) have no outgoing arcs. Values are lowercase to match the
// on-disk task-file **Status:** convention.
const (
	TaskPlanning   Status = "planning"
	TaskQueued     Status = "queued"
	TaskInProgress Status = "in_progress"
	TaskBlocked    Status = "blocked"
	TaskComplete   Status = "complete"
	TaskFailed     Status = "failed"
	TaskAborted    Status = "aborted"
)

// Lesson statuses. A Lesson climbs the enforcement ladder — Recorded (Tier 0,
// written down), Stated (Tier 1, loaded by an agent before acting), Enforced
// (Tier 2, a machine refuses) — and may reach one of two terminal
// dispositions at any rung: Withdrawn (turned out wrong / not applicable) or
// Superseded (replaced by a newer lesson). The vocabulary deliberately
// mirrors the Plan disposition set rather than inventing new terms.
const (
	LessonRecorded   Status = "Recorded"
	LessonStated     Status = "Stated"
	LessonEnforced   Status = "Enforced"
	LessonWithdrawn  Status = "Withdrawn"
	LessonSuperseded Status = "Superseded"
)

// Decision statuses. The vocabulary is closed by pkg/lint's D-status-values
// rule (decisionValidStatuses in pkg/lint/decision_rules.go) — these six
// values are the ONLY recognized **Status:** tokens for a Decision artifact;
// this const block mirrors that closed set rather than inventing one.
//
// The three pre-terminal statuses (Draft, In Review, Approved) form the prep
// band, mirroring Plan's shape. The three dispositions (Rejected, Superseded,
// Deprecated) are terminal: D-archived-location requires every Decision
// carrying one of them to live under spec/decisions/archived/, which
// `decision change-status` enforces by relocating the file as part of the
// same atomic transition (see pkg/decision.ChangeStatus).
const (
	DecisionDraft      Status = "Draft"
	DecisionInReview   Status = "In Review"
	DecisionApproved   Status = "Approved"
	DecisionRejected   Status = "Rejected"
	DecisionSuperseded Status = "Superseded"
	DecisionDeprecated Status = "Deprecated"
)

// ErrInvalidTransition is returned by Transition (and Validate) when the
// requested (from, to) pair is not present in the kind's matrix.
//
// The error carries the kind, source status, target status, and the legal
// target set from the current source, so the CLI layer can render a
// user-friendly message without re-querying the matrix.
var ErrInvalidTransition = errors.New("invalid lifecycle transition")

// InvalidTransitionError is a typed error carrying the context of a rejected
// transition. It wraps ErrInvalidTransition, so callers can use errors.Is to
// detect this category.
type InvalidTransitionError struct {
	Kind         Kind
	From         Status
	To           Status
	LegalTargets []Status
}

// Error implements the error interface. The message is human-readable and
// names both endpoints plus the legal target set from the current source.
func (e *InvalidTransitionError) Error() string {
	targets := make([]string, len(e.LegalTargets))
	for i, t := range e.LegalTargets {
		targets[i] = string(t)
	}
	if len(targets) == 0 {
		return fmt.Sprintf("invalid lifecycle transition for kind %q: %q has no legal targets (cannot transition to %q)",
			string(e.Kind), string(e.From), string(e.To))
	}
	return fmt.Sprintf("invalid lifecycle transition for kind %q: %q → %q not allowed; legal targets from %q: {%s}",
		string(e.Kind), string(e.From), string(e.To), string(e.From), strings.Join(targets, ", "))
}

// Unwrap exposes ErrInvalidTransition so errors.Is(err, ErrInvalidTransition)
// returns true.
func (e *InvalidTransitionError) Unwrap() error { return ErrInvalidTransition }

// transitionRow describes a single legal arc in the matrix.
type transitionRow struct {
	From Status
	To   Status
}

// transitionMatrix maps each Kind to its legal arcs.
//
// IMPORTANT: per REQ: not-idempotent, no row in any kind's table may have
// From == To. validateMatrix enforces the invariant at init time.
var transitionMatrix = map[Kind][]transitionRow{
	KindIdea: {
		{From: IdeaDraft, To: IdeaInReview},
		{From: IdeaDraft, To: IdeaApproved},
		// IdeaDraft -> IdeaRejected: completes the disposition vocabulary.
		// Draft is the state where an idea is most likely to be killed
		// deliberately, and it was the LAST pre-terminal status still
		// missing an active-turn-down exit (it already had the passive-decay
		// one, -> Stale). Feature's Draft has the same active-abandonment
		// door, spelled -> Deprecated because Feature's disposition set
		// includes it; Idea has no Deprecated, so -> Rejected is Idea's
		// equivalent. With this row every pre-terminal Idea status has both
		// a Rejected and a Stale exit. See
		// spec/features/cli/idea/change-status/README.md's legal-transition
		// matrix note.
		{From: IdeaDraft, To: IdeaRejected},
		{From: IdeaDraft, To: IdeaStale},
		{From: IdeaInReview, To: IdeaApproved},
		{From: IdeaInReview, To: IdeaRejected},
		{From: IdeaInReview, To: IdeaStale},
		// IdeaApproved -> IdeaRejected: an agreed-but-not-yet-specified Idea
		// can be actively turned down, not just left to decay to Stale. See
		// spec/features/cli/idea/change-status/README.md's legal-transition
		// matrix note.
		{From: IdeaApproved, To: IdeaRejected},
		{From: IdeaApproved, To: IdeaSpecifying},
		{From: IdeaApproved, To: IdeaStale},
		{From: IdeaSpecifying, To: IdeaSpecified},
		// IdeaSpecifying/IdeaSpecified/IdeaImplementing -> IdeaRejected, and
		// IdeaImplementing -> IdeaStale: complete the disposition vocabulary
		// (active Rejected vs. passive Stale) across the rest of the
		// pre-terminal Idea lifecycle, not just its early Approved stage.
		// Implementing previously had no disposition exit at all (its only
		// arc was -> Implemented), so a build that was cancelled or simply
		// petered out had no legal terminal status to record that. See
		// spec/features/cli/idea/change-status/README.md's legal-transition
		// matrix note.
		{From: IdeaSpecifying, To: IdeaRejected},
		{From: IdeaSpecifying, To: IdeaStale},
		{From: IdeaSpecified, To: IdeaImplementing},
		{From: IdeaSpecified, To: IdeaRejected},
		{From: IdeaSpecified, To: IdeaStale},
		{From: IdeaImplementing, To: IdeaImplemented},
		{From: IdeaImplementing, To: IdeaRejected},
		{From: IdeaImplementing, To: IdeaStale},
	},
	KindFeature: {
		{From: FeatureDraft, To: FeatureInReview},
		{From: FeatureDraft, To: FeatureApproved},
		{From: FeatureDraft, To: FeatureDeprecated},
		{From: FeatureInReview, To: FeatureApproved},
		{From: FeatureInReview, To: FeatureRejected},
		{From: FeatureApproved, To: FeatureImplementing},
		// An Approved-but-unbuilt Feature previously had exactly one legal
		// exit (-> Implementing), so a design that was approved and then
		// needed to change, be cancelled, or be retired before any code
		// existed had nowhere legal to go. These three rows close that gap;
		// see spec/features/cli/feature/change-status/README.md's
		// legal-transition matrix note and
		// spec/features/status-vocabulary/README.md#req-feature-amending.
		{From: FeatureApproved, To: FeatureAmending},
		{From: FeatureApproved, To: FeatureRejected},
		{From: FeatureApproved, To: FeatureDeprecated},
		{From: FeatureImplementing, To: FeatureStable},
		{From: FeatureImplementing, To: FeatureDeprecated},
		{From: FeatureStable, To: FeatureAmending},
		{From: FeatureAmending, To: FeatureStable},
		// Amending returns to the band it was entered from: an Amending
		// Feature that was entered from Approved (never built) returns to
		// Approved, not Stable, which would falsely claim an implementation.
		{From: FeatureAmending, To: FeatureApproved},
		{From: FeatureStable, To: FeatureDeprecated},
	},
	// KindPlan carries ONLY the human-authored arcs: the prep band and the
	// dispositions. The execution band (Executing/Blocked/Implemented/Failed)
	// is lint-derived (P-007) and is NOT enterable via change-status, so there
	// is deliberately no Approved→Executing arc. Execution-band states appear
	// here only as From-states for the dispositions, so a plan that lint has
	// advanced into the execution band can still be Withdrawn/Superseded/
	// Deprecated by a human.
	KindPlan: {
		{From: PlanDraft, To: PlanInReview},
		{From: PlanDraft, To: PlanApproved}, // direct approve — skips review
		{From: PlanInReview, To: PlanDraft}, // revisions requested
		{From: PlanInReview, To: PlanApproved},
		{From: PlanInReview, To: PlanRejected},

		{From: PlanApproved, To: PlanWithdrawn},
		{From: PlanExecuting, To: PlanWithdrawn},
		{From: PlanBlocked, To: PlanWithdrawn},
		{From: PlanImplemented, To: PlanWithdrawn},
		{From: PlanFailed, To: PlanWithdrawn},

		{From: PlanApproved, To: PlanSuperseded},
		{From: PlanExecuting, To: PlanSuperseded},
		{From: PlanBlocked, To: PlanSuperseded},
		{From: PlanImplemented, To: PlanSuperseded},
		{From: PlanFailed, To: PlanSuperseded},

		{From: PlanApproved, To: PlanDeprecated},
		{From: PlanExecuting, To: PlanDeprecated},
		{From: PlanBlocked, To: PlanDeprecated},
		{From: PlanImplemented, To: PlanDeprecated},
		{From: PlanFailed, To: PlanDeprecated},
	},
	// KindTask is the strict single-actor task lifecycle. Complete, Failed, and
	// Aborted are terminal (no outgoing arcs); the matrix carries no self-loops,
	// so re-running change-status on the current status is rejected
	// (REQ: not-idempotent → exit 4).
	KindTask: {
		{From: TaskPlanning, To: TaskQueued},
		{From: TaskPlanning, To: TaskAborted},

		{From: TaskQueued, To: TaskInProgress},
		{From: TaskQueued, To: TaskAborted},

		{From: TaskInProgress, To: TaskBlocked},
		{From: TaskInProgress, To: TaskComplete},
		{From: TaskInProgress, To: TaskFailed},
		{From: TaskInProgress, To: TaskAborted},

		{From: TaskBlocked, To: TaskInProgress},
		{From: TaskBlocked, To: TaskAborted},
	},
	// KindLesson: forward-only motion up the enforcement ladder
	// (Recorded -> Stated -> Enforced), including a direct Recorded ->
	// Enforced skip-ahead arc for a lesson that reaches a machine gate
	// without ever passing through an advisory Stated stage. Both
	// dispositions (Withdrawn, Superseded) are reachable from every rung —
	// mirroring how Plan's dispositions are reachable from every one of its
	// active statuses — so a lesson can be retired regardless of how far up
	// the ladder it climbed.
	KindLesson: {
		{From: LessonRecorded, To: LessonStated},
		{From: LessonRecorded, To: LessonEnforced},
		{From: LessonStated, To: LessonEnforced},

		{From: LessonRecorded, To: LessonWithdrawn},
		{From: LessonStated, To: LessonWithdrawn},
		{From: LessonEnforced, To: LessonWithdrawn},

		{From: LessonRecorded, To: LessonSuperseded},
		{From: LessonStated, To: LessonSuperseded},
		{From: LessonEnforced, To: LessonSuperseded},
	},
	// KindDecision carries the prep band (Draft/In Review/Approved) plus the
	// three dispositions (Rejected/Superseded/Deprecated). Rejected is reachable
	// only pre-Approval (Draft/In Review) — an actively-turned-down decision
	// that was never committed. Once Approved, D-immutability-once-accepted
	// freezes the content sections, so the only legal exits are the two
	// dispositions that retire an ALREADY-COMMITTED decision without
	// disowning it: Superseded (replaced by a named successor) and Deprecated
	// (retired with no specific successor). This mirrors Plan's prep-band /
	// disposition-band split; Decision has no Withdrawn analogue because that
	// word is not in the closed Decision status vocabulary.
	KindDecision: {
		{From: DecisionDraft, To: DecisionInReview},
		{From: DecisionDraft, To: DecisionApproved}, // direct approve — skips review
		{From: DecisionDraft, To: DecisionRejected},

		{From: DecisionInReview, To: DecisionDraft}, // revisions requested
		{From: DecisionInReview, To: DecisionApproved},
		{From: DecisionInReview, To: DecisionRejected},

		{From: DecisionApproved, To: DecisionSuperseded},
		{From: DecisionApproved, To: DecisionDeprecated},
	},
}

// kindStatuses enumerates every Status that is recognized for a kind. This
// is the union of all From and To values in the kind's matrix, used by the
// CLI layer to validate a --to flag value BEFORE running the state-machine
// check (so unrecognized status names exit 2 InvalidArgs, not 4
// InvalidTransition). It is computed once at init time.
var kindStatuses = map[Kind][]Status{}

func init() {
	for kind, rows := range transitionMatrix {
		kindStatuses[kind] = computeStatusUnion(rows)
	}
}

// validateMatrix enforces REQ: not-idempotent on a transition table. It
// returns an error if any row's From equals its To. The function is also
// invoked by tests against deliberately-bad rows to assert the invariant
// fires (without mutating the production matrix).
func validateMatrix(rows []transitionRow) error {
	for _, r := range rows {
		if r.From == r.To {
			return fmt.Errorf("self-loop forbidden: %q → %q", string(r.From), string(r.To))
		}
	}
	return nil
}

// computeStatusUnion returns every Status that appears in any row of the
// matrix (either as From or To), sorted alphabetically for deterministic
// output.
func computeStatusUnion(rows []transitionRow) []Status {
	seen := make(map[Status]struct{}, 2*len(rows))
	for _, r := range rows {
		seen[r.From] = struct{}{}
		seen[r.To] = struct{}{}
	}
	out := make([]Status, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return string(out[i]) < string(out[j]) })
	return out
}

// Transition validates that (from, to) is a legal transition in kind's
// matrix. It returns nil on success and a wrapped ErrInvalidTransition on
// failure. The wrapped error carries the legal target set from the current
// source so callers can render a useful message.
//
// Transition does NOT touch the filesystem; it is pure matrix lookup.
func Transition(kind Kind, from Status, to Status) error {
	rows, ok := transitionMatrix[kind]
	if !ok {
		return &InvalidTransitionError{Kind: kind, From: from, To: to}
	}
	for _, r := range rows {
		if r.From == from && r.To == to {
			return nil
		}
	}
	return &InvalidTransitionError{
		Kind:         kind,
		From:         from,
		To:           to,
		LegalTargets: LegalTargets(kind, from),
	}
}

// LegalTargets returns the legal target statuses reachable from (kind, from),
// sorted alphabetically. The empty slice is returned (never nil for an
// unknown kind, but never nil for a known kind either) when from is not a
// legal source state in the kind's matrix.
func LegalTargets(kind Kind, from Status) []Status {
	rows, ok := transitionMatrix[kind]
	if !ok {
		return []Status{}
	}
	var out []Status
	for _, r := range rows {
		if r.From == from {
			out = append(out, r.To)
		}
	}
	sort.Slice(out, func(i, j int) bool { return string(out[i]) < string(out[j]) })
	if out == nil {
		return []Status{}
	}
	return out
}

// LegalSources is the inverse of LegalTargets: which from-states can
// transition INTO to. Used by error-message construction when target is
// valid as a status name but invalid for the current source state.
func LegalSources(kind Kind, to Status) []Status {
	rows, ok := transitionMatrix[kind]
	if !ok {
		return []Status{}
	}
	var out []Status
	for _, r := range rows {
		if r.To == to {
			out = append(out, r.From)
		}
	}
	sort.Slice(out, func(i, j int) bool { return string(out[i]) < string(out[j]) })
	if out == nil {
		return []Status{}
	}
	return out
}

// LegalStatuses returns every recognized status for a kind (the union of
// every From and To in its matrix), sorted alphabetically. Used by the CLI
// layer to validate a --to flag value before invoking the state-machine
// check.
func LegalStatuses(kind Kind) []Status {
	out, ok := kindStatuses[kind]
	if !ok {
		return []Status{}
	}
	// Return a copy so callers cannot mutate the package-level slice.
	cp := make([]Status, len(out))
	copy(cp, out)
	return cp
}

// ParseStatus does case-insensitive parsing of a raw flag-string against the
// kind's recognized statuses, returning the canonical title-cased Status on
// success.
//
// Whitespace is trimmed; case is folded (so "draft", "Draft", "DRAFT", and
// "  Draft  " all match). Multi-word statuses ("Under Review") match
// case-insensitively but the internal-whitespace shape MUST match (i.e.,
// "underreview" without a space does NOT match "Under Review"). This is the
// least-surprising behavior for a CLI flag.
func ParseStatus(kind Kind, raw string) (Status, bool) {
	needle := strings.TrimSpace(raw)
	if needle == "" {
		return "", false
	}
	for _, s := range LegalStatuses(kind) {
		if strings.EqualFold(string(s), needle) {
			return s, true
		}
	}
	return "", false
}
