// specscore:feature/cli/spec/lint
//
// Implements the `index-entries` rule and its --fix support. The bidirectional
// check (phantom links + orphan children) satisfies
// REQ:index-entries-bidirectional. The fixer satisfies
// REQ:index-entries-fix-deletes-phantom-rows (Phase 1) and
// REQ:index-entries-fix-inserts-orphan-rows (Phase 2). All three REQs and
// their ACs live under the "Features index synchronization" subsection of
// the cli/spec/lint feature README.
package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/specscore/specscore-cli/pkg/feature"
)

// osReadDir is the injectable ReadDir; tests may replace it to simulate errors.
var osReadDir = os.ReadDir

// indexEntriesChecker verifies that feature README indices match actual child directories.
type indexEntriesChecker struct{}

func newIndexEntriesChecker() checker {
	return &indexEntriesChecker{}
}

func (c *indexEntriesChecker) name() string     { return "index-entries" }
func (c *indexEntriesChecker) severity() string { return "error" }

func (c *indexEntriesChecker) check(specRoot string) ([]Violation, error) {
	var violations []Violation

	featureDir := filepath.Join(specRoot, "features")
	info, err := os.Stat(featureDir)
	if err != nil || !info.IsDir() {
		return violations, nil
	}

	err = filepath.Walk(featureDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return nil
		}

		readmePath := filepath.Join(path, "README.md")
		if _, statErr := os.Stat(readmePath); statErr != nil {
			return nil
		}

		// Get actual child directories (excluding hidden and _args convention dirs).
		entries, readErr := osReadDir(path)
		if readErr != nil {
			return nil
		}

		var actualChildren []string
		for _, entry := range entries {
			if entry.IsDir() && !strings.HasPrefix(entry.Name(), ".") &&
				!strings.HasPrefix(entry.Name(), "_") &&
				!isReservedFeatureChild(entry.Name()) {
				actualChildren = append(actualChildren, entry.Name())
			}
		}

		relPath, _ := filepath.Rel(specRoot, readmePath)
		mentioned, parseErr := extractChildRefsFromReadme(readmePath)
		if parseErr != nil {
			// A table whose identity column cannot be determined must not be
			// treated as evidence that arbitrary links in later cells list the
			// children. Report the unsafe schema when it matters to an actual
			// parent; support READMEs with unrelated tables and no child Features
			// remain outside this rule.
			if len(actualChildren) > 0 {
				violations = append(violations, Violation{
					File:     relPath,
					Line:     0,
					Severity: "error",
					Rule:     "index-entries",
					Message:  "Index child identity schema is invalid: " + parseErr.Error(),
				})
			}
			return nil
		}

		// Flag index entries that reference non-existent directories.
		actualSet := make(map[string]bool, len(actualChildren))
		for _, a := range actualChildren {
			actualSet[a] = true
		}
		mentionedCounts := make(map[string]int, len(mentioned))
		for _, m := range mentioned {
			mentionedCounts[m]++
			if mentionedCounts[m] > 1 {
				violations = append(violations, Violation{
					File:     relPath,
					Line:     0,
					Severity: "error",
					Rule:     "index-entries",
					Message:  "Index lists child directory more than once: " + m,
				})
			}
			if !actualSet[m] {
				violations = append(violations, Violation{
					File:     relPath,
					Line:     0,
					Severity: "error",
					Rule:     "index-entries",
					Message:  "Index mentions non-existent directory: " + m,
				})
			}
		}

		// Flag child directories that are not mentioned in the index.
		mentionedSet := make(map[string]bool, len(mentioned))
		for _, m := range mentioned {
			mentionedSet[m] = true
		}
		for _, a := range actualChildren {
			if !mentionedSet[a] {
				violations = append(violations, Violation{
					File:     relPath,
					Line:     0,
					Severity: "error",
					Rule:     "index-entries",
					Message:  "Child directory not listed in index: " + a,
				})
			}
		}

		return nil
	})

	return violations, err
}

// fix implements both index-entries autofix REQs:
//
//   - index-entries-fix-deletes-phantom-rows: rows whose link target points
//     at a non-existent child directory are deleted.
//   - index-entries-fix-inserts-orphan-rows: child directories that exist on
//     disk but are not linked from the parent index get a fresh row appended.
//     Status is parsed from the child README via feature.ParseFeatureStatus.
//     Kind and Description use the same placeholder convention that
//     `specscore feature new` already codifies (`—` and `TODO: Add
//     description.`) — both columns are hand-maintained in features-index
//     and have no per-feature source-of-truth in the child README.
//
// Phase 1 (delete) runs first so subsequent Phase 2 (insert) reads a
// phantom-free index. Both phases are idempotent: pass 2 finds no phantom
// rows to delete and no unlinked children to insert.
func (c *indexEntriesChecker) fix(specRoot string) error {
	featureDir := filepath.Join(specRoot, "features")
	info, err := os.Stat(featureDir)
	if err != nil || !info.IsDir() {
		return nil
	}

	return filepath.Walk(featureDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !info.IsDir() {
			return nil
		}

		readmePath := filepath.Join(path, "README.md")
		if _, statErr := os.Stat(readmePath); statErr != nil {
			return nil
		}

		// Collect actual child dirs (those with their own README — i.e., features).
		entries, readErr := osReadDir(path)
		if readErr != nil {
			return nil
		}
		var actualChildren []string
		actualSet := make(map[string]bool)
		for _, e := range entries {
			if !e.IsDir() || strings.HasPrefix(e.Name(), ".") || strings.HasPrefix(e.Name(), "_") ||
				isReservedFeatureChild(e.Name()) {
				continue
			}
			if _, err := os.Stat(filepath.Join(path, e.Name(), "README.md")); err != nil {
				continue
			}
			actualChildren = append(actualChildren, e.Name())
			actualSet[e.Name()] = true
		}

		// Parse the table schema before any mutation. A missing or ambiguous
		// identity column makes the whole fix write-free; otherwise Phase 1 and
		// Phase 2 are composed in memory and published by one write below.
		content, err := os.ReadFile(readmePath)
		if err != nil {
			return nil
		}
		lines := strings.Split(string(content), "\n")
		schema, hasTable, parseErr := childIndexSchema(lines)
		if parseErr != nil {
			return nil
		}

		// A README without a table retains the established scaffolding shape,
		// but all missing rows are composed and published once rather than
		// delegating to one write per child.
		if !hasTable {
			sort.Strings(actualChildren)
			if len(actualChildren) == 0 {
				return nil
			}
			lines = scaffoldChildIndex(lines, path == featureDir, path, actualChildren)
			if err := os.WriteFile(readmePath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
				return err
			}
			return nil
		}

		// Phase 1: drop phantom rows and repeated child identities from the
		// already-validated identity column. The first row for a real child is
		// preserved byte-for-byte.
		lines, schema, changed := dropPhantomIndexRowsWithSchema(lines, schema, actualSet)

		// Phase 2: append orphan rows in the existing table's exact schema.
		mentioned := childRefsFromLines(lines, schema)
		mentionedSet := make(map[string]bool, len(mentioned))
		for _, m := range mentioned {
			mentionedSet[m] = true
		}

		sort.Strings(actualChildren)
		for _, child := range actualChildren {
			if mentionedSet[child] {
				continue
			}
			status := childFeatureStatus(filepath.Join(path, child, "README.md"))
			row := renderChildIndexRow(schema, child, status, path)
			lines = insertStringAt(lines, schema.dataEnd, row)
			schema.dataEnd++
			mentionedSet[child] = true
			changed = true
		}
		if changed {
			if err := os.WriteFile(readmePath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
				return err
			}
		}
		return nil
	})
}

func childFeatureStatus(readmePath string) string {
	status, _ := feature.ParseFeatureStatus(readmePath)
	if status == "" || status == "Unknown" {
		return "Draft"
	}
	return status
}

func scaffoldChildIndex(lines []string, isRoot bool, parentDir string, children []string) []string {
	rows := make([]string, 0, len(children))
	for _, child := range children {
		status := childFeatureStatus(filepath.Join(parentDir, child, "README.md"))
		if isRoot {
			rows = append(rows, fmt.Sprintf("| [%s](%s/README.md) | %s | — | TODO: Add description. |", child, child, status))
		} else {
			rows = append(rows, fmt.Sprintf("| [%s](%s/README.md) | TODO: Add description. |", child, child))
		}
	}
	if isRoot {
		return append(append(lines, ""), rows...)
	}

	insertAt := 0
	for i, line := range lines {
		if strings.TrimSpace(line) != "## Summary" {
			continue
		}
		insertAt = i + 1
		for insertAt < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[insertAt]), "## ") {
			insertAt++
		}
		break
	}
	block := []string{"## Contents", "", "| Child | Description |", "|---|---|"}
	block = append(block, rows...)
	block = append(block, "")
	result := make([]string, 0, len(lines)+len(block))
	result = append(result, lines[:insertAt]...)
	result = append(result, block...)
	return append(result, lines[insertAt:]...)
}

// dropPhantomIndexRows returns content with every canonical child-index row
// removed whose identity-cell link targets a child absent from actualSet, and
// with every repeated real-child row after the first removed. Links in later
// content cells cannot cause a deletion. A table with a missing or ambiguous
// identity schema is left byte-for-byte untouched.
func dropPhantomIndexRows(content string, actualSet map[string]bool) (string, bool) {
	lines := strings.Split(content, "\n")
	schema, ok, err := childIndexSchema(lines)
	if err != nil || !ok {
		return content, false
	}
	lines, _, changed := dropPhantomIndexRowsWithSchema(lines, schema, actualSet)
	if !changed {
		return content, false
	}
	return strings.Join(lines, "\n"), true
}

func dropPhantomIndexRowsWithSchema(lines []string, schema childIndexTableSchema, actualSet map[string]bool) ([]string, childIndexTableSchema, bool) {
	out := make([]string, 0, len(lines))
	changed := false
	dropped := 0
	seen := make(map[string]bool)
	for i, line := range lines {
		if i >= schema.dataStart && i < schema.dataEnd {
			if dirname, ok := directChildRefFromIndexRow(line, schema.identityColumn); ok {
				if !actualSet[dirname] || seen[dirname] {
					changed = true
					dropped++
					continue
				}
				seen[dirname] = true
			}
		}
		out = append(out, line)
	}
	schema.dataEnd -= dropped
	return out, schema, changed
}

// phantomDirInTableRow retains the compatibility helper used by focused tests:
// a caller supplying an isolated row is asking about its first cell. Production
// table parsing calls phantomDirInTableRowAtColumn with the schema-derived
// identity column.
func phantomDirInTableRow(line string, actualSet map[string]bool) (string, bool) {
	return phantomDirInTableRowAtColumn(line, 0, actualSet)
}

func phantomDirInTableRowAtColumn(line string, identityColumn int, actualSet map[string]bool) (string, bool) {
	dirname, ok := directChildRefFromIndexRow(line, identityColumn)
	if !ok || actualSet[dirname] {
		return "", false
	}
	return dirname, true
}

// directChildRefFromIndexRow parses the canonical identity link from the
// schema-selected cell of one Markdown table row. It deliberately ignores
// every other cell: row-sync may copy link-rich Feature summaries into
// Description, and those links describe the indexed Feature rather than
// siblings in this index.
func directChildRefFromIndexRow(line string, identityColumn int) (string, bool) {
	cells, ok := splitMarkdownTableCells(strings.TrimSpace(line))
	if !ok || identityColumn < 0 || identityColumn >= len(cells) {
		return "", false
	}
	_, dirname, ok := parseFeatureIndexLink(cells[identityColumn])
	if !ok || strings.Contains(dirname, "/") || dirname == "." || dirname == ".." || strings.HasPrefix(dirname, "_") {
		return "", false
	}
	return dirname, true
}

type childIndexTableSchema struct {
	identityColumn int
	dataStart      int
	dataEnd        int
	headers        []string
}

// childIndexSchema locates the first contiguous Markdown table under
// ## Contents or ## Index (falling back to the document's first table for
// legacy files), then resolves exactly one artifact identity column from its
// header. Standard Feature/Child/Directory headers may appear in any column.
// A custom artifact label such as Mini-product is accepted only when every
// other column is recognized metadata, leaving one unambiguous candidate.
func childIndexSchema(lines []string) (childIndexTableSchema, bool, error) {
	sectionStart, sectionEnd := 0, len(lines)
	inCodeBlock := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			continue
		}
		if !inCodeBlock && (trimmed == "## Contents" || trimmed == "## Index") {
			sectionStart = i + 1
			for j := sectionStart; j < len(lines); j++ {
				if strings.HasPrefix(strings.TrimSpace(lines[j]), "## ") {
					sectionEnd = j
					break
				}
			}
			break
		}
	}

	headerLine := -1
	inCodeBlock = false
	for i := sectionStart; i+1 < sectionEnd; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "```") {
			inCodeBlock = !inCodeBlock
			continue
		}
		if inCodeBlock {
			continue
		}
		next := strings.TrimSpace(lines[i+1])
		if strings.HasPrefix(trimmed, "|") && strings.HasSuffix(trimmed, "|") &&
			isMarkdownTableSeparator(next) {
			headerLine = i
			break
		}
	}
	if headerLine < 0 {
		return childIndexTableSchema{}, false, nil
	}

	headers, _ := splitMarkdownTableCells(strings.TrimSpace(lines[headerLine]))
	identityColumn, err := childIdentityColumn(headers)
	if err != nil {
		return childIndexTableSchema{}, false, err
	}

	dataEnd := sectionEnd
	for i := headerLine + 2; i < sectionEnd; i++ {
		if !strings.HasPrefix(strings.TrimSpace(lines[i]), "|") {
			dataEnd = i
			break
		}
	}
	return childIndexTableSchema{
		identityColumn: identityColumn,
		dataStart:      headerLine + 2,
		dataEnd:        dataEnd,
		headers:        headers,
	}, true, nil
}

func childIdentityColumn(headers []string) (int, error) {
	var canonical []int
	for i, header := range headers {
		if isCanonicalChildIdentityHeader(normalizeIndexHeader(header)) {
			canonical = append(canonical, i)
		}
	}
	if len(canonical) == 1 {
		return canonical[0], nil
	}
	if len(canonical) > 1 {
		return -1, fmt.Errorf("multiple child identity columns")
	}

	var custom []int
	for i, header := range headers {
		normalized := normalizeIndexHeader(header)
		if normalized != "" && !isIndexMetadataHeader(normalized) {
			custom = append(custom, i)
		}
	}
	if len(custom) == 1 {
		return custom[0], nil
	}
	if len(custom) == 0 {
		return -1, fmt.Errorf("missing child identity column")
	}
	return -1, fmt.Errorf("ambiguous child identity columns")
}

func normalizeIndexHeader(header string) string {
	return strings.ToLower(strings.Trim(strings.TrimSpace(header), "*_`"))
}

func isCanonicalChildIdentityHeader(header string) bool {
	switch header {
	case "feature", "child", "directory", "dir", "artifact":
		return true
	default:
		return false
	}
}

func isIndexMetadataHeader(header string) bool {
	switch header {
	case "#", "no", "no.", "number",
		"status", "kind", "type", "stage",
		"description", "desc", "summary", "purpose", "notes",
		"domain", "captures", "owner", "title",
		"url", "consumer path", "index":
		return true
	default:
		return false
	}
}

func childRefsFromLines(lines []string, schema childIndexTableSchema) []string {
	var children []string
	for _, line := range lines[schema.dataStart:schema.dataEnd] {
		dirname, ok := directChildRefFromIndexRow(line, schema.identityColumn)
		if ok {
			children = append(children, dirname)
		}
	}
	if len(children) == 0 {
		return nil
	}
	return children
}

func renderChildIndexRow(schema childIndexTableSchema, child, status, parentDir string) string {
	cells := make([]string, len(schema.headers))
	for i, header := range schema.headers {
		if i == schema.identityColumn {
			cells[i] = fmt.Sprintf("[%s](%s/README.md)", child, child)
			continue
		}
		switch normalizeIndexHeader(header) {
		case "status":
			cells[i] = status
		case "description", "desc", "summary", "purpose":
			cells[i] = "TODO: Add description."
		case "kind":
			if strings.HasSuffix(child, "-index") {
				cells[i] = "Index"
			} else {
				cells[i] = "—"
			}
		case "url":
			cells[i] = childSpecURL(filepath.Join(parentDir, child, "README.md"))
			if cells[i] == "" {
				cells[i] = "—"
			}
		default:
			cells[i] = "—"
		}
	}
	return "| " + strings.Join(cells, " | ") + " |"
}

func childSpecURL(readmePath string) string {
	data, err := os.ReadFile(readmePath)
	if err != nil {
		return ""
	}
	const marker = "*This document follows the "
	idx := strings.LastIndex(string(data), marker)
	if idx < 0 {
		return ""
	}
	rest := string(data)[idx+len(marker):]
	end := strings.Index(rest, "*")
	if end < 0 {
		return ""
	}
	url := strings.TrimSpace(rest[:end])
	if !strings.HasPrefix(url, "http://") && !strings.HasPrefix(url, "https://") {
		return ""
	}
	return url
}

func insertStringAt(lines []string, at int, value string) []string {
	lines = append(lines, "")
	copy(lines[at+1:], lines[at:])
	lines[at] = value
	return lines
}

// extractChildRefsFromReadme reads direct-child links from the first Markdown
// table under ## Contents or ## Index. Legacy READMEs without either heading
// fall back to their first Markdown table. Links in prose or loose rows outside
// that table do not satisfy index completeness.
func extractChildRefsFromReadme(readmePath string) ([]string, error) {
	data, err := os.ReadFile(readmePath)
	if err != nil {
		return nil, err
	}

	lines := strings.Split(string(data), "\n")
	schema, ok, err := childIndexSchema(lines)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, nil
	}

	return childRefsFromLines(lines, schema), nil
}

func isMarkdownTableSeparator(line string) bool {
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "|") || !strings.HasSuffix(trimmed, "|") {
		return false
	}
	for _, cell := range strings.Split(strings.Trim(trimmed, "|"), "|") {
		cell = strings.TrimSpace(cell)
		if !strings.Contains(cell, "-") || strings.Trim(cell, "-:") != "" {
			return false
		}
	}
	return true
}
