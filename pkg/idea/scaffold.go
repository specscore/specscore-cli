package idea

import (
	"fmt"
	"strings"
	"time"
)

// FormatURL is the canonical spec URL for the Idea document type, emitted in
// the frontmatter `format:` field per the artifact-frontmatter-convention.
const FormatURL = "https://specscore.md/idea-specification"

// ScaffoldOptions controls the content emitted by Scaffold. Any field left
// empty keeps the section's default HTML-comment prompt.
type ScaffoldOptions struct {
	Slug string
	// Title is the human-readable name after `# Idea: ` or `# Proposal: `.
	// Defaults to a title-cased version of the slug.
	Title string
	Owner string
	// Date in ISO-8601 (YYYY-MM-DD). Defaults to today's UTC date.
	Date string
	// Status defaults to "Draft".
	Status string

	// Type is the idea type: "feature-request" (default) or "change-request".
	// When "change-request", the title prefix becomes "# Proposal: " and the
	// Type/Targets fields are emitted in the header.
	Type string
	// Targets is the feature slug this change-request targets. Only meaningful
	// when Type is "change-request".
	Targets string
	// Phase is an optional lifecycle phase value pre-populated in the header.
	Phase string

	// Section content. Empty strings leave the default prompt in place.
	HMW                  string // Problem Statement
	Context              string
	RecommendedDirection string
	Alternatives         []string // Each element is a bullet for Alternatives Considered.
	MVP                  string
	// NotDoing is a list of exclusions. When empty, a lint-clean default
	// list is emitted so the Not Doing section passes idea-not-doing-non-empty.
	NotDoing []string
	// Assumptions is an optional list of assumption-table rows.
	// Each row is {Tier, Assumption, HowToValidate}. When empty, a
	// lint-clean default table with one Must-be-true row is emitted.
	Assumptions [][3]string
	// SpecScore Integration overrides.
	NewFeatures  string
	Existing     string
	Dependencies string
	// OpenQuestions bullets (optional).
	OpenQuestions []string
}

// defaultOwner returns a reasonable fallback owner.
func defaultOwner() string {
	return "unknown"
}

// titleCaseFromSlug turns "payment-fraud-signals" into "Payment Fraud Signals".
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

// Scaffold returns a lint-clean Idea file body for the given options.
// Every required section is emitted either with an HTML-comment prompt
// (the default) or with the supplied content.
func Scaffold(opts ScaffoldOptions) ([]byte, error) {
	if err := ValidateSlug(opts.Slug); err != nil {
		return nil, err
	}

	title := opts.Title
	if strings.TrimSpace(title) == "" {
		title = titleCaseFromSlug(opts.Slug)
	}
	owner := strings.TrimSpace(opts.Owner)
	if owner == "" {
		owner = defaultOwner()
	}
	date := strings.TrimSpace(opts.Date)
	if date == "" {
		date = time.Now().UTC().Format("2006-01-02")
	}
	status := strings.TrimSpace(opts.Status)
	if status == "" {
		status = "Draft"
	}

	// Each section body: supplied content or a prompt.
	hmw := strings.TrimSpace(opts.HMW)
	if hmw == "" {
		hmw = `<!-- One "How Might We…" sentence framing the problem. -->`
	}

	ctx := strings.TrimSpace(opts.Context)
	if ctx == "" {
		ctx = `<!-- Triggering observation, related specs, prior art. -->`
	}

	direction := strings.TrimSpace(opts.RecommendedDirection)
	if direction == "" {
		direction = `<!-- 2–3 paragraphs: what and why, over the alternatives. -->`
	}

	var alts string
	if len(opts.Alternatives) > 0 {
		var b strings.Builder
		for _, a := range opts.Alternatives {
			a = strings.TrimSpace(a)
			if a == "" {
				continue
			}
			fmt.Fprintf(&b, "- %s\n", a)
		}
		alts = strings.TrimRight(b.String(), "\n")
	} else {
		alts = `<!-- 2–3 directions that lost, and why each lost. -->`
	}

	mvp := strings.TrimSpace(opts.MVP)
	if mvp == "" {
		mvp = `<!-- The single job the MVP nails. Timeboxed, not feature-listed. -->`
	}

	// Not Doing must always be a non-empty bullet list (REQ: not-doing-non-empty).
	notDoingItems := opts.NotDoing
	if len(notDoingItems) == 0 {
		notDoingItems = []string{
			"<!-- placeholder exclusion -->placeholder — fill in during authoring",
		}
	}
	var nd strings.Builder
	for _, item := range notDoingItems {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		fmt.Fprintf(&nd, "- %s\n", item)
	}
	notDoing := strings.TrimRight(nd.String(), "\n")

	// Assumptions table — at least one Must-be-true row with non-empty content
	// (REQ: must-be-true-present).
	assumptions := opts.Assumptions
	if len(assumptions) == 0 {
		assumptions = [][3]string{
			{"Must-be-true", "placeholder dealbreaker assumption", "describe how to validate"},
			{"Should-be-true", "…", "…"},
			{"Might-be-true", "…", "…"},
		}
	}
	var tab strings.Builder
	tab.WriteString("| Tier | Assumption | How to validate |\n")
	tab.WriteString("|------|------------|-----------------|\n")
	for _, row := range assumptions {
		fmt.Fprintf(&tab, "| %s | %s | %s |\n", row[0], row[1], row[2])
	}

	newFeats := strings.TrimSpace(opts.NewFeatures)
	if newFeats == "" {
		newFeats = `TBD at design time`
	}
	existing := strings.TrimSpace(opts.Existing)
	if existing == "" {
		existing = `none`
	}
	deps := strings.TrimSpace(opts.Dependencies)
	if deps == "" {
		deps = `none`
	}

	var oq string
	if len(opts.OpenQuestions) > 0 {
		var b strings.Builder
		for _, q := range opts.OpenQuestions {
			q = strings.TrimSpace(q)
			if q == "" {
				continue
			}
			fmt.Fprintf(&b, "- %s\n", q)
		}
		oq = strings.TrimRight(b.String(), "\n")
	} else {
		oq = "None at this time."
	}

	// Determine title prefix based on Type.
	isChangeRequest := strings.TrimSpace(opts.Type) == "change-request"
	titlePrefix := "Idea"
	if isChangeRequest {
		titlePrefix = "Proposal"
	}

	var out strings.Builder
	// artifact-frontmatter-convention REQ:scaffold-and-change-status — an Idea is
	// a status-bearing type, so emit the `format:`/`status:` frontmatter on
	// creation (status: mirrors the body `**Status:**`). Proposals
	// (change-requests) are a different document type and are left untouched
	// here.
	if !isChangeRequest {
		fmt.Fprintf(&out, "---\nformat: %s\nstatus: %s\n---\n\n", FormatURL, status)
	}
	fmt.Fprintf(&out, "# %s: %s\n\n", titlePrefix, title)
	fmt.Fprintf(&out, "**Status:** %s\n", status)
	if isChangeRequest {
		fmt.Fprintf(&out, "**Type:** %s\n", strings.TrimSpace(opts.Type))
		targets := strings.TrimSpace(opts.Targets)
		if targets == "" {
			targets = "—"
		}
		fmt.Fprintf(&out, "**Targets:** %s\n", targets)
	}
	if phase := strings.TrimSpace(opts.Phase); phase != "" {
		fmt.Fprintf(&out, "**Phase:** %s\n", phase)
	}
	fmt.Fprintf(&out, "**Date:** %s\n", date)
	fmt.Fprintf(&out, "**Owner:** %s\n", owner)
	out.WriteString("**Promotes To:** —\n")
	out.WriteString("**Supersedes:** —\n")
	out.WriteString("**Related Ideas:** —\n\n")

	fmt.Fprintf(&out, "## Problem Statement\n\n%s\n\n", hmw)
	fmt.Fprintf(&out, "## Context\n\n%s\n\n", ctx)
	fmt.Fprintf(&out, "## Recommended Direction\n\n%s\n\n", direction)
	fmt.Fprintf(&out, "## Alternatives Considered\n\n%s\n\n", alts)
	fmt.Fprintf(&out, "## MVP Scope\n\n%s\n\n", mvp)
	fmt.Fprintf(&out, "## Not Doing (and Why)\n\n%s\n\n", notDoing)
	fmt.Fprintf(&out, "## Key Assumptions to Validate\n\n%s\n", tab.String())
	out.WriteString("\n")
	fmt.Fprintf(&out, "## SpecScore Integration\n\n")
	fmt.Fprintf(&out, "- **New Features this would create:** %s\n", newFeats)
	fmt.Fprintf(&out, "- **Existing Features affected:** %s\n", existing)
	fmt.Fprintf(&out, "- **Dependencies:** %s\n\n", deps)
	fmt.Fprintf(&out, "## Open Questions\n\n%s\n", oq)

	return []byte(out.String()), nil
}
