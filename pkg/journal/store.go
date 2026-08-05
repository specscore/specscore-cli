package journal

import (
	"context"
	"fmt"
	"os"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/dbschema"
	"github.com/dal-go/dalgo/ddl"
	"github.com/dal-go/record"

	"github.com/ingitdb/dalgo2ingitdb"
	"github.com/ingitdb/ingitdb-go/ingitdb/validator"
)

// collectionID is the inGitDB collection that holds journal events.
const collectionID = "events"

// Injectable seams for tests.
var (
	mkdirAllFn    = os.MkdirAll
	newDatabaseFn = func(path string) (dal.DB, error) {
		return dalgo2ingitdb.NewDatabase(path, validator.NewCollectionsReader())
	}
	createCollectionFn = ddl.CreateCollection
)

// Store is the inGitDB-backed journal event store. The SpecScore CLI is the
// only writer of journal records (per the journal-and-summary Feature).
type Store struct {
	db dal.DB
}

// eventsCollectionDef declares the `events` collection schema.
func eventsCollectionDef() dbschema.CollectionDef {
	return dbschema.CollectionDef{
		Name: collectionID,
		Fields: []dbschema.FieldDef{
			{Name: "type", Type: dbschema.String},
			{Name: "timestamp", Type: dbschema.String},
			{Name: "machine_id", Type: dbschema.String},
			{Name: "source_repo", Type: dbschema.String},
			{Name: "stream", Type: dbschema.String},
		},
	}
}

// Open opens (creating if absent) the inGitDB journal at journalPath and ensures
// the events collection exists. Idempotent across runs.
func Open(journalPath string) (*Store, error) {
	if err := mkdirAllFn(journalPath, 0o755); err != nil {
		return nil, fmt.Errorf("creating journal dir: %w", err)
	}
	db, err := newDatabaseFn(journalPath)
	if err != nil {
		return nil, fmt.Errorf("opening journal db: %w", err)
	}
	if err := createCollectionFn(context.Background(), db, eventsCollectionDef(), ddl.IfNotExists()); err != nil {
		return nil, fmt.Errorf("creating events collection: %w", err)
	}
	return &Store{db: db}, nil
}

// Write persists one event as a single date-sharded record. shortUUID
// disambiguates same-instant writes from the same machine.
func (s *Store) Write(ctx context.Context, e Event, shortUUID string) error {
	key := record.NewKeyWithID(collectionID, eventKey(e.Timestamp, e.MachineID, shortUUID))
	return s.db.RunReadwriteTransaction(ctx, func(ctx context.Context, tx dal.ReadwriteTransaction) error {
		return tx.Insert(ctx, record.NewRecordWithData(key, e.toMap()))
	})
}
