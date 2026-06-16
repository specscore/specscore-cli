package lifecycle

import (
	"errors"
	"strings"
	"testing"
)

// designated is a small reason-required set used across these tests:
// Queued → Rejected is reason-required; nothing else is.
func designated() ReasonRequiredSet {
	return NewReasonRequiredSet(
		ReasonRequiredTransition{From: "Queued", To: "Rejected"},
	)
}

func TestGuardReason_DesignatedMissingNote_Errors(t *testing.T) {
	err := GuardReason(designated(), "Queued", "Rejected", "")
	if err == nil {
		t.Fatal("expected a reason-required error for a designated transition with no note, got nil")
	}
	var rr *ReasonRequiredError
	if !errors.As(err, &rr) {
		t.Fatalf("expected *ReasonRequiredError, got %T", err)
	}
	if rr.From != "Queued" || rr.To != "Rejected" {
		t.Fatalf("error should carry the (from,to) transition; got %q → %q", rr.From, rr.To)
	}
	msg := err.Error()
	// The message MUST name the transition and state a reason is required.
	if !strings.Contains(msg, "Rejected") {
		t.Errorf("message should name the target transition; got %q", msg)
	}
	if !strings.Contains(strings.ToLower(msg), "reason") {
		t.Errorf("message should state that a reason is required; got %q", msg)
	}
}

func TestGuardReason_DesignatedWhitespaceNote_Errors(t *testing.T) {
	err := GuardReason(designated(), "Queued", "Rejected", "   \t\n")
	if err == nil {
		t.Fatal("expected a reason-required error for a designated transition with whitespace-only note")
	}
	var rr *ReasonRequiredError
	if !errors.As(err, &rr) {
		t.Fatalf("expected *ReasonRequiredError, got %T", err)
	}
}

func TestGuardReason_DesignatedWithNote_OK(t *testing.T) {
	if err := GuardReason(designated(), "Queued", "Rejected", "no longer relevant"); err != nil {
		t.Fatalf("a designated transition WITH a note must proceed; got %v", err)
	}
}

func TestGuardReason_NonDesignatedMissingNote_OK(t *testing.T) {
	// Queued → Promoted is NOT designated, so a missing note must proceed.
	if err := GuardReason(designated(), "Queued", "Promoted", ""); err != nil {
		t.Fatalf("a non-designated transition with no note must proceed; got %v", err)
	}
}

func TestGuardReason_EmptySet_AlwaysOK(t *testing.T) {
	var empty ReasonRequiredSet
	if err := GuardReason(empty, "Queued", "Rejected", ""); err != nil {
		t.Fatalf("a verb designating no reason-required transitions is unaffected; got %v", err)
	}
}

func TestRequiresReason(t *testing.T) {
	set := designated()
	if !set.RequiresReason("Queued", "Rejected") {
		t.Error("Queued → Rejected should be reason-required")
	}
	if set.RequiresReason("Queued", "Promoted") {
		t.Error("Queued → Promoted should not be reason-required")
	}
}
