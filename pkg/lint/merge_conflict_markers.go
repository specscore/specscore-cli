package lint

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/specscore/specscore-cli/pkg/conflictmarker"
	"github.com/specscore/specscore-cli/pkg/projectdef"
)

// MergeConflictMarkersRule is the rule ID of the repository-wide git
// merge-conflict-marker gate (cli/spec/lint#req:merge-conflict-markers-detected).
const MergeConflictMarkersRule = "merge-conflict-markers"

// mergeConflictMarkersRule is the package-internal alias kept for brevity.
const mergeConflictMarkersRule = MergeConflictMarkersRule

// Package-level seams, swapped in tests to drive the defensive error branches
// (unreadable directory or file, unresolvable root) that real filesystem
// state cannot reliably reproduce on every host. Same convention as
// lifecycle_lock_litter.go.
var (
	conflictMarkerWalkDir      = filepath.WalkDir
	conflictMarkerReadFile     = os.ReadFile
	conflictMarkerRel          = filepath.Rel
	conflictMarkerEvalSymlinks = filepath.EvalSymlinks
)

// mergeConflictMarkerChecker scans every text file SpecScore manages for git
// merge-conflict markers left behind by an unresolved merge: the spec tree,
// an externally routed Plans directory, the project-root tasks/ board, and
// specscore.yaml.
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

// ConflictMarkers runs the merge-conflict-marker gate alone, with exactly the
// scope LintWithResult uses for opts, and returns its findings (empty when the
// managed files are clean). Producer verbs call it before mutating any file
// or preparing any event (cli/spec/lint#req:writers-refuse-conflicted-files).
func ConflictMarkers(opts Options) ([]Violation, error) {
	c := newMergeConflictMarkerChecker(opts.effectiveProjectRoot(), opts.PlansDir, opts.PlanRouteError != "")
	return checkWithTransientRemovalRetry(c, opts.SpecRoot)
}

func (c *mergeConflictMarkerChecker) check(specRoot string) ([]Violation, error) {
	localPlans := filepath.Join(specRoot, "plans")
	skipLocalPlans := c.skipLocalPlans
	var roots []string
	if isDir(specRoot) { // a migration may run before spec/ exists
		roots = append(roots, specRoot)
	}
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

	// Scan roots are resolved first: filepath.WalkDir does not descend into
	// a root that is itself a symlink, while the fixers (os.ReadDir and
	// friends) follow it, so an unresolved symlinked root would let a fixer
	// rewrite files this gate never read.
	// Findings are still reported at each file's logical (unresolved) path,
	// relative to specRoot, exactly as every other rule reports them.
	var violations []Violation
	report := func(path string, data []byte) {
		rel, relErr := conflictMarkerRel(specRoot, path)
		if relErr != nil {
			rel = path
		}
		for _, m := range conflictmarker.Scan(data, conflictmarker.IsMarkdown(path)) {
			violations = append(violations, Violation{
				File:     filepath.ToSlash(rel),
				Line:     m.Line,
				Severity: "error",
				Rule:     mergeConflictMarkersRule,
				Message: fmt.Sprintf(
					"unresolved git merge-conflict marker %q; resolve the conflict by hand (decide the intended content) — every other rule is skipped and `spec lint --fix` modifies nothing while any marker remains",
					m.Marker),
			})
		}
	}
	for _, root := range roots {
		resolvedRoot, err := conflictMarkerEvalSymlinks(root)
		if err != nil {
			return nil, err
		}
		resolvedLocalPlans := ""
		if root == specRoot {
			resolvedLocalPlans = filepath.Join(resolvedRoot, "plans")
		}
		err = conflictMarkerWalkDir(resolvedRoot, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if path != resolvedRoot && skipConflictScanDir(d.Name()) {
					return filepath.SkipDir
				}
				if skipLocalPlans && resolvedLocalPlans != "" && path == resolvedLocalPlans {
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
			// path descends from resolvedRoot (this walk's own root), so
			// filepath.Rel cannot fail; map it back under the logical root.
			inRoot, _ := filepath.Rel(resolvedRoot, path)
			report(filepath.Join(root, inRoot), data)
			return nil
		})
		if err != nil {
			return nil, err
		}
	}

	// specscore.yaml is read by several rules that treat a parse error as a
	// soft default, so a conflicted config would otherwise lint clean.
	if cfg := filepath.Join(projectRoot, projectdef.SpecConfigFile); isRegularFile(cfg) {
		data, err := conflictMarkerReadFile(cfg)
		if err != nil {
			return nil, err
		}
		report(cfg, data)
	}
	return violations, nil
}

// skipConflictScanDir reports whether a directory below a scan root is
// outside the managed set: a nested `.git` directory (an externally routed
// Plans checkout) and per-feature benchmark fixture subtrees, whose findings
// lint never reports. Other hidden directories ARE scanned, because some
// rules (oq-section) read them. The fixers that walk the tree (oqFixWalk,
// fixLegacyStatusesInTree) skip exactly the same directories and every
// symlink, so no fixer rewrites a file this gate did not read.
func skipConflictScanDir(name string) bool {
	return name == ".git" || name == reservedFeatureSubtree
}

// ConflictMarkerFiles returns the sorted, de-duplicated files named by
// marker violations — the set `spec lint --fix` reports as refused.
func ConflictMarkerFiles(violations []Violation) []string {
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

// isRegularFile reports whether path names an existing regular file
// (following symlinks).
func isRegularFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// isWithin reports whether path lies inside (or is) root, lexically.
func isWithin(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}
