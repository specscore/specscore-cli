package lesson

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestValidateOccurrence_RejectsUnsafeOrNonSchemaContext(t *testing.T) {
	base := Occurrence{SchemaVersion: 1, ID: "01234567-89ab-4def-8123-456789abcdef", OccurredAt: time.Now().UTC(), Summary: "safe", Context: map[string]any{}, Evidence: Evidence{Kind: "none"}}
	for name, context := range map[string]map[string]any{
		"raw prompt":    {"prompt": "do the thing"},
		"string git":    {"git": "main"},
		"absolute path": {"worktree": map[string]any{"path_hint": "/Users/alex"}},
		"bad execution": {"execution": map[string]any{"kind": "claude"}},
		"github token":  {"execution": map[string]any{"id": "ghp_abcdefghijklmnopqrstuvwxyz123456"}},
	} {
		t.Run(name, func(t *testing.T) {
			o := base
			o.Context = context
			if err := ValidateOccurrence(o); err == nil {
				t.Fatal("expected rejection")
			} else if strings.Contains(err.Error(), "ghp_") {
				t.Fatal("secret must not be echoed")
			}
		})
	}
}

func TestDiscoverOccurrences_RejectsUnknownFieldsAndOffsetTimestamp(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "spec", "lessons", "x", "README.md")
	if err := os.MkdirAll(filepath.Join(filepath.Dir(path), "occurrences"), 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := ScaffoldCanonical(ScaffoldOptions{Slug: "x"}, []string{"process"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	id := "01234567-89ab-4def-8123-456789abcdef"
	raw := `{"schema_version":1,"id":"` + id + `","occurred_at":"2026-08-10T10:00:00+00:00","summary":"x","context":{},"evidence":{"kind":"none","ref":null},"redactions":[],"prompt":"no"}`
	if err := os.WriteFile(filepath.Join(filepath.Dir(path), "occurrences", id+".json"), []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := DiscoverOccurrences(path); err == nil {
		t.Fatal("expected strict JSON schema rejection")
	}
}
