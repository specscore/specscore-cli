package ingr

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/internal/studio/fact"
)

func sampleFact() fact.Fact {
	return fact.Fact{
		Subject:    "repo-a#x",
		Predicate:  "has-status",
		Object:     "Approved",
		Evidence:   fact.Evidence{Class: fact.Declared, Pointer: "spec/features/x/README.md:3"},
		Adapter:    fact.Adapter{ID: "specscore", Version: "0.1.0"},
		ObservedAt: "2026-07-10T00:00:00Z",
		Ecosystem:  "demo",
	}
}

func TestExport_WritesOneRecordsetPerRepo(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ingr")
	repos := []Repo{
		{Slug: "repo-a", Facts: []fact.Fact{sampleFact()}},
		{Slug: "repo-b"}, // zero facts still gets an (empty) recordset
	}

	if err := Export(dir, repos); err != nil {
		t.Fatalf("Export: %v", err)
	}

	a, err := os.ReadFile(filepath.Join(dir, "repo-a", "facts.ingr"))
	if err != nil {
		t.Fatalf("reading repo-a recordset: %v", err)
	}
	want := header + "\n" +
		`"repo-a#x"` + "\n" +
		`"has-status"` + "\n" +
		`"Approved"` + "\n" +
		`"declared"` + "\n" +
		`"spec/features/x/README.md:3"` + "\n" +
		`"specscore"` + "\n" +
		`"0.1.0"` + "\n" +
		`"2026-07-10T00:00:00Z"` + "\n" +
		`"demo"` + "\n" +
		"# 1 records\n"
	if string(a) != want {
		t.Errorf("repo-a recordset:\n%s\nwant:\n%s", a, want)
	}

	b, err := os.ReadFile(filepath.Join(dir, "repo-b", "facts.ingr"))
	if err != nil {
		t.Fatalf("reading repo-b recordset: %v", err)
	}
	if got, want := string(b), header+"\n# 0 records\n"; got != want {
		t.Errorf("empty recordset:\n%s\nwant:\n%s", got, want)
	}
}

func TestExport_ReplacesPreviousExport(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ingr")
	if err := Export(dir, []Repo{{Slug: "stale"}}); err != nil {
		t.Fatalf("first Export: %v", err)
	}
	if err := Export(dir, []Repo{{Slug: "fresh"}}); err != nil {
		t.Fatalf("second Export: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "stale")); !errors.Is(err, fs.ErrNotExist) {
		t.Errorf("stale repo dir survived the re-export: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "fresh", "facts.ingr")); err != nil {
		t.Errorf("fresh repo recordset missing: %v", err)
	}
}

func TestExport_RemoveAllError(t *testing.T) {
	old := osRemoveAllFn
	osRemoveAllFn = func(string) error { return errors.New("rm boom") }
	t.Cleanup(func() { osRemoveAllFn = old })

	err := Export(filepath.Join(t.TempDir(), "ingr"), nil)
	if err == nil || !strings.Contains(err.Error(), "clearing INGR export directory") {
		t.Errorf("want clearing error, got %v", err)
	}
}

func TestExport_MkdirAllError(t *testing.T) {
	old := osMkdirAllFn
	osMkdirAllFn = func(string, os.FileMode) error { return errors.New("mkdir boom") }
	t.Cleanup(func() { osMkdirAllFn = old })

	err := Export(filepath.Join(t.TempDir(), "ingr"), []Repo{{Slug: "repo-a"}})
	if err == nil || !strings.Contains(err.Error(), "creating INGR export directory") {
		t.Errorf("want mkdir error, got %v", err)
	}
}

func TestExport_WriteFileError(t *testing.T) {
	old := osWriteFileFn
	osWriteFileFn = func(string, []byte, os.FileMode) error { return errors.New("write boom") }
	t.Cleanup(func() { osWriteFileFn = old })

	err := Export(filepath.Join(t.TempDir(), "ingr"), []Repo{{Slug: "repo-a"}})
	if err == nil || !strings.Contains(err.Error(), "writing INGR recordset") {
		t.Errorf("want write error, got %v", err)
	}
}
