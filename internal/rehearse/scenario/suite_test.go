package scenario_test

import (
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/internal/rehearse/scenario"
)

func TestSplitSuite_FlatIsNotASuite(t *testing.T) {
	// Flat scenarios use H2 Given/When/Then — never `### When` (H3).
	_, _, nested := scenario.SplitSuite([]byte("## Given x\n## When y\n## Then z\n"))
	if nested {
		t.Error("a flat (H2) scenario was reported as a nested suite")
	}
}

func TestSplitSuite_Nested(t *testing.T) {
	src := "**Verifies:** demo#ac:x\n\n## Given setup\n\n```bash\necho g\n```\n\n" +
		"### When a happens\n\n```bash\necho a\n```\n\n#### Then ok\n\n" +
		"### When b happens\n\n```bash\necho b\n```\n"
	given, whens, nested := scenario.SplitSuite([]byte(src))
	if !nested {
		t.Fatal("nested suite not detected")
	}
	if !strings.Contains(given, "## Given setup") || !strings.Contains(given, "**Verifies:**") {
		t.Errorf("given preamble missing setup or metadata:\n%s", given)
	}
	if len(whens) != 2 {
		t.Fatalf("whens = %d, want 2", len(whens))
	}
	if whens[0].Label != "When a happens" {
		t.Errorf("whens[0].Label = %q", whens[0].Label)
	}
	if !strings.Contains(whens[0].Content, "echo a") || strings.Contains(whens[0].Content, "echo b") {
		t.Errorf("whens[0] content leaked across branches:\n%s", whens[0].Content)
	}
	if whens[1].Label != "When b happens" || !strings.Contains(whens[1].Content, "echo b") {
		t.Errorf("whens[1] = %+v", whens[1])
	}
}

func TestSplitSuite_WhenInFenceIgnored(t *testing.T) {
	_, _, nested := scenario.SplitSuite([]byte("```bash\n### When not-a-heading\n```\n"))
	if nested {
		t.Error("`### When` inside a fence was counted as a branch")
	}
}
