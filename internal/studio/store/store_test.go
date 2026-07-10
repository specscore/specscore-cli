package store

import (
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/internal/studio/fact"
	"github.com/specscore/specscore-cli/pkg/exitcode"
)

func sampleFacts() []fact.Fact {
	return []fact.Fact{
		{
			Subject:    "pkg:a",
			Predicate:  "imports",
			Object:     "pkg:b",
			Evidence:   fact.Evidence{Class: fact.Derived, Pointer: "codegraph/pkg.json"},
			Adapter:    fact.Adapter{ID: "codegraph", Version: "1"},
			ObservedAt: "2026-07-10T00:00:00Z",
			Ecosystem:  "demo",
		},
		{
			Subject:    "repo#x",
			Predicate:  "has-status",
			Object:     "Approved",
			Evidence:   fact.Evidence{Class: fact.Declared, Pointer: "spec/features/x/README.md:9"},
			Adapter:    fact.Adapter{ID: "specscore", Version: "1"},
			ObservedAt: "2026-07-10T00:00:00Z",
			Ecosystem:  "demo",
		},
		{
			Subject:    "pkg:a",
			Predicate:  "imports",
			Object:     "pkg:c",
			Evidence:   fact.Evidence{Class: fact.Derived, Pointer: "codegraph/pkg.json"},
			Adapter:    fact.Adapter{ID: "codegraph", Version: "1"},
			ObservedAt: "2026-07-10T00:00:00Z",
			Ecosystem:  "demo",
		},
	}
}

func newStorePath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), ".specscore-studio", "facts.db")
}

func exit2(t *testing.T, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error")
	}
	var ec interface{ ExitCode() int }
	if !errors.As(err, &ec) {
		t.Fatalf("error carries no exit code: %v", err)
	}
	if ec.ExitCode() != exitcode.InvalidArgs {
		t.Fatalf("exit code = %d, want %d (%v)", ec.ExitCode(), exitcode.InvalidArgs, err)
	}
}

// --- Rebuild + Query round trip ---

func TestRebuildAndQuery_All(t *testing.T) {
	path := newStorePath(t)
	if err := Rebuild(path, sampleFacts()); err != nil {
		t.Fatal(err)
	}
	got, err := Query(path, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("Query returned %d facts, want 3", len(got))
	}
	// Full fact shape survives the round trip (deterministic order).
	want := sampleFacts()[1] // repo#x sorts after pkg:a rows
	if got[2] != want {
		t.Errorf("round-tripped fact = %+v, want %+v", got[2], want)
	}
}

func TestRebuild_ReplacesWholeStore(t *testing.T) {
	path := newStorePath(t)
	if err := Rebuild(path, sampleFacts()); err != nil {
		t.Fatal(err)
	}
	// Second rebuild with a single different fact: nothing survives.
	one := sampleFacts()[:1]
	if err := Rebuild(path, one); err != nil {
		t.Fatal(err)
	}
	got, err := Query(path, Filter{})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0] != one[0] {
		t.Errorf("store after rebuild = %+v, want exactly %+v", got, one[0])
	}
}

func TestRebuild_FailureLeavesPreviousStoreIntact(t *testing.T) {
	path := newStorePath(t)
	if err := Rebuild(path, sampleFacts()); err != nil {
		t.Fatal(err)
	}
	bad := []fact.Fact{{Subject: "s", Predicate: "p", Object: "o",
		Evidence: fact.Evidence{Class: "bogus", Pointer: "x"}}}
	if err := Rebuild(path, bad); err == nil {
		t.Fatal("Rebuild with an invalid evidence class did not fail")
	}
	got, err := Query(path, Filter{})
	if err != nil {
		t.Fatalf("previous store not intact: %v", err)
	}
	if len(got) != 3 {
		t.Errorf("previous store has %d facts, want 3", len(got))
	}
	// No temp file left behind.
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Errorf("leftover temp file %s", e.Name())
		}
	}
}

func TestRebuild_MkdirAllError(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "file")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Parent "directory" is a regular file → MkdirAll fails.
	err := Rebuild(filepath.Join(blocker, "sub", "facts.db"), nil)
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestRebuild_RenameError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "facts.db")
	if err := os.MkdirAll(filepath.Join(path, "occupied"), 0o755); err != nil {
		t.Fatal(err) // path is a non-empty directory → rename fails
	}
	err := Rebuild(path, sampleFacts())
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error %q does not name the store path", err)
	}
}

func TestWriteStore_SchemaExecError(t *testing.T) {
	// The store path is an existing directory → sqlite cannot open it.
	err := writeStore(t.TempDir(), nil)
	if err == nil {
		t.Fatal("expected an error")
	}
}

func TestWriteStore_OpenError(t *testing.T) {
	old := sqlOpenFn
	sqlOpenFn = func(string, string) (*sql.DB, error) { return nil, errors.New("open boom") }
	t.Cleanup(func() { sqlOpenFn = old })
	if err := writeStore(filepath.Join(t.TempDir(), "f.db"), nil); err == nil {
		t.Fatal("expected an error")
	}
}

func TestWriteStore_BeginError(t *testing.T) {
	old := dbBeginFn
	dbBeginFn = func(*sql.DB) (*sql.Tx, error) { return nil, errors.New("begin boom") }
	t.Cleanup(func() { dbBeginFn = old })
	if err := writeStore(filepath.Join(t.TempDir(), "f.db"), nil); err == nil {
		t.Fatal("expected an error")
	}
}

func TestWriteStore_CommitError(t *testing.T) {
	old := txCommitFn
	txCommitFn = func(tx *sql.Tx) error { _ = tx.Rollback(); return errors.New("commit boom") }
	t.Cleanup(func() { txCommitFn = old })
	if err := writeStore(filepath.Join(t.TempDir(), "f.db"), nil); err == nil {
		t.Fatal("expected an error")
	}
}

// --- Query filters ---

func TestQuery_Filters(t *testing.T) {
	path := newStorePath(t)
	if err := Rebuild(path, sampleFacts()); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name string
		f    Filter
		want int
	}{
		{"predicate exact", Filter{Predicate: "imports"}, 2},
		{"subject exact", Filter{Subject: "repo#x"}, 1},
		{"subject exact miss", Filter{Subject: "pkg"}, 0},
		{"subject prefix", Filter{Subject: "pkg:*"}, 2},
		{"object exact", Filter{Object: "Approved"}, 1},
		{"object prefix", Filter{Object: "pkg:*"}, 2},
		{"class", Filter{Class: "declared"}, 1},
		{"adapter", Filter{Adapter: "codegraph"}, 2},
		{"combined", Filter{Predicate: "imports", Object: "pkg:c"}, 1},
		{"combined miss", Filter{Predicate: "imports", Class: "declared"}, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Query(path, tt.f)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != tt.want {
				t.Errorf("Query(%+v) returned %d facts, want %d", tt.f, len(got), tt.want)
			}
		})
	}
}

func TestQuery_PrefixEscapesLikeMetacharacters(t *testing.T) {
	path := newStorePath(t)
	facts := sampleFacts()
	facts[0].Subject = `odd%_\name`
	if err := Rebuild(path, facts); err != nil {
		t.Fatal(err)
	}
	got, err := Query(path, Filter{Subject: `odd%_\*`})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("escaped prefix matched %d facts, want 1", len(got))
	}
	// A literal % must not act as a wildcard.
	got, err = Query(path, Filter{Subject: `odd%*`})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("literal %% prefix matched %d facts, want 1", len(got))
	}
	got, err = Query(path, Filter{Subject: `oddX*`})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("non-matching prefix matched %d facts, want 0", len(got))
	}
}

// --- Query error paths ---

func TestQuery_MissingStoreExits2(t *testing.T) {
	path := newStorePath(t)
	_, err := Query(path, Filter{})
	exit2(t, err)
	if !strings.Contains(err.Error(), path) {
		t.Errorf("error %q does not name the store path", err)
	}
	if !strings.Contains(err.Error(), "studio index") {
		t.Errorf("error %q does not suggest `studio index`", err)
	}
}

func TestQuery_EmptyStoreExits2(t *testing.T) {
	path := newStorePath(t)
	if err := Rebuild(path, nil); err != nil {
		t.Fatal(err)
	}
	_, err := Query(path, Filter{})
	exit2(t, err)
	if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "studio index") {
		t.Errorf("error %q does not name the store path and suggest `studio index`", err)
	}
}

func TestQuery_NotAStoreExits2(t *testing.T) {
	path := filepath.Join(t.TempDir(), "facts.db")
	if err := os.WriteFile(path, []byte("this is not sqlite"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Query(path, Filter{})
	exit2(t, err)
	if !strings.Contains(err.Error(), "studio index") {
		t.Errorf("error %q does not suggest `studio index`", err)
	}
}

func TestQuery_OpenError(t *testing.T) {
	path := newStorePath(t)
	if err := Rebuild(path, sampleFacts()); err != nil {
		t.Fatal(err)
	}
	old := sqlOpenFn
	sqlOpenFn = func(string, string) (*sql.DB, error) { return nil, errors.New("open boom") }
	t.Cleanup(func() { sqlOpenFn = old })
	if _, err := Query(path, Filter{}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestQuery_SelectError(t *testing.T) {
	path := newStorePath(t)
	if err := Rebuild(path, sampleFacts()); err != nil {
		t.Fatal(err)
	}
	old := dbQueryFn
	dbQueryFn = func(*sql.DB, string, ...any) (*sql.Rows, error) { return nil, errors.New("query boom") }
	t.Cleanup(func() { dbQueryFn = old })
	if _, err := Query(path, Filter{}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestQuery_ScanError(t *testing.T) {
	path := newStorePath(t)
	if err := Rebuild(path, sampleFacts()); err != nil {
		t.Fatal(err)
	}
	old := rowsScanFn
	rowsScanFn = func(*sql.Rows, ...any) error { return errors.New("scan boom") }
	t.Cleanup(func() { rowsScanFn = old })
	if _, err := Query(path, Filter{}); err == nil {
		t.Fatal("expected an error")
	}
}

func TestQuery_RowsError(t *testing.T) {
	path := newStorePath(t)
	if err := Rebuild(path, sampleFacts()); err != nil {
		t.Fatal(err)
	}
	old := rowsErrFn
	rowsErrFn = func(*sql.Rows) error { return errors.New("rows boom") }
	t.Cleanup(func() { rowsErrFn = old })
	if _, err := Query(path, Filter{}); err == nil {
		t.Fatal("expected an error")
	}
}
