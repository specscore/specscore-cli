package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/specscore/specscore-cli/pkg/feature"
)

// featureIndexChecker dispatches the feature-index-row-sync rule. The
// rule logic lives in `featureIndexRules` and is invoked twice — once
// in report mode from `check()`, once in mutation mode from `fix()` —
// matching the `checker` / `fixer` split used by every other rule in
// this package.
type featureIndexChecker struct {
	pending []Reconciliation
}

func newFeatureIndexChecker() *featureIndexChecker {
	return &featureIndexChecker{}
}

func (c *featureIndexChecker) name() string     { return "feature-index-row-sync" }
func (c *featureIndexChecker) severity() string { return "error" }

func (c *featureIndexChecker) check(specRoot string) ([]Violation, error) {
	vs, _, _ := featureIndexRules(specRoot, false)
	return vs, nil
}

// fix implements the fixer interface: rewrites drifted derived cells in
// the features-index to match each feature README. The check
// pass that follows reports zero violations because the rewrite is
// complete; idempotency is satisfied because the second pass finds no
// drift to rewrite. Every drifted cell is also recorded as a Reconciliation
// (see reconcile.go) so the rewrite is loud, not silent — the file wins
// mechanically, but the fact that it disagreed with the index is preserved.
func (c *featureIndexChecker) fix(specRoot string) error {
	_, _, rc := featureIndexRules(specRoot, true)
	c.pending = append(c.pending, rc...)
	return nil
}

func (c *featureIndexChecker) takeReconciliations() []Reconciliation {
	rc := c.pending
	c.pending = nil
	return rc
}

// featureIndexRules enforces:
//   - feature-index-row-sync: each top-level row's title, `Status`, and
//     `Description` (when that column exists) in
//     spec/features/README.md mirrors the corresponding feature's
//     `**Status:**` value at spec/features/<feature_id>/README.md.
//     Drift typically arises after a Status line is rewritten by hand
//     (or by a future `change-status` verb); the index row, being
//     derived state, must follow.
//
// Scope: top-level features only (entries whose slug contains a "/"
// are sub-features and are NOT listed in the features-index).
//
// What --fix does: rewrites the derived cells in the index row to match the
// feature README. Other schema-specific cells remain untouched. Every
// rewritten row is also returned as a Reconciliation (see reconcile.go) so
// the correction is loud rather than silent.
func featureIndexRules(specRoot string, fix bool) ([]Violation, bool, []Reconciliation) {
	var vs []Violation
	fixed := false

	featuresDir := filepath.Join(specRoot, "features")
	if info, err := os.Stat(featuresDir); err != nil || !info.IsDir() {
		return nil, false, nil
	}

	indexPath := filepath.Join(featuresDir, "README.md")
	if _, err := os.Stat(indexPath); err != nil {
		return nil, false, nil
	}

	rows, err := readFeatureIndexRows(indexPath)
	if err != nil {
		return nil, false, nil
	}

	type drift struct {
		slug           string
		actual, want   featureIndexValue
		hasDescription bool
		lineNum        int
	}
	var drifts []drift
	for _, r := range rows {
		// Top-level features only — sub-features (slug contains "/")
		// are not listed in the features-index.
		if strings.Contains(r.slug, "/") {
			continue
		}
		featureReadme := filepath.Join(featuresDir, r.slug, "README.md")
		if _, err := os.Stat(featureReadme); err != nil {
			// No matching feature directory — that is an
			// orphaned row, which is a different concern from
			// row-sync. Skip silently here; other rules
			// (or future feature-index-completeness) would
			// cover it.
			continue
		}
		status, err := feature.ParseFeatureStatus(featureReadme)
		if err != nil {
			continue
		}
		title, err := feature.ParseFeatureTitle(featureReadme)
		if err != nil {
			continue
		}
		summary, err := parseFeatureIndexSummaryFile(featureReadme)
		if err != nil {
			continue
		}
		// Older feature files may not have a Summary section. Do not invent an
		// empty description for them; once a summary exists it is canonical.
		summaryIsDerived := summary != ""
		if !summaryIsDerived {
			summary = r.summary
		}
		want := featureIndexValue{
			title:   escapeMarkdownTableCell(title),
			status:  escapeMarkdownTableCell(status),
			summary: summary,
		}
		if summaryIsDerived {
			want.summary = escapeMarkdownTableCell(summary)
		}
		if r.title != want.title || r.status != want.status || (r.hasDescription && r.summary != want.summary) {
			drifts = append(drifts, drift{
				slug: r.slug, actual: featureIndexValue{title: r.title, status: r.status, summary: r.summary}, want: want,
				hasDescription: r.hasDescription, lineNum: r.lineNum,
			})
		}
	}

	if len(drifts) == 0 {
		return nil, false, nil
	}

	rel, _ := filepath.Rel(specRoot, indexPath)

	if fix {
		updates := make(map[string]featureIndexValue, len(drifts))
		for _, d := range drifts {
			updates[d.slug] = d.want
		}
		if err := rewriteFeatureIndexRows(indexPath, updates); err == nil {
			sort.Slice(drifts, func(i, j int) bool { return drifts[i].slug < drifts[j].slug })
			reconciled := make([]Reconciliation, 0, len(drifts))
			for _, d := range drifts {
				var changes []FieldChange
				if d.actual.title != d.want.title {
					changes = append(changes, FieldChange{Field: "title", IndexValue: d.actual.title, FileValue: d.want.title})
				}
				if d.actual.status != d.want.status {
					changes = append(changes, FieldChange{Field: "status", IndexValue: d.actual.status, FileValue: d.want.status})
				}
				// Mirror the drift-detection guard exactly: rewriteFeatureIndexRows
				// only ever writes the Description cell when the table declares
				// one (schema.descriptionColumn >= 0, i.e. r.hasDescription).
				// Without that guard here, a row with no Description column but a
				// feature file that DOES have a parsed Summary would report a
				// "summary changed" reconciliation for a cell that was never
				// actually written to disk.
				if d.hasDescription && d.actual.summary != d.want.summary {
					changes = append(changes, FieldChange{Field: "summary", IndexValue: d.actual.summary, FileValue: d.want.summary})
				}
				reconciled = append(reconciled, Reconciliation{Rule: "feature-index-row-sync", Artifact: d.slug, Changes: changes})
			}
			return nil, true, reconciled
		}
		// Fall through to reporting if the rewrite failed.
		slugs := make([]string, 0, len(drifts))
		for _, d := range drifts {
			slugs = append(slugs, d.slug)
		}
		sort.Strings(slugs)
		vs = append(vs, Violation{
			File: rel, Line: 0, Severity: "error",
			Rule:    "feature-index-row-sync",
			Message: fmt.Sprintf("features-index rows drifted from feature READMEs: %s (fix failed)", strings.Join(slugs, ", ")),
		})
		return vs, false, nil
	}

	sort.Slice(drifts, func(i, j int) bool { return drifts[i].slug < drifts[j].slug })
	for _, d := range drifts {
		vs = append(vs, Violation{
			File: rel, Line: d.lineNum, Severity: "error",
			Rule:    "feature-index-row-sync",
			Message: fmt.Sprintf("features-index row for %q is stale (title/status/summary) (run `specscore spec lint --fix`)", d.slug),
		})
	}
	return vs, fixed, nil
}

// featureIndexRow captures one parsed top-level row of the features
type featureIndexRow struct {
	slug, title, status, summary string
	hasDescription               bool
	lineNum                      int
}

type featureIndexValue struct{ title, status, summary string }

// parseFeatureIndexSummaryFile is a narrow seam for exercising the defensive
// read-error path below. Production always uses parseFeatureIndexSummary.
var parseFeatureIndexSummaryFile = parseFeatureIndexSummary

type featureIndexTableSchema struct {
	cellCount         int
	descriptionColumn int // -1 when the table has no Description column
	dataStart         int
	dataEnd           int
}

// featureIndexSchema finds the Feature/Status table and its explicit
// Description column. The column is never inferred from its position: indices
// frequently end with URL, Index, or other hand-maintained fields.
func featureIndexSchema(lines []string) (featureIndexTableSchema, bool) {
	for i, line := range lines {
		cells, ok := splitMarkdownTableCells(strings.TrimSpace(line))
		if !ok || len(cells) < 2 || strings.TrimSpace(cells[0]) != "Feature" || strings.TrimSpace(cells[1]) != "Status" {
			continue
		}
		schema := featureIndexTableSchema{cellCount: len(cells), descriptionColumn: -1, dataStart: i + 1, dataEnd: len(lines)}
		for column, cell := range cells {
			if strings.TrimSpace(cell) == "Description" {
				schema.descriptionColumn = column
				break
			}
		}
		for j := schema.dataStart; j < len(lines); j++ {
			trimmed := strings.TrimSpace(lines[j])
			if trimmed == "" || strings.HasPrefix(trimmed, "#") || !strings.HasPrefix(trimmed, "|") {
				schema.dataEnd = j
				break
			}
		}
		return schema, true
	}
	return featureIndexTableSchema{}, false
}

// splitMarkdownTableCells keeps escaped pipes inside their cell. Rows with an
// unescaped pipe have a different number of cells than their header and are
// ignored by callers rather than being rewritten incorrectly.
func splitMarkdownTableCells(line string) ([]string, bool) {
	if !strings.HasPrefix(line, "|") || !strings.HasSuffix(line, "|") {
		return nil, false
	}
	var cells []string
	start := 1
	backslashes := 0
	for i := 1; i < len(line); i++ {
		if line[i] == '|' && backslashes%2 == 0 {
			cells = append(cells, line[start:i])
			start = i + 1
		}
		if line[i] == '\\' {
			backslashes++
		} else {
			backslashes = 0
		}
	}
	return cells, true
}

func parseFeatureIndexLink(cell string) (title, slug string, ok bool) {
	cell = strings.TrimSpace(cell)
	if !strings.HasPrefix(cell, "[") {
		return "", "", false
	}
	separator := strings.Index(cell, "](")
	if separator < 1 || !strings.HasSuffix(cell, "/README.md)") {
		return "", "", false
	}
	title = cell[1:separator]
	slug = strings.TrimSuffix(cell[separator+2:], "/README.md)")
	return title, slug, slug != ""
}

// escapeMarkdownTableCell renders a derived Markdown-table value without
// allowing a literal pipe to create an additional column.
func escapeMarkdownTableCell(value string) string {
	return strings.ReplaceAll(strings.TrimSpace(value), "|", "\\|")
}

// readFeatureIndexRows scans the features-index README and returns one
// featureIndexRow per row of the top-level table. Header and separator
// lines are skipped. Rows whose slug contains "/" (deeper links) are
// retained so the caller can filter; the caller is responsible for
// excluding sub-features from the row-sync check.
func readFeatureIndexRows(path string) ([]featureIndexRow, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(data), "\n")
	schema, ok := featureIndexSchema(lines)
	if !ok {
		return nil, nil
	}
	var rows []featureIndexRow
	for i := schema.dataStart; i < schema.dataEnd; i++ {
		cells, ok := splitMarkdownTableCells(strings.TrimSpace(lines[i]))
		if !ok || len(cells) != schema.cellCount {
			continue
		}
		title, slug, ok := parseFeatureIndexLink(cells[0])
		if !ok {
			continue
		}
		row := featureIndexRow{slug: slug, title: title, status: strings.TrimSpace(cells[1]), lineNum: i + 1}
		if schema.descriptionColumn >= 0 {
			row.hasDescription = true
			row.summary = strings.TrimSpace(cells[schema.descriptionColumn])
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func rewriteFeatureIndexRows(path string, updates map[string]featureIndexValue) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	schema, ok := featureIndexSchema(lines)
	if !ok {
		return nil
	}
	changed := false
	for i := schema.dataStart; i < schema.dataEnd; i++ {
		parts, ok := splitMarkdownTableCells(strings.TrimSpace(lines[i]))
		if !ok || len(parts) != schema.cellCount {
			continue
		}
		_, slug, ok := parseFeatureIndexLink(parts[0])
		if !ok {
			continue
		}
		update, ok := updates[slug]
		if !ok {
			continue
		}
		// Only derived Feature, Status, and explicit Description cells are
		// rewritten. Every other schema-specific cell round-trips byte-for-byte.
		parts[0] = " [" + update.title + "](" + slug + "/README.md) "
		parts[1] = " " + update.status + " "
		if schema.descriptionColumn >= 0 {
			parts[schema.descriptionColumn] = " " + update.summary + " "
		}
		lines[i] = "|" + strings.Join(parts, "|") + "|"
		changed = true
	}
	if !changed {
		return nil
	}
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

// rewriteFeatureIndexStatuses remains as a narrow compatibility helper for
// existing callers/tests. The row-sync fixer uses rewriteFeatureIndexRows so
// title and Description stay canonical too.
func rewriteFeatureIndexStatuses(path string, updates map[string]string) error {
	rows, err := readFeatureIndexRows(path)
	if err != nil {
		return err
	}
	values := make(map[string]featureIndexValue, len(updates))
	for _, row := range rows {
		if status, ok := updates[row.slug]; ok {
			values[row.slug] = featureIndexValue{title: row.title, status: status, summary: row.summary}
		}
	}
	return rewriteFeatureIndexRows(path, values)
}

func parseFeatureIndexSummary(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	lines := strings.Split(string(data), "\n")
	in := false
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "## Summary" {
			in = true
			continue
		}
		if in && strings.HasPrefix(trimmed, "## ") {
			break
		}
		if in && trimmed != "" {
			return trimmed, nil
		}
	}
	return "", nil
}
