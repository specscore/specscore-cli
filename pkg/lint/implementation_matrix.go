package lint

import (
	"fmt"
	"path/filepath"
	"strings"
)

// matrixColumns are the required columns of a Capability's Implementation
// Matrix, in canonical order (capability-and-platform-implementations#
// req:matrix-section).
var matrixColumns = []string{"Platform", "Status", "Brief", "Link"}

// matrixStatusVocabulary is the closed set of parity levels a Status cell may
// take (capability-and-platform-implementations#req:matrix-status-vocabulary).
var matrixStatusVocabulary = map[string]bool{
	"Full": true, "Partial": true, "Planned": true, "Absent": true,
}

// implementationMatrixChecker validates the shape of a Capability's
// "## Implementation Matrix" table: required columns, the Status vocabulary,
// and single-line Brief cells. It performs no status rollup.
type implementationMatrixChecker struct{}

func newImplementationMatrixChecker() checker { return &implementationMatrixChecker{} }

func (c *implementationMatrixChecker) name() string     { return "implementation-matrix" }
func (c *implementationMatrixChecker) severity() string { return "error" }

func (c *implementationMatrixChecker) check(specRoot string) ([]Violation, error) {
	var violations []Violation
	walkErr := walkFeatureReadmes(specRoot, func(readmePath string, content []byte) {
		if !classifyFeatureRole(string(content)).isCapability {
			return
		}
		rel, _ := filepath.Rel(specRoot, readmePath)
		for _, msg := range checkImplementationMatrix(string(content)) {
			violations = append(violations, Violation{
				File:     rel,
				Line:     0,
				Severity: "error",
				Rule:     "implementation-matrix",
				Message:  msg,
			})
		}
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return violations, nil
}

// checkImplementationMatrix validates a Capability README's Implementation
// Matrix and returns a list of human-readable violation messages (empty when
// the matrix is well-formed).
func checkImplementationMatrix(content string) []string {
	rows := extractMatrixRows(content)
	if len(rows) == 0 {
		return []string{missingColumnsMessage(matrixColumns)}
	}

	header := rows[0]
	colIndex := make(map[string]int, len(header))
	for i, cell := range header {
		colIndex[strings.ToLower(cell)] = i
	}

	var msgs []string
	var missing []string
	for _, col := range matrixColumns {
		if _, ok := colIndex[strings.ToLower(col)]; !ok {
			missing = append(missing, col)
		}
	}
	if len(missing) > 0 {
		msgs = append(msgs, missingColumnsMessage(missing))
	}

	statusIdx, hasStatus := colIndex["status"]
	briefIdx, hasBrief := colIndex["brief"]
	for _, row := range matrixDataRows(rows) {
		if hasStatus && statusIdx < len(row) && !matrixStatusVocabulary[row[statusIdx]] {
			msgs = append(msgs, fmt.Sprintf(
				"Implementation Matrix Status %q is not one of Full, Partial, Planned, Absent (capability-and-platform-implementations#req:matrix-status-vocabulary)",
				row[statusIdx]))
		}
		if hasBrief && briefIdx < len(row) && strings.Contains(strings.ToLower(row[briefIdx]), "<br") {
			msgs = append(msgs,
				"Implementation Matrix Brief must be a single line; remove the embedded line break (capability-and-platform-implementations#req:matrix-index-only)")
		}
	}
	return msgs
}

func missingColumnsMessage(missing []string) string {
	return fmt.Sprintf(
		"Implementation Matrix is missing required column(s): %s (required: %s) (capability-and-platform-implementations#req:matrix-section)",
		strings.Join(missing, ", "), strings.Join(matrixColumns, ", "))
}

// extractMatrixRows returns the cell rows of the table immediately following
// the "## Implementation Matrix" heading. Leading blank lines between the
// heading and the table are skipped; the table ends at the first blank line,
// non-table line, or end of content. Returns nil when the heading is absent or
// no table follows it.
func extractMatrixRows(content string) [][]string {
	lines := strings.Split(content, "\n")
	start := -1
	for i, l := range lines {
		if strings.TrimSpace(l) == matrixHeading {
			start = i
			break
		}
	}
	if start < 0 {
		return nil
	}

	var rows [][]string
	for _, l := range lines[start+1:] {
		t := strings.TrimSpace(l)
		if t == "" {
			if len(rows) > 0 {
				break
			}
			continue
		}
		if !strings.HasPrefix(t, "|") {
			break
		}
		rows = append(rows, splitTableRow(t))
	}
	return rows
}

// matrixDataRows returns the data rows of a parsed matrix table, skipping the
// header row and the "| --- |" separator row.
func matrixDataRows(rows [][]string) [][]string {
	if len(rows) < 2 {
		return nil
	}
	return rows[2:]
}

// splitTableRow splits a Markdown table row into trimmed cell values, dropping
// the leading and trailing pipe delimiters.
func splitTableRow(line string) []string {
	trimmed := strings.Trim(strings.TrimSpace(line), "|")
	parts := strings.Split(trimmed, "|")
	cells := make([]string, len(parts))
	for i, p := range parts {
		cells[i] = strings.TrimSpace(p)
	}
	return cells
}
