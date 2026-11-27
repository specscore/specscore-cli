package feature

import (
	"regexp"
	"strings"
)

// acHeadingRe is the single definition of a well-formed Acceptance Criteria
// heading in a Feature README (cli/spec/lint/ac-heading-format): exactly
// "### AC: " followed by a kebab-case id, optionally followed by exactly one
// space and a grandfathered `(verifies ...)` annotation, and nothing else.
// `spec lint` (ac-heading-format) and `code deps --check` both use it, so a
// heading lint accepts is always one a source citation can resolve.
var acHeadingRe = regexp.MustCompile(`^### AC: ([a-z0-9]+(?:-[a-z0-9]+)*)(?: \(verifies\b[^()]*\))?$`)

// ACHeadingID reports the id an Acceptance Criteria heading declares, and
// whether line is a well-formed AC heading at all. Up to three spaces of
// indentation and any trailing whitespace are ignored, as Markdown ignores
// them for an ATX heading; a line indented as a code block
// (IsIndentedCodeLine) is never a heading.
//
// specscore:implements https://specscore.org/github.com/specscore/specscore-cli/spec/features/cli/code/deps#req:offline-typed-source-link-check
func ACHeadingID(line string) (string, bool) {
	if IsIndentedCodeLine(line) {
		return "", false
	}
	m := acHeadingRe.FindStringSubmatch(strings.TrimSpace(line))
	if m == nil {
		return "", false
	}
	return m[1], true
}

// IsIndentedCodeLine reports whether line's leading whitespace reaches four
// columns, a tab advancing to the next multiple of four, which makes it
// Markdown indented code rather than a heading.
func IsIndentedCodeLine(line string) bool {
	column := 0
	for _, r := range line {
		switch r {
		case ' ':
			column++
		case '\t':
			column += 4 - column%4
		default:
			return false
		}
		if column >= 4 {
			return true
		}
	}
	return false
}
