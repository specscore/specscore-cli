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
type oqSectionChecker struct{}

func newOQSectionChecker() checker {
	return &oqSectionChecker{}
}

func (c *oqSectionChecker) name() string     { return "oq-section" }
func (c *oqSectionChecker) severity() string { return "error" }

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

	walkErr := filepath.Walk(specRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		// Only README.md files participate in the section-presence check.
		// The fix phase is broader (any .md), but only README.md is the
		// canonical anchor for the Open Questions section per AGENTS.md.
		if info.Name() != "README.md" {
			return nil
		}

		result, parseErr := parseOQSection(path)
		if parseErr != nil {
			return nil
		}

		relPath, _ := filepath.Rel(specRoot, path)

		switch {
		case result.legacy:
			violations = append(violations, Violation{
				File:     relPath,
				Line:     result.line,
				Severity: "error",
				Rule:     "oq-section",
				Message:  `Legacy heading "## Outstanding Questions" found; rename to "## Open Questions" (run with --fix to migrate)`,
			})
		case !result.found:
			violations = append(violations, Violation{
				File:     relPath,
				Line:     0,
				Severity: "error",
				Rule:     "oq-section",
				Message:  "Open Questions section not found",
			})
		case result.empty:
			violations = append(violations, Violation{
				File:     relPath,
				Line:     result.line,
				Severity: "warning",
				Rule:     "oq-not-empty",
				Message:  "Open Questions section appears empty",
			})
		}

		return nil
	})

	if walkErr != nil {
		return nil, walkErr
	}

	return violations, nil
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
func (c *oqSectionChecker) fix(specRoot string) error {
	info, err := os.Stat(specRoot)
	if err != nil || !info.IsDir() {
		return nil
	}

	return filepath.Walk(specRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
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

		return writeLintFile(specRoot, path, []byte(rewritten), 0o644)
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
