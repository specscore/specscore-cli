package lint

import "strings"

// matrixHeading is the line-exact section marker that designates a Feature as
// a Capability (capability-and-platform-implementations#req:capability-role).
const matrixHeading = "## Implementation Matrix"

// implementsPrefix is the header-block field that designates a Feature as an
// Implementation (capability-and-platform-implementations#req:implementation-role).
const implementsPrefix = "**Implements:**"

// featureRole captures the structural markers that classify a Feature into the
// Capability / Implementation roles defined by the
// capability-and-platform-implementations Feature. The two markers are
// independent: classification reports each on its own merits (a Feature is
// normally one or the other, but both flags are computed separately).
type featureRole struct {
	isCapability     bool   // contains a line-exact "## Implementation Matrix" section
	isImplementation bool   // header block carries an "**Implements:**" line
	implementsLine   string // raw text of the "**Implements:**" line, when present
}

// classifyFeatureRole inspects a Feature README's content and reports its role
// markers. A Capability owns an "## Implementation Matrix" section (matched
// line-exact to avoid matching headings like "## Implementation Matrix Notes");
// an Implementation declares an "**Implements:**" line in its body metadata,
// whose raw text is captured for reference resolution.
func classifyFeatureRole(content string) featureRole {
	var role featureRole
	for _, line := range strings.Split(content, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == matrixHeading {
			role.isCapability = true
		}
		if strings.HasPrefix(trimmed, implementsPrefix) {
			role.isImplementation = true
			role.implementsLine = line
		}
	}
	return role
}
