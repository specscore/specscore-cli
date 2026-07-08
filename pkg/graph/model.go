package graph

import (
	"path/filepath"
	"sort"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// Concept is one ModelSpec concept declared in a module's HCL sources: an
// entity, component, enum, collection, or recordset (decision 0006; ModelSpec
// core-model; decision 0011 addressable concepts).
type Concept struct {
	Name       string
	Kind       string // "entity", "component", "enum", "collection", or "recordset"
	File       string // absolute path to the .hcl file
	Line       int
	EnumValues []string // populated for enum concepts only
}

// conceptScope maps a concept kind to its name scope (decision 0011 / ModelSpec
// decision 0015): entity, component, and enum share one flat "trio" scope;
// collections and recordsets each have their own scope. Concept-name uniqueness
// and reference resolution are per scope.
func conceptScope(kind string) string {
	switch kind {
	case "entity", "component", "enum":
		return "trio"
	default:
		return kind // "collection" | "recordset"
	}
}

// conceptLookup is the outcome of resolving a concept name (optionally
// constrained to a kind) within a module.
type conceptLookup struct {
	found      bool   // an exact match (kind honored, or trio hit for the kind-free form)
	exists     bool   // the name is declared under some other kind (kind mismatch)
	actualKind string // the kind the name actually has, when exists is true
}

// lookupConcept resolves name within the module. The kind-free form (kind == "")
// resolves only in the trio scope (decision 0011); the kind-explicit form
// requires an exact kind match and otherwise reports a kind mismatch when the
// name is declared under a different kind.
func (m *ModelModule) lookupConcept(kind, name string) conceptLookup {
	if kind == "" {
		for _, c := range m.Concepts {
			if c.Name == name && conceptScope(c.Kind) == "trio" {
				return conceptLookup{found: true}
			}
		}
		return conceptLookup{}
	}
	var res conceptLookup
	for _, c := range m.Concepts {
		if c.Name != name {
			continue
		}
		if c.Kind == kind {
			return conceptLookup{found: true}
		}
		res = conceptLookup{exists: true, actualKind: c.Kind}
	}
	return res
}

// ModelRef is one module-qualified or bare reference inside HCL sources — a
// property/field type reference (entity/component/enum) or an entity `use`
// entry (decision 0014). Target is bare (<Name>) for same-module references or
// qualified (<module>.<Name>) for cross-module references.
type ModelRef struct {
	Target string
	Attr   string // "entity", "component", "enum", or "use"
	File   string
	Line   int
}

// ModelDiag is a located diagnostic produced while parsing HCL sources.
type ModelDiag struct {
	File    string
	Line    int
	Message string
}

// ModelModule is the parsed ModelSpec module formed by all *.hcl files under a
// graph module's models/ directory (decision 0006: one models/ dir = one
// ModelSpec module whose short name is the graph module id).
type ModelModule struct {
	ID          string
	Dir         string
	Concepts    []*Concept
	Refs        []*ModelRef
	ParseErrors []ModelDiag
}

// HasConcept reports whether the module declares a concept with the given name
// in any kind (used for bare same-module HCL references, which are
// attribute-typed and therefore kind-unambiguous at the source level).
func (m *ModelModule) HasConcept(name string) bool {
	for _, c := range m.Concepts {
		if c.Name == name {
			return true
		}
	}
	return false
}

// LoadModelModule parses every *.hcl file under dir into one ModelModule.
func LoadModelModule(dir, id string) (*ModelModule, error) {
	m := &ModelModule{ID: id, Dir: dir}
	entries, err := readDirFn(dir)
	if err != nil {
		return nil, err
	}
	var files []string
	for _, e := range entries {
		if e.IsDir() || filepath.Ext(e.Name()) != ".hcl" {
			continue
		}
		files = append(files, filepath.Join(dir, e.Name()))
	}
	sort.Strings(files)
	for _, path := range files {
		if err := m.parseFile(path); err != nil {
			return nil, err
		}
	}
	return m, nil
}

// parseFile parses one HCL file and appends its concepts, references, and any
// syntax diagnostics.
func (m *ModelModule) parseFile(path string) error {
	src, err := readFileFn(path)
	if err != nil {
		return err
	}
	file, diags := hclsyntax.ParseConfig(src, path, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		for _, d := range diags {
			m.ParseErrors = append(m.ParseErrors, ModelDiag{File: path, Line: diagLine(d), Message: d.Summary})
		}
		return nil
	}
	// hclsyntax.ParseConfig always yields an *hclsyntax.Body.
	body := file.Body.(*hclsyntax.Body)
	for _, blk := range body.Blocks {
		m.parseBlock(path, blk)
	}
	return nil
}

// diagLine returns a diagnostic's subject start line; hcl.Diagnostic.Subject
// is optional (*hcl.Range), so a nil subject yields 0.
func diagLine(d *hcl.Diagnostic) int {
	if d.Subject == nil {
		return 0
	}
	return d.Subject.Start.Line
}

// parseBlock handles a top-level entity/component/enum block.
func (m *ModelModule) parseBlock(path string, blk *hclsyntax.Block) {
	line := blk.DefRange().Start.Line
	name := ""
	if len(blk.Labels) > 0 {
		name = blk.Labels[0]
	}
	switch blk.Type {
	case "entity", "component":
		m.Concepts = append(m.Concepts, &Concept{Name: name, Kind: blk.Type, File: path, Line: line})
		m.collectMemberRefs(path, blk.Body)
	case "enum":
		c := &Concept{Name: name, Kind: "enum", File: path, Line: line}
		if vals, ok := stringListAttr(blk.Body, "values"); ok {
			c.EnumValues = vals
		}
		m.Concepts = append(m.Concepts, c)
	case "collection", "recordset":
		// Indexed so modelspec://<module>.collections|recordsets.<Name> refs
		// resolve. Their source/bind attributes are not reference attributes
		// (ModelSpec decision 0014), so no member refs are collected.
		m.Concepts = append(m.Concepts, &Concept{Name: name, Kind: blk.Type, File: path, Line: line})
	}
	// Other block types (projection, index, key) are tolerated and ignored.
}

// collectMemberRefs extracts references from an entity/component body: the
// `use` list plus every property/field block's entity/component/enum attribute.
func (m *ModelModule) collectMemberRefs(path string, body *hclsyntax.Body) {
	if uses, ok := stringListAttr(body, "use"); ok {
		useLine := body.Attributes["use"].SrcRange.Start.Line
		for _, u := range uses {
			m.Refs = append(m.Refs, &ModelRef{Target: u, Attr: "use", File: path, Line: useLine})
		}
	}
	for _, blk := range body.Blocks {
		if blk.Type != "property" && blk.Type != "field" {
			continue
		}
		for _, attr := range []string{"entity", "component", "enum"} {
			if v, ok := stringAttr(blk.Body, attr); ok {
				m.Refs = append(m.Refs, &ModelRef{
					Target: v,
					Attr:   attr,
					File:   path,
					Line:   blk.Body.Attributes[attr].SrcRange.Start.Line,
				})
			}
		}
	}
}

// stringAttr returns the literal string value of a named attribute, or ok=false
// when the attribute is absent or not a plain string literal.
func stringAttr(body *hclsyntax.Body, name string) (string, bool) {
	attr, ok := body.Attributes[name]
	if !ok {
		return "", false
	}
	val, diags := attr.Expr.Value(nil)
	if diags.HasErrors() || val.IsNull() || !val.Type().Equals(cty.String) {
		return "", false
	}
	return val.AsString(), true
}

// stringListAttr returns the literal string list value of a named attribute, or
// ok=false when the attribute is absent or not a list of string literals.
func stringListAttr(body *hclsyntax.Body, name string) ([]string, bool) {
	attr, ok := body.Attributes[name]
	if !ok {
		return nil, false
	}
	val, diags := attr.Expr.Value(nil)
	if diags.HasErrors() || val.IsNull() || !val.CanIterateElements() {
		return nil, false
	}
	var out []string
	for it := val.ElementIterator(); it.Next(); {
		_, ev := it.Element()
		if ev.IsNull() || !ev.Type().Equals(cty.String) {
			return nil, false
		}
		out = append(out, ev.AsString())
	}
	return out, true
}
