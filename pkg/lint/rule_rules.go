// specscore:feature/cli/spec/lint/rule-rules
//
// Implements the R-family lint rules for the Rule doc kind, which has two forms
// sharing one identity: an INLINE rule is a single row in spec/rules/README.md,
// and a DETAILED rule is that same row plus spec/rules/<slug>/README.md.
//
// The index row is the source of truth for every field it carries. R-011 is the
// rule that keeps the two forms from becoming two different rules: a detail
// document's mirrored header MUST equal its row, and --fix rewrites the
// document from the row, never the reverse.
//
// It mirrors the L-family (pkg/lint/lesson_rules.go) deliberately: one checker
// registered under every rule name, an index the fix pass can repair, and
// violations carrying the source line of the offending row or field.
package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/specscore/specscore-cli/pkg/idea"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/specscore/specscore-cli/pkg/projectdef"
	"github.com/specscore/specscore-cli/pkg/rule"
)

// ruleRuleIDs is the ordered rule-name set the ruleRulesChecker answers to;
// linter.go registers the single checker instance under each.
var ruleRuleIDs = []string{
	"R-001", "R-002", "R-003", "R-004", "R-005",
	"R-006", "R-007", "R-008", "R-009", "R-010", "R-011",
}

// RuleFamilyIDs returns a copy of the R-family rule IDs, for callers (such as
// `specscore rule lint`) that want to run only this family.
func RuleFamilyIDs() []string { return append([]string(nil), ruleRuleIDs...) }

// ruleRulesChecker implements R-001..R-011. One checker emits violations for
// every rule name; the linter framework dedupes by pointer identity so a single
// walk produces the full finding set.
type ruleRulesChecker struct {
	projectRoot string
	// autofix, when true, lets the fix pass repair the index table shape, add a
	// row for an orphan detail document, correct a row's link cell, and rewrite
	// a drifted detail header from its row.
	autofix bool
}

func newRuleRulesChecker(projectRoot ...string) *ruleRulesChecker {
	var root string
	if len(projectRoot) > 0 {
		root = projectRoot[0]
	}
	return &ruleRulesChecker{projectRoot: root}
}

func (c *ruleRulesChecker) name() string     { return "R-001" }
func (c *ruleRulesChecker) severity() string { return "error" }

// fixTargets declares the --fix action names this checker answers to, so
// `spec lint --fix=R-003` reaches it the way `--fix=L-003` reaches the Lesson
// index fixer.
func (c *ruleRulesChecker) fixTargets() []string { return ruleRuleIDs }

func (c *ruleRulesChecker) effectiveProjectRoot(specRoot string) string {
	return lintProjectRoot(c.projectRoot, specRoot)
}

// ruleWorld is one consistent snapshot of everything the family reasons over.
type ruleWorld struct {
	specRoot    string
	projectRoot string
	rulesDir    string
	indexPath   string
	indexRel    string
	report      rule.IndexReport
	rows        map[string]rule.Row
	details     map[string]*rule.Detail
	skills      map[string]*rule.Skill
	skillsDir   string
}

// loadRuleWorld reads the index, the detail documents, and the skills in one
// pass. ok is false when there is no rules tree at all, which must produce no
// violations: the family has to stay silent in a repository that has recorded
// no rule.
func (c *ruleRulesChecker) loadRuleWorld(specRoot string) (*ruleWorld, bool, error) {
	rulesDir := filepath.Join(specRoot, "rules")
	if info, err := os.Stat(rulesDir); err != nil || !info.IsDir() {
		return nil, false, nil
	}
	projectRoot := c.effectiveProjectRoot(specRoot)
	w := &ruleWorld{
		specRoot:    specRoot,
		projectRoot: projectRoot,
		rulesDir:    rulesDir,
		indexPath:   rule.IndexPath(rulesDir),
		skillsDir:   rule.SkillsDir(projectRoot, configuredSkillsPath(projectRoot)),
	}
	w.indexRel = mustRel(specRoot, w.indexPath)

	details, err := rule.DetailsBySlug(rulesDir)
	if err != nil {
		return nil, false, err
	}
	w.details = details

	if _, err := os.Stat(w.indexPath); err == nil {
		report, readErr := rule.ReadIndex(w.indexPath)
		if readErr != nil {
			return nil, false, readErr
		}
		w.report = report
	}
	w.rows = w.report.BySlug()

	skills, err := rule.SkillsByName(w.skillsDir)
	if err != nil {
		return nil, false, err
	}
	w.skills = skills
	return w, true, nil
}

// configuredSkillsPath reads the optional `rules.skills_path` override. A
// missing or malformed config falls back to the default location rather than
// failing the lint run: a repository with no specscore.yaml still has skills.
func configuredSkillsPath(projectRoot string) string {
	cfg, err := projectdef.ReadSpecConfig(projectRoot)
	if err != nil {
		return ""
	}
	raw, ok := cfg.Extras["rules"].(map[string]any)
	if !ok {
		return ""
	}
	value, _ := raw["skills_path"].(string)
	return value
}

func (c *ruleRulesChecker) check(specRoot string) ([]Violation, error) {
	w, ok, err := c.loadRuleWorld(specRoot)
	if err != nil || !ok {
		return nil, err
	}

	var violations []Violation
	violations = append(violations, lintRuleIndexShape(w)...)
	violations = append(violations, lintRuleRowFields(w)...)
	violations = append(violations, lintRuleRowDetailPairing(w)...)
	violations = append(violations, lintUndeclaredRuleDirs(w)...)
	for _, slug := range sortedDetailSlugs(w.details) {
		violations = append(violations, lintRuleDetail(w, w.details[slug])...)
	}
	violations = append(violations, lintRuleSupersession(w)...)
	violations = append(violations, lintRuleLessonPairing(w)...)
	violations = append(violations, lintRuleSkillPairing(w)...)

	sort.SliceStable(violations, func(i, j int) bool {
		if violations[i].File != violations[j].File {
			return violations[i].File < violations[j].File
		}
		if violations[i].Line != violations[j].Line {
			return violations[i].Line < violations[j].Line
		}
		return violations[i].Rule < violations[j].Rule
	})
	return violations, nil
}

// fix repairs exactly what is derivable: the index table shape, a missing row
// for an existing detail document, a row's link cell, and a detail document
// whose mirrored header drifted from its row. It never invents a row's data
// cells from a document (the row is authoritative) and never edits a
// document's own fields — Why, Instructions and Examples are the author's.
func (c *ruleRulesChecker) fix(specRoot string) error {
	if !c.autofix {
		return nil
	}
	w, ok, err := c.loadRuleWorld(specRoot)
	if err != nil || !ok {
		return err
	}
	if _, statErr := os.Stat(w.indexPath); statErr != nil {
		return nil
	}

	rows := make([]rule.Row, 0, len(w.report.Rows))
	kept := map[string]rule.Row{}
	var order []string
	for _, row := range w.report.Rows {
		previous, exists := kept[row.Slug]
		if exists {
			// A duplicate is only safe to drop when it says exactly the same
			// thing. Two rows that disagree are ambiguous — picking one would
			// discard a rule's content on a coin flip — so both are kept and
			// R-003 keeps reporting until a human resolves it.
			if previous.Equals(row) {
				continue
			}
			rows = append(rows, row)
			continue
		}
		kept[row.Slug] = row
		order = append(order, row.Slug)
	}
	for _, slug := range order {
		row := kept[slug]
		// The link cell is derivable: it states whether a document exists.
		row.Linked = w.details[row.Slug] != nil
		rows = append(rows, row)
	}
	// A detail document with no row gets one projected from the document — the
	// only direction in which a document may seed a row, and only because the
	// alternative is an artifact invisible to every reader of the index.
	for _, slug := range sortedDetailSlugs(w.details) {
		if _, exists := kept[slug]; !exists {
			rows = append(rows, rule.RowFromDetail(w.details[slug]))
		}
	}

	// A row that did not parse is repaired only when the repair is
	// unambiguous; otherwise it is carried through verbatim and left to
	// R-003. Dropping it is never an option.
	var preserved []rule.MalformedRow
	for _, m := range w.report.Malformed {
		repaired, ok := m.Repair()
		if !ok {
			preserved = append(preserved, m)
			continue
		}
		if _, exists := kept[repaired.Slug]; exists {
			preserved = append(preserved, m)
			continue
		}
		repaired.Linked = w.details[repaired.Slug] != nil
		kept[repaired.Slug] = repaired
		rows = append(rows, repaired)
	}

	if err := ruleWriteIndexRowsFn(w.indexPath, rows, preserved); err != nil {
		return err
	}

	// Re-read so the mirror repair works against the published rows.
	report, err := ruleReadIndexFn(w.indexPath)
	if err != nil {
		return err
	}
	byslug := report.BySlug()
	for _, slug := range sortedDetailSlugs(w.details) {
		row, exists := byslug[slug]
		if !exists {
			continue
		}
		if err := mirrorRowIntoDetail(w.details[slug], row); err != nil {
			return err
		}
	}
	return nil
}

// mirrorRowIntoDetail rewrites a detail document's mirrored header fields from
// its row when they disagree.
func mirrorRowIntoDetail(d *rule.Detail, row rule.Row) error {
	want, got := rule.MirroredValuesOf(row), d.MirroredValues()
	var edits []rule.FieldEdit
	for _, name := range rule.MirroredFields {
		if want[name] != got[name] {
			edits = append(edits, rule.FieldEdit{Name: name, Value: want[name]})
		}
	}
	if len(edits) == 0 {
		return nil
	}
	content, err := ruleReadFileFn(d.Path)
	if err != nil {
		return err
	}
	updated, err := ruleApplyFieldEditsFn(content, edits)
	if err != nil {
		return err
	}
	return ruleWriteFileAtomicFn(d.Path, updated)
}

func sortedDetailSlugs(details map[string]*rule.Detail) []string {
	out := make([]string, 0, len(details))
	for slug := range details {
		out = append(out, slug)
	}
	sort.Strings(out)
	return out
}

// ----- R-003: index table shape -----

func lintRuleIndexShape(w *ruleWorld) []Violation {
	if _, err := os.Stat(w.indexPath); err != nil {
		// A rules directory holding only detail documents and no index is
		// itself the finding: the index is where a rule is published.
		if len(w.details) > 0 {
			return []Violation{{File: mustRel(w.specRoot, w.rulesDir), Severity: "error", Rule: "R-003",
				Message: "spec/rules/ has detail documents but no README.md index; the index is where a rule is published (run `specscore spec lint --fix`)"}}
		}
		return nil
	}
	var out []Violation
	if !w.report.HeaderSeen {
		out = append(out, Violation{File: w.indexRel, Severity: "error", Rule: "R-003",
			Message: fmt.Sprintf("rules index lacks the canonical table header %q (run `specscore spec lint --fix`)", rule.IndexHeaderRow)})
	}
	for _, m := range w.report.Malformed {
		// The message names the row, so a reader can see WHICH rule needs a
		// hand without opening the file — and says plainly that nothing was
		// discarded, because the obvious fear on seeing this is that something
		// was.
		subject := "row"
		if m.SlugHint != "" {
			subject = "row for " + m.SlugHint
		}
		repairable := ""
		if _, ok := m.Repair(); ok {
			repairable = " (`specscore spec lint --fix` can repair this one by escaping the surplus `|`)"
		}
		out = append(out, Violation{File: w.indexRel, Line: m.Line, Severity: "error", Rule: "R-003",
			Message: fmt.Sprintf("%s does not parse: %s; it is preserved verbatim and no verb will drop it%s",
				subject, m.Reason, repairable)})
	}
	for _, slug := range w.report.Duplicates {
		out = append(out, Violation{File: w.indexRel, Severity: "error", Rule: "R-003",
			Message: "rules index lists " + slug + " more than once; --fix removes a duplicate only when it is identical to the row it repeats, so differing duplicates are kept and must be resolved by hand"})
	}
	if !sortedBySlug(w.report.Rows) {
		out = append(out, Violation{File: w.indexRel, Severity: "error", Rule: "R-003",
			Message: "rules index rows are not sorted by slug (run `specscore spec lint --fix`)"})
	}
	return out
}

func sortedBySlug(rows []rule.Row) bool {
	for i := 1; i < len(rows); i++ {
		if rows[i-1].Slug > rows[i].Slug {
			return false
		}
	}
	return true
}

// ----- R-002 / R-005 / R-006 / R-007: row field validity -----

func lintRuleRowFields(w *ruleWorld) []Violation {
	var out []Violation
	add := func(id string, line int, msg string) {
		out = append(out, Violation{File: w.indexRel, Line: line, Severity: "error", Rule: id, Message: msg})
	}
	for _, row := range w.report.Rows {
		statement := strings.TrimSpace(row.Statement)
		if !isRuleValuePresent(statement) {
			add("R-003", row.Line, "rule "+row.Slug+" has no Statement; a rule with no normative sentence is not a rule")
		}
		status := strings.TrimSpace(row.Status)
		if !rule.IsStatus(status) {
			add("R-002", row.Line, fmt.Sprintf("rule %s has invalid Status %q (accepted: %s)", row.Slug, status, rule.StatusList()))
		}
		tier := strings.TrimSpace(row.Enforcement)
		switch {
		case !rule.IsEnforcement(tier):
			add("R-005", row.Line, fmt.Sprintf("rule %s has invalid Enforcement %q (accepted: %s)", row.Slug, tier, rule.EnforcementList()))
		case rule.RequiresControl(tier) && !row.HasControl():
			add("R-005", row.Line, fmt.Sprintf(
				"rule %s is %s but names no Control — an enforced rule with no control is a stated rule wearing a stronger label", row.Slug, tier))
		}
		out = append(out, lintScopeValues(w.indexRel, row.Line, "rule "+row.Slug, row.ScopeList())...)
		out = append(out, lintSourceValues(w, w.indexRel, row.Line, "rule "+row.Slug, row.SourceList())...)
	}
	return out
}

func lintScopeValues(file string, line int, subject string, scopes []string) []Violation {
	var out []Violation
	if len(scopes) == 0 {
		return []Violation{{File: file, Line: line, Severity: "error", Rule: "R-006",
			Message: subject + " must name at least one Scope (fleet, product:<name>, repo:<owner/repo>, or path:<glob>)"}}
	}
	seen := map[string]bool{}
	for _, raw := range scopes {
		if seen[raw] {
			out = append(out, Violation{File: file, Line: line, Severity: "error", Rule: "R-006",
				Message: subject + " lists scope " + raw + " more than once"})
			continue
		}
		seen[raw] = true
		if _, err := rule.ParseScope(raw); err != nil {
			out = append(out, Violation{File: file, Line: line, Severity: "error", Rule: "R-006",
				Message: subject + ": " + err.Error()})
		}
	}
	return out
}

func lintSourceValues(w *ruleWorld, file string, line int, subject string, sources []string) []Violation {
	var out []Violation
	seen := map[string]bool{}
	for _, raw := range sources {
		if seen[raw] {
			out = append(out, Violation{File: file, Line: line, Severity: "error", Rule: "R-007",
				Message: subject + " lists source " + raw + " more than once"})
			continue
		}
		seen[raw] = true
		ref, err := rule.ParseSource(raw)
		if err != nil {
			out = append(out, Violation{File: file, Line: line, Severity: "error", Rule: "R-007",
				Message: subject + ": " + err.Error()})
			continue
		}
		if msg := ruleSourceResolutionError(w.specRoot, ref); msg != "" {
			out = append(out, Violation{File: file, Line: line, Severity: "error", Rule: "R-007",
				Message: subject + ": " + msg})
		}
	}
	return out
}

// ruleSourceResolutionError returns a message when a typed source reference
// names an artifact that does not exist. Free URLs are checked syntactically
// only — resolving them would make lint depend on the network.
func ruleSourceResolutionError(specRoot string, ref rule.SourceRef) string {
	switch ref.Kind {
	case rule.SourceLesson:
		if _, err := lesson.ResolveLessonFile(filepath.Join(specRoot, "lessons"), ref.Value); err != nil {
			return fmt.Sprintf("source %s does not resolve to a Lesson under spec/lessons/", ref.String())
		}
	case rule.SourceIdea:
		if !ideaSourceExists(specRoot, ref.Value) {
			return fmt.Sprintf("source %s does not resolve to an Idea under spec/ideas/", ref.String())
		}
	case rule.SourceDecision:
		if !decisionSourceExists(specRoot, ref.Value) {
			return fmt.Sprintf("source %s does not resolve to a Decision under spec/decisions/", ref.String())
		}
	}
	return ""
}

// ideaSourceExists resolves an `idea:<slug>` reference against the active and
// archived Idea directories directly, rather than walking the whole Idea graph:
// a rule's source names a top-level Idea, and a per-source tree walk would make
// lint cost grow with the product of rules and Ideas.
func ideaSourceExists(specRoot, slug string) bool {
	ideasDir := idea.ResolveIdeasDir(specRoot)
	for _, candidate := range []string{
		filepath.Join(ideasDir, slug+".md"),
		filepath.Join(ideasDir, "archived", slug+".md"),
	} {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return true
		}
	}
	return false
}

func decisionSourceExists(specRoot, ref string) bool {
	decisionsDir := filepath.Join(specRoot, "decisions")
	number, _, _ := strings.Cut(ref, "-")
	for _, dir := range []string{decisionsDir, filepath.Join(decisionsDir, "archived")} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
				continue
			}
			stem := strings.TrimSuffix(e.Name(), ".md")
			if stem == ref || strings.HasPrefix(stem, number+"-") {
				return true
			}
		}
	}
	return false
}

func isRuleValuePresent(v string) bool {
	t := strings.TrimSpace(v)
	return t != "" && t != rule.Sentinel && t != "-"
}

// ----- R-004: row <-> detail-document pairing -----

func lintRuleRowDetailPairing(w *ruleWorld) []Violation {
	var out []Violation
	for _, row := range w.report.Rows {
		detail := w.details[row.Slug]
		switch {
		case row.Linked && detail == nil:
			out = append(out, Violation{File: w.indexRel, Line: row.Line, Severity: "error", Rule: "R-004",
				Message: fmt.Sprintf("rule %s links to %s but no such detail document exists; either write it (`specscore rule expand %s`) or drop the link", row.Slug, row.DetailLink(), row.Slug)})
		case !row.Linked && detail != nil:
			out = append(out, Violation{File: w.indexRel, Line: row.Line, Severity: "error", Rule: "R-004",
				Message: fmt.Sprintf("rule %s has a detail document at %s but its index row does not link to it (run `specscore spec lint --fix`)", row.Slug, row.DetailLink())})
		}
	}
	for _, slug := range sortedDetailSlugs(w.details) {
		if _, listed := w.rows[slug]; !listed {
			out = append(out, Violation{File: mustRel(w.specRoot, w.details[slug].Path), Severity: "error", Rule: "R-004",
				Message: "detail document has no row in the rules index, so no reader of the index can find it (run `specscore spec lint --fix`)"})
		}
	}
	return out
}

// lintUndeclaredRuleDirs reports a spec/rules/<slug>/README.md that discovery
// skipped: either its directory name is not a canonical slug, or its body
// carries no `# Rule: <title>` heading. Silently skipping it would leave the
// file on disk and invisible to every check.
func lintUndeclaredRuleDirs(w *ruleWorld) []Violation {
	entries, err := osReadDir(w.rulesDir)
	if err != nil {
		return nil
	}
	var out []Violation
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		readme := filepath.Join(w.rulesDir, e.Name(), "README.md")
		if _, statErr := os.Stat(readme); statErr != nil {
			continue
		}
		if slugErr := rule.ValidateSlug(e.Name()); slugErr != nil {
			out = append(out, Violation{File: mustRel(w.specRoot, readme), Severity: "error", Rule: "R-001",
				Message: "rule directory name is not a canonical slug: " + slugErr.Error()})
			continue
		}
		if w.details[e.Name()] == nil {
			out = append(out, Violation{File: mustRel(w.specRoot, readme), Severity: "error", Rule: "R-001",
				Message: "rule detail document declares no non-empty `# Rule: <title>` heading, so it is invisible to the rules index and to every other check"})
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].File < out[j].File })
	return out
}

// ----- R-001 / R-011: detail-document shape and mirror -----

var ruleDateRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}$`)

// optionalDetailFields carry the em-dash sentinel when they hold nothing.
var optionalDetailFields = []string{"Control", "Sources", "Supersedes", "Superseded By"}

// requiredNonEmptyDetailFields must carry real content. Statement and Why are
// the two that make a rule transferable at all.
var requiredNonEmptyDetailFields = []string{"Status", "Date", "Owner", "Statement", "Enforcement", "Why", "Exceptions"}

func lintRuleDetail(w *ruleWorld, d *rule.Detail) []Violation {
	rel := mustRel(w.specRoot, d.Path)
	var out []Violation
	add := func(id string, line int, msg string) {
		out = append(out, Violation{File: rel, Line: line, Severity: "error", Rule: id, Message: msg})
	}

	if strings.TrimSpace(d.Title) == "" {
		add("R-001", d.TitleLine, "rule detail document requires a non-empty `# Rule: <title>` heading")
	}

	fieldLine := detailFieldLines(d)
	for _, name := range rule.DetailFields {
		if fieldLine[name] == 0 {
			add("R-001", d.TitleLine, "missing required metadata field: **"+name+":**")
		}
		if name != "Scope" && name != "Sources" && d.FieldCounts[name] > 1 {
			add("R-001", fieldLine[name], "metadata field is duplicated: **"+name+":**")
		}
	}
	last := 0
	for _, name := range d.FieldOrder {
		position := detailFieldPosition(name)
		if position < last {
			add("R-001", fieldLine[name], fmt.Sprintf(
				"metadata fields are out of order at **%s:** (canonical order: %s)",
				name, strings.Join(rule.DetailFields, ", ")))
			break
		}
		last = position
	}

	values := detailFieldValues(d)
	for _, name := range requiredNonEmptyDetailFields {
		if fieldLine[name] != 0 && !isRuleValuePresent(values[name]) {
			add("R-001", fieldLine[name], "**"+name+":** must not be empty or the — sentinel")
		}
	}
	for _, name := range optionalDetailFields {
		if fieldLine[name] != 0 && strings.TrimSpace(values[name]) == "" {
			add("R-001", fieldLine[name], "**"+name+":** must use — when absent")
		}
	}
	if fieldLine["Date"] != 0 && isRuleValuePresent(d.Date) && !ruleDateRe.MatchString(strings.TrimSpace(d.Date)) {
		add("R-001", d.DateLine, "**Date:** must be YYYY-MM-DD")
	}
	if missing := d.MissingSections(); len(missing) > 0 {
		add("R-001", d.TitleLine, "missing required section(s): "+strings.Join(missing, ", "))
	}
	if d.HasSection("Examples") {
		if missing := d.MissingExampleSubsections(); len(missing) > 0 {
			add("R-001", d.SectionLines["Examples"], "## Examples must carry both `### Compliant` and `### Violation`; missing: "+
				strings.Join(missing, ", ")+" — an example set that shows only the happy path leaves the reader guessing at the boundary the rule draws")
		}
	}

	// R-011: the mirrored header must equal its row.
	row, listed := w.rows[d.Slug]
	if !listed {
		return out
	}
	want, got := rule.MirroredValuesOf(row), d.MirroredValues()
	for _, name := range rule.MirroredFields {
		if want[name] == got[name] {
			continue
		}
		out = append(out, Violation{File: rel, Line: fieldLine[name], Severity: "error", Rule: "R-011",
			Message: fmt.Sprintf(
				"**%s:** is %q but the index row says %q; the row is the source of truth (run `specscore spec lint --fix` to rewrite the document from the row)",
				name, got[name], want[name])})
	}
	return out
}

func detailFieldPosition(name string) int {
	for i, f := range rule.DetailFields {
		if f == name {
			return i
		}
	}
	return -1
}

func detailFieldLines(d *rule.Detail) map[string]int {
	return map[string]int{
		"Status": d.StatusLine, "Date": d.DateLine, "Owner": d.OwnerLine,
		"Statement": d.StatementLine, "Scope": d.ScopeLine, "Enforcement": d.EnforcementLine,
		"Control": d.ControlLine, "Sources": d.SourcesLine, "Why": d.WhyLine,
		"Exceptions": d.ExceptionsLine, "Supersedes": d.SupersedesLine, "Superseded By": d.SupersededByLine,
	}
}

func detailFieldValues(d *rule.Detail) map[string]string {
	return map[string]string{
		"Status": d.Status, "Date": d.Date, "Owner": d.Owner,
		"Statement": d.Statement, "Scope": d.ScopeText, "Enforcement": d.Enforcement,
		"Control": d.Control, "Sources": d.SourcesText, "Why": d.Why,
		"Exceptions": d.Exceptions, "Supersedes": d.Supersedes, "Superseded By": d.SupersededBy,
	}
}

// ----- R-009: supersession integrity (detail documents only) -----

func lintRuleSupersession(w *ruleWorld) []Violation {
	var out []Violation

	// An INLINE row at Superseded has nowhere to name its successor, so the
	// retirement leaves no forwarding address and nothing else in the family
	// would ever notice. Report it against the row.
	for _, row := range w.report.Rows {
		if strings.TrimSpace(row.Status) != "Superseded" || w.details[row.Slug] != nil {
			continue
		}
		out = append(out, Violation{File: w.indexRel, Line: row.Line, Severity: "error", Rule: "R-009",
			Message: fmt.Sprintf(
				"rule %s is Superseded but inline, so it has nowhere to record **Superseded By:**; run `specscore rule expand %s` and name the successor",
				row.Slug, row.Slug)})
	}

	slugs := sortedDetailSlugs(w.details)
	for _, slug := range slugs {
		d := w.details[slug]
		rel := mustRel(w.specRoot, d.Path)
		// The two pointers are checked asymmetrically, on purpose.
		//
		// **Superseded By:** points FORWARD to the rule that replaced this one.
		// It must resolve, or the retirement has no destination and a reader
		// following it lands nowhere.
		//
		// **Supersedes:** points BACKWARD at what this rule replaced, and that
		// rule may legitimately have been deleted — `rule delete
		// --supersede-with` writes exactly this breadcrumb so the references it
		// just warned about have a trail back. Requiring it to resolve would
		// make the only record of a retired rule illegal to keep. A malformed
		// slug is still reported; an absent one is history.
		if target := strings.TrimSpace(d.SupersededBy); isRuleValuePresent(target) {
			if _, listed := w.rows[target]; rule.ValidateSlug(target) != nil || !listed {
				out = append(out, Violation{File: rel, Line: d.SupersededByLine, Severity: "error", Rule: "R-009",
					Message: "**Superseded By:** does not resolve to a rule listed in the rules index"})
			}
		}
		if target := strings.TrimSpace(d.Supersedes); isRuleValuePresent(target) {
			if rule.ValidateSlug(target) != nil {
				out = append(out, Violation{File: rel, Line: d.SupersedesLine, Severity: "error", Rule: "R-009",
					Message: "**Supersedes:** is not a canonical slug"})
			}
		}
		if strings.TrimSpace(d.Status) == "Superseded" && !isRuleValuePresent(d.SupersededBy) {
			out = append(out, Violation{File: rel, Line: d.StatusLine, Severity: "error", Rule: "R-009",
				Message: "**Status:** Superseded requires **Superseded By:** naming the rule that replaces this one"})
		}
		if prior := w.details[strings.TrimSpace(d.Supersedes)]; isRuleValuePresent(d.Supersedes) && prior != nil {
			if strings.TrimSpace(prior.SupersededBy) != slug {
				out = append(out, Violation{File: rel, Line: d.SupersedesLine, Severity: "error", Rule: "R-009",
					Message: "**Supersedes:** target must carry the inverse **Superseded By:** pointer"})
			}
		}
	}
	for _, start := range slugs {
		seen := map[string]bool{}
		for current := start; current != ""; {
			if seen[current] {
				out = append(out, Violation{File: mustRel(w.specRoot, w.details[start].Path), Line: w.details[start].SupersedesLine,
					Severity: "error", Rule: "R-009", Message: "rule supersession cycle detected"})
				break
			}
			seen[current] = true
			d := w.details[current]
			if d == nil || !isRuleValuePresent(d.Supersedes) {
				break
			}
			current = strings.TrimSpace(d.Supersedes)
		}
	}
	return out
}

// ----- R-008: the strict lesson<->rule promotion pair -----

// lintRuleLessonPairing enforces the pair in both directions:
//
//   - A Lesson carrying `**Promotes To:** rule:<slug>` MUST name a rule listed
//     in the index whose Sources contain `lesson:<that lesson>`.
//   - A rule listing `lesson:<slug>` in Sources MUST name an existing Lesson
//     whose **Promotes To:** points back at that rule.
//
// The second half makes the relation one-to-many in exactly one direction: a
// rule may cite several Lessons, and each of those Lessons promotes into that
// one rule. A Lesson cannot be a source of two rules, because it has one
// promotion pointer — which is the constraint that keeps "which rule did this
// Lesson become?" answerable rather than ambiguous.
func lintRuleLessonPairing(w *ruleWorld) []Violation {
	lessonsDir := filepath.Join(w.specRoot, "lessons")
	lessons, err := lesson.Discover(lessonsDir)
	if err != nil {
		lessons = nil
	}
	byLesson := make(map[string]*lesson.Lesson, len(lessons))
	for _, l := range lessons {
		byLesson[l.Slug] = l
	}

	var out []Violation
	for _, row := range w.report.Rows {
		for _, raw := range row.SourceList() {
			ref, err := rule.ParseSource(raw)
			if err != nil || ref.Kind != rule.SourceLesson {
				continue
			}
			l := byLesson[ref.Value]
			if l == nil {
				continue // R-007 already reports the unresolvable source.
			}
			target, ok, parseErr := rule.ParsePromotesTo(l.PromotesTo)
			switch {
			case parseErr != nil:
				out = append(out, Violation{File: mustRel(w.specRoot, l.Path), Line: l.PromotesToLine, Severity: "error", Rule: "R-008",
					Message: parseErr.Error()})
			case !ok:
				out = append(out, Violation{File: w.indexRel, Line: row.Line, Severity: "error", Rule: "R-008",
					Message: fmt.Sprintf("source lesson:%s does not point back: add `**Promotes To:** rule:%s` to spec/lessons/%s/README.md (or run `specscore rule promote --from-lesson %s %s`)",
						ref.Value, row.Slug, ref.Value, ref.Value, row.Slug)})
			case target != row.Slug:
				out = append(out, Violation{File: w.indexRel, Line: row.Line, Severity: "error", Rule: "R-008",
					Message: fmt.Sprintf("source lesson:%s promotes to rule:%s, not rule:%s — a Lesson has one promotion pointer and cannot be a source of two rules",
						ref.Value, target, row.Slug)})
			}
		}
	}

	for _, l := range lessons {
		target, ok, parseErr := rule.ParsePromotesTo(l.PromotesTo)
		if parseErr != nil || !ok {
			continue // the malformed case is already reported above when cited.
		}
		rel := mustRel(w.specRoot, l.Path)
		row, listed := w.rows[target]
		if !listed {
			out = append(out, Violation{File: rel, Line: l.PromotesToLine, Severity: "error", Rule: "R-008",
				Message: fmt.Sprintf("**Promotes To:** rule:%s is not listed in the rules index", target)})
			continue
		}
		cited := false
		for _, raw := range row.SourceList() {
			if ref, err := rule.ParseSource(raw); err == nil && ref.Kind == rule.SourceLesson && ref.Value == l.Slug {
				cited = true
				break
			}
		}
		if !cited {
			out = append(out, Violation{File: rel, Line: l.PromotesToLine, Severity: "error", Rule: "R-008",
				Message: fmt.Sprintf("**Promotes To:** rule:%s is not reciprocated: add `lesson:%s` to that rule's Sources (or run `specscore rule update %s --add-source lesson:%s`)",
					target, l.Slug, target, l.Slug)})
		}
	}
	return out
}

// ----- R-010: the rule<->skill pair -----

// lintRuleSkillPairing enforces the pair in both directions. A rule's
// instructions naming `skill:<name>` must reach a skill that lists the rule
// back, and a skill's `## Rules` entry must reach a rule that names it. A skill
// that silently outlives the rule constraining it is the failure this catches.
func lintRuleSkillPairing(w *ruleWorld) []Violation {
	var out []Violation
	for _, slug := range sortedDetailSlugs(w.details) {
		d := w.details[slug]
		rel := mustRel(w.specRoot, d.Path)
		for _, name := range d.SkillRefs {
			skill := w.skills[name]
			if skill == nil {
				out = append(out, Violation{File: rel, Line: d.SectionLines["Instructions"], Severity: "error", Rule: "R-010",
					Message: fmt.Sprintf("skill:%s does not resolve to a skill under %s", name, filepath.ToSlash(relOrPath(w.projectRoot, w.skillsDir)))})
				continue
			}
			if !containsSlug(skill.RuleRefs, slug) {
				out = append(out, Violation{File: rel, Line: d.SectionLines["Instructions"], Severity: "error", Rule: "R-010",
					Message: fmt.Sprintf("skill:%s does not point back: add `- rule:%s` under `## Rules` in %s",
						name, slug, filepath.ToSlash(relOrPath(w.projectRoot, skill.Path)))})
			}
		}
	}

	names := make([]string, 0, len(w.skills))
	for name := range w.skills {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		skill := w.skills[name]
		for _, slug := range skill.RuleRefs {
			file := filepath.ToSlash(relOrPath(w.projectRoot, skill.Path))
			detail := w.details[slug]
			if _, listed := w.rows[slug]; !listed {
				out = append(out, Violation{File: file, Line: skill.RulesHeadingLine, Severity: "error", Rule: "R-010",
					Message: fmt.Sprintf("rule:%s is not listed in the rules index", slug)})
				continue
			}
			if detail == nil {
				out = append(out, Violation{File: file, Line: skill.RulesHeadingLine, Severity: "error", Rule: "R-010",
					Message: fmt.Sprintf("rule:%s is an inline rule with no detail document, so it cannot name this skill; expand it first (`specscore rule expand %s`)", slug, slug)})
				continue
			}
			if !containsSlug(detail.SkillRefs, name) {
				out = append(out, Violation{File: file, Line: skill.RulesHeadingLine, Severity: "error", Rule: "R-010",
					Message: fmt.Sprintf("rule:%s does not point back: reference `skill:%s` in that rule's Instructions", slug, name)})
			}
		}
	}
	return out
}

func containsSlug(values []string, want string) bool {
	for _, v := range values {
		if v == want {
			return true
		}
	}
	return false
}

// relOrPath renders a path relative to the project root when it is inside it,
// and verbatim otherwise (a configured absolute skills directory).
func relOrPath(projectRoot, path string) string {
	rel, err := filepath.Rel(projectRoot, path)
	if err != nil || strings.HasPrefix(rel, "..") {
		return path
	}
	return rel
}
