package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// readFixture reads one file under pkg/lint/_testdata/property/.
func readFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("_testdata", "property", name))
	if err != nil {
		t.Fatalf("read fixture %q: %v", name, err)
	}
	return string(data)
}

// writePropertyTree creates a fake spec tree where each entry in `files`
// maps a project-relative path (under `spec/`) to its contents. Returns
// specRoot.
func writePropertyTree(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	specRoot := filepath.Join(dir, "spec")
	for rel, content := range files {
		path := filepath.Join(specRoot, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return specRoot
}

// requireRule asserts that `vs` contains at least one violation with the
// given rule name, and returns the first such violation.
func requireRule(t *testing.T, vs []Violation, rule string) Violation {
	t.Helper()
	for _, v := range vs {
		if v.Rule == rule {
			return v
		}
	}
	t.Fatalf("expected violation with rule %q; got %+v", rule, vs)
	return Violation{}
}

func TestPropertyChecker_Clean(t *testing.T) {
	specRoot := writePropertyTree(t, map[string]string{
		"features/shared/email.property.md": readFixture(t, "valid-clean.property.md"),
	})
	vs, err := checkProperties(specRoot, false)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range vs {
		if strings.HasPrefix(v.Rule, "property-") {
			t.Errorf("expected no property-* violations; got %+v", v)
		}
	}
}

func TestPropertyChecker_Location(t *testing.T) {
	specRoot := writePropertyTree(t, map[string]string{
		// Misplaced: in spec/properties/ rather than spec/features/**.
		"properties/email.property.md": readFixture(t, "valid-clean.property.md"),
	})
	vs, _ := checkProperties(specRoot, false)
	requireRule(t, vs, "property-location")
}

func TestPropertyChecker_SlugFormat(t *testing.T) {
	// Email.property.md — uppercase slug.
	body := readFixture(t, "valid-clean.property.md")
	specRoot := writePropertyTree(t, map[string]string{
		"features/shared/Email.property.md": body,
	})
	vs, _ := checkProperties(specRoot, false)
	requireRule(t, vs, "property-slug-format")
}

func TestPropertyChecker_SingleFile(t *testing.T) {
	specRoot := writePropertyTree(t, map[string]string{
		// Directory named *.property/ — illegal per [property#req:single-file].
		"features/shared/email.property/README.md": "placeholder",
	})
	vs, _ := checkProperties(specRoot, false)
	requireRule(t, vs, "property-single-file")
}

func TestPropertyChecker_FrontmatterRequired(t *testing.T) {
	specRoot := writePropertyTree(t, map[string]string{
		"features/shared/missing-frontmatter.property.md": readFixture(t, "missing-frontmatter.property.md"),
	})
	vs, _ := checkProperties(specRoot, false)
	requireRule(t, vs, "property-frontmatter-required")
}

func TestPropertyChecker_FrontmatterRequiredFields(t *testing.T) {
	specRoot := writePropertyTree(t, map[string]string{
		"features/shared/frontmatter-missing-required-fields.property.md": readFixture(t, "frontmatter-missing-required-fields.property.md"),
	})
	vs, _ := checkProperties(specRoot, false)
	requireRule(t, vs, "property-frontmatter-required-fields")
}

func TestPropertyChecker_IDEqualsSlug(t *testing.T) {
	specRoot := writePropertyTree(t, map[string]string{
		"features/shared/email.property.md": readFixture(t, "id-mismatch-slug.property.md"),
	})
	vs, _ := checkProperties(specRoot, false)
	requireRule(t, vs, "property-id-equals-slug")
}

func TestPropertyChecker_DataTypeValues(t *testing.T) {
	specRoot := writePropertyTree(t, map[string]string{
		"features/shared/bad-type.property.md": readFixture(t, "invalid-data-type.property.md"),
	})
	vs, _ := checkProperties(specRoot, false)
	requireRule(t, vs, "property-data-type-values")
}

func TestPropertyChecker_ChecksShape_InapplicableError(t *testing.T) {
	specRoot := writePropertyTree(t, map[string]string{
		"features/shared/inapplicable-check.property.md": readFixture(t, "inapplicable-check.property.md"),
	})
	vs, _ := checkProperties(specRoot, false)
	v := requireRule(t, vs, "property-checks-shape")
	if v.Severity != "error" {
		t.Errorf("expected severity=error for inapplicable check; got %q", v.Severity)
	}
}

func TestPropertyChecker_ChecksShape_UnknownWarning(t *testing.T) {
	specRoot := writePropertyTree(t, map[string]string{
		"features/shared/unknown-check.property.md": readFixture(t, "unknown-check.property.md"),
	})
	vs, _ := checkProperties(specRoot, false)
	v := requireRule(t, vs, "property-checks-shape")
	if v.Severity != "warning" {
		t.Errorf("expected severity=warning for unknown check key; got %q", v.Severity)
	}

	// Re-confirm: filtering to default `--severity error` drops it.
	filtered := FilterBySeverity(vs, "error")
	for _, vv := range filtered {
		if vv.Rule == "property-checks-shape" {
			t.Errorf("severity=error filter should drop the warning-level shape violation")
		}
	}
}

func TestPropertyChecker_TitleFormat(t *testing.T) {
	specRoot := writePropertyTree(t, map[string]string{
		"features/shared/title-mismatch.property.md": readFixture(t, "title-mismatch.property.md"),
	})
	vs, _ := checkProperties(specRoot, false)
	requireRule(t, vs, "property-title-format")
}

func TestPropertyChecker_RequiredSections(t *testing.T) {
	specRoot := writePropertyTree(t, map[string]string{
		"features/shared/missing-sections.property.md": readFixture(t, "missing-sections.property.md"),
	})
	vs, _ := checkProperties(specRoot, false)
	requireRule(t, vs, "property-required-sections")
}

func TestPropertyChecker_ReferencedByManaged(t *testing.T) {
	specRoot := writePropertyTree(t, map[string]string{
		"features/shared/hand-edited-referenced-by.property.md": readFixture(t, "hand-edited-referenced-by.property.md"),
	})
	vs, _ := checkProperties(specRoot, false)
	requireRule(t, vs, "property-referenced-by-managed")
}

// ---------------------------------------------------------------------
// property-checks-shape applicability table (all 13 keys × 9 data_types).
// ---------------------------------------------------------------------

func TestPropertyChecksShapeApplicability(t *testing.T) {
	// (dataType, checkKey, wantViolation, wantSeverity)
	type row struct {
		dataType   string
		checkKey   string
		wantViol   bool
		wantSever  string
		wantUnknwn bool
	}
	cases := []row{
		// required + enum are valid for every type.
		{"string", "required", false, "", false},
		{"integer", "required", false, "", false},
		{"object", "enum", false, "", false},
		// min / max valid only for integer/number/date/datetime.
		{"integer", "min", false, "", false},
		{"string", "min", true, "error", false},
		{"boolean", "max", true, "error", false},
		// min_length / max_length valid for string + array.
		{"string", "min_length", false, "", false},
		{"array", "max_length", false, "", false},
		{"integer", "min_length", true, "error", false},
		// pattern, trim, lowercase, uppercase — string only.
		{"string", "pattern", false, "", false},
		{"integer", "pattern", true, "error", false},
		{"string", "trim", false, "", false},
		{"integer", "lowercase", true, "error", false},
		// items — array only.
		{"array", "items", false, "", false},
		{"string", "items", true, "error", false},
		// json_schema — object only.
		{"object", "json_schema", false, "", false},
		{"string", "json_schema", true, "error", false},
		// entity_ref — ref only.
		{"ref", "entity_ref", false, "", false},
		{"string", "entity_ref", true, "error", false},
		// Unknown key → warning regardless of data_type.
		{"string", "custom_validator", true, "warning", true},
	}

	for _, c := range cases {
		c := c
		name := c.dataType + "_" + c.checkKey
		t.Run(name, func(t *testing.T) {
			body := "---\nkind: property\nid: probe\ndata_type: " + c.dataType + "\nchecks:\n  " + c.checkKey + ": true\n---\n\n# Property: probe\n\n## Description\n\nProbe.\n\n## Referenced by\n\n<!-- managed-by: specscore lint --fix -->\n- _No references yet._\n<!-- end-managed -->\n\n---\n*This document follows the https://specscore.md/property-specification*\n"
			specRoot := writePropertyTree(t, map[string]string{
				"features/shared/probe.property.md": body,
			})
			vs, _ := checkProperties(specRoot, false)
			var found *Violation
			for i := range vs {
				if vs[i].Rule == "property-checks-shape" {
					found = &vs[i]
					break
				}
			}
			if c.wantViol {
				if found == nil {
					t.Fatalf("expected property-checks-shape violation for (data_type=%s, key=%s); got %+v", c.dataType, c.checkKey, vs)
				}
				if found.Severity != c.wantSever {
					t.Fatalf("expected severity=%q for (data_type=%s, key=%s); got %q", c.wantSever, c.dataType, c.checkKey, found.Severity)
				}
			} else {
				if found != nil {
					t.Fatalf("expected no property-checks-shape violation for (data_type=%s, key=%s); got %+v", c.dataType, c.checkKey, *found)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------
// `## Referenced by` rendering tests.
// ---------------------------------------------------------------------

const cleanEntityWithRefBody = `---
kind: entity
id: user
singular: User
plural: Users
description: Authenticated user.
properties:
  - name: email
    ref: ../shared/email.property.md
---

# Entity: User

## Description

A user.

## Properties

<!-- managed-by: specscore lint --fix -->
<!-- end-managed -->

## Referenced by

<!-- managed-by: specscore lint --fix -->
- _No references yet._
<!-- end-managed -->

---
*This document follows the https://specscore.md/entity-specification*
`

func TestRenderPropertyReferencedByFromEntities(t *testing.T) {
	specRoot := writePropertyTree(t, map[string]string{
		"features/shared/email.property.md": readFixture(t, "valid-clean.property.md"),
		"features/user/user.entity.md":      cleanEntityWithRefBody,
	})
	if _, err := runPropertyFix(specRoot); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(specRoot, "features", "shared", "email.property.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "- Entity: [user](../user/user.entity.md)") {
		t.Errorf("expected `## Referenced by` to list user entity; got:\n%s", got)
	}
}

const entityWithDuplicateRefs = `---
kind: entity
id: user
singular: User
plural: Users
description: Authenticated user with two email fields, both ref'ing email.property.md.
properties:
  - name: home_email
    ref: ../shared/email.property.md
  - name: work_email
    ref: ../shared/email.property.md
---

# Entity: User

## Description

A user.

## Properties

<!-- managed-by: specscore lint --fix -->
<!-- end-managed -->

## Referenced by

<!-- managed-by: specscore lint --fix -->
- _No references yet._
<!-- end-managed -->

---
*This document follows the https://specscore.md/entity-specification*
`

func TestRenderPropertyReferencedByDedup(t *testing.T) {
	specRoot := writePropertyTree(t, map[string]string{
		"features/shared/email.property.md": readFixture(t, "valid-clean.property.md"),
		"features/user/user.entity.md":      entityWithDuplicateRefs,
	})
	if _, err := runPropertyFix(specRoot); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(specRoot, "features", "shared", "email.property.md"))
	if err != nil {
		t.Fatal(err)
	}
	count := strings.Count(string(got), "- Entity: [user]")
	if count != 1 {
		t.Errorf("expected exactly 1 `- Entity: [user]` row in dedup case; got %d. content:\n%s", count, got)
	}
}

func TestRenderPropertyReferencedByNoReferencesFallback(t *testing.T) {
	specRoot := writePropertyTree(t, map[string]string{
		"features/shared/email.property.md": readFixture(t, "valid-clean.property.md"),
	})
	if _, err := runPropertyFix(specRoot); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(specRoot, "features", "shared", "email.property.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "- _No references yet._") {
		t.Errorf("expected fallback `- _No references yet._` for property with no consumers; got:\n%s", got)
	}
}

// Inline (non-ref) property items MUST NOT count as consumers.
const entityWithInlineEmail = `---
kind: entity
id: user
singular: User
plural: Users
description: User with INLINE email property (no ref).
properties:
  - name: email
    data_type: string
    checks:
      required: true
---

# Entity: User

## Description

A user with inline email property.

## Properties

<!-- managed-by: specscore lint --fix -->
<!-- end-managed -->

## Referenced by

<!-- managed-by: specscore lint --fix -->
- _No references yet._
<!-- end-managed -->

---
*This document follows the https://specscore.md/entity-specification*
`

func TestRenderPropertyReferencedByIgnoresInlineDefinitions(t *testing.T) {
	specRoot := writePropertyTree(t, map[string]string{
		"features/shared/email.property.md": readFixture(t, "valid-clean.property.md"),
		"features/user/user.entity.md":      entityWithInlineEmail,
	})
	if _, err := runPropertyFix(specRoot); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(filepath.Join(specRoot, "features", "shared", "email.property.md"))
	if strings.Contains(string(got), "- Entity: [user]") {
		t.Errorf("inline email property must NOT count as a consumer; got:\n%s", got)
	}
	if !strings.Contains(string(got), "- _No references yet._") {
		t.Errorf("expected fallback for property with only inline consumers; got:\n%s", got)
	}
}

// ---------------------------------------------------------------------
// Autofix tests.
// ---------------------------------------------------------------------

func TestPropertyIDEqualsSlugAutofix(t *testing.T) {
	specRoot := writePropertyTree(t, map[string]string{
		"features/shared/email.property.md": readFixture(t, "id-mismatch-slug.property.md"),
	})
	if _, err := runPropertyFix(specRoot); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(specRoot, "features", "shared", "email.property.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "id: email") {
		t.Errorf("expected post-fix `id: email`; got:\n%s", got)
	}
	if !strings.Contains(string(got), "# Authoritative comment that the autofix MUST preserve.") {
		t.Errorf("autofix must preserve YAML comments; got:\n%s", got)
	}
	// Post-fix, the property-id-equals-slug rule should no longer fire.
	vs, _ := checkProperties(specRoot, false)
	for _, v := range vs {
		if v.Rule == "property-id-equals-slug" {
			t.Errorf("post-fix lint still reports property-id-equals-slug: %+v", v)
		}
	}
}

func TestPropertyTitleFormatAutofix(t *testing.T) {
	specRoot := writePropertyTree(t, map[string]string{
		"features/shared/title-mismatch.property.md": readFixture(t, "title-mismatch.property.md"),
	})
	if _, err := runPropertyFix(specRoot); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(specRoot, "features", "shared", "title-mismatch.property.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(got), "# Property: title-mismatch") {
		t.Errorf("expected title rewritten to `# Property: title-mismatch`; got:\n%s", got)
	}
}

func TestPropertyFixIsIdempotent(t *testing.T) {
	specRoot := writePropertyTree(t, map[string]string{
		"features/shared/email.property.md": readFixture(t, "valid-clean.property.md"),
		"features/user/user.entity.md":      cleanEntityWithRefBody,
	})
	// First pass.
	if _, err := runPropertyFix(specRoot); err != nil {
		t.Fatal(err)
	}
	afterFirst, err := os.ReadFile(filepath.Join(specRoot, "features", "shared", "email.property.md"))
	if err != nil {
		t.Fatal(err)
	}
	// Second pass — MUST be byte-equal.
	if _, err := runPropertyFix(specRoot); err != nil {
		t.Fatal(err)
	}
	afterSecond, err := os.ReadFile(filepath.Join(specRoot, "features", "shared", "email.property.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(afterFirst) != string(afterSecond) {
		t.Errorf("--fix is not idempotent:\nfirst:\n%s\nsecond:\n%s", afterFirst, afterSecond)
	}
}

// ---------------------------------------------------------------------
// Registry registration test.
// ---------------------------------------------------------------------

func TestAllPropertyRuleNamesRegistered(t *testing.T) {
	all := AllRuleNames()
	for _, n := range propertyRuleNames {
		if !all[n] {
			t.Errorf("rule %q in propertyRuleNames is not in allRuleNames", n)
		}
	}
}
