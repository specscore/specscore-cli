package journal

import (
	"testing"
	"time"
)

func ts(t *testing.T) time.Time {
	t.Helper()
	return time.Date(2026, 6, 2, 14, 23, 45, 123456789, time.UTC)
}

func TestEventKey_DateShardedPath(t *testing.T) {
	got := eventKey(ts(t), "mbp", "ab12cd34")
	want := "2026/06/02/2026-06-02T14-23-45.123456789Z-mbp-ab12cd34"
	if got != want {
		t.Errorf("eventKey = %q, want %q", got, want)
	}
}

func TestEventKey_ConvertsToUTC(t *testing.T) {
	// A non-UTC instant must shard/format by its UTC date.
	loc := time.FixedZone("plus5", 5*3600)
	local := time.Date(2026, 1, 1, 2, 0, 0, 0, loc) // 2025-12-31T21:00:00Z
	got := eventKey(local, "m", "u")
	if got[:10] != "2025/12/31" {
		t.Errorf("eventKey date prefix = %q, want 2025/12/31 (UTC)", got[:10])
	}
}

func TestEvent_ToMap_RequiredFields(t *testing.T) {
	e := Event{
		Type: "idea.approved", Timestamp: ts(t), MachineID: "mbp",
		SourceRepo: "/x/specscore", Stream: "specscore",
	}
	m := e.toMap()
	if m["type"] != "idea.approved" || m["machine_id"] != "mbp" ||
		m["source_repo"] != "/x/specscore" || m["stream"] != "specscore" {
		t.Errorf("toMap missing/incorrect required fields: %#v", m)
	}
	if m["timestamp"] != "2026-06-02T14:23:45.123456789Z" {
		t.Errorf("timestamp = %v, want RFC3339Nano UTC", m["timestamp"])
	}
}

func TestEvent_ToMap_OmitsEmptyOriginAndNilData(t *testing.T) {
	e := Event{Type: "x", Timestamp: ts(t), MachineID: "m", SourceRepo: "/r", Stream: "s"}
	m := e.toMap()
	if _, ok := m["source_origin"]; ok {
		t.Error("source_origin should be omitted when empty")
	}
	if _, ok := m["data"]; ok {
		t.Error("data should be omitted when nil")
	}
}

func TestEvent_ToMap_IncludesOriginAndData(t *testing.T) {
	e := Event{
		Type: "recap.completed", Timestamp: ts(t), MachineID: "m", SourceRepo: "/r", Stream: "s",
		SourceOrigin: "git@github.com:specscore/specscore.git",
		Data:         map[string]any{"report_path": "x/recap/abc.md", "contradiction_count": 0},
	}
	m := e.toMap()
	if m["source_origin"] != "git@github.com:specscore/specscore.git" {
		t.Errorf("source_origin = %v", m["source_origin"])
	}
	d, ok := m["data"].(map[string]any)
	if !ok || d["report_path"] != "x/recap/abc.md" {
		t.Errorf("data not carried through: %#v", m["data"])
	}
}
