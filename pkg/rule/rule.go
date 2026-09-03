// Package rule parses, scaffolds, and indexes SpecScore Rule artifacts.
//
// A Rule is the *normative* artifact kind: one sentence an agent or human MUST
// or MUST NEVER do, with the scope it binds, the reason it exists, the sources
// that produced it, and the control that enforces it. It is deliberately the
// smallest kind in the spec tree — a Lesson explains a process gap, a Decision
// explains a choice, an Idea explains a direction; a Rule is the single
// transferable sentence that survives all three and can be handed to a fresh
// agent with no other context.
//
// Rules exist because durable operating knowledge accumulated in per-agent
// memory files, which do not transfer: a new session, a new machine, or a new
// runtime starts blind. A Rule is reviewable, greppable, scoped, and — at the
// Enforced and Automated tiers — attached to a named control.
//
// # Two forms, one entity
//
// An INLINE rule is exactly one row in spec/rules/README.md and nothing else.
// Most rules are one sentence; giving each of them a directory would be
// ceremony that discourages recording them at all.
//
// A DETAILED rule keeps the identical index row and adds
// spec/rules/<slug>/README.md carrying the reason, worked compliant and
// violating examples, agent instructions, exceptions, and supersession. Its
// index row's identity cell is a link, so the index alone says which rules have
// more to read.
//
// The index row is the source of truth for every field it carries. A detail
// document repeats those fields in its header for readability, and lint (R-011)
// requires them to agree — with --fix rewriting the document from the row,
// never the reverse. That is what keeps two representations of one rule from
// drifting into two different rules.
package rule

import (
	"fmt"
	"regexp"
	"strings"
)

// FormatURL is the canonical spec URL for the Rule detail document. It is
// carried verbatim in both the frontmatter `format:` field and the
// adherence-footer line, per the artifact-frontmatter-convention.
const FormatURL = "https://specscore.md/rule-specification"

// IndexFormatURL is the canonical spec URL for the rules index
// (spec/rules/README.md) — which, unusually for an index, is also the primary
// artifact: an inline rule lives nowhere else.
const IndexFormatURL = "https://specscore.md/rules-index-specification"

// Sentinel is the em-dash placeholder every optional field carries when it has
// no value. An empty value is a violation; the sentinel is the explicit
// "nothing here" a reader can trust (mirroring the canonical Lesson's relation
// fields).
const Sentinel = "—"

// Statuses is the closed, ordered Rule **Status:** vocabulary.
//
//   - Draft      — written down, not yet binding.
//   - Active     — binding within its declared Scope.
//   - Superseded — replaced by another Rule, named in **Superseded By:**.
var Statuses = []string{"Draft", "Active", "Superseded"}

// EnforcementTiers is the closed, ordered **Enforcement:** vocabulary.
//
//   - Stated    — an agent or human is told; nothing refuses.
//   - Enforced  — a named control refuses (a wb verb, a hook profile, a CI
//     check, a review probe).
//   - Automated — a named control both refuses and repairs, with no human in
//     the loop.
//
// Enforced and Automated MUST name a control; Stated MUST NOT be required to,
// because the whole point of that tier is that no control exists yet.
var EnforcementTiers = []string{"Stated", "Enforced", "Automated"}

// MirroredFields are the header fields a detail document repeats from its index
// row, in canonical order. The row is authoritative for every one of them.
var MirroredFields = []string{"Status", "Statement", "Scope", "Enforcement", "Control", "Sources"}

// DetailFields is the closed, ordered set of bold metadata fields every detail
// document MUST declare. The mirrored block comes first, then the fields the
// document alone owns.
var DetailFields = []string{
	"Status",
	"Date",
	"Owner",
	"Statement",
	"Scope",
	"Enforcement",
	"Control",
	"Sources",
	"Why",
	"Exceptions",
	"Supersedes",
	"Superseded By",
}

// DetailSections is the closed, ordered set of H2 headings a detail document
// MUST carry. Instructions is what an agent acts on; Examples is what stops
// "never mock a backend" from being read three different ways.
var DetailSections = []string{"Instructions", "Examples", "Open Questions"}

// ExampleSubsections are the H3 headings required inside `## Examples`. Both
// are required: a rule with only a compliant example teaches the happy path and
// leaves the reader guessing at the boundary the rule actually draws.
var ExampleSubsections = []string{"Compliant", "Violation"}

// slugRe matches a lowercase, hyphen-separated, URL-safe identifier.
var slugRe = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

// ValidateSlug returns nil when slug is a lowercase, hyphen-separated,
// URL-safe identifier with no `/`.
func ValidateSlug(slug string) error {
	if slug == "" {
		return fmt.Errorf("slug must not be empty")
	}
	if !slugRe.MatchString(slug) {
		return fmt.Errorf("slug %q must be lowercase, hyphen-separated, and URL-safe (no '/')", slug)
	}
	return nil
}

// IsStatus reports whether value is one of the canonical Rule statuses.
func IsStatus(value string) bool { return inVocabulary(Statuses, value) }

// ParseStatus resolves a case-insensitive status name to its canonical
// spelling.
func ParseStatus(value string) (string, bool) { return parseVocabulary(Statuses, value) }

// IsEnforcement reports whether value is one of the canonical tiers.
func IsEnforcement(value string) bool { return inVocabulary(EnforcementTiers, value) }

// ParseEnforcement resolves a case-insensitive tier name to its canonical
// spelling.
func ParseEnforcement(value string) (string, bool) { return parseVocabulary(EnforcementTiers, value) }

func inVocabulary(vocabulary []string, value string) bool {
	for _, v := range vocabulary {
		if v == value {
			return true
		}
	}
	return false
}

func parseVocabulary(vocabulary []string, value string) (string, bool) {
	for _, v := range vocabulary {
		if strings.EqualFold(v, strings.TrimSpace(value)) {
			return v, true
		}
	}
	return "", false
}

// RequiresControl reports whether an enforcement tier obliges the Rule to name
// a control. Stated is exactly the tier that does not.
func RequiresControl(tier string) bool {
	return tier == "Enforced" || tier == "Automated"
}

// StatusList renders the canonical status vocabulary for an error message.
func StatusList() string { return strings.Join(Statuses, ", ") }

// EnforcementList renders the canonical tier vocabulary for an error message.
func EnforcementList() string { return strings.Join(EnforcementTiers, ", ") }

// isRealValue reports whether a field carries content rather than the em-dash
// sentinel (or nothing at all).
func isRealValue(v string) bool {
	trimmed := strings.TrimSpace(v)
	return trimmed != "" && trimmed != Sentinel && trimmed != "-"
}

// collapseWhitespace folds newlines and runs of spaces into single spaces, so a
// multi-line value still writes one field or one table cell.
func collapseWhitespace(s string) string { return strings.Join(strings.Fields(s), " ") }

// splitList splits a comma-separated value into trimmed, non-empty,
// sentinel-free entries.
func splitList(value string) []string {
	trimmed := strings.TrimSpace(value)
	if !isRealValue(trimmed) {
		return nil
	}
	var out []string
	for _, part := range strings.Split(trimmed, ",") {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

// joinOrSentinel renders a list as a comma-separated value, or the sentinel
// when empty.
func joinOrSentinel(values []string) string {
	cleaned := make([]string, 0, len(values))
	for _, v := range values {
		if v = strings.TrimSpace(v); v != "" {
			cleaned = append(cleaned, v)
		}
	}
	if len(cleaned) == 0 {
		return Sentinel
	}
	return strings.Join(cleaned, ", ")
}

// sentinelOr returns value trimmed, or the em-dash sentinel when it is empty.
func sentinelOr(value string) string {
	if trimmed := strings.TrimSpace(value); trimmed != "" {
		return trimmed
	}
	return Sentinel
}

// TitleCaseFromSlug turns "never-mock-backends" into "Never Mock Backends".
func TitleCaseFromSlug(slug string) string {
	parts := strings.Split(slug, "-")
	for i, p := range parts {
		if p == "" {
			continue
		}
		parts[i] = strings.ToUpper(p[:1]) + p[1:]
	}
	return strings.Join(parts, " ")
}
