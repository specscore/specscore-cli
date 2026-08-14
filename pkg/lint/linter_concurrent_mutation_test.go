package lint

import (
	"errors"
	"os"
	"testing"
)

type transientRemovalChecker struct {
	calls    int
	failures []error
}

func (c *transientRemovalChecker) check(string) ([]Violation, error) {
	c.calls++
	if c.calls <= len(c.failures) {
		return []Violation{{Rule: "discard-partial"}}, c.failures[c.calls-1]
	}
	return []Violation{{Rule: "complete"}}, nil
}

func (*transientRemovalChecker) name() string     { return "transient-removal" }
func (*transientRemovalChecker) severity() string { return "error" }

func TestCheckWithTransientRemovalRetryRestartsChecker(t *testing.T) {
	disappeared := &os.PathError{Op: "lstat", Path: ".lesson-index-temporary", Err: os.ErrNotExist}
	checker := &transientRemovalChecker{failures: []error{disappeared}}
	violations, err := checkWithTransientRemovalRetry(checker, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if checker.calls != 2 || len(violations) != 1 || violations[0].Rule != "complete" {
		t.Fatalf("calls=%d violations=%v, want one clean restart result", checker.calls, violations)
	}
}

func TestCheckWithTransientRemovalRetryFailsClosedAfterBound(t *testing.T) {
	disappeared := &os.PathError{Op: "lstat", Path: "artifact.md", Err: os.ErrNotExist}
	checker := &transientRemovalChecker{failures: []error{disappeared, disappeared, disappeared}}
	violations, err := checkWithTransientRemovalRetry(checker, t.TempDir())
	if !errors.Is(err, os.ErrNotExist) || checker.calls != checkerConcurrentMutationAttempts {
		t.Fatalf("calls=%d error=%v, want bounded not-exist failure", checker.calls, err)
	}
	if len(violations) != 1 || violations[0].Rule != "discard-partial" {
		t.Fatalf("terminal violations=%v, want final-attempt result", violations)
	}
}

func TestCheckWithTransientRemovalRetryDoesNotRetryOtherErrors(t *testing.T) {
	boom := errors.New("permission denied")
	checker := &transientRemovalChecker{failures: []error{boom}}
	_, err := checkWithTransientRemovalRetry(checker, t.TempDir())
	if !errors.Is(err, boom) || checker.calls != 1 {
		t.Fatalf("calls=%d error=%v, want immediate non-retryable failure", checker.calls, err)
	}
}
