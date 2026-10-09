package graph

import (
	"fmt"
	"path/filepath"
	"slices"
	"sort"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
)

// Concept is one ModelSpec concept declared in a module's HCL sources: a record
// type, component, or enum (decision 0006; ModelSpec core-model; decision 0011
// addressable concepts). A record type is kept as Kind "entity" whichever word
// declared it (`record` or the earlier `entity`), so every finding reads the same
// for a model in either spelling.
type Concept struct {
	Name       string
	Kind       string // "entity" (a record type), "component", or "enum"
	File       string // absolute path to the .hcl file
	Line       int
	EnumValues []string // populated for enum concepts only
	Properties []string // property/field member names (entity/component concepts; decision 0013 actor-is.model-role)
}

// trioScope names the one name scope the concept kinds share (decision 0011 /
// ModelSpec decision 0015): record types, components, and enums live in one flat
// scope. Concept-name uniqueness and reference resolution are per scope.
const trioScope = "trio"

// conceptLookup is the outcome of resolving a concept name (optionally
// constrained to a kind) within a module.
type conceptLookup struct {
	found      bool     // an exact match (kind honored, or trio hit for the kind-free form)
	exists     bool     // the name is declared under some other kind (kind mismatch)
	actualKind string   // the kind the name actually has, when exists is true
	concept    *Concept // the matched concept, when found is true
}

// lookupConcept resolves name within the module. The kind-free form (kind == "")
// resolves in the trio scope (decision 0011); the kind-explicit form requires an
// exact kind match and otherwise reports a kind mismatch when the name is
// declared under a different kind.
func (m *ModelModule) lookupConcept(kind, name string) conceptLookup {
	if kind == "" {
		for _, c := range m.Concepts {
			if c.Name == name {
				return conceptLookup{found: true, concept: c}
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
			return conceptLookup{found: true, concept: c}
		}
		res = conceptLookup{exists: true, actualKind: c.Kind}
	}
	return res
}

// ModelRef is one module-qualified or bare reference inside HCL sources — a
// property/field type reference (record/entity, component, or enum) or a record
// type `use` entry (decision 0014). A `record =` reference and the earlier
// `entity =` reference are both Attr "entity". Target is bare (<Name>) for same-module references or
// qualified (<module>.<Name>) for cross-module references. Owner is the name
// of the concept whose body declares the reference — navigation uses it to
// derive graph edges from model structure (association-object visibility).
type ModelRef struct {
	Target string
	Attr   string // "entity" (a record type), "component", "enum", or "use"
	Owner  string // declaring concept name
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
	// Refused lists what ModelSpec forbids in a file that parses: a removed
	// construct, a reserved word, or a member with both record and entity.
	Refused []ModelDiag
	// Deprecated lists, one per file, the files that use the earlier ModelSpec
	// spelling (entity, property, entity =). Such a file is read in full; the
	// linter reports it as advisory.
	Deprecated []DeprecatedSpelling
}

// DeprecatedSpelling records that one HCL file uses the earlier ModelSpec
// spelling. Line is where the first earlier word appears; Words lists each
// earlier spelling found (entity, property, "entity ="), in order of first
// appearance.
type DeprecatedSpelling struct {
	File  string
	Line  int
	Words []string
}

// noteDeprecated records the earlier spelling word found at line of path. Files
// are parsed one after another, so a file's entry is the last one in the list.
func (m *ModelModule) noteDeprecated(path string, line int, word string) {
	if n := len(m.Deprecated); n == 0 || m.Deprecated[n-1].File != path {
		m.Deprecated = append(m.Deprecated, DeprecatedSpelling{File: path, Line: line})
	}
	d := &m.Deprecated[len(m.Deprecated)-1]
	if !slices.Contains(d.Words, word) {
		d.Words = append(d.Words, word)
	}
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

// refusedWords maps the HCL block words a ModelSpec model may not use (ModelSpec
// decision 0019) to their status: collection, recordset and column are removed
// constructs; projection, index and migration are reserved words with no
// content yet.
var refusedWords = map[string]string{
	"collection": "removed",
	"recordset":  "removed",
	"column":     "removed",
	"projection": "reserved",
	"index":      "reserved",
	"migration":  "reserved",
}

// refuseWord adds a parse error when blockType is a removed construct or a
// reserved word, and reports whether it did. The block is not read.
func (m *ModelModule) refuseWord(path string, line int, blockType string) bool {
	status, ok := refusedWords[blockType]
	if !ok {
		return false
	}
	msg := fmt.Sprintf("%s is a reserved word in ModelSpec with no content yet (decision 0019), so a model cannot use it", blockType)
	if status == "removed" {
		msg = fmt.Sprintf("%s blocks were removed from ModelSpec (decision 0019); the shape of a query result or a view is a record type with no key", blockType)
	}
	m.Refused = append(m.Refused, ModelDiag{File: path, Line: line, Message: msg})
	return true
}

// refuseNested refuses a removed construct or reserved word used as a block
// inside body — the body of a member or of an enum, which hold no blocks.
func (m *ModelModule) refuseNested(path string, body *hclsyntax.Body) {
	for _, blk := range body.Blocks {
		m.refuseWord(path, blk.DefRange().Start.Line, blk.Type)
	}
}

// parseBlock handles a top-level record/entity/component/enum block. `record`
// and the earlier `entity` both declare a record type, kept as Kind "entity".
func (m *ModelModule) parseBlock(path string, blk *hclsyntax.Block) {
	line := blk.DefRange().Start.Line
	name := ""
	if len(blk.Labels) > 0 {
		name = blk.Labels[0]
	}
	switch blk.Type {
	case "entity", "record", "component":
		kind := blk.Type
		if blk.Type == "entity" {
			m.noteDeprecated(path, line, "entity")
		}
		if blk.Type == "record" {
			kind = "entity"
		}
		c := &Concept{Name: name, Kind: kind, File: path, Line: line}
		c.Properties = m.parseBody(path, name, kind == "entity", blk.Body)
		m.Concepts = append(m.Concepts, c)
	case "enum":
		c := &Concept{Name: name, Kind: "enum", File: path, Line: line}
		if vals, ok := stringListAttr(blk.Body, "values"); ok {
			c.EnumValues = vals
		}
		m.refuseNested(path, blk.Body)
		m.Concepts = append(m.Concepts, c)
	default:
		// Removed constructs and reserved words are refused; other block types
		// (key) are tolerated and ignored.
		m.refuseWord(path, line, blk.Type)
	}
}

// parseBody reads a record type or component body: it records the `use` list,
// refuses removed and reserved block words, and for every property/field member
// returns its name and collects its reference attributes. record is true for a
// record type, where the earlier `property` word is a deprecated spelling of
// `field`; it returns the member names, which a PolicySpec actor-is.model-role
// clause addresses (decision 0013).
func (m *ModelModule) parseBody(path, owner string, record bool, body *hclsyntax.Body) []string {
	if uses, ok := stringListAttr(body, "use"); ok {
		useLine := body.Attributes["use"].SrcRange.Start.Line
		for _, u := range uses {
			m.Refs = append(m.Refs, &ModelRef{Target: u, Attr: "use", Owner: owner, File: path, Line: useLine})
		}
	}
	var members []string
	for _, blk := range body.Blocks {
		if m.refuseWord(path, blk.DefRange().Start.Line, blk.Type) {
			continue
		}
		if blk.Type != "property" && blk.Type != "field" {
			continue
		}
		if blk.Type == "property" && record {
			m.noteDeprecated(path, blk.DefRange().Start.Line, "property")
		}
		if len(blk.Labels) > 0 {
			members = append(members, blk.Labels[0])
		}
		m.refuseNested(path, blk.Body)
		m.collectMemberRefs(path, owner, blk.Body)
	}
	return members
}

// collectMemberRefs extracts the references of one property/field member: its
// record/entity, component, and enum attributes. `record =` and the earlier
// `entity =` are the same reference; a member that carries both is refused.
func (m *ModelModule) collectMemberRefs(path, owner string, body *hclsyntax.Body) {
	if a, ok := body.Attributes["entity"]; ok {
		line := a.SrcRange.Start.Line
		m.noteDeprecated(path, line, "entity =")
		if _, both := body.Attributes["record"]; both {
			m.Refused = append(m.Refused, ModelDiag{File: path, Line: line,
				Message: "a member has both record and entity; entity is the earlier spelling of record, and a member refers to one record type"})
		}
	}
	for _, attr := range []string{"entity", "record", "component", "enum"} {
		if v, ok := stringAttr(body, attr); ok {
			kind := attr
			if attr == "record" {
				kind = "entity"
			}
			m.Refs = append(m.Refs, &ModelRef{
				Target: v,
				Attr:   kind,
				Owner:  owner,
				File:   path,
				Line:   body.Attributes[attr].SrcRange.Start.Line,
			})
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
