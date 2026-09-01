package event

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeYAML is a small helper for the config tests; it writes the canonical
// schema-header line followed by `body` to <dir>/specscore.yaml.
func writeYAML(t *testing.T, dir, body string) {
	t.Helper()
	header := "# SpecScore Repo Config Schema: https://specscore.md/repo-config\n\n"
	path := filepath.Join(dir, "specscore.yaml")
	if err := os.WriteFile(path, []byte(header+body), 0o644); err != nil {
		t.Fatalf("write specscore.yaml: %v", err)
	}
}

func TestLoadSubscribers_DefaultWhenAbsent(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "project:\n  title: Test\n")

	subs, err := LoadSubscribers(dir)
	if err != nil {
		t.Fatalf("LoadSubscribers: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscriber, got %d", len(subs))
	}
	jw, ok := subs[0].(*JsonlWriter)
	if !ok {
		t.Fatalf("expected *JsonlWriter, got %T", subs[0])
	}
	wantSuffix := filepath.Join(".specscore", "events.jsonl")
	if !strings.HasSuffix(jw.Name(), wantSuffix) {
		t.Fatalf("expected Name() to end with %q, got %q", wantSuffix, jw.Name())
	}
}

func TestLoadSubscribers_ExplicitEmpty(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "project:\n  title: Test\nevents:\n  subscribers: []\n")

	subs, err := LoadSubscribers(dir)
	if err != nil {
		t.Fatalf("LoadSubscribers: %v", err)
	}
	if len(subs) != 0 {
		t.Fatalf("expected 0 subscribers, got %d", len(subs))
	}
}

func TestLoadSubscribers_UnknownType(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `project:
  title: Test
events:
  subscribers:
    - type: webhook
      url: "https://example.com"
`)

	_, err := LoadSubscribers(dir)
	if err == nil {
		t.Fatal("expected error for unknown subscriber type, got nil")
	}
	msg := err.Error()
	for _, want := range []string{
		"specscore.yaml",
		"events.subscribers[0].type",
		"webhook",
		"jsonl",
		"noop",
		"exec",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message missing %q; got: %s", want, msg)
		}
	}
}

func TestLoadSubscribers_MixedValid(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `project:
  title: Test
events:
  subscribers:
    - type: jsonl
      path: custom/events.jsonl
    - type: exec
      command: [/bin/true]
      timeout_ms: 1500
    - type: noop
`)

	subs, err := LoadSubscribers(dir)
	if err != nil {
		t.Fatalf("LoadSubscribers: %v", err)
	}
	if len(subs) != 3 {
		t.Fatalf("expected 3 subscribers, got %d", len(subs))
	}
	if _, ok := subs[0].(*JsonlWriter); !ok {
		t.Errorf("subs[0]: expected *JsonlWriter, got %T", subs[0])
	}
	if _, ok := subs[1].(*Exec); !ok {
		t.Errorf("subs[1]: expected *Exec, got %T", subs[1])
	}
	if _, ok := subs[2].(NoOp); !ok {
		t.Errorf("subs[2]: expected NoOp, got %T", subs[2])
	}
}

func TestLoadSubscribers_MissingJsonlPath(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `project:
  title: Test
events:
  subscribers:
    - type: jsonl
`)
	_, err := LoadSubscribers(dir)
	if err == nil {
		t.Fatal("expected error for missing jsonl path, got nil")
	}
	msg := err.Error()
	for _, want := range []string{"specscore.yaml", "events.subscribers[0].path"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message missing %q; got: %s", want, msg)
		}
	}
}

func TestLoadSubscribers_MissingExecCommand(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `project:
  title: Test
events:
  subscribers:
    - type: exec
`)
	_, err := LoadSubscribers(dir)
	if err == nil {
		t.Fatal("expected error for missing exec command, got nil")
	}
	if !strings.Contains(err.Error(), "events.subscribers[0].command") {
		t.Errorf("error message missing key path; got: %s", err.Error())
	}
}

func TestLoadSubscribers_TimeoutOutOfRange(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `project:
  title: Test
events:
  subscribers:
    - type: exec
      command: [/bin/true]
      timeout_ms: 50
`)
	_, err := LoadSubscribers(dir)
	if err == nil {
		t.Fatal("expected error for timeout_ms < 100, got nil")
	}
	if !strings.Contains(err.Error(), "events.subscribers[0].timeout_ms") {
		t.Errorf("error message missing key path; got: %s", err.Error())
	}
}

func TestLoadSubscribers_NoConfigFile(t *testing.T) {
	dir := t.TempDir()
	// No specscore.yaml at all — treat as absent events: block.
	subs, err := LoadSubscribers(dir)
	if err != nil {
		t.Fatalf("LoadSubscribers: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("expected default JsonlWriter, got %d subs", len(subs))
	}
	if _, ok := subs[0].(*JsonlWriter); !ok {
		t.Fatalf("expected *JsonlWriter, got %T", subs[0])
	}
}

// TestLoadSubscribers_ReadError covers the non-ErrNotExist read error branch
// (e.g., a directory where the file is expected).
func TestLoadSubscribers_ReadError(t *testing.T) {
	dir := t.TempDir()
	// Create specscore.yaml as a directory so ReadFile fails with a non-ErrNotExist error.
	if err := os.MkdirAll(filepath.Join(dir, "specscore.yaml"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := LoadSubscribers(dir)
	if err == nil {
		t.Fatal("expected error when specscore.yaml is a directory, got nil")
	}
	if !strings.Contains(err.Error(), "read specscore.yaml") {
		t.Errorf("error message missing prefix; got: %s", err.Error())
	}
}

func TestConfiguredLedgerPath_DefaultAndAlias(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "project:\n  title: Test\n")
	want := filepath.Join(dir, ".specscore", "events.jsonl")
	got, err := ConfiguredLedgerPath(dir)
	if err != nil || got != want {
		t.Fatalf("ConfiguredLedgerPath = %q, %v; want %q", got, err, want)
	}
	got, err = EventLedgerPath(dir)
	if err != nil || got != want {
		t.Fatalf("EventLedgerPath = %q, %v; want %q", got, err, want)
	}
}

func TestConfiguredLedgerPath_RejectsMissingAndAmbiguousTargets(t *testing.T) {
	missing := t.TempDir()
	writeYAML(t, missing, "project:\n  title: Test\nevents:\n  subscribers: []\n")
	if _, err := ConfiguredLedgerPath(missing); err == nil || !strings.Contains(err.Error(), "no jsonl event ledger") {
		t.Fatalf("missing target error = %v", err)
	}

	ambiguous := t.TempDir()
	writeYAML(t, ambiguous, `project:
  title: Test
events:
  subscribers:
    - type: jsonl
      path: one.jsonl
    - type: noop
    - type: jsonl
      path: two.jsonl
`)
	if _, err := ConfiguredLedgerPath(ambiguous); err == nil || !strings.Contains(err.Error(), "ambiguous") {
		t.Fatalf("ambiguous target error = %v", err)
	}

}

// TestLoadSubscribers_InvalidYAML covers the YAML parse-error branch.
func TestLoadSubscribers_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "specscore.yaml")
	if err := os.WriteFile(path, []byte(":\n  :\n[invalid yaml"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := LoadSubscribers(dir)
	if err == nil {
		t.Fatal("expected error for invalid YAML, got nil")
	}
	if !strings.Contains(err.Error(), "parse") {
		t.Errorf("error message missing 'parse'; got: %s", err.Error())
	}
}

// TestLoadSubscribers_EventsPresentNoSubscribersKey covers the branch where
// the events: key is present but the subscribers: key is absent.
func TestLoadSubscribers_EventsPresentNoSubscribersKey(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "project:\n  title: Test\nevents:\n  other_key: true\n")

	subs, err := LoadSubscribers(dir)
	if err != nil {
		t.Fatalf("LoadSubscribers: %v", err)
	}
	if len(subs) != 0 {
		t.Fatalf("expected 0 subscribers, got %d", len(subs))
	}
}

// TestLoadSubscribers_EventsDecodeError covers the events block decode error
// path (e.g., subscribers is not a sequence).
func TestLoadSubscribers_EventsDecodeError(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, "project:\n  title: Test\nevents:\n  subscribers: not-a-list\n")

	_, err := LoadSubscribers(dir)
	if err == nil {
		t.Fatal("expected error for malformed events block, got nil")
	}
	if !strings.Contains(err.Error(), "events") {
		t.Errorf("error message missing 'events'; got: %s", err.Error())
	}
}

// TestLoadSubscribers_TimeoutTooHigh covers the upper bound of timeout_ms.
func TestLoadSubscribers_TimeoutTooHigh(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `project:
  title: Test
events:
  subscribers:
    - type: exec
      command: [/bin/true]
      timeout_ms: 99999
`)
	_, err := LoadSubscribers(dir)
	if err == nil {
		t.Fatal("expected error for timeout_ms > 30000, got nil")
	}
	if !strings.Contains(err.Error(), "timeout_ms") {
		t.Errorf("error message missing 'timeout_ms'; got: %s", err.Error())
	}
}

// TestLoadSubscribers_TimeoutNotInteger covers a non-integer timeout_ms value.
func TestLoadSubscribers_TimeoutNotInteger(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `project:
  title: Test
events:
  subscribers:
    - type: exec
      command: [/bin/true]
      timeout_ms: "not-a-number"
`)
	_, err := LoadSubscribers(dir)
	if err == nil {
		t.Fatal("expected error for non-integer timeout_ms, got nil")
	}
	if !strings.Contains(err.Error(), "timeout_ms") {
		t.Errorf("error message missing 'timeout_ms'; got: %s", err.Error())
	}
	if !strings.Contains(err.Error(), "must be an integer") {
		t.Errorf("error message missing 'must be an integer'; got: %s", err.Error())
	}
}

// TestLoadSubscribers_ExecWithEnv covers the exec subscriber with an env mapping.
func TestLoadSubscribers_ExecWithEnv(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `project:
  title: Test
events:
  subscribers:
    - type: exec
      command: [/bin/true]
      env:
        FOO: bar
        BAZ: qux
`)
	subs, err := LoadSubscribers(dir)
	if err != nil {
		t.Fatalf("LoadSubscribers: %v", err)
	}
	if len(subs) != 1 {
		t.Fatalf("expected 1 subscriber, got %d", len(subs))
	}
	if _, ok := subs[0].(*Exec); !ok {
		t.Fatalf("expected *Exec, got %T", subs[0])
	}
}

// TestLoadSubscribers_ExecBadEnv covers the exec subscriber with a non-mapping env.
func TestLoadSubscribers_ExecBadEnv(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `project:
  title: Test
events:
  subscribers:
    - type: exec
      command: [/bin/true]
      env: not-a-mapping
`)
	_, err := LoadSubscribers(dir)
	if err == nil {
		t.Fatal("expected error for non-mapping env, got nil")
	}
	if !strings.Contains(err.Error(), "env") {
		t.Errorf("error message missing 'env'; got: %s", err.Error())
	}
}

// TestLoadSubscribers_ExecBadCommandType covers an exec command that is not a sequence.
func TestLoadSubscribers_ExecBadCommandType(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `project:
  title: Test
events:
  subscribers:
    - type: exec
      command: /bin/true
`)
	_, err := LoadSubscribers(dir)
	if err == nil {
		t.Fatal("expected error for command that is not a sequence, got nil")
	}
	if !strings.Contains(err.Error(), "command") {
		t.Errorf("error message missing 'command'; got: %s", err.Error())
	}
}

// TestLoadSubscribers_EmptyType covers the case where type field is missing
// (defaults to empty string, hits the default branch).
func TestLoadSubscribers_EmptyType(t *testing.T) {
	dir := t.TempDir()
	writeYAML(t, dir, `project:
  title: Test
events:
  subscribers:
    - path: foo.jsonl
`)
	_, err := LoadSubscribers(dir)
	if err == nil {
		t.Fatal("expected error for missing type, got nil")
	}
	if !strings.Contains(err.Error(), "unknown value") {
		t.Errorf("error message missing 'unknown value'; got: %s", err.Error())
	}
}

// TestDecodeArgv covers decodeArgv edge cases directly.
func TestDecodeArgv(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		out, err := decodeArgv(nil)
		if err != nil {
			t.Fatalf("decodeArgv(nil) error: %v", err)
		}
		if out != nil {
			t.Fatalf("decodeArgv(nil) = %v, want nil", out)
		}
	})

	t.Run("not_a_sequence", func(t *testing.T) {
		_, err := decodeArgv("a string")
		if err == nil {
			t.Fatal("expected error for non-sequence, got nil")
		}
	})

	t.Run("non_string_item", func(t *testing.T) {
		_, err := decodeArgv([]any{"ok", 42})
		if err == nil {
			t.Fatal("expected error for non-string item, got nil")
		}
	})

	t.Run("valid", func(t *testing.T) {
		out, err := decodeArgv([]any{"echo", "hello"})
		if err != nil {
			t.Fatalf("decodeArgv error: %v", err)
		}
		if len(out) != 2 || out[0] != "echo" || out[1] != "hello" {
			t.Fatalf("decodeArgv = %v, want [echo hello]", out)
		}
	})
}

// TestDecodeEnv covers decodeEnv edge cases directly.
func TestDecodeEnv(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		out, err := decodeEnv(nil)
		if err != nil {
			t.Fatalf("decodeEnv(nil) error: %v", err)
		}
		if out != nil {
			t.Fatalf("decodeEnv(nil) = %v, want nil", out)
		}
	})

	t.Run("not_a_mapping", func(t *testing.T) {
		_, err := decodeEnv("a string")
		if err == nil {
			t.Fatal("expected error for non-mapping, got nil")
		}
	})

	t.Run("non_string_value", func(t *testing.T) {
		_, err := decodeEnv(map[string]any{"key": 42})
		if err == nil {
			t.Fatal("expected error for non-string value, got nil")
		}
	})

	t.Run("valid", func(t *testing.T) {
		out, err := decodeEnv(map[string]any{"FOO": "bar"})
		if err != nil {
			t.Fatalf("decodeEnv error: %v", err)
		}
		if out["FOO"] != "bar" {
			t.Fatalf("decodeEnv = %v, want map[FOO:bar]", out)
		}
	})
}

// TestToInt covers toInt edge cases directly.
func TestToInt(t *testing.T) {
	t.Run("int", func(t *testing.T) {
		n, ok := toInt(42)
		if !ok || n != 42 {
			t.Fatalf("toInt(42) = (%d, %v), want (42, true)", n, ok)
		}
	})

	t.Run("int64", func(t *testing.T) {
		n, ok := toInt(int64(99))
		if !ok || n != 99 {
			t.Fatalf("toInt(int64(99)) = (%d, %v), want (99, true)", n, ok)
		}
	})

	t.Run("float64_integral", func(t *testing.T) {
		n, ok := toInt(float64(500))
		if !ok || n != 500 {
			t.Fatalf("toInt(500.0) = (%d, %v), want (500, true)", n, ok)
		}
	})

	t.Run("float64_non_integral", func(t *testing.T) {
		_, ok := toInt(float64(1.5))
		if ok {
			t.Fatal("toInt(1.5) should return ok=false")
		}
	})

	t.Run("string", func(t *testing.T) {
		_, ok := toInt("not a number")
		if ok {
			t.Fatal("toInt(string) should return ok=false")
		}
	})

	t.Run("bool", func(t *testing.T) {
		_, ok := toInt(true)
		if ok {
			t.Fatal("toInt(bool) should return ok=false")
		}
	})
}

// TestExtractEventsNode_NonYAML covers an edge case in extractEventsNode
// where the top-level is a non-mapping (e.g. a scalar).
func TestExtractEventsNode_NonYAML(t *testing.T) {
	// YAML that is just a scalar string — root is not a MappingNode.
	present, node, err := extractEventsNode([]byte("just a string\n"))
	if err != nil {
		t.Fatalf("extractEventsNode error: %v", err)
	}
	if present {
		t.Fatal("expected present=false for non-mapping root")
	}
	if node != nil {
		t.Fatal("expected node=nil for non-mapping root")
	}
}

// TestExtractEventsNode_EmptyDocument covers the empty-document edge case.
func TestExtractEventsNode_EmptyDocument(t *testing.T) {
	present, node, err := extractEventsNode([]byte(""))
	if err != nil {
		t.Fatalf("extractEventsNode error: %v", err)
	}
	if present {
		t.Fatal("expected present=false for empty document")
	}
	if node != nil {
		t.Fatal("expected node=nil for empty document")
	}
}
