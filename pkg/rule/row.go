package rule

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// The rules index is both the index and the primary artifact. An inline rule
// lives in exactly one row and nowhere else; a detailed rule keeps the same row
// and links it to spec/rules/<slug>/README.md. Everything in this file treats
// the row as the source of truth.

// IndexHeading is the H2 the rules table lives under.
const IndexHeading = "## Rules"

// IndexHeaderRow and IndexSeparatorRow are the exact canonical table header
// lines, compared byte-for-byte so a hand-edited index that dropped a column is
// reported rather than silently reinterpreted.
const (
	IndexHeaderRow    = "| Rule | Status | Scope | Enforcement | Control | Sources | Statement |"
	IndexSeparatorRow = "|---|---|---|---|---|---|---|"
)

// IndexEmptyPlaceholder is written in place of the table body when a project
// has no Rules yet.
const IndexEmptyPlaceholder = "_No rules recorded yet._"

// indexColumns is the cell count the canonical row shape carries.
const indexColumns = 7

// Row is one rule as the index records it — the whole of an inline rule, and
// the authoritative half of a detailed one.
type Row struct {
	Slug string
	// Linked is true when the identity cell is a Markdown link, which is the
	// index's own statement that spec/rules/<slug>/README.md exists.
	Linked      bool
	Status      string
	Scope       string // raw, comma-separated
	Enforcement string
	Control     string
	Sources     string // raw, comma-separated
	Statement   string
	// Line is the 1-based source line of the row, for violation reporting.
	Line int
}

// ScopeList parses the row's scope cell.
func (r Row) ScopeList() []string { return splitList(unescapeCell(r.Scope)) }

// SourceList parses the row's sources cell.
func (r Row) SourceList() []string { return splitList(unescapeCell(r.Sources)) }

// HasControl reports whether the row names a real control.
func (r Row) HasControl() bool { return isRealValue(unescapeCell(r.Control)) }

// Detailed reports whether this rule is expected to have a detail document.
func (r Row) Detailed() bool { return r.Linked }

// DetailLink is the identity cell's link target for a detailed rule.
func (r Row) DetailLink() string { return r.Slug + "/README.md" }

// identityCell renders the first cell: a bare slug for an inline rule, a link
// for a detailed one.
func (r Row) identityCell() string {
	if r.Linked {
		return fmt.Sprintf("[%s](%s)", r.Slug, r.DetailLink())
	}
	return r.Slug
}

// Render renders the row as one Markdown table line.
func (r Row) Render() string {
	return fmt.Sprintf("| %s | %s | %s | %s | %s | %s | %s |",
		r.identityCell(), sentinelOr(r.Status), sentinelOr(r.Scope), sentinelOr(r.Enforcement),
		sentinelOr(r.Control), sentinelOr(r.Sources), sentinelOr(r.Statement))
}

// Equals compares two rows on every field but their source line.
func (r Row) Equals(o Row) bool {
	r.Line, o.Line = 0, 0
	return r == o
}

// escapeCell makes a free-text value safe inside a Markdown table cell. The
// value has already had its newlines collapsed by the caller, so a hand-wrapped
// statement reaches the row whole rather than truncated at its first physical
// line — the exact failure that made feature and lesson index rows unreadable
// before the paragraph-joining fixes.
func escapeCell(value string) string {
	v := collapseWhitespace(value)
	if v == "" {
		return Sentinel
	}
	return strings.ReplaceAll(v, "|", `\|`)
}

// unescapeCell reverses escapeCell, so a cell can be compared against the
// unescaped value a detail document carries.
func unescapeCell(value string) string { return strings.ReplaceAll(value, `\|`, "|") }

// NewRow builds a canonical row from already-normalized values.
func NewRow(slug string, linked bool, status, statement string, scopes []string, enforcement, control string, sources []string) Row {
	return Row{
		Slug: slug, Linked: linked,
		Status:      sentinelOr(status),
		Scope:       escapeCell(joinOrSentinel(scopes)),
		Enforcement: sentinelOr(enforcement),
		Control:     escapeCell(control),
		Sources:     escapeCell(joinOrSentinel(sources)),
		Statement:   escapeCell(statement),
	}
}

// RowFromDetail projects a detail document back into its canonical row. It is
// used only to repair an index that lost a row for an existing document; the
// row remains authoritative everywhere else.
func RowFromDetail(d *Detail) Row {
	return NewRow(d.Slug, true, d.Status, d.Statement, d.ScopesRaw, d.Enforcement, d.Control, d.SourcesRaw)
}

// ----- index file I/O -----

// RulesDir returns the rules directory for a project root (the directory that
// contains spec/, not spec/ itself).
func RulesDir(projectRoot string) string { return filepath.Join(projectRoot, "spec", "rules") }

// IndexPath returns the rules index path.
func IndexPath(rulesDir string) string { return filepath.Join(rulesDir, "README.md") }

// DetailPath returns the detail-document path for a slug.
func DetailPath(rulesDir, slug string) string {
	return filepath.Join(rulesDir, slug, "README.md")
}

// IndexContent is the lint-clean stub written when spec/rules/README.md does
// not exist yet.
func IndexContent() string {
	return "---\nformat: " + IndexFormatURL + "\n---\n\n" +
		"# Rules\n\n" +
		"Normative one-sentence rules with their scope, enforcing control, and sources. " +
		"A Rule is the transferable half of what would otherwise live only in an agent's memory.\n\n" +
		"Most rules are one row and nothing more. A linked rule name has a detail document " +
		"alongside it carrying the reason, worked examples, and agent instructions.\n\n" +
		IndexHeading + "\n\n" +
		IndexHeaderRow + "\n" +
		IndexSeparatorRow + "\n\n" +
		IndexEmptyPlaceholder + "\n\n" +
		"## Open Questions\n\nNone at this time.\n\n" +
		"---\n*This document follows the " + IndexFormatURL + "*\n"
}

// MalformedRow is a row-like line the seven-column contract cannot represent.
//
// Text is kept verbatim, and every writer in this package re-emits it
// unchanged. That is the whole point: the most ordinary authoring slip there is
// — an unescaped `|` pasted into a Statement — must never cause the rule to
// disappear. A kind that exists so operating knowledge stops evaporating cannot
// have a path where a benign command evaporates one.
type MalformedRow struct {
	// Line is the 1-based source line in the index.
	Line int
	// Text is the line exactly as written, including its leading pipe.
	Text string
	// SlugHint is the identity cell's slug when that much parsed, so a caller
	// can tell "the row I am editing is the broken one" from "I am about to
	// step on someone else's broken row".
	SlugHint string
	// Reason explains, in one clause, why the line did not parse.
	Reason string
}

// IndexReport is everything a reader or a linter needs about the index file in
// one pass.
type IndexReport struct {
	Rows []Row
	// HeaderSeen is true when the canonical header + separator pair was found.
	HeaderSeen bool
	// Malformed carries every row-like line the contract cannot represent,
	// verbatim, so no writer has to guess at what it would be discarding.
	Malformed []MalformedRow
	// Duplicates lists slugs that appear in more than one row.
	Duplicates []string
}

// HasMalformed reports whether the index carries unparseable row-like content.
func (rep IndexReport) HasMalformed() bool { return len(rep.Malformed) > 0 }

// MalformedLines returns the 1-based source lines of the unparseable content.
func (rep IndexReport) MalformedLines() []int {
	out := make([]int, 0, len(rep.Malformed))
	for _, m := range rep.Malformed {
		out = append(out, m.Line)
	}
	return out
}

// MalformedExcept returns the malformed rows that are NOT one of the named
// slugs. A verb editing `x` may proceed over a broken row whose identity cell
// reads `x` — it is about to replace that row — but must not touch the index
// while some other rule's row is broken.
func (rep IndexReport) MalformedExcept(slugs ...string) []MalformedRow {
	addressed := make(map[string]bool, len(slugs))
	for _, s := range slugs {
		if s != "" {
			addressed[s] = true
		}
	}
	var out []MalformedRow
	for _, m := range rep.Malformed {
		if m.SlugHint == "" || !addressed[m.SlugHint] {
			out = append(out, m)
		}
	}
	return out
}

// BySlug indexes the report's rows.
func (rep IndexReport) BySlug() map[string]Row {
	out := make(map[string]Row, len(rep.Rows))
	for _, row := range rep.Rows {
		if _, seen := out[row.Slug]; !seen {
			out[row.Slug] = row
		}
	}
	return out
}

// Slugs returns every row slug, sorted and deduplicated.
func (rep IndexReport) Slugs() []string {
	seen := map[string]bool{}
	var out []string
	for _, row := range rep.Rows {
		if !seen[row.Slug] {
			seen[row.Slug] = true
			out = append(out, row.Slug)
		}
	}
	sort.Strings(out)
	return out
}

// ReadIndex scans the canonical `## Rules` table.
func ReadIndex(path string) (IndexReport, error) {
	b, err := osReadFile(path)
	if err != nil {
		return IndexReport{}, err
	}
	var rep IndexReport
	inRules := false
	seen := map[string]bool{}
	for i, raw := range strings.Split(string(b), "\n") {
		line := strings.TrimSpace(raw)
		lineNo := i + 1
		if line == IndexHeading {
			inRules = true
			continue
		}
		if inRules && strings.HasPrefix(line, "## ") {
			break
		}
		if !inRules {
			continue
		}
		if line == IndexHeaderRow {
			rep.HeaderSeen = true
			continue
		}
		if line == IndexSeparatorRow || line == "" || line == IndexEmptyPlaceholder {
			continue
		}
		if !strings.HasPrefix(line, "|") {
			continue
		}
		row, reason, ok := parseIndexRow(line)
		if !ok {
			rep.Malformed = append(rep.Malformed, MalformedRow{
				Line: lineNo, Text: line, SlugHint: identityHint(line), Reason: reason,
			})
			continue
		}
		row.Line = lineNo
		if seen[row.Slug] {
			rep.Duplicates = append(rep.Duplicates, row.Slug)
		}
		seen[row.Slug] = true
		rep.Rows = append(rep.Rows, row)
	}
	sort.Strings(rep.Duplicates)
	return rep, nil
}

// parseIndexRow parses one canonical table line, reporting why it failed.
func parseIndexRow(line string) (Row, string, bool) {
	cells := splitMarkdownRow(line)
	if len(cells) != indexColumns {
		return Row{}, fmt.Sprintf("row has %d cells, want %d (an unescaped `|` in a free-text cell is the usual cause)",
			len(cells), indexColumns), false
	}
	slug, linked, ok := parseIdentityCell(cells[0])
	if !ok {
		return Row{}, "first cell is neither a bare slug nor [slug](slug/README.md)", false
	}
	return rowFromCells(slug, linked, cells), "", true
}

func rowFromCells(slug string, linked bool, cells []string) Row {
	return Row{
		Slug: slug, Linked: linked,
		Status: cells[1], Scope: cells[2], Enforcement: cells[3],
		Control: cells[4], Sources: cells[5], Statement: cells[6],
	}
}

// identityHint recovers the slug from a line whose identity cell parses even
// though the row as a whole does not.
func identityHint(line string) string {
	cells := splitMarkdownRow(line)
	if len(cells) == 0 {
		return ""
	}
	slug, _, ok := parseIdentityCell(cells[0])
	if !ok {
		return ""
	}
	return slug
}

// Repair attempts the one repair that is unambiguous: a row with SURPLUS cells
// because the Statement — the last column, and the only free-text one after
// Sources — carried an unescaped `|`. The surplus cells are rejoined with their
// pipe restored and then escaped.
//
// It is only safe because the four columns between the identity cell and the
// Statement have constrained grammars. All four must validate before the tail
// is rejoined, which is what rules out the other reading — a pipe inside
// Control, where the same line shape would otherwise be silently reinterpreted.
// Anything else is reported rather than guessed at.
func (m MalformedRow) Repair() (Row, bool) {
	cells := splitMarkdownRow(m.Text)
	if len(cells) <= indexColumns {
		return Row{}, false
	}
	slug, linked, ok := parseIdentityCell(cells[0])
	if !ok {
		return Row{}, false
	}
	if _, ok := ParseStatus(cells[1]); !ok {
		return Row{}, false
	}
	if scopes := splitList(unescapeCell(cells[2])); len(scopes) == 0 {
		return Row{}, false
	} else if _, err := ParseScopes(scopes); err != nil {
		return Row{}, false
	}
	if _, ok := ParseEnforcement(cells[3]); !ok {
		return Row{}, false
	}
	if _, err := ParseSources(splitList(unescapeCell(cells[5]))); err != nil {
		return Row{}, false
	}
	head := append([]string(nil), cells[:indexColumns]...)
	head[indexColumns-1] = escapeCell(unescapeCell(strings.Join(cells[indexColumns-1:], " | ")))
	return rowFromCells(slug, linked, head), true
}

// parseIdentityCell accepts either a bare slug (inline) or `[slug](slug/README.md)`
// (detailed). Any other shape is malformed: an identity cell is the one place a
// guess would silently rename a rule.
func parseIdentityCell(cell string) (slug string, linked bool, ok bool) {
	cell = strings.TrimSpace(cell)
	if !strings.HasPrefix(cell, "[") {
		if ValidateSlug(cell) != nil {
			return "", false, false
		}
		return cell, false, true
	}
	mid := strings.Index(cell, "](")
	if mid <= 1 || !strings.HasSuffix(cell, ")") {
		return "", false, false
	}
	label, link := cell[1:mid], cell[mid+2:len(cell)-1]
	if ValidateSlug(label) != nil || link != label+"/README.md" {
		return "", false, false
	}
	return label, true, true
}

// splitMarkdownRow splits a table row into trimmed cells, honouring `\|`
// escapes so an escaped pipe inside a statement never fabricates a column.
func splitMarkdownRow(line string) []string {
	line = strings.TrimSpace(strings.Trim(line, "|"))
	if line == "" {
		return nil
	}
	var parts []string
	var current strings.Builder
	for i := 0; i < len(line); i++ {
		if line[i] == '\\' && i+1 < len(line) && line[i+1] == '|' {
			current.WriteString(`\|`)
			i++
			continue
		}
		if line[i] == '|' {
			parts = append(parts, strings.TrimSpace(current.String()))
			current.Reset()
			continue
		}
		current.WriteByte(line[i])
	}
	return append(parts, strings.TrimSpace(current.String()))
}

// WriteIndexRows replaces the `## Rules` table with rows, preserving the
// prologue before the heading and everything from the next H2 onward
// (typically `## Open Questions`). Row data is never derived from anything but
// the rows handed in: the index is the source of truth, so a regeneration that
// re-read the detail documents could overwrite an author's edit with a stale
// mirror.
//
// preserved holds row-like lines the contract could not parse. They are
// re-emitted verbatim, after the sorted rows, so that no write path in this
// package can reduce the rule set. A caller that drops them is deleting a rule
// it never managed to read — which is exactly the failure this signature exists
// to make impossible to write by accident.
func WriteIndexRows(path string, rows []Row, preserved []MalformedRow) error {
	data, err := osReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")

	indexStart, nextH2 := -1, -1
	for i, ln := range lines {
		t := strings.TrimSpace(ln)
		if t == IndexHeading && indexStart == -1 {
			indexStart = i
		} else if strings.HasPrefix(t, "## ") && indexStart != -1 && nextH2 == -1 {
			nextH2 = i
			break
		}
	}
	if indexStart == -1 {
		return fmt.Errorf("cannot locate %q heading in %s", IndexHeading, path)
	}

	ordered := append([]Row(nil), rows...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Slug < ordered[j].Slug })

	var tbl strings.Builder
	tbl.WriteString(IndexHeading + "\n\n")
	tbl.WriteString(IndexHeaderRow + "\n")
	tbl.WriteString(IndexSeparatorRow + "\n")
	switch {
	case len(ordered) == 0 && len(preserved) == 0:
		tbl.WriteString("\n" + IndexEmptyPlaceholder + "\n\n")
	default:
		for _, row := range ordered {
			tbl.WriteString(row.Render() + "\n")
		}
		// Verbatim, and last, so a reader sees the rules that parse first and
		// the ones needing a human immediately after.
		for _, m := range preserved {
			tbl.WriteString(m.Text + "\n")
		}
		tbl.WriteString("\n")
	}

	var out []string
	out = append(out, lines[:indexStart]...)
	out = append(out, strings.Split(strings.TrimRight(tbl.String(), "\n"), "\n")...)
	out = append(out, "")
	if nextH2 != -1 {
		out = append(out, lines[nextH2:]...)
	}
	return WriteFileAtomic(path, []byte(strings.Join(out, "\n")))
}

// UpsertRow inserts or replaces exactly the row owned by row.Slug, leaving
// every other row byte-identical. It is deliberately narrower than a full
// rewrite so a create or update verb has a bounded declared write set.
func UpsertRow(rulesDir string, row Row) error {
	path := IndexPath(rulesDir)
	report, err := ReadIndex(path)
	if err != nil {
		return err
	}
	if !report.HeaderSeen {
		return fmt.Errorf("rules index lacks the canonical %d-column table", indexColumns)
	}
	for _, duplicate := range report.Duplicates {
		if duplicate == row.Slug {
			return fmt.Errorf("rules index contains duplicate rows for %q", row.Slug)
		}
	}
	replaced := false
	rows := make([]Row, 0, len(report.Rows)+1)
	for _, existing := range report.Rows {
		if existing.Slug == row.Slug {
			rows = append(rows, row)
			replaced = true
			continue
		}
		rows = append(rows, existing)
	}
	if !replaced {
		rows = append(rows, row)
	}
	// A broken line whose identity cell names this slug IS this row; every
	// other one is someone else's and is carried through untouched.
	return WriteIndexRows(path, rows, report.MalformedExcept(row.Slug))
}

// RemoveRow deletes slug's row, restoring the empty placeholder when the table
// becomes empty. A missing row or a missing index file is a no-op, not an
// error: `rule delete` must still finish on a tree whose index had drifted.
func RemoveRow(rulesDir, slug string) error {
	path := IndexPath(rulesDir)
	report, err := ReadIndex(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	rows := make([]Row, 0, len(report.Rows))
	for _, existing := range report.Rows {
		if existing.Slug != slug {
			rows = append(rows, existing)
		}
	}
	preserved := report.MalformedExcept(slug)
	if len(rows) == len(report.Rows) && len(preserved) == len(report.Malformed) {
		return nil
	}
	return WriteIndexRows(path, rows, preserved)
}

// EnsureIndex writes the lint-clean index stub when spec/rules/README.md does
// not exist. An existing file is left byte-for-byte untouched.
func EnsureIndex(rulesDir string) error {
	if err := osMkdirAll(rulesDir, 0o755); err != nil {
		return err
	}
	path := IndexPath(rulesDir)
	if _, err := osStat(path); err == nil {
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	return osWriteFile(path, []byte(IndexContent()), 0o644)
}

// WriteFileAtomic publishes data to path through a same-directory temp file and
// a rename, then fsyncs the directory, so a crash mid-write can never leave a
// half-written index behind.
func WriteFileAtomic(path string, data []byte) error {
	dir := filepath.Dir(path)
	perm := os.FileMode(0o644)
	if info, err := osStat(path); err == nil {
		perm = info.Mode().Perm()
	}
	tmp, err := osCreateTemp(dir, ".rule-index-")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = osRemove(tmpPath) }()
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if n, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	} else if n != len(data) {
		_ = tmp.Close()
		return io.ErrShortWrite
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := osRename(tmpPath, path); err != nil {
		return err
	}
	d, err := osOpenDir(dir)
	if err != nil {
		return err
	}
	if err := d.Sync(); err != nil {
		_ = d.Close()
		return err
	}
	return d.Close()
}
