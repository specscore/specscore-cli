package sidekick

import (
	"errors"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

// exitCodeOf extracts the numeric exit code from an error that carries one.
func exitCodeOf(t *testing.T, err error) int {
	t.Helper()
	var ec interface{ ExitCode() int }
	if !errors.As(err, &ec) {
		t.Fatalf("error %v does not carry an exit code", err)
	}
	return ec.ExitCode()
}

func TestParseSeedTarget_unrecognized_exit2(t *testing.T) {
	// AC: unrecognized-target-rejected — --to=stable is not a seed terminal.
	_, err := ParseSeedTarget("stable")
	if err == nil {
		t.Fatal("ParseSeedTarget(\"stable\") = nil err; want exit-2 error")
	}
	if got := exitCodeOf(t, err); got != exitcode.InvalidArgs {
		t.Errorf("exit code = %d; want %d (InvalidArgs)", got, exitcode.InvalidArgs)
	}
}

func TestParseSeedTarget_caseInsensitive_canonical(t *testing.T) {
	cases := map[string]lifecycle.Status{
		"implemented": SeedImplemented,
		"IMPLEMENTED": SeedImplemented,
		"Implemented": SeedImplemented,
		"  rejected ": SeedRejected,
		"ArChIvEd":    SeedArchived,
	}
	for in, want := range cases {
		got, err := ParseSeedTarget(in)
		if err != nil {
			t.Errorf("ParseSeedTarget(%q) err = %v; want nil", in, err)
			continue
		}
		if got != want {
			t.Errorf("ParseSeedTarget(%q) = %q; want %q (canonical title-case)", in, got, want)
		}
	}
}

func TestParseSeedTarget_queuedNotATarget(t *testing.T) {
	// Queued is the only legal source, never a target.
	_, err := ParseSeedTarget("queued")
	if err == nil {
		t.Fatal("ParseSeedTarget(\"queued\") = nil err; want exit-2 error")
	}
	if got := exitCodeOf(t, err); got != exitcode.InvalidArgs {
		t.Errorf("exit code = %d; want %d", got, exitcode.InvalidArgs)
	}
}

func TestCheckSeedSource_nonQueued_exit4(t *testing.T) {
	// AC: non-queued-source-rejected — current status not Queued exits 4,
	// naming the current status and the legal source set {Queued}.
	err := CheckSeedSource("Implemented")
	if err == nil {
		t.Fatal("CheckSeedSource(\"Implemented\") = nil err; want exit-4 error")
	}
	if got := exitCodeOf(t, err); got != exitcode.InvalidState {
		t.Errorf("exit code = %d; want %d (InvalidState)", got, exitcode.InvalidState)
	}
	msg := err.Error()
	if !strings.Contains(msg, "Implemented") {
		t.Errorf("message %q does not name the current status", msg)
	}
	if !strings.Contains(msg, "Queued") {
		t.Errorf("message %q does not name the legal source set {Queued}", msg)
	}
}

func TestCheckSeedSource_queued_ok(t *testing.T) {
	// Case-insensitive on the on-disk value: queued is the legal source.
	if err := CheckSeedSource("queued"); err != nil {
		t.Errorf("CheckSeedSource(\"queued\") = %v; want nil", err)
	}
	if err := CheckSeedSource("Queued"); err != nil {
		t.Errorf("CheckSeedSource(\"Queued\") = %v; want nil", err)
	}
}

func TestSeedReasonRequiredSet_rejectedRequiresReason(t *testing.T) {
	set := SeedReasonRequiredSet()
	if !set.RequiresReason(SeedQueued, SeedRejected) {
		t.Error("Queued → Rejected MUST be reason-required")
	}
	if set.RequiresReason(SeedQueued, SeedImplemented) {
		t.Error("Queued → Implemented MUST NOT be reason-required")
	}
	if set.RequiresReason(SeedQueued, SeedArchived) {
		t.Error("Queued → Archived MUST NOT be reason-required")
	}
}
