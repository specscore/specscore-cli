package graph

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func parseStr(t *testing.T, content, module, coll string) *Artifact {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "a.md")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	a, err := ParseArtifact(p, module, coll)
	if err != nil {
		t.Fatalf("ParseArtifact: %v", err)
	}
	return a
}

func TestParseArtifact_AllFields(t *testing.T) {
	content := `---
kind: command
id: create-booking
name: CreateBooking
status: draft
summary: A summary.
model: modelspec://m.X
from: a.b
to: c.d
cardinality: one-to-many
subject: m.booking
dependsOn: [a, b]
actors:
  - identity.user
participants:
  - catalog.asset
possibleEvents:
  - m.booking-created
sources:
  - timer
lifecycle:
  states: [a, b]
metadata:
  role: scalar-value
  ref: identity.TeamRole
inputs:
  - name: p
    ref: m.a
  - name: q
    model: modelspec://m.B
    extra: junk
---

body
`
	a := parseStr(t, content, "m", "commands")
	if !a.HasFrontmatter {
		t.Fatal("expected frontmatter")
	}
	if a.Kind != "command" || a.ID != "create-booking" || a.Name != "CreateBooking" || a.Status != "draft" {
		t.Fatalf("scalars wrong: %+v", a)
	}
	if a.Model != "modelspec://m.X" || a.From != "a.b" || a.To != "c.d" || a.Cardinality != "one-to-many" || a.Subject != "m.booking" {
		t.Fatal("ref scalars wrong")
	}
	if len(a.DependsOn) != 2 || len(a.Actors) != 1 || len(a.Participants) != 1 || len(a.PossibleEvents) != 1 || len(a.Sources) != 1 {
		t.Fatal("lists wrong")
	}
	if !a.LifecycleStatesPresent || len(a.LifecycleStates) != 2 {
		t.Fatal("lifecycle wrong")
	}
	if len(a.Metadata) != 2 {
		t.Fatalf("metadata wrong: %+v", a.Metadata)
	}
	if len(a.Inputs) != 2 || a.Inputs[0].Name != "p" || !a.Inputs[0].HasRef || !a.Inputs[1].HasModel || len(a.Inputs[1].ExtraKeys) != 1 {
		t.Fatalf("inputs wrong: %+v", a.Inputs)
	}
	if a.QualifiedID() != "m.create-booking" {
		t.Fatalf("qualified id: %q", a.QualifiedID())
	}
	if a.KeyLine("kind") == 0 {
		t.Fatal("expected a key line for kind")
	}
	if !a.HasKey("actors") {
		t.Fatal("HasKey actors")
	}
}

func TestParseArtifact_FMErrors(t *testing.T) {
	cases := map[string]string{
		"empty-file":     "",
		"no-frontmatter": "# just a title\n",
		"unterminated":   "---\nkind: entity\n",
		"invalid-yaml":   "---\nkind: [unclosed\n---\n",
		"empty-block":    "---\n---\n",
		"non-mapping":    "---\n- a\n- b\n---\n",
	}
	for name, content := range cases {
		t.Run(name, func(t *testing.T) {
			a := parseStr(t, content, "m", "entities")
			if a.HasFrontmatter || a.FMError == "" {
				t.Fatalf("expected FMError, got HasFrontmatter=%v FMError=%q", a.HasFrontmatter, a.FMError)
			}
		})
	}
}

func TestParseArtifact_ShapeEdges(t *testing.T) {
	// non-scalar mapping key is skipped; lifecycle/metadata/inputs non-standard shapes.
	content := `---
kind: entity
id: x
? [complex, key]
: value
lifecycle: not-a-map
metadata: not-a-map
inputs: not-a-list
---
`
	a := parseStr(t, content, "m", "entities")
	if a.LifecycleStatesPresent {
		t.Fatal("lifecycle non-map should not set states")
	}
	if len(a.Metadata) != 1 || a.Metadata[0].Scalar {
		t.Fatalf("metadata non-map should be one non-scalar entry: %+v", a.Metadata)
	}
	if !a.inputsMalformed {
		t.Fatal("inputs non-list should be malformed")
	}
}

func TestParseArtifact_InputsNonMappingItem(t *testing.T) {
	content := `---
kind: command
id: c
inputs:
  - just-a-string
sources: notaseq
---
`
	a := parseStr(t, content, "m", "commands")
	if !a.inputsMalformed {
		t.Fatal("non-mapping input item should mark malformed")
	}
	if a.Sources != nil {
		t.Fatalf("non-seq sources should be nil, got %v", a.Sources)
	}
}

func TestParseArtifact_MetadataNonScalarValue(t *testing.T) {
	content := `---
kind: relationship
id: r
metadata:
  ok: scalar
  bad:
    - nested-list
---
`
	a := parseStr(t, content, "m", "relationships")
	var sawScalar, sawNonScalar bool
	for _, e := range a.Metadata {
		if e.Scalar {
			sawScalar = true
		} else {
			sawNonScalar = true
		}
	}
	if !sawScalar || !sawNonScalar {
		t.Fatalf("expected mixed metadata scalars: %+v", a.Metadata)
	}
}

func TestParseArtifact_ReadError(t *testing.T) {
	orig := readFileFn
	readFileFn = func(string) ([]byte, error) { return nil, errors.New("boom") }
	defer func() { readFileFn = orig }()
	if _, err := ParseArtifact("whatever.md", "m", ""); err == nil {
		t.Fatal("expected read error")
	}
}

func TestArtifact_ZeroValueAccessors(t *testing.T) {
	var a Artifact
	if a.KeyLine("x") != 0 {
		t.Fatal("zero KeyLine")
	}
	if a.HasKey("x") {
		t.Fatal("zero HasKey")
	}
	if a.QualifiedID() != "" {
		t.Fatal("zero QualifiedID")
	}
	a2 := Artifact{Module: "m", ID: ""}
	if a2.QualifiedID() != "" {
		t.Fatal("empty id qualified")
	}
}

func TestScalarHelpers(t *testing.T) {
	// scalarList on a mapping node returns nil (non-seq); scalar on seq returns "".
	content := "---\nkind: entity\nid: x\nactors: {a: b}\nname:\n  - seqname\n---\n"
	a := parseStr(t, content, "m", "entities")
	if a.Actors != nil {
		t.Fatalf("mapping actors should be nil, got %v", a.Actors)
	}
	if a.Name != "" {
		t.Fatalf("sequence name should scalar to empty, got %q", a.Name)
	}
}
