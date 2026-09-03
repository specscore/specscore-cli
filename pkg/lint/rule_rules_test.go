package lint

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/rule"
)

// ----- fixtures -----

// ruleRowFields are the six data cells of a canonical row, in table order.
type ruleRowFields struct {
	Status      string
	Scope       string
	Enforcement string
	Control     string
	Sources     string
	Statement   string
}

func defaultRowFields() ruleRowFields {
	return ruleRowFields{
		Status: "Draft", Scope: "fleet", Enforcement: "Stated", Control: "—", Sources: "—",
		Statement: "Never ship a mocked extension backend.",
	}
}

func withRow(mutate func(*ruleRowFields)) ruleRowFields {
	f := defaultRowFields()
	mutate(&f)
	return f
}

func (f ruleRowFields) render(slug string, linked bool) string {
	identity := slug
	if linked {
		identity = "[" + slug + "](" + slug + "/README.md)"
	}
	return "| " + identity + " | " + f.Status + " | " + f.Scope + " | " + f.Enforcement +
		" | " + f.Control + " | " + f.Sources + " | " + f.Statement + " |"
}

// ruleIndexWith renders a lint-clean index carrying the given rows verbatim.
func ruleIndexWith(rows ...string) string {
	body := "---\nformat: " + rule.IndexFormatURL + "\n---\n\n# Rules\n\n" +
		rule.IndexHeading + "\n\n" + rule.IndexHeaderRow + "\n" + rule.IndexSeparatorRow + "\n"
	if len(rows) == 0 {
		body += "\n" + rule.IndexEmptyPlaceholder + "\n"
	}
	for _, row := range rows {
		body += row + "\n"
	}
	return body + "\n## Open Questions\n\nNone at this time.\n\n---\n*This document follows the " + rule.IndexFormatURL + "*\n"
}

func defaultDetailFields() map[string]string {
	return map[string]string{
		"Status": "Draft", "Date": "2026-09-03", "Owner": "alex",
		"Statement": "Never ship a mocked extension backend.", "Scope": "fleet",
		"Enforcement": "Stated", "Control": "—", "Sources": "—",
		"Why": "A mock passes review and then fails in production.", "Exceptions": "none",
		"Supersedes": "—", "Superseded By": "—",
	}
}

// ruleDetail renders a lint-clean detail document with the given overrides. A
// value of "\x00omit" drops the field entirely.
func ruleDetail(overrides map[string]string) string {
	fields := defaultDetailFields()
	for k, v := range overrides {
		fields[k] = v
	}
	var b strings.Builder
	b.WriteString("---\nformat: " + rule.FormatURL + "\nstatus: " + fields["Status"] + "\n---\n\n")
	b.WriteString("# Rule: Fixture\n\n")
	for _, name := range rule.DetailFields {
		if fields[name] == "\x00omit" {
			continue
		}
		b.WriteString("**" + name + ":** " + fields[name] + "\n")
	}
	b.WriteString("\n## Instructions\n\nDo the thing.\n")
	b.WriteString("\n## Examples\n\n### Compliant\n\nx\n\n### Violation\n\ny\n")
	b.WriteString("\n## Open Questions\n\nNone at this time.\n\n")
	b.WriteString("---\n*This document follows the " + rule.FormatURL + "*\n")
	return b.String()
}

// rowMatching renders the row a detail document's own fields project to, so a
// shape test isolates one finding instead of also tripping the mirror rule.
func rowMatching(slug string, overrides map[string]string) string {
	merged := defaultDetailFields()
	for k, v := range overrides {
		merged[k] = v
	}
	fields := ruleRowFields{
		Status: merged["Status"], Scope: merged["Scope"], Enforcement: merged["Enforcement"],
		Control: merged["Control"], Sources: merged["Sources"], Statement: merged["Statement"],
	}
	if strings.TrimSpace(fields.Sources) == "" {
		fields.Sources = "—"
	}
	return fields.render(slug, true)
}

// ruleTree materializes a project with the given index body and detail
// documents, and returns the project root.
func ruleTree(t *testing.T, index string, details map[string]string) string {
	t.Helper()
	root := t.TempDir()
	rulesDir := rule.RulesDir(root)
	if err := os.MkdirAll(rulesDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if index != "" {
		if err := os.WriteFile(rule.IndexPath(rulesDir), []byte(index), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	for slug, body := range details {
		dir := filepath.Join(rulesDir, slug)
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func lintRules(t *testing.T, root string) []Violation {
	t.Helper()
	violations, err := newRuleRulesChecker(root).check(filepath.Join(root, "spec"))
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	return violations
}

func fixRules(t *testing.T, root string) {
	t.Helper()
	checker := newRuleRulesChecker(root)
	checker.autofix = true
	if err := checker.fix(filepath.Join(root, "spec")); err != nil {
		t.Fatalf("fix: %v", err)
	}
}

func hasRFamilyViolation(violations []Violation, id, substring string) bool {
	for _, v := range violations {
		if v.Rule == id && strings.Contains(v.Message, substring) {
			return true
		}
	}
	return false
}

func ruleViolationIDs(violations []Violation) []string {
	out := make([]string, 0, len(violations))
	for _, v := range violations {
		out = append(out, v.Rule+": "+v.Message)
	}
	return out
}

func writeRuleLesson(t *testing.T, root, slug, body string) {
	t.Helper()
	dir := filepath.Join(root, "spec", "lessons", slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writeRuleSkill(t *testing.T, root, name, body string) {
	t.Helper()
	dir := filepath.Join(root, rule.DefaultSkillsPath, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ----- silence and clean cases -----

// The family must never fire in a repository that has recorded no rule.
func TestRuleRulesSilentWithoutRulesDirectory(t *testing.T) {
	if got := lintRules(t, t.TempDir()); len(got) != 0 {
		t.Fatalf("violations without spec/rules: %v", ruleViolationIDs(got))
	}
}

func TestRuleRulesCleanInlineRule(t *testing.T) {
	root := ruleTree(t, ruleIndexWith(defaultRowFields().render("clean", false)), nil)
	if got := lintRules(t, root); len(got) != 0 {
		t.Fatalf("a clean inline rule produced violations: %v", ruleViolationIDs(got))
	}
}

func TestRuleRulesCleanDetailedRule(t *testing.T) {
	root := ruleTree(t,
		ruleIndexWith(defaultRowFields().render("clean", true)),
		map[string]string{"clean": ruleDetail(nil)})
	if got := lintRules(t, root); len(got) != 0 {
		t.Fatalf("a clean detailed rule produced violations: %v", ruleViolationIDs(got))
	}
}

// ----- R-002 / R-003 / R-005 / R-006 / R-007: row validity -----

func TestRuleRowViolations(t *testing.T) {
	cases := []struct {
		name    string
		fields  ruleRowFields
		wantID  string
		wantMsg string
	}{
		{name: "empty statement", fields: withRow(func(f *ruleRowFields) { f.Statement = "—" }),
			wantID: "R-003", wantMsg: "has no Statement"},
		{name: "bad status", fields: withRow(func(f *ruleRowFields) { f.Status = "Pending" }),
			wantID: "R-002", wantMsg: "invalid Status"},
		{name: "bad enforcement", fields: withRow(func(f *ruleRowFields) { f.Enforcement = "Mandatory" }),
			wantID: "R-005", wantMsg: "invalid Enforcement"},
		{name: "enforced without control", fields: withRow(func(f *ruleRowFields) { f.Enforcement = "Enforced" }),
			wantID: "R-005", wantMsg: "wearing a stronger label"},
		{name: "automated without control", fields: withRow(func(f *ruleRowFields) { f.Enforcement = "Automated" }),
			wantID: "R-005", wantMsg: "wearing a stronger label"},
		{name: "empty scope", fields: withRow(func(f *ruleRowFields) { f.Scope = "—" }),
			wantID: "R-006", wantMsg: "must name at least one Scope"},
		{name: "bad scope", fields: withRow(func(f *ruleRowFields) { f.Scope = "team:platform" }),
			wantID: "R-006", wantMsg: "unknown kind"},
		{name: "duplicate scope", fields: withRow(func(f *ruleRowFields) { f.Scope = "fleet, fleet" }),
			wantID: "R-006", wantMsg: "lists scope fleet more than once"},
		{name: "bad source", fields: withRow(func(f *ruleRowFields) { f.Sources = "memo:x" }),
			wantID: "R-007", wantMsg: "unknown kind"},
		{name: "duplicate source", fields: withRow(func(f *ruleRowFields) { f.Sources = "decision:0001, decision:0001" }),
			wantID: "R-007", wantMsg: "more than once"},
		{name: "unresolvable lesson", fields: withRow(func(f *ruleRowFields) { f.Sources = "lesson:ghost" }),
			wantID: "R-007", wantMsg: "does not resolve to a Lesson"},
		{name: "unresolvable idea", fields: withRow(func(f *ruleRowFields) { f.Sources = "idea:ghost" }),
			wantID: "R-007", wantMsg: "does not resolve to an Idea"},
		{name: "unresolvable decision", fields: withRow(func(f *ruleRowFields) { f.Sources = "decision:9999" }),
			wantID: "R-007", wantMsg: "does not resolve to a Decision"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := ruleTree(t, ruleIndexWith(tc.fields.render("x", false)), nil)
			got := lintRules(t, root)
			if !hasRFamilyViolation(got, tc.wantID, tc.wantMsg) {
				t.Fatalf("want %s containing %q; got %v", tc.wantID, tc.wantMsg, ruleViolationIDs(got))
			}
			// Every row violation must point at the row's source line.
			for _, v := range got {
				if v.Rule == tc.wantID && v.Line == 0 {
					t.Errorf("violation %s carries no source line", v.Rule)
				}
			}
		})
	}
}

func TestRuleSourceResolutionAcceptsExistingArtifacts(t *testing.T) {
	fields := withRow(func(f *ruleRowFields) {
		f.Sources = "lesson:kinder-fake, decision:0012, idea:rules-entity, https://example.com/x"
	})
	root := ruleTree(t, ruleIndexWith(fields.render("x", false)), nil)
	specSub := filepath.Join(root, "spec")

	writeRuleLesson(t, root, "kinder-fake",
		"# Lesson: Kinder Fake\n\n**Status:** Recorded\n**Superseded By:** —\n**Promotes To:** rule:x\n\n## Open Questions\n\nNone.\n")
	for _, dir := range []string{"decisions", "ideas"} {
		if err := os.MkdirAll(filepath.Join(specSub, dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(specSub, "decisions", "0012-payment-rail.md"), []byte("# Decision: X\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specSub, "ideas", "rules-entity.md"), []byte("# Idea: X\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	for _, v := range lintRules(t, root) {
		if v.Rule == "R-007" || v.Rule == "R-008" {
			t.Fatalf("resolvable sources reported: %v", ruleViolationIDs(lintRules(t, root)))
		}
	}
}

func TestIdeaSourceResolvesArchivedAndRejectsDirectory(t *testing.T) {
	root := ruleTree(t, ruleIndexWith(), nil)
	specSub := filepath.Join(root, "spec")
	archived := filepath.Join(specSub, "ideas", "archived")
	if err := os.MkdirAll(archived, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(archived, "retired.md"), []byte("# Idea: X\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !ideaSourceExists(specSub, "retired") {
		t.Fatal("an archived Idea must resolve")
	}
	if err := os.MkdirAll(filepath.Join(specSub, "ideas", "trap.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	if ideaSourceExists(specSub, "trap") {
		t.Fatal("a directory must not resolve as an Idea")
	}
}

// The decisions scan must skip a subdirectory and a non-markdown file rather
// than matching them by name prefix.
func TestDecisionSourceSkipsNonMarkdownEntries(t *testing.T) {
	root := ruleTree(t, ruleIndexWith(), nil)
	decisionsDir := filepath.Join(root, "spec", "decisions")
	if err := os.MkdirAll(filepath.Join(decisionsDir, "0012-a-directory"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(decisionsDir, "0012-notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if decisionSourceExists(filepath.Join(root, "spec"), "0012") {
		t.Fatal("only a .md file may resolve a decision reference")
	}
}

// ----- R-003: index shape -----

func TestRuleIndexShapeViolations(t *testing.T) {
	t.Run("no header", func(t *testing.T) {
		body := "# Rules\n\n" + rule.IndexHeading + "\n\n" + defaultRowFields().render("x", false) + "\n\n## Open Questions\n\nNone.\n"
		root := ruleTree(t, body, nil)
		if !hasRFamilyViolation(lintRules(t, root), "R-003", "canonical table header") {
			t.Fatalf("got %v", ruleViolationIDs(lintRules(t, root)))
		}
	})
	t.Run("malformed row", func(t *testing.T) {
		root := ruleTree(t, ruleIndexWith("| x | Draft |"), nil)
		if !hasRFamilyViolation(lintRules(t, root), "R-003", "canonical seven-column shape") {
			t.Fatalf("got %v", ruleViolationIDs(lintRules(t, root)))
		}
	})
	t.Run("duplicate row", func(t *testing.T) {
		row := defaultRowFields().render("x", false)
		root := ruleTree(t, ruleIndexWith(row, row), nil)
		if !hasRFamilyViolation(lintRules(t, root), "R-003", "more than once") {
			t.Fatalf("got %v", ruleViolationIDs(lintRules(t, root)))
		}
	})
	t.Run("unsorted rows", func(t *testing.T) {
		fields := defaultRowFields()
		root := ruleTree(t, ruleIndexWith(fields.render("zeta", false), fields.render("alpha", false)), nil)
		if !hasRFamilyViolation(lintRules(t, root), "R-003", "not sorted by slug") {
			t.Fatalf("got %v", ruleViolationIDs(lintRules(t, root)))
		}
	})
	t.Run("detail documents but no index", func(t *testing.T) {
		root := ruleTree(t, "", map[string]string{"x": ruleDetail(nil)})
		if !hasRFamilyViolation(lintRules(t, root), "R-003", "no README.md index") {
			t.Fatalf("got %v", ruleViolationIDs(lintRules(t, root)))
		}
	})
	t.Run("no index and no rules is silent", func(t *testing.T) {
		root := ruleTree(t, "", nil)
		if got := lintRules(t, root); len(got) != 0 {
			t.Fatalf("got %v", ruleViolationIDs(got))
		}
	})
}

// ----- R-004: row <-> document pairing -----

func TestRuleRowDetailPairing(t *testing.T) {
	t.Run("linked row with no document", func(t *testing.T) {
		root := ruleTree(t, ruleIndexWith(defaultRowFields().render("x", true)), nil)
		if !hasRFamilyViolation(lintRules(t, root), "R-004", "no such detail document exists") {
			t.Fatalf("got %v", ruleViolationIDs(lintRules(t, root)))
		}
	})
	t.Run("document with an unlinked row", func(t *testing.T) {
		root := ruleTree(t, ruleIndexWith(defaultRowFields().render("x", false)),
			map[string]string{"x": ruleDetail(nil)})
		if !hasRFamilyViolation(lintRules(t, root), "R-004", "does not link to it") {
			t.Fatalf("got %v", ruleViolationIDs(lintRules(t, root)))
		}
	})
	t.Run("document with no row", func(t *testing.T) {
		root := ruleTree(t, ruleIndexWith(), map[string]string{"x": ruleDetail(nil)})
		if !hasRFamilyViolation(lintRules(t, root), "R-004", "has no row in the rules index") {
			t.Fatalf("got %v", ruleViolationIDs(lintRules(t, root)))
		}
	})
}

// A README under spec/rules/ that discovery skips must still be reported, or a
// mistyped heading (or a non-slug directory) would silently remove an artifact
// from the index and from every other rule.
func TestRuleRulesReportsUndiscoverableDirectories(t *testing.T) {
	cases := []struct {
		name    string
		dir     string
		body    string
		wantMsg string
	}{
		{name: "no title at all", dir: "x", body: "**Status:** Draft\n\n## Open Questions\n\nNone.\n",
			wantMsg: "declares no non-empty `# Rule: <title>` heading"},
		{name: "non-slug directory", dir: "Not_A_Slug", body: "# Rule: X\n\n## Open Questions\n\nNone.\n",
			wantMsg: "not a canonical slug"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := ruleTree(t, ruleIndexWith(), map[string]string{tc.dir: tc.body})
			if !hasRFamilyViolation(lintRules(t, root), "R-001", tc.wantMsg) {
				t.Fatalf("want R-001 containing %q; got %v", tc.wantMsg, ruleViolationIDs(lintRules(t, root)))
			}
		})
	}
}

// A slug-named directory with no README.md is not an artifact at all and must
// be passed over in silence.
func TestRuleRulesSkipsDirectoryWithoutReadme(t *testing.T) {
	root := ruleTree(t, ruleIndexWith(), nil)
	if err := os.MkdirAll(filepath.Join(rule.RulesDir(root), "empty-dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := lintRules(t, root); len(got) != 0 {
		t.Fatalf("violations = %v", ruleViolationIDs(got))
	}
}

// Two undeclared directories exercise the deterministic ordering the report
// relies on.
func TestRuleRulesSortsUndeclaredDirectories(t *testing.T) {
	root := ruleTree(t, ruleIndexWith(), map[string]string{
		"zeta":  "# Notes\n",
		"alpha": "# Notes\n",
	})
	got := lintRules(t, root)
	if len(got) < 2 || !strings.Contains(got[0].File, "alpha") {
		t.Fatalf("violations are not sorted by file: %v", ruleViolationIDs(got))
	}
}

func TestLintUndeclaredRuleDirsHandlesUnreadableDirectory(t *testing.T) {
	prev := osReadDir
	osReadDir = func(string) ([]os.DirEntry, error) { return nil, errors.New("boom") }
	t.Cleanup(func() { osReadDir = prev })
	if got := lintUndeclaredRuleDirs(&ruleWorld{specRoot: "spec", rulesDir: "spec/rules"}); got != nil {
		t.Fatalf("lintUndeclaredRuleDirs = %v, want nil on an unreadable directory", got)
	}
}

// ----- R-001: detail-document shape -----

func TestRuleDetailViolations(t *testing.T) {
	cases := []struct {
		name      string
		overrides map[string]string
		body      string
		wantID    string
		wantMsg   string
	}{
		{name: "missing field", overrides: map[string]string{"Owner": "\x00omit"},
			wantID: "R-001", wantMsg: "missing required metadata field: **Owner:**"},
		{name: "empty required value", overrides: map[string]string{"Why": "—"},
			wantID: "R-001", wantMsg: "**Why:** must not be empty"},
		{name: "empty optional value", overrides: map[string]string{"Sources": ""},
			wantID: "R-001", wantMsg: "**Sources:** must use — when absent"},
		{name: "bad date", overrides: map[string]string{"Date": "3 Sep 2026"},
			wantID: "R-001", wantMsg: "**Date:** must be YYYY-MM-DD"},
		{name: "unresolvable supersedes", overrides: map[string]string{"Supersedes": "ghost"},
			wantID: "R-009", wantMsg: "**Supersedes:** does not resolve"},
		{name: "superseded without successor", overrides: map[string]string{"Status": "Superseded"},
			wantID: "R-009", wantMsg: "requires **Superseded By:**"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := ruleTree(t, ruleIndexWith(rowMatching("x", tc.overrides)),
				map[string]string{"x": ruleDetail(tc.overrides)})
			got := lintRules(t, root)
			if !hasRFamilyViolation(got, tc.wantID, tc.wantMsg) {
				t.Fatalf("want %s containing %q; got %v", tc.wantID, tc.wantMsg, ruleViolationIDs(got))
			}
		})
	}
}

func TestRuleDetailStructuralViolations(t *testing.T) {
	cases := []struct {
		name    string
		body    string
		wantMsg string
	}{
		{name: "duplicated field",
			body:    "# Rule: X\n\n**Status:** Draft\n**Status:** Active\n\n## Open Questions\n\nNone.\n",
			wantMsg: "metadata field is duplicated: **Status:**"},
		{name: "out of order",
			body:    "# Rule: X\n\n**Owner:** alex\n**Status:** Draft\n\n## Open Questions\n\nNone.\n",
			wantMsg: "metadata fields are out of order"},
		{name: "missing sections",
			body:    "# Rule: X\n\n**Status:** Draft\n",
			wantMsg: "missing required section(s)"},
		{name: "examples without both subsections",
			body:    "# Rule: X\n\n**Status:** Draft\n\n## Instructions\n\nx\n\n## Examples\n\n### Compliant\n\nx\n\n## Open Questions\n\nNone.\n",
			wantMsg: "must carry both `### Compliant` and `### Violation`"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := ruleTree(t, ruleIndexWith(defaultRowFields().render("x", true)),
				map[string]string{"x": tc.body})
			if !hasRFamilyViolation(lintRules(t, root), "R-001", tc.wantMsg) {
				t.Fatalf("want R-001 containing %q; got %v", tc.wantMsg, ruleViolationIDs(lintRules(t, root)))
			}
		})
	}
}

// Scope and Sources are list fields and may legitimately repeat across lines,
// so the duplicate-field rule must not fire on them.
func TestRuleDetailAllowsRepeatedListFields(t *testing.T) {
	body := strings.Replace(ruleDetail(nil), "**Scope:** fleet\n", "**Scope:** fleet\n**Scope:** product:sneat\n", 1)
	fields := withRow(func(f *ruleRowFields) { f.Scope = "fleet, product:sneat" })
	root := ruleTree(t, ruleIndexWith(fields.render("x", true)), map[string]string{"x": body})
	for _, v := range lintRules(t, root) {
		if strings.Contains(v.Message, "duplicated") {
			t.Fatalf("repeated list field reported as duplicated: %v", ruleViolationIDs(lintRules(t, root)))
		}
	}
}

// A detail document with no row has nothing to mirror against, so R-011 must
// stay quiet and leave the finding to R-004.
func TestRuleDetailWithoutRowSkipsTheMirrorCheck(t *testing.T) {
	root := ruleTree(t, ruleIndexWith(), map[string]string{"x": ruleDetail(nil)})
	for _, v := range lintRules(t, root) {
		if v.Rule == "R-011" {
			t.Fatalf("mirror check fired without a row: %v", ruleViolationIDs(lintRules(t, root)))
		}
	}
}

// ----- R-011: the mirror -----

// R-011 is the rule that keeps two representations of one rule from becoming
// two different rules.
func TestRuleDetailMirrorMustMatchItsRow(t *testing.T) {
	for _, field := range rule.MirroredFields {
		t.Run(field, func(t *testing.T) {
			overrides := map[string]string{}
			switch field {
			case "Status":
				overrides[field] = "Active"
			case "Enforcement":
				overrides["Enforcement"] = "Enforced"
				overrides["Control"] = "wb hook"
			case "Control":
				overrides[field] = "wb hook"
			case "Scope":
				overrides[field] = "product:sneat"
			case "Sources":
				overrides[field] = "decision:0001"
			default:
				overrides[field] = "Something else entirely."
			}
			root := ruleTree(t,
				ruleIndexWith(defaultRowFields().render("x", true)),
				map[string]string{"x": ruleDetail(overrides)})
			if !hasRFamilyViolation(lintRules(t, root), "R-011", "the index row says") {
				t.Fatalf("want an R-011 mirror violation for %s; got %v", field, ruleViolationIDs(lintRules(t, root)))
			}
		})
	}
}

// A drifted document is repaired FROM the row, never the reverse.
func TestRuleFixRewritesTheDocumentFromTheRow(t *testing.T) {
	root := ruleTree(t,
		ruleIndexWith(defaultRowFields().render("x", true)),
		map[string]string{"x": ruleDetail(map[string]string{"Status": "Active"})})

	fixRules(t, root)

	if got := lintRules(t, root); len(got) != 0 {
		t.Fatalf("fix left violations: %v", ruleViolationIDs(got))
	}
	detail, _ := os.ReadFile(rule.DetailPath(rule.RulesDir(root), "x"))
	if !strings.Contains(string(detail), "**Status:** Draft") {
		t.Fatalf("document not rewritten from the row:\n%s", detail)
	}
	if !strings.Contains(string(detail), "status: Draft") {
		t.Fatalf("frontmatter mirror not rewritten:\n%s", detail)
	}
	index, _ := os.ReadFile(rule.IndexPath(rule.RulesDir(root)))
	if !strings.Contains(string(index), "| Draft |") {
		t.Fatalf("the row must stay authoritative:\n%s", index)
	}
	// Fixing twice is a byte-for-byte no-op.
	before, _ := os.ReadFile(rule.DetailPath(rule.RulesDir(root), "x"))
	fixRules(t, root)
	after, _ := os.ReadFile(rule.DetailPath(rule.RulesDir(root), "x"))
	if string(before) != string(after) {
		t.Fatal("the mirror fix is not idempotent")
	}
}

func TestRuleFixRepairsIndexShape(t *testing.T) {
	t.Run("adds a row for an orphan document", func(t *testing.T) {
		root := ruleTree(t, ruleIndexWith(), map[string]string{"x": ruleDetail(nil)})
		fixRules(t, root)
		if got := lintRules(t, root); len(got) != 0 {
			t.Fatalf("fix left violations: %v", ruleViolationIDs(got))
		}
		index, _ := os.ReadFile(rule.IndexPath(rule.RulesDir(root)))
		if !strings.Contains(string(index), "[x](x/README.md)") {
			t.Fatalf("orphan document did not gain a linked row:\n%s", index)
		}
	})

	t.Run("corrects a stale link cell in both directions", func(t *testing.T) {
		root := ruleTree(t,
			ruleIndexWith(defaultRowFields().render("linked-no-doc", true), defaultRowFields().render("unlinked-doc", false)),
			map[string]string{"unlinked-doc": ruleDetail(nil)})
		fixRules(t, root)
		index, _ := os.ReadFile(rule.IndexPath(rule.RulesDir(root)))
		if strings.Contains(string(index), "[linked-no-doc]") {
			t.Fatalf("stale link retained:\n%s", index)
		}
		if !strings.Contains(string(index), "[unlinked-doc](unlinked-doc/README.md)") {
			t.Fatalf("missing link not added:\n%s", index)
		}
	})

	t.Run("sorts and deduplicates", func(t *testing.T) {
		fields := defaultRowFields()
		root := ruleTree(t, ruleIndexWith(
			fields.render("zeta", false), fields.render("alpha", false), fields.render("zeta", false)), nil)
		fixRules(t, root)
		if got := lintRules(t, root); len(got) != 0 {
			t.Fatalf("fix left violations: %v", ruleViolationIDs(got))
		}
		index, _ := os.ReadFile(rule.IndexPath(rule.RulesDir(root)))
		if strings.Count(string(index), "| zeta |") != 1 {
			t.Fatalf("duplicate not removed:\n%s", index)
		}
		if strings.Index(string(index), "| alpha |") > strings.Index(string(index), "| zeta |") {
			t.Fatalf("rows not sorted:\n%s", index)
		}
	})

	t.Run("does not run without autofix", func(t *testing.T) {
		root := ruleTree(t, ruleIndexWith(), map[string]string{"x": ruleDetail(nil)})
		before, _ := os.ReadFile(rule.IndexPath(rule.RulesDir(root)))
		if err := newRuleRulesChecker(root).fix(filepath.Join(root, "spec")); err != nil {
			t.Fatalf("fix: %v", err)
		}
		after, _ := os.ReadFile(rule.IndexPath(rule.RulesDir(root)))
		if string(before) != string(after) {
			t.Fatal("fix ran without autofix")
		}
	})

	t.Run("is a no-op with no rules tree or no index", func(t *testing.T) {
		fixRules(t, t.TempDir())
		fixRules(t, ruleTree(t, "", nil))
	})
}

// The fixer must never rewrite the fields a document alone owns: Why,
// Instructions and Examples are the author's, and a fixer that guessed at them
// would be inventing policy.
func TestRuleFixNeverEditsAuthoredContent(t *testing.T) {
	body := ruleDetail(map[string]string{"Status": "Active"})
	body = strings.Replace(body, "Do the thing.", "Do the very specific thing.", 1)
	root := ruleTree(t, ruleIndexWith(defaultRowFields().render("x", true)), map[string]string{"x": body})
	fixRules(t, root)
	got, _ := os.ReadFile(rule.DetailPath(rule.RulesDir(root), "x"))
	for _, want := range []string{"Do the very specific thing.", "A mock passes review", "### Compliant", "### Violation"} {
		if !strings.Contains(string(got), want) {
			t.Errorf("fix discarded authored content %q:\n%s", want, got)
		}
	}
}

// ----- R-009: supersession -----

func TestRuleSupersessionIntegrity(t *testing.T) {
	t.Run("inverse pointer required", func(t *testing.T) {
		newer := map[string]string{"Supersedes": "old"}
		root := ruleTree(t, ruleIndexWith(rowMatching("new", newer), rowMatching("old", nil)),
			map[string]string{"new": ruleDetail(newer), "old": ruleDetail(nil)})
		if !hasRFamilyViolation(lintRules(t, root), "R-009", "inverse **Superseded By:** pointer") {
			t.Fatalf("got %v", ruleViolationIDs(lintRules(t, root)))
		}
	})
	t.Run("consistent pair is clean", func(t *testing.T) {
		newer := map[string]string{"Supersedes": "old"}
		older := map[string]string{"Status": "Superseded", "Superseded By": "new"}
		root := ruleTree(t, ruleIndexWith(rowMatching("new", newer), rowMatching("old", older)),
			map[string]string{"new": ruleDetail(newer), "old": ruleDetail(older)})
		for _, v := range lintRules(t, root) {
			if v.Rule == "R-009" {
				t.Fatalf("consistent supersession reported: %v", ruleViolationIDs(lintRules(t, root)))
			}
		}
	})
	t.Run("cycle detected", func(t *testing.T) {
		a := map[string]string{"Supersedes": "b", "Status": "Superseded", "Superseded By": "b"}
		b := map[string]string{"Supersedes": "a", "Status": "Superseded", "Superseded By": "a"}
		root := ruleTree(t, ruleIndexWith(rowMatching("a", a), rowMatching("b", b)),
			map[string]string{"a": ruleDetail(a), "b": ruleDetail(b)})
		if !hasRFamilyViolation(lintRules(t, root), "R-009", "cycle detected") {
			t.Fatalf("got %v", ruleViolationIDs(lintRules(t, root)))
		}
	})
}

// ----- R-008: the strict lesson<->rule pair -----

func TestRuleLessonPairingIsStrictBothWays(t *testing.T) {
	cases := []struct {
		name        string
		ruleSources string
		lessonBody  string
		wantMsg     string
		wantClean   bool
	}{
		{
			name:        "reciprocal pair is clean",
			ruleSources: "lesson:l",
			lessonBody:  "# Lesson: L\n\n**Status:** Recorded\n**Superseded By:** —\n**Promotes To:** rule:x\n\n## Open Questions\n\nNone.\n",
			wantClean:   true,
		},
		{
			name:        "rule cites a lesson that does not point back",
			ruleSources: "lesson:l",
			lessonBody:  "# Lesson: L\n\n**Status:** Recorded\n**Superseded By:** —\n\n## Open Questions\n\nNone.\n",
			wantMsg:     "does not point back",
		},
		{
			name:        "lesson promotes elsewhere",
			ruleSources: "lesson:l",
			lessonBody:  "# Lesson: L\n\n**Status:** Recorded\n**Superseded By:** —\n**Promotes To:** rule:other\n\n## Open Questions\n\nNone.\n",
			wantMsg:     "promotes to rule:other",
		},
		{
			name:        "lesson promotes to a rule that is not indexed",
			ruleSources: "—",
			lessonBody:  "# Lesson: L\n\n**Status:** Recorded\n**Superseded By:** —\n**Promotes To:** rule:ghost\n\n## Open Questions\n\nNone.\n",
			wantMsg:     "is not listed in the rules index",
		},
		{
			name:        "lesson promotes to a rule that does not cite it",
			ruleSources: "—",
			lessonBody:  "# Lesson: L\n\n**Status:** Recorded\n**Superseded By:** —\n**Promotes To:** rule:x\n\n## Open Questions\n\nNone.\n",
			wantMsg:     "is not reciprocated",
		},
		{
			name:        "malformed promotion pointer",
			ruleSources: "lesson:l",
			lessonBody:  "# Lesson: L\n\n**Status:** Recorded\n**Superseded By:** —\n**Promotes To:** never-mock\n\n## Open Questions\n\nNone.\n",
			wantMsg:     "must be rule:<slug>",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fields := withRow(func(f *ruleRowFields) { f.Sources = tc.ruleSources })
			root := ruleTree(t, ruleIndexWith(fields.render("x", false)), nil)
			writeRuleLesson(t, root, "l", tc.lessonBody)
			got := lintRules(t, root)
			if tc.wantClean {
				for _, v := range got {
					if v.Rule == "R-008" {
						t.Fatalf("reciprocal pair reported: %v", ruleViolationIDs(got))
					}
				}
				return
			}
			if !hasRFamilyViolation(got, "R-008", tc.wantMsg) {
				t.Fatalf("want R-008 containing %q; got %v", tc.wantMsg, ruleViolationIDs(got))
			}
		})
	}
}

// A Lesson slug present in both the flat and canonical layouts makes Lesson
// discovery fail; the pairing rule must degrade to "no lessons" rather than
// aborting the whole lint run.
func TestRuleLessonPairingToleratesLessonDiscoveryFailure(t *testing.T) {
	root := ruleTree(t, ruleIndexWith(defaultRowFields().render("x", false)), nil)
	body := "# Lesson: L\n\n**Status:** Recorded\n\n## Open Questions\n\nNone.\n"
	writeRuleLesson(t, root, "l", body)
	if err := os.WriteFile(filepath.Join(root, "spec", "lessons", "l.md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, v := range lintRules(t, root) {
		if v.Rule == "R-008" {
			t.Fatalf("pairing must stay silent when Lesson discovery fails: %v", ruleViolationIDs(lintRules(t, root)))
		}
	}
}

// ----- R-010: the rule<->skill pair -----

const pairedSkill = `---
name: go-hygiene
description: Keep Go changes formatted.
---

# Go hygiene

## Rules

- rule:x
`

func TestRuleSkillPairingIsStrictBothWays(t *testing.T) {
	detailWithSkill := strings.Replace(ruleDetail(nil), "Do the thing.", "Do the thing. Applies to skill:go-hygiene.", 1)

	t.Run("reciprocal pair is clean", func(t *testing.T) {
		root := ruleTree(t, ruleIndexWith(defaultRowFields().render("x", true)),
			map[string]string{"x": detailWithSkill})
		writeRuleSkill(t, root, "go-hygiene", pairedSkill)
		for _, v := range lintRules(t, root) {
			if v.Rule == "R-010" {
				t.Fatalf("reciprocal pair reported: %v", ruleViolationIDs(lintRules(t, root)))
			}
		}
	})

	t.Run("rule names a skill that does not exist", func(t *testing.T) {
		root := ruleTree(t, ruleIndexWith(defaultRowFields().render("x", true)),
			map[string]string{"x": detailWithSkill})
		if !hasRFamilyViolation(lintRules(t, root), "R-010", "does not resolve to a skill") {
			t.Fatalf("got %v", ruleViolationIDs(lintRules(t, root)))
		}
	})

	t.Run("skill does not point back", func(t *testing.T) {
		root := ruleTree(t, ruleIndexWith(defaultRowFields().render("x", true)),
			map[string]string{"x": detailWithSkill})
		writeRuleSkill(t, root, "go-hygiene", "---\nname: go-hygiene\n---\n\n# Go hygiene\n")
		if !hasRFamilyViolation(lintRules(t, root), "R-010", "does not point back") {
			t.Fatalf("got %v", ruleViolationIDs(lintRules(t, root)))
		}
	})

	t.Run("skill names a rule that is not indexed", func(t *testing.T) {
		root := ruleTree(t, ruleIndexWith(), nil)
		writeRuleSkill(t, root, "go-hygiene", pairedSkill)
		if !hasRFamilyViolation(lintRules(t, root), "R-010", "is not listed in the rules index") {
			t.Fatalf("got %v", ruleViolationIDs(lintRules(t, root)))
		}
	})

	// A skill can only bind a rule that has somewhere to name it back.
	t.Run("skill names an inline rule", func(t *testing.T) {
		root := ruleTree(t, ruleIndexWith(defaultRowFields().render("x", false)), nil)
		writeRuleSkill(t, root, "go-hygiene", pairedSkill)
		if !hasRFamilyViolation(lintRules(t, root), "R-010", "inline rule with no detail document") {
			t.Fatalf("got %v", ruleViolationIDs(lintRules(t, root)))
		}
	})

	t.Run("rule does not name the skill back", func(t *testing.T) {
		root := ruleTree(t, ruleIndexWith(defaultRowFields().render("x", true)),
			map[string]string{"x": ruleDetail(nil)})
		writeRuleSkill(t, root, "go-hygiene", pairedSkill)
		if !hasRFamilyViolation(lintRules(t, root), "R-010", "does not point back: reference `skill:go-hygiene`") {
			t.Fatalf("got %v", ruleViolationIDs(lintRules(t, root)))
		}
	})
}

// The skills directory is configurable, so a repository that keeps skills
// elsewhere still gets the pair checked.
func TestRuleSkillPairingHonoursConfiguredPath(t *testing.T) {
	detailWithSkill := strings.Replace(ruleDetail(nil), "Do the thing.", "Do the thing. Applies to skill:go-hygiene.", 1)
	root := ruleTree(t, ruleIndexWith(defaultRowFields().render("x", true)),
		map[string]string{"x": detailWithSkill})
	dir := filepath.Join(root, "tools", "skills", "go-hygiene")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "SKILL.md"), []byte(pairedSkill), 0o644); err != nil {
		t.Fatal(err)
	}
	// Without the override the skill is invisible and the pair is reported.
	if !hasRFamilyViolation(lintRules(t, root), "R-010", "does not resolve to a skill") {
		t.Fatalf("got %v", ruleViolationIDs(lintRules(t, root)))
	}
	config := "# SpecScore Repo Config Schema: https://specscore.md/repo-config\nversion: 1\nrules:\n  skills_path: tools/skills\n"
	if err := os.WriteFile(filepath.Join(root, "specscore.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, v := range lintRules(t, root) {
		if v.Rule == "R-010" {
			t.Fatalf("configured skills path not honoured: %v", ruleViolationIDs(lintRules(t, root)))
		}
	}
}

func TestConfiguredSkillsPathFallsBack(t *testing.T) {
	root := t.TempDir()
	if got := configuredSkillsPath(root); got != "" {
		t.Fatalf("no config should yield the default, got %q", got)
	}
	if err := os.WriteFile(filepath.Join(root, "specscore.yaml"),
		[]byte("# SpecScore Repo Config Schema: https://specscore.md/repo-config\nversion: 1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := configuredSkillsPath(root); got != "" {
		t.Fatalf("a config without a rules block should yield the default, got %q", got)
	}
}

func TestRelOrPath(t *testing.T) {
	if got := relOrPath("/proj", "/proj/ai/skills/x/SKILL.md"); got != filepath.Join("ai", "skills", "x", "SKILL.md") {
		t.Fatalf("relOrPath = %q", got)
	}
	if got := relOrPath("/proj", "/elsewhere/SKILL.md"); got != "/elsewhere/SKILL.md" {
		t.Fatalf("relOrPath outside the project = %q", got)
	}
}

// ----- registration and integration -----

func TestRuleCheckerMetadata(t *testing.T) {
	c := newRuleRulesChecker()
	if c.name() != "R-001" || c.severity() != "error" {
		t.Fatalf("checker metadata: %s / %s", c.name(), c.severity())
	}
	if len(c.fixTargets()) != len(ruleRuleIDs) {
		t.Fatalf("fixTargets = %v", c.fixTargets())
	}
	// The exported copy must be a copy: a caller mutating it cannot corrupt the
	// registration set.
	ids := RuleFamilyIDs()
	ids[0] = "mutated"
	if ruleRuleIDs[0] != "R-001" {
		t.Fatal("RuleFamilyIDs leaked the package slice")
	}
}

func TestRuleCheckPropagatesDiscoveryError(t *testing.T) {
	root := ruleTree(t, ruleIndexWith(), nil)
	// A README.md that is a directory makes ParseDetail fail, which must abort
	// the whole check rather than yield a partial rule set.
	if err := os.MkdirAll(filepath.Join(rule.RulesDir(root), "x", "README.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := newRuleRulesChecker(root).check(filepath.Join(root, "spec")); err == nil {
		t.Fatal("check should propagate a discovery failure")
	}
	checker := newRuleRulesChecker(root)
	checker.autofix = true
	if err := checker.fix(filepath.Join(root, "spec")); err == nil {
		t.Fatal("fix should propagate a discovery failure")
	}
}

func TestRuleCheckPropagatesIndexReadError(t *testing.T) {
	root := t.TempDir()
	// An index that exists but is a directory: Stat succeeds, the read fails.
	if err := os.MkdirAll(rule.IndexPath(rule.RulesDir(root)), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := newRuleRulesChecker(root).check(filepath.Join(root, "spec")); err == nil {
		t.Fatal("check should propagate an index read failure")
	}
}

// Every R-family ID must be registered in the rule registry, or the catalog and
// the checker would disagree about what exists.
func TestRuleFamilyIsRegistered(t *testing.T) {
	registry := AllRuleNames()
	for _, id := range ruleRuleIDs {
		if !registry[id] {
			t.Errorf("rule %s is emittable but not registered", id)
		}
	}
	if err := CheckRegistryParity(); err != nil {
		t.Fatalf("registry parity: %v", err)
	}
}

// The Rule and rules-index document kinds must be registered as doc types, or
// the shared frontmatter/footer rules would skip them entirely.
func TestRuleDocTypesAreRegistered(t *testing.T) {
	var detailTarget, indexTarget *docTypeTarget
	for i := range docTypeTargets {
		switch docTypeTargets[i].url {
		case rule.FormatURL:
			detailTarget = &docTypeTargets[i]
		case rule.IndexFormatURL:
			indexTarget = &docTypeTargets[i]
		}
	}
	if detailTarget == nil || !detailTarget.statusBearing {
		t.Fatal("the Rule detail README must be registered as a status-bearing doc type")
	}
	if indexTarget == nil || indexTarget.statusBearing {
		t.Fatal("the rules-index README must be registered as a status-less doc type")
	}

	root := ruleTree(t, ruleIndexWith(defaultRowFields().render("x", true)), map[string]string{"x": ruleDetail(nil)})
	specSub := filepath.Join(root, "spec")
	var walked []string
	if err := detailTarget.walk(specSub, func(path string, _ []byte) { walked = append(walked, path) }); err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(walked) != 1 || !strings.HasSuffix(walked[0], filepath.Join("rules", "x", "README.md")) {
		t.Fatalf("the detail walker visited %v", walked)
	}
	walked = nil
	if err := indexTarget.walk(specSub, func(path string, _ []byte) { walked = append(walked, path) }); err != nil {
		t.Fatalf("walk: %v", err)
	}
	if len(walked) != 1 || !strings.HasSuffix(walked[0], filepath.Join("rules", "README.md")) {
		t.Fatalf("the index walker visited %v", walked)
	}
}

// Both forms of a scaffolded rule must satisfy the shared frontmatter, footer
// and section rules as well as their own family — the property that makes
// `rule new` usable in a repository whose CI gates on `spec lint`.
func TestScaffoldedRulesAreCleanUnderFullLint(t *testing.T) {
	root := t.TempDir()
	rulesDir := rule.RulesDir(root)
	if err := rule.EnsureIndex(rulesDir); err != nil {
		t.Fatal(err)
	}
	detailed := rule.Options{
		Slug: "gofmt-first", Owner: "alex", Date: "2026-09-03",
		Statement: "Always run gofmt on changed Go files.", Why: "Unformatted Go reddens CI.",
	}
	if err := detailed.Normalize(); err != nil {
		t.Fatal(err)
	}
	body, err := rule.ScaffoldDetail(detailed)
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(rulesDir, detailed.Slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	inline := rule.Options{Slug: "never-mock-backends", Date: "2026-09-03", Statement: "Never mock a backend."}
	if err := inline.Normalize(); err != nil {
		t.Fatal(err)
	}
	for _, row := range []rule.Row{detailed.Row(true), inline.Row(false)} {
		if err := rule.UpsertRow(rulesDir, row); err != nil {
			t.Fatal(err)
		}
	}

	violations, err := Lint(Options{SpecRoot: filepath.Join(root, "spec"), ProjectRoot: root, Severity: "error"})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	for _, v := range violations {
		if strings.HasPrefix(v.File, "rules") {
			t.Errorf("a scaffolded rule is not lint-clean: %s:%d [%s] %s", v.File, v.Line, v.Rule, v.Message)
		}
	}
}

// ----- fixer fault injection -----

var errRuleInjected = errors.New("injected rule failure")

func swapRuleSeams(t *testing.T, apply func()) {
	t.Helper()
	writeIndex, readIndex := ruleWriteIndexRowsFn, ruleReadIndexFn
	readFile, applyEdits, writeFile := ruleReadFileFn, ruleApplyFieldEditsFn, ruleWriteFileAtomicFn
	t.Cleanup(func() {
		ruleWriteIndexRowsFn, ruleReadIndexFn = writeIndex, readIndex
		ruleReadFileFn, ruleApplyFieldEditsFn, ruleWriteFileAtomicFn = readFile, applyEdits, writeFile
	})
	apply()
}

func TestRuleFixPropagatesFailures(t *testing.T) {
	newTree := func(t *testing.T) string {
		return ruleTree(t, ruleIndexWith(defaultRowFields().render("x", true)),
			map[string]string{"x": ruleDetail(map[string]string{"Status": "Active"})})
	}
	cases := []struct {
		name  string
		apply func()
	}{
		{name: "index write fails", apply: func() {
			ruleWriteIndexRowsFn = func(string, []rule.Row) error { return errRuleInjected }
		}},
		{name: "index re-read fails", apply: func() {
			ruleReadIndexFn = func(string) (rule.IndexReport, error) { return rule.IndexReport{}, errRuleInjected }
		}},
		{name: "document read fails", apply: func() {
			ruleReadFileFn = func(string) ([]byte, error) { return nil, errRuleInjected }
		}},
		{name: "document edit fails", apply: func() {
			ruleApplyFieldEditsFn = func([]byte, []rule.FieldEdit) ([]byte, error) { return nil, errRuleInjected }
		}},
		{name: "document write fails", apply: func() {
			ruleWriteFileAtomicFn = func(string, []byte) error { return errRuleInjected }
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := newTree(t)
			swapRuleSeams(t, tc.apply)
			checker := newRuleRulesChecker(root)
			checker.autofix = true
			if err := checker.fix(filepath.Join(root, "spec")); !errors.Is(err, errRuleInjected) {
				t.Fatalf("fix error = %v, want the injected failure", err)
			}
		})
	}
}

// A document whose row vanished between the write and the re-read is skipped,
// not mirrored against nothing.
func TestRuleFixSkipsDocumentWithNoRowAfterRewrite(t *testing.T) {
	root := ruleTree(t, ruleIndexWith(defaultRowFields().render("x", true)),
		map[string]string{"x": ruleDetail(map[string]string{"Status": "Active"})})
	swapRuleSeams(t, func() {
		ruleReadIndexFn = func(string) (rule.IndexReport, error) { return rule.IndexReport{}, nil }
	})
	checker := newRuleRulesChecker(root)
	checker.autofix = true
	if err := checker.fix(filepath.Join(root, "spec")); err != nil {
		t.Fatalf("fix: %v", err)
	}
	// The document is untouched because there was no row to mirror from.
	got, _ := os.ReadFile(rule.DetailPath(rule.RulesDir(root), "x"))
	if !strings.Contains(string(got), "**Status:** Active") {
		t.Fatalf("document was rewritten without a row:\n%s", got)
	}
}

// A skills path that exists but is not a directory is a real error, not an
// absent-skills no-op: silently ignoring it would disable the pair check.
func TestRuleCheckPropagatesSkillDiscoveryError(t *testing.T) {
	root := ruleTree(t, ruleIndexWith(), nil)
	if err := os.MkdirAll(filepath.Join(root, "ai"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, rule.DefaultSkillsPath), []byte("not a directory"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := newRuleRulesChecker(root).check(filepath.Join(root, "spec")); err == nil {
		t.Fatal("check should propagate a skill discovery failure")
	}
}

// An empty `# Rule:` heading is discovered but titleless, which is its own
// finding rather than the invisible-document one.
func TestRuleDetailEmptyTitleIsReported(t *testing.T) {
	body := strings.Replace(ruleDetail(nil), "# Rule: Fixture", "# Rule:", 1)
	root := ruleTree(t, ruleIndexWith(defaultRowFields().render("x", true)), map[string]string{"x": body})
	if !hasRFamilyViolation(lintRules(t, root), "R-001", "requires a non-empty `# Rule: <title>` heading") {
		t.Fatalf("got %v", ruleViolationIDs(lintRules(t, root)))
	}
}

func TestDetailFieldPositionRejectsUnknownField(t *testing.T) {
	if got := detailFieldPosition("Nonsense"); got != -1 {
		t.Fatalf("detailFieldPosition = %d, want -1", got)
	}
}
