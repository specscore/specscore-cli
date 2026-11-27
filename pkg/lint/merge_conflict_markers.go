package lint

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/specscore/specscore-cli/pkg/conflictmarker"
	"github.com/specscore/specscore-cli/pkg/projectdef"
	"github.com/specscore/specscore-cli/pkg/rule"
)

// MergeConflictMarkersRule is the rule ID of the repository-wide git
// merge-conflict-marker gate (cli/spec/lint#req:merge-conflict-markers-detected).
const MergeConflictMarkersRule = "merge-conflict-markers"

// mergeConflictMarkersRule is the package-internal alias kept for brevity.
const mergeConflictMarkersRule = MergeConflictMarkersRule

// mergeConflictMarkerChecker scans every text file SpecScore manages for git
// merge-conflict markers left behind by an unresolved merge: the spec tree,
// an externally routed Plans directory, the project-root tasks/ board,
// specscore.yaml, and the paired skill files the rule rules read — exactly
// the files `spec lint` reads, honouring Plans routing the same way the
// Plan-owned rules do.
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
		roots = append(roots, c.plansDir)
	}
	projectRoot := lintProjectRoot(c.projectRoot, specRoot)
	if tasksDir := filepath.Join(projectRoot, "tasks"); !isWithin(specRoot, tasksDir) {
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
		rel, relErr := filepath.Rel(specRoot, path)
		if relErr != nil {
			rel = path
		}
		for _, m := range conflictmarker.Scan(data) {
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
		resolvedRoot, err := filepath.EvalSymlinks(root)
		if errors.Is(err, fs.ErrNotExist) {
			continue // no routed Plans checkout, no tasks/ board
		}
		if err != nil {
			return nil, err // e.g. a symlink loop: fail closed
		}
		if !isDir(resolvedRoot) {
			continue
		}
		resolvedLocalPlans := ""
		if root == specRoot {
			resolvedLocalPlans = filepath.Join(resolvedRoot, "plans")
		}
		err = filepath.WalkDir(resolvedRoot, func(path string, d fs.DirEntry, err error) error {
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
			data, err := os.ReadFile(path)
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

	// Single files lint reads outside the walked roots: specscore.yaml (read
	// by several rules that treat a parse error as a soft default, so a
	// conflicted config would otherwise lint clean) and, when the project
	// has a Rules tree, each paired <skills_path>/<name>/SKILL.md the rule
	// rules read.
	files := []string{filepath.Join(projectRoot, projectdef.SpecConfigFile)}
	if isDir(filepath.Join(specRoot, "rules")) {
		skillsDir := rule.SkillsDir(projectRoot, configuredSkillsPath(projectRoot))
		entries, err := os.ReadDir(skillsDir)
		if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, err // unreadable skills directory: fail closed
		}
		for _, e := range entries {
			files = append(files, filepath.Join(skillsDir, e.Name(), "SKILL.md"))
		}
	}
	for _, file := range files {
		if !isRegularFile(file) || isWithinAny(file, roots) {
			continue // absent, or already scanned by a root walk
		}
		data, err := os.ReadFile(file)
		if err != nil {
			return nil, err
		}
		report(file, data)
	}
	return violations, nil
}

// isWithinAny reports whether path lies inside any of roots, lexically.
func isWithinAny(path string, roots []string) bool {
	for _, root := range roots {
		if isWithin(root, path) {
			return true
		}
	}
	return false
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
