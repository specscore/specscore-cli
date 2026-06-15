package lint

// Issue lint rules (I-001..I-015) per the cli/spec/lint/issue-rules
// Feature. This file holds the single checker that the linter registers
// under all 15 rule IDs (mirroring the planRulesChecker pattern in
// plan_rules.go). Only I-009 (dual-location) has logic in this initial
// scaffold; the remaining 14 rules land in subsequent Plan tasks and
// currently return no violations.

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/specscore/specscore-cli/pkg/issue"
	"gopkg.in/yaml.v3"
)

// issueDiscoverAll is injectable; tests may replace it to simulate errors.
var issueDiscoverAll = issue.DiscoverAll

// issueParseFn is injectable; tests may replace it to simulate parse errors.
var issueParseFn = issue.Parse

// osMkdirAllFn is injectable; tests may replace it to simulate errors.
var osMkdirAllFn = os.MkdirAll

// issueRequiredKeys names the five always-required frontmatter fields
// for an `issue` artifact (rule I-001).
var issueRequiredKeys = []string{
	"type",
	"slug",
	"status",
	"captured_at",
	"captured_by",
}

// issueOptionalKeys names the optional frontmatter fields whose presence
// alone is allowed (shape validation lives in later rules). Together with
// issueRequiredKeys these form the closed "known keys" set used by I-001's
// unknown-field check.
//
// `format` is the artifact-frontmatter-convention type URL that the
// published `issue new` template emits; it is accepted here so scaffolded
// issues lint clean. It is optional (not required) so issues created by the
// embedded scaffolder, which omits it, also remain valid.
var issueOptionalKeys = []string{
	"format",
	"severity",
	"affected_component",
	"first_seen",
	"github_issue",
	"rejection_reason",
	"rejection_notes",
	"bugs",
}

var issueKnownKeySet = func() map[string]bool {
	m := make(map[string]bool, len(issueRequiredKeys)+len(issueOptionalKeys))
	for _, k := range issueRequiredKeys {
		m[k] = true
	}
	for _, k := range issueOptionalKeys {
		m[k] = true
	}
	return m
}()

// issueStatusValues enumerates the four valid `status` values per
// rule I-002.
var issueStatusValues = []string{"open", "investigating", "resolved", "rejected"}

var issueStatusValueSet = func() map[string]bool {
	m := make(map[string]bool, len(issueStatusValues))
	for _, v := range issueStatusValues {
		m[v] = true
	}
	return m
}()

// issueSeverityValues enumerates the five valid `severity` values per
// rule I-003.
var issueSeverityValues = []string{"low", "medium", "high", "critical", "unset"}

var issueSeverityValueSet = func() map[string]bool {
	m := make(map[string]bool, len(issueSeverityValues))
	for _, v := range issueSeverityValues {
		m[v] = true
	}
	return m
}()

// issueNonEmptyStringOptionals names the optional frontmatter fields
// that I-003 requires to be a non-empty string when present. The
// `severity` enum and `bugs` list are validated separately.
var issueNonEmptyStringOptionals = []string{
	"affected_component",
	"first_seen",
	"github_issue",
	"rejection_reason",
	"rejection_notes",
}

// issueTransitionStatuses names the three `status` values that trigger
// I-005's severity-required-on-transition check. Once an issue moves out
// of `open`, severity must be set to a concrete level.
var issueTransitionStatuses = map[string]bool{
	"investigating": true,
	"resolved":      true,
	"rejected":      true,
}

// issueRejectionReasonValues enumerates the six valid `rejection_reason`
// enum values per rule I-006.
var issueRejectionReasonValues = []string{
	"not-a-defect",
	"wont-fix",
	"duplicate",
	"not-reproducible",
	"by-design",
	"deferred",
}

var issueRejectionReasonValueSet = func() map[string]bool {
	m := make(map[string]bool, len(issueRejectionReasonValues))
	for _, v := range issueRejectionReasonValues {
		m[v] = true
	}
	return m
}()

// issueRuleIDs enumerates the 15 canonical rule IDs in order.
var issueRuleIDs = []string{
	"I-001", "I-002", "I-003", "I-004", "I-005",
	"I-006", "I-007", "I-008", "I-009", "I-010",
	"I-011", "I-012", "I-013", "I-014", "I-015",
}

type issueRulesChecker struct{}

func newIssueRulesChecker() checker { return &issueRulesChecker{} }

// name returns the primary rule ID. The checker is registered under all
// 15 IDs in linter.go so that --rules / --ignore work per-rule.
func (c *issueRulesChecker) name() string     { return "I-001" }
func (c *issueRulesChecker) severity() string { return "error" }

func (c *issueRulesChecker) check(specRoot string) ([]Violation, error) {
	discovered, err := issue.DiscoverAll(specRoot)
	if err != nil {
		return nil, fmt.Errorf("discovering issue artifacts: %w", err)
	}

	var violations []Violation
	violations = append(violations, lintI009(specRoot, discovered)...)
	violations = append(violations, lintI001AndI002(specRoot, discovered)...)
	violations = append(violations, lintI011(discovered)...)
	violations = append(violations, lintI013AndI014(specRoot, discovered)...)
	violations = append(violations, lintI015(specRoot, discovered)...)

	// Stable order: by file, then rule.
	sort.SliceStable(violations, func(i, j int) bool {
		if violations[i].File != violations[j].File {
			return violations[i].File < violations[j].File
		}
		return violations[i].Rule < violations[j].Rule
	})
	return violations, nil
}

// fix implements the fixer interface for the issue rules checker. It
// scaffolds any missing root or Feature-scoped `issues/README.md` index
// that I-013/I-014 would otherwise flag. The fix is idempotent — files
// that already exist are left untouched. Only I-013 and I-014 are
// auto-fixed; I-015 (column-shape) is intentionally not auto-fixed
// because column drift usually signals a different schema choice and
// silently rewriting it would destroy authorial intent.
func (c *issueRulesChecker) fix(specRoot string) error {
	discovered, err := issueDiscoverAll(specRoot)
	if err != nil {
		return fmt.Errorf("discovering issue artifacts: %w", err)
	}
	missing := missingIndexPaths(specRoot, discovered)
	for _, m := range missing {
		if err := osMkdirAllFn(filepath.Dir(m.absPath), 0o755); err != nil {
			return fmt.Errorf("creating directory for %s: %w", m.relPath, err)
		}
		content := issueIndexTemplate(m.h1)
		if err := os.WriteFile(m.absPath, []byte(content), 0o644); err != nil {
			return fmt.Errorf("writing %s: %w", m.relPath, err)
		}
	}
	return nil
}

// missingIndex is the (rule, absPath, relPath, h1) tuple shared between
// lint I-013/I-014 detection and the --fix scaffolder.
type missingIndex struct {
	rule    string
	absPath string
	relPath string
	h1      string
}

// missingIndexPaths returns every root or Feature-scoped issues
// directory that contains ≥1 on-pattern issue artifact but has no
// README.md. Off-pattern (I-009) files are not considered — those have
// no canonical home directory.
//
// Output is sorted by relPath for deterministic ordering.
func missingIndexPaths(specRoot string, discovered []issue.Discovered) []missingIndex {
	hasRoot := false
	featureSlugs := make(map[string]bool)
	for _, d := range discovered {
		if !d.MatchesPattern {
			continue
		}
		if d.FeatureSlug == "" {
			hasRoot = true
		} else {
			featureSlugs[d.FeatureSlug] = true
		}
	}

	var out []missingIndex
	if hasRoot {
		abs := filepath.Join(specRoot, "issues", "README.md")
		if _, err := os.Stat(abs); err != nil {
			out = append(out, missingIndex{
				rule:    "I-013",
				absPath: abs,
				relPath: "issues/README.md",
				h1:      "Issues",
			})
		}
	}
	var slugs []string
	for s := range featureSlugs {
		slugs = append(slugs, s)
	}
	sort.Strings(slugs)
	for _, s := range slugs {
		abs := filepath.Join(specRoot, "features", s, "issues", "README.md")
		if _, err := os.Stat(abs); err != nil {
			out = append(out, missingIndex{
				rule:    "I-014",
				absPath: abs,
				relPath: "features/" + s + "/issues/README.md",
				h1:      "Issues",
			})
		}
	}
	return out
}

// issueIndexColumns names the five required Contents-table column
// headers in canonical order per rule I-015.
var issueIndexColumns = []string{"Slug", "Title", "Status", "Severity", "Captured"}

// issueIndexTemplate returns the canonical lint-clean issues-index
// README body used by --fix to scaffold a missing I-013/I-014 README.
// The result is intentionally minimal: type: index frontmatter,
// `**Status:** Stable`, a single empty Contents table with the five
// canonical column headers, an empty Open Questions section, and the
// SpecScore adherence footer.
func issueIndexTemplate(h1 string) string {
	var b strings.Builder
	b.WriteString("---\n")
	b.WriteString("type: index\n")
	b.WriteString("---\n")
	b.WriteString("\n")
	b.WriteString("**Status:** Stable\n")
	b.WriteString("\n")
	b.WriteString("# ")
	b.WriteString(h1)
	b.WriteString("\n")
	b.WriteString("\n")
	b.WriteString("## Contents\n")
	b.WriteString("\n")
	b.WriteString("| ")
	b.WriteString(strings.Join(issueIndexColumns, " | "))
	b.WriteString(" |\n")
	b.WriteString("| --- | --- | --- | --- | --- |\n")
	b.WriteString("\n")
	b.WriteString("## Open Questions\n")
	b.WriteString("\n")
	b.WriteString("None at this time.\n")
	b.WriteString("\n")
	b.WriteString("---\n")
	b.WriteString("*This document follows the https://specscore.md/issues-index-specification*\n")
	return b.String()
}

// lintI013AndI014 emits one violation per missing index README. I-013
// covers the root `spec/issues/README.md`; I-014 covers each missing
// `spec/features/<feature-slug>/issues/README.md`.
func lintI013AndI014(specRoot string, discovered []issue.Discovered) []Violation {
	missing := missingIndexPaths(specRoot, discovered)
	if len(missing) == 0 {
		return nil
	}
	out := make([]Violation, 0, len(missing))
	for _, m := range missing {
		var msg string
		switch m.rule {
		case "I-013":
			msg = fmt.Sprintf("missing root issues index README: %s (run `specscore spec lint --fix` to scaffold)", m.relPath)
		case "I-014":
			msg = fmt.Sprintf("missing Feature-scoped issues index README: %s (run `specscore spec lint --fix` to scaffold)", m.relPath)
		}
		out = append(out, Violation{
			File:     m.relPath,
			Line:     0,
			Severity: "error",
			Rule:     m.rule,
			Message:  msg,
		})
	}
	return out
}

// lintI015 validates the Contents-table column headers of every existing
// issues-index README (root and per-Feature) against the five canonical
// columns in canonical order. The rule stays silent when the README is
// absent (that case is I-013/I-014's concern), when the README has no
// `## Contents` section, or when the Contents section has no table at
// all — those failure modes are not the column-shape concern.
func lintI015(specRoot string, discovered []issue.Discovered) []Violation {
	// Build the set of index README paths to inspect: every directory
	// that contains ≥1 on-pattern issue AND has a README.md present.
	hasRoot := false
	featureSlugs := make(map[string]bool)
	for _, d := range discovered {
		if !d.MatchesPattern {
			continue
		}
		if d.FeatureSlug == "" {
			hasRoot = true
		} else {
			featureSlugs[d.FeatureSlug] = true
		}
	}

	type target struct {
		abs string
		rel string
	}
	var targets []target
	if hasRoot {
		abs := filepath.Join(specRoot, "issues", "README.md")
		if info, err := os.Stat(abs); err == nil && !info.IsDir() {
			targets = append(targets, target{abs: abs, rel: "issues/README.md"})
		}
	}
	var slugs []string
	for s := range featureSlugs {
		slugs = append(slugs, s)
	}
	sort.Strings(slugs)
	for _, s := range slugs {
		abs := filepath.Join(specRoot, "features", s, "issues", "README.md")
		if info, err := os.Stat(abs); err == nil && !info.IsDir() {
			targets = append(targets, target{abs: abs, rel: "features/" + s + "/issues/README.md"})
		}
	}

	var out []Violation
	for _, t := range targets {
		data, err := os.ReadFile(t.abs)
		if err != nil {
			continue
		}
		headers, found := parseContentsTableHeaders(string(data))
		if !found {
			// No Contents section or no table — out of I-015's scope.
			continue
		}
		if columnsMatch(headers, issueIndexColumns) {
			continue
		}
		out = append(out, Violation{
			File:     t.rel,
			Line:     0,
			Severity: "error",
			Rule:     "I-015",
			Message: fmt.Sprintf(
				"issues-index Contents table columns must be %s in that order (got %s)",
				strings.Join(issueIndexColumns, ", "),
				strings.Join(headers, ", "),
			),
		})
	}
	return out
}

// parseContentsTableHeaders finds the first markdown pipe-table that
// follows a `## Contents` heading in body and returns its header cell
// values. It returns (nil, false) when no Contents section exists or
// when that section contains no pipe-table. The check is intentionally
// permissive about separator-row dash count.
func parseContentsTableHeaders(body string) ([]string, bool) {
	lines := strings.Split(body, "\n")
	inContents := false
	for i, ln := range lines {
		t := strings.TrimSpace(ln)
		if strings.HasPrefix(t, "## Contents") {
			inContents = true
			continue
		}
		if inContents && strings.HasPrefix(t, "## ") {
			// Left the Contents section without finding a table.
			return nil, false
		}
		if !inContents {
			continue
		}
		if !strings.HasPrefix(t, "|") {
			continue
		}
		// Found candidate header row. Next non-blank line should be a
		// `|---|---|...` separator. We accept any line starting with
		// `|` that contains at least one `-`.
		if i+1 >= len(lines) {
			return nil, false
		}
		sep := strings.TrimSpace(lines[i+1])
		if !strings.HasPrefix(sep, "|") || !strings.Contains(sep, "-") {
			return nil, false
		}
		return splitPipeRow(t), true
	}
	return nil, false
}

// splitPipeRow splits a markdown pipe-table row into its trimmed cell
// values, discarding the leading and trailing pipe.
func splitPipeRow(row string) []string {
	row = strings.TrimSpace(row)
	row = strings.TrimPrefix(row, "|")
	row = strings.TrimSuffix(row, "|")
	parts := strings.Split(row, "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}

// columnsMatch reports whether got equals want element-wise.
func columnsMatch(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range got {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

// lintI009 enforces dual-location placement per upstream REQ
// issue-dual-location. Any file declaring `type: issue` outside the two
// canonical patterns emits a violation. Feature-scoped issues
// additionally require the parent Feature directory to exist (i.e.
// `spec/features/<feature-slug>/README.md` is present); when the parent
// is missing, the issue is treated as off-pattern.
func lintI009(specRoot string, discovered []issue.Discovered) []Violation {
	var out []Violation
	for _, d := range discovered {
		// Pattern match plus, for Feature-scoped issues, the parent
		// Feature directory must actually be a Feature (README.md present).
		ok := d.MatchesPattern
		if ok && d.FeatureSlug != "" {
			parentReadme := filepath.Join(specRoot, "features", d.FeatureSlug, "README.md")
			if info, statErr := os.Stat(parentReadme); statErr != nil || info.IsDir() {
				ok = false
			}
		}
		if ok {
			continue
		}
		out = append(out, Violation{
			File:     d.RelPath,
			Line:     0,
			Severity: "error",
			Rule:     "I-009",
			Message:  "issue artifact must live under spec/issues/ or spec/features/<feature-slug>/issues/ (and the parent Feature directory must exist)",
		})
	}
	return out
}

// lintI001AndI002 enforces frontmatter schema rules I-001 (required +
// known fields, plus slug-matches-filename) and I-002 (status enum).
//
// I-001 emits up to three distinct violations per file, each under its
// own template so violation taxonomy stays unambiguous when more than
// one defect occurs on the same artifact:
//   - "missing required frontmatter field: <name>"  (one per missing key)
//   - "unknown frontmatter field: <name>"           (one per unknown key)
//   - "slug %q does not match filename %q"          (slug/basename mismatch)
//
// I-002 emits at most one violation per file, listing the four valid
// status values verbatim.
func lintI001AndI002(specRoot string, discovered []issue.Discovered) []Violation {
	var out []Violation
	for _, d := range discovered {
		iss, err := issueParseFn(d.Path)
		if err != nil || iss == nil {
			continue
		}
		out = append(out, checkIssueI001(d.RelPath, iss)...)
		out = append(out, checkIssueI002(d.RelPath, iss)...)
		out = append(out, checkIssueI003(d.RelPath, iss)...)
		out = append(out, checkIssueI004(d.RelPath, iss)...)
		out = append(out, checkIssueI005(d.RelPath, iss)...)
		out = append(out, checkIssueI006(d.RelPath, iss)...)
		out = append(out, checkIssueI007(d.RelPath, iss)...)
		out = append(out, checkIssueI008(d.RelPath, iss)...)
		out = append(out, checkIssueI010(d.RelPath, iss)...)
		out = append(out, checkIssueI012(specRoot, d.RelPath, iss)...)
	}
	return out
}

// checkIssueI012 enforces the affected_component cross-artifact
// reference. When `affected_component` is present and non-empty, the
// referenced Feature directory must contain a README.md at
// `spec/features/<value>/README.md`. A missing README (including when
// the directory itself does not exist) emits one violation.
//
// Absence or present-but-empty are handled by other rules (I-001
// missing/known-fields and I-003 non-empty-string shape) — I-012 stays
// silent in those cases.
func checkIssueI012(specRoot, relPath string, iss *issue.Issue) []Violation {
	slug, present := iss.Frontmatter["affected_component"]
	if !present || strings.TrimSpace(slug) == "" {
		return nil
	}
	readme := filepath.Join(specRoot, "features", slug, "README.md")
	info, err := os.Stat(readme)
	if err == nil && !info.IsDir() {
		return nil
	}
	return []Violation{{
		File:     relPath,
		Line:     0,
		Severity: "error",
		Rule:     "I-012",
		Message:  fmt.Sprintf("affected_component %q does not resolve to spec/features/%s/README.md", slug, slug),
	}}
}

// checkIssueI010 enforces the filename-vs-frontmatter slug equality
// invariant. I-001 already emits a (semantically equivalent) violation
// under its slug-vs-filename template; I-010 fires the same diagnostic
// under its own rule ID so --rules/--ignore can target the slug-match
// concern specifically. By Plan design both rules emit on a mismatch —
// they're intentionally redundant.
//
// I-010 stays silent when slug is absent or empty (those are I-001
// missing-field cases, not slug-mismatch cases).
func checkIssueI010(relPath string, iss *issue.Issue) []Violation {
	slugVal, present := iss.Frontmatter["slug"]
	if !present || strings.TrimSpace(slugVal) == "" {
		return nil
	}
	if slugVal == iss.Slug {
		return nil
	}
	return []Violation{{
		File:     relPath,
		Line:     0,
		Severity: "error",
		Rule:     "I-010",
		Message:  fmt.Sprintf("frontmatter slug %q does not match filename %q", slugVal, iss.Slug),
	}}
}

// lintI011 enforces global slug uniqueness across all `issue` artifacts.
// One corpus pass builds slug → []relPath; any slug appearing under more
// than one path emits a single violation that names every colliding path.
//
// The violation is attached to the first colliding path (alphabetically;
// `discovered` is pre-sorted by DiscoverAll) so diagnostic ordering stays
// deterministic. Off-pattern files are still considered for collision —
// they declare `type: issue` and so should not silently reuse a slug
// owned by an on-pattern artifact.
func lintI011(discovered []issue.Discovered) []Violation {
	bySlug := make(map[string][]string, len(discovered))
	for _, d := range discovered {
		bySlug[d.Slug] = append(bySlug[d.Slug], d.RelPath)
	}

	// Emit in deterministic order: iterate slugs sorted alphabetically.
	var slugs []string
	for s, paths := range bySlug {
		if len(paths) > 1 {
			slugs = append(slugs, s)
		}
	}
	sort.Strings(slugs)

	var out []Violation
	for _, s := range slugs {
		paths := bySlug[s]
		// paths are already in sorted order because discovered is sorted.
		out = append(out, Violation{
			File:     paths[0],
			Line:     0,
			Severity: "error",
			Rule:     "I-011",
			Message:  fmt.Sprintf("slug %q used by multiple issue artifacts: %s", s, strings.Join(paths, ", ")),
		})
	}
	return out
}

// issueRequiredSections names the three required H2 section titles in
// canonical order per rule I-008.
var issueRequiredSections = []string{
	"Description",
	"Steps to Reproduce",
	"Expected vs Actual",
}

// h1Pattern is the canonical H1 contract for an issue body per rule
// I-007: the line must start with `# Issue: ` followed by at least one
// non-whitespace character. The check is implemented inline against
// raw body lines (no markdown-tree parser dependency) because the
// shape we care about is a single anchored prefix.
const h1Pattern = "^# Issue: .+$"

// checkIssueI007 enforces the canonical H1 line. Missing H1 or an H1
// whose prefix is not `# Issue: ` (followed by at least one non-
// whitespace character) emits a single violation.
func checkIssueI007(relPath string, iss *issue.Issue) []Violation {
	h1 := firstH1(iss.Body)
	if h1 != "" && isCanonicalIssueH1(h1) {
		return nil
	}
	return []Violation{{
		File:     relPath,
		Line:     0,
		Severity: "error",
		Rule:     "I-007",
		Message:  fmt.Sprintf("H1 must match %q", h1Pattern),
	}}
}

// firstH1 returns the first markdown H1 line (verbatim, without trailing
// newline) in body, or "" if none is found. A line is treated as an H1
// when it starts with `# ` after no leading whitespace.
func firstH1(body string) string {
	for _, line := range strings.Split(body, "\n") {
		if strings.HasPrefix(line, "# ") {
			return strings.TrimRight(line, "\r")
		}
	}
	return ""
}

// isCanonicalIssueH1 reports whether line matches `^# Issue: .+$` —
// i.e. starts with the literal `# Issue: ` and contains at least one
// non-whitespace character after it.
func isCanonicalIssueH1(line string) bool {
	const prefix = "# Issue: "
	if !strings.HasPrefix(line, prefix) {
		return false
	}
	return strings.TrimSpace(line[len(prefix):]) != ""
}

// checkIssueI008 validates the three required H2 sections in canonical
// order. The rule emits distinct violation sub-types via distinct
// message templates so future advisory grouping can be added without
// changing rule IDs:
//
//   - "missing required section: `## <Name>`"
//   - "required section is empty: `## <Name>`"
//   - "required section appears more than once: `## <Name>`"
//   - "required sections must appear in canonical order: Description, Steps to Reproduce, Expected vs Actual"
//
// Additional H2s after the third canonical section are unconstrained
// and are not inspected.
func checkIssueI008(relPath string, iss *issue.Issue) []Violation {
	sections := collectH2Sections(iss.Body)

	// Bucket presence/duplication by canonical name.
	counts := make(map[string]int, len(issueRequiredSections))
	bodies := make(map[string]string, len(issueRequiredSections))
	for _, s := range sections {
		counts[s.title]++
		// Keep the first occurrence's body for the empty check; that
		// matches "appears exactly once" semantics — if a section is
		// duplicated we emit the duplicated violation separately.
		if _, seen := bodies[s.title]; !seen {
			bodies[s.title] = s.body
		}
	}

	var vs []Violation

	// (a) missing sections.
	for _, name := range issueRequiredSections {
		if counts[name] == 0 {
			vs = append(vs, Violation{
				File:     relPath,
				Line:     0,
				Severity: "error",
				Rule:     "I-008",
				Message:  fmt.Sprintf("missing required section: `## %s`", name),
			})
		}
	}

	// (b) duplicated sections.
	for _, name := range issueRequiredSections {
		if counts[name] > 1 {
			vs = append(vs, Violation{
				File:     relPath,
				Line:     0,
				Severity: "error",
				Rule:     "I-008",
				Message:  fmt.Sprintf("required section appears more than once: `## %s`", name),
			})
		}
	}

	// (c) empty sections — only when the section is present.
	for _, name := range issueRequiredSections {
		if counts[name] >= 1 && strings.TrimSpace(bodies[name]) == "" {
			vs = append(vs, Violation{
				File:     relPath,
				Line:     0,
				Severity: "error",
				Rule:     "I-008",
				Message:  fmt.Sprintf("required section is empty: `## %s`", name),
			})
		}
	}

	// (d) order check — only meaningful when all three required
	// sections are present. The first three H2 encounters whose
	// titles are in the required set must equal the canonical order.
	if counts[issueRequiredSections[0]] >= 1 &&
		counts[issueRequiredSections[1]] >= 1 &&
		counts[issueRequiredSections[2]] >= 1 {
		var observed []string
		seen := make(map[string]bool, len(issueRequiredSections))
		for _, s := range sections {
			isRequired := false
			for _, r := range issueRequiredSections {
				if s.title == r {
					isRequired = true
					break
				}
			}
			if !isRequired || seen[s.title] {
				continue
			}
			seen[s.title] = true
			observed = append(observed, s.title)
			if len(observed) == len(issueRequiredSections) {
				break
			}
		}
		ordered := len(observed) == len(issueRequiredSections)
		if ordered {
			for i, name := range issueRequiredSections {
				if observed[i] != name {
					ordered = false
					break
				}
			}
		}
		if !ordered {
			vs = append(vs, Violation{
				File:     relPath,
				Line:     0,
				Severity: "error",
				Rule:     "I-008",
				Message: fmt.Sprintf(
					"required sections must appear in canonical order: %s",
					strings.Join(issueRequiredSections, ", "),
				),
			})
		}
	}

	return vs
}

// h2Section captures an H2 heading and the raw body text that follows
// it (up to but not including the next H2 or end of body).
type h2Section struct {
	title string
	body  string
}

// collectH2Sections scans body for H2 headings (lines starting with
// `## `) and returns each heading title plus the text between it and
// the next H2 (or end of body). The returned slice preserves source
// order.
func collectH2Sections(body string) []h2Section {
	lines := strings.Split(body, "\n")
	var sections []h2Section
	var current *h2Section
	var buf []string
	flush := func() {
		if current != nil {
			current.body = strings.Join(buf, "\n")
			sections = append(sections, *current)
		}
	}
	for _, line := range lines {
		stripped := strings.TrimRight(line, "\r")
		if strings.HasPrefix(stripped, "## ") {
			flush()
			title := strings.TrimSpace(strings.TrimPrefix(stripped, "## "))
			current = &h2Section{title: title}
			buf = nil
			continue
		}
		if current != nil {
			buf = append(buf, stripped)
		}
	}
	flush()
	return sections
}

func checkIssueI001(relPath string, iss *issue.Issue) []Violation {
	var vs []Violation

	// Missing required fields. Treat empty-string values as missing
	// (a present-but-empty `captured_by:` provides no useful identity).
	for _, k := range issueRequiredKeys {
		v, present := iss.Frontmatter[k]
		if !present || strings.TrimSpace(v) == "" {
			vs = append(vs, Violation{
				File:     relPath,
				Line:     0,
				Severity: "error",
				Rule:     "I-001",
				Message:  fmt.Sprintf("missing required frontmatter field: %s", k),
			})
		}
	}

	// Unknown frontmatter keys (anything outside the closed
	// required+optional set).
	var unknown []string
	for _, k := range iss.FrontmatterKeyOrder {
		if !issueKnownKeySet[k] {
			unknown = append(unknown, k)
		}
	}
	sort.Strings(unknown)
	for _, k := range unknown {
		vs = append(vs, Violation{
			File:     relPath,
			Line:     0,
			Severity: "error",
			Rule:     "I-001",
			Message:  fmt.Sprintf("unknown frontmatter field: %s", k),
		})
	}

	// slug must equal the filename basename (minus `.md`). Only
	// emit when slug is present and non-empty — absence is already
	// covered by the missing-field branch above.
	if slugVal, present := iss.Frontmatter["slug"]; present && strings.TrimSpace(slugVal) != "" {
		if slugVal != iss.Slug {
			vs = append(vs, Violation{
				File:     relPath,
				Line:     0,
				Severity: "error",
				Rule:     "I-001",
				Message:  fmt.Sprintf("slug %q does not match filename %q", slugVal, iss.Slug),
			})
		}
	}

	return vs
}

func checkIssueI002(relPath string, iss *issue.Issue) []Violation {
	status, present := iss.Frontmatter["status"]
	if !present || strings.TrimSpace(status) == "" {
		// Absence is an I-001 missing-field violation; not our concern.
		return nil
	}
	if issueStatusValueSet[status] {
		return nil
	}
	return []Violation{{
		File:     relPath,
		Line:     0,
		Severity: "error",
		Rule:     "I-002",
		Message:  fmt.Sprintf("status %q is not one of {%s}", status, strings.Join(issueStatusValues, ", ")),
	}}
}

// checkIssueI003 validates the shape of optional frontmatter fields.
// Absence of any optional field is always valid; only present-but-
// malformed values emit a violation. Per the Plan, I-003 only checks
// type and non-emptiness for the rejection_* fields — the
// presence/absence wiring against `status: rejected` is I-006's job.
func checkIssueI003(relPath string, iss *issue.Issue) []Violation {
	var vs []Violation

	// `severity` enum check.
	if sev, present := iss.Frontmatter["severity"]; present {
		if !issueSeverityValueSet[sev] {
			vs = append(vs, Violation{
				File:     relPath,
				Line:     0,
				Severity: "error",
				Rule:     "I-003",
				Message:  fmt.Sprintf("severity %q is not one of {%s}", sev, strings.Join(issueSeverityValues, ", ")),
			})
		}
	}

	// Non-empty-string checks for the remaining optional string fields.
	for _, k := range issueNonEmptyStringOptionals {
		v, present := iss.Frontmatter[k]
		if !present {
			continue
		}
		if strings.TrimSpace(v) == "" {
			vs = append(vs, Violation{
				File:     relPath,
				Line:     0,
				Severity: "error",
				Rule:     "I-003",
				Message:  fmt.Sprintf("optional field %q must be a non-empty string when present", k),
			})
		}
	}

	return vs
}

// checkIssueI005 enforces severity-required-on-transition. Once `status`
// leaves `open` (i.e. is one of investigating/resolved/rejected),
// `severity` MUST be set to a concrete level — `low`, `medium`, `high`,
// or `critical`. Absent severity or `severity: unset` both violate.
//
// I-005 says nothing when `status` is `open`, missing, or an invalid
// enum value (I-001/I-002 cover those). I-005 also says nothing when
// `severity` is set to any non-`unset` string — even one that is not in
// the I-003 enum — because I-003 already handles enum-shape and the
// concern of I-005 is solely "did the author make a transition-time
// commitment".
func checkIssueI005(relPath string, iss *issue.Issue) []Violation {
	status, present := iss.Frontmatter["status"]
	if !present {
		return nil
	}
	if !issueTransitionStatuses[status] {
		return nil
	}
	sev, sevPresent := iss.Frontmatter["severity"]
	if !sevPresent || strings.TrimSpace(sev) == "" || sev == "unset" {
		return []Violation{{
			File:     relPath,
			Line:     0,
			Severity: "error",
			Rule:     "I-005",
			Message:  fmt.Sprintf("severity-required-on-transition: status %q requires severity to be one of {low, medium, high, critical} (not absent, not unset)", status),
		}}
	}
	return nil
}

// checkIssueI006 enforces the rejection_reason / rejection_notes
// contract:
//
//   - status=rejected requires rejection_reason present and non-empty.
//   - status!=rejected requires rejection_reason to be absent.
//   - rejection_reason, when present, must be one of the six enum values.
//   - rejection_notes must be absent when rejection_reason is absent
//     (orphan-notes check).
//
// Each sub-check emits its own violation so taxonomy stays unambiguous.
func checkIssueI006(relPath string, iss *issue.Issue) []Violation {
	var vs []Violation
	status := iss.Frontmatter["status"]
	reason, reasonPresent := iss.Frontmatter["rejection_reason"]
	_, notesPresent := iss.Frontmatter["rejection_notes"]
	reasonNonEmpty := reasonPresent && strings.TrimSpace(reason) != ""

	// (a) status: rejected requires rejection_reason.
	if status == "rejected" && !reasonNonEmpty {
		vs = append(vs, Violation{
			File:     relPath,
			Line:     0,
			Severity: "error",
			Rule:     "I-006",
			Message:  "status \"rejected\" requires rejection_reason to be set",
		})
	}

	// (b) status != rejected forbids rejection_reason.
	if status != "rejected" && reasonPresent {
		vs = append(vs, Violation{
			File:     relPath,
			Line:     0,
			Severity: "error",
			Rule:     "I-006",
			Message:  fmt.Sprintf("rejection_reason must be absent when status is not \"rejected\" (got status %q)", status),
		})
	}

	// (c) rejection_reason value enum check. Only run when present and
	// non-empty — I-003 covers the present-but-empty case under its
	// non-empty-string rule.
	if reasonNonEmpty && !issueRejectionReasonValueSet[reason] {
		vs = append(vs, Violation{
			File:     relPath,
			Line:     0,
			Severity: "error",
			Rule:     "I-006",
			Message:  fmt.Sprintf("rejection_reason %q is not one of {%s}", reason, strings.Join(issueRejectionReasonValues, ", ")),
		})
	}

	// Orphan rejection_notes: notes present but reason absent.
	if notesPresent && !reasonPresent {
		vs = append(vs, Violation{
			File:     relPath,
			Line:     0,
			Severity: "error",
			Rule:     "I-006",
			Message:  "rejection_notes must be absent when rejection_reason is absent",
		})
	}

	return vs
}

// checkIssueI004 validates the reserved `bugs` field. Absence is valid;
// an empty list is valid; a list whose every element is a string scalar
// is valid. Anything else (scalar value, mapping, or list containing a
// non-string element) emits one violation.
//
// Lint MUST NOT resolve the string elements to bug artifacts — the
// `bug` artifact type does not exist in this MVP and the field is
// opaque by design.
func checkIssueI004(relPath string, iss *issue.Issue) []Violation {
	node := iss.BugsRaw
	if node == nil {
		// Field absent — valid.
		return nil
	}
	if node.Kind != yaml.SequenceNode {
		return []Violation{{
			File:     relPath,
			Line:     0,
			Severity: "error",
			Rule:     "I-004",
			Message:  "bugs must be a YAML list whose every element is a string",
		}}
	}
	// Empty list is valid.
	for i, elem := range node.Content {
		if elem.Kind != yaml.ScalarNode || (elem.Tag != "" && elem.Tag != "!!str") {
			return []Violation{{
				File:     relPath,
				Line:     0,
				Severity: "error",
				Rule:     "I-004",
				Message:  fmt.Sprintf("bugs element at index %d (%q) is not a string; every element of bugs must be a string", i, elem.Value),
			}}
		}
	}
	return nil
}
