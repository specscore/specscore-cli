package sqlblock

import (
	"database/sql"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	_ "modernc.org/sqlite"

	"github.com/specscore/specscore-cli/internal/rehearse/blocks"
)

// fixtureDB creates a sqlite database with a `t(id, username)` table holding
// three rows and returns its absolute path.
func fixtureDB(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "fixture.db")
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if _, err := db.Exec(`CREATE TABLE t (id INTEGER, username TEXT);
		INSERT INTO t VALUES (42, 'alice'), (7, 'bob'), (9, 'carol');`); err != nil {
		t.Fatal(err)
	}
	return path
}

func run(t *testing.T, body, dsn string, workDir string) blocks.StepResult {
	t.Helper()
	if workDir == "" {
		workDir = t.TempDir()
	}
	return New().Run(blocks.StepCtx{WorkDir: workDir, Body: body, Params: map[string]string{"dsn": dsn}})
}

func TestKind(t *testing.T) {
	if got := New().Kind(); got != "sql" {
		t.Fatalf("Kind() = %q, want sql", got)
	}
}

// AC: sql-assert-rows — a query over a 3-row fixture with `-- assert-rows: 3`
// passes.
func TestRun_AssertRowsPass(t *testing.T) {
	res := run(t, "SELECT * FROM t;\n-- assert-rows: 3", "sqlite:"+fixtureDB(t), "")
	if res.Status != blocks.StatusPass {
		t.Fatalf("status = %q, detail = %s", res.Status, res.Detail)
	}
	if res.Output != "3 row(s)" {
		t.Errorf("output = %q", res.Output)
	}
}

func TestRun_AssertRowsMismatchFails(t *testing.T) {
	res := run(t, "SELECT * FROM t;\n-- assert-rows: 5", "sqlite:"+fixtureDB(t), "")
	if res.Status != blocks.StatusFail || !strings.Contains(res.Detail, "got 3 row(s), want 5") {
		t.Fatalf("res = %+v", res)
	}
}

func TestRun_AssertRowJSONPass(t *testing.T) {
	body := "SELECT id, username FROM t ORDER BY id LIMIT 1;\n" +
		`-- assert-row-json: {"id": 7, "username": "bob"}`
	res := run(t, body, "sqlite:"+fixtureDB(t), "")
	if res.Status != blocks.StatusPass {
		t.Fatalf("status = %q, detail = %s", res.Status, res.Detail)
	}
}

func TestRun_AssertRowJSONMismatchFails(t *testing.T) {
	body := "SELECT id, username FROM t ORDER BY id LIMIT 1;\n" +
		`-- assert-row-json: {"id": 7, "username": "alice"}`
	res := run(t, body, "sqlite:"+fixtureDB(t), "")
	if res.Status != blocks.StatusFail || !strings.Contains(res.Detail, "does not equal") {
		t.Fatalf("res = %+v", res)
	}
}

func TestRun_CaptureStoredOnStepResult(t *testing.T) {
	body := "SELECT id, username FROM t WHERE id = 42;\n-- assert-rows: 1\n-- capture: name = username\n-- capture: uid = id"
	res := run(t, body, "sqlite:"+fixtureDB(t), "")
	if res.Status != blocks.StatusPass {
		t.Fatalf("status = %q, detail = %s", res.Status, res.Detail)
	}
	want := []blocks.Capture{{Name: "name", Value: "alice"}, {Name: "uid", Value: "42"}}
	if len(res.Captures) != 2 || res.Captures[0] != want[0] || res.Captures[1] != want[1] {
		t.Errorf("captures = %+v, want %+v", res.Captures, want)
	}
}

func TestRun_MultiStatementFixtureSetup(t *testing.T) {
	// The block itself creates the fixture: every statement but the last is
	// executed for effect, the last one feeds the assertions.
	workDir := t.TempDir()
	body := "CREATE TABLE t (id INTEGER);\nINSERT INTO t VALUES (1), (2), (3);\nSELECT * FROM t;\n-- assert-rows: 3"
	res := run(t, body, "sqlite:local.db", workDir)
	if res.Status != blocks.StatusPass {
		t.Fatalf("status = %q, detail = %s", res.Status, res.Detail)
	}
	// The relative DSN path resolved against the scenario working dir.
	if _, err := sql.Open("sqlite", filepath.Join(workDir, "local.db")); err != nil {
		t.Errorf("relative DSN not resolved against WorkDir: %v", err)
	}
}

func TestRun_MissingDSNFails(t *testing.T) {
	res := New().Run(blocks.StepCtx{WorkDir: t.TempDir(), Body: "SELECT 1", Params: map[string]string{}})
	if res.Status != blocks.StatusFail || !strings.Contains(res.Detail, "requires dsn=") {
		t.Fatalf("res = %+v", res)
	}
}

func TestRun_UnsupportedDSNFails(t *testing.T) {
	for _, dsn := range []string{"postgres:whatever", "sqlite:"} {
		res := run(t, "SELECT 1", dsn, "")
		if res.Status != blocks.StatusFail || !strings.Contains(res.Detail, "unsupported DSN") {
			t.Fatalf("dsn %q: res = %+v", dsn, res)
		}
	}
}

func TestRun_BadDirectiveFails(t *testing.T) {
	res := run(t, "SELECT 1;\n-- assert-rows: lots", "sqlite:"+fixtureDB(t), "")
	if res.Status != blocks.StatusFail || !strings.Contains(res.Detail, "not an integer") {
		t.Fatalf("res = %+v", res)
	}
}

func TestRun_EmptyBlockFails(t *testing.T) {
	res := run(t, "\n-- assert-rows: 0", "sqlite:"+fixtureDB(t), "")
	if res.Status != blocks.StatusFail || !strings.Contains(res.Detail, "no statement") {
		t.Fatalf("res = %+v", res)
	}
}

func TestRun_BadSQLFails(t *testing.T) {
	res := run(t, "SELECT * FROM missing_table;", "sqlite:"+fixtureDB(t), "")
	if res.Status != blocks.StatusFail || !strings.Contains(res.Detail, "missing_table") {
		t.Fatalf("res = %+v", res)
	}
}

func TestRun_BadSetupStatementFails(t *testing.T) {
	res := run(t, "INSERT INTO missing_table VALUES (1);\nSELECT 1;", "sqlite:"+fixtureDB(t), "")
	if res.Status != blocks.StatusFail || !strings.Contains(res.Detail, "executing statement") {
		t.Fatalf("res = %+v", res)
	}
}

func TestRun_OpenErrorFails(t *testing.T) {
	restore := sqlOpenFn
	sqlOpenFn = func(string, string) (*sql.DB, error) { return nil, errors.New("driver exploded") }
	t.Cleanup(func() { sqlOpenFn = restore })

	res := run(t, "SELECT 1", "sqlite:"+fixtureDB(t), "")
	if res.Status != blocks.StatusFail || !strings.Contains(res.Detail, "driver exploded") {
		t.Fatalf("res = %+v", res)
	}
}

func TestRun_ColumnsErrorFails(t *testing.T) {
	restore := rowsColumnsFn
	rowsColumnsFn = func(*sql.Rows) ([]string, error) { return nil, errors.New("columns gone") }
	t.Cleanup(func() { rowsColumnsFn = restore })

	res := run(t, "SELECT 1", "sqlite:"+fixtureDB(t), "")
	if res.Status != blocks.StatusFail || !strings.Contains(res.Detail, "columns gone") {
		t.Fatalf("res = %+v", res)
	}
}

func TestRun_ScanErrorFails(t *testing.T) {
	restore := rowsScanFn
	rowsScanFn = func(*sql.Rows, ...any) error { return errors.New("scan broke") }
	t.Cleanup(func() { rowsScanFn = restore })

	res := run(t, "SELECT * FROM t", "sqlite:"+fixtureDB(t), "")
	if res.Status != blocks.StatusFail || !strings.Contains(res.Detail, "scan broke") {
		t.Fatalf("res = %+v", res)
	}
}

func TestRun_RowsErrFails(t *testing.T) {
	restore := rowsErrFn
	rowsErrFn = func(*sql.Rows) error { return errors.New("iteration broke") }
	t.Cleanup(func() { rowsErrFn = restore })

	res := run(t, "SELECT * FROM t", "sqlite:"+fixtureDB(t), "")
	if res.Status != blocks.StatusFail || !strings.Contains(res.Detail, "iteration broke") {
		t.Fatalf("res = %+v", res)
	}
}

func TestRun_BlobValuesNormalizedToString(t *testing.T) {
	res := run(t, "SELECT CAST('hello' AS BLOB) AS b;\n-- capture: v = b", "sqlite:"+fixtureDB(t), "")
	if res.Status != blocks.StatusPass {
		t.Fatalf("status = %q, detail = %s", res.Status, res.Detail)
	}
	if len(res.Captures) != 1 || res.Captures[0] != (blocks.Capture{Name: "v", Value: "hello"}) {
		t.Errorf("captures = %+v", res.Captures)
	}
}

func TestRun_AbsoluteDSNPathUsedVerbatim(t *testing.T) {
	path := fixtureDB(t)
	res := run(t, "SELECT username FROM t WHERE id = 42;\n-- assert-row-json: {\"username\": \"alice\"}", "sqlite:"+path, t.TempDir())
	if res.Status != blocks.StatusPass {
		t.Fatalf("status = %q, detail = %s", res.Status, res.Detail)
	}
}
