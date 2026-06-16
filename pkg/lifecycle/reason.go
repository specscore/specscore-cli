package lifecycle

import (
	"fmt"
	"strings"
)

// reason.go implements the generic reason-required-transition mechanism per
// spec/features/cli/lifecycle-transitions/README.md REQ: reason-required-transitions.
//
// A verb MAY designate a subset of its legal transitions as reason-required.
// For a designated (from, to) transition, a missing or empty/whitespace-only
// `--note` MUST cause the verb to fail BEFORE any mutation, with a message
// naming the transition. Non-designated transitions keep `--note` optional;
// a verb that designates none is unaffected.
//
// The mechanism is kind-agnostic and carries no verb-specific knowledge — a
// verb supplies the set of (from, to) pairs it considers reason-required.
// Like the rest of pkg/lifecycle, this layer does NOT import pkg/exitcode:
// it returns a typed *ReasonRequiredError that the consuming verb maps to
// exit code 2 (InvalidArgs), mirroring how the verb maps InvalidTransitionError.

// ReasonRequiredTransition names a single (from, to) arc a verb designates
// as reason-required.
type ReasonRequiredTransition struct {
	From Status
	To   Status
}

// ReasonRequiredSet is the set of transitions a verb designates as
// reason-required. The zero value is a valid empty set (designates nothing).
type ReasonRequiredSet struct {
	m map[ReasonRequiredTransition]struct{}
}

// NewReasonRequiredSet builds a ReasonRequiredSet from the given transitions.
// Passing no transitions yields an empty set, equivalent to the zero value.
func NewReasonRequiredSet(transitions ...ReasonRequiredTransition) ReasonRequiredSet {
	if len(transitions) == 0 {
		return ReasonRequiredSet{}
	}
	m := make(map[ReasonRequiredTransition]struct{}, len(transitions))
	for _, t := range transitions {
		m[t] = struct{}{}
	}
	return ReasonRequiredSet{m: m}
}

// RequiresReason reports whether the (from, to) transition is designated
// reason-required in this set.
func (s ReasonRequiredSet) RequiresReason(from, to Status) bool {
	if s.m == nil {
		return false
	}
	_, ok := s.m[ReasonRequiredTransition{From: from, To: to}]
	return ok
}

// ReasonRequiredError is returned by GuardReason when a designated
// reason-required transition is attempted without a non-empty note. It
// carries the (from, to) transition so the consuming verb can render a
// message and map it to exit code 2 (InvalidArgs).
type ReasonRequiredError struct {
	From Status
	To   Status
}

// Error implements the error interface. The message names the transition and
// states that a reason is required, satisfying the contract's stderr message
// requirement.
func (e *ReasonRequiredError) Error() string {
	return fmt.Sprintf(
		"the %s → %s transition requires a reason: pass --note with the reasoning for this transition",
		string(e.From), string(e.To))
}

// GuardReason is the pre-mutation guard for reason-required transitions. If
// (from, to) is designated reason-required in set and note is empty or
// whitespace-only, it returns a *ReasonRequiredError naming the transition.
// Otherwise it returns nil.
//
// Callers MUST invoke GuardReason BEFORE any artifact mutation, so a
// designated transition with a missing reason fails without touching the
// file (REQ: reason-required-transitions). Non-designated transitions and an
// empty set always return nil here, leaving `--note` optional.
func GuardReason(set ReasonRequiredSet, from, to Status, note string) error {
	if !set.RequiresReason(from, to) {
		return nil
	}
	if strings.TrimSpace(note) == "" {
		return &ReasonRequiredError{From: from, To: to}
	}
	return nil
}
