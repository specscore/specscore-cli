package entity

import (
	"os"
	"testing"
)

// metaSpecUserEntity is the canonical smoke-test fixture from the upstream
// meta-spec repository — see plan T1's "Smoke-test fixture" reference.
// The path is a sibling checkout of specscore/specscore alongside
// this CLI repo; we skip when that checkout is absent so CI in
// containers without the sibling repo still passes.
const metaSpecUserEntity = "/home/ai/projects/specscore/specscore/spec/features/idea/user.entity.md"

func TestParse_MetaSpecUserEntitySmoke(t *testing.T) {
	if _, err := os.Stat(metaSpecUserEntity); err != nil {
		t.Skipf("meta-spec fixture not available at %s — skipping smoke test", metaSpecUserEntity)
	}

	doc, err := Parse(metaSpecUserEntity)
	if err != nil {
		t.Fatalf("Parse(%s) error: %v", metaSpecUserEntity, err)
	}
	if doc == nil {
		t.Fatal("expected non-nil Doc")
		return
	}
	if doc.Slug != "user" {
		t.Errorf("Slug = %q, want user", doc.Slug)
	}
	if doc.Frontmatter == nil {
		t.Fatal("Frontmatter should not be nil for the meta-spec smoke fixture")
	}
	if doc.Frontmatter.Kind != "entity" {
		t.Errorf("Kind = %q, want entity", doc.Frontmatter.Kind)
	}
	if doc.Frontmatter.ID != "user" {
		t.Errorf("ID = %q, want user", doc.Frontmatter.ID)
	}
	if doc.Frontmatter.Singular != "User" {
		t.Errorf("Singular = %q, want User", doc.Frontmatter.Singular)
	}
	if doc.Frontmatter.Plural != "Users" {
		t.Errorf("Plural = %q, want Users", doc.Frontmatter.Plural)
	}
	if len(doc.Properties) != 4 {
		t.Fatalf("Properties = %d items, want 4 (id, email, display_name, created_at)", len(doc.Properties))
	}
	wantNames := []string{"id", "email", "display_name", "created_at"}
	for i, want := range wantNames {
		if doc.Properties[i].Name != want {
			t.Errorf("Properties[%d].Name = %q, want %q", i, doc.Properties[i].Name, want)
		}
	}
	// The email property uses the ref: form.
	if doc.Properties[1].Ref != "./email.property.md" {
		t.Errorf("Properties[1].Ref = %q, want ./email.property.md", doc.Properties[1].Ref)
	}
	if doc.Properties[1].DataType != "" {
		t.Errorf("Properties[1].DataType = %q, want empty for ref-form item", doc.Properties[1].DataType)
	}
	// The id property is inline.
	if doc.Properties[0].DataType != "string" {
		t.Errorf("Properties[0].DataType = %q, want string", doc.Properties[0].DataType)
	}
	if req, ok := doc.Properties[0].Checks["required"]; !ok || req != true {
		t.Errorf("Properties[0].Checks[required] = %v (ok=%v), want true", req, ok)
	}
	// Body sections are all present.
	for _, want := range []string{"Description", "Properties", "Referenced by"} {
		if _, ok := doc.SectionByTitle[want]; !ok {
			t.Errorf("SectionByTitle[%q] missing", want)
		}
	}
	if !doc.HasTitle || doc.TitleName != "User" {
		t.Errorf("Title parse: HasTitle=%v, TitleName=%q", doc.HasTitle, doc.TitleName)
	}
}
