// Package directives parses the trailing directive comments shared by the
// data-oriented rehearse blocks (`sql`, `dtql`): `-- assert-rows: <N>`,
// `-- assert-row-json: {...}` and `-- capture: <name> = <column>`
// (REQ: sql-block, REQ: dtql-block, REQ: context-bag), and applies the
// parsed assertions/captures to a query's result rows.
package directives

import (
	"encoding/json"
	"fmt"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/specscore/specscore-cli/internal/rehearse/blocks"
)

// Directive is one trailing `-- <key>: <value>` comment line.
type Directive struct {
	Key   string
	Value string
}

// directiveLine matches a directive comment: `-- <kebab-key>: <value>`.
var directiveLine = regexp.MustCompile(`^--\s*([a-z][a-z0-9-]*):\s*(.*)$`)

// Split separates a block body into its query text and the trailing
// directive comment lines, in document order. Only the trailing run of
// directive lines (blank lines allowed between them) is consumed; a `--`
// comment earlier in the body, or a trailing comment that does not match
// the `-- key: value` shape, stays part of the query text.
func Split(body string) (string, []Directive) {
	lines := strings.Split(body, "\n")
	var dirs []Directive
	end := len(lines)
	for i := len(lines) - 1; i >= 0; i-- {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			continue
		}
		m := directiveLine.FindStringSubmatch(trimmed)
		if m == nil {
			break
		}
		dirs = append([]Directive{{Key: m[1], Value: strings.TrimSpace(m[2])}}, dirs...)
		end = i
	}
	return strings.Join(lines[:end], "\n"), dirs
}

// Capture is one parsed `-- capture: <name> = <column>` directive.
type Capture struct {
	Name   string
	Column string
}

// Asserts holds the parsed assertion/capture directives of one sql/dtql
// block.
type Asserts struct {
	// Rows is the expected result row count (`-- assert-rows:`); nil when
	// not asserted.
	Rows *int
	// RowJSON is the expected first row as a JSON object
	// (`-- assert-row-json:`); nil when not asserted.
	RowJSON map[string]any
	// Captures are the `-- capture:` directives in document order.
	Captures []Capture
}

// ParseAsserts interprets the directives of an sql/dtql block. Unknown
// directive keys are an error so typos fail loudly instead of silently
// skipping an assertion.
func ParseAsserts(dirs []Directive) (Asserts, error) {
	var a Asserts
	for _, d := range dirs {
		switch d.Key {
		case "assert-rows":
			n, err := strconv.Atoi(d.Value)
			if err != nil {
				return Asserts{}, fmt.Errorf("-- assert-rows: %q is not an integer", d.Value)
			}
			a.Rows = &n
		case "assert-row-json":
			obj := map[string]any{}
			if err := json.Unmarshal([]byte(d.Value), &obj); err != nil {
				return Asserts{}, fmt.Errorf("-- assert-row-json: %q is not a JSON object: %v", d.Value, err)
			}
			a.RowJSON = obj
		case "capture":
			name, column, ok := strings.Cut(d.Value, "=")
			name, column = strings.TrimSpace(name), strings.TrimSpace(column)
			if !ok || name == "" || column == "" {
				return Asserts{}, fmt.Errorf("-- capture: %q does not match `<name> = <column>`", d.Value)
			}
			a.Captures = append(a.Captures, Capture{Name: name, Column: column})
		default:
			return Asserts{}, fmt.Errorf("unknown directive -- %s: (supported: assert-rows, assert-row-json, capture)", d.Key)
		}
	}
	return a, nil
}

// Row is one query result row keyed by column name.
type Row map[string]any

// Apply checks the assertions against the result rows and evaluates the
// capture directives against the first row, returning the captured values
// in directive order (REQ: sql-block, REQ: dtql-block, REQ: context-bag).
func Apply(a Asserts, rows []Row) ([]blocks.Capture, error) {
	if a.Rows != nil && len(rows) != *a.Rows {
		return nil, fmt.Errorf("assert-rows: got %d row(s), want %d", len(rows), *a.Rows)
	}
	if a.RowJSON != nil {
		if len(rows) == 0 {
			return nil, fmt.Errorf("assert-row-json: the query returned no rows")
		}
		got, err := normalizeRow(rows[0])
		if err != nil {
			return nil, err
		}
		if !reflect.DeepEqual(got, a.RowJSON) {
			return nil, fmt.Errorf("assert-row-json: first row %s does not equal %s", mustJSON(got), mustJSON(a.RowJSON))
		}
	}
	var captures []blocks.Capture
	for _, c := range a.Captures {
		if len(rows) == 0 {
			return nil, fmt.Errorf("capture %s: the query returned no rows", c.Name)
		}
		value, ok := rows[0][c.Column]
		if !ok {
			return nil, fmt.Errorf("capture %s: the first row has no column %q", c.Name, c.Column)
		}
		captures = append(captures, blocks.Capture{Name: c.Name, Value: fmt.Sprintf("%v", value)})
	}
	return captures, nil
}

// normalizeRow round-trips a result row through JSON so driver-native value
// types (int64, float64, []byte) compare equal to the JSON object literal of
// an `-- assert-row-json:` directive.
func normalizeRow(row Row) (map[string]any, error) {
	data, err := json.Marshal(row)
	if err != nil {
		return nil, fmt.Errorf("assert-row-json: first row is not JSON-representable: %v", err)
	}
	out := map[string]any{}
	// Unmarshal of a marshalled map[string]any into map[string]any cannot fail.
	_ = json.Unmarshal(data, &out)
	return out, nil
}

// mustJSON renders a value for assertion diagnostics; the input is always a
// plain JSON-shaped map, so marshalling cannot fail.
func mustJSON(v map[string]any) string {
	data, _ := json.Marshal(v)
	return string(data)
}
