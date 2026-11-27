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
// whether line is a well-formed AC heading at all. Surrounding whitespace is
// ignored, as ac-heading-format ignores it.
//
// specscore:implements https://specscore.org/github.com/specscore/specscore-cli/spec/features/cli/code/deps#req:offline-typed-source-link-check
func ACHeadingID(line string) (string, bool) {
	m := acHeadingRe.FindStringSubmatch(strings.TrimSpace(line))
	if m == nil {
		return "", false
	}
	return m[1], true
}
