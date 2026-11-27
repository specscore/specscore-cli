package lint

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// mergeConflictMarkersRule is the rule ID of the repository-wide git
// merge-conflict-marker gate (cli/spec/lint#req:merge-conflict-markers-detected).
const mergeConflictMarkersRule = "merge-conflict-markers"

// conflictMarkerWidth is git's default conflict-marker-size: every marker git
// writes is exactly seven repeated characters.
const conflictMarkerWidth = 7

// Package-level seams, swapped in tests to drive the defensive error branches
// (unreadable directory or file) that real filesystem state cannot reliably
// reproduce on every host. Same convention as lifecycle_lock_litter.go.
var (
	conflictMarkerWalkDir  = filepath.WalkDir
	conflictMarkerReadFile = os.ReadFile
	conflictMarkerRel      = filepath.Rel
)

// mergeConflictMarkerChecker scans every text file SpecScore manages for git
// merge-conflict markers left behind by an unresolved merge: the spec tree,
// an externally routed Plans directory, and the project-root tasks/ board.
//
// It is a precondition gate rather than an ordinary rule. When two branches
// edit a generated index (e.g. spec/lessons/README.md) git leaves
// `<<<<<<< `, `=======` and `>>>>>>> ` lines between two row-shaped halves;
// every other rule parses both halves as ordinary Markdown and passes, and a
// --fix pass can rewrite the index around the conflict and conceal it. So
// LintWithResult runs this checker first and, when it finds anything, reports
// only its findings and runs no fixer at all
// (cli/spec/lint#req:merge-conflict-markers-gate-precedes-rules,
// cli/spec/lint#req:merge-conflict-markers-block-fix).
type mergeConflictMarkerChecker struct {
	projectRoot string
	plansDir    string
	// skipLocalPlans is true when Plans are routed away from spec/plans (or
	// the route is broken): the local spec/plans tree is then a possibly
	// stale artifact no Plan-owned rule reads, so this gate does not read it
	// either (repo-config#req:plan-route-required).
	skipLocalPlans bool
}

func newMergeConflictMarkerChecker(projectRoot, plansDir string, routeBroken bool) *mergeConflictMarkerChecker {
	return &mergeConflictMarkerChecker{projectRoot: projectRoot, plansDir: plansDir, skipLocalPlans: routeBroken}
}

func (c *mergeConflictMarkerChecker) name() string     { return mergeConflictMarkersRule }
func (c *mergeConflictMarkerChecker) severity() string { return "error" }

func (c *mergeConflictMarkerChecker) check(specRoot string) ([]Violation, error) {
	localPlans := filepath.Join(specRoot, "plans")
	skipLocalPlans := c.skipLocalPlans
	roots := []string{specRoot}
	if c.plansDir != "" && filepath.Clean(c.plansDir) != filepath.Clean(localPlans) {
		skipLocalPlans = true
		if isDir(c.plansDir) {
			roots = append(roots, c.plansDir)
		}
	}
	projectRoot := lintProjectRoot(c.projectRoot, specRoot)
	if tasksDir := filepath.Join(projectRoot, "tasks"); isDir(tasksDir) && !isWithin(specRoot, tasksDir) {
		roots = append(roots, tasksDir)
	}

	var violations []Violation
	for _, root := range roots {
		err := conflictMarkerWalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if path != root && (strings.HasPrefix(d.Name(), ".") || d.Name() == reservedFeatureSubtree) {
					return filepath.SkipDir
				}
				if skipLocalPlans && path == localPlans {
					return filepath.SkipDir
				}
				return nil
			}
			if !d.Type().IsRegular() {
				return nil
			}
			data, err := conflictMarkerReadFile(path)
			if err != nil {
				return err
			}
			if bytes.IndexByte(data, 0) >= 0 {
				return nil // binary file: git does not write text conflict markers into it
			}
			rel, relErr := conflictMarkerRel(specRoot, path)
			if relErr != nil {
				rel = path
			}
			for _, m := range conflictMarkerLines(data) {
				violations = append(violations, Violation{
					File:     filepath.ToSlash(rel),
					Line:     m.line,
					Severity: "error",
					Rule:     mergeConflictMarkersRule,
					Message: fmt.Sprintf(
						"unresolved git merge-conflict marker %q; resolve the conflict by hand (decide the intended content) — every other rule is skipped and `spec lint --fix` modifies nothing while any marker remains",
						m.marker),
				})
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	return violations, nil
}

// conflictMarker is one marker line found in a file.
type conflictMarker struct {
	line   int
	marker string
}

// conflictMarkerLines returns every git merge-conflict marker line in data:
// a line starting with `<<<<<<< ` (ours), `||||||| ` (diff3/zdiff3 base) or
// `>>>>>>> ` (theirs), and a line that is exactly `=======` — the separator —
// but only while inside a `<<<<<<<` block, because a bare `=======` is also a
// valid Markdown setext H1 underline. CRLF line endings are tolerated.
func conflictMarkerLines(data []byte) []conflictMarker {
	var out []conflictMarker
	inBlock := false
	for i, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimSuffix(raw, "\r")
		switch {
		case isLabelledConflictMarker(line, '<'):
			out = append(out, conflictMarker{line: i + 1, marker: "<<<<<<<"})
			inBlock = true
		case isLabelledConflictMarker(line, '|'):
			out = append(out, conflictMarker{line: i + 1, marker: "|||||||"})
		case inBlock && line == strings.Repeat("=", conflictMarkerWidth):
			out = append(out, conflictMarker{line: i + 1, marker: "======="})
		case isLabelledConflictMarker(line, '>'):
			out = append(out, conflictMarker{line: i + 1, marker: ">>>>>>>"})
			inBlock = false
		}
	}
	return out
}

// isLabelledConflictMarker reports whether line is a git conflict marker made
// of exactly seven ch characters followed by a space (git always appends the
// side's label, e.g. `<<<<<<< HEAD`). Requiring the space keeps an eighth
// repeated character (a longer run, a deeper blockquote) from matching.
func isLabelledConflictMarker(line string, ch byte) bool {
	return strings.HasPrefix(line, strings.Repeat(string(ch), conflictMarkerWidth)+" ")
}

// conflictMarkerFiles returns the sorted, de-duplicated files named by
// marker violations — the set `spec lint --fix` reports as refused.
func conflictMarkerFiles(violations []Violation) []string {
	seen := map[string]bool{}
	var files []string
	for _, v := range violations {
		if !seen[v.File] {
			seen[v.File] = true
			files = append(files, v.File)
		}
	}
	sort.Strings(files)
	return files
}

// isDir reports whether path names an existing directory.
func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

// isWithin reports whether path lies inside (or is) root, lexically.
func isWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
