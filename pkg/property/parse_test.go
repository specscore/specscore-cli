package property

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestValidateSlug(t *testing.T) {
	cases := []struct {
		slug    string
		wantErr bool
	}{
		{"email", false},
		{"phone-number", false},
		{"customer-tier", false},
		{"iso-currency-code", false},
		{"a", false},
		{"a1-b2", false},
		{"", true},
		{"Email", true},
		{"email_address", true},
		{"-email", true},
		{"email-", true},
		{"email--address", true},
	}
	for _, tc := range cases {
		t.Run(tc.slug, func(t *testing.T) {
			err := ValidateSlug(tc.slug)
			if (err != nil) != tc.wantErr {
				t.Fatalf("ValidateSlug(%q) err=%v, wantErr=%v", tc.slug, err, tc.wantErr)
			}
		})
	}
}

func TestParse_ValidMinimal(t *testing.T) {
	path := filepath.Join("_testdata", "valid-minimal.property.md")
	doc, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// Slug derives from filename, not frontmatter id.
	if doc.Slug != "valid-minimal" {
		t.Errorf("Slug = %q, want %q", doc.Slug, "valid-minimal")
	}
	if !doc.HasTitle {
		t.Errorf("HasTitle = false, want true")
	}
	// TitleName comes from the body's `# Property: email` line.
	if doc.TitleName != "email" {
		t.Errorf("TitleName = %q, want %q", doc.TitleName, "email")
	}
	if doc.Frontmatter == nil {
		t.Fatal("Frontmatter is nil")
	}
	if doc.Frontmatter.Kind != "property" {
		t.Errorf("Frontmatter.Kind = %q, want %q", doc.Frontmatter.Kind, "property")
	}
	if doc.Frontmatter.ID != "email" {
		t.Errorf("Frontmatter.ID = %q, want %q", doc.Frontmatter.ID, "email")
	}
	if doc.Frontmatter.DataType != "string" {
		t.Errorf("Frontmatter.DataType = %q, want %q", doc.Frontmatter.DataType, "string")
	}
	if doc.Frontmatter.Checks == nil {
		t.Errorf("Frontmatter.Checks is nil; empty checks {} should produce a non-nil map")
	}
	if len(doc.Frontmatter.Checks) != 0 {
		t.Errorf("Frontmatter.Checks = %v, want empty", doc.Frontmatter.Checks)
	}
	if doc.FmRaw == nil {
		t.Errorf("FmRaw is nil; required for round-tripping")
	}
	if _, ok := doc.SectionByTitle["Description"]; !ok {
		t.Errorf("missing Description section")
	}
	if _, ok := doc.SectionByTitle["Referenced by"]; !ok {
		t.Errorf("missing Referenced by section")
	}
	if len(doc.RawLines) == 0 {
		t.Errorf("RawLines is empty")
	}
}

func TestParse_ValidWithChecks(t *testing.T) {
	path := filepath.Join("_testdata", "valid-with-checks.property.md")
	doc, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if doc.Frontmatter == nil {
		t.Fatal("Frontmatter is nil")
	}
	if doc.Frontmatter.Description == "" {
		t.Errorf("Description should be populated")
	}
	checks := doc.Frontmatter.Checks
	if len(checks) == 0 {
		t.Fatalf("Checks should be populated, got %v", checks)
	}
	if req, ok := checks["required"].(bool); !ok || !req {
		t.Errorf("checks.required = %v, want true", checks["required"])
	}
	// max_length unmarshals to int via yaml.v3.
	if ml, ok := checks["max_length"].(int); !ok || ml != 320 {
		t.Errorf("checks.max_length = %v (%T), want 320", checks["max_length"], checks["max_length"])
	}
	if _, ok := checks["pattern"].(string); !ok {
		t.Errorf("checks.pattern type = %T, want string", checks["pattern"])
	}
}

func TestParse_MissingFrontmatter_IsResilient(t *testing.T) {
	path := filepath.Join("_testdata", "missing-frontmatter.property.md")
	doc, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse should not error on missing frontmatter, got %v", err)
	}
	// Parse is resilient: Doc is non-nil on every readable file. The only
	// nil-Doc path is a non-nil error from os.Open, exercised separately.
	if doc.Frontmatter != nil {
		t.Errorf("Frontmatter should be nil when absent, got %+v", doc.Frontmatter)
	}
	if doc.FmRaw != nil {
		t.Errorf("FmRaw should be nil when frontmatter absent, got %+v", doc.FmRaw)
	}
	if !doc.HasTitle {
		t.Errorf("Title should still be parsed even without frontmatter")
	}
	if doc.Slug != "missing-frontmatter" {
		t.Errorf("Slug should be derived from filename even without frontmatter, got %q", doc.Slug)
	}
}

func TestParse_FrontmatterNotFirst_IsResilient(t *testing.T) {
	path := filepath.Join("_testdata", "frontmatter-not-first.property.md")
	doc, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// Frontmatter that is not the first block MUST NOT be picked up — lint
	// rule property-frontmatter-required reports the violation. Parser
	// surfaces a nil Frontmatter so lint can fire the diagnostic.
	if doc.Frontmatter != nil {
		t.Errorf("Frontmatter should be nil when not first, got %+v", doc.Frontmatter)
	}
}

func TestParse_InvalidDataType_SurfacesValueAsIs(t *testing.T) {
	path := filepath.Join("_testdata", "invalid-data-type.property.md")
	doc, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if doc.Frontmatter == nil {
		t.Fatal("Frontmatter should parse even if data_type is invalid")
	}
	// Parser does NOT validate — it surfaces the raw value. The lint rule
	// property-data-type-values does the classification.
	if doc.Frontmatter.DataType != "blob" {
		t.Errorf("DataType = %q, want %q (parser must not normalise)", doc.Frontmatter.DataType, "blob")
	}
	if LegalDataTypes[doc.Frontmatter.DataType] {
		t.Errorf("LegalDataTypes[%q] should be false", doc.Frontmatter.DataType)
	}
}

func TestParse_UnknownCheckKey_SurfacedRaw(t *testing.T) {
	path := filepath.Join("_testdata", "unknown-check-key.property.md")
	doc, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if doc.Frontmatter == nil {
		t.Fatal("Frontmatter is nil")
	}
	if _, ok := doc.Frontmatter.Checks["custom_validator"]; !ok {
		t.Errorf("Checks should surface custom_validator key as-is, got %v", doc.Frontmatter.Checks)
	}
	// CheckKeyApplicability does not classify it — that's lint's job, but the
	// helper map must NOT contain it (else lint would silently pass).
	if _, ok := CheckKeyApplicability["custom_validator"]; ok {
		t.Errorf("CheckKeyApplicability should NOT contain custom_validator")
	}
}

func TestParse_IDMismatchSlug(t *testing.T) {
	path := filepath.Join("_testdata", "id-mismatch-slug.property.md")
	doc, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if doc.Slug != "id-mismatch-slug" {
		t.Errorf("Slug derives from filename: got %q, want %q", doc.Slug, "id-mismatch-slug")
	}
	if doc.Frontmatter == nil || doc.Frontmatter.ID != "emai" {
		t.Errorf("Frontmatter.ID = %q, want %q (lint compares; parser surfaces)", doc.Frontmatter.ID, "emai")
	}
}

func TestParse_FmRawIsYAMLNode(t *testing.T) {
	path := filepath.Join("_testdata", "valid-with-checks.property.md")
	doc, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if doc.FmRaw == nil {
		t.Fatal("FmRaw is nil; required for autofix round-trip")
	}
	// The node should be a MappingNode at the root (frontmatter is a YAML map).
	// Or a DocumentNode wrapping a MappingNode — either is acceptable.
	n := doc.FmRaw
	if n.Kind == yaml.DocumentNode && len(n.Content) > 0 {
		n = n.Content[0]
	}
	if n.Kind != yaml.MappingNode {
		t.Fatalf("FmRaw root expected MappingNode, got Kind=%d", n.Kind)
	}
	// Round-trip: marshalling FmRaw should produce something parseable
	// containing the id key with value "email".
	out, err := yaml.Marshal(doc.FmRaw)
	if err != nil {
		t.Fatalf("yaml.Marshal(FmRaw): %v", err)
	}
	if !strings.Contains(string(out), "id: email") {
		t.Errorf("round-trip output missing `id: email`:\n%s", out)
	}
}

func TestParse_LegalDataTypesEnumeration(t *testing.T) {
	// The meta-spec enumeration MUST match exactly.
	want := []string{"string", "integer", "number", "boolean", "date", "datetime", "object", "array", "ref"}
	for _, dt := range want {
		if !LegalDataTypes[dt] {
			t.Errorf("LegalDataTypes[%q] should be true", dt)
		}
	}
	if len(LegalDataTypes) != len(want) {
		t.Errorf("LegalDataTypes size = %d, want %d (extras: %v)", len(LegalDataTypes), len(want), LegalDataTypes)
	}
}

func TestParse_CheckKeyApplicability(t *testing.T) {
	// Spot-check a few entries to lock in the matrix from property#req:checks-shape.
	if !CheckKeyApplicability["required"]["string"] {
		t.Errorf("required should apply to string")
	}
	if !CheckKeyApplicability["pattern"]["string"] {
		t.Errorf("pattern should apply to string")
	}
	if CheckKeyApplicability["pattern"]["integer"] {
		t.Errorf("pattern MUST NOT apply to integer")
	}
	if !CheckKeyApplicability["min"]["integer"] {
		t.Errorf("min should apply to integer")
	}
	if CheckKeyApplicability["min"]["string"] {
		t.Errorf("min MUST NOT apply to string")
	}
	if !CheckKeyApplicability["items"]["array"] {
		t.Errorf("items should apply to array")
	}
	if !CheckKeyApplicability["json_schema"]["object"] {
		t.Errorf("json_schema should apply to object")
	}
	if !CheckKeyApplicability["entity_ref"]["ref"] {
		t.Errorf("entity_ref should apply to ref")
	}
}

// TestParse_MetaSpecSmokeFixture drives Parse against the canonical meta-spec
// smoke fixture (when available on the developer's machine). Skips when the
// upstream checkout is absent so this test is hermetic enough for CI.
func TestParse_MetaSpecSmokeFixture(t *testing.T) {
	path := "/home/ai/projects/specscore/specscore/spec/features/idea/email.property.md"
	if _, err := os.Stat(path); err != nil {
		t.Skipf("meta-spec smoke fixture not available at %s; skipping", path)
	}
	doc, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse(meta-spec smoke fixture): %v", err)
	}
	if doc.Slug != "email" {
		t.Errorf("Slug = %q, want %q", doc.Slug, "email")
	}
	if doc.Frontmatter == nil {
		t.Fatal("Frontmatter is nil for the canonical fixture")
	}
	if doc.Frontmatter.Kind != "property" {
		t.Errorf("Kind = %q, want %q", doc.Frontmatter.Kind, "property")
	}
	if doc.Frontmatter.ID != "email" {
		t.Errorf("ID = %q, want %q", doc.Frontmatter.ID, "email")
	}
	if doc.Frontmatter.DataType != "string" {
		t.Errorf("DataType = %q, want %q", doc.Frontmatter.DataType, "string")
	}
	if !LegalDataTypes[doc.Frontmatter.DataType] {
		t.Errorf("DataType %q should be legal", doc.Frontmatter.DataType)
	}
	if _, ok := doc.Frontmatter.Checks["required"]; !ok {
		t.Errorf("Checks should include `required`, got %v", doc.Frontmatter.Checks)
	}
	if _, ok := doc.SectionByTitle["Description"]; !ok {
		t.Errorf("missing Description section")
	}
	if _, ok := doc.SectionByTitle["Referenced by"]; !ok {
		t.Errorf("missing Referenced by section")
	}
}

func TestParse_RawLinesPreservedByteForByte(t *testing.T) {
	path := filepath.Join("_testdata", "valid-minimal.property.md")
	doc, err := Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	// The first line should be the opening frontmatter delimiter.
	if len(doc.RawLines) == 0 || doc.RawLines[0] != "---" {
		t.Errorf("RawLines[0] = %q, want %q", doc.RawLines[0], "---")
	}
	// Title line content should be present somewhere.
	found := false
	for _, l := range doc.RawLines {
		if l == "# Property: email" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("title line not found in RawLines")
	}
}
