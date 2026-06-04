package ideapromote

import (
	"fmt"
	"strings"
)

// SeedFate is the disposition of the source seed after promotion.
type SeedFate string

const (
	// FateMoved is the same-repo outcome: the seed was git-moved to the
	// Idea path.
	FateMoved SeedFate = "moved"
	// FateArchived is the cross-repo outcome: the seed was archived to
	// spec/ideas/archived/<slug>.md.
	FateArchived SeedFate = "archived"
)

// Result is the deterministic, structured summary of a successful
// promotion. It drives both the text summary and the --format json|yaml
// output, mirroring `idea relocate`'s stdout-format shape.
type Result struct {
	// Slug is the promoted slug.
	Slug string `json:"slug" yaml:"slug"`
	// IdeaPath is the absolute path of the created Idea.
	IdeaPath string `json:"idea_path" yaml:"idea_path"`
	// SeedFate is "moved" (same-repo) or "archived" (cross-repo).
	SeedFate SeedFate `json:"seed_fate" yaml:"seed_fate"`
	// SeedPath is the absolute path the seed ended up at: the Idea path
	// (moved) or the archived path (archived).
	SeedPath string `json:"seed_path" yaml:"seed_path"`
	// ReconciledBackLinks lists the repo-relative paths of same-repo
	// back-link files rewritten from the seeds path to the Idea path.
	// Empty in the cross-repo path. Always non-nil for stable JSON output.
	ReconciledBackLinks []string `json:"reconciled_back_links" yaml:"reconciled_back_links"`
}

// FormatText renders the deterministic human-readable summary: the
// created Idea path, the seed's fate, and each reconciled back-link.
func (r Result) FormatText() string {
	var sb strings.Builder
	fmt.Fprintf(&sb, "created idea: %s\n", r.IdeaPath)
	fmt.Fprintf(&sb, "seed %s: %s\n", r.SeedFate, r.SeedPath)
	for _, bl := range r.ReconciledBackLinks {
		fmt.Fprintf(&sb, "reconciled back-link: %s\n", bl)
	}
	return sb.String()
}
