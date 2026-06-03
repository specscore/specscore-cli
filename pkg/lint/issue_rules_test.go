package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// AC: type-registered (rule-family half — every I-NNN ID is wired to a
// checker that the linter knows about).
func TestIssueRules_AllFifteenIDsRegistered(t *testing.T) {
	l := newLinter(Options{SpecRoot: t.TempDir()})
	for _, id := range issueRuleIDs {
		if _, ok := l.ruleSet[id]; !ok {
			t.Errorf("rule %q is not registered with the linter", id)
		}
	}
	// Sanity: every ID is also in allRuleNames (so ValidateRuleNames accepts it).
	all := AllRuleNames()
	for _, id := range issueRuleIDs {
		if !all[id] {
			t.Errorf("rule %q missing from allRuleNames", id)
		}
	}
}

// AC: default-suite-includes-i-rules. A spec tree with one valid issue
// must lint with zero violations when no flags are passed (all 15 I-
// rules run; stubs emit nothing; I-009 sees a pattern-matching path).
func TestIssueRules_DefaultSuiteIncludesIRules(t *testing.T) {
	specRoot := copyTestdataSpec(t, "rules/issue/testdata/default-suite/spec")
	vs, err := Lint(Options{SpecRoot: specRoot})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	for _, v := range vs {
		if v.Rule[:2] == "I-" {
			t.Errorf("unexpected I-rule violation on default-suite fixture: %+v", v)
		}
	}
}

// AC: rules-filter-by-id. When --rules I-009 is set, only I-009
// violations surface (other I-rules whose stubs would emit nothing
// anyway are filtered too, but the key invariant is: a non-I-009
// violation that would otherwise fire is suppressed).
//
// In this scaffold only I-009 has logic, so we exercise the filter by
// confirming (a) ValidateRuleNames accepts "I-009" alone, and (b) when
// the fixture trips I-009, the violation surfaces under --rules=I-009
// and is the only one.
func TestIssueRules_FilterByID_AcceptsI009(t *testing.T) {
	if err := ValidateRuleNames([]string{"I-009"}); err != nil {
		t.Fatalf("ValidateRuleNames(I-009): %v", err)
	}
}

func TestIssueRules_FilterByID_OnlyI009Emits(t *testing.T) {
	specRoot := copyTestdataSpec(t, "rules/issue/testdata/dual-location/spec")
	vs, err := Lint(Options{SpecRoot: specRoot, Rules: []string{"I-009"}})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	if len(vs) == 0 {
		t.Fatalf("expected at least one I-009 violation; got none")
	}
	for _, v := range vs {
		if v.Rule != "I-009" {
			t.Errorf("unexpected rule under --rules=I-009 filter: %+v", v)
		}
	}
}

// AC: dual-location-violation. A file at spec/random-dir/foo.md
// declaring `type: issue` triggers I-009.
func TestIssueRules_I009_DualLocationViolation(t *testing.T) {
	specRoot := copyTestdataSpec(t, "rules/issue/testdata/dual-location/spec")
	vs, err := Lint(Options{SpecRoot: specRoot})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	var i009 *Violation
	for i := range vs {
		if vs[i].Rule == "I-009" {
			i009 = &vs[i]
			break
		}
	}
	if i009 == nil {
		t.Fatalf("expected an I-009 violation; got %+v", vs)
	}
	if i009.File != "random-dir/foo.md" {
		t.Errorf("violation file = %q; want %q", i009.File, "random-dir/foo.md")
	}
	if i009.Severity != "error" {
		t.Errorf("violation severity = %q; want %q", i009.Severity, "error")
	}
}

// AC: missing-required-field-violation. A fixture issue missing the
// `captured_by` frontmatter field trips I-001 with a "missing required
// field" message naming the field.
func TestIssueRules_I001_MissingRequiredField(t *testing.T) {
	specRoot := copyTestdataSpec(t, "rules/issue/testdata/missing-field/spec")
	vs, err := Lint(Options{SpecRoot: specRoot})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	var i001 []Violation
	for _, v := range vs {
		if v.Rule == "I-001" {
			i001 = append(i001, v)
		}
	}
	if len(i001) == 0 {
		t.Fatalf("expected an I-001 violation; got %+v", vs)
	}
	found := false
	for _, v := range i001 {
		if strings.Contains(v.Message, "captured_by") && strings.Contains(v.Message, "missing") {
			found = true
			if v.Severity != "error" {
				t.Errorf("severity = %q; want error", v.Severity)
			}
		}
	}
	if !found {
		t.Errorf("expected I-001 violation naming captured_by under 'missing' template; got %+v", i001)
	}
}

// AC: invalid-status-enum-violation. A fixture issue with
// `status: triaged` trips I-002 listing the four valid values.
func TestIssueRules_I002_InvalidStatusEnum(t *testing.T) {
	specRoot := copyTestdataSpec(t, "rules/issue/testdata/invalid-status/spec")
	vs, err := Lint(Options{SpecRoot: specRoot})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	var i002 *Violation
	for i := range vs {
		if vs[i].Rule == "I-002" {
			i002 = &vs[i]
			break
		}
	}
	if i002 == nil {
		t.Fatalf("expected an I-002 violation; got %+v", vs)
	}
	for _, want := range []string{"open", "investigating", "resolved", "rejected"} {
		if !strings.Contains(i002.Message, want) {
			t.Errorf("I-002 message %q does not list valid value %q", i002.Message, want)
		}
	}
	if !strings.Contains(i002.Message, "triaged") {
		t.Errorf("I-002 message %q does not name the invalid value 'triaged'", i002.Message)
	}
}

// AC: unknown-frontmatter-key-violation. A fixture issue with an
// unknown `priority: high` key trips I-001 under a distinct "unknown
// field" template naming `priority`.
func TestIssueRules_I001_UnknownFrontmatterKey(t *testing.T) {
	specRoot := copyTestdataSpec(t, "rules/issue/testdata/unknown-key/spec")
	vs, err := Lint(Options{SpecRoot: specRoot})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	var i001 []Violation
	for _, v := range vs {
		if v.Rule == "I-001" {
			i001 = append(i001, v)
		}
	}
	if len(i001) == 0 {
		t.Fatalf("expected an I-001 violation; got %+v", vs)
	}
	found := false
	for _, v := range i001 {
		if strings.Contains(v.Message, "unknown") && strings.Contains(v.Message, "priority") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected I-001 violation under 'unknown' template naming priority; got %+v", i001)
	}
	// Also ensure no "missing" violation was emitted (required fields are all present).
	for _, v := range i001 {
		if strings.Contains(v.Message, "missing") {
			t.Errorf("unexpected 'missing' I-001 violation when all required fields are present: %+v", v)
		}
	}
}

// AC: optional-field-shape-violation. A fixture issue with
// `severity: extreme` trips I-003 under a message listing the five
// valid `severity` values (`low`, `medium`, `high`, `critical`, `unset`).
func TestIssueRules_I003_InvalidSeverityEnum(t *testing.T) {
	specRoot := copyTestdataSpec(t, "rules/issue/testdata/invalid-severity/spec")
	vs, err := Lint(Options{SpecRoot: specRoot})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	var i003 *Violation
	for i := range vs {
		if vs[i].Rule == "I-003" {
			i003 = &vs[i]
			break
		}
	}
	if i003 == nil {
		t.Fatalf("expected an I-003 violation; got %+v", vs)
	}
	for _, want := range []string{"low", "medium", "high", "critical", "unset"} {
		if !strings.Contains(i003.Message, want) {
			t.Errorf("I-003 message %q does not list valid value %q", i003.Message, want)
		}
	}
	if !strings.Contains(i003.Message, "severity") {
		t.Errorf("I-003 message %q does not name the field 'severity'", i003.Message)
	}
	if i003.Severity != "error" {
		t.Errorf("severity = %q; want error", i003.Severity)
	}
}

// AC: bugs-opaque-non-string-violation. A fixture issue with
// `bugs: [123, "valid-slug"]` trips I-004 stating every element of
// `bugs` must be a string.
func TestIssueRules_I004_BugsNonStringElement(t *testing.T) {
	specRoot := copyTestdataSpec(t, "rules/issue/testdata/bugs-non-string/spec")
	vs, err := Lint(Options{SpecRoot: specRoot})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	var i004 *Violation
	for i := range vs {
		if vs[i].Rule == "I-004" {
			i004 = &vs[i]
			break
		}
	}
	if i004 == nil {
		t.Fatalf("expected an I-004 violation; got %+v", vs)
	}
	if !strings.Contains(i004.Message, "bugs") {
		t.Errorf("I-004 message %q does not reference the `bugs` field", i004.Message)
	}
	if !strings.Contains(i004.Message, "string") {
		t.Errorf("I-004 message %q does not state elements must be strings", i004.Message)
	}
	if !strings.Contains(i004.Message, "123") {
		t.Errorf("I-004 message %q does not reference the non-string element 123", i004.Message)
	}
	if i004.Severity != "error" {
		t.Errorf("severity = %q; want error", i004.Severity)
	}
}

// AC: severity-required-on-transition-violation. A fixture issue with
// `status: investigating` and no `severity` field trips I-005 with a
// message naming severity-required-on-transition.
func TestIssueRules_I005_SeverityRequiredOnTransition(t *testing.T) {
	specRoot := copyTestdataSpec(t, "rules/issue/testdata/severity-required-transition/spec")
	vs, err := Lint(Options{SpecRoot: specRoot})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	var i005 *Violation
	for i := range vs {
		if vs[i].Rule == "I-005" {
			i005 = &vs[i]
			break
		}
	}
	if i005 == nil {
		t.Fatalf("expected an I-005 violation; got %+v", vs)
	}
	if !strings.Contains(i005.Message, "severity-required-on-transition") {
		t.Errorf("I-005 message %q does not name severity-required-on-transition", i005.Message)
	}
	if i005.Severity != "error" {
		t.Errorf("severity = %q; want error", i005.Severity)
	}
	// I-006 must NOT fire here: status != rejected and rejection_reason absent.
	for _, v := range vs {
		if v.Rule == "I-006" {
			t.Errorf("unexpected I-006 violation on severity-required-transition fixture: %+v", v)
		}
	}
}

// AC: rejection-reason-enum-violation. A fixture issue with
// `status: rejected`, valid `severity: low`, and
// `rejection_reason: not-real-enough` trips I-006 listing the six valid
// values. I-005 must stay silent (severity is set to a valid non-unset
// value).
func TestIssueRules_I006_RejectionReasonEnum(t *testing.T) {
	specRoot := copyTestdataSpec(t, "rules/issue/testdata/rejection-reason-enum/spec")
	vs, err := Lint(Options{SpecRoot: specRoot})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	var i006 *Violation
	for i := range vs {
		if vs[i].Rule == "I-006" {
			i006 = &vs[i]
			break
		}
	}
	if i006 == nil {
		t.Fatalf("expected an I-006 violation; got %+v", vs)
	}
	for _, want := range []string{
		"not-a-defect", "wont-fix", "duplicate",
		"not-reproducible", "by-design", "deferred",
	} {
		if !strings.Contains(i006.Message, want) {
			t.Errorf("I-006 message %q does not list valid value %q", i006.Message, want)
		}
	}
	if !strings.Contains(i006.Message, "not-real-enough") {
		t.Errorf("I-006 message %q does not name the invalid value 'not-real-enough'", i006.Message)
	}
	if i006.Severity != "error" {
		t.Errorf("severity = %q; want error", i006.Severity)
	}
	// Disambiguation: I-005 must NOT fire — severity is `low` (valid, non-unset).
	for _, v := range vs {
		if v.Rule == "I-005" {
			t.Errorf("unexpected I-005 violation on rejection-reason-enum fixture (severity is `low`): %+v", v)
		}
	}
}

// AC: h1-prefix-violation. A fixture issue whose H1 reads
// `# Bug: Menu crashes` (instead of `# Issue: ...`) trips I-007 with a
// message stating the H1 must match `^# Issue: .+$`. No other I-rule
// should fire on this fixture.
func TestIssueRules_I007_H1PrefixViolation(t *testing.T) {
	specRoot := copyTestdataSpec(t, "rules/issue/testdata/h1-prefix/spec")
	vs, err := Lint(Options{SpecRoot: specRoot})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	var i007 *Violation
	for i := range vs {
		if vs[i].Rule == "I-007" {
			i007 = &vs[i]
			break
		}
	}
	if i007 == nil {
		t.Fatalf("expected an I-007 violation; got %+v", vs)
	}
	if !strings.Contains(i007.Message, "^# Issue: .+$") {
		t.Errorf("I-007 message %q does not name the required H1 pattern", i007.Message)
	}
	if i007.Severity != "error" {
		t.Errorf("severity = %q; want error", i007.Severity)
	}
	// No I-008 should fire — the three required sections are present
	// in canonical order with non-empty bodies.
	for _, v := range vs {
		if v.Rule == "I-008" {
			t.Errorf("unexpected I-008 violation on h1-prefix fixture: %+v", v)
		}
	}
}

// AC: body-section-order-violation. A fixture issue with the three
// required H2 sections present but in non-canonical order trips I-008
// with a message naming the canonical order. I-007 must stay silent
// (the H1 is canonical).
func TestIssueRules_I008_BodySectionOrderViolation(t *testing.T) {
	specRoot := copyTestdataSpec(t, "rules/issue/testdata/body-section-order/spec")
	vs, err := Lint(Options{SpecRoot: specRoot})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	var i008 *Violation
	for i := range vs {
		if vs[i].Rule == "I-008" {
			i008 = &vs[i]
			break
		}
	}
	if i008 == nil {
		t.Fatalf("expected an I-008 violation; got %+v", vs)
	}
	if !strings.Contains(i008.Message, "canonical order") {
		t.Errorf("I-008 message %q does not mention canonical order", i008.Message)
	}
	for _, want := range []string{"Description", "Steps to Reproduce", "Expected vs Actual"} {
		if !strings.Contains(i008.Message, want) {
			t.Errorf("I-008 message %q does not name canonical section %q", i008.Message, want)
		}
	}
	if i008.Severity != "error" {
		t.Errorf("severity = %q; want error", i008.Severity)
	}
	// I-007 must stay silent — the H1 is `# Issue: Foo`.
	for _, v := range vs {
		if v.Rule == "I-007" {
			t.Errorf("unexpected I-007 violation on body-section-order fixture: %+v", v)
		}
	}
}

// AC: slug-mismatch-violation. A fixture issue at spec/issues/foo.md
// whose frontmatter `slug` is `bar` trips I-010 with a message naming
// the mismatch.
func TestIssueRules_I010_SlugMismatchViolation(t *testing.T) {
	specRoot := copyTestdataSpec(t, "rules/issue/testdata/slug-mismatch/spec")
	vs, err := Lint(Options{SpecRoot: specRoot})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	var i010 *Violation
	for i := range vs {
		if vs[i].Rule == "I-010" {
			i010 = &vs[i]
			break
		}
	}
	if i010 == nil {
		t.Fatalf("expected an I-010 violation; got %+v", vs)
	}
	if !strings.Contains(i010.Message, "bar") {
		t.Errorf("I-010 message %q does not name the frontmatter slug 'bar'", i010.Message)
	}
	if !strings.Contains(i010.Message, "foo") {
		t.Errorf("I-010 message %q does not name the filename slug 'foo'", i010.Message)
	}
	if i010.Severity != "error" {
		t.Errorf("severity = %q; want error", i010.Severity)
	}
	if i010.File != "issues/foo.md" {
		t.Errorf("violation file = %q; want %q", i010.File, "issues/foo.md")
	}
}

// AC: slug-globally-unique-violation. Two fixture issues at
// spec/issues/foo.md and spec/features/example/issues/foo.md, both
// lint-valid in isolation, trip I-011 with a message naming both paths
// and the colliding slug `foo`.
func TestIssueRules_I011_GlobalSlugUniquenessViolation(t *testing.T) {
	specRoot := copyTestdataSpec(t, "rules/issue/testdata/slug-globally-unique/spec")
	vs, err := Lint(Options{SpecRoot: specRoot})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	var i011 []Violation
	for _, v := range vs {
		if v.Rule == "I-011" {
			i011 = append(i011, v)
		}
	}
	if len(i011) == 0 {
		t.Fatalf("expected at least one I-011 violation; got %+v", vs)
	}
	// One violation should name both paths and the colliding slug.
	found := false
	for _, v := range i011 {
		if strings.Contains(v.Message, "foo") &&
			strings.Contains(v.Message, "issues/foo.md") &&
			strings.Contains(v.Message, "features/example/issues/foo.md") {
			found = true
			if v.Severity != "error" {
				t.Errorf("severity = %q; want error", v.Severity)
			}
		}
	}
	if !found {
		t.Errorf("expected I-011 violation naming both paths and slug 'foo'; got %+v", i011)
	}
}

// AC: affected-component-ref-violation. A fixture issue with
// `affected_component: nonexistent-feature` and no
// `spec/features/nonexistent-feature/README.md` trips I-012 with a
// message naming the unresolved slug.
func TestIssueRules_I012_AffectedComponentRefViolation(t *testing.T) {
	specRoot := copyTestdataSpec(t, "rules/issue/testdata/affected-component-ref/spec")
	vs, err := Lint(Options{SpecRoot: specRoot})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	var i012 []Violation
	for _, v := range vs {
		if v.Rule == "I-012" {
			i012 = append(i012, v)
		}
	}
	if len(i012) != 1 {
		t.Fatalf("expected exactly one I-012 violation; got %d: %+v", len(i012), i012)
	}
	v := i012[0]
	if !strings.Contains(v.Message, "nonexistent-feature") {
		t.Errorf("I-012 message %q does not name the unresolved slug 'nonexistent-feature'", v.Message)
	}
	if v.Severity != "error" {
		t.Errorf("severity = %q; want error", v.Severity)
	}
	if v.File != "issues/foo.md" {
		t.Errorf("violation file = %q; want %q", v.File, "issues/foo.md")
	}
}

// AC: affected-component-ref-violation (positive case). When
// `affected_component` resolves to an existing
// `spec/features/<slug>/README.md`, I-012 stays silent.
func TestIssueRules_I012_AffectedComponentRefResolves(t *testing.T) {
	specRoot := copyTestdataSpec(t, "rules/issue/testdata/affected-component-ref-resolves/spec")
	vs, err := Lint(Options{SpecRoot: specRoot})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	for _, v := range vs {
		if v.Rule == "I-012" {
			t.Errorf("unexpected I-012 violation on resolving fixture: %+v", v)
		}
	}
}

// AC: root-index-required-violation. A fixture with an issue under
// `spec/issues/foo.md` but no `spec/issues/README.md` trips I-013 with
// a message naming the missing index path.
func TestIssueRules_I013_RootIndexRequiredViolation(t *testing.T) {
	specRoot := copyTestdataSpec(t, "rules/issue/testdata/root-index-required/spec")
	vs, err := Lint(Options{SpecRoot: specRoot})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	var i013 *Violation
	for i := range vs {
		if vs[i].Rule == "I-013" {
			i013 = &vs[i]
			break
		}
	}
	if i013 == nil {
		t.Fatalf("expected an I-013 violation; got %+v", vs)
	}
	if !strings.Contains(i013.Message, "issues/README.md") {
		t.Errorf("I-013 message %q does not name the missing index path", i013.Message)
	}
	if i013.Severity != "error" {
		t.Errorf("severity = %q; want error", i013.Severity)
	}
	if i013.File != "issues/README.md" {
		t.Errorf("violation file = %q; want %q", i013.File, "issues/README.md")
	}
}

// AC: root-index-fix-scaffolds-readme. Running --fix against the same
// fixture creates `spec/issues/README.md` with the canonical minimal
// index template and a follow-up lint pass reports zero I-013
// violations. The scaffold is also idempotent — running --fix twice
// produces no diff on the second run.
func TestIssueRules_I013_FixScaffoldsReadme(t *testing.T) {
	specRoot := copyTestdataSpec(t, "rules/issue/testdata/root-index-required/spec")
	indexPath := filepath.Join(specRoot, "issues", "README.md")

	// Sanity precondition: the README must be absent before --fix.
	if _, err := os.Stat(indexPath); err == nil {
		t.Fatalf("precondition: %s already exists", indexPath)
	}

	// First pass with --fix.
	if _, err := Lint(Options{SpecRoot: specRoot, Fix: true}); err != nil {
		t.Fatalf("Lint --fix: %v", err)
	}
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatalf("expected %s to exist after --fix: %v", indexPath, err)
	}
	got := string(data)
	for _, want := range []string{
		"type: index",
		"**Status:** Stable",
		"## Contents",
		"| Slug | Title | Status | Severity | Captured |",
		"## Open Questions",
		"None at this time.",
		"specscore.md/",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("scaffolded README missing %q\n---\n%s", want, got)
		}
	}

	// Follow-up lint pass without --fix: I-013 must no longer fire on
	// this fixture.
	vs, err := Lint(Options{SpecRoot: specRoot})
	if err != nil {
		t.Fatalf("Lint (post-fix): %v", err)
	}
	for _, v := range vs {
		if v.Rule == "I-013" {
			t.Errorf("unexpected I-013 violation after --fix: %+v", v)
		}
	}

	// Idempotency: running --fix again must not change the file.
	before, _ := os.ReadFile(indexPath)
	if _, err := Lint(Options{SpecRoot: specRoot, Fix: true}); err != nil {
		t.Fatalf("Lint --fix (second pass): %v", err)
	}
	after, _ := os.ReadFile(indexPath)
	if string(before) != string(after) {
		t.Errorf("--fix is not idempotent: file changed on second pass\nbefore:\n%s\nafter:\n%s", before, after)
	}
}

// AC: feature-scoped-index-required-violation. A fixture with an issue
// under `spec/features/example/issues/foo.md` but no neighbouring
// README.md trips I-014 with a message naming the missing index path.
func TestIssueRules_I014_FeatureScopedIndexRequiredViolation(t *testing.T) {
	specRoot := copyTestdataSpec(t, "rules/issue/testdata/feature-index-required/spec")
	vs, err := Lint(Options{SpecRoot: specRoot})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	var i014 *Violation
	for i := range vs {
		if vs[i].Rule == "I-014" {
			i014 = &vs[i]
			break
		}
	}
	if i014 == nil {
		t.Fatalf("expected an I-014 violation; got %+v", vs)
	}
	if !strings.Contains(i014.Message, "features/example/issues/README.md") {
		t.Errorf("I-014 message %q does not name the missing index path", i014.Message)
	}
	if i014.Severity != "error" {
		t.Errorf("severity = %q; want error", i014.Severity)
	}
	if i014.File != "features/example/issues/README.md" {
		t.Errorf("violation file = %q; want %q", i014.File, "features/example/issues/README.md")
	}
}

// AC: index-columns-violation. A fixture issues-index README whose
// Contents table is missing the `Severity` column trips I-015 with a
// message listing the five required columns in canonical order.
func TestIssueRules_I015_IndexColumnsViolation(t *testing.T) {
	specRoot := copyTestdataSpec(t, "rules/issue/testdata/index-columns/spec")
	vs, err := Lint(Options{SpecRoot: specRoot})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	var i015 *Violation
	for i := range vs {
		if vs[i].Rule == "I-015" {
			i015 = &vs[i]
			break
		}
	}
	if i015 == nil {
		t.Fatalf("expected an I-015 violation; got %+v", vs)
	}
	for _, want := range []string{"Slug", "Title", "Status", "Severity", "Captured"} {
		if !strings.Contains(i015.Message, want) {
			t.Errorf("I-015 message %q does not name canonical column %q", i015.Message, want)
		}
	}
	// Order check: the message must list the columns in canonical order.
	canonical := "Slug, Title, Status, Severity, Captured"
	if !strings.Contains(i015.Message, canonical) {
		t.Errorf("I-015 message %q does not list columns in canonical order %q", i015.Message, canonical)
	}
	if i015.Severity != "error" {
		t.Errorf("severity = %q; want error", i015.Severity)
	}
	if i015.File != "issues/README.md" {
		t.Errorf("violation file = %q; want %q", i015.File, "issues/README.md")
	}
}

// copyTestdataSpec copies the spec/ subtree of a testdata fixture into
// a temporary spec root. The fixture directory passed must contain a
// `spec/` subtree (rules/issue/testdata/<name>/spec/...). The function
// returns the path to the temp `spec/` directory.
func copyTestdataSpec(t *testing.T, fixtureSpecPath string) string {
	t.Helper()
	srcRoot := filepath.Join(fixtureSpecPath)
	dstRoot := filepath.Join(t.TempDir(), "spec")
	if err := os.MkdirAll(dstRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	err := filepath.Walk(srcRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(srcRoot, path)
		if relErr != nil {
			return relErr
		}
		dst := filepath.Join(dstRoot, rel)
		if info.IsDir() {
			return os.MkdirAll(dst, 0o755)
		}
		data, rerr := os.ReadFile(path)
		if rerr != nil {
			return rerr
		}
		return os.WriteFile(dst, data, 0o644)
	})
	if err != nil {
		t.Fatalf("copy testdata: %v", err)
	}
	return dstRoot
}
