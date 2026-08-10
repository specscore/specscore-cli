package lesson

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestOccurrenceCompensationAndRelationVisibility(t *testing.T) {
	lessons := relationFixture(t, "anchor", "retired")
	path := filepath.Join(lessons, "anchor", "README.md")
	o, err := AddOccurrence(AddOccurrenceOptions{LessonPath: path, ID: "01234567-89ab-4def-8123-456789abcdef", Summary: "Compensate the exact published child.", Now: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	if err := RemoveOccurrence(o.Path); err != nil {
		t.Fatal(err)
	}
	if err := RemoveOccurrence(o.Path); err != nil {
		t.Fatal(err)
	}
	if got, err := DiscoverOccurrences(path); err != nil || len(got) != 0 {
		t.Fatalf("compensation = %#v, %v", got, err)
	}
	if err := AddRelation(lessons, "anchor", "related", "retired"); err != nil {
		t.Fatal(err)
	}
	retired := filepath.Join(lessons, "retired", "README.md")
	b, err := os.ReadFile(retired)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(retired, []byte(strings.Replace(string(b), "**Superseded By:** —", "**Superseded By:** anchor", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	rels, err := ListRelations(lessons, "anchor")
	if err != nil {
		t.Fatal(err)
	}
	if len(rels) != 2 || RelationToken("anchor", "related", "retired") == RelationToken("retired", "related", "anchor") {
		t.Fatalf("relations = %#v", rels)
	}
}
