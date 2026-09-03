package rule

import (
	"fmt"
	"strings"
	"time"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// Options carries everything both forms of a rule need. Only Slug is
// mandatory: a rule recorded under time pressure with nothing but a slug is
// still a lint-clean inline row that a later `rule update` can sharpen, which
// is the whole reason the kind exists.
type Options struct {
	Slug        string
	Title       string   // detail only; defaults to a title-cased slug
	Owner       string   // detail only; defaults to "unknown"
	Date        string   // detail only; ISO-8601, defaults to today's UTC date
	Status      string   // defaults to Draft
	Statement   string   // defaults to a TODO prompt
	Scopes      []string // defaults to ["fleet"]
	Enforcement string   // defaults to Stated
	Control     string   // defaults to the em-dash sentinel
	Sources     []string // defaults to the em-dash sentinel

	// Detail-only fields.
	Why          string   // defaults to a TODO prompt
	Exceptions   string   // defaults to "none"
	Supersedes   string   // defaults to the em-dash sentinel
	Instructions string   // defaults to a TODO prompt
	Compliant    string   // a worked compliant example
	Violation    string   // a worked violating example
	Skills       []string // `skill:<name>` references to record in Instructions
}

const (
	statementPlaceholder    = "TODO: one normative sentence — MUST or NEVER, plain words."
	whyPlaceholder          = "TODO: the reason, plus one concrete use case that shows the cost of breaking it."
	instructionsPlaceholder = "TODO: what an agent should actually do to comply, step by step."
	compliantPlaceholder    = "TODO: a short worked example that satisfies this rule."
	violationPlaceholder    = "TODO: the closest thing that looks fine and still breaks it."
)

// Normalize fills every unset field with its default and validates the closed
// vocabularies. It is shared by the row builder and the detail scaffolder, so
// the two forms of one rule cannot drift apart at creation time.
func (o *Options) Normalize() error {
	if err := ValidateSlug(o.Slug); err != nil {
		return err
	}
	if strings.TrimSpace(o.Title) == "" {
		o.Title = TitleCaseFromSlug(o.Slug)
	}
	o.Title = collapseWhitespace(o.Title)
	if strings.TrimSpace(o.Owner) == "" {
		o.Owner = "unknown"
	}
	o.Owner = strings.TrimSpace(o.Owner)
	if strings.TrimSpace(o.Date) == "" {
		o.Date = time.Now().UTC().Format("2006-01-02")
	}
	o.Date = strings.TrimSpace(o.Date)

	if strings.TrimSpace(o.Status) == "" {
		o.Status = Statuses[0]
	}
	status, ok := ParseStatus(o.Status)
	if !ok {
		return fmt.Errorf("invalid status %q (legal values: %s)", o.Status, StatusList())
	}
	o.Status = status

	if strings.TrimSpace(o.Statement) == "" {
		o.Statement = statementPlaceholder
	}
	o.Statement = collapseWhitespace(o.Statement)

	if len(o.Scopes) == 0 {
		o.Scopes = []string{ScopeFleet}
	}
	if _, err := ParseScopes(o.Scopes); err != nil {
		return err
	}
	if _, err := ParseSources(o.Sources); err != nil {
		return err
	}

	if strings.TrimSpace(o.Enforcement) == "" {
		o.Enforcement = EnforcementTiers[0]
	}
	tier, ok := ParseEnforcement(o.Enforcement)
	if !ok {
		return fmt.Errorf("invalid enforcement %q (legal values: %s)", o.Enforcement, EnforcementList())
	}
	o.Enforcement = tier
	o.Control = collapseWhitespace(o.Control)
	if RequiresControl(o.Enforcement) && !isRealValue(o.Control) {
		return fmt.Errorf("enforcement %q requires --control naming the mechanism that refuses", o.Enforcement)
	}

	if strings.TrimSpace(o.Why) == "" {
		o.Why = whyPlaceholder
	}
	o.Why = collapseWhitespace(o.Why)
	if strings.TrimSpace(o.Exceptions) == "" {
		o.Exceptions = "none"
	}
	o.Exceptions = collapseWhitespace(o.Exceptions)
	if strings.TrimSpace(o.Instructions) == "" {
		o.Instructions = instructionsPlaceholder
	}
	o.Instructions = strings.TrimSpace(o.Instructions)
	if strings.TrimSpace(o.Compliant) == "" {
		o.Compliant = compliantPlaceholder
	}
	o.Compliant = strings.TrimSpace(o.Compliant)
	if strings.TrimSpace(o.Violation) == "" {
		o.Violation = violationPlaceholder
	}
	o.Violation = strings.TrimSpace(o.Violation)
	o.Supersedes = strings.TrimSpace(o.Supersedes)

	for _, name := range o.Skills {
		if err := ValidateSlug(strings.TrimSpace(name)); err != nil {
			return fmt.Errorf("invalid skill name %q: %v", name, err)
		}
	}
	return nil
}

// Row projects normalized options into the canonical index row.
func (o Options) Row(linked bool) Row {
	return NewRow(o.Slug, linked, o.Status, o.Statement, o.Scopes, o.Enforcement, o.Control, o.Sources)
}

// ScaffoldDetail returns a lint-clean rule detail document: the
// artifact-frontmatter-convention frontmatter, the `# Rule:` title, the twelve
// ordered bold fields (the first six mirroring the index row), Instructions,
// paired Examples, Open Questions, and the adherence footer.
func ScaffoldDetail(opts Options) ([]byte, error) {
	o := opts
	if err := o.Normalize(); err != nil {
		return nil, err
	}

	var b strings.Builder
	fmt.Fprintf(&b, "---\nformat: %s\nstatus: %s\n---\n\n", FormatURL, o.Status)
	fmt.Fprintf(&b, "# Rule: %s\n\n", o.Title)
	fmt.Fprintf(&b, "**Status:** %s\n", o.Status)
	fmt.Fprintf(&b, "**Date:** %s\n", o.Date)
	fmt.Fprintf(&b, "**Owner:** %s\n", o.Owner)
	fmt.Fprintf(&b, "**Statement:** %s\n", o.Statement)
	fmt.Fprintf(&b, "**Scope:** %s\n", joinOrSentinel(o.Scopes))
	fmt.Fprintf(&b, "**Enforcement:** %s\n", o.Enforcement)
	fmt.Fprintf(&b, "**Control:** %s\n", sentinelOr(o.Control))
	fmt.Fprintf(&b, "**Sources:** %s\n", joinOrSentinel(o.Sources))
	fmt.Fprintf(&b, "**Why:** %s\n", o.Why)
	fmt.Fprintf(&b, "**Exceptions:** %s\n", o.Exceptions)
	fmt.Fprintf(&b, "**Supersedes:** %s\n", sentinelOr(o.Supersedes))
	fmt.Fprintf(&b, "**Superseded By:** %s\n", Sentinel)

	b.WriteString("\n## Instructions\n\n")
	b.WriteString(o.Instructions + "\n")
	if len(o.Skills) > 0 {
		b.WriteString("\nApplies to: ")
		refs := make([]string, 0, len(o.Skills))
		for _, name := range o.Skills {
			refs = append(refs, "skill:"+strings.TrimSpace(name))
		}
		b.WriteString(strings.Join(refs, ", ") + "\n")
	}
	b.WriteString("\n## Examples\n\n### Compliant\n\n")
	b.WriteString(o.Compliant + "\n")
	b.WriteString("\n### Violation\n\n")
	b.WriteString(o.Violation + "\n")
	b.WriteString("\n## Open Questions\n\nNone at this time.\n\n")
	fmt.Fprintf(&b, "---\n*This document follows the %s*\n", FormatURL)

	return []byte(b.String()), nil
}

// ResolveRow returns the index row for slug, or an exit-3 NotFound error naming
// it. The index is the source of truth, so this — not a directory probe — is
// how every verb resolves a rule.
func ResolveRow(rulesDir, slug string) (Row, error) {
	if err := ValidateSlug(slug); err != nil {
		return Row{}, exitcode.InvalidArgsErrorf("invalid slug %q: %v", slug, err)
	}
	report, err := ReadIndex(IndexPath(rulesDir))
	if err != nil {
		return Row{}, exitcode.NotFoundErrorf("no rules index at %s", IndexPath(rulesDir))
	}
	for _, row := range report.Rows {
		if row.Slug == slug {
			return row, nil
		}
	}
	return Row{}, exitcode.NotFoundErrorf("rule %q is not listed in %s", slug, IndexPath(rulesDir))
}
