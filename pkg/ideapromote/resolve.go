// Package ideapromote implements `specscore idea promote <slug>` —
// turning a sidekick seed (spec/ideas/seeds/<slug>.md) into a lint-clean
// Idea (spec/ideas/<slug>.md). See spec/features/cli/idea/promote/README.md.
//
// This file houses the pre-mutation resolution primitives: resolving the
// seed path, detecting a destination collision, and computing the paths
// the verb will touch for the clean-tree pre-flight. Mutation logic
// (move/transform, back-link reconcile, cross-repo archive, verdict
// carry-forward) lives in sibling files.
package ideapromote

import (
	"os"
	"path/filepath"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// Paths bundles the repo-relative on-disk locations the promote verb
// reasons about for a given slug.
type Paths struct {
	// SeedRel is the source seed path: spec/ideas/seeds/<slug>.md.
	SeedRel string
	// IdeaRel is the destination Idea path: spec/ideas/<slug>.md.
	IdeaRel string
	// ArchivedSeedRel is the cross-repo archive destination:
	// spec/ideas/archived/<slug>.md.
	ArchivedSeedRel string
}

// PathsFor returns the canonical repo-relative paths for a slug.
func PathsFor(slug string) Paths {
	return Paths{
		SeedRel:         filepath.ToSlash(filepath.Join("spec", "ideas", "seeds", slug+".md")),
		IdeaRel:         filepath.ToSlash(filepath.Join("spec", "ideas", slug+".md")),
		ArchivedSeedRel: filepath.ToSlash(filepath.Join("spec", "ideas", "archived", slug+".md")),
	}
}

// ResolveSeed verifies a seed exists at spec/ideas/seeds/<slug>.md within
// specRoot. When absent, it returns an *exitcode.Error with code 3
// (NotFound) naming the missing path; no file is touched.
func ResolveSeed(specRoot, slug string) (string, error) {
	seedPath := filepath.Join(specRoot, "spec", "ideas", "seeds", slug+".md")
	if !fileExists(seedPath) {
		return "", exitcode.NotFoundErrorf("no seed found at %s", seedPath)
	}
	return seedPath, nil
}

// CheckCollision returns an *exitcode.Error with code 1 (Conflict) when a
// destination Idea already exists at spec/ideas/<slug>.md and force is
// false. With force=true an existing destination is permitted.
func CheckCollision(specRoot, slug string, force bool) error {
	ideaPath := filepath.Join(specRoot, "spec", "ideas", slug+".md")
	if !force && fileExists(ideaPath) {
		return exitcode.ConflictErrorf(
			"destination Idea already exists: %s (pass --force to overwrite)", ideaPath)
	}
	return nil
}

// fileExists reports whether the named path exists and is a regular file
// (not a directory).
func fileExists(path string) bool {
	info, err := os.Stat(path)
	if err != nil {
		return false
	}
	return !info.IsDir()
}
