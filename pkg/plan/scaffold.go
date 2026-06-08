package plan

import (
	"fmt"
	"regexp"
	"strings"
	"time"
)

// FormatURL is the canonical spec URL for the Plan document type. It is carried
// verbatim in both the frontmatter `format:` field and the adherence-footer
// line, per the artifact-frontmatter-convention.
const FormatURL = "https://specscore.md/plan-specification"

var planSlugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidateSlug returns nil when slug is a lowercase, hyphen-separated, URL-safe
// identifier with no `/` (cli/plan/new#req:slug-format).
func ValidateSlug(slug string) error {
	if slug == "" {
		return fmt.Errorf("slug must not be empty")
	}
	if !planSlugRe.MatchString(slug) {
		return fmt.Errorf("slug %q must be lowercase, hyphen-separated, and URL-safe (no '/')", slug)
	}
	return nil
}

// ScaffoldOptions controls the flat Plan file Scaffold emits.
type ScaffoldOptions struct {
	Slug  string
	Title string // defaults to a title-cased slug
	Owner string // defaults to "unknown"
	Date  string // ISO-8601 (YYYY-MM-DD); defaults to today's UTC date
	// At most one of SourceFeature / SourceIdea may be set: a plan decomposes
	// one Feature, one Idea, or no source at all (source-less, emitted as
	// `**Source:** none`) (cli/plan/new#req:source-optional).
	SourceFeature string
	SourceIdea    string
	// Parent, when non-empty, records the master plan this plan is a sub-plan of
	// (cli/plan/new#req:parent-ref-optional). It is emitted verbatim as a
	// `**Parent:** <value>` header line after `**Supersedes:**`; resolution is
	// deferred to lint (P-005). A same-repo slug or a cross-repo
	// `<repo-slug>:<plan-slug>` soft reference.
	Parent string
}

// titleCaseFromSlug turns "add-batch-mode" into "Add Batch Mode".
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

// Scaffold returns a lint-clean flat Plan file body: the
// artifact-frontmatter-convention frontmatter (`format:` + `status:` mirroring
// the body `**Status:** Draft`), the `# Plan:` title, the body-metadata header,
// the four required sections with HTML-comment prompts, and the adherence
// footer whose URL agrees with `format:`.
func Scaffold(opts ScaffoldOptions) ([]byte, error) {
	if err := ValidateSlug(opts.Slug); err != nil {
		return nil, err
	}
	if opts.SourceFeature != "" && opts.SourceIdea != "" {
		return nil, fmt.Errorf("at most one of SourceFeature or SourceIdea may be set")
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

	// One source line, by mode: a Feature slug, an Idea slug, or `none`
	// (source-less). Exactly one is emitted; at most one input is set.
	sourceLine := "**Source:** none"
	switch {
	case opts.SourceFeature != "":
		sourceLine = "**Source Feature:** " + opts.SourceFeature
	case opts.SourceIdea != "":
		sourceLine = "**Source:** idea:" + opts.SourceIdea
	}

	var b strings.Builder
	fmt.Fprintf(&b, "---\nformat: %s\nstatus: Draft\n---\n\n", FormatURL)
	fmt.Fprintf(&b, "# Plan: %s\n\n", title)
	b.WriteString("**Status:** Draft\n")
	b.WriteString(sourceLine + "\n")
	fmt.Fprintf(&b, "**Date:** %s\n", date)
	fmt.Fprintf(&b, "**Owner:** %s\n", owner)
	b.WriteString("**Supersedes:** —\n")
	if parent := strings.TrimSpace(opts.Parent); parent != "" {
		fmt.Fprintf(&b, "**Parent:** %s\n", parent)
	}
	b.WriteString("\n")
	b.WriteString("## Summary\n\n<!-- TODO: 1–3 sentences — what this plan covers and how it decomposes the source. -->\n\n")
	b.WriteString("## Approach\n\n<!-- TODO: ≤1 paragraph — what was grouped, what was deferred and why. -->\n\n")
	b.WriteString("## Tasks\n\n<!-- TODO: add `### Task N:` blocks, each with a **Verifies:** line naming a source-Feature AC (feature-slug#ac:<ac-slug>). -->\n\n")
	b.WriteString("## Open Questions\n\nNone at this time.\n\n")
	fmt.Fprintf(&b, "---\n*This document follows the %s*\n", FormatURL)

	return []byte(b.String()), nil
}
