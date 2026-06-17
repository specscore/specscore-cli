package decision

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type ScaffoldOptions struct {
	Slug       string
	Title      string
	Owner      string
	Date       string
	Tags       string
	SourceIdea string
	Supersedes string
}

var slugRe = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// FormatURL is the canonical spec URL for the Decision document type, emitted in
// both the frontmatter `format:` field and the adherence-footer line.
const FormatURL = "https://specscore.md/decision-specification"

func ValidateSlug(slug string) error {
	if slug == "" {
		return fmt.Errorf("slug must not be empty")
	}
	if !slugRe.MatchString(slug) {
		return fmt.Errorf("slug %q must be lowercase, hyphen-separated, and URL-safe", slug)
	}
	return nil
}

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

// NextNumber scans spec/decisions/ and spec/decisions/archived/ for the
// highest NNNN prefix and returns max + 1. Returns 1 if no decisions exist.
func NextNumber(specRoot string) (int, error) {
	decisionsDir := filepath.Join(specRoot, "decisions")
	highest := 0

	re := regexp.MustCompile(`^(\d{4})-`)

	for _, dir := range []string{decisionsDir, filepath.Join(decisionsDir, "archived")} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			m := re.FindStringSubmatch(e.Name())
			if m == nil {
				continue
			}
			n, _ := strconv.Atoi(m[1])
			if n > highest {
				highest = n
			}
		}
	}

	return highest + 1, nil
}

// AllNumbers returns all decision numbers found in spec/decisions/ and archived/,
// sorted ascending.
func AllNumbers(specRoot string) []int {
	decisionsDir := filepath.Join(specRoot, "decisions")
	re := regexp.MustCompile(`^(\d{4})-`)
	var nums []int

	for _, dir := range []string{decisionsDir, filepath.Join(decisionsDir, "archived")} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			m := re.FindStringSubmatch(e.Name())
			if m == nil {
				continue
			}
			n, _ := strconv.Atoi(m[1])
			nums = append(nums, n)
		}
	}

	sort.Ints(nums)
	return nums
}

// Scaffold returns a lint-clean Decision file body.
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

	tags := strings.TrimSpace(opts.Tags)
	if tags == "" {
		tags = "—"
	}

	sourceIdea := strings.TrimSpace(opts.SourceIdea)
	if sourceIdea == "" {
		sourceIdea = "—"
	}

	supersedes := strings.TrimSpace(opts.Supersedes)
	if supersedes == "" {
		supersedes = "—"
	}

	var out strings.Builder
	// artifact-frontmatter-convention REQ:scaffold-and-change-status — a Decision
	// is a status-bearing type, so emit format:/status: frontmatter (status:
	// mirrors the body **Status:** Draft below).
	out.WriteString("---\nformat: " + FormatURL + "\nstatus: Draft\n---\n\n")
	fmt.Fprintf(&out, "# Decision: %s\n\n", title)
	fmt.Fprintf(&out, "**Status:** Draft\n")
	fmt.Fprintf(&out, "**Date:** %s\n", date)
	fmt.Fprintf(&out, "**Owner:** %s\n", owner)
	fmt.Fprintf(&out, "**Tags:** %s\n", tags)
	fmt.Fprintf(&out, "**Source Idea:** %s\n", sourceIdea)
	fmt.Fprintf(&out, "**Supersedes:** %s\n", supersedes)
	fmt.Fprintf(&out, "**Superseded By:** —\n")

	out.WriteString("\n## Context\n\n")
	out.WriteString("<!-- What problem forced a choice. What was known at the time. -->\n")

	out.WriteString("\n## Decision\n\n")
	out.WriteString("<!-- The chosen option, stated as a short declarative sentence or two. -->\n")

	out.WriteString("\n## Rationale\n\n")
	out.WriteString("<!-- Why this option was chosen. 1–3 paragraphs. -->\n")

	out.WriteString("\n## Declined Alternatives\n\n")
	out.WriteString("### <!-- Alternative name -->\n\n")
	out.WriteString("<!-- One-line pitch. Why it lost. -->\n")

	out.WriteString("\n## Consequences at Decision Time\n\n")
	out.WriteString("<!-- What we expected to follow from this choice — positive and negative. -->\n")

	out.WriteString("\n## Observed Consequences\n\n")
	out.WriteString("None observed yet.\n")

	out.WriteString("\n## Affected Features\n\n")
	out.WriteString("None at this time.\n")

	out.WriteString("\n---\n")
	out.WriteString("*This document follows the " + FormatURL + "*\n")

	return []byte(out.String()), nil
}
