package ideapromote

import (
	"fmt"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/idearelocate"
)

// Preflight verifies that every repo-relative path in subjects is clean
// in the source working tree (no staged, unstaged, or untracked changes).
// It returns an *exitcode.Error with code 7 (DirtyTree) naming the dirty
// paths when any is dirty; a non-git repo is treated as vacuously clean,
// mirroring `idea relocate`'s pre-flight discipline.
//
// subjects are deduplicated; a non-existent path that is untracked-clean
// (git reports nothing) does not trip the check.
func Preflight(specRoot string, subjects []string) error {
	var dirty []string
	seen := make(map[string]struct{}, len(subjects))
	for _, rel := range subjects {
		if rel == "" {
			continue
		}
		if _, ok := seen[rel]; ok {
			continue
		}
		seen[rel] = struct{}{}
		clean, err := isPathCleanFn(specRoot, rel)
		if err != nil {
			return exitcode.UnexpectedErrorf("pre-flight check %s: %v", rel, err)
		}
		if !clean {
			dirty = append(dirty, rel)
		}
	}
	if len(dirty) == 0 {
		return nil
	}
	var sb strings.Builder
	sb.WriteString("pre-flight: uncommitted changes in paths that would be modified — promote aborted:\n")
	for _, p := range dirty {
		fmt.Fprintf(&sb, "  %s: %s\n", specRoot, p)
	}
	sb.WriteString("Commit or stash the changes above and re-run.")
	return exitcode.Newf(exitcode.DirtyTree, "%s", sb.String())
}

// isPathCleanFn is a seam for testing. Production delegates to
// idearelocate.IsPathClean, which shares the verb family's clean-tree
// semantics.
var isPathCleanFn = idearelocate.IsPathClean
