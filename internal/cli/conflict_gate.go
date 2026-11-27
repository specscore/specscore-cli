package cli

import (
	"errors"
	"strings"

	"github.com/specscore/specscore-cli/pkg/conflictmarker"
	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lint"
)

// refuseConflictedTables is the pre-write precondition of the verbs that
// rewrite a source-of-truth table — the Lessons index (`lesson
// change-status`) and the Rules index (`rule new/update/delete/promote/expand`),
// the only copy of an inline rule. If any of paths still carries git
// merge-conflict markers, the verb exits 1 (Conflict) naming the files before
// it writes anything or prepares any event, because rewriting the table would
// silently drop one side of the conflict
// (cli/spec/lint#req:writers-refuse-conflicted-files). A missing file is not
// conflicted. Every other writer relies on its post-write lint instead (see
// the Open Question in cli/spec/lint).
func refuseConflictedTables(projectRoot string, paths ...string) error {
	var conflicted []string
	for _, path := range paths {
		err := conflictmarker.CheckFile(path)
		if errors.Is(err, conflictmarker.ErrConflicted) {
			conflicted = append(conflicted, repoRelative(projectRoot, path))
			continue
		}
		if err != nil {
			return exitcode.UnexpectedErrorf("checking %s for merge-conflict markers: %v", repoRelative(projectRoot, path), err)
		}
	}
	if len(conflicted) == 0 {
		return nil
	}
	return conflictRefusal(conflicted)
}

// conflictRefusal renders the shared refusal for a set of conflicted files.
func conflictRefusal(files []string) error {
	return exitcode.ConflictErrorf(
		"refusing to modify the spec tree: unresolved git merge-conflict markers in %s; resolve each conflict, then re-run (nothing was modified)",
		strings.Join(files, ", "))
}

// fixRefusal maps a *lint.FixRefusedError (a refused --fix) onto the shared
// Conflict refusal, and returns nil for any other error.
func fixRefusal(specRoot string, err error) error {
	var refused *lint.FixRefusedError
	if errors.As(err, &refused) {
		return conflictRefusal(projectRelativeSpecPaths(specRoot, refused.Files))
	}
	return nil
}
