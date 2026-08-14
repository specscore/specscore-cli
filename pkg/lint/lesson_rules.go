// specscore:feature/cli/spec/lint/lesson-rules
//
// Implements the L-family lint rules for the Lesson doc kind: required
// section presence (L-001), status-value validity (L-002), and lessons-index
// completeness + row sync (L-003 / L-004) — mirroring the plan-rules /
// idea-index rule families as closely as a flatter, sectionless-metadata kind
// allows.
package lint

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gofrs/flock"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/specscore/specscore-cli/pkg/projectdef"
)

type lessonIndexLocker interface {
	Lock() error
	Unlock() error
}

type lessonIndexLockDeps struct {
	mkdirAll func(string, os.FileMode) error
	newLock  func(string) lessonIndexLocker
}

type lessonIndexFile interface {
	Name() string
	Chmod(os.FileMode) error
	Write([]byte) (int, error)
	Sync() error
	Close() error
}

type lessonIndexWriteOps struct {
	stat       func(string) (os.FileInfo, error)
	createTemp func(string, string) (lessonIndexFile, error)
	rename     func(string, string) error
	remove     func(string) error
	open       func(string) (lessonIndexFile, error)
}

func defaultLessonIndexWriteOps() lessonIndexWriteOps {
	return lessonIndexWriteOps{
		stat:       os.Stat,
		createTemp: func(dir, pattern string) (lessonIndexFile, error) { return os.CreateTemp(dir, pattern) },
		rename:     os.Rename,
		remove:     os.Remove,
		open:       func(path string) (lessonIndexFile, error) { return os.Open(path) },
	}
}

func defaultLessonIndexLockDeps() lessonIndexLockDeps {
	return lessonIndexLockDeps{
		mkdirAll: os.MkdirAll,
		newLock:  func(path string) lessonIndexLocker { return flock.New(path) },
	}
}

// lessonRuleIDs is the ordered rule-name set the lessonRulesChecker answers
// to; linter.go registers the single checker instance under each.
var lessonRuleIDs = []string{"L-001", "L-002", "L-003", "L-004", "L-005", "L-006", "L-007", "L-008", "L-009"}

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
	projectRoot string
	// fixIndex, when true, makes the fix pass regenerate spec/lessons/README.md
	// from the parsed Lesson set (L-003 missing rows, L-004 drifted rows).
	fixIndex bool
	// withIndexLock is an operation-scoped test seam. Production uses the
	// shared cross-process Lesson-index lock.
	withIndexLock func(string, func() error) error
}

func newLessonRulesChecker(projectRoot ...string) *lessonRulesChecker {
	var root string
	if len(projectRoot) > 0 {
		root = projectRoot[0]
	}
	return &lessonRulesChecker{projectRoot: root}
}

func (c *lessonRulesChecker) effectiveProjectRoot(specRoot string) string {
	return lintProjectRoot(c.projectRoot, specRoot)
}

func (c *lessonRulesChecker) name() string     { return "L-001" }
func (c *lessonRulesChecker) severity() string { return "error" }

func (c *lessonRulesChecker) check(specRoot string) ([]Violation, error) {
	lessonsDir := filepath.Join(specRoot, "lessons")
	info, err := os.Stat(lessonsDir)
	if err != nil || !info.IsDir() {
		return nil, nil
	}

	var violations []Violation
	parsed := map[string]*lesson.Lesson{}
	lessons, err := lesson.Discover(lessonsDir)
	if err != nil {
		if strings.Contains(err.Error(), "both legacy flat and canonical directory forms") {
			violations = append(violations, Violation{File: "lessons", Severity: "error", Rule: "L-005", Message: err.Error()})
			return violations, nil
		}
		return nil, err
	}
	projectRoot := c.effectiveProjectRoot(specRoot)
	allowed, configErr := lessonClassificationsAtProject(projectRoot)
	for _, l := range lessons {
		relPath, _ := filepath.Rel(specRoot, l.Path)
		violations = append(violations, lintLesson(l, relPath)...)
		if l.Canonical {
			violations = append(violations, lintCanonicalLessonAtProject(projectRoot, specRoot, l, relPath, allowed, configErr)...)
			violations = append(violations, lintOccurrenceChildren(specRoot, l)...)
		}
		parsed[l.Slug] = l
	}
	violations = append(violations, lintLessonRelations(specRoot, parsed)...)
	if _, err := lesson.ListRelations(lessonsDir, firstLessonSlug(lessons)); err != nil && len(lessons) > 0 {
		violations = append(violations, Violation{File: "lessons/.relations.json", Severity: "error", Rule: "L-008", Message: err.Error()})
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
	lock := c.withIndexLock
	if lock == nil {
		lock = func(root string, mutate func() error) error {
			return withLessonIndexLockAtProject(c.effectiveProjectRoot(root), root, defaultLessonIndexLockDeps(), mutate)
		}
	}
	// This repair does not mutate a Lesson artifact, so it takes only the
	// shared index lock. Holding it before discovery makes the parsed set and
	// replacement table one serialized snapshot: a lifecycle writer that has
	// already published waits here, then reconciles its current row afterward.
	return lock(specRoot, func() error {
		parsed := map[string]*lesson.Lesson{}
		lessons, err := lesson.Discover(lessonsDir)
		if err != nil {
			return err
		}
		for _, l := range lessons {
			parsed[l.Slug] = l
		}
		_, _ = lessonIndexRulesWithRewrite(specRoot, parsed, true, rewriteLessonIndexUnlocked)
		return nil
	})
}

// lintLesson runs L-001 (required sections) and L-002 (status value) against
// a single parsed Lesson.
func lintLesson(l *lesson.Lesson, relPath string) []Violation {
	var v []Violation

	if missing := l.MissingRequiredSectionsForLayout(); len(missing) > 0 {
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

func firstLessonSlug(items []*lesson.Lesson) string {
	if len(items) == 0 {
		return ""
	}
	return items[0].Slug
}
func lessonClassifications(specRoot string) (map[string]bool, error) {
	return lessonClassificationsAtProject(lintProjectRoot("", specRoot))
}

func lessonClassificationsAtProject(projectRoot string) (map[string]bool, error) {
	cfg, err := projectdef.ReadSpecConfig(projectRoot)
	if err != nil {
		return nil, err
	}
	return lessonClassificationsFromConfig(cfg)
}

func lessonClassificationsFromConfig(cfg projectdef.SpecConfig) (map[string]bool, error) {
	raw, ok := cfg.Extras["lessons"].(map[string]any)
	if !ok {
		return nil, fmt.Errorf("specscore.yaml lacks lessons.classifications")
	}
	return lessonClassificationsFrom(raw["classifications"])
}

func lessonClassificationsFrom(raw any) (map[string]bool, error) {
	var values []string
	switch x := raw.(type) {
	case []any:
		for _, v := range x {
			if s, ok := v.(string); ok {
				values = append(values, s)
			}
		}
	case []string:
		values = x
	}
	if len(values) == 0 {
		return nil, fmt.Errorf("specscore.yaml lessons.classifications is empty")
	}
	out := map[string]bool{}
	for _, v := range values {
		v = strings.TrimSpace(v)
		if err := lesson.ValidateSlug(v); err != nil {
			return nil, fmt.Errorf("invalid lesson classification vocabulary")
		}
		if out[v] {
			return nil, fmt.Errorf("duplicate lesson classification vocabulary")
		}
		out[v] = true
	}
	return out, nil
}

func lintCanonicalLesson(specRoot string, l *lesson.Lesson, rel string, allowed map[string]bool, configErr error) []Violation {
	return lintCanonicalLessonAtProject(lintProjectRoot("", specRoot), specRoot, l, rel, allowed, configErr)
}

func lintCanonicalLessonAtProject(projectRoot, specRoot string, l *lesson.Lesson, rel string, allowed map[string]bool, configErr error) []Violation {
	var out []Violation
	if !l.HasLessonTitle || strings.TrimSpace(l.Title) == "" {
		out = append(out, Violation{File: rel, Rule: "L-005", Severity: "error", Message: "canonical Lesson requires a non-empty # Lesson: title"})
	}
	required := []struct {
		name string
		line int
	}{{"Status", l.StatusLine}, {"Date", l.DateLine}, {"Owner", l.OwnerLine}, {"Classifications", l.ClassificationsLine}, {"Legacy Provenance", l.LegacyProvenanceLine}, {"Duplicate Of", l.DuplicateOfLine}, {"Supersedes", l.SupersedesLine}, {"Superseded By", l.SupersededByLine}}
	last := 0
	for _, f := range required {
		if f.line == 0 {
			out = append(out, Violation{File: rel, Rule: "L-005", Severity: "error", Message: "missing canonical metadata field: " + f.name})
		} else if f.line <= last {
			out = append(out, Violation{File: rel, Line: f.line, Rule: "L-005", Severity: "error", Message: "canonical metadata fields are out of order"})
		} else {
			last = f.line
		}
		if l.FieldCounts[f.name] > 1 {
			out = append(out, Violation{File: rel, Line: f.line, Rule: "L-005", Severity: "error", Message: "canonical metadata field is duplicated: " + f.name})
		}
	}
	if l.Date == "" || l.Owner == "" {
		out = append(out, Violation{File: rel, Rule: "L-005", Severity: "error", Message: "canonical Date and Owner must be non-empty"})
	} else if _, err := time.Parse("2006-01-02", l.Date); err != nil {
		out = append(out, Violation{File: rel, Line: l.DateLine, Rule: "L-005", Severity: "error", Message: "canonical Date must be YYYY-MM-DD"})
	}
	for _, name := range []string{"Legacy Provenance", "Duplicate Of", "Supersedes", "Superseded By"} {
		value := map[string]string{"Legacy Provenance": l.LegacyProvenance, "Duplicate Of": l.DuplicateOf, "Supersedes": l.Supersedes, "Superseded By": l.SupersededBy}[name]
		if strings.TrimSpace(value) == "" {
			out = append(out, Violation{File: rel, Rule: "L-005", Severity: "error", Message: name + " must use — when absent"})
		}
	}
	requiredSections := lesson.RequiredSectionsFor(l)
	sectionLast := 0
	for _, name := range requiredSections {
		line := l.SectionLines[name]
		if line > 0 && line <= sectionLast {
			out = append(out, Violation{File: rel, Line: line, Rule: "L-005", Severity: "error", Message: "canonical Lesson sections are out of order"})
		}
		if line > sectionLast {
			sectionLast = line
		}
	}
	if configErr != nil {
		out = append(out, Violation{File: rel, Rule: "L-005", Severity: "error", Message: configErr.Error()})
	} else {
		seen := map[string]bool{}
		if len(l.Classifications) == 0 {
			out = append(out, Violation{File: rel, Rule: "L-005", Severity: "error", Message: "Classifications must be non-empty"})
		}
		for _, v := range l.Classifications {
			if seen[v] || !allowed[v] {
				out = append(out, Violation{File: rel, Line: l.ClassificationsLine, Rule: "L-005", Severity: "error", Message: "classification is duplicate or outside configured vocabulary"})
			}
			seen[v] = true
		}
	}
	b, _ := os.ReadFile(l.Path)
	text := string(b)
	if err := lesson.ValidateSafeContent("Lesson README", text); err != nil {
		out = append(out, Violation{File: rel, Rule: "L-005", Severity: "error", Message: err.Error()})
	}
	if l.RecurredLine != 0 || !strings.Contains(text, "- **Occurrence store:** `occurrences/`") || !strings.Contains(text, "- **Recurrence metadata:** derived from child JSON; never hand-maintained here.") || !strings.Contains(text, "- **Occurrence schema:** `https://specscore.md/new/lesson-occurrence.schema.json`") {
		out = append(out, Violation{File: rel, Rule: "L-006", Severity: "error", Message: "canonical Tracking must declare occurrence store, derived recurrence, and schema; Recurred is forbidden"})
	}
	if l.Status == "Enforced" {
		if l.Control == "" || l.Control == "—" || l.Verification == "" || l.Verification == "—" || l.Evidence == "" || l.Evidence == "—" || strings.Contains(strings.ToLower(l.Verification), "manual") || !stableLessonEvidenceAtProject(projectRoot, l.Evidence) {
			out = append(out, Violation{File: rel, Rule: "L-007", Severity: "error", Message: "Enforced Lesson requires deterministic Control, Verification, and Evidence"})
		}
	}
	return out
}

func stableLessonEvidence(specRoot, value string) bool {
	return stableLessonEvidenceAtProject(lintProjectRoot("", specRoot), value)
}

func stableLessonEvidenceAtProject(projectRoot, value string) bool {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(value, "sha256:") && len(value) > len("sha256:")+16 {
		return true
	}
	if strings.HasPrefix(value, "https://") || strings.HasPrefix(value, "http://") {
		return strings.Contains(value, "/commit/") || strings.Contains(value, "/blob/") || strings.Contains(value, "sha256=")
	}
	if strings.Contains(value, "\\") || filepath.IsAbs(value) || strings.HasPrefix(value, "../") || strings.Contains(value, "/../") || value == "." || value == ".." {
		return false
	}
	_, err := os.Stat(filepath.Join(projectRoot, filepath.FromSlash(value)))
	return err == nil
}

func lintLessonRelations(specRoot string, parsed map[string]*lesson.Lesson) []Violation {
	var out []Violation
	for slug, l := range parsed {
		if !l.Canonical {
			continue
		}
		for name, target := range map[string]string{"Duplicate Of": l.DuplicateOf, "Supersedes": l.Supersedes, "Superseded By": l.SupersededBy} {
			if target == "" || target == "—" {
				continue
			}
			if err := lesson.ValidateSlug(target); err != nil || parsed[target] == nil {
				out = append(out, Violation{File: mustRel(specRoot, l.Path), Rule: "L-008", Severity: "error", Message: name + " does not resolve to a canonical Lesson"})
			}
		}
		if l.DuplicateOf != "" && l.DuplicateOf != "—" {
			if l.Status == "Enforced" || l.Status != "Superseded" || l.SupersededBy != l.DuplicateOf {
				out = append(out, Violation{File: mustRel(specRoot, l.Path), Rule: "L-008", Severity: "error", Message: "retained duplicate must be Superseded and point Duplicate Of and Superseded By at the same canonical Lesson"})
			}
		}
		if l.Supersedes != "" && l.Supersedes != "—" {
			prior := parsed[l.Supersedes]
			if prior != nil && (prior.SupersededBy != slug || prior.Status != "Superseded") {
				out = append(out, Violation{File: mustRel(specRoot, l.Path), Rule: "L-008", Severity: "error", Message: "Supersedes target must be Superseded with the inverse Superseded By pointer"})
			}
		}
	}
	for start := range parsed {
		seen := map[string]bool{}
		for current := start; current != ""; {
			if seen[current] {
				l := parsed[start]
				out = append(out, Violation{File: mustRel(specRoot, l.Path), Rule: "L-008", Severity: "error", Message: "Lesson relation cycle detected"})
				break
			}
			seen[current] = true
			l := parsed[current]
			if l == nil || l.Supersedes == "" || l.Supersedes == "—" {
				break
			}
			current = l.Supersedes
		}
	}
	return out
}

func lintOccurrenceChildren(specRoot string, l *lesson.Lesson) []Violation {
	entries, err := os.ReadDir(l.OccurrencesDir)
	if os.IsNotExist(err) {
		return []Violation{{File: mustRel(specRoot, l.OccurrencesDir), Rule: "L-009", Severity: "error", Message: "occurrences directory is missing"}}
	}
	if err != nil {
		return []Violation{{File: mustRel(specRoot, l.OccurrencesDir), Rule: "L-009", Severity: "error", Message: err.Error()}}
	}
	var out []Violation
	for _, e := range entries {
		path := filepath.Join(l.OccurrencesDir, e.Name())
		rel := mustRel(specRoot, path)
		if e.Name() == lesson.OccurrenceStoreKeepFile && !e.IsDir() {
			info, err := e.Info()
			if err == nil && info.Size() == 0 {
				continue
			}
			out = append(out, Violation{File: rel, Rule: "L-009", Severity: "error", Message: "occurrence-store marker must be an empty file"})
			continue
		}
		if e.IsDir() || filepath.Ext(e.Name()) != ".json" {
			out = append(out, Violation{File: rel, Rule: "L-009", Severity: "error", Message: "occurrences contains a non-JSON child"})
			continue
		}
		if _, err := lesson.ValidateOccurrenceFile(path); err != nil {
			out = append(out, Violation{File: rel, Rule: "L-009", Severity: "error", Message: err.Error()})
		}
	}
	return out
}
func mustRel(root, path string) string { rel, _ := filepath.Rel(root, path); return rel }

// ----- L-003 / L-004: lessons-index completeness and row sync -----

// lessonIndexRow is the six-column core index projection. Link is kept
// separately so flat compatibility entries cannot masquerade as canonical.
type lessonIndexRow struct {
	slug            string
	link            string
	status          string
	classifications string
	occurrences     string
	lastOccurred    string
	enforcement     string
}

func (r lessonIndexRow) equals(o lessonIndexRow) bool {
	return r.slug == o.slug && r.link == o.link && r.status == o.status &&
		r.classifications == o.classifications && r.occurrences == o.occurrences &&
		r.lastOccurred == o.lastOccurred && r.enforcement == o.enforcement
}

func expectedLessonIndexRow(slug string, l *lesson.Lesson) (lessonIndexRow, error) {
	r := lessonIndexRow{slug: slug, link: slug + "/README.md", occurrences: "0", enforcement: "—"}
	if l == nil {
		return r, nil
	}
	r.status = strings.TrimSpace(l.Status)
	if !l.Canonical {
		r.link = slug + ".md"
		r.classifications = "Legacy"
		r.occurrences = strconv.Itoa(l.Recurred)
		return r, nil
	}
	r.classifications = strings.Join(l.Classifications, ", ")
	items, err := lesson.DiscoverOccurrences(l.Path)
	if err != nil {
		return r, err
	}
	r.occurrences = strconv.Itoa(len(items))
	if len(items) > 0 {
		r.lastOccurred = items[len(items)-1].OccurredAt.UTC().Format("2006-01-02T15:04:05Z")
	}
	if l.DuplicateOf != "" && l.DuplicateOf != "—" {
		r.enforcement = "Duplicate Of: " + l.DuplicateOf
	} else if strings.TrimSpace(l.Control) != "" {
		r.enforcement = strings.TrimSpace(l.Control)
	}
	return r, nil
}

// lessonIndexRules validates (and, when fix, regenerates) spec/lessons/README.md
// against the discovered Lesson set. Mirrors ideaIndexRules, minus the
// archived-index half: a Lesson never relocates on disposition, so there is
// only one active index.
func lessonIndexRules(specRoot string, parsed map[string]*lesson.Lesson, fix bool) ([]Violation, bool) {
	return lessonIndexRulesWithRewrite(specRoot, parsed, fix, rewriteLessonIndex)
}

func lessonIndexRulesWithRewrite(specRoot string, parsed map[string]*lesson.Lesson, fix bool, rewrite func(string, []string, map[string]*lesson.Lesson) error) ([]Violation, bool) {
	var vs []Violation
	fixed := false

	idxPath := filepath.Join(specRoot, "lessons", "README.md")
	if _, err := os.Stat(idxPath); err != nil {
		return vs, false
	}
	listedRows, canonicalShape, malformed, err := readLessonIndexRows(idxPath)
	if err != nil {
		return []Violation{{File: mustRel(specRoot, idxPath), Severity: "error", Rule: "L-004", Message: err.Error()}}, false
	}
	listedSet := make(map[string]lessonIndexRow, len(listedRows))
	var duplicates []string
	for _, r := range listedRows {
		if _, exists := listedSet[r.slug]; exists {
			duplicates = append(duplicates, r.slug)
		}
		listedSet[r.slug] = r
	}

	slugs := make([]string, 0, len(parsed))
	hasCanonical := false
	for slug := range parsed {
		slugs = append(slugs, slug)
		hasCanonical = hasCanonical || parsed[slug].Canonical
	}
	sort.Strings(slugs)

	var missing, drifted, orphaned []string
	for _, slug := range slugs {
		row, present := listedSet[slug]
		if !present {
			missing = append(missing, slug)
			continue
		}
		expected, expectedErr := expectedLessonIndexRow(slug, parsed[slug])
		if expectedErr != nil || !row.equals(expected) {
			drifted = append(drifted, slug)
		}
	}
	for slug := range listedSet {
		if parsed[slug] == nil {
			orphaned = append(orphaned, slug)
		}
	}
	sort.Strings(orphaned)
	sort.Strings(duplicates)

	rel, _ := filepath.Rel(specRoot, idxPath)
	shapeMismatch := hasCanonical && !canonicalShape
	needsRewrite := shapeMismatch || malformed || len(missing) > 0 || len(drifted) > 0 || len(orphaned) > 0 || len(duplicates) > 0

	if needsRewrite && fix {
		if err := rewrite(idxPath, slugs, parsed); err == nil {
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
	if shapeMismatch || malformed || len(drifted) > 0 || len(orphaned) > 0 || len(duplicates) > 0 {
		parts := make([]string, 0, 4)
		if shapeMismatch || malformed {
			parts = append(parts, "table shape")
		}
		if len(drifted) > 0 {
			parts = append(parts, "drifted: "+strings.Join(drifted, ", "))
		}
		if len(orphaned) > 0 {
			parts = append(parts, "orphaned: "+strings.Join(orphaned, ", "))
		}
		if len(duplicates) > 0 {
			parts = append(parts, "duplicated: "+strings.Join(duplicates, ", "))
		}
		vs = append(vs, Violation{
			File: rel, Line: 0, Severity: "error",
			Rule:    "L-004",
			Message: fmt.Sprintf("lessons index does not match the canonical six-column projection (%s) (run `specscore spec lint --fix`)", strings.Join(parts, "; ")),
		})
	}
	return vs, fixed
}

// readLessonIndexRows scans the exact ## Lessons table. malformed is true for
// row-like content that cannot be represented by the six-column contract.
func readLessonIndexRows(path string) (rows []lessonIndexRow, canonical, malformed bool, err error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, false, false, err
	}
	inLessons := false
	legacySection := false
	headerSeen := false
	for _, raw := range strings.Split(string(b), "\n") {
		line := strings.TrimSpace(raw)
		if line == "## Lessons" {
			inLessons = true
			legacySection = false
			continue
		}
		if line == "## Index" {
			inLessons = true
			legacySection = true
			continue
		}
		if inLessons && strings.HasPrefix(line, "## ") {
			break
		}
		if !inLessons {
			continue
		}
		if line == "| Lesson | Status | Classifications | Occurrences | Last Occurred | Enforcement |" {
			headerSeen = true
			continue
		}
		if legacySection && line == "| Lesson | Status | Recurred | Date | Owner |" {
			headerSeen = true
			continue
		}
		if line == "|---|---|---|---:|---|---|" || line == "|---|---|---|---|---|---|" || line == "" {
			continue
		}
		if legacySection && line == "|---|---|---|---|---|" {
			continue
		}
		if !strings.HasPrefix(line, "|") {
			continue
		}
		cells := splitMarkdownRow(line)
		if legacySection && len(cells) == 5 {
			label, link, ok := parseMarkdownLink(cells[0])
			if !ok || link != label+".md" {
				malformed = true
				continue
			}
			rows = append(rows, lessonIndexRow{slug: label, link: link, status: cells[1], classifications: "Legacy", occurrences: cells[2], enforcement: "—"})
			continue
		}
		if len(cells) != 6 {
			malformed = true
			continue
		}
		label, link, ok := parseMarkdownLink(cells[0])
		if !ok {
			malformed = true
			continue
		}
		slug := strings.TrimSuffix(strings.TrimSuffix(link, "/README.md"), ".md")
		if label == "" || slug == "" || label != slug || (link != slug+"/README.md" && link != slug+".md") {
			malformed = true
			continue
		}
		rows = append(rows, lessonIndexRow{slug: slug, link: link, status: cells[1], classifications: cells[2], occurrences: cells[3], lastOccurred: cells[4], enforcement: cells[5]})
	}
	return rows, inLessons && headerSeen && !legacySection, malformed, nil
}

func splitMarkdownRow(line string) []string {
	line = strings.TrimSpace(strings.Trim(line, "|"))
	if line == "" {
		return nil
	}
	parts := strings.Split(line, "|")
	for i := range parts {
		parts[i] = strings.TrimSpace(parts[i])
	}
	return parts
}

func parseMarkdownLink(cell string) (label, link string, ok bool) {
	if !strings.HasPrefix(cell, "[") {
		return "", "", false
	}
	mid := strings.Index(cell, "](")
	if mid <= 1 || !strings.HasSuffix(cell, ")") {
		return "", "", false
	}
	return cell[1:mid], cell[mid+2 : len(cell)-1], true
}

// rewriteLessonIndex regenerates spec/lessons/README.md, preserving the
// prologue before "## Index" and everything from the next H2 heading onward
// (typically "## Open Questions"), replacing only the table body in between.
func rewriteLessonIndex(path string, slugs []string, parsed map[string]*lesson.Lesson) error {
	specRoot := filepath.Dir(filepath.Dir(path))
	return withLessonIndexLock(specRoot, defaultLessonIndexLockDeps(), func() error {
		return rewriteLessonIndexUnlocked(path, slugs, parsed)
	})
}

func rewriteLessonIndexUnlocked(path string, slugs []string, parsed map[string]*lesson.Lesson) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")

	indexStart := -1
	nextH2 := -1
	for i, ln := range lines {
		t := strings.TrimSpace(ln)
		if (t == "## Lessons" || t == "## Index") && indexStart == -1 {
			indexStart = i
		} else if strings.HasPrefix(t, "## ") && indexStart != -1 && nextH2 == -1 {
			nextH2 = i
			break
		}
	}
	if indexStart == -1 {
		return fmt.Errorf("cannot locate ## Lessons heading")
	}

	var tbl strings.Builder
	tbl.WriteString("## Lessons\n\n")
	tbl.WriteString("| Lesson | Status | Classifications | Occurrences | Last Occurred | Enforcement |\n")
	tbl.WriteString("|---|---|---|---:|---|---|\n")

	if len(slugs) == 0 {
		tbl.WriteString("\n_No lessons recorded yet._\n\n")
	} else {
		for _, slug := range slugs {
			row, err := expectedLessonIndexRow(slug, parsed[slug])
			if err != nil {
				return err
			}
			fmt.Fprintf(&tbl, "| [%s](%s) | %s | %s | %s | %s | %s |\n", row.slug, row.link, row.status, row.classifications, row.occurrences, row.lastOccurred, row.enforcement)
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
	return writeLessonIndexAtomic(path, []byte(strings.Join(newLines, "\n")))
}

// UpsertLessonIndexRow mutates only the one canonical row owned by l. It is
// intentionally narrower than lint --fix so scaffold commands have a bounded
// declared write set and cannot repair unrelated historical artifacts.
func UpsertLessonIndexRow(specRoot string, l *lesson.Lesson) error {
	if l == nil {
		return fmt.Errorf("lesson index upsert requires a Lesson")
	}
	return upsertLessonIndexRowWithLock(specRoot, l, defaultLessonIndexLockDeps())
}

func upsertLessonIndexRowWithLock(specRoot string, l *lesson.Lesson, deps lessonIndexLockDeps) error {
	return withLessonIndexLock(specRoot, deps, func() error {
		return upsertLessonIndexRowUnlocked(specRoot, l)
	})
}

func withLessonIndexLock(specRoot string, deps lessonIndexLockDeps, mutate func() error) error {
	return withLessonIndexLockAtProject(lintProjectRoot("", specRoot), specRoot, deps, mutate)
}

func withLessonIndexLockAtProject(projectRoot, _ string, deps lessonIndexLockDeps, mutate func() error) error {
	lockDir := filepath.Join(projectRoot, ".specscore", "locks")
	if err := deps.mkdirAll(lockDir, 0o700); err != nil {
		return fmt.Errorf("creating Lesson index lock directory: %w", err)
	}
	lock := deps.newLock(filepath.Join(lockDir, "lesson-index.lock"))
	if err := lock.Lock(); err != nil {
		return fmt.Errorf("acquiring Lesson index lock: %w", err)
	}
	locked := true
	defer func() {
		if locked {
			_ = lock.Unlock()
		}
	}()
	if err := mutate(); err != nil {
		return err
	}
	if err := lock.Unlock(); err != nil {
		return &lesson.MutationError{Outcome: lesson.MutationUncertain, Err: fmt.Errorf("releasing Lesson index lock: %w", err)}
	}
	locked = false
	return nil
}

func upsertLessonIndexRowUnlocked(specRoot string, l *lesson.Lesson) error {
	if l == nil {
		return fmt.Errorf("lesson index upsert requires a Lesson")
	}
	path := filepath.Join(specRoot, "lessons", "README.md")
	b, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	row, err := expectedLessonIndexRow(l.Slug, l)
	if err != nil {
		return err
	}
	if !l.Canonical {
		return upsertLegacyLessonIndexRow(path, b, row)
	}
	// A repository can still have the flat five-column projection when its
	// first canonical Lesson is created.  The creation command owns the lesson
	// index, so migrate that one declared projection in place rather than
	// rejecting an otherwise valid canonical scaffold or requiring a broad
	// repository-wide --fix.  Discovering the complete Lesson set is necessary:
	// a mixed six-column table must represent every pre-existing flat Lesson as
	// well as the new directory-form one.
	if strings.Contains(string(b), "| Lesson | Status | Recurred | Date | Owner |") {
		lessons, err := lesson.Discover(filepath.Join(specRoot, "lessons"))
		if err != nil {
			return fmt.Errorf("discovering Lessons for canonical index migration: %w", err)
		}
		parsed := make(map[string]*lesson.Lesson, len(lessons))
		slugs := make([]string, 0, len(lessons))
		for _, found := range lessons {
			parsed[found.Slug] = found
			slugs = append(slugs, found.Slug)
		}
		return rewriteLessonIndexUnlocked(path, slugs, parsed)
	}
	line := fmt.Sprintf("| [%s](%s) | %s | %s | %s | %s | %s |", row.slug, row.link, row.status, row.classifications, row.occurrences, row.lastOccurred, row.enforcement)
	lines := strings.Split(string(b), "\n")
	header := -1
	separator := -1
	found := -1
	for i, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "| Lesson | Status | Classifications | Occurrences | Last Occurred | Enforcement |" {
			header = i
			continue
		}
		if header >= 0 && separator < 0 && (trimmed == "|---|---|---|---:|---|---|" || trimmed == "|---|---|---|---|---|---|") {
			separator = i
			continue
		}
		if lessonIndexRowOwnsSlug(trimmed, l.Slug) {
			if found >= 0 {
				return fmt.Errorf("lessons index contains duplicate row for %q", l.Slug)
			}
			found = i
		}
	}
	if header < 0 || separator != header+1 {
		return fmt.Errorf("lessons index lacks the canonical six-column table")
	}
	if found >= 0 {
		lines[found] = line
	} else {
		insert := separator + 1
		for insert < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[insert]), "| [") {
			insert++
		}
		lines = append(lines, "")
		copy(lines[insert+1:], lines[insert:])
		lines[insert] = line
	}
	return writeLessonIndexAtomic(path, []byte(strings.Join(lines, "\n")))
}

func upsertLegacyLessonIndexRow(path string, b []byte, row lessonIndexRow) error {
	lines := strings.Split(string(b), "\n")
	header, separator, found := -1, -1, -1
	line := fmt.Sprintf("| [%s](%s) | %s | %s | %s | %s |", row.slug, row.link, row.status, row.occurrences, "", "")
	for i, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "| Lesson | Status | Recurred | Date | Owner |" {
			header = i
			continue
		}
		if header >= 0 && separator < 0 && trimmed == "|---|---|---|---|---|" {
			separator = i
			continue
		}
		if lessonIndexRowOwnsSlug(trimmed, row.slug) {
			if found >= 0 {
				return fmt.Errorf("lessons index contains duplicate row for %q", row.slug)
			}
			found = i
		}
	}
	if header < 0 || separator != header+1 {
		return fmt.Errorf("legacy lessons index lacks the five-column compatibility table")
	}
	if found >= 0 {
		old := splitMarkdownRow(lines[found])
		if len(old) == 5 {
			line = fmt.Sprintf("| [%s](%s) | %s | %s | %s | %s |", row.slug, row.link, row.status, row.occurrences, old[3], old[4])
		}
		lines[found] = line
	} else {
		insert := separator + 1
		lines = append(lines, "")
		copy(lines[insert+1:], lines[insert:])
		lines[insert] = line
	}
	return writeLessonIndexAtomic(path, []byte(strings.Join(lines, "\n")))
}

func writeLessonIndexAtomic(path string, data []byte) error {
	return writeLessonIndexAtomicWithOps(path, data, defaultLessonIndexWriteOps())
}

func writeLessonIndexAtomicWithOps(path string, data []byte, ops lessonIndexWriteOps) error {
	info, err := ops.stat(path)
	if err != nil {
		return err
	}
	tmp, err := ops.createTemp(filepath.Dir(path), ".lesson-index-")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	defer func() { _ = ops.remove(tmpPath) }()
	if err := tmp.Chmod(info.Mode().Perm()); err != nil {
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
	if err := ops.rename(tmpPath, path); err != nil {
		return err
	}
	dir, err := ops.open(filepath.Dir(path))
	if err != nil {
		return &lesson.MutationError{Outcome: lesson.MutationUncertain, Err: fmt.Errorf("opening Lesson index directory after publication: %w", err)}
	}
	if err := dir.Sync(); err != nil {
		_ = dir.Close()
		return &lesson.MutationError{Outcome: lesson.MutationUncertain, Err: fmt.Errorf("syncing Lesson index directory after publication: %w", err)}
	}
	if err := dir.Close(); err != nil {
		return &lesson.MutationError{Outcome: lesson.MutationUncertain, Err: fmt.Errorf("closing Lesson index directory after publication: %w", err)}
	}
	return nil
}

func firstMarkdownCell(line string) string {
	parts := splitMarkdownRow(line)
	if len(parts) == 0 {
		return ""
	}
	return parts[0]
}

// lessonIndexRowOwnsSlug reports whether line is the index row for slug. Both
// link shapes are the same row because the index holds one row per slug, not
// one per layout: a migrating Lesson is re-linked in place from <slug>.md to
// <slug>/README.md. This must stay identical to the slug readLessonIndexRows
// derives, or an upsert appends a row that L-003/L-004 then reads as a
// duplicate.
func lessonIndexRowOwnsSlug(line, slug string) bool {
	label, link, ok := parseMarkdownLink(firstMarkdownCell(line))
	return ok && label == slug && (link == slug+"/README.md" || link == slug+".md")
}
