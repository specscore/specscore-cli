package sidekick

// Seed legal-transition matrix, --to validation, and strict source check.
//
// Features implemented: cli/sidekick/change-status (legal-transition-matrix,
// target-status-flag) and the seed-kind half of
// cli/lifecycle-transitions (state-machine-strictness exit 4, not-idempotent).
//
// A sidekick-seed has its own tiny state machine, distinct from the Idea and
// Feature machines in pkg/lifecycle: every legal arc starts at `Queued` and
// ends at one of three terminal statuses.
//
//	Queued → Implemented
//	Queued → Rejected   (reason-required)
//	Queued → Stale
//
// Because the matrix is seed-specific and tiny, it lives here rather than as a
// new Kind in pkg/lifecycle. This file owns three primitives the change-status
// verb (Task 6) composes:
//
//   - ParseSeedTarget: case-insensitive `--to` value → canonical title-case
//     terminal Status, rejecting any value outside the terminal set with an
//     exit-2 (InvalidArgs) error BEFORE state-machine validation
//     (cli/sidekick/change-status#req:target-status-flag).
//   - CheckSeedSource: strict source-state check — a current frontmatter
//     status other than `Queued` yields an exit-4 (InvalidTransition) error
//     naming the current status and the legal source set `{Queued}`
//     (cli/sidekick/change-status#req:legal-transition-matrix,
//     lifecycle-transitions#req:state-machine-strictness).
//   - SeedReasonRequiredSet: the `Queued → Rejected` reason-required
//     designation, consuming the generic mechanism in pkg/lifecycle
//     (cli/sidekick/change-status#req:reason-required-rejected).
//
// Relocation and cobra wiring are owned by later tasks.

import (
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

// Seed lifecycle statuses. Values reuse the canonical title-case strings from
// pkg/lifecycle where they overlap; `Queued` is seed-specific.
const (
	SeedQueued      lifecycle.Status = "Queued"
	SeedImplemented lifecycle.Status = "Implemented"
	SeedRejected    lifecycle.Status = "Rejected"
	SeedStale       lifecycle.Status = "Stale"
)

// seedLegalSource is the single legal source state for every seed transition.
var seedLegalSource = SeedQueued

// seedTargets is the ordered set of legal `--to` targets — every To column in
// the seed matrix. Order is the rendering/title order used in messages.
var seedTargets = []lifecycle.Status{SeedImplemented, SeedRejected, SeedStale}

// ParseSeedTarget does case-insensitive parsing of a raw `--to` flag value
// against the seed terminal set {Implemented, Rejected, Stale}, returning
// the canonical title-case Status on success. Whitespace is trimmed and case
// is folded ("implemented", "IMPLEMENTED", "  Rejected " all match).
//
// An unrecognized value (including `Queued`, which is a source-only status and
// never a target) yields an exit-2 (InvalidArgs) error. Callers MUST invoke
// this BEFORE the state-machine source check so a bad `--to` fails at exit 2
// rather than slipping through to an exit-4 rejection
// (cli/sidekick/change-status#req:target-status-flag).
func ParseSeedTarget(raw string) (lifecycle.Status, error) {
	needle := strings.TrimSpace(raw)
	for _, t := range seedTargets {
		if strings.EqualFold(string(t), needle) {
			return t, nil
		}
	}
	return "", exitcode.InvalidArgsErrorf(
		"unrecognized --to value %q for sidekick seed; legal values: %s",
		raw, strings.Join(SeedTargetNames(), ", "))
}

// SeedTargetNames returns the canonical title-case names of the legal `--to`
// values, for cobra help text and stderr rendering.
func SeedTargetNames() []string {
	out := make([]string, len(seedTargets))
	for i, t := range seedTargets {
		out[i] = string(t)
	}
	return out
}

// CheckSeedSource enforces the strict source-state check: the legal source set
// for every seed target is {Queued}. A current frontmatter status (matched
// case-insensitively against `Queued`) other than the legal source yields an
// exit-4 (InvalidTransition) error whose message names the current status and
// the legal source set, per
// cli/sidekick/change-status#req:legal-transition-matrix and
// lifecycle-transitions#req:state-machine-strictness. A `Queued` source
// returns nil.
//
// current is the verbatim on-disk frontmatter `status:` value.
func CheckSeedSource(current string) error {
	if strings.EqualFold(strings.TrimSpace(current), string(seedLegalSource)) {
		return nil
	}
	return exitcode.InvalidStateErrorf(
		"invalid sidekick-seed transition: current status %q is not a legal source; legal source set: {%s}",
		current, string(seedLegalSource))
}

// SeedReasonRequiredSet returns the seed verb's reason-required designation:
// the `Queued → Rejected` arc requires a non-empty `--note`. Implemented and
// Stale keep `--note` optional. The change-status verb passes this set to
// lifecycle.GuardReason before any mutation
// (cli/sidekick/change-status#req:reason-required-rejected).
func SeedReasonRequiredSet() lifecycle.ReasonRequiredSet {
	return lifecycle.NewReasonRequiredSet(
		lifecycle.ReasonRequiredTransition{From: SeedQueued, To: SeedRejected},
	)
}
