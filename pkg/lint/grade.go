package lint

import (
	"fmt"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/specscore/specscore-cli/pkg/projectdef"
)

// gradeRuleNames are the rule IDs implemented by gradeChecker. All four are
// registered to the single checker; per-violation filtering uses the
// Violation.Rule field, mirroring the plan-rules / issue-rules pattern.
//
// Implements the canonical-grade-metadata-field Feature (meta-spec):
//   - grade-values-shape  → REQ:grade-values-shape (config shape errors)
//   - grade-single-value  → REQ:grade-single-value (exactly one token)
//   - grade-placement     → REQ:grade-placement   (last header-block line)
//   - grade-value         → REQ:grade-value-validated (value ∈ effective set)
var gradeRuleNames = []string{"grade-values-shape", "grade-single-value", "grade-placement", "grade-value"}

// metaLineRe matches a body-metadata line of the form `**Key:** value`.
// The key may contain spaces (e.g. "Source Ideas") but no `*`.
var metaLineRe = regexp.MustCompile(`^\*\*([^*]+):\*\*[ \t]?(.*)$`)

// gradeChecker enforces the canonical `**Grade:**` body-metadata field
// uniformly across every artifact kind that has a header block. The rule is
// generic: it never gates on the artifact's Status or on any reviewer-gate
// workflow (canonical-grade-metadata-field#req:grade-generic-definition,
// #req:grade-no-status-coupling, #req:grade-artifact-scope).
type gradeChecker struct{}

func newGradeChecker() *gradeChecker { return &gradeChecker{} }

func (c *gradeChecker) name() string     { return "grade-value" }
func (c *gradeChecker) severity() string { return "error" }

// check reads the effective grade value set and validates every `**Grade:**`
// line found in any markdown artifact under specRoot.
func (c *gradeChecker) check(specRoot string) ([]Violation, error) {
	projectRoot := filepath.Dir(specRoot)
	cfg, err := projectdef.ReadSpecConfig(projectRoot)
	if err != nil {
		// No / unreadable specscore.yaml: other rules surface that; the grade
		// rule proceeds with the built-in default value set so a bare spec
		// tree still validates grade values.
		cfg = projectdef.SpecConfig{}
	}

	if shapeErr := cfg.GradeShapeError(); shapeErr != "" {
		// Malformed grade.values: one hard error, no implicit set substituted
		// (canonical-grade-metadata-field#req:grade-values-shape). Skip the
		// per-file value checks — there is no valid set to validate against.
		return []Violation{{
			File:     projectdef.SpecConfigFile,
			Line:     0,
			Severity: "error",
			Rule:     "grade-values-shape",
			Message:  shapeErr,
		}}, nil
	}

	values := cfg.EffectiveGradeValues()
	valSet := make(map[string]bool, len(values))
	for _, v := range values {
		valSet[v] = true
	}

	var violations []Violation
	if cfg.GradeValuesHasDuplicates() {
		violations = append(violations, Violation{
			File:     projectdef.SpecConfigFile,
			Line:     0,
			Severity: "warning",
			Rule:     "grade-values-shape",
			Message:  "grade.values contains duplicate tokens; duplicates are de-duplicated (canonical-grade-metadata-field#req:grade-values-shape)",
		})
	}

	walkErr := walkMatchingFiles(specRoot,
		func(_ string, _ int, name string) bool { return strings.HasSuffix(name, ".md") },
		func(path string, content []byte) {
			rel, _ := filepath.Rel(specRoot, path)
			rel = filepath.ToSlash(rel)
			violations = append(violations, checkGradeInFile(rel, string(content), valSet, values)...)
		})
	if walkErr != nil {
		return nil, walkErr
	}
	return violations, nil
}

// checkGradeInFile validates the `**Grade:**` line(s) of a single artifact.
func checkGradeInFile(rel, content string, valSet map[string]bool, values []string) []Violation {
	lines := strings.Split(content, "\n")

	var gradeIdx []int
	for i, l := range lines {
		if k, _, ok := parseMetaLine(l); ok && k == "Grade" {
			gradeIdx = append(gradeIdx, i)
		}
	}
	if len(gradeIdx) == 0 {
		return nil // optional — absence is valid
	}

	if len(gradeIdx) > 1 {
		return []Violation{{
			File:     rel,
			Line:     gradeIdx[1] + 1,
			Severity: "error",
			Rule:     "grade-placement",
			Message:  "multiple **Grade:** lines; an artifact MUST carry at most one (canonical-grade-metadata-field#req:grade-single-value)",
		}}
	}

	gi := gradeIdx[0]
	var violations []Violation

	bs, be := headerBlockRange(lines)
	switch {
	case bs < 0 || gi < bs || gi >= be:
		violations = append(violations, Violation{
			File:     rel,
			Line:     gi + 1,
			Severity: "error",
			Rule:     "grade-placement",
			Message:  "**Grade:** must appear in the header block as the last metadata line, after **Status:** (canonical-grade-metadata-field#req:grade-placement)",
		})
	case gi != be-1:
		violations = append(violations, Violation{
			File:     rel,
			Line:     gi + 1,
			Severity: "error",
			Rule:     "grade-placement",
			Message:  "**Grade:** must be the last header-block line, after **Status:** and any **Source Ideas:**/**Supersedes:** (canonical-grade-metadata-field#req:grade-placement)",
		})
	}

	_, val, _ := parseMetaLine(lines[gi])
	violations = append(violations, validateGradeValue(rel, gi+1, val, valSet, values)...)
	return violations
}

// validateGradeValue enforces single-token (#req:grade-single-value) and
// value-membership (#req:grade-value-validated).
func validateGradeValue(rel string, lineNo int, raw string, valSet map[string]bool, values []string) []Violation {
	v := strings.TrimSpace(raw)
	if v == "" {
		return []Violation{{
			File:     rel,
			Line:     lineNo,
			Severity: "error",
			Rule:     "grade-single-value",
			Message:  "**Grade:** must carry exactly one value; found an empty value (canonical-grade-metadata-field#req:grade-single-value)",
		}}
	}
	if strings.ContainsAny(v, " \t,") {
		return []Violation{{
			File:     rel,
			Line:     lineNo,
			Severity: "error",
			Rule:     "grade-single-value",
			Message:  fmt.Sprintf("**Grade:** must carry exactly one value; found multiple tokens %q (canonical-grade-metadata-field#req:grade-single-value)", v),
		}}
	}
	if !valSet[v] {
		return []Violation{{
			File:     rel,
			Line:     lineNo,
			Severity: "error",
			Rule:     "grade-value",
			Message:  fmt.Sprintf("grade value %q is not in the effective value set [%s] (canonical-grade-metadata-field#req:grade-value-validated)", v, strings.Join(values, ", ")),
		}}
	}
	return nil
}

// fix implements the fixer interface: normalize a single misplaced
// `**Grade:**` line to the last position of the header block
// (canonical-grade-metadata-field#req:grade-placement, --fix normalization).
// Value problems are never auto-fixed; multi-grade files are left untouched.
func (c *gradeChecker) fix(specRoot string) error {
	return walkMatchingFiles(specRoot,
		func(_ string, _ int, name string) bool { return strings.HasSuffix(name, ".md") },
		func(path string, content []byte) {
			s := string(content)
			trailingLF := strings.HasSuffix(s, "\n")
			lines := strings.Split(s, "\n")
			if trailingLF && len(lines) > 0 && lines[len(lines)-1] == "" {
				lines = lines[:len(lines)-1]
			}

			var gradeIdx []int
			for i, l := range lines {
				if k, _, ok := parseMetaLine(l); ok && k == "Grade" {
					gradeIdx = append(gradeIdx, i)
				}
			}
			if len(gradeIdx) != 1 {
				return // only the single-grade case is normalizable
			}
			gi := gradeIdx[0]
			gradeLine := lines[gi]

			// Remove the grade line, then recompute the header block on the
			// remaining lines and reinsert the grade as the last metadata line.
			rest := append([]string{}, lines[:gi]...)
			rest = append(rest, lines[gi+1:]...)
			bs, be := headerBlockRange(rest)
			if bs < 0 {
				return // grade was the only metadata line — nothing to anchor to
			}
			out := append([]string{}, rest[:be]...)
			out = append(out, gradeLine)
			out = append(out, rest[be:]...)

			if equalLines(out, lines) {
				return // already canonical — idempotent no-op
			}
			writeBack(path, out, trailingLF)
		})
}

// parseMetaLine parses a `**Key:** value` body-metadata line. ok is false
// when the line is not a metadata line.
func parseMetaLine(line string) (key, value string, ok bool) {
	m := metaLineRe.FindStringSubmatch(line)
	if m == nil {
		return "", "", false
	}
	return strings.TrimSpace(m[1]), m[2], true
}

// headerBlockRange returns [start, end) line indices of the contiguous run of
// body-metadata lines that forms the artifact's header block — the metadata
// run after the title, skipping blank lines and a leading studio-toolbar
// blockquote. Returns (-1, -1) when there is no header block.
func headerBlockRange(lines []string) (int, int) {
	i := 0
	for ; i < len(lines); i++ {
		if strings.HasPrefix(lines[i], "# ") {
			i++ // start scanning after the title
			break
		}
	}
	start := -1
	for ; i < len(lines); i++ {
		if _, _, ok := parseMetaLine(lines[i]); ok {
			start = i
			break
		}
		if strings.TrimSpace(lines[i]) == "" || strings.HasPrefix(lines[i], "> ") {
			continue // tolerate blank lines and the toolbar blockquote
		}
		break // hit content before any metadata
	}
	if start < 0 {
		return -1, -1
	}
	end := start
	for end < len(lines) {
		if _, _, ok := parseMetaLine(lines[end]); !ok {
			break
		}
		end++
	}
	return start, end
}

// equalLines reports whether two line slices are identical.
func equalLines(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
