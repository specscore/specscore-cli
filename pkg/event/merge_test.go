package event

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func mergeTestEvent(id string, payload string) Event {
	ts, _ := time.Parse(time.RFC3339, "2026-08-31T12:00:00Z")
	return Event{
		Name: "lesson.observed", Version: 1, UUID: id, Timestamp: ts,
		Actor:    Actor{Kind: "external", ID: "merge-test"},
		Artifact: Artifact{Type: "lesson", ID: "ledger", Path: "spec/lessons/ledger/README.md", Revision: "uncommitted"},
		Payload:  json.RawMessage(payload),
	}
}

func writeMergeLedger(t *testing.T, path string, events ...Event) []byte {
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
	if err := os.WriteFile(path, b, 0o644); err != nil {
		t.Fatal(err)
	}
	return b
}

func TestMergeLedgers_ConcurrentSourcesPreserveTargetAndSortAdditions(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.jsonl")
	sourceA := filepath.Join(dir, "branch-a.jsonl")
	sourceB := filepath.Join(dir, "branch-b.jsonl")
	targetBytes := writeMergeLedger(t, target, mergeTestEvent("00000000-0000-4000-8000-000000000001", `{"target":true}`))
	writeMergeLedger(t, sourceA, mergeTestEvent("00000000-0000-4000-8000-000000000003", `{"branch":"a"}`))
	writeMergeLedger(t, sourceB, mergeTestEvent("00000000-0000-4000-8000-000000000002", `{"branch":"b"}`))

	result, err := MergeLedgers(target, []string{sourceB, sourceA})
	if err != nil {
		t.Fatalf("MergeLedgers: %v", err)
	}
	if result.Existing != 1 || result.Added != 2 {
		t.Fatalf("result = %#v, want existing=1 added=2", result)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(got), string(targetBytes)) {
		t.Fatalf("target prefix was rewritten:\n got %q\nwant prefix %q", got, targetBytes)
	}
	lines := strings.Split(strings.TrimSuffix(string(got), "\n"), "\n")
	if len(lines) != 3 || !strings.Contains(lines[1], "000000000002") || !strings.Contains(lines[2], "000000000003") {
		t.Fatalf("merged lines are not target-then-UUID order: %v", lines)
	}
}

func TestMergeLedgers_IdenticalCanonicalDuplicateIsSkippedAndIdempotent(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.jsonl")
	source := filepath.Join(dir, "source.jsonl")
	e := mergeTestEvent("00000000-0000-4000-8000-000000000010", `{"b":2,"a":1}`)
	original := []byte(`{"payload":{"a":1,"b":2},"artifact":{"revision":"uncommitted","path":"spec/lessons/ledger/README.md","id":"ledger","type":"lesson"},"actor":{"id":"merge-test","kind":"external"},"timestamp":"2026-08-31T12:00:00+00:00","uuid":"00000000-0000-4000-8000-000000000010","version":1,"name":"lesson.observed"}` + "\n")
	if err := os.WriteFile(target, original, 0o644); err != nil {
		t.Fatal(err)
	}
	writeMergeLedger(t, source, e)

	result, err := MergeLedgers(target, []string{source})
	if err != nil {
		t.Fatalf("MergeLedgers duplicate: %v", err)
	}
	if result.Added != 0 || result.Skipped != 1 {
		t.Fatalf("result = %#v, want skipped=1 and no append", result)
	}
	got, _ := os.ReadFile(target)
	if string(got) != string(original) {
		t.Fatalf("identical duplicate rewrote target bytes")
	}
	second, err := MergeLedgers(target, []string{source})
	if err != nil || second.Added != 0 {
		t.Fatalf("idempotent rerun = %#v, %v", second, err)
	}
	got, _ = os.ReadFile(target)
	if string(got) != string(original) {
		t.Fatalf("idempotent rerun changed target bytes")
	}
}

func TestMergeLedgers_ConflictMalformedAndSelfLeaveTargetUnchanged(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.jsonl")
	source := filepath.Join(dir, "source.jsonl")
	e := mergeTestEvent("00000000-0000-4000-8000-000000000020", `{"ok":true}`)
	original := writeMergeLedger(t, target, mergeTestEvent("00000000-0000-4000-8000-000000000001", `{"target":true}`))

	writeMergeLedger(t, source, e)
	if err := os.WriteFile(source, append(mustMergeJSON(t, e), '\n', '{', 'b'), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := MergeLedgers(target, []string{source}); err == nil || !IsMergeInputError(err) {
		t.Fatal("expected malformed source merge to fail")
	}
	assertMergeUnchanged(t, target, original)

	writeMergeLedger(t, source, mergeTestEvent("00000000-0000-4000-8000-000000000001", `{"ok":false}`))
	if _, err := MergeLedgers(target, []string{source}); err == nil || !IsMergeInputError(err) {
		t.Fatalf("expected conflicting duplicate input error, got %v", err)
	}
	assertMergeUnchanged(t, target, original)

	if _, err := MergeLedgers(target, []string{target}); err == nil || !IsMergeInputError(err) {
		t.Fatalf("expected self-merge input error, got %v", err)
	}
	assertMergeUnchanged(t, target, original)
}

func mustMergeJSON(t *testing.T, e Event) []byte {
	t.Helper()
	b, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func assertMergeUnchanged(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != string(want) {
		t.Fatalf("target changed after failed merge: got %q want %q", got, want)
	}
}
