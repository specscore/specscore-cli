// Package store persists SpecScore Studio facts in a single-file SQLite
// database (pure-Go driver, house precedent) and answers the filter queries
// behind `specscore studio facts`.
//
// Feature: cli/studio/index (REQ: rebuild-only, REQ: fact-shape,
// REQ: facts-query)
//
// The store is a disposable cache: Rebuild replaces the whole file via an
// atomic temp-file swap, so a failed rebuild leaves the previous store
// intact and facts from previous runs never survive unless re-derived.
package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite" // pure-Go sqlite driver

	"github.com/specscore/specscore-cli/internal/studio/fact"
	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// Test seams — package-level vars wrapping external functions.
// Production code calls these vars; tests replace them via t.Cleanup.
var (
	sqlOpenFn    = sql.Open
	osMkdirAllFn = os.MkdirAll
	osRenameFn   = os.Rename
	dbBeginFn    = func(db *sql.DB) (*sql.Tx, error) { return db.Begin() }
	txCommitFn   = func(tx *sql.Tx) error { return tx.Commit() }
	dbQueryFn    = func(db *sql.DB, query string, args ...any) (*sql.Rows, error) {
		return db.Query(query, args...)
	}
	rowsScanFn = func(rows *sql.Rows, dest ...any) error { return rows.Scan(dest...) }
	rowsErrFn  = func(rows *sql.Rows) error { return rows.Err() }
)

const schemaSQL = `
CREATE TABLE facts (
	subject          TEXT NOT NULL,
	predicate        TEXT NOT NULL,
	object           TEXT NOT NULL,
	evidence_class   TEXT NOT NULL CHECK (evidence_class IN ('declared', 'derived', 'verified-behavior')),
	evidence_pointer TEXT NOT NULL,
	adapter_id       TEXT NOT NULL,
	adapter_version  TEXT NOT NULL,
	observed_at      TEXT NOT NULL,
	ecosystem        TEXT NOT NULL
);
CREATE INDEX facts_subject ON facts (subject);
CREATE INDEX facts_predicate ON facts (predicate);
CREATE INDEX facts_object ON facts (object);
`

const insertSQL = `INSERT INTO facts
	(subject, predicate, object, evidence_class, evidence_pointer,
	 adapter_id, adapter_version, observed_at, ecosystem)
	VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`

const selectSQL = `SELECT subject, predicate, object, evidence_class,
	evidence_pointer, adapter_id, adapter_version, observed_at, ecosystem
	FROM facts`

// Rebuild replaces the fact store at path with one containing exactly the
// given facts. The new store is written to a temp file in the same directory
// and swapped in atomically on success; on any failure the previous store is
// left intact and the temp file is removed.
func Rebuild(path string, facts []fact.Fact) error {
	if err := osMkdirAllFn(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("creating fact-store directory for %s: %w", path, err)
	}
	tmp := path + ".rebuild.tmp"
	_ = os.Remove(tmp)
	if err := writeStore(tmp, facts); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := osRenameFn(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("swapping rebuilt fact store into %s: %w", path, err)
	}
	return nil
}

// writeStore creates a fresh SQLite fact store at path (which must not
// exist) holding exactly the given facts, inserted in one transaction.
func writeStore(path string, facts []fact.Fact) error {
	db, err := sqlOpenFn("sqlite", path)
	if err != nil {
		return fmt.Errorf("opening fact store %s: %w", path, err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(schemaSQL); err != nil {
		return fmt.Errorf("creating fact-store schema in %s: %w", path, err)
	}
	tx, err := dbBeginFn(db)
	if err != nil {
		return fmt.Errorf("starting fact-store transaction in %s: %w", path, err)
	}
	for _, f := range facts {
		if _, err := tx.Exec(insertSQL, f.Subject, f.Predicate, f.Object,
			string(f.Class), f.Pointer, f.Adapter.ID, f.Adapter.Version,
			f.ObservedAt, f.Ecosystem); err != nil {
			_ = tx.Rollback()
			return fmt.Errorf("writing fact to store %s: %w", path, err)
		}
	}
	if err := txCommitFn(tx); err != nil {
		return fmt.Errorf("committing fact store %s: %w", path, err)
	}
	return nil
}

// Filter selects facts by exact field match; empty fields match everything.
// Subject and Object also accept a trailing `*` for prefix matching.
type Filter struct {
	Subject   string
	Predicate string
	Object    string
	Class     string
	Adapter   string
}

// Query returns the facts at path matching the filter, in deterministic
// (subject, predicate, object) order. A missing, unreadable, or empty store
// returns an exit-2 error naming the store path and suggesting
// `specscore studio index`.
func Query(path string, f Filter) ([]fact.Fact, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, exitcode.InvalidArgsErrorf("fact store not found: %s — run `specscore studio index` to build it", path)
	}
	db, err := sqlOpenFn("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening fact store %s: %w", path, err)
	}
	defer func() { _ = db.Close() }()

	var total int
	if err := db.QueryRow("SELECT COUNT(*) FROM facts").Scan(&total); err != nil {
		return nil, exitcode.InvalidArgsErrorf("%s is not a fact store — rebuild it with `specscore studio index`", path)
	}
	if total == 0 {
		return nil, exitcode.InvalidArgsErrorf("fact store is empty: %s — run `specscore studio index` to build it", path)
	}

	where, args := f.whereClause()
	rows, err := dbQueryFn(db, selectSQL+where+" ORDER BY subject, predicate, object", args...)
	if err != nil {
		return nil, fmt.Errorf("querying fact store %s: %w", path, err)
	}
	defer func() { _ = rows.Close() }()

	var out []fact.Fact
	for rows.Next() {
		var ft fact.Fact
		var class string
		if err := rowsScanFn(rows, &ft.Subject, &ft.Predicate, &ft.Object,
			&class, &ft.Pointer, &ft.Adapter.ID, &ft.Adapter.Version,
			&ft.ObservedAt, &ft.Ecosystem); err != nil {
			return nil, fmt.Errorf("reading fact from store %s: %w", path, err)
		}
		ft.Class = fact.Class(class)
		out = append(out, ft)
	}
	if err := rowsErrFn(rows); err != nil {
		return nil, fmt.Errorf("reading facts from store %s: %w", path, err)
	}
	return out, nil
}

// whereClause renders the filter as a SQL WHERE clause plus bind args.
func (f Filter) whereClause() (string, []any) {
	var conds []string
	var args []any
	addPrefixable := func(column, value string) {
		if value == "" {
			return
		}
		if prefix, ok := strings.CutSuffix(value, "*"); ok {
			conds = append(conds, column+` LIKE ? ESCAPE '\'`)
			args = append(args, likeEscape(prefix)+"%")
			return
		}
		conds = append(conds, column+" = ?")
		args = append(args, value)
	}
	addExact := func(column, value string) {
		if value == "" {
			return
		}
		conds = append(conds, column+" = ?")
		args = append(args, value)
	}
	addPrefixable("subject", f.Subject)
	addExact("predicate", f.Predicate)
	addPrefixable("object", f.Object)
	addExact("evidence_class", f.Class)
	addExact("adapter_id", f.Adapter)
	if len(conds) == 0 {
		return "", nil
	}
	return " WHERE " + strings.Join(conds, " AND "), args
}

// likeEscape escapes SQL LIKE metacharacters in a literal prefix, using `\`
// as the escape character.
func likeEscape(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `%`, `\%`, `_`, `\_`)
	return r.Replace(s)
}
