package lint

import (
	"path/filepath"
	"regexp"
	"strings"
)

// otherPlatformsHeading marks an Implementation's optional section listing
// sibling Implementations by link (capability-and-platform-implementations#
// req:other-platforms-section).
const otherPlatformsHeading = "## Other Platforms"

// parityTokenRe matches a parity status token as a whole word. These tokens are
// authoritative only in the Capability's Implementation Matrix and must not be
// restated in an "## Other Platforms" section.
var parityTokenRe = regexp.MustCompile(`\b(Full|Partial|Planned|Absent)\b`)

// otherPlatformsChecker enforces that an "## Other Platforms" section carries
// links only and never restates a parity status.
type otherPlatformsChecker struct{}

func newOtherPlatformsChecker() checker { return &otherPlatformsChecker{} }

func (c *otherPlatformsChecker) name() string     { return "other-platforms-links-only" }
func (c *otherPlatformsChecker) severity() string { return "error" }

func (c *otherPlatformsChecker) check(specRoot string) ([]Violation, error) {
	var violations []Violation
	walkErr := walkFeatureReadmes(specRoot, func(readmePath string, content []byte) {
		if !otherPlatformsHasParityToken(string(content)) {
			return
		}
		rel, _ := filepath.Rel(specRoot, readmePath)
		violations = append(violations, Violation{
			File:     rel,
			Line:     0,
			Severity: "error",
			Rule:     "other-platforms-links-only",
			Message: "Other Platforms section must carry links only; a parity status belongs solely in the " +
				"Capability's Implementation Matrix (capability-and-platform-implementations#req:other-platforms-section)",
		})
	})
	if walkErr != nil {
		return nil, walkErr
	}
	return violations, nil
}

// otherPlatformsHasParityToken reports whether the "## Other Platforms" section
// of content contains a parity status token. The section spans from its heading
// to the next Markdown heading (or end of content).
func otherPlatformsHasParityToken(content string) bool {
	inSection := false
	for _, line := range strings.Split(content, "\n") {
		t := strings.TrimSpace(line)
		if t == otherPlatformsHeading {
			inSection = true
			continue
		}
		if !inSection {
			continue
		}
		if strings.HasPrefix(t, "#") {
			break
		}
		if parityTokenRe.MatchString(line) {
			return true
		}
	}
	return false
}
