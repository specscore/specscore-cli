package graph

import (
	"reflect"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/lint"
)

// spellings holds one model written two ways: the earlier spelling (entity,
// property, entity =) and the current one (record, field, record =).
type spellings struct{ earlier, current string }

const spellingTemplate = `
component "Auditable" {
  field "createdAt" {
    type = "datetime"
  }
}

enum "Status" {
  values = ["open", "closed"]
}

@BLOCK "Team" {
  key = ["id"]

  @MEMBER "id" {
    type = "uuid"
  }
}

@BLOCK "Booking" {
  key = ["id"]
  use = ["Auditable"]

  @MEMBER "id" {
    type = "uuid"
  }

  @MEMBER "team" {
    @REF = "Team"
  }

  @MEMBER "outsider" {
    @REF = "other.Thing"
  }

  @MEMBER "missing" {
    @REF = "Nowhere"
  }

  @MEMBER "status" {
    enum = "Status"
  }
}
`

func spelled(template string) spellings {
	earlier := strings.NewReplacer("@BLOCK", "entity", "@MEMBER", "property", "@REF", "entity").Replace(template)
	current := strings.NewReplacer("@BLOCK", "record", "@MEMBER", "field", "@REF", "record").Replace(template)
	return spellings{earlier: earlier, current: current}
}

func modelRepo(t *testing.T, hcl string, extra map[string]string) string {
	t.Helper()
	files := map[string]string{
		"spec/graph/modules/m/README.md":    fmModule("m", "[]"),
		"spec/graph/modules/m/models/m.hcl": hcl,
	}
	for k, v := range extra {
		files[k] = v
	}
	return repoWith(t, files)
}

// withoutAdvisory drops the earlier-spelling notice, the one finding a model in
// the earlier spelling has that its current-spelling twin does not.
func withoutAdvisory(vs []lint.Violation) []lint.Violation {
	var out []lint.Violation
	for _, v := range vs {
		if v.Rule != "graph-model-deprecated-spelling" {
			out = append(out, v)
		}
	}
	return out
}

func TestLoadModelModule_SpellingsAreTheSameModel(t *testing.T) {
	sp := spelled(spellingTemplate)
	early := loadModelSrc(t, sp.earlier)
	cur := loadModelSrc(t, sp.current)
	if len(early.ParseErrors)+len(cur.ParseErrors) != 0 {
		t.Fatalf("unexpected parse errors: %+v %+v", early.ParseErrors, cur.ParseErrors)
	}
	strip := func(m *ModelModule) ([]Concept, []ModelRef) {
		var cs []Concept
		for _, c := range m.Concepts {
			cc := *c
			cc.File = ""
			cs = append(cs, cc)
		}
		var rs []ModelRef
		for _, r := range m.Refs {
			rr := *r
			rr.File = ""
			rs = append(rs, rr)
		}
		return cs, rs
	}
	ec, er := strip(early)
	cc, cr := strip(cur)
	if !reflect.DeepEqual(ec, cc) {
		t.Fatalf("concepts differ:\n%+v\n%+v", ec, cc)
	}
	if !reflect.DeepEqual(er, cr) {
		t.Fatalf("refs differ:\n%+v\n%+v", er, cr)
	}
	for _, c := range cc {
		if c.Name == "Booking" && (c.Kind != "entity" || !reflect.DeepEqual(c.Properties, []string{"id", "team", "outsider", "missing", "status"})) {
			t.Fatalf("record type read wrongly: %+v", c)
		}
	}
	if len(cur.Deprecated) != 0 {
		t.Fatalf("the current spelling must not be reported: %+v", cur.Deprecated)
	}
	if len(early.Deprecated) != 1 {
		t.Fatalf("expected one entry for the file: %+v", early.Deprecated)
	}
	d := early.Deprecated[0]
	if d.Line != 12 || !reflect.DeepEqual(d.Words, []string{"entity", "property", "entity ="}) {
		t.Fatalf("deprecated entry: %+v", d)
	}
}

func TestLoadModelModule_MixedSpellings(t *testing.T) {
	m := loadModelSrc(t, "record \"A\" {\n  property \"x\" {\n    type = \"string\"\n  }\n}\n\nrecord \"B\" {\n  field \"a\" {\n    entity = \"A\"\n  }\n}\n")
	if len(m.ParseErrors) != 0 || len(m.Concepts) != 2 || len(m.Refs) != 1 {
		t.Fatalf("mixed file misread: %+v %+v %+v", m.ParseErrors, m.Concepts, m.Refs)
	}
	if len(m.Deprecated) != 1 || !reflect.DeepEqual(m.Deprecated[0].Words, []string{"property", "entity ="}) || m.Deprecated[0].Line != 2 {
		t.Fatalf("deprecated entry: %+v", m.Deprecated)
	}
	if m.Refs[0].Attr != "entity" || m.Refs[0].Target != "A" {
		t.Fatalf("ref: %+v", m.Refs[0])
	}
}

func TestLoadModelModule_OneNoticePerFile(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir+"/a.hcl", "entity \"A\" {}\nentity \"B\" {}\n")
	writeFile(t, dir+"/b.hcl", "record \"C\" {}\n")
	writeFile(t, dir+"/c.hcl", "record \"D\" {\n  property \"p\" {}\n}\n")
	m, err := LoadModelModule(dir, "m")
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Deprecated) != 2 || !strings.HasSuffix(m.Deprecated[0].File, "a.hcl") || !strings.HasSuffix(m.Deprecated[1].File, "c.hcl") {
		t.Fatalf("expected one entry each for a.hcl and c.hcl: %+v", m.Deprecated)
	}
	if !reflect.DeepEqual(m.Deprecated[0].Words, []string{"entity"}) {
		t.Fatalf("a repeated word is listed once: %+v", m.Deprecated[0])
	}
}

// A property block in a component was read before this change and still is; it
// is not an earlier spelling the standard defines, so it is not reported.
func TestLoadModelModule_PropertyInComponentIsReadSilently(t *testing.T) {
	m := loadModelSrc(t, "component \"C\" {\n  property \"p\" {\n    entity = \"X\"\n  }\n}\n")
	if len(m.ParseErrors) != 0 || len(m.Concepts) != 1 || !reflect.DeepEqual(m.Concepts[0].Properties, []string{"p"}) {
		t.Fatalf("misread: %+v %+v", m.ParseErrors, m.Concepts)
	}
	if len(m.Deprecated) != 1 || !reflect.DeepEqual(m.Deprecated[0].Words, []string{"entity ="}) {
		t.Fatalf("only the entity = reference is reported: %+v", m.Deprecated)
	}
}

func TestLoadModelModule_NonLiteralReferenceStillNoticed(t *testing.T) {
	m := loadModelSrc(t, "record \"A\" {\n  field \"x\" {\n    entity = somevar\n  }\n}\n")
	if len(m.Refs) != 0 || len(m.Deprecated) != 1 || m.Deprecated[0].Line != 3 {
		t.Fatalf("refs %+v, deprecated %+v", m.Refs, m.Deprecated)
	}
}

func TestLoadModelModule_MemberWithBothReferenceWordsIsRefused(t *testing.T) {
	m := loadModelSrc(t, "record \"A\" {}\nrecord \"B\" {\n  field \"a\" {\n    entity = \"A\"\n    record = \"A\"\n  }\n}\n")
	if len(m.ParseErrors) != 1 || m.ParseErrors[0].Line != 4 || !strings.Contains(m.ParseErrors[0].Message, "both record and entity") {
		t.Fatalf("expected the member to be refused: %+v", m.ParseErrors)
	}
}

func TestLoadModelModule_RemovedAndReservedWordsAreRefused(t *testing.T) {
	removed := []string{"collection", "recordset", "column"}
	reserved := []string{"projection", "index", "migration"}
	for _, spelling := range []struct{ name, block string }{{"current", "record"}, {"earlier", "entity"}} {
		for _, w := range append(append([]string{}, removed...), reserved...) {
			// At the top level.
			top := loadModelSrc(t, spelling.block+" \"A\" {}\n"+w+" \"X\" {\n  source = \"A\"\n}\n")
			if len(top.ParseErrors) != 1 || top.ParseErrors[0].Line != 2 || !strings.Contains(top.ParseErrors[0].Message, w) {
				t.Errorf("%s top-level %s: %+v", spelling.name, w, top.ParseErrors)
			}
			if len(top.Concepts) != 1 {
				t.Errorf("%s top-level %s must not become a concept: %+v", spelling.name, w, top.Concepts)
			}
			// Inside a record type and inside a component.
			for _, outer := range []string{spelling.block, "component"} {
				in := loadModelSrc(t, outer+" \"A\" {\n  "+w+" \"X\" {}\n}\n")
				if len(in.ParseErrors) != 1 || in.ParseErrors[0].Line != 2 || !strings.Contains(in.ParseErrors[0].Message, w) {
					t.Errorf("%s %s containing %s: %+v", spelling.name, outer, w, in.ParseErrors)
				}
			}
		}
	}
	for _, w := range removed {
		m := loadModelSrc(t, w+" \"X\" {}\n")
		if !strings.Contains(m.ParseErrors[0].Message, "removed") {
			t.Errorf("%s: %q should say removed", w, m.ParseErrors[0].Message)
		}
	}
	for _, w := range reserved {
		m := loadModelSrc(t, w+" \"X\" {}\n")
		if !strings.Contains(m.ParseErrors[0].Message, "reserved") {
			t.Errorf("%s: %q should say reserved", w, m.ParseErrors[0].Message)
		}
	}
}

func TestLoadModelModule_OtherBlocksStayTolerated(t *testing.T) {
	m := loadModelSrc(t, "record \"A\" {\n  note \"x\" {}\n}\nkey \"k\" {}\n")
	if len(m.ParseErrors) != 0 || len(m.Concepts) != 1 {
		t.Fatalf("unknown blocks other than the refused words are ignored as before: %+v", m)
	}
}

func TestNoticeLint_EarlierSpellingIsAnAdvisoryNoticeOncePerFile(t *testing.T) {
	sp := spelled(spellingTemplate)
	root := modelRepo(t, sp.earlier, map[string]string{
		"spec/graph/modules/m/models/second.hcl": "entity \"Extra\" {}\nentity \"More\" {}\n",
	})
	res := lintRepo(t, root, func(o *LintOptions) { o.Severity = "info" })
	var got []lint.Violation
	for _, v := range res.Violations {
		if v.Rule == "graph-model-deprecated-spelling" {
			got = append(got, v)
		}
	}
	if len(got) != 2 {
		t.Fatalf("expected one notice per file, got %+v", got)
	}
	for _, v := range got {
		if v.Severity != "info" || !strings.Contains(v.Message, "modelspec rewrite --write "+v.File) {
			t.Errorf("notice must be info and name the rewrite command for its file: %+v", v)
		}
	}
	if got[0].File != "spec/graph/modules/m/models/m.hcl" || got[0].Line != 12 {
		t.Errorf("notice must point at the first earlier word: %+v", got[0])
	}
	// At the default severity the notice is not shown, so no run changes.
	def := lintRepo(t, root, func(o *LintOptions) { o.Severity = "error" })
	if hasRule(def.Violations, "graph-model-deprecated-spelling") {
		t.Fatalf("default run must not show the notice: %+v", def.Violations)
	}
	// It can be selected, and ignored, like any rule.
	only := lintRepo(t, root, func(o *LintOptions) { o.Severity = "info"; o.Rules = []string{"graph-model-deprecated-spelling"} })
	if len(only.Violations) != 2 {
		t.Fatalf("--rules selects the notice: %+v", only.Violations)
	}
	ign := lintRepo(t, root, func(o *LintOptions) { o.Severity = "info"; o.Ignore = []string{"graph-model-deprecated-spelling"} })
	if hasRule(ign.Violations, "graph-model-deprecated-spelling") {
		t.Fatalf("--ignore drops the notice: %+v", ign.Violations)
	}
	if err := ValidateRuleNames([]string{"graph-model-deprecated-spelling"}); err != nil {
		t.Fatal(err)
	}
}

func TestNoticeLint_CurrentSpellingHasNoNotice(t *testing.T) {
	sp := spelled(spellingTemplate)
	res := lintRepo(t, modelRepo(t, sp.current, nil), func(o *LintOptions) { o.Severity = "info" })
	if hasRule(res.Violations, "graph-model-deprecated-spelling") {
		t.Fatalf("unexpected notice: %+v", res.Violations)
	}
}

// TestLint_SpellingsGiveTheSameFindings runs the linter on a model that has
// findings of several kinds (an unresolved reference, an unknown module, a
// reserved name, a duplicate) in both spellings and compares everything but the
// notice.
func TestLint_SpellingsGiveTheSameFindings(t *testing.T) {
	template := spellingTemplate + "\n@BLOCK \"Team\" {}\n@BLOCK \"records\" {}\n"
	sp := spelled(template)
	extra := map[string]string{
		"spec/graph/modules/m/entities/e.md": fmArt("entity", "e", "model: modelspec:///m.records.Booking"),
		"spec/graph/modules/m/entities/f.md": fmArt("entity", "f", "model: modelspec:///m.entities.Booking"),
	}
	opts := func(o *LintOptions) { o.Severity = "info" }
	early := lintRepo(t, modelRepo(t, sp.earlier, extra), opts)
	cur := lintRepo(t, modelRepo(t, sp.current, extra), opts)
	if len(cur.Violations) == 0 {
		t.Fatal("the model should have findings to compare")
	}
	if !reflect.DeepEqual(withoutAdvisory(early.Violations), cur.Violations) {
		t.Fatalf("findings differ:\nearlier: %+v\ncurrent: %+v", withoutAdvisory(early.Violations), cur.Violations)
	}
	for _, rule := range []string{"graph-model-ref-resolves", "graph-model-reserved-name", "graph-model-duplicate-concept"} {
		if !hasRule(cur.Violations, rule) {
			t.Errorf("fixture should exercise %s: %+v", rule, cur.Violations)
		}
	}
	if len(early.Violations) != len(cur.Violations)+1 {
		t.Fatalf("the earlier spelling adds exactly the notice: %d vs %d", len(early.Violations), len(cur.Violations))
	}
}

func TestLint_RefusedWordsAreFindings(t *testing.T) {
	res := lintRepo(t, modelRepo(t, "record \"A\" {}\ncollection \"c\" {\n  source = \"A\"\n}\nentity \"B\" {\n  index \"i\" {}\n}\n", nil))
	n := 0
	for _, v := range res.Violations {
		if v.Rule == "graph-model-ref-resolves" && v.Severity == "error" &&
			(strings.Contains(v.Message, "collection") || strings.Contains(v.Message, "index")) {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("expected a finding for each refused word: %+v", res.Violations)
	}
}
