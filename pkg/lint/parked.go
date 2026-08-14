package lint

import (
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"github.com/specscore/specscore-cli/pkg/projectdef"
)

// parkedRuleNames are the rule IDs implemented by parkedChecker. Both are
// registered to the single checker; per-violation filtering uses the
// Violation.Rule field, mirroring the grade / plan-rules / issue-rules
// pattern (one checker, several rule IDs with different severities).
//
//   - parked-shape (error)   — a `**Parked:** true` axis MUST carry a
//     non-empty **Parked Reason:** and a well-formed **Parked Date:**
//     (cli/parked#req:reason-and-date-required).
//   - parked-stale (warning) — surfaces artifacts parked longer than the
//     configured review window, so a park never quietly becomes permanent
//     (cli/parked#req:stale-surfaced).
var parkedRuleNames = []string{"parked-shape", "parked-stale"}

// dateLayout is the repo-wide ISO-8601 date convention (see
// pkg/lifecycle.parkedTodayUTC and every kind's scaffold).
const dateLayout = "2006-01-02"

// parkedChecker enforces the parked axis uniformly across every artifact
// kind that carries a **Status:** header block — the same generic,
// kind-agnostic scan `gradeChecker` uses for **Grade:**, since `**Parked:**`
// is likewise a plain body-metadata line rather than a per-kind construct.
type parkedChecker struct{ projectRoot string }

func newParkedChecker(projectRoot ...string) *parkedChecker {
	var root string
	if len(projectRoot) > 0 {
		root = projectRoot[0]
	}
	return &parkedChecker{projectRoot: root}
}

func (c *parkedChecker) name() string     { return "parked-shape" }
func (c *parkedChecker) severity() string { return "error" }

func (c *parkedChecker) check(specRoot string) ([]Violation, error) {
	projectRoot := lintProjectRoot(c.projectRoot, specRoot)
	cfg, err := projectdef.ReadSpecConfig(projectRoot)
	if err != nil {
		// No / unreadable specscore.yaml: other rules surface that; this
		// rule proceeds with the built-in default review window.
		cfg = projectdef.SpecConfig{}
	}
	staleDays := cfg.EffectiveParkedStaleDays()

	var violations []Violation
	walkErr := walkMatchingFiles(specRoot,
		func(_ string, _ int, name string) bool { return strings.HasSuffix(name, ".md") },
		func(path string, content []byte) {
			rel, _ := filepath.Rel(specRoot, path)
			rel = filepath.ToSlash(rel)
			violations = append(violations, checkParkedInFile(rel, string(content), staleDays)...)
		})
	if walkErr != nil {
		return nil, walkErr
	}
	return violations, nil
}

// checkParkedInFile validates the parked axis of a single artifact. Absence
// of `**Parked:** true` is valid (the axis is optional) and produces no
// violations — including when a dangling **Parked Reason:**/**Parked
// Date:** line exists without it, which is surprising hand-editing but not
// this rule's concern.
func checkParkedInFile(rel, content string, staleDays int) []Violation {
	lines := strings.Split(content, "\n")

	var parked bool
	var parkedLine int
	var reason, date string
	var dateLine int
	for i, l := range lines {
		key, val, ok := parseMetaLine(l)
		if !ok {
			continue
		}
		switch key {
		case "Parked":
			parked = strings.EqualFold(strings.TrimSpace(val), "true")
			parkedLine = i + 1
		case "Parked Reason":
			reason = strings.TrimSpace(val)
		case "Parked Date":
			date = strings.TrimSpace(val)
			dateLine = i + 1
		}
	}
	if !parked {
		return nil
	}

	var out []Violation
	if reason == "" {
		out = append(out, Violation{
			File: rel, Line: parkedLine, Severity: "error", Rule: "parked-shape",
			Message: "**Parked:** true requires a non-empty **Parked Reason:** line " +
				"(use `specscore <kind> park <slug> --reason \"...\"` rather than hand-editing the header)",
		})
	}
	switch date {
	case "":
		out = append(out, Violation{
			File: rel, Line: parkedLine, Severity: "error", Rule: "parked-shape",
			Message: "**Parked:** true requires a **Parked Date:** line " +
				"(use `specscore <kind> park <slug> --reason \"...\"` rather than hand-editing the header)",
		})
	default:
		parsed, err := time.Parse(dateLayout, date)
		if err != nil {
			out = append(out, Violation{
				File: rel, Line: dateLine, Severity: "error", Rule: "parked-shape",
				Message: fmt.Sprintf("**Parked Date:** %q is not a valid YYYY-MM-DD date", date),
			})
		} else if ageDays := int(time.Now().UTC().Sub(parsed).Hours() / 24); ageDays > staleDays {
			out = append(out, Violation{
				File: rel, Line: dateLine, Severity: "warning", Rule: "parked-stale",
				Message: fmt.Sprintf(
					"parked %d days ago, past the %d-day review window; revisit it — unpark, or re-run "+
						"`park --reason` with a fresh reason to confirm it is still deliberately deferred",
					ageDays, staleDays),
			})
		}
	}
	return out
}
