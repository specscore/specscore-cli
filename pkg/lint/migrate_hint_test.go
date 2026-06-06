package lint

import (
	"strings"
	"testing"
)

const wantHint = "specscore spec migrate"

func ruleMessage(t *testing.T, c checker, specRoot, rule string) string {
	t.Helper()
	vs, err := c.check(specRoot)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range vs {
		if v.Rule == rule {
			return v.Message
		}
	}
	t.Fatalf("no %s violation produced", rule)
	return ""
}

// The migrate-fixable frontmatter violations point the user at `spec migrate`.
func TestFrontmatterRules_MigrateHintOnFixableViolations(t *testing.T) {
	// A feature with a body **Status:** but no frontmatter → format-field
	// (missing format) and status-mirror (missing status) both fire.
	missing := writeSpec(t, map[string]string{
		"features/foo/README.md": "# Feature: Foo\n\n**Status:** Draft\n\n## Summary\n\nx\n\n---\n*This document follows the https://specscore.md/feature-specification*\n",
	})
	if m := ruleMessage(t, newFormatFieldChecker(), missing, "format-field"); !strings.Contains(m, wantHint) {
		t.Errorf("format-field message lacks migrate hint: %q", m)
	}
	if m := ruleMessage(t, newStatusMirrorChecker(), missing, "status-mirror"); !strings.Contains(m, wantHint) {
		t.Errorf("status-mirror message lacks migrate hint: %q", m)
	}

	// A feature whose footer URL disagrees with a present frontmatter format.
	footerDrift := writeSpec(t, map[string]string{
		"features/bar/README.md": "---\nformat: https://specscore.md/feature-specification\nstatus: Draft\n---\n\n# Feature: Bar\n\n**Status:** Draft\n\n## Summary\n\nx\n\n---\n*This document follows the https://specscore.md/plan-specification*\n",
	})
	if m := ruleMessage(t, newFooterFormatMirrorChecker(), footerDrift, "footer-format-mirror"); !strings.Contains(m, wantHint) {
		t.Errorf("footer-format-mirror message lacks migrate hint: %q", m)
	}
}

// The status-less rejection is a hand-fix (migrate only adds frontmatter, it
// won't strip a stray status:), so its message must NOT suggest spec migrate.
func TestFrontmatterRules_NoMigrateHintOnStatusLess(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"features/README.md": "---\nstatus: Draft\n---\n\n# Features\n",
	})
	if m := ruleMessage(t, newStatusMirrorChecker(), specRoot, "status-mirror"); strings.Contains(m, wantHint) {
		t.Errorf("status-less rejection must not suggest spec migrate: %q", m)
	}
}
