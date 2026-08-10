package lesson

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAddRelation_RequiresHumanPathAndRefusesCycle(t *testing.T) {
	dir := t.TempDir()
	lessons := filepath.Join(dir, "lessons")
	for _, slug := range []string{"a", "b"} {
		path := filepath.Join(lessons, slug, "README.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		body, err := ScaffoldCanonical(ScaffoldOptions{Slug: slug}, []string{"process"})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if err := AddRelation(lessons, "a", "supersedes", "b"); err != nil {
		t.Fatal(err)
	}
	if err := AddRelation(lessons, "b", "supersedes", "a"); err == nil {
		t.Fatal("cycle must be refused before mutation")
	}
	if err := AddRelation(lessons, "a", "duplicates", "b"); err != nil {
		t.Fatal(err)
	}
	r, err := ListRelations(lessons, "a")
	if err != nil {
		t.Fatal(err)
	}
	if len(r) == 0 {
		t.Fatal("relation list empty")
	}
}
