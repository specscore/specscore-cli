package lifecycle

import (
	"errors"
	"os"
	"testing"
)

// park requires a non-empty reason — rejected BEFORE any mutation.
func TestSetParked_EmptyReasonRejected(t *testing.T) {
	t.Parallel()
	body := "# Feature: X\n\n**Status:** Draft\n\n## Summary\n"
	path := writeHeaderFixture(t, body)

	if _, wrote, err := SetParked(path, "   "); !errors.Is(err, ErrParkReasonRequired) || wrote {
		t.Fatalf("expected ErrParkReasonRequired, got wrote=%v err=%v", wrote, err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != body {
		t.Errorf("file mutated on rejected park:\n%s", got)
	}
}

// A missing **Status:** line has nowhere to anchor the block.
func TestSetParked_NoStatusLineRejected(t *testing.T) {
	t.Parallel()
	body := "# Feature: X\n\n## Summary\n"
	path := writeHeaderFixture(t, body)

	if _, wrote, err := SetParked(path, "not v1"); !errors.Is(err, ErrStatusLineNotFound) || wrote {
		t.Fatalf("expected ErrStatusLineNotFound, got wrote=%v err=%v", wrote, err)
	}
}

// SetParked writes the three-line block immediately after **Status:**,
// leaves **Status:** itself untouched, and stamps today's UTC date.
//
// Not t.Parallel(): freezeParkedToday overrides the package-level
// parkedTodayUTC var, which every parallel subtest would otherwise race on
// (mirrors pkg/plan/reconcile_test.go's pinReconcileDate convention).
func TestSetParked_InsertsBlockAfterStatus(t *testing.T) {
	freezeParkedToday(t, "2026-08-04")

	body := "# Feature: X\n\n**Status:** Approved\n\n## Summary\n"
	path := writeHeaderFixture(t, body)

	orig, wrote, err := SetParked(path, "good idea, not v1")
	if err != nil || !wrote {
		t.Fatalf("SetParked: wrote=%v err=%v", wrote, err)
	}
	if string(orig) != body {
		t.Errorf("returned original bytes mismatch")
	}

	got, _ := os.ReadFile(path)
	want := "# Feature: X\n\n**Status:** Approved\n" +
		"**Parked:** true\n**Parked Reason:** good idea, not v1\n**Parked Date:** 2026-08-04\n" +
		"\n## Summary\n"
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// Status is never touched by park, whatever it is.
func TestSetParked_StatusPreserved(t *testing.T) {
	t.Parallel()
	for _, status := range []string{"Draft", "Approved", "Implementing", "Stable"} {
		body := "**Status:** " + status + "\n"
		path := writeHeaderFixture(t, body)
		if _, _, err := SetParked(path, "deferred"); err != nil {
			t.Fatalf("SetParked(%s): %v", status, err)
		}
		got, _ := os.ReadFile(path)
		if want := "**Status:** " + status + "\n"; len(got) < len(want) || string(got[:len(want)]) != want {
			t.Errorf("status line altered for %s: %q", status, got)
		}
	}
}

// Re-parking an already-parked artifact overwrites reason/date in place
// rather than duplicating the block.
func TestSetParked_ReParkOverwritesInPlace(t *testing.T) {
	body := "**Status:** Draft\n"
	path := writeHeaderFixture(t, body)

	freezeParkedToday(t, "2026-01-01")
	if _, _, err := SetParked(path, "first reason"); err != nil {
		t.Fatalf("first SetParked: %v", err)
	}

	freezeParkedToday(t, "2026-06-15")
	if _, _, err := SetParked(path, "updated reason"); err != nil {
		t.Fatalf("second SetParked: %v", err)
	}

	got, _ := os.ReadFile(path)
	want := "**Status:** Draft\n**Parked:** true\n**Parked Reason:** updated reason\n**Parked Date:** 2026-06-15\n"
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
}

// CRLF line endings are preserved.
func TestSetParked_CRLFPreserved(t *testing.T) {
	freezeParkedToday(t, "2026-08-04")
	body := "**Status:** Approved\r\n**Source Ideas:** —\r\n"
	path := writeHeaderFixture(t, body)
	if _, _, err := SetParked(path, "later"); err != nil {
		t.Fatalf("SetParked: %v", err)
	}
	got, _ := os.ReadFile(path)
	want := "**Status:** Approved\r\n**Parked:** true\r\n**Parked Reason:** later\r\n**Parked Date:** 2026-08-04\r\n**Source Ideas:** —\r\n"
	if string(got) != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

// ReadParked reports Parked: false and empty fields when absent.
func TestReadParked_AbsentIsFalse(t *testing.T) {
	t.Parallel()
	path := writeHeaderFixture(t, "**Status:** Draft\n")
	info, err := ReadParked(path)
	if err != nil {
		t.Fatalf("ReadParked: %v", err)
	}
	if info.Parked || info.Reason != "" || info.Date != "" {
		t.Errorf("expected zero-value ParkedInfo, got %+v", info)
	}
}

func TestParkedReaders_ReportReadErrors(t *testing.T) {
	missing := "nonexistent/parked.md"
	if _, err := ReadParked(missing); err == nil {
		t.Fatal("ReadParked missing file succeeded")
	}
	if _, err := IsParked(missing); err == nil {
		t.Fatal("IsParked missing file succeeded")
	}
}

func TestSetAndClearParked_ReportReadErrors(t *testing.T) {
	missing := "nonexistent/parked.md"
	if _, wrote, err := SetParked(missing, "deferred"); err == nil || wrote {
		t.Fatalf("SetParked missing = wrote:%v err:%v", wrote, err)
	}
	if _, wrote, err := ClearParked(missing); err == nil || wrote {
		t.Fatalf("ClearParked missing = wrote:%v err:%v", wrote, err)
	}
}

func TestSetParked_AddsTerminatorToUnterminatedStatus(t *testing.T) {
	freezeParkedToday(t, "2026-08-14")
	path := writeHeaderFixture(t, "**Status:** Draft")
	if _, wrote, err := SetParked(path, "deferred"); err != nil || !wrote {
		t.Fatalf("SetParked = wrote:%v err:%v", wrote, err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "**Status:** Draft\n**Parked:** true\n**Parked Reason:** deferred\n**Parked Date:** 2026-08-14\n" {
		t.Fatalf("unterminated status result = %q", got)
	}
}

// Unparking is not a no-op when never parked — it errors so a mistyped
// slug the caller believed was parked surfaces immediately.
func TestClearParked_NotParkedRejected(t *testing.T) {
	t.Parallel()
	body := "**Status:** Draft\n"
	path := writeHeaderFixture(t, body)
	if _, wrote, err := ClearParked(path); !errors.Is(err, ErrNotParked) || wrote {
		t.Fatalf("expected ErrNotParked, got wrote=%v err=%v", wrote, err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != body {
		t.Errorf("file mutated on rejected unpark:\n%s", got)
	}
}

// Round-trip: park then unpark restores the file byte-for-byte to its
// pre-park state (Status preserved throughout, nothing left behind).
func TestParkUnpark_RoundTrips(t *testing.T) {
	t.Parallel()
	body := "# Idea: X\n\n**Status:** Approved\n**Source Ideas:** —\n\n## Summary\n"
	path := writeHeaderFixture(t, body)

	if _, wrote, err := SetParked(path, "not v1"); err != nil || !wrote {
		t.Fatalf("SetParked: wrote=%v err=%v", wrote, err)
	}
	parked, err := IsParked(path)
	if err != nil || !parked {
		t.Fatalf("IsParked after park: parked=%v err=%v", parked, err)
	}

	orig, wrote, err := ClearParked(path)
	if err != nil || !wrote {
		t.Fatalf("ClearParked: wrote=%v err=%v", wrote, err)
	}
	if string(orig) == body {
		t.Errorf("ClearParked's returned `original` should be the PARKED bytes, not pre-park")
	}

	got, _ := os.ReadFile(path)
	if string(got) != body {
		t.Errorf("round-trip mismatch, got:\n%s\nwant:\n%s", got, body)
	}
	parked, err = IsParked(path)
	if err != nil || parked {
		t.Fatalf("IsParked after unpark: parked=%v err=%v", parked, err)
	}
}

// RestoreBody undoes a SetParked write using the returned original bytes —
// the same rollback contract every other structured header writer in this
// package offers.
func TestSetParked_RestoreBodyRollsBack(t *testing.T) {
	t.Parallel()
	body := "**Status:** Draft\n"
	path := writeHeaderFixture(t, body)
	orig, _, err := SetParked(path, "deferred")
	if err != nil {
		t.Fatalf("SetParked: %v", err)
	}
	if err := RestoreBody(path, orig); err != nil {
		t.Fatalf("RestoreBody: %v", err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != body {
		t.Errorf("RestoreBody did not roll back, got:\n%s", got)
	}
}

// freezeParkedToday overrides parkedTodayUTC for the duration of the test,
// restoring it via t.Cleanup. Callers MUST NOT mark the test t.Parallel() —
// parkedTodayUTC is a shared package-level var (mirrors
// pkg/plan/reconcile_test.go's pinReconcileDate convention).
func freezeParkedToday(t *testing.T, date string) {
	t.Helper()
	prev := parkedTodayUTC
	parkedTodayUTC = func() string { return date }
	t.Cleanup(func() { parkedTodayUTC = prev })
}
