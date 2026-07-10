// Package sqlblock implements the ```sql rehearse step block: the block's
// statement(s) run against the DSN from the info string (v0.3 driver:
// `sqlite:<path>` via the pure-Go driver already in go.mod), with trailing
// `-- assert-rows:` / `-- assert-row-json:` / `-- capture:` directives
// checked against the final statement's result rows (REQ: sql-block).
package sqlblock

import (
	"database/sql"
	"fmt"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite" // pure-Go sqlite driver (house precedent)

	"github.com/specscore/specscore-cli/internal/rehearse/blocks"
	"github.com/specscore/specscore-cli/internal/rehearse/blocks/directives"
)

// Test seams — package-level vars wrapping external functions.
var (
	sqlOpenFn     = sql.Open
	rowsColumnsFn = func(rows *sql.Rows) ([]string, error) { return rows.Columns() }
	rowsScanFn    = func(rows *sql.Rows, dest ...any) error { return rows.Scan(dest...) }
	rowsErrFn     = func(rows *sql.Rows) error { return rows.Err() }
	dbExecFn      = func(db *sql.DB, stmt string) error { _, err := db.Exec(stmt); return err }
	dbQueryFn     = func(db *sql.DB, stmt string) (*sql.Rows, error) { return db.Query(stmt) }
)

// Executor runs sql step blocks.
type Executor struct{}

// New returns the sql block executor.
func New() *Executor { return &Executor{} }

// Kind returns "sql".
func (*Executor) Kind() string { return "sql" }

// Run executes the block's statements against the `dsn=` DSN: every
// statement but the last is executed for effect (fixture setup), the last
// one's result rows feed the assertion/capture directives.
func (*Executor) Run(ctx blocks.StepCtx) blocks.StepResult {
	path, res := sqlitePath(ctx)
	if res != nil {
		return *res
	}

	query, dirs := directives.Split(ctx.Body)
	asserts, err := directives.ParseAsserts(dirs)
	if err != nil {
		return fail("%v", err)
	}
	statements := splitStatements(query)
	if len(statements) == 0 {
		return fail("sql block contains no statement")
	}

	db, err := sqlOpenFn("sqlite", path)
	if err != nil {
		return fail("opening sqlite database %s: %v", path, err)
	}
	defer func() { _ = db.Close() }()

	for _, stmt := range statements[:len(statements)-1] {
		if err := dbExecFn(db, stmt); err != nil {
			return fail("executing statement %q: %v", stmt, err)
		}
	}
	last := statements[len(statements)-1]
	rows, err := collectRows(db, last)
	if err != nil {
		return fail("executing statement %q: %v", last, err)
	}

	captures, err := directives.Apply(asserts, rows)
	if err != nil {
		return fail("%v", err)
	}
	return blocks.StepResult{
		Status:   blocks.StatusPass,
		Output:   fmt.Sprintf("%d row(s)", len(rows)),
		Captures: captures,
	}
}

// sqlitePath validates the `dsn=` info-string param and returns the sqlite
// file path, resolved against the scenario working dir when relative. The
// second return value is non-nil when the DSN is unusable.
func sqlitePath(ctx blocks.StepCtx) (string, *blocks.StepResult) {
	dsn := ctx.Params["dsn"]
	if dsn == "" {
		res := fail("sql block requires dsn=<driver:path> in its info string (e.g. ```sql dsn=sqlite:app.db)")
		return "", &res
	}
	path, ok := strings.CutPrefix(dsn, "sqlite:")
	if !ok || path == "" {
		res := fail("unsupported DSN %q: v0.3 supports sqlite:<path> only", dsn)
		return "", &res
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(ctx.WorkDir, path)
	}
	return path, nil
}

// splitStatements splits SQL text into statements on `;`. Semicolons inside
// string literals are not handled — a v0.3 simplification the sql-block
// fixture corpus stays clear of.
func splitStatements(query string) []string {
	var out []string
	for _, stmt := range strings.Split(query, ";") {
		if trimmed := strings.TrimSpace(stmt); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

// collectRows runs the final statement and materializes its result rows.
// Non-SELECT statements yield zero columns and zero rows (still executed).
func collectRows(db *sql.DB, stmt string) ([]directives.Row, error) {
	rows, err := dbQueryFn(db, stmt)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	cols, err := rowsColumnsFn(rows)
	if err != nil {
		return nil, fmt.Errorf("reading result columns: %w", err)
	}
	var out []directives.Row
	for rows.Next() {
		values := make([]any, len(cols))
		scan := make([]any, len(cols))
		for i := range values {
			scan[i] = &values[i]
		}
		if err := rowsScanFn(rows, scan...); err != nil {
			return nil, fmt.Errorf("reading result row: %w", err)
		}
		row := directives.Row{}
		for i, col := range cols {
			row[col] = normalizeValue(values[i])
		}
		out = append(out, row)
	}
	if err := rowsErrFn(rows); err != nil {
		return nil, fmt.Errorf("reading result rows: %w", err)
	}
	return out, nil
}

// normalizeValue maps driver-native byte slices to strings so captures and
// row-JSON comparisons see text, not base64.
func normalizeValue(v any) any {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	return v
}

// fail builds a failing StepResult with a formatted diagnostic.
func fail(format string, args ...any) blocks.StepResult {
	return blocks.StepResult{Status: blocks.StatusFail, Detail: fmt.Sprintf("sql step failed: "+format, args...)}
}
