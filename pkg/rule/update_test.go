package rule

import (
	"strings"
	"testing"
)

const editableRule = `---
format: https://specscore.md/rule-specification
status: Draft
---

# Rule: X

**Status:** Draft
**Date:** 2026-09-03
**Owner:** alex
**Statement:** Always x.
**Scope:** fleet
**Enforcement:** Stated
**Control:** —
**Sources:** —
**Why:** Because.
**Exceptions:** none
**Supersedes:** —
**Superseded By:** —

<!-- a hand-written note the fixer must never touch -->

## Open Questions

Is the fleet scope right here?

---
*This document follows the https://specscore.md/rule-specification*
`

func TestApplyFieldEditsRewritesInPlace(t *testing.T) {
	got, err := ApplyFieldEdits([]byte(editableRule), []FieldEdit{
		{Name: "Statement", Value: "Never x."},
		{Name: "Enforcement", Value: "Enforced"},
		{Name: "Control", Value: "wb pre-push hook"},
	})
	if err != nil {
		t.Fatalf("ApplyFieldEdits: %v", err)
	}
	text := string(got)
	for _, want := range []string{
		"**Statement:** Never x.\n",
		"**Enforcement:** Enforced\n",
		"**Control:** wb pre-push hook\n",
		"<!-- a hand-written note the fixer must never touch -->",
		"Is the fleet scope right here?",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("edit lost %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "**Statement:** Always x.") {
		t.Error("old statement retained")
	}
}

// The frontmatter mirror must move with the body value, or every status edit
// would leave the artifact failing the status-mirror rule.
func TestApplyFieldEditsMirrorsStatusIntoFrontmatter(t *testing.T) {
	got, err := ApplyFieldEdits([]byte(editableRule), []FieldEdit{{Name: "Status", Value: "Active"}})
	if err != nil {
		t.Fatalf("ApplyFieldEdits: %v", err)
	}
	text := string(got)
	if !strings.Contains(text, "status: Active\n") || !strings.Contains(text, "**Status:** Active\n") {
		t.Fatalf("status mirror not updated:\n%s", text)
	}
	if strings.Contains(text, "status: Draft") {
		t.Fatalf("stale frontmatter status retained:\n%s", text)
	}
}

func TestApplyFieldEditsWithoutFrontmatterIsSafe(t *testing.T) {
	body := "# Rule: X\n\n**Status:** Draft\n\n## Open Questions\n\nNone.\n"
	got, err := ApplyFieldEdits([]byte(body), []FieldEdit{{Name: "Status", Value: "Active"}})
	if err != nil {
		t.Fatalf("ApplyFieldEdits: %v", err)
	}
	if !strings.Contains(string(got), "**Status:** Active") {
		t.Fatalf("body status not updated:\n%s", got)
	}
}

// A field missing from an older artifact is inserted at its canonical position,
// so the ordering rule stays satisfied after the edit.
func TestApplyFieldEditsInsertsMissingFieldInOrder(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		field  string
		before string
		after  string
	}{
		{
			name:   "after its predecessor",
			body:   "# Rule: X\n\n**Status:** Draft\n**Date:** 2026-09-03\n**Superseded By:** —\n\n## Open Questions\n\nNone.\n",
			field:  "Owner",
			before: "**Date:** 2026-09-03",
			after:  "**Superseded By:** —",
		},
		{
			name:   "before its successor when no predecessor exists",
			body:   "# Rule: X\n\n**Owner:** alex\n\n## Open Questions\n\nNone.\n",
			field:  "Status",
			before: "# Rule: X",
			after:  "**Owner:** alex",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ApplyFieldEdits([]byte(tc.body), []FieldEdit{{Name: tc.field, Value: "v"}})
			if err != nil {
				t.Fatalf("ApplyFieldEdits: %v", err)
			}
			text := string(got)
			inserted := strings.Index(text, "**"+tc.field+":** v")
			if inserted < 0 {
				t.Fatalf("field not inserted:\n%s", text)
			}
			if before := strings.Index(text, tc.before); before >= inserted {
				t.Fatalf("field inserted before %q:\n%s", tc.before, text)
			}
			if after := strings.Index(text, tc.after); after >= 0 && after <= inserted {
				t.Fatalf("field inserted after %q:\n%s", tc.after, text)
			}
		})
	}
}

// A document with a title but no metadata field at all still gets its first
// field placed just under the title.
func TestApplyFieldEditsInsertsUnderTitleWhenNoFieldExists(t *testing.T) {
	got, err := ApplyFieldEdits([]byte("# Rule: X\n\n## Open Questions\n\nNone.\n"),
		[]FieldEdit{{Name: "Status", Value: "Draft"}})
	if err != nil {
		t.Fatalf("ApplyFieldEdits: %v", err)
	}
	lines := strings.Split(string(got), "\n")
	if lines[0] != "# Rule: X" || lines[2] != "**Status:** Draft" {
		t.Fatalf("field not placed under the title:\n%s", got)
	}
}

func TestApplyFieldEditsRejects(t *testing.T) {
	if _, err := ApplyFieldEdits([]byte(editableRule), []FieldEdit{{Name: "Nonsense", Value: "v"}}); err == nil {
		t.Fatal("ApplyFieldEdits should reject a non-Rule field")
	}
	// A document with no title and no fields has no anchor to insert against.
	if _, err := ApplyFieldEdits([]byte("just prose\n"), []FieldEdit{{Name: "Status", Value: "Draft"}}); err == nil {
		t.Fatal("ApplyFieldEdits should refuse a document with no `# Rule:` title")
	}
}

func TestApplyFieldEditsNoEditsIsIdentity(t *testing.T) {
	got, err := ApplyFieldEdits([]byte(editableRule), nil)
	if err != nil {
		t.Fatalf("ApplyFieldEdits: %v", err)
	}
	if string(got) != editableRule {
		t.Fatal("no-op edit changed bytes")
	}
}

// The edited artifact must still parse into the same shape, or an update would
// silently produce something the linter reads differently.
func TestApplyFieldEditsRoundTripsThroughParse(t *testing.T) {
	root := t.TempDir()
	path := writeDetail(t, root, "x", editableRule)
	updated, err := ApplyFieldEdits(mustRead(t, path), []FieldEdit{
		{Name: "Scope", Value: "path:**/*.go, product:sneat"},
		{Name: "Sources", Value: "lesson:a"},
	})
	if err != nil {
		t.Fatalf("ApplyFieldEdits: %v", err)
	}
	if err := WriteFileAtomic(path, updated); err != nil {
		t.Fatal(err)
	}
	r, err := ParseDetail(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(r.ScopesRaw) != 2 || r.ScopesRaw[0] != "path:**/*.go" {
		t.Fatalf("scopes = %v", r.ScopesRaw)
	}
	if len(r.SourcesRaw) != 1 || r.SourcesRaw[0] != "lesson:a" {
		t.Fatalf("sources = %v", r.SourcesRaw)
	}
}
