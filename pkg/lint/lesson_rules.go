// specscore:feature/cli/spec/lint/lesson-rules
//
// Implements the L-family lint rules for the Lesson doc kind: required
// section presence (L-001), status-value validity (L-002), and lessons-index
// completeness + row sync (L-003 / L-004) — mirroring the plan-rules /
// idea-index rule families as closely as a flatter, sectionless-metadata kind
// allows.
package lint

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/specscore/specscore-cli/pkg/lesson"
)

// lessonRuleIDs is the ordered rule-name set the lessonRulesChecker answers
// to; linter.go registers the single checker instance under each.
var lessonRuleIDs = []string{"L-001", "L-002", "L-003", "L-004"}

// canonicalLessonStatuses is the legal Lesson **Status:** set: the three-rung
// enforcement ladder plus the two dispositions.
var canonicalLessonStatuses = map[string]bool{
	"Recorded":   true,
	"Stated":     true,
	"Enforced":   true,
	"Withdrawn":  true,
	"Superseded": true,
}

const canonicalLessonStatusList = "Recorded, Stated, Enforced, Withdrawn, Superseded"

// lessonRulesChecker implements L-001..L-004. One checker emits violations
// for all four rule names; the linter framework dedupes by pointer identity
// so a single walk produces every finding.
type lessonRulesChecker struct {
	// fixIndex, when true, makes the fix pass regenerate spec/lessons/README.md
	// from the parsed Lesson set (L-003 missing rows, L-004 drifted rows).
	fixIndex bool
}

func newLessonRulesChecker() *lessonRulesChecker { return &lessonRulesChecker{} }

func (c *lessonRulesChecker) name() string     { return "L-001" }
func (c *lessonRulesChecker) severity() string { return "error" }

func (c *lessonRulesChecker) check(specRoot string) ([]Violation, error) {
	lessonsDir := filepath.Join(specRoot, "lessons")
	info, err := os.Stat(lessonsDir)
	if err != nil || !info.IsDir() {
		return nil, nil
	}

	entries, err := os.ReadDir(lessonsDir)
	if err != nil {
		return nil, fmt.Errorf("reading lessons dir: %w", err)
	}

	var violations []Violation
	parsed := map[string]*lesson.Lesson{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "README.md" || !strings.HasSuffix(name, ".md") {
			continue
		}
		lessonPath := filepath.Join(lessonsDir, name)
		l, parseErr := lesson.Parse(lessonPath)
		if parseErr != nil {
			return nil, fmt.Errorf("parsing lesson %s: %w", lessonPath, parseErr)
		}
		if !l.HasLessonTitle {
			continue // not a SpecScore single-file Lesson
		}
		relPath, _ := filepath.Rel(specRoot, lessonPath)
		violations = append(violations, lintLesson(l, relPath)...)
		parsed[l.Slug] = l
	}

	idxViolations, _ := lessonIndexRules(specRoot, parsed, false)
	violations = append(violations, idxViolations...)

	sort.SliceStable(violations, func(i, j int) bool {
		if violations[i].File != violations[j].File {
			return violations[i].File < violations[j].File
		}
		if violations[i].Line != violations[j].Line {
			return violations[i].Line < violations[j].Line
		}
		return violations[i].Rule < violations[j].Rule
	})
	return violations, nil
}

func (c *lessonRulesChecker) fix(specRoot string) error {
	if !c.fixIndex {
		return nil
	}
	lessonsDir := filepath.Join(specRoot, "lessons")
	if info, err := os.Stat(lessonsDir); err != nil || !info.IsDir() {
		return nil
	}
	entries, err := os.ReadDir(lessonsDir)
	if err != nil {
		return fmt.Errorf("reading lessons dir: %w", err)
	}
	parsed := map[string]*lesson.Lesson{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if name == "README.md" || !strings.HasSuffix(name, ".md") {
			continue
		}
		l, parseErr := lesson.Parse(filepath.Join(lessonsDir, name))
		if parseErr != nil {
			return fmt.Errorf("parsing lesson %s: %w", name, parseErr)
		}
		if !l.HasLessonTitle {
			continue
		}
		parsed[l.Slug] = l
	}
	_, _ = lessonIndexRules(specRoot, parsed, true)
	return nil
}

// lintLesson runs L-001 (required sections) and L-002 (status value) against
// a single parsed Lesson.
func lintLesson(l *lesson.Lesson, relPath string) []Violation {
	var v []Violation

	if missing := l.MissingRequiredSections(); len(missing) > 0 {
		v = append(v, Violation{
			File:     relPath,
			Line:     l.TitleLine,
			Severity: "error",
			Rule:     "L-001",
			Message: fmt.Sprintf(
				"missing required section(s): %s — a lesson recording an incident but naming no process gap or enforcement path is the entry this rule exists to refuse",
				strings.Join(missing, ", "),
			),
		})
	}

	if l.StatusLine != 0 && !canonicalLessonStatuses[l.Status] {
		v = append(v, Violation{
			File:     relPath,
			Line:     l.StatusLine,
			Severity: "error",
			Rule:     "L-002",
			Message: fmt.Sprintf(
				"invalid lesson **Status:** value %q (accepted: %s)",
				l.Status, canonicalLessonStatusList,
			),
		})
	}

	return v
}

// ----- L-003 / L-004: lessons-index completeness and row sync -----

// lessonIndexRow captures one parsed row of the lessons index table.
type lessonIndexRow struct {
	slug     string
	status   string
	recurred string
	date     string
	owner    string
}

func (r lessonIndexRow) equals(o lessonIndexRow) bool {
	return r.slug == o.slug && r.status == o.status && r.recurred == o.recurred &&
		r.date == o.date && r.owner == o.owner
}

func expectedLessonIndexRow(slug string, l *lesson.Lesson) lessonIndexRow {
	r := lessonIndexRow{slug: slug, recurred: "0"}
	if l == nil {
		return r
	}
	r.status = strings.TrimSpace(l.Status)
	r.recurred = strconv.Itoa(l.Recurred)
	r.date = strings.TrimSpace(l.Date)
	r.owner = strings.TrimSpace(l.Owner)
	return r
}

// lessonIndexRowRe matches one row of the lessons index table:
//
//	| [<slug>](<slug>.md) | <status> | <recurred> | <date> | <owner> |
var lessonIndexRowRe = regexp.MustCompile(`^\|\s*\[[^\]]+\]\(([^)]+)\.md\)\s*\|\s*(.*?)\s*\|\s*(.*?)\s*\|\s*(.*?)\s*\|\s*(.*?)\s*\|\s*$`)

// lessonIndexRules validates (and, when fix, regenerates) spec/lessons/README.md
// against the discovered Lesson set. Mirrors ideaIndexRules, minus the
// archived-index half: a Lesson never relocates on disposition, so there is
// only one active index.
func lessonIndexRules(specRoot string, parsed map[string]*lesson.Lesson, fix bool) ([]Violation, bool) {
	var vs []Violation
	fixed := false

	idxPath := filepath.Join(specRoot, "lessons", "README.md")
	if _, err := os.Stat(idxPath); err != nil {
		return vs, false
	}
	listedRows, err := readLessonIndexRows(idxPath)
	if err != nil {
		return vs, false
	}
	listedSet := make(map[string]lessonIndexRow, len(listedRows))
	for _, r := range listedRows {
		listedSet[r.slug] = r
	}

	slugs := make([]string, 0, len(parsed))
	for slug := range parsed {
		slugs = append(slugs, slug)
	}
	sort.Strings(slugs)

	var missing, drifted []string
	for _, slug := range slugs {
		row, present := listedSet[slug]
		if !present {
			missing = append(missing, slug)
			continue
		}
		if expected := expectedLessonIndexRow(slug, parsed[slug]); !row.equals(expected) {
			drifted = append(drifted, slug)
		}
	}

	rel, _ := filepath.Rel(specRoot, idxPath)
	needsRewrite := len(missing) > 0 || len(drifted) > 0

	if needsRewrite && fix {
		if err := rewriteLessonIndex(idxPath, slugs, parsed); err == nil {
			fixed = true
			return vs, fixed
		}
	}
	if len(missing) > 0 {
		vs = append(vs, Violation{
			File: rel, Line: 0, Severity: "error",
			Rule:    "L-003",
			Message: fmt.Sprintf("lessons index missing entries: %s (run `specscore spec lint --fix`)", strings.Join(missing, ", ")),
		})
	}
	if len(drifted) > 0 {
		vs = append(vs, Violation{
			File: rel, Line: 0, Severity: "error",
			Rule:    "L-004",
			Message: fmt.Sprintf("lessons index rows drifted from Lesson files (Status/Recurred/Date/Owner): %s (run `specscore spec lint --fix`)", strings.Join(drifted, ", ")),
		})
	}
	return vs, fixed
}

// readLessonIndexRows scans an index README and returns one lessonIndexRow
// per row of the ## Index table.
func readLessonIndexRows(path string) ([]lessonIndexRow, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	var rows []lessonIndexRow
	inIndex := false
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if strings.HasPrefix(line, "## Index") {
			inIndex = true
			continue
		}
		if inIndex && strings.HasPrefix(line, "## ") {
			break
		}
		if !inIndex {
			continue
		}
		m := lessonIndexRowRe.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		slug := m[1]
		if strings.Contains(slug, "/") {
			continue
		}
		rows = append(rows, lessonIndexRow{
			slug: slug, status: strings.TrimSpace(m[2]), recurred: strings.TrimSpace(m[3]),
			date: strings.TrimSpace(m[4]), owner: strings.TrimSpace(m[5]),
		})
	}
	return rows, scanner.Err()
}

// rewriteLessonIndex regenerates spec/lessons/README.md, preserving the
// prologue before "## Index" and everything from the next H2 heading onward
// (typically "## Open Questions"), replacing only the table body in between.
func rewriteLessonIndex(path string, slugs []string, parsed map[string]*lesson.Lesson) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")

	indexStart := -1
	nextH2 := -1
	for i, ln := range lines {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "## Index") && indexStart == -1 {
			indexStart = i
		} else if strings.HasPrefix(t, "## ") && indexStart != -1 && nextH2 == -1 {
			nextH2 = i
			break
		}
	}
	if indexStart == -1 {
		return fmt.Errorf("cannot locate ## Index heading")
	}

	var tbl strings.Builder
	tbl.WriteString("## Index\n\n")
	tbl.WriteString("| Lesson | Status | Recurred | Date | Owner |\n")
	tbl.WriteString("|---|---|---|---|---|\n")

	if len(slugs) == 0 {
		tbl.WriteString("\n_No lessons recorded yet._\n\n")
	} else {
		for _, slug := range slugs {
			l := parsed[slug]
			status, recurred, date, owner := "", "0", "", ""
			if l != nil {
				status = strings.TrimSpace(l.Status)
				recurred = strconv.Itoa(l.Recurred)
				date = strings.TrimSpace(l.Date)
				owner = strings.TrimSpace(l.Owner)
			}
			fmt.Fprintf(&tbl, "| [%s](%s.md) | %s | %s | %s | %s |\n", slug, slug, status, recurred, date, owner)
		}
		tbl.WriteString("\n")
	}

	var newLines []string
	newLines = append(newLines, lines[:indexStart]...)
	newLines = append(newLines, strings.Split(strings.TrimRight(tbl.String(), "\n"), "\n")...)
	newLines = append(newLines, "")
	if nextH2 != -1 {
		newLines = append(newLines, lines[nextH2:]...)
	}
	return os.WriteFile(path, []byte(strings.Join(newLines, "\n")), 0o644)
}
