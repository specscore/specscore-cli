package lesson

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

var lessonSlugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidateSlug returns nil when slug is a lowercase, hyphen-separated,
// URL-safe identifier with no `/`.
func ValidateSlug(slug string) error {
	if slug == "" {
		return fmt.Errorf("slug must not be empty")
	}
	if !lessonSlugRe.MatchString(slug) {
		return fmt.Errorf("slug %q must be lowercase, hyphen-separated, and URL-safe (no '/')", slug)
	}
	return nil
}

// ScaffoldOptions controls the flat Lesson file Scaffold emits.
type ScaffoldOptions struct {
	Slug  string
	Title string // defaults to a title-cased slug
	Owner string // defaults to "unknown"
	Date  string // ISO-8601 (YYYY-MM-DD); defaults to today's UTC date
}

// titleCaseFromSlug turns "kinder-fake-hides-bug" into "Kinder Fake Hides Bug".
func titleCaseFromSlug(slug string) string {
	parts := strings.Split(slug, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}

// Scaffold returns a lint-clean flat Lesson file body: the
// artifact-frontmatter-convention frontmatter (`format:` + `status:`
// mirroring the body `**Status:** Recorded`), the `# Lesson:` title, the
// body-metadata header, the four required sections (Incident, Process gap,
// Check, Enforcement) with HTML-comment prompts, and the adherence footer
// whose URL agrees with `format:`.
//
// A freshly scaffolded Lesson is immediately lint-clean: every required
// section exists (lint checks presence only, never content), so recording a
// lesson is a single command with no required flags beyond the slug — the
// friction-near-zero design this artifact kind depends on for agents to
// actually use it under time pressure.
func Scaffold(opts ScaffoldOptions) ([]byte, error) {
	if err := ValidateSlug(opts.Slug); err != nil {
		return nil, err
	}

	title := strings.TrimSpace(opts.Title)
	if title == "" {
		title = titleCaseFromSlug(opts.Slug)
	}
	owner := strings.TrimSpace(opts.Owner)
	if owner == "" {
		owner = "unknown"
	}
	date := strings.TrimSpace(opts.Date)
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}

	var b strings.Builder
	fmt.Fprintf(&b, "---\nformat: %s\nstatus: Recorded\n---\n\n", FormatURL)
	fmt.Fprintf(&b, "# Lesson: %s\n\n", title)
	b.WriteString("**Status:** Recorded\n")
	fmt.Fprintf(&b, "**Date:** %s\n", date)
	fmt.Fprintf(&b, "**Owner:** %s\n", owner)
	b.WriteString("**Recurred:** 0\n")
	b.WriteString("\n")
	b.WriteString("## Incident\n\n<!-- TODO: what happened, briefly. Evidence only, not the point of the entry. -->\n\n")
	b.WriteString("## Process gap\n\n<!-- TODO: which check, gate, convention, or review step was missing, ambiguous, or existed but never ran. THIS is the lesson. -->\n\n")
	b.WriteString("## Check\n\n<!-- TODO: the concrete check or balance that would catch it next time. -->\n\n")
	b.WriteString("## Enforcement\n\n<!-- TODO: the tier (Recorded / Stated / Enforced) and where it binds. -->\n\n")
	fmt.Fprintf(&b, "---\n*This document follows the %s*\n", FormatURL)

	return []byte(b.String()), nil
}

// ScaffoldCanonical returns the compact directory-form Lesson README. The
// incident diary lives in immutable child occurrences, so the README contains
// only the durable lesson and its enforceable control.
func ScaffoldCanonical(opts ScaffoldOptions, classifications []string) ([]byte, error) {
	if err := ValidateSlug(opts.Slug); err != nil {
		return nil, err
	}
	title := strings.TrimSpace(opts.Title)
	if title == "" {
		title = titleCaseFromSlug(opts.Slug)
	}
	owner := strings.TrimSpace(opts.Owner)
	if owner == "" {
		owner = "unknown"
	}
	date := strings.TrimSpace(opts.Date)
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}
	if len(classifications) == 0 {
		classifications = []string{"process"}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "---\nformat: %s\nstatus: Recorded\n---\n\n", FormatURL)
	fmt.Fprintf(&b, "# Lesson: %s\n\n", title)
	fmt.Fprintf(&b, "**Status:** Recorded\n**Date:** %s\n**Owner:** %s\n", date, owner)
	fmt.Fprintf(&b, "**Classifications:** %s\n", strings.Join(classifications, ", "))
	b.WriteString("**Legacy Provenance:** —\n**Duplicate Of:** —\n**Supersedes:** —\n**Superseded By:** —\n\n")
	b.WriteString("## Lesson\n\n<!-- TODO: the durable rule. -->\n\n## Process Gap\n\n<!-- TODO: what should have caught this, and why it did not. -->\n\n## Tracking\n\n- **Occurrence store:** `occurrences/`\n- **Recurrence metadata:** derived from child JSON; never hand-maintained here.\n- **Occurrence schema:** `https://specscore.md/new/lesson-occurrence.schema.json`\n\n## Enforcement\n\n**Control:** —\n**Verification:** —\n**Evidence:** —\n\n## Open Questions\n\nNone at this time.\n\n")
	fmt.Fprintf(&b, "---\n*This document follows the %s*\n", FormatURL)
	return []byte(b.String()), nil
}
