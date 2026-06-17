package lint

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

var decisionsIndexRuleIDs = []string{
	"DI-list-section-heading",
	"DI-index-columns",
	"DI-status-excludes-archived",
	"DI-numeric-ordering",
	"DI-archived-index-chronological",
	"DI-completeness",
	"DI-archived-status-excludes-active",
}

type decisionsIndexChecker struct {
	autofix bool
}

func newDecisionsIndexChecker() *decisionsIndexChecker {
	return &decisionsIndexChecker{}
}

func (c *decisionsIndexChecker) name() string     { return "DI-list-section-heading" }
func (c *decisionsIndexChecker) severity() string { return "error" }

func (c *decisionsIndexChecker) check(specRoot string) ([]Violation, error) {
	return checkDecisionsIndex(specRoot, c.autofix)
}

func (c *decisionsIndexChecker) fix(specRoot string) error {
	_, err := checkDecisionsIndex(specRoot, true)
	return err
}

// decisionsIndexRow represents one parsed row of the decisions index table.
type decisionsIndexRow struct {
	number   int
	numStr   string // zero-padded e.g. "0001"
	slug     string // full "0001-slug"
	title    string
	status   string
	date     string
	tags     string
	affected string
	rawLine  string
}

var decisionsIndexRowRe = regexp.MustCompile(
	`^\|\s*\[(\d{4})\]\(([^)]+)\)\s*\|\s*(.*?)\s*\|\s*(.*?)\s*\|\s*(.*?)\s*\|\s*(.*?)\s*\|\s*(.*?)\s*\|`,
)

var decisionsIndexHeaderRe = regexp.MustCompile(
	`^\|\s*#\s*\|\s*Decision\s*\|\s*Status\s*\|\s*Date\s*\|\s*Tags\s*\|\s*Affected\s*\|`,
)

var decisionsIndexSeparatorRe = regexp.MustCompile(
	`^\|[-\s|]+\|$`,
)

func checkDecisionsIndex(specRoot string, fix bool) ([]Violation, error) {
	var vs []Violation

	decisionsDir := filepath.Join(specRoot, "decisions")
	if info, err := os.Stat(decisionsDir); err != nil || !info.IsDir() {
		return nil, nil
	}

	// Check active index
	activeIdx := filepath.Join(decisionsDir, "README.md")
	if _, err := os.Stat(activeIdx); err == nil {
		v, err := checkActiveDecisionsIndex(specRoot, activeIdx, fix)
		if err != nil {
			return nil, err
		}
		vs = append(vs, v...)
	}

	// Check archived index
	archivedIdx := filepath.Join(decisionsDir, "archived", "README.md")
	if _, err := os.Stat(archivedIdx); err == nil {
		v, err := checkArchivedDecisionsIndex(specRoot, archivedIdx, fix)
		if err != nil {
			return nil, err
		}
		vs = append(vs, v...)
	}

	return vs, nil
}

func checkActiveDecisionsIndex(specRoot, indexPath string, fix bool) ([]Violation, error) {
	var vs []Violation
	rel, _ := filepath.Rel(specRoot, indexPath)

	data, err := osReadFileDecisionIndex(indexPath)
	if err != nil {
		return nil, err
	}
	content := string(data)
	lines := strings.Split(content, "\n")

	// DI-list-section-heading: must have ## Decisions
	hasDecisionsHeading := false
	decisionsHeadingLine := 0
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "## Decisions" {
			hasDecisionsHeading = true
			decisionsHeadingLine = i
			break
		}
	}
	if !hasDecisionsHeading {
		vs = append(vs, Violation{
			File: rel, Line: 0, Severity: "error",
			Rule:    "DI-list-section-heading",
			Message: "decisions index must contain a `## Decisions` section",
		})
		return vs, nil
	}

	// Find the table within the ## Decisions section
	tableStart := -1
	tableEnd := -1
	hasHeader := false
	for i := decisionsHeadingLine + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "## ") {
			break
		}
		if decisionsIndexHeaderRe.MatchString(trimmed) {
			hasHeader = true
			tableStart = i
		}
		if tableStart >= 0 && trimmed == "" && tableEnd < 0 && i > tableStart+2 {
			tableEnd = i
		}
	}
	if tableEnd < 0 && tableStart >= 0 {
		tableEnd = len(lines)
	}

	// DI-index-columns
	if !hasHeader {
		vs = append(vs, Violation{
			File: rel, Line: decisionsHeadingLine + 1, Severity: "error",
			Rule:    "DI-index-columns",
			Message: "decisions index table must include columns: #, Decision, Status, Date, Tags, Affected (in that order)",
		})
		return vs, nil
	}

	// Parse table rows
	var rows []decisionsIndexRow
	if tableStart >= 0 {
		for i := tableStart + 2; i < tableEnd; i++ {
			trimmed := strings.TrimSpace(lines[i])
			if trimmed == "" {
				continue
			}
			if decisionsIndexSeparatorRe.MatchString(trimmed) {
				continue
			}
			m := decisionsIndexRowRe.FindStringSubmatch(trimmed)
			if m == nil {
				continue
			}
			num := 0
			_, _ = fmt.Sscanf(m[1], "%d", &num)
			slug := strings.TrimSuffix(m[2], ".md")
			if strings.HasSuffix(m[2], ".md") {
				slug = strings.TrimSuffix(filepath.Base(m[2]), ".md")
			}
			rows = append(rows, decisionsIndexRow{
				number:   num,
				numStr:   m[1],
				slug:     slug,
				title:    strings.TrimSpace(m[3]),
				status:   strings.TrimSpace(m[4]),
				date:     strings.TrimSpace(m[5]),
				tags:     strings.TrimSpace(m[6]),
				affected: strings.TrimSpace(m[7]),
				rawLine:  trimmed,
			})
		}
	}

	// DI-status-excludes-archived: no Superseded/Deprecated rows
	for _, row := range rows {
		if row.status == "Superseded" || row.status == "Deprecated" {
			vs = append(vs, Violation{
				File: rel, Line: 0, Severity: "error",
				Rule:    "DI-status-excludes-archived",
				Message: fmt.Sprintf("active decisions index must not list %s decisions (found %s)", row.status, row.numStr),
			})
		}
	}

	// DI-numeric-ordering
	outOfOrder := false
	for i := 1; i < len(rows); i++ {
		if rows[i].number < rows[i-1].number {
			outOfOrder = true
			break
		}
	}
	if outOfOrder {
		if fix {
			sort.Slice(rows, func(i, j int) bool {
				return rows[i].number < rows[j].number
			})
			if err := rewriteDecisionsIndexTable(indexPath, lines, decisionsHeadingLine, rows); err != nil {
				vs = append(vs, Violation{
					File: rel, Line: 0, Severity: "error",
					Rule:    "DI-numeric-ordering",
					Message: fmt.Sprintf("decisions index rows not in ascending numeric order (fix failed: %v)", err),
				})
			}
		} else {
			vs = append(vs, Violation{
				File: rel, Line: 0, Severity: "error",
				Rule:    "DI-numeric-ordering",
				Message: "decisions index rows must be in ascending numeric order by # (run `specscore spec lint --fix`)",
			})
		}
	}

	// DI-completeness: every active decision must have a row
	decisions, err := discoverDecisionFiles(specRoot)
	if err != nil {
		return vs, nil
	}

	listedSet := make(map[string]bool)
	for _, row := range rows {
		listedSet[row.slug] = true
	}

	var missingDecisions []*parsedDecision
	for _, d := range decisions {
		if d.archived {
			continue
		}
		if d.slug != "" && !listedSet[d.slug] {
			missingDecisions = append(missingDecisions, d)
		}
	}

	if len(missingDecisions) > 0 {
		if fix {
			// Add missing rows and rewrite
			for _, d := range missingDecisions {
				status := ""
				if f, ok := d.fieldByName["Status"]; ok {
					status = f.Value
				}
				date := ""
				if f, ok := d.fieldByName["Date"]; ok {
					date = f.Value
				}
				tags := "—"
				if f, ok := d.fieldByName["Tags"]; ok && f.Value != "" && f.Value != "—" {
					tags = f.Value
				}
				numStr := fmt.Sprintf("%04d", d.number)
				row := decisionsIndexRow{
					number:   d.number,
					numStr:   numStr,
					slug:     d.slug,
					title:    d.title,
					status:   status,
					date:     date,
					tags:     tags,
					affected: "—",
					rawLine:  fmt.Sprintf("| [%s](%s.md) | %s | %s | %s | %s | %s |", numStr, d.slug, d.title, status, date, tags, "—"),
				}
				rows = append(rows, row)
			}
			sort.Slice(rows, func(i, j int) bool {
				return rows[i].number < rows[j].number
			})
			// Re-read the file since numeric-ordering fix may have rewritten it
			freshData, readErr := os.ReadFile(indexPath)
			if readErr == nil {
				freshLines := strings.Split(string(freshData), "\n")
				if err := rewriteDecisionsIndexTable(indexPath, freshLines, decisionsHeadingLine, rows); err != nil {
					var slugs []string
					for _, d := range missingDecisions {
						slugs = append(slugs, d.slug)
					}
					vs = append(vs, Violation{
						File: rel, Line: 0, Severity: "error",
						Rule:    "DI-completeness",
						Message: fmt.Sprintf("active decisions index missing entries: %s (fix failed: %v)", strings.Join(slugs, ", "), err),
					})
				}
			}
		} else {
			var slugs []string
			for _, d := range missingDecisions {
				slugs = append(slugs, d.slug)
			}
			sort.Strings(slugs)
			vs = append(vs, Violation{
				File: rel, Line: 0, Severity: "error",
				Rule:    "DI-completeness",
				Message: fmt.Sprintf("active decisions index missing entries: %s", strings.Join(slugs, ", ")),
			})
		}
	}

	return vs, nil
}

func checkArchivedDecisionsIndex(specRoot, indexPath string, fix bool) ([]Violation, error) {
	var vs []Violation
	rel, _ := filepath.Rel(specRoot, indexPath)

	data, err := osReadFileDecisionIndex(indexPath)
	if err != nil {
		return nil, err
	}
	content := string(data)

	// Parse archived entries: - YYYY-MM-DD — [NNNN-slug](NNNN-slug.md) — Status — reason
	entries := parseArchivedDecisionEntries(content)

	// Cross-check archived index entries against actual decision files.
	if decisions, discErr := discoverDecisionFiles(specRoot); discErr == nil {
		listedSet := make(map[string]bool)
		for _, e := range entries {
			listedSet[e.slug] = true
		}

		// DI-completeness: every archived decision file must have an index row.
		var missing []string
		for _, d := range decisions {
			if d.archived && d.slug != "" && !listedSet[d.slug] {
				missing = append(missing, d.slug)
			}
		}
		if len(missing) > 0 {
			sort.Strings(missing)
			vs = append(vs, Violation{
				File: rel, Line: 0, Severity: "error",
				Rule:    "DI-completeness",
				Message: fmt.Sprintf("archived decisions index missing entries: %s", strings.Join(missing, ", ")),
			})
		}

		// DI-archived-status-excludes-active: the archived index must not list
		// decisions whose file Status is still active (Draft/In Review/Approved).
		fileStatus := make(map[string]string)
		for _, d := range decisions {
			if d.slug != "" {
				if f, ok := d.fieldByName["Status"]; ok {
					fileStatus[d.slug] = f.Value
				}
			}
		}
		for _, e := range entries {
			if st, ok := fileStatus[e.slug]; ok && (st == "Draft" || st == "In Review" || st == "Approved") {
				vs = append(vs, Violation{
					File: rel, Line: 0, Severity: "error",
					Rule:    "DI-archived-status-excludes-active",
					Message: fmt.Sprintf("archived decisions index must not list %s decisions (found %s)", st, e.slug),
				})
			}
		}
	}

	// DI-archived-index-chronological
	outOfOrder := false
	for i := 1; i < len(entries); i++ {
		if entries[i].date < entries[i-1].date {
			outOfOrder = true
			break
		}
	}
	if outOfOrder {
		if fix {
			sort.Slice(entries, func(i, j int) bool {
				if entries[i].date != entries[j].date {
					return entries[i].date < entries[j].date
				}
				return entries[i].slug < entries[j].slug
			})
			if err := rewriteArchivedDecisionsIndex(indexPath, entries); err != nil {
				vs = append(vs, Violation{
					File: rel, Line: 0, Severity: "error",
					Rule:    "DI-archived-index-chronological",
					Message: fmt.Sprintf("archived decisions index not in chronological order (fix failed: %v)", err),
				})
			}
		} else {
			vs = append(vs, Violation{
				File: rel, Line: 0, Severity: "error",
				Rule:    "DI-archived-index-chronological",
				Message: "archived decisions index entries must be in chronological order by Date (oldest first)",
			})
		}
	}

	return vs, nil
}

type archivedDecisionEntry struct {
	date   string
	slug   string
	reason string
	raw    string
}

var archivedDecisionEntryRe = regexp.MustCompile(
	`^-\s+(\d{4}-\d{2}-\d{2})\s+—\s+\[([^\]]+)\]\(([^)]+)\)\s+—\s+(.+)$`,
)

func parseArchivedDecisionEntries(content string) []archivedDecisionEntry {
	var entries []archivedDecisionEntry
	scanner := bufio.NewScanner(strings.NewReader(content))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		m := archivedDecisionEntryRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		entries = append(entries, archivedDecisionEntry{
			date:   m[1],
			slug:   m[2],
			reason: m[4],
			raw:    line,
		})
	}
	return entries
}

func rewriteDecisionsIndexTable(path string, lines []string, decisionsHeadingIdx int, rows []decisionsIndexRow) error {
	// Find the table boundaries
	tableHeaderIdx := -1
	tableSepIdx := -1
	tableEndIdx := -1

	for i := decisionsHeadingIdx + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "## ") {
			tableEndIdx = i
			break
		}
		if decisionsIndexHeaderRe.MatchString(trimmed) {
			tableHeaderIdx = i
		}
		if tableHeaderIdx >= 0 && tableSepIdx < 0 && decisionsIndexSeparatorRe.MatchString(trimmed) {
			tableSepIdx = i
		}
	}

	if tableHeaderIdx < 0 || tableSepIdx < 0 {
		return fmt.Errorf("cannot locate table header/separator")
	}

	// Find where data rows end: first empty line or next ## heading after separator
	dataEnd := len(lines)
	for i := tableSepIdx + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" || strings.HasPrefix(trimmed, "## ") {
			dataEnd = i
			break
		}
	}

	// Build new data rows
	var newRows []string
	for _, r := range rows {
		newRows = append(newRows, r.rawLine)
	}

	var result []string
	result = append(result, lines[:tableSepIdx+1]...)
	result = append(result, newRows...)
	if tableEndIdx >= 0 {
		result = append(result, lines[dataEnd:]...)
	}

	return osWriteFileDecisionIndex(path, []byte(strings.Join(result, "\n")), 0o644)
}

func rewriteArchivedDecisionsIndex(path string, entries []archivedDecisionEntry) error {
	data, err := osReadFileDecisionIndex(path)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")

	// Find the region of archived entries
	entryStart := -1
	entryEnd := len(lines)
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if archivedDecisionEntryRe.MatchString(trimmed) {
			if entryStart < 0 {
				entryStart = i
			}
			entryEnd = i + 1
		}
	}

	if entryStart < 0 {
		return nil
	}

	var newEntryLines []string
	for _, e := range entries {
		newEntryLines = append(newEntryLines, e.raw)
	}

	var result []string
	result = append(result, lines[:entryStart]...)
	result = append(result, newEntryLines...)
	result = append(result, lines[entryEnd:]...)

	return osWriteFileDecisionIndex(path, []byte(strings.Join(result, "\n")), 0o644)
}
