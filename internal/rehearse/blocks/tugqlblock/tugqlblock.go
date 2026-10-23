// Package tugqlblock implements the ```tugql rehearse step block: the block
// body is a TugQL query document (deserialized by dalgo's tugql package)
// executed against the SQLite store at `db=<path>` via the dalgo SQLite
// adapter (dalgo2sqlite), with the same trailing `-- assert-rows:` /
// `-- assert-row-json:` / `-- capture:` directives as the sql block
// (REQ: tugql-block). This makes a Studio fact store (facts.db) directly
// assertable by scenarios.
package tugqlblock

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/dalgo/tugql"
	"github.com/dal-go/dalgo2sqlite"

	"github.com/specscore/specscore-cli/internal/rehearse/blocks"
	"github.com/specscore/specscore-cli/internal/rehearse/blocks/directives"
)

// Test seams — package-level vars wrapping external functions.
var (
	osStatFn      = os.Stat
	newDatabaseFn = func(path string) (*dalgo2sqlite.Database, error) {
		return dalgo2sqlite.NewDatabase(path)
	}
	readAllFn = func(ctx context.Context, q dal.Query, qe dal.QueryExecutor) (recordset.Recordset, error) {
		return dal.ExecuteQueryAndReadAllToRecordset(ctx, q, qe)
	}
	getValueFn = func(col recordset.Column[any], row int) (any, error) { return col.GetValue(row) }
)

// Executor runs tugql step blocks.
type Executor struct{}

// New returns the tugql block executor.
func New() *Executor { return &Executor{} }

// Kind returns "tugql".
func (*Executor) Kind() string { return "tugql" }

// Run deserializes the TugQL document, executes it against the `db=` SQLite
// store through the dalgo SQLite adapter, and applies the block's
// assertion/capture directives to the result rows.
func (*Executor) Run(ctx blocks.StepCtx) blocks.StepResult {
	path := ctx.Params["db"]
	if path == "" {
		return fail("tugql block requires db=<sqlite-path> in its info string (e.g. ```tugql db=facts.db)")
	}
	if !filepath.IsAbs(path) {
		path = filepath.Join(ctx.WorkDir, path)
	}
	// Opening a missing file would silently create an empty database; a
	// missing store must be an actionable failure instead.
	if _, err := osStatFn(path); err != nil {
		return fail("sqlite store not found: %s", path)
	}

	doc, dirs := directives.Split(ctx.Body)
	asserts, err := directives.ParseAsserts(dirs)
	if err != nil {
		return fail("%v", err)
	}
	query, err := tugql.Deserialize([]byte(doc))
	if err != nil {
		return fail("invalid TugQL document: %v", err)
	}

	db, err := newDatabaseFn(path)
	if err != nil {
		return fail("opening sqlite store %s: %v", path, err)
	}
	defer func() { _ = db.Close() }()

	rs, err := readAllFn(context.Background(), query, db)
	if err != nil {
		return fail("executing TugQL query against %s: %v", path, err)
	}
	rows, err := recordsetRows(rs)
	if err != nil {
		return fail("%v", err)
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

// recordsetRows materializes a dalgo recordset into directive rows.
func recordsetRows(rs recordset.Recordset) ([]directives.Row, error) {
	cols := rs.Columns()
	var out []directives.Row
	for i := 0; i < rs.RowsCount(); i++ {
		row := directives.Row{}
		for _, col := range cols {
			value, err := getValueFn(col, i)
			if err != nil {
				return nil, fmt.Errorf("reading column %q of result row %d: %w", col.Name(), i, err)
			}
			row[col.Name()] = value
		}
		out = append(out, row)
	}
	return out, nil
}

// fail builds a failing StepResult with a formatted diagnostic.
func fail(format string, args ...any) blocks.StepResult {
	return blocks.StepResult{Status: blocks.StatusFail, Detail: fmt.Sprintf("tugql step failed: "+format, args...)}
}
