package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Task 6 — a parity status token inside an Implementation's "## Other
// Platforms" section is an error: parity status is authoritative only in the
// Capability's Implementation Matrix
// (capability-and-platform-implementations#ac:other-platforms-links-only).
func TestOtherPlatforms_StatusToken_Errors(t *testing.T) {
	tmp := t.TempDir()
	content := "# Feature: Dashboards (CLI)\n\n" +
		"**Implements:** specscore:feature/dashboards\n\n" +
		"## Other Platforms\n\n" +
		"- Web: Full — specscore:feature/dashboards-web\n"
	writeFeatureReadme(t, tmp, "dashboards-cli", content)

	violations := runOtherPlatformsCheck(t, tmp)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(violations), violations)
	}
	v := violations[0]
	if v.Rule != "other-platforms-links-only" {
		t.Errorf("Rule = %q, want other-platforms-links-only", v.Rule)
	}
	if !strings.Contains(v.Message, "Implementation Matrix") {
		t.Errorf("Message = %q, want it to point at the Implementation Matrix", v.Message)
	}
}

// Links-only Other Platforms sections (no parity token) are accepted, and the
// section ends cleanly at the next heading.
func TestOtherPlatforms_LinksOnly_NoViolation(t *testing.T) {
	tmp := t.TempDir()
	content := "# Feature: Dashboards (CLI)\n\n" +
		"**Implements:** specscore:feature/dashboards\n\n" +
		"## Other Platforms\n\n" +
		"- Web: specscore:feature/dashboards-web\n\n" +
		"## Open Questions\n\nNone.\n"
	writeFeatureReadme(t, tmp, "dashboards-cli", content)

	violations := runOtherPlatformsCheck(t, tmp)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations, got %d: %+v", len(violations), violations)
	}
}

// A parity-looking word that is only a substring of another word does not
// trigger the rule, and a status token outside the section is ignored.
func TestOtherPlatforms_TokenScopingAndWordBoundary(t *testing.T) {
	tmp := t.TempDir()
	content := "# Feature: Dashboards (CLI)\n\n" +
		"**Implements:** specscore:feature/dashboards\n\n" +
		"## Summary\n\nThis surface is Fully documented and Partial-word safe.\n\n" +
		"## Other Platforms\n\n" +
		"- Web: specscore:feature/dashboards-web\n"
	writeFeatureReadme(t, tmp, "dashboards-cli", content)

	violations := runOtherPlatformsCheck(t, tmp)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations, got %d: %+v", len(violations), violations)
	}
}

// No Other Platforms section at all — nothing to check.
func TestOtherPlatforms_NoSection_NoViolation(t *testing.T) {
	tmp := t.TempDir()
	writeFeatureReadme(t, tmp, "dashboards-cli",
		"# Feature: Dashboards (CLI)\n\n**Implements:** specscore:feature/dashboards\n\n## Summary\n")

	violations := runOtherPlatformsCheck(t, tmp)
	if len(violations) != 0 {
		t.Fatalf("expected 0 violations, got %d: %+v", len(violations), violations)
	}
}

func TestOtherPlatforms_Metadata(t *testing.T) {
	c := newOtherPlatformsChecker()
	if c.name() != "other-platforms-links-only" {
		t.Errorf("name = %q", c.name())
	}
	if c.severity() != "error" {
		t.Errorf("severity = %q", c.severity())
	}
}

func TestOtherPlatforms_WalkError(t *testing.T) {
	tmp := t.TempDir()
	writeFeatureReadme(t, tmp, "dashboards-cli",
		"# Feature: Dashboards (CLI)\n\n## Other Platforms\n\n- Web: Full\n")
	featDir := filepath.Join(tmp, "features", "dashboards-cli")
	if err := os.Chmod(featDir, 0o111); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(featDir, 0o755) }()

	c := newOtherPlatformsChecker()
	if _, err := c.check(tmp); err == nil {
		t.Fatal("expected a walk error from the unreadable features subtree")
	}
}

func runOtherPlatformsCheck(t *testing.T, specRoot string) []Violation {
	t.Helper()
	c := newOtherPlatformsChecker()
	violations, err := c.check(specRoot)
	if err != nil {
		t.Fatalf("check returned error: %v", err)
	}
	return violations
}
