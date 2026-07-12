package scenario

import (
	"regexp"
	"strings"
)

// SuiteWhen is one `### When …` branch of a nested scenario suite: its heading
// label and the verbatim markdown of the branch (from its heading to the next
// `### When` or end of file). Feature: cli/rehearse/nested-suites.
type SuiteWhen struct {
	// Label is the branch heading text (e.g. "When he completes the callback").
	Label string
	// Content is the verbatim markdown of the branch.
	Content string
}

// whenHeading matches an H3 `### When …` branch heading — exactly three hashes,
// so it never matches a flat `## When` (H2) or a nested `#### Then` (H4).
var whenHeading = regexp.MustCompile(`^###\s+When\b`)

// SplitSuite reports whether data is a nested scenario *suite* — one that
// branches with `### When` (H3) headings — and, if so, splits it into the
// shared Given preamble (everything before the first `### When`, including
// frontmatter and body metadata) and one SuiteWhen per branch. Each root-to-leaf
// path (Given + one When) is an independently-executed case. `### When` headings
// inside fenced code blocks are ignored. A file with no `### When` heading is
// not a suite (nested=false) and runs flat. Feature: cli/rehearse/nested-suites.
func SplitSuite(data []byte) (given string, whens []SuiteWhen, nested bool) {
	lines := strings.Split(string(data), "\n")
	var starts []int
	inFence := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			inFence = !inFence
			continue
		}
		if !inFence && whenHeading.MatchString(line) {
			starts = append(starts, i)
		}
	}
	if len(starts) == 0 {
		return "", nil, false
	}
	given = strings.Join(lines[:starts[0]], "\n")
	for k, s := range starts {
		end := len(lines)
		if k+1 < len(starts) {
			end = starts[k+1]
		}
		label := strings.TrimSpace(strings.TrimLeft(lines[s], "#"))
		whens = append(whens, SuiteWhen{Label: label, Content: strings.Join(lines[s:end], "\n")})
	}
	return given, whens, true
}
