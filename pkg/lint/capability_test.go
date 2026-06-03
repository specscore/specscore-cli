package lint

import "testing"

// Task 1 — role classification (capability-and-platform-implementations#
// ac:capability-marker, #ac:implementation-marker). A Feature is a Capability
// when it contains an "## Implementation Matrix" section, and an Implementation
// when its header block carries an "**Implements:**" line. The two markers are
// independent flags reported on their own merits.
func TestClassifyFeatureRole(t *testing.T) {
	cases := []struct {
		name             string
		content          string
		wantCapability   bool
		wantImplementing bool
	}{
		{
			name:           "capability marker recognized",
			content:        "# Feature: Dashboards\n\n**Status:** Approved\n\n## Implementation Matrix\n\n| Platform | Status | Brief | Link |\n",
			wantCapability: true,
		},
		{
			name:             "implementation marker recognized",
			content:          "# Feature: Dashboards (CLI)\n\n**Status:** Approved\n**Implements:** specscore:feature/dashboards\n\n## Summary\n",
			wantImplementing: true,
		},
		{
			name:    "plain feature is neither",
			content: "# Feature: Plain\n\n**Status:** Approved\n\n## Summary\n",
		},
		{
			name:           "implementation matrix heading must be line-exact",
			content:        "# Feature: X\n\n## Implementation Matrix Notes\n\nnot the real section\n",
			wantCapability: false,
		},
		{
			name:             "both markers reported independently",
			content:          "# Feature: Weird\n\n**Implements:** specscore:feature/base\n\n## Implementation Matrix\n",
			wantCapability:   true,
			wantImplementing: true,
		},
		{
			name:             "implements detected despite leading whitespace",
			content:          "# Feature: Indented\n\n  **Implements:** specscore:feature/base\n",
			wantImplementing: true,
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			role := classifyFeatureRole(tc.content)
			if role.isCapability != tc.wantCapability {
				t.Errorf("isCapability = %v, want %v", role.isCapability, tc.wantCapability)
			}
			if role.isImplementation != tc.wantImplementing {
				t.Errorf("isImplementation = %v, want %v", role.isImplementation, tc.wantImplementing)
			}
		})
	}
}
