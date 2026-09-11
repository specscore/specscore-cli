package lint

import (
	"bufio"
	"os"
	"path/filepath"
	"strings"
)

// oqSectionChecker verifies that feature/plan READMEs have an Open Questions
// section. A legacy "## Outstanding Questions" heading is reported as a
// distinct violation and is autofixable: --fix rewrites the heading line
// in place to "## Open Questions".
type oqSectionChecker struct {
	projectRoot, plansDir string
	// skipPlans excludes spec/plans from check()/fix() entirely, without
	// ever reading it — set only when Plan routing is configured but failed
	// to resolve (finding 1 / repo-config#req:plan-route-required), so this
	// rule never falls back to the possibly-stale local plans tree the way
	// an empty plansDir normally would. See adherenceFooterChecker.skipPlans
	// for the fuller rationale.
	skipPlans bool
}

func newOQSectionChecker(projectRoot ...string) checker {
	var root string
	if len(projectRoot) > 0 {
		root = projectRoot[0]
	}
	var plans string
	if len(projectRoot) > 1 {
		plans = projectRoot[1]
	}
	return &oqSectionChecker{projectRoot: root, plansDir: plans}
}

// newOQSectionCheckerSkipPlans returns an oq-section checker configured for
// a configured-but-broken Plan route: see skipPlans.
func newOQSectionCheckerSkipPlans(projectRoot string) checker {
	return &oqSectionChecker{projectRoot: projectRoot, skipPlans: true}
}

func (c *oqSectionChecker) name() string     { return "oq-section" }
func (c *oqSectionChecker) severity() string { return "error" }

// routeErrorChecker implements planOwnedChecker: see linter.go's
// registerPlanOwned, the ONE place that decides which checker to register
// when Plan routing is configured but broken.
func (c *oqSectionChecker) routeErrorChecker() checker {
	return newOQSectionCheckerSkipPlans(c.projectRoot)
}

// isPlansSubtreePath reports whether path is root/plans itself or a
// descendant of it (slash-normalized comparison against the root-relative
// path). Used to exclude the local spec/plans mirror from a walk rooted at
// specRoot when Plan routing supplies a different plansDir, or when routing
// is configured but broken — mirroring readmeExistsChecker's excludePlans.
// Every call site passes a path filepath.Walk(root, ...) itself produced, so
// path is always a descendant of root and filepath.Rel cannot fail — the
// error is intentionally ignored, matching readmeExistsChecker's own
// `rel, _ := filepath.Rel(...)` convention for the identical situation.
func isPlansSubtreePath(root, path string) bool {
	rel, _ := filepath.Rel(root, path)
	rel = filepath.ToSlash(rel)
	return rel == "plans" || strings.HasPrefix(rel, "plans/")
}

// Canonical and legacy heading text. Detection is line-exact (after trim)
// to avoid matching headings like "## Open Questions and Concerns".
const (
	oqCanonicalHeading = "## Open Questions"
	oqLegacyHeading    = "## Outstanding Questions"
)

func (c *oqSectionChecker) check(specRoot string) ([]Violation, error) {
	var violations []Violation

	info, err := os.Stat(specRoot)
	if err != nil || !info.IsDir() {
		return violations, nil
	}

	// excludePlans is true whenever the local spec/plans mirror under
	// specRoot must be excluded from this walk: either Plan routing
	// supplies a different plansDir to walk in its place (below), or
	// routing is configured but broken (c.skipPlans), in which case
	// spec/plans is skipped entirely and nothing replaces it — mirroring
	// readmeExistsChecker's excludePlans shape (finding 1 /
	// repo-config#req:plan-route-required: never read or write the local
	// spec/plans tree once a route is broken).
	excludePlans := c.plansDir != "" || c.skipPlans
	walkErr := filepath.Walk(specRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if excludePlans && isPlansSubtreePath(specRoot, path) {
				return filepath.SkipDir
			}
			return nil
		}
		// Only README.md files participate in the section-presence check.
		// The fix phase is broader (any .md), but only README.md is the
		// canonical anchor for the Open Questions section per AGENTS.md.
		if info.Name() != "README.md" {
			return nil
		}
		relPath, _ := filepath.Rel(specRoot, path)
		if v := oqSectionViolation(path, relPath); v != nil {
			violations = append(violations, *v)
		}
		return nil
	})
	if walkErr != nil {
		return nil, walkErr
	}

	if !c.skipPlans && c.plansDir != "" {
		plansErr := filepath.Walk(c.plansDir, func(path string, info os.FileInfo, err error) error {
			if err != nil {
				if os.IsNotExist(err) {
					return nil
				}
				return err
			}
			if info.IsDir() {
				return nil
			}
			if info.Name() != "README.md" {
				return nil
			}
			// path always descends from c.plansDir here (this walk's own
			// root), so filepath.Rel cannot fail — ignored, matching
			// readmeExistsChecker's convention for the identical situation.
			rel, _ := filepath.Rel(c.plansDir, path)
			relPath := filepath.Join("plans", rel)
			if v := oqSectionViolation(path, relPath); v != nil {
				violations = append(violations, *v)
			}
			return nil
		})
		if plansErr != nil {
			return nil, plansErr
		}
	}

	return violations, nil
}

// oqSectionViolation parses one README.md and returns the single violation
// (if any) it should report, with File already set to relPath. A file that
// fails to parse reports nothing (matches the pre-existing check() behavior:
// parseErr was silently swallowed there too).
func oqSectionViolation(path, relPath string) *Violation {
	result, parseErr := parseOQSection(path)
	if parseErr != nil {
		return nil
	}
	switch {
	case result.legacy:
		return &Violation{
			File:     relPath,
			Line:     result.line,
			Severity: "error",
			Rule:     "oq-section",
			Message:  `Legacy heading "## Outstanding Questions" found; rename to "## Open Questions" (run with --fix to migrate)`,
		}
	case !result.found:
		return &Violation{
			File:     relPath,
			Line:     0,
			Severity: "error",
			Rule:     "oq-section",
			Message:  "Open Questions section not found",
		}
	case result.empty:
		return &Violation{
			File:     relPath,
			Line:     result.line,
			Severity: "warning",
			Rule:     "oq-not-empty",
			Message:  "Open Questions section appears empty",
		}
	}
	return nil
}

// fix rewrites any legacy "## Outstanding Questions" heading line to the
// canonical "## Open Questions" heading, leaving the rest of the file
// byte-for-byte unchanged. Prose, code blocks, and anchor identifiers
// that mention "Outstanding Questions" are NOT touched.
//
// Walks every .md file under spec/ recursively — including the root
// spec/README.md, sibling subtrees like spec/research/ and
// spec/decisions/, single-file Idea artifacts, and nested
// feature/plan READMEs. The fix is broader than the check (which only
// considers README.md files): legacy headings in non-README .md files
// are rare but possible (e.g., Idea slug files), and rewriting them in
// the same pass means one `--fix` invocation migrates the whole tree.
//
// The local spec/plans mirror under specRoot is excluded exactly as in
// check() — replaced by a separate walk of c.plansDir when Plan routing
// supplies one, or skipped with nothing to replace it when routing is
// configured but broken (c.skipPlans) — so --fix never writes to spec/plans
// once a route is broken (finding 1 / repo-config#req:plan-route-required).
func (c *oqSectionChecker) fix(specRoot string) error {
	excludePlans := c.plansDir != "" || c.skipPlans
	if err := oqFixWalk(c.projectRoot, specRoot, specRoot, func(path string) bool {
		return excludePlans && isPlansSubtreePath(specRoot, path)
	}); err != nil {
		return err
	}
	if !c.skipPlans && c.plansDir != "" {
		return oqFixWalk(c.projectRoot, specRoot, c.plansDir, nil)
	}
	return nil
}

// oqFixWalk rewrites every legacy "## Outstanding Questions" heading under
// walkRoot to the canonical "## Open Questions" heading. skipDir, when
// non-nil, is consulted for every directory Walk visits and, when true,
// prunes that subtree via filepath.SkipDir without ever reading it.
// specRoot is passed through to writeLintFile unchanged regardless of
// walkRoot (which may be an externally-routed plansDir outside specRoot
// entirely) — writeLintFile already tolerates a path outside specRoot.
func oqFixWalk(projectRoot, specRoot, walkRoot string, skipDir func(path string) bool) error {
	info, err := os.Stat(walkRoot)
	if err != nil || !info.IsDir() {
		return nil
	}
	return filepath.Walk(walkRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if skipDir != nil && skipDir(path) {
				return filepath.SkipDir
			}
			return nil
		}
		if filepath.Ext(path) != ".md" {
			return nil
		}

		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return nil
		}

		rewritten, changed := rewriteLegacyOQHeading(string(content))
		if !changed {
			return nil
		}

		return writeLintFile(projectRoot, specRoot, path, content, []byte(rewritten), 0o644)
	})
}

// rewriteLegacyOQHeading replaces any line whose trimmed form equals
// "## Outstanding Questions" with the canonical "## Open Questions".
// Returns the rewritten text and a flag indicating whether any line was
// changed. The transform is line-scoped: a line containing the phrase
// inside prose, code, or anchors is left alone.
func rewriteLegacyOQHeading(s string) (string, bool) {
	if !strings.Contains(s, oqLegacyHeading) {
		return s, false
	}
	lines := strings.Split(s, "\n")
	changed := false
	for i, line := range lines {
		if strings.TrimRight(line, " \t") == oqLegacyHeading {
			lines[i] = oqCanonicalHeading
			changed = true
		}
	}
	if !changed {
		return s, false
	}
	return strings.Join(lines, "\n"), true
}

type oqResult struct {
	found  bool
	legacy bool
	empty  bool
	line   int
}

// parseOQSection scans a README for the canonical "## Open Questions"
// heading, the legacy "## Outstanding Questions" heading, and (when the
// canonical heading is found) whether the section has content. The
// legacy heading is detected as a distinct condition so callers can
// emit a dedicated, actionable violation.
func parseOQSection(readmePath string) (oqResult, error) {
	file, err := os.Open(readmePath)
	if err != nil {
		return oqResult{}, err
	}
	defer func() { _ = file.Close() }()

	scanner := bufio.NewScanner(file)
	lineNum := 0

	for scanner.Scan() {
		lineNum++
		line := scanner.Text()
		trimmed := strings.TrimRight(line, " \t")

		if trimmed == oqLegacyHeading {
			return oqResult{legacy: true, line: lineNum}, nil
		}

		if trimmed != oqCanonicalHeading {
			continue
		}

		oqLine := lineNum

		// Scan forward to see if the section has content.
		for scanner.Scan() {
			lineNum++
			next := strings.TrimSpace(scanner.Text())
			if next == "" {
				continue
			}
			// A new heading means the OQ section was empty.
			if strings.HasPrefix(next, "#") {
				return oqResult{found: true, empty: true, line: oqLine}, nil
			}
			// Any non-blank, non-heading content means it's populated.
			return oqResult{found: true, empty: false, line: oqLine}, nil
		}

		// OQ heading was the last thing in the file with no content after it.
		return oqResult{found: true, empty: true, line: oqLine}, nil
	}

	return oqResult{found: false}, scanner.Err()
}
