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
	if !strings.Contains(got, "type: sidekick-seed") {
		t.Errorf("synthesized frontmatter must carry type: sidekick-seed; got:\n%s", got)
	}
	if !strings.Contains(got, "status: promoted") || !strings.Contains(got, "promoted_to: baz") {
		t.Errorf("synthesized frontmatter must carry promotion keys; got:\n%s", got)
	}
}

// TestMarkArchivedSeed_InjectsTypeWhenAbsent locks in the archive-time `type`
// injection: captured seeds in spec/ideas/seeds/ no longer store `type`, so
// archiving must add `type: sidekick-seed` (the key the archived-dir lint scan
// and Idea-discovery exclusion key off).
func TestMarkArchivedSeed_InjectsTypeWhenAbsent(t *testing.T) {
	seed := "---\ncaptured_by: user\nstatus: queued\n---\n# Baz\n\nprose\n"
	got := markArchivedSeed(seed, "baz")
	if !strings.Contains(got, "type: sidekick-seed") {
		t.Errorf("type: sidekick-seed should be injected at archive time; got:\n%s", got)
	}
	if !strings.Contains(got, "status: promoted") || !strings.Contains(got, "promoted_to: baz") {
		t.Errorf("promotion keys must be set; got:\n%s", got)
	}
}
