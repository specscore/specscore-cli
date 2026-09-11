package event

// This file closes out pre-existing statement-coverage gaps in pkg/event
// discovered while landing an unrelated feature (Plan repository routing):
// the repo's coverage gate (scripts/coverage-gate.sh, run by CI's "Build,
// vet, test" job) measures the WHOLE module in one profile, so any package
// below 100% blocks every PR regardless of whether that PR touches it. These
// branches reproduced consistently (verified with `go test -count=5`, not a
// one-off flake), so closing them here is the correct fix rather than
// treating them as someone else's problem.

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestConfiguredLedgerPathPropagatesLoadSubscribersError(t *testing.T) {
	root := t.TempDir()
	// A directory where LoadSubscribers expects to os.ReadFile a regular
	// file makes the read fail with something other than "not exist".
	if err := os.Mkdir(filepath.Join(root, "specscore.yaml"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := ConfiguredLedgerPath(root); err == nil {
		t.Fatal("expected LoadSubscribers's read error to propagate")
	}
}

func TestMergeLedgersSourceStatErrorWhenTargetExists(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "target.jsonl")
	source := filepath.Join(dir, "source.jsonl")
	writeMergeLedger(t, target, mergeTestEvent("00000000-0000-4000-8000-000000000091", `{"target":true}`))
	writeMergeLedger(t, source, mergeTestEvent("00000000-0000-4000-8000-000000000092", `{"source":true}`))

	original := mergeStatFn
	t.Cleanup(func() { mergeStatFn = original })
	mergeStatFn = func(path string) (os.FileInfo, error) {
		if path == source {
			return nil, errors.New("boom source stat")
		}
		return os.Stat(path)
	}
	// target genuinely exists (targetInfo != nil after a real, successful
	// stat), so the source-stat failure is reached specifically inside the
	// "does this source alias the target" check, not the earlier
	// target-stat branch an existing test already covers.
	if _, err := MergeLedgers(target, []string{source}); err == nil || !IsMergeInputError(err) {
		t.Fatalf("err = %v, want MergeInputError", err)
	}
}

func TestDecodeUniqueJSONValueClosingTokenErrors(t *testing.T) {
	// dec.More() only inspects the next non-whitespace byte for a comma; it
	// does not check that a following '}'/']' actually matches the
	// composite currently being decoded. So a mismatched closer (an object
	// closed with ']', or an array closed with '}') makes dec.More() report
	// "no more elements" one token early, and the mismatch only surfaces
	// once decodeUniqueJSONValue consumes what it assumed was the matching
	// closing delimiter.
	t.Run("object closed with mismatched bracket", func(t *testing.T) {
		dec := json.NewDecoder(bytes.NewReader([]byte(`{"a":"x"]`)))
		if _, err := decodeUniqueJSONValue(dec); err == nil || !strings.Contains(err.Error(), "closing JSON object") {
			t.Fatalf("err = %v, want a wrapped closing-JSON-object error", err)
		}
	})
	t.Run("array closed with mismatched bracket", func(t *testing.T) {
		dec := json.NewDecoder(bytes.NewReader([]byte(`["x"}`)))
		if _, err := decodeUniqueJSONValue(dec); err == nil || !strings.Contains(err.Error(), "closing JSON array") {
			t.Fatalf("err = %v, want a wrapped closing-JSON-array error", err)
		}
	})
}

func TestDecodeLedgerEventUnmarshalErrors(t *testing.T) {
	t.Run("name wrong type", func(t *testing.T) {
		_, _, err := decodeLedgerEvent([]byte(`{"name": 123}`))
		if err == nil || !strings.Contains(err.Error(), "malformed JSON or unknown field") {
			t.Fatalf("err = %v", err)
		}
	})
	t.Run("legacy event wrong type", func(t *testing.T) {
		_, _, err := decodeLedgerEvent([]byte(`{"event": 123}`))
		if err == nil || !strings.Contains(err.Error(), "malformed JSON or unknown field: legacy event") {
			t.Fatalf("err = %v", err)
		}
	})
}
