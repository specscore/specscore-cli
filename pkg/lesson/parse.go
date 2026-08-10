// Package lesson parses and scaffolds single-file Lesson artifacts at
// spec/lessons/<slug>.md.
//
// A Lesson records a gap in process — a missing check, gate, convention, or
// review step that let a defect ship unnoticed — and climbs a three-rung
// enforcement ladder (Recorded -> Stated -> Enforced) as the gap is closed by
// an increasingly binding mechanism. It is deliberately the flattest kind in
// the spec tree: no hierarchy, no source-Feature dependency, a single status
// line, and a free-form prose body whose only structural requirement is that
// four sections exist (lint checks presence, never content) — Incident,
// Process gap, Check, Enforcement — matching the append-only lessons-learned
// convention this artifact kind formalizes.
package lesson

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// FormatURL is the canonical spec URL for the Lesson document type. It is
// carried verbatim in both the frontmatter `format:` field and the
// adherence-footer line, per the artifact-frontmatter-convention.
const FormatURL = "https://specscore.md/lesson-specification"

// RequiredSections is the closed, ordered set of H2 section headings every
// Lesson body MUST declare. Lint checks presence ONLY — never content,
// length, or wording — so a section holding nothing but a `<!-- TODO -->`
// prompt is lint-clean. Process gap and Enforcement are the load-bearing
// pair: a Lesson describing an Incident but naming no gap in the process is
// the useless entry the Process-gap requirement exists to refuse, and a
// Lesson proposing no enforcement path is the other half of that same
// refusal.
var RequiredSections = []string{"Incident", "Process gap", "Check", "Enforcement"}

// RequiredSectionsFor preserves legacy compatibility while exposing the
// canonical compact Lesson's actual contract to readers.
func RequiredSectionsFor(l *Lesson) []string {
	if l != nil && l.Canonical {
		return []string{"Lesson", "Process Gap", "Tracking", "Enforcement", "Open Questions"}
	}
	return RequiredSections
}

func (l *Lesson) MissingRequiredSectionsForLayout() []string {
	var missing []string
	for _, req := range RequiredSectionsFor(l) {
		if !l.HasSection(req) {
			missing = append(missing, req)
		}
	}
	return missing
}

// Lesson is a parsed single-file Lesson artifact.
type Lesson struct {
	Path string // absolute path on disk
	Slug string // filename without `.md`
	// Canonical is true when Path is spec/lessons/<slug>/README.md.
	Canonical bool
	// OccurrencesDir is non-empty only for canonical directory lessons.
	OccurrencesDir string

	HasLessonTitle bool   // first H1 line was `# Lesson: <title>`
	TitleLine      int    // 1-based line number of the title (0 when absent)
	Title          string // the `<title>` portion after `# Lesson: `

	Status     string // value of `**Status:**` (empty when missing)
	StatusLine int    // 1-based line of the field; 0 when absent

	Date     string // value of `**Date:**` (empty when missing)
	DateLine int    // 1-based line of the field; 0 when absent

	Owner     string // value of `**Owner:**` (empty when missing)
	OwnerLine int    // 1-based line of the field; 0 when absent

	Recurred      int    // parsed `**Recurred:**` count; 0 when absent or unparsable
	RecurredRaw   string // raw value as written
	RecurredLine  int    // 1-based line of the field; 0 when absent
	RecurredValid bool   // true when RecurredRaw parsed cleanly as a non-negative integer

	SupersededBy     string // value of `**Superseded By:**` (empty when missing)
	SupersededByLine int    // 1-based line of the field; 0 when absent

	// SectionLines maps a present H2 section title to its 1-based heading
	// line. Only sections found in the body appear here; callers check
	// RequiredSections membership against this map's keys to find gaps.
	SectionLines map[string]int
}

// HasSection reports whether title is present in the parsed body as an H2
// heading.
func (l *Lesson) HasSection(title string) bool {
	_, ok := l.SectionLines[title]
	return ok
}

// MissingRequiredSections returns the subset of RequiredSections absent from
// the parsed body, in RequiredSections order. Returns nil when none are
// missing.
func (l *Lesson) MissingRequiredSections() []string {
	var missing []string
	for _, req := range RequiredSections {
		if !l.HasSection(req) {
			missing = append(missing, req)
		}
	}
	return missing
}

// IsSingleFileLessonPath reports whether filePath looks like a single-file
// Lesson candidate location — directly under lessonsDir, with a `.md`
// extension, and not named README.md (the index file). It does NOT read the
// file; callers still must validate the title prefix via Parse().
func IsSingleFileLessonPath(lessonsDir, filePath string) bool {
	if !strings.HasSuffix(filePath, ".md") {
		return false
	}
	if strings.HasSuffix(filePath, string(os.PathSeparator)+"README.md") || filePath == "README.md" {
		return false
	}
	rel, err := filepathRel(lessonsDir, filePath)
	if err != nil {
		return false
	}
	if strings.ContainsRune(rel, os.PathSeparator) {
		return false
	}
	return true
}

// boldFieldRe matches `**Name:** value`. The bold prefix MUST be at column 0
// after trimming so this stays unambiguous.
var boldFieldRe = regexp.MustCompile(`^\*\*([^*]+?):\*\*\s*(.*)$`)

func matchBoldField(line string) (name, val string, ok bool) {
	m := boldFieldRe.FindStringSubmatch(line)
	if m == nil {
		return "", "", false
	}
	return strings.TrimSpace(m[1]), strings.TrimSpace(m[2]), true
}

// Parse reads a candidate Lesson file. It returns a populated Lesson even
// when the file is not actually a Lesson (HasLessonTitle == false in that
// case) so callers can distinguish "not a Lesson" from "malformed Lesson".
func Parse(path string) (*Lesson, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	slug := slugFromPath(path)
	canonical := filepath.Base(path) == "README.md" && filepath.Base(filepath.Dir(filepath.Dir(path))) == "lessons"
	if canonical {
		slug = filepath.Base(filepath.Dir(path))
	}
	l := &Lesson{Path: path, Slug: slug, Canonical: canonical, SectionLines: map[string]int{}}
	if canonical {
		l.OccurrencesDir = filepath.Join(filepath.Dir(path), "occurrences")
	}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	var lines []string
	for scanner.Scan() {
		lines = append(lines, scanner.Text())
	}

	for i, raw := range lines {
		trimmed := strings.TrimSpace(raw)
		if rest, ok := strings.CutPrefix(trimmed, "# "); !l.HasLessonTitle && ok {
			if title, ok := strings.CutPrefix(rest, "Lesson:"); ok {
				l.HasLessonTitle = true
				l.TitleLine = i + 1
				l.Title = strings.TrimSpace(title)
			}
			continue
		}
		if title, ok := strings.CutPrefix(trimmed, "## "); ok {
			t := strings.TrimSpace(title)
			if _, seen := l.SectionLines[t]; !seen {
				l.SectionLines[t] = i + 1
			}
			continue
		}
		if name, val, ok := matchBoldField(trimmed); ok {
			switch name {
			case "Status":
				l.Status = val
				l.StatusLine = i + 1
			case "Date":
				l.Date = val
				l.DateLine = i + 1
			case "Owner":
				l.Owner = val
				l.OwnerLine = i + 1
			case "Recurred":
				l.RecurredRaw = val
				l.RecurredLine = i + 1
				if n, err := strconv.Atoi(val); err == nil && n >= 0 {
					l.Recurred = n
					l.RecurredValid = true
				}
			case "Superseded By":
				l.SupersededBy = val
				l.SupersededByLine = i + 1
			}
		}
	}

	if !l.HasLessonTitle {
		return l, nil
	}

	return l, nil
}

// slugFromPath returns the filename without `.md`.
func slugFromPath(path string) string {
	base := path
	for i := len(path) - 1; i >= 0; i-- {
		if path[i] == os.PathSeparator || path[i] == '/' {
			base = path[i+1:]
			break
		}
	}
	return strings.TrimSuffix(base, ".md")
}

// filepathRel is a tiny inlined variant of filepath.Rel that returns an error
// when filePath is not under lessonsDir. Kept inline (mirroring pkg/plan) to
// avoid an extra import in a tight helper.
func filepathRel(lessonsDir, filePath string) (string, error) {
	if !strings.HasPrefix(filePath, lessonsDir) {
		return "", fmt.Errorf("not under lessons dir")
	}
	rel := strings.TrimPrefix(filePath, lessonsDir)
	rel = strings.TrimPrefix(rel, string(os.PathSeparator))
	if rel == "" {
		return "", fmt.Errorf("path is lessons dir itself")
	}
	return rel, nil
}
