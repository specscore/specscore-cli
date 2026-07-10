package directives_test

import (
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/internal/rehearse/blocks"
	"github.com/specscore/specscore-cli/internal/rehearse/blocks/directives"
)

func intp(n int) *int { return &n }

func TestSplit_TrailingDirectivesExtracted(t *testing.T) {
	body := "SELECT * FROM t;\n\n-- assert-rows: 3\n-- capture: name = username\n"
	query, dirs := directives.Split(body)
	if strings.TrimSpace(query) != "SELECT * FROM t;" {
		t.Errorf("query = %q", query)
	}
	want := []directives.Directive{
		{Key: "assert-rows", Value: "3"},
		{Key: "capture", Value: "name = username"},
	}
	if len(dirs) != len(want) {
		t.Fatalf("dirs = %+v, want %+v", dirs, want)
	}
	for i := range want {
		if dirs[i] != want[i] {
			t.Errorf("dirs[%d] = %+v, want %+v", i, dirs[i], want[i])
		}
	}
}

func TestSplit_BlankLinesBetweenDirectivesAllowed(t *testing.T) {
	query, dirs := directives.Split("SELECT 1\n-- assert-rows: 1\n\n-- capture: a = b\n\n")
	if strings.TrimSpace(query) != "SELECT 1" {
		t.Errorf("query = %q", query)
	}
	if len(dirs) != 2 || dirs[0].Key != "assert-rows" || dirs[1].Key != "capture" {
		t.Errorf("dirs = %+v", dirs)
	}
}

func TestSplit_EarlierCommentsStayInQuery(t *testing.T) {
	body := "-- fixture setup\nSELECT * FROM t\n-- assert-rows: 2"
	query, dirs := directives.Split(body)
	if !strings.Contains(query, "-- fixture setup") {
		t.Errorf("leading comment stripped from query: %q", query)
	}
	if len(dirs) != 1 || dirs[0].Key != "assert-rows" {
		t.Errorf("dirs = %+v", dirs)
	}
}

func TestSplit_NonDirectiveTrailingCommentStopsScan(t *testing.T) {
	// A trailing plain comment does not match `-- key: value`, so it and
	// everything above it stays in the query text.
	body := "SELECT 1\n-- assert-rows: 1\n-- just a note"
	query, dirs := directives.Split(body)
	if len(dirs) != 0 {
		t.Errorf("dirs = %+v, want none", dirs)
	}
	if !strings.Contains(query, "-- assert-rows: 1") || !strings.Contains(query, "-- just a note") {
		t.Errorf("query = %q", query)
	}
}

func TestSplit_NoDirectives(t *testing.T) {
	query, dirs := directives.Split("SELECT 1")
	if query != "SELECT 1" || len(dirs) != 0 {
		t.Errorf("query = %q, dirs = %+v", query, dirs)
	}
}

func TestParseAsserts_AllDirectives(t *testing.T) {
	a, err := directives.ParseAsserts([]directives.Directive{
		{Key: "assert-rows", Value: "3"},
		{Key: "assert-row-json", Value: `{"id": 1, "name": "alice"}`},
		{Key: "capture", Value: "name = username"},
		{Key: "capture", Value: "uid=id"},
	})
	if err != nil {
		t.Fatalf("ParseAsserts: %v", err)
	}
	if a.Rows == nil || *a.Rows != 3 {
		t.Errorf("Rows = %v", a.Rows)
	}
	if a.RowJSON["name"] != "alice" {
		t.Errorf("RowJSON = %v", a.RowJSON)
	}
	want := []directives.Capture{{Name: "name", Column: "username"}, {Name: "uid", Column: "id"}}
	if len(a.Captures) != 2 || a.Captures[0] != want[0] || a.Captures[1] != want[1] {
		t.Errorf("Captures = %+v, want %+v", a.Captures, want)
	}
}

func TestParseAsserts_Errors(t *testing.T) {
	tests := []struct {
		name    string
		dir     directives.Directive
		wantErr string
	}{
		{"non-integer rows", directives.Directive{Key: "assert-rows", Value: "many"}, "not an integer"},
		{"invalid row json", directives.Directive{Key: "assert-row-json", Value: "[1]"}, "not a JSON object"},
		{"capture without =", directives.Directive{Key: "capture", Value: "name"}, "does not match"},
		{"capture empty column", directives.Directive{Key: "capture", Value: "name ="}, "does not match"},
		{"capture empty name", directives.Directive{Key: "capture", Value: "= col"}, "does not match"},
		{"unknown key", directives.Directive{Key: "assert-jsonpath", Value: "$.x == 1"}, "unknown directive"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := directives.ParseAsserts([]directives.Directive{tt.dir})
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("error = %v, want contains %q", err, tt.wantErr)
			}
		})
	}
}

func TestApply_RowCountPassAndFail(t *testing.T) {
	rows := []directives.Row{{"id": int64(1)}, {"id": int64(2)}}
	if _, err := directives.Apply(directives.Asserts{Rows: intp(2)}, rows); err != nil {
		t.Errorf("matching assert-rows failed: %v", err)
	}
	_, err := directives.Apply(directives.Asserts{Rows: intp(3)}, rows)
	if err == nil || !strings.Contains(err.Error(), "got 2 row(s), want 3") {
		t.Errorf("error = %v", err)
	}
}

func TestApply_RowJSONNormalizesDriverTypes(t *testing.T) {
	rows := []directives.Row{{"id": int64(1), "name": "alice", "score": 1.5}}
	asserts := directives.Asserts{RowJSON: map[string]any{"id": float64(1), "name": "alice", "score": 1.5}}
	if _, err := directives.Apply(asserts, rows); err != nil {
		t.Errorf("normalized row-json comparison failed: %v", err)
	}
}

func TestApply_RowJSONMismatch(t *testing.T) {
	rows := []directives.Row{{"name": "bob"}}
	_, err := directives.Apply(directives.Asserts{RowJSON: map[string]any{"name": "alice"}}, rows)
	if err == nil || !strings.Contains(err.Error(), "does not equal") {
		t.Errorf("error = %v", err)
	}
}

func TestApply_RowJSONNoRows(t *testing.T) {
	_, err := directives.Apply(directives.Asserts{RowJSON: map[string]any{"a": true}}, nil)
	if err == nil || !strings.Contains(err.Error(), "returned no rows") {
		t.Errorf("error = %v", err)
	}
}

func TestApply_RowJSONUnrepresentableRow(t *testing.T) {
	rows := []directives.Row{{"f": func() {}}}
	_, err := directives.Apply(directives.Asserts{RowJSON: map[string]any{}}, rows)
	if err == nil || !strings.Contains(err.Error(), "not JSON-representable") {
		t.Errorf("error = %v", err)
	}
}

func TestApply_CapturesFirstRowInOrder(t *testing.T) {
	rows := []directives.Row{{"id": int64(42), "username": "alice"}, {"id": int64(7), "username": "bob"}}
	asserts := directives.Asserts{Captures: []directives.Capture{
		{Name: "uid", Column: "id"},
		{Name: "name", Column: "username"},
	}}
	captures, err := directives.Apply(asserts, rows)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	want := []blocks.Capture{{Name: "uid", Value: "42"}, {Name: "name", Value: "alice"}}
	if len(captures) != 2 || captures[0] != want[0] || captures[1] != want[1] {
		t.Errorf("captures = %+v, want %+v", captures, want)
	}
}

func TestApply_CaptureErrors(t *testing.T) {
	asserts := directives.Asserts{Captures: []directives.Capture{{Name: "uid", Column: "id"}}}
	if _, err := directives.Apply(asserts, nil); err == nil || !strings.Contains(err.Error(), "returned no rows") {
		t.Errorf("no-rows error = %v", err)
	}
	_, err := directives.Apply(asserts, []directives.Row{{"other": 1}})
	if err == nil || !strings.Contains(err.Error(), `no column "id"`) {
		t.Errorf("missing-column error = %v", err)
	}
}
