package lint

// Features implemented: cli/spec/lint/ac-heading-format

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// acHeadingChecker enforces the canonical Acceptance Criteria heading format
// on Feature README files: `### AC: <id>` with exactly one space after the
// colon and an id matching kebab-case ([a-z0-9]+(-[a-z0-9]+)*), nothing more
// — with one grandfathered exception. Some existing Feature READMEs (in this
// repo and the upstream `specscore` meta-spec repo the entity/property
// integration test lints against) carry a trailing `(verifies REQ:...)`
// annotation that plan_rules.go's own acHeadingRe already treats as a named,
// parseable construct ("the verifies-parenthetical"). That annotation is
// redundant with the AC's `**Requirements:**` line but is a real,
// widely-used, pre-existing convention — not a target of this rule — so a
// heading of the form `### AC: <id> (verifies ...)` is accepted, with only
// its surrounding whitespace subject to the same formatting checks as the
// bare form.
//
// Any line that even attempts to be an AC heading (matches
// acHeadingTriggerRe) but deviates from both accepted forms is a violation.
// `--fix` repairs only pure whitespace deviations (a missing or doubled
// space around "###", "AC", the colon, or the id/parenthetical boundary)
// when the extracted id is already valid kebab-case. An invalid id
// (uppercase letters, underscores, or other non-kebab characters) or any
// OTHER trailing text after the id is reported but never auto-fixed — both
// require a human decision.
type acHeadingChecker struct{}

// walkACFeatureReadmes is a narrow test seam around the shared filesystem
// walker. Production always uses walkFeatureReadmes; the seam lets the checker
// prove that traversal errors remain visible rather than being mistaken for a
// clean rule result.
var walkACFeatureReadmes = walkFeatureReadmes

func newACHeadingFormatChecker() *acHeadingChecker { return &acHeadingChecker{} }

func (c *acHeadingChecker) name() string     { return "ac-heading-format" }
func (c *acHeadingChecker) severity() string { return "error" }

// acHeadingTriggerRe recognizes any (trimmed) line attempting to be an AC
// heading, however malformed: three hashes, optional whitespace, the literal
// "AC", optional whitespace, and a colon. Intentionally loose so every
// malformed variant in scope is caught; a matching line is then checked
// against acHeadingCanonicalRe to see whether it is already well-formed.
var acHeadingTriggerRe = regexp.MustCompile(`^###\s*AC\s*:`)

// acHeadingCanonicalRe is the accepted form: exactly "### AC: " followed by a
// kebab-case id, optionally followed by exactly one space and a grandfathered
// `(verifies ...)` annotation (see type doc), and nothing else.
var acHeadingCanonicalRe = regexp.MustCompile(`^### AC: [a-z0-9]+(?:-[a-z0-9]+)*(?: \(verifies\b[^()]*\))?$`)

// acHeadingParseRe captures everything after the (loosely-matched) "AC:"
// marker, with any leading whitespace already stripped, for classifying
// exactly how a non-canonical heading is malformed.
var acHeadingParseRe = regexp.MustCompile(`^###\s*AC\s*:\s*(.*)$`)

// acKebabRe matches a well-formed kebab-case id in isolation.
var acKebabRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// acVerifiesParentheticalRe matches the grandfathered `(verifies ...)`
// annotation in isolation (already whitespace-trimmed).
var acVerifiesParentheticalRe = regexp.MustCompile(`^\(verifies\b.*\)$`)

func (c *acHeadingChecker) check(specRoot string) ([]Violation, error) {
	var out []Violation
	err := walkACFeatureReadmes(specRoot, func(readmePath string, content []byte) {
		if !isFeatureReadmeContent(content) {
			return
		}
		relPath, _ := filepath.Rel(specRoot, readmePath)
		lines := strings.Split(string(content), "\n")
		for i, raw := range lines {
			trimmed := strings.TrimSpace(raw)
			if !acHeadingTriggerRe.MatchString(trimmed) || acHeadingCanonicalRe.MatchString(trimmed) {
				continue
			}
			msg, _, _ := classifyACHeading(trimmed)
			out = append(out, Violation{
				File:     relPath,
				Line:     i + 1,
				Severity: "error",
				Rule:     "ac-heading-format",
				Message:  msg,
			})
		}
	})
	if err != nil {
		return nil, err
	}
	// filepath.Walk visits paths in lexical order and each file is scanned in
	// line order, so `out` is already deterministically ordered.
	return out, nil
}

// fix rewrites every AC heading whose only defect is whitespace formatting to
// its canonical form (`### AC: <id>`, or `### AC: <id> (verifies ...)` when a
// grandfathered annotation is present). Headings with an invalid id or other
// trailing content are left untouched — those need a human decision.
// Idempotent.
func (c *acHeadingChecker) fix(specRoot string) error {
	return walkFeatureReadmes(specRoot, func(readmePath string, content []byte) {
		if !isFeatureReadmeContent(content) {
			return
		}
		lines := strings.Split(string(content), "\n")
		changed := false
		for i, raw := range lines {
			trimmed := strings.TrimSpace(raw)
			if !acHeadingTriggerRe.MatchString(trimmed) || acHeadingCanonicalRe.MatchString(trimmed) {
				continue
			}
			_, fixable, canonical := classifyACHeading(trimmed)
			if !fixable {
				continue
			}
			lines[i] = canonical
			changed = true
		}
		if !changed {
			return
		}
		_ = os.WriteFile(readmePath, []byte(strings.Join(lines, "\n")), 0o644)
	})
}

// classifyACHeading explains why a trimmed line matching acHeadingTriggerRe
// (but not acHeadingCanonicalRe) is malformed, whether the defect is
// whitespace-only (and therefore safe for `--fix` to repair), and — when
// fixable — the exact canonical replacement line.
func classifyACHeading(trimmed string) (message string, fixable bool, canonical string) {
	m := acHeadingParseRe.FindStringSubmatch(trimmed)
	rest := ""
	if m != nil {
		rest = m[1]
	}
	if rest == "" {
		return fmt.Sprintf("AC heading %q is missing an id; expected `### AC: <id>`", trimmed), false, ""
	}

	candidateID := rest
	trailing := ""
	if idx := strings.IndexAny(rest, " \t"); idx != -1 {
		candidateID = rest[:idx]
		trailing = strings.TrimSpace(rest[idx:])
	}

	if !acKebabRe.MatchString(candidateID) {
		return fmt.Sprintf("AC id %q is not kebab-case; expected lowercase letters, digits, and hyphens only, e.g. `### AC: %s`", candidateID, candidateID), false, ""
	}
	if trailing == "" {
		return fmt.Sprintf("AC heading %q does not match the canonical `### AC: %s` (exactly one space after \"AC:\", no other spacing)", trimmed, candidateID),
			true, "### AC: " + candidateID
	}
	if acVerifiesParentheticalRe.MatchString(trailing) {
		return fmt.Sprintf("AC heading %q does not match the canonical `### AC: %s %s` (exactly one space after \"AC:\" and before the parenthetical)", trimmed, candidateID, trailing),
			true, "### AC: " + candidateID + " " + trailing
	}
	return fmt.Sprintf("AC heading has trailing content after the id (%q); expected exactly `### AC: %s`", trailing, candidateID), false, ""
}

// isFeatureReadmeContent reports whether content is a Feature README,
// recognized by a `# Feature: <title>` H1 line (mirrors the check in
// feature_rules.go's featureSourceIdeas).
func isFeatureReadmeContent(content []byte) bool {
	for _, l := range strings.Split(string(content), "\n") {
		if strings.HasPrefix(strings.TrimSpace(l), "# Feature: ") {
			return true
		}
	}
	return false
}
