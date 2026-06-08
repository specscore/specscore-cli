package lint

// Features implemented: cli/spec/lint/feature-rules

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// featureRulesChecker implements feature-level lint rules. Today it carries a
// single rule, feature-source-ideas-required, which enforces that every Feature
// README carries a **Source Ideas:** body-metadata line with an explicit
// sentinel (— / none) or a slug list, and (under --fix) backfills the sentinel
// on Features that omit the line (cli/spec/lint/feature-rules).
type featureRulesChecker struct {
	autofix bool
}

func newFeatureRulesChecker() *featureRulesChecker { return &featureRulesChecker{} }

func (c *featureRulesChecker) name() string     { return "feature-source-ideas-required" }
func (c *featureRulesChecker) severity() string { return "error" }

// featureSourceIdeas inspects a single README's lines. isFeature is true when
// the file is a Feature README (has a `# Feature: ` H1). present/value report
// the **Source Ideas:** line; sourceLine/statusLine are 1-based line numbers
// (0 when absent).
func featureSourceIdeas(content []byte) (isFeature, present bool, value string, sourceLine, statusLine int) {
	lines := strings.Split(string(content), "\n")
	for i, l := range lines {
		t := strings.TrimSpace(l)
		if !isFeature && strings.HasPrefix(t, "# Feature: ") {
			isFeature = true
		}
		if statusLine == 0 && strings.HasPrefix(t, "**Status:**") {
			statusLine = i + 1
		}
		if !present && strings.HasPrefix(t, "**Source Ideas:**") {
			present = true
			value = strings.TrimSpace(strings.TrimPrefix(t, "**Source Ideas:**"))
			sourceLine = i + 1
		}
	}
	return isFeature, present, value, sourceLine, statusLine
}

func (c *featureRulesChecker) check(specRoot string) ([]Violation, error) {
	var out []Violation
	err := walkFeatureReadmes(specRoot, func(readmePath string, content []byte) {
		isFeature, present, value, sourceLine, statusLine := featureSourceIdeas(content)
		if !isFeature {
			return
		}
		relPath, _ := filepath.Rel(specRoot, readmePath)
		if !present {
			line := statusLine
			if line == 0 {
				line = 1
			}
			out = append(out, Violation{
				File: relPath, Line: line, Severity: "error", Rule: "feature-source-ideas-required",
				Message: "Feature is missing a **Source Ideas:** line (use `—` / `none` for no upstream Idea, or a comma-separated Idea slug list)",
			})
			return
		}
		if value == "" {
			out = append(out, Violation{
				File: relPath, Line: sourceLine, Severity: "error", Rule: "feature-source-ideas-required",
				Message: "**Source Ideas:** has an empty value; use `—` / `none`, or a comma-separated Idea slug list",
			})
		}
	})
	if err != nil {
		return nil, err
	}
	// At most one violation per Feature README, so a stable sort by file path
	// fully determines the order.
	sort.SliceStable(out, func(i, j int) bool { return out[i].File < out[j].File })
	return out, nil
}

// fix backfills `**Source Ideas:** —` after the `**Status:**` line on every
// Feature README that omits the line. Features that already carry a
// **Source Ideas:** line (any value) are left untouched. Idempotent.
func (c *featureRulesChecker) fix(specRoot string) error {
	return walkFeatureReadmes(specRoot, func(readmePath string, content []byte) {
		isFeature, present, _, _, statusLine := featureSourceIdeas(content)
		if !isFeature || present || statusLine == 0 {
			return
		}
		lines := strings.Split(string(content), "\n")
		// Insert the sentinel line immediately after the Status line.
		idx := statusLine // 0-based position to insert AT (== after statusLine-1)
		newLines := make([]string, 0, len(lines)+1)
		newLines = append(newLines, lines[:idx]...)
		newLines = append(newLines, "**Source Ideas:** —")
		newLines = append(newLines, lines[idx:]...)
		_ = os.WriteFile(readmePath, []byte(strings.Join(newLines, "\n")), 0o644)
	})
}
