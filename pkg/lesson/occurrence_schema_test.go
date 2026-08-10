package lesson

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

func TestOccurrenceContractValidatesAgainstPinnedCoreSchema(t *testing.T) {
	const (
		schemaURL      = "https://specscore.md/new/lesson-occurrence.schema.json"
		schemaRevision = "b16caad0d0693c407bfb4230b492bce5f2aa8458"
		schemaSHA256   = "6d97a499ae0bf11d01395ebdd7921efc97e01840c3bf9388b32edbf933d522ee"
	)
	type provenanceShape struct {
		Repository string `json:"repository"`
		Revision   string `json:"revision"`
		Path       string `json:"path"`
		SchemaID   string `json:"schema_id"`
		SHA256     string `json:"sha256"`
	}
	schemaPath := filepath.Join("..", "..", "schemas", "lesson-occurrence.schema.json")
	provenancePath := filepath.Join("..", "..", "schemas", "lesson-occurrence.schema.provenance.json")
	schemaBytes, err := os.ReadFile(schemaPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := fmt.Sprintf("%x", sha256.Sum256(schemaBytes)); got != schemaSHA256 {
		t.Fatalf("pinned occurrence schema digest drifted: %s", got)
	}
	provenanceBytes, err := os.ReadFile(provenancePath)
	if err != nil {
		t.Fatal(err)
	}
	var provenance provenanceShape
	decoder := json.NewDecoder(bytes.NewReader(provenanceBytes))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&provenance); err != nil {
		t.Fatal(err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		t.Fatalf("occurrence schema provenance has trailing JSON: %v", err)
	}
	wantProvenance := provenanceShape{Repository: "github.com/specscore/specscore", Revision: schemaRevision, Path: "new/lesson-occurrence.schema.json", SchemaID: schemaURL, SHA256: schemaSHA256}
	if provenance != wantProvenance {
		t.Fatalf("occurrence schema provenance drifted: got %#v want %#v", provenance, wantProvenance)
	}
	var schemaDocument any
	decoder = json.NewDecoder(bytes.NewReader(schemaBytes))
	decoder.UseNumber()
	if err := decoder.Decode(&schemaDocument); err != nil {
		t.Fatal(err)
	}
	if err := decoder.Decode(&trailing); err != io.EOF {
		t.Fatalf("occurrence schema has trailing JSON: %v", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource(schemaURL, schemaDocument); err != nil {
		t.Fatal(err)
	}
	compiled, err := compiler.Compile(schemaURL)
	if err != nil {
		t.Fatal(err)
	}
	valid := map[string]any{
		"schema_version": json.Number("1"),
		"id":             "01234567-89ab-4def-8123-456789abcdef",
		"occurred_at":    "2026-08-10T10:00:00Z",
		"summary":        "A bounded process gap recurred.",
		"context": map[string]any{
			"repository": "github.com/example/project",
			"git":        map[string]any{"commit": "abcdef1", "branch": "main"},
			"worktree":   map[string]any{"path_hint": "redacted", "id": "sha256:abc"},
			"execution":  map[string]any{"kind": "unknown", "id": "run-1"},
		},
		"evidence":   map[string]any{"kind": "none", "ref": nil},
		"redactions": []any{"prompt omitted"},
	}
	if err := compiled.Validate(valid); err != nil {
		t.Fatalf("known-valid occurrence violates pinned core schema: %v", err)
	}
	valid["prompt"] = "must never persist"
	if err := compiled.Validate(valid); err == nil {
		t.Fatal("pinned occurrence schema accepted an undeclared prompt field")
	}
}

func TestValidateOccurrence_RejectsUnsafeOrNonSchemaContext(t *testing.T) {
	base := Occurrence{SchemaVersion: 1, ID: "01234567-89ab-4def-8123-456789abcdef", OccurredAt: time.Now().UTC(), Summary: "safe", Context: map[string]any{}, Evidence: Evidence{Kind: "none"}}
	for name, context := range map[string]map[string]any{
		"raw prompt":    {"prompt": "do the thing"},
		"string git":    {"git": "main"},
		"absolute path": {"worktree": map[string]any{"path_hint": "/Users/alex"}},
		"bad execution": {"execution": map[string]any{"kind": "claude"}},
		"github token":  {"execution": map[string]any{"id": "ghp_" + "abcdefghijklmnopqrstuvwxyz123456"}},
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

func TestValidateOccurrence_StrictPathAndBoundedValueParity(t *testing.T) {
	ref := "spec/evidence/result.txt"
	base := Occurrence{SchemaVersion: 1, ID: "01234567-89ab-4def-8123-456789abcdef", OccurredAt: time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC), Summary: "safe", Context: map[string]any{"worktree": map[string]any{"path_hint": "redacted"}}, Evidence: Evidence{Kind: "path", Ref: &ref}, Redactions: []string{"prompt omitted"}}
	if err := ValidateOccurrence(base); err != nil {
		t.Fatalf("valid occurrence: %v", err)
	}
	for name, mutate := range map[string]func(*Occurrence){
		"summary newline": func(o *Occurrence) { o.Summary = "first\nsecond" },
		"dot path":        func(o *Occurrence) { o.Context = map[string]any{"worktree": map[string]any{"path_hint": "."}} },
		"dotdot path":     func(o *Occurrence) { o.Context = map[string]any{"worktree": map[string]any{"path_hint": "a/../b"}} },
		"backslash path":  func(o *Occurrence) { o.Context = map[string]any{"worktree": map[string]any{"path_hint": `a\b`}} },
		"drive path":      func(o *Occurrence) { o.Context = map[string]any{"worktree": map[string]any{"path_hint": "C:/work"}} },
		"evidence dotdot": func(o *Occurrence) { v := "../secret"; o.Evidence = Evidence{Kind: "path", Ref: &v} },
		"bad URL":         func(o *Occurrence) { v := "https://"; o.Evidence = Evidence{Kind: "url", Ref: &v} },
		"duplicate redact": func(o *Occurrence) {
			o.Redactions = []string{"same", "same"}
		},
	} {
		t.Run(name, func(t *testing.T) {
			o := base
			mutate(&o)
			if err := ValidateOccurrence(o); err == nil {
				t.Fatal("expected rejection")
			}
		})
	}
}

func TestValidateOccurrenceFile_RecursivePolicyLexicalUTCAndNoSecretEcho(t *testing.T) {
	dir := t.TempDir()
	id := "01234567-89ab-4def-8123-456789abcdef"
	path := filepath.Join(dir, id+".json")
	secret := "ghp_" + "abcdefghijklmnopqrstuvwxyz123456"
	cases := map[string]string{
		"mixed-case forbidden property": `{"schema_version":1,"id":"` + id + `","occurred_at":"2026-08-10T10:00:00Z","summary":"x","context":{"execution":{"Original_Prompt":"hidden"}},"evidence":{"kind":"none","ref":null},"redactions":[]}`,
		"nested secret":                 `{"schema_version":1,"id":"` + id + `","occurred_at":"2026-08-10T10:00:00Z","summary":"x","context":{"execution":{"id":"` + secret + `"}},"evidence":{"kind":"none","ref":null},"redactions":[]}`,
		"email":                         `{"schema_version":1,"id":"` + id + `","occurred_at":"2026-08-10T10:00:00Z","summary":"contact person` + "@" + `example.com","context":{},"evidence":{"kind":"none","ref":null},"redactions":[]}`,
		"invalid calendar":              `{"schema_version":1,"id":"` + id + `","occurred_at":"2026-02-30T10:00:00Z","summary":"x","context":{},"evidence":{"kind":"none","ref":null},"redactions":[]}`,
		"too precise":                   `{"schema_version":1,"id":"` + id + `","occurred_at":"2026-08-10T10:00:00.1234567890Z","summary":"x","context":{},"evidence":{"kind":"none","ref":null},"redactions":[]}`,
		"trailing JSON":                 `{"schema_version":1,"id":"` + id + `","occurred_at":"2026-08-10T10:00:00Z","summary":"x","context":{},"evidence":{"kind":"none","ref":null},"redactions":[]} {}`,
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := ValidateOccurrenceFile(path)
			if err == nil {
				t.Fatal("expected rejection")
			}
			if strings.Contains(err.Error(), secret) {
				t.Fatal("secret value echoed in validation error")
			}
		})
	}
}

func TestValidateOccurrenceFile_FilenameMustEqualID(t *testing.T) {
	dir := t.TempDir()
	id := "01234567-89ab-4def-8123-456789abcdef"
	other := "11111111-1111-4111-8111-111111111111"
	raw := `{"schema_version":1,"id":"` + id + `","occurred_at":"2026-08-10T10:00:00Z","summary":"safe","context":{},"evidence":{"kind":"none","ref":null},"redactions":[]}`
	path := filepath.Join(dir, other+".json")
	if err := os.WriteFile(path, []byte(raw), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidateOccurrenceFile(path); err == nil || !strings.Contains(err.Error(), "filename and id differ") {
		t.Fatalf("filename/id mismatch accepted: %v", err)
	}
}

func TestAddOccurrence_ExclusiveAtomicPublicationDoesNotOverwrite(t *testing.T) {
	dir := t.TempDir()
	lessonPath := filepath.Join(dir, "spec", "lessons", "safe", "README.md")
	if err := os.MkdirAll(filepath.Join(filepath.Dir(lessonPath), "occurrences"), 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := ScaffoldCanonical(ScaffoldOptions{Slug: "safe", Owner: "tester"}, []string{"process"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(lessonPath, body, 0o644); err != nil {
		t.Fatal(err)
	}
	id := "01234567-89ab-4def-8123-456789abcdef"
	opts := AddOccurrenceOptions{LessonPath: lessonPath, ID: id, Summary: "first", Context: map[string]any{}, Evidence: Evidence{Kind: "none"}, Now: time.Date(2026, 8, 10, 10, 0, 0, 0, time.UTC)}
	first, err := AddOccurrence(opts)
	if err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(first.Path)
	if err != nil {
		t.Fatal(err)
	}
	opts.Summary = "second"
	if _, err := AddOccurrence(opts); err == nil {
		t.Fatal("same ID unexpectedly overwrote immutable occurrence")
	}
	after, err := os.ReadFile(first.Path)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatal("failed append changed existing occurrence bytes")
	}
	entries, err := os.ReadDir(filepath.Dir(first.Path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != id+".json" {
		t.Fatalf("atomic append left temporary artifacts: %#v", entries)
	}
}
