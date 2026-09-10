package event

import (
	"encoding/json"
	"testing"
)

// TestLedgerUUIDs_EmptyAndNilData asserts an absent/empty ledger — the
// ordinary "nothing here yet" case `event check` must tolerate at --base —
// parses to an empty set rather than an error.
func TestLedgerUUIDs_EmptyAndNilData(t *testing.T) {
	for _, data := range [][]byte{nil, {}} {
		got, err := LedgerUUIDs(data, "test-ledger")
		if err != nil {
			t.Fatalf("LedgerUUIDs(%v) error: %v", data, err)
		}
		if len(got) != 0 {
			t.Fatalf("LedgerUUIDs(%v) = %v, want empty set", data, got)
		}
	}
}

// TestLedgerUUIDs_ParsesEveryRecord asserts the returned set has exactly
// one entry per distinct event UUID in the ledger.
func TestLedgerUUIDs_ParsesEveryRecord(t *testing.T) {
	data := marshalCheckLedger(t,
		mergeTestEvent("00000000-0000-4000-8000-000000000001", `{"a":1}`),
		mergeTestEvent("00000000-0000-4000-8000-000000000002", `{"b":2}`),
	)
	got, err := LedgerUUIDs(data, "test-ledger")
	if err != nil {
		t.Fatalf("LedgerUUIDs error: %v", err)
	}
	want := map[string]struct{}{
		"00000000-0000-4000-8000-000000000001": {},
		"00000000-0000-4000-8000-000000000002": {},
	}
	if len(got) != len(want) {
		t.Fatalf("LedgerUUIDs = %v, want %v", got, want)
	}
	for id := range want {
		if _, ok := got[id]; !ok {
			t.Errorf("LedgerUUIDs missing expected uuid %s", id)
		}
	}
}

// TestLedgerUUIDs_MalformedLineIsAnError asserts a corrupt JSONL line
// (unparseable) is surfaced as an error rather than silently skipped — a
// check command that ignored parse errors could report false monotonicity.
func TestLedgerUUIDs_MalformedLineIsAnError(t *testing.T) {
	_, err := LedgerUUIDs([]byte("not json at all\n"), "test-ledger")
	if err == nil {
		t.Fatal("LedgerUUIDs on malformed JSONL: expected an error, got nil")
	}
}

// TestMissingUUIDs_EmptyWhenWorkingHasEverything asserts a working set that
// is a superset (or exact match) of base yields no missing UUIDs.
func TestMissingUUIDs_EmptyWhenWorkingHasEverything(t *testing.T) {
	base := map[string]struct{}{"a": {}, "b": {}}
	working := map[string]struct{}{"a": {}, "b": {}, "c": {}}
	if got := MissingUUIDs(base, working); len(got) != 0 {
		t.Fatalf("MissingUUIDs = %v, want empty", got)
	}
}

// TestMissingUUIDs_ReportsEveryLostUUIDSorted asserts every base-only UUID
// is reported, in ascending sorted order (so output is deterministic and
// diff-stable across runs).
func TestMissingUUIDs_ReportsEveryLostUUIDSorted(t *testing.T) {
	base := map[string]struct{}{"z-lost": {}, "a-lost": {}, "kept": {}}
	working := map[string]struct{}{"kept": {}}
	got := MissingUUIDs(base, working)
	want := []string{"a-lost", "z-lost"}
	if len(got) != len(want) {
		t.Fatalf("MissingUUIDs = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("MissingUUIDs = %v, want %v", got, want)
		}
	}
}

// TestMissingUUIDs_EmptyBaseNeverReportsAnything asserts a base with zero
// UUIDs (e.g. the ledger did not exist yet at --base) never reports a
// missing event, regardless of the working set.
func TestMissingUUIDs_EmptyBaseNeverReportsAnything(t *testing.T) {
	base := map[string]struct{}{}
	working := map[string]struct{}{}
	if got := MissingUUIDs(base, working); len(got) != 0 {
		t.Fatalf("MissingUUIDs = %v, want empty", got)
	}
}

func marshalCheckLedger(t *testing.T, events ...Event) []byte {
	t.Helper()
	var b []byte
	for _, e := range events {
		line, err := json.Marshal(e)
		if err != nil {
			t.Fatal(err)
		}
		b = append(b, line...)
		b = append(b, '\n')
	}
	return b
}
