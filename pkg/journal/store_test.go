package journal

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/dbschema"
	"github.com/dal-go/dalgo/ddl"
)

// Persistence and storage layout are the dalgo driver's concern, not
// SpecScore's. These tests exercise the journal Store's behavior through the
// dalgo abstraction only — they never inspect on-disk files or record format.

func TestStore_OpenIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	if _, err := Open(dir); err != nil {
		t.Fatalf("first Open: %v", err)
	}
	if _, err := Open(dir); err != nil {
		t.Fatalf("second Open (must be idempotent): %v", err)
	}
}

func TestStore_WritePersists_DuplicateKeyErrors(t *testing.T) {
	dir := t.TempDir()
	s, err := Open(dir)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	e := Event{
		Type: "idea.approved", Timestamp: ts(t), MachineID: "mbp",
		SourceRepo: "/x/specscore", Stream: "specscore",
		Data: map[string]any{"slug": "foo"},
	}
	// First write succeeds...
	if err := s.Write(context.Background(), e, "uuid1234"); err != nil {
		t.Fatalf("first write: %v", err)
	}
	// ...and the record is now persisted, so an identical-key write fails —
	// proving the first write hit the backing store (verified via the abstraction).
	if err := s.Write(context.Background(), e, "uuid1234"); err == nil {
		t.Fatal("expected duplicate-key error on identical second write")
	}
}

func TestStore_Open_MkdirError(t *testing.T) {
	orig := mkdirAllFn
	mkdirAllFn = func(string, os.FileMode) error { return errors.New("boom") }
	defer func() { mkdirAllFn = orig }()
	if _, err := Open(t.TempDir()); err == nil {
		t.Fatal("expected mkdir error")
	}
}

func TestStore_Open_NewDatabaseError(t *testing.T) {
	orig := newDatabaseFn
	newDatabaseFn = func(string) (dal.DB, error) { return nil, errors.New("boom") }
	defer func() { newDatabaseFn = orig }()
	if _, err := Open(t.TempDir()); err == nil {
		t.Fatal("expected new-database error")
	}
}

func TestStore_Open_CreateCollectionError(t *testing.T) {
	orig := createCollectionFn
	createCollectionFn = func(context.Context, dal.DB, dbschema.CollectionDef, ...ddl.Option) error {
		return errors.New("boom")
	}
	defer func() { createCollectionFn = orig }()
	if _, err := Open(t.TempDir()); err == nil {
		t.Fatal("expected create-collection error")
	}
}
