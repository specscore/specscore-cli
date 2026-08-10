package lesson

import (
	"errors"
	"fmt"
)

// MutationOutcome says what this invocation can prove about an artifact write.
// It deliberately describes the writer's own publication, not whether a path
// happens to exist: a competing writer may already own that path.
type MutationOutcome uint8

const (
	// MutationPrePublication proves this invocation did not publish an artifact.
	MutationPrePublication MutationOutcome = iota
	// MutationCompensated proves a published artifact was removed and that
	// removal was durably synced.
	MutationCompensated
	// MutationUncertain means publication, removal, or a required durability
	// fence may have happened but cannot be proved either way. Callers must
	// retain their prepared event for explicit reconciliation.
	MutationUncertain
)

// MutationError preserves a normal failure while carrying the only safe
// decision a transaction coordinator may make about its prepared event.
type MutationError struct {
	Outcome MutationOutcome
	Err     error
}

func (e *MutationError) Error() string { return e.Err.Error() }
func (e *MutationError) Unwrap() error { return e.Err }

func mutationFailure(outcome MutationOutcome, err error) error {
	if err == nil {
		return nil
	}
	return &MutationError{Outcome: outcome, Err: err}
}

// MutationOutcomeOf is conservative for errors from older writers: absent an
// explicit proof, a caller must assume publication may have survived.
func MutationOutcomeOf(err error) MutationOutcome {
	if err == nil {
		return MutationPrePublication
	}
	var mutation *MutationError
	if errors.As(err, &mutation) {
		return mutation.Outcome
	}
	return MutationUncertain
}

// CompensatePublication runs the caller's exact inverse mutation and its
// durability fence. It returns a typed outcome so a prepared event is aborted
// only after full compensation is proven.
func CompensatePublication(removeAndSync func() error, cause error) error {
	if err := removeAndSync(); err != nil {
		return mutationFailure(MutationUncertain, fmt.Errorf("%v; compensation is not durable: %w", cause, err))
	}
	return mutationFailure(MutationCompensated, cause)
}
