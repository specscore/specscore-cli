package ideapromote

import (
	"strings"
	"testing"
)

// AC: never-deprecated + cross-repo frontmatter marking.
func TestMarkArchivedSeed_StatusPromotedNeverDeprecated(t *testing.T) {
	seed := "---\n" +
		"type: sidekick-seed\n" +
		"slug: baz\n" +
		"status: queued\n" +
		"captured_by: x\n" +
		"---\n" +
		"# Baz\n\nprose\n"
	got := markArchivedSeed(seed, "baz")

	if !strings.Contains(got, "status: promoted") {
		t.Errorf("status should become promoted; got:\n%s", got)
	}
	if strings.Contains(got, "status: queued") {
		t.Errorf("old status: queued should be replaced; got:\n%s", got)
	}
	if !strings.Contains(got, "promoted_to: baz") {
		t.Errorf("promoted_to should be set; got:\n%s", got)
	}
	if strings.Contains(got, "deprecated") {
		t.Errorf("deprecated must NEVER be set; got:\n%s", got)
	}
	// Body preserved.
	if !strings.Contains(got, "# Baz") || !strings.Contains(got, "prose") {
		t.Errorf("seed body must be preserved; got:\n%s", got)
	}
}

func TestMarkArchivedSeed_AddsKeysWhenAbsent(t *testing.T) {
	seed := "---\ntype: sidekick-seed\nslug: baz\n---\n# Baz\n\nprose\n"
	got := markArchivedSeed(seed, "baz")
	if !strings.Contains(got, "status: promoted") {
		t.Errorf("status: promoted should be added when absent; got:\n%s", got)
	}
	if !strings.Contains(got, "promoted_to: baz") {
		t.Errorf("promoted_to should be added when absent; got:\n%s", got)
	}
}

func TestMarkArchivedSeed_SynthesizesFrontmatterWhenMissing(t *testing.T) {
	seed := "# Baz\n\nprose only, no frontmatter\n"
	got := markArchivedSeed(seed, "baz")
	if !strings.HasPrefix(got, "---\n") {
		t.Errorf("frontmatter should be synthesized; got:\n%s", got)
	}
	if !strings.Contains(got, "status: promoted") || !strings.Contains(got, "promoted_to: baz") {
		t.Errorf("synthesized frontmatter must carry promotion keys; got:\n%s", got)
	}
}
