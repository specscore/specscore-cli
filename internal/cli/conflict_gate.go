package cli

import (
	"errors"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lint"
)

// conflictMarkersFn is the seam for the merge-conflict-marker scan producers
// run before their first write.
var conflictMarkersFn = lint.ConflictMarkers

// refuseConflictedProject is the one shared precondition every producer verb
// and index writer runs before it mutates any file or prepares any event: if
// any SpecScore-managed file under projectRoot still carries git
// merge-conflict markers, the verb exits 1 (Conflict) naming the files and
// writes nothing (cli/spec/lint#req:writers-refuse-conflicted-files). It
// uses exactly the scope `spec lint` gates on, so a producer can never write
// into a tree lint would refuse to fix.
func refuseConflictedProject(projectRoot string) error {
	markers, err := conflictMarkersFn(lint.Options{SpecRoot: filepath.Join(projectRoot, "spec"), ProjectRoot: projectRoot})
	if err != nil {
		return exitcode.UnexpectedErrorf("scanning for merge-conflict markers: %v", err)
	}
	if len(markers) == 0 {
		return nil
	}
	return conflictRefusal(projectRelativeSpecPaths(filepath.Join(projectRoot, "spec"), lint.ConflictMarkerFiles(markers)))
}

// conflictRefusal renders the shared refusal for a set of conflicted files.
func conflictRefusal(files []string) error {
	return exitcode.ConflictErrorf(
		"refusing to modify the spec tree: unresolved git merge-conflict markers in %s; resolve each conflict, then re-run (nothing was modified)",
		strings.Join(files, ", "))
}

// fixRefusal maps a *lint.FixRefusedError (a refused --fix or migration)
// onto the shared Conflict refusal, and returns nil for any other error.
func fixRefusal(specRoot string, err error) error {
	var refused *lint.FixRefusedError
	if errors.As(err, &refused) {
		return conflictRefusal(projectRelativeSpecPaths(specRoot, refused.Files))
	}
	return nil
}
