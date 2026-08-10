package lesson

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func relationFixture(t *testing.T, slugs ...string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "lessons")
	for _, slug := range slugs {
		path := filepath.Join(dir, slug, "README.md")
		if err := os.MkdirAll(filepath.Join(filepath.Dir(path), "occurrences"), 0o755); err != nil {
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
	return dir
}

func TestAddRelation_SupersedesRefusesCycle(t *testing.T) {
	lessons := relationFixture(t, "a", "b")
	if err := AddRelation(lessons, "a", "supersedes", "b"); err != nil {
		t.Fatal(err)
	}
	beforeA, _ := os.ReadFile(filepath.Join(lessons, "a", "README.md"))
	beforeB, _ := os.ReadFile(filepath.Join(lessons, "b", "README.md"))
	if err := AddRelation(lessons, "b", "supersedes", "a"); err == nil {
		t.Fatal("cycle must be refused before mutation")
	}
	afterA, _ := os.ReadFile(filepath.Join(lessons, "a", "README.md"))
	afterB, _ := os.ReadFile(filepath.Join(lessons, "b", "README.md"))
	if !bytes.Equal(beforeA, afterA) || !bytes.Equal(beforeB, afterB) {
		t.Fatal("cycle rejection mutated a Lesson")
	}
}

func TestAddRelation_DuplicateRetiresOnlyRetainedLessonAndIsVisibleFromTarget(t *testing.T) {
	lessons := relationFixture(t, "retained", "canonical")
	canonicalPath := filepath.Join(lessons, "canonical", "README.md")
	canonicalBefore, _ := os.ReadFile(canonicalPath)
	if err := AddRelation(lessons, "retained", "duplicates", "canonical"); err != nil {
		t.Fatal(err)
	}
	canonicalAfter, _ := os.ReadFile(canonicalPath)
	if !bytes.Equal(canonicalBefore, canonicalAfter) {
		t.Fatal("canonical duplicate target changed")
	}
	retained, _ := os.ReadFile(filepath.Join(lessons, "retained", "README.md"))
	for _, want := range []string{"status: Superseded", "**Status:** Superseded", "**Duplicate Of:** canonical", "**Superseded By:** canonical"} {
		if !strings.Contains(string(retained), want) {
			t.Fatalf("retained duplicate missing %q:\n%s", want, retained)
		}
	}
	relations, err := ListRelations(lessons, "canonical")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, relation := range relations {
		found = found || (relation.From == "retained" && relation.Type == "duplicates" && relation.To == "canonical")
	}
	if !found {
		t.Fatalf("canonical target lacks inverse visibility: %#v", relations)
	}
}

func TestAddRelation_EnforcedDuplicateAndConflictingTargetAreWriteFree(t *testing.T) {
	lessons := relationFixture(t, "a", "b", "c")
	aPath := filepath.Join(lessons, "a", "README.md")
	raw, _ := os.ReadFile(aPath)
	enforced := strings.Replace(string(raw), "status: Recorded", "status: Enforced", 1)
	enforced = strings.Replace(enforced, "**Status:** Recorded", "**Status:** Enforced", 1)
	if err := os.WriteFile(aPath, []byte(enforced), 0o644); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(aPath)
	if err := AddRelation(lessons, "a", "duplicates", "b"); err == nil {
		t.Fatal("Enforced duplicate must be rejected")
	}
	after, _ := os.ReadFile(aPath)
	if !bytes.Equal(before, after) {
		t.Fatal("Enforced duplicate rejection mutated source")
	}

	recorded := strings.Replace(enforced, "status: Enforced", "status: Recorded", 1)
	recorded = strings.Replace(recorded, "**Status:** Enforced", "**Status:** Recorded", 1)
	recorded = strings.Replace(recorded, "**Duplicate Of:** —", "**Duplicate Of:** c", 1)
	if err := os.WriteFile(aPath, []byte(recorded), 0o644); err != nil {
		t.Fatal(err)
	}
	before, _ = os.ReadFile(aPath)
	if err := AddRelation(lessons, "a", "duplicates", "b"); err == nil {
		t.Fatal("conflicting duplicate target must be rejected")
	}
	after, _ = os.ReadFile(aPath)
	if !bytes.Equal(before, after) {
		t.Fatal("conflict rejection mutated source")
	}
}

func TestAddRelation_MalformedSidecarIsFatalAndWriteFree(t *testing.T) {
	lessons := relationFixture(t, "a", "b")
	sidecar := filepath.Join(lessons, relatedRelationsFile)
	if err := os.WriteFile(sidecar, []byte(`[{"from":"a","type":"related","to":"b","ignored":true}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(sidecar)
	if err := AddRelation(lessons, "a", "related", "b"); err == nil {
		t.Fatal("malformed sidecar must be fatal")
	}
	after, _ := os.ReadFile(sidecar)
	if !bytes.Equal(before, after) {
		t.Fatal("malformed sidecar was overwritten")
	}
}

func TestAddRelation_SecondPublishFailureRollsBackFirstLesson(t *testing.T) {
	lessons := relationFixture(t, "successor", "prior")
	from := filepath.Join(lessons, "successor", "README.md")
	to := filepath.Join(lessons, "prior", "README.md")
	beforeFrom, _ := os.ReadFile(from)
	beforeTo, _ := os.ReadFile(to)
	orig := relationRename
	calls := 0
	relationRename = func(old, new string) error {
		calls++
		if calls == 2 {
			return errors.New("injected second publish failure")
		}
		return os.Rename(old, new)
	}
	t.Cleanup(func() { relationRename = orig })
	if err := AddRelation(lessons, "successor", "supersedes", "prior"); err == nil {
		t.Fatal("expected injected failure")
	}
	afterFrom, _ := os.ReadFile(from)
	afterTo, _ := os.ReadFile(to)
	if !bytes.Equal(beforeFrom, afterFrom) || !bytes.Equal(beforeTo, afterTo) {
		t.Fatal("failed two-Lesson publication was not rolled back")
	}
}
