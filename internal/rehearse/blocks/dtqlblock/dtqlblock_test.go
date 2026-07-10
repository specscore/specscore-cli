package dtqlblock

import (
	"context"
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/dal-go/dalgo/dal"
	"github.com/dal-go/dalgo/recordset"
	"github.com/dal-go/dalgo2sqlite"
	_ "modernc.org/sqlite"

	"github.com/specscore/specscore-cli/internal/rehearse/blocks"
)

// factStore creates a sqlite database shaped like a Studio fact store
// (table `facts`) holding three facts and returns its absolute path.
func factStore(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "facts.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`CREATE TABLE facts (subject TEXT NOT NULL, predicate TEXT NOT NULL, object TEXT NOT NULL);
		INSERT INTO facts VALUES ('a', 'imports', 'b'), ('b', 'imports', 'c'), ('a', 'declares', 'x');`); err != nil {
		t.Fatal(err)
	}
	return path
}

func run(t *testing.T, body, db string, workDir string) blocks.StepResult {
	t.Helper()
	if workDir == "" {
		workDir = t.TempDir()
	}
	return New().Run(blocks.StepCtx{WorkDir: workDir, Body: body, Params: map[string]string{"db": db}})
}

const allFacts = "from:\n  name: facts\n"

func TestKind(t *testing.T) {
	if got := New().Kind(); got != "dtql" {
		t.Fatalf("Kind() = %q, want dtql", got)
	}
}

// AC: dtql-counts-facts — a DTQL query over the fact store with
// `-- assert-rows:` equal to the known fact count passes.
func TestRun_AssertRowsPass(t *testing.T) {
	res := run(t, allFacts+"-- assert-rows: 3", factStore(t), "")
	if res.Status != blocks.StatusPass {
		t.Fatalf("status = %q, detail = %s", res.Status, res.Detail)
	}
	if res.Output != "3 row(s)" {
		t.Errorf("output = %q", res.Output)
	}
}

func TestRun_AssertRowsMismatchFails(t *testing.T) {
	res := run(t, allFacts+"-- assert-rows: 7", factStore(t), "")
	if res.Status != blocks.StatusFail || !strings.Contains(res.Detail, "got 3 row(s), want 7") {
		t.Fatalf("res = %+v", res)
	}
}

func TestRun_WhereClauseAndRowJSON(t *testing.T) {
	body := "from:\n  name: facts\nwhere:\n  op: ==\n  left:\n    field: predicate\n  right:\n    value: declares\n" +
		"-- assert-rows: 1\n" +
		`-- assert-row-json: {"subject": "a", "predicate": "declares", "object": "x"}`
	res := run(t, body, factStore(t), "")
	if res.Status != blocks.StatusPass {
		t.Fatalf("status = %q, detail = %s", res.Status, res.Detail)
	}
}

func TestRun_CaptureStoredOnStepResult(t *testing.T) {
	body := "from:\n  name: facts\nwhere:\n  op: ==\n  left:\n    field: predicate\n  right:\n    value: declares\n" +
		"-- capture: subj = subject"
	res := run(t, body, factStore(t), "")
	if res.Status != blocks.StatusPass {
		t.Fatalf("status = %q, detail = %s", res.Status, res.Detail)
	}
	if len(res.Captures) != 1 || res.Captures[0] != (blocks.Capture{Name: "subj", Value: "a"}) {
		t.Errorf("captures = %+v", res.Captures)
	}
}

func TestRun_RelativeDBPathResolvedAgainstWorkDir(t *testing.T) {
	path := factStore(t)
	res := New().Run(blocks.StepCtx{
		WorkDir: filepath.Dir(path),
		Body:    allFacts + "-- assert-rows: 3",
		Params:  map[string]string{"db": filepath.Base(path)},
	})
	if res.Status != blocks.StatusPass {
		t.Fatalf("status = %q, detail = %s", res.Status, res.Detail)
	}
}

func TestRun_MissingDBParamFails(t *testing.T) {
	res := New().Run(blocks.StepCtx{WorkDir: t.TempDir(), Body: allFacts, Params: map[string]string{}})
	if res.Status != blocks.StatusFail || !strings.Contains(res.Detail, "requires db=") {
		t.Fatalf("res = %+v", res)
	}
}

func TestRun_MissingStoreFileFails(t *testing.T) {
	res := run(t, allFacts, filepath.Join(t.TempDir(), "absent.db"), "")
	if res.Status != blocks.StatusFail || !strings.Contains(res.Detail, "sqlite store not found") {
		t.Fatalf("res = %+v", res)
	}
}

func TestRun_BadDirectiveFails(t *testing.T) {
	res := run(t, allFacts+"-- assert-rows: lots", factStore(t), "")
	if res.Status != blocks.StatusFail || !strings.Contains(res.Detail, "not an integer") {
		t.Fatalf("res = %+v", res)
	}
}

func TestRun_InvalidDTQLDocumentFails(t *testing.T) {
	res := run(t, "limit: 5\n-- assert-rows: 1", factStore(t), "")
	if res.Status != blocks.StatusFail || !strings.Contains(res.Detail, "invalid DTQL document") {
		t.Fatalf("res = %+v", res)
	}
}

func TestRun_OpenErrorFails(t *testing.T) {
	restore := newDatabaseFn
	newDatabaseFn = func(string) (*dalgo2sqlite.Database, error) { return nil, errors.New("adapter exploded") }
	t.Cleanup(func() { newDatabaseFn = restore })

	res := run(t, allFacts, factStore(t), "")
	if res.Status != blocks.StatusFail || !strings.Contains(res.Detail, "adapter exploded") {
		t.Fatalf("res = %+v", res)
	}
}

func TestRun_QueryErrorFails(t *testing.T) {
	// A DTQL document naming a table the store does not have fails at
	// execution time through the real adapter.
	res := run(t, "from:\n  name: no_such_table\n", factStore(t), "")
	if res.Status != blocks.StatusFail || !strings.Contains(res.Detail, "executing DTQL query") {
		t.Fatalf("res = %+v", res)
	}
}

func TestRun_ReadAllErrorFails(t *testing.T) {
	restore := readAllFn
	readAllFn = func(context.Context, dal.Query, dal.QueryExecutor) (recordset.Recordset, error) {
		return nil, errors.New("reader broke")
	}
	t.Cleanup(func() { readAllFn = restore })

	res := run(t, allFacts, factStore(t), "")
	if res.Status != blocks.StatusFail || !strings.Contains(res.Detail, "reader broke") {
		t.Fatalf("res = %+v", res)
	}
}

func TestRun_ColumnValueErrorFails(t *testing.T) {
	restore := getValueFn
	getValueFn = func(recordset.Column[any], int) (any, error) { return nil, errors.New("cell unreadable") }
	t.Cleanup(func() { getValueFn = restore })

	res := run(t, allFacts, factStore(t), "")
	if res.Status != blocks.StatusFail || !strings.Contains(res.Detail, "cell unreadable") {
		t.Fatalf("res = %+v", res)
	}
}
