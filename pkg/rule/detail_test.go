package rule

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestParseDetailRoundTrip(t *testing.T) {
	root := t.TempDir()
	path := scaffoldInto(t, root, Options{
		Slug: "gofmt-first", Owner: "alex", Date: "2026-09-03", Status: "Active",
		Statement: "Always run gofmt on changed Go files.", Scopes: []string{"fleet", "path:**/*.go"},
		Enforcement: "Enforced", Control: "wb pre-commit hook", Sources: []string{"lesson:a", "decision:0001"},
		Why: "Unformatted Go reddens CI.", Exceptions: "none", Skills: []string{"go-hygiene"},
	})

	d, err := ParseDetail(path)
	if err != nil {
		t.Fatalf("ParseDetail: %v", err)
	}
	if !d.HasRuleTitle || d.Title != "Gofmt First" || d.Slug != "gofmt-first" {
		t.Fatalf("title = %q slug = %q", d.Title, d.Slug)
	}
	if d.Status != "Active" || d.FrontmatterStatus != "Active" {
		t.Fatalf("status body=%q frontmatter=%q", d.Status, d.FrontmatterStatus)
	}
	if want := []string{"fleet", "path:**/*.go"}; !reflect.DeepEqual(d.ScopesRaw, want) {
		t.Fatalf("scopes = %v, want %v", d.ScopesRaw, want)
	}
	if want := []string{"lesson:a", "decision:0001"}; !reflect.DeepEqual(d.SourcesRaw, want) {
		t.Fatalf("sources = %v, want %v", d.SourcesRaw, want)
	}
	if d.Enforcement != "Enforced" || d.Control != "wb pre-commit hook" || !d.HasControl() {
		t.Fatalf("enforcement=%q control=%q", d.Enforcement, d.Control)
	}
	if want := []string{"go-hygiene"}; !reflect.DeepEqual(d.SkillRefs, want) {
		t.Fatalf("skill refs = %v, want %v", d.SkillRefs, want)
	}
	if len(d.MissingSections()) != 0 || len(d.MissingExampleSubsections()) != 0 {
		t.Fatalf("sections = %v subsections = %v", d.SectionLines, d.SubsectionLines)
	}
	if !reflect.DeepEqual(d.FieldOrder, DetailFields) {
		t.Fatalf("field order = %v, want %v", d.FieldOrder, DetailFields)
	}
	if scopes, err := d.Scopes(); err != nil || len(scopes) != 2 {
		t.Fatalf("Scopes() = %v, %v", scopes, err)
	}
	if sources, err := d.Sources(); err != nil || len(sources) != 2 {
		t.Fatalf("Sources() = %v, %v", sources, err)
	}
	if !d.HasSection("Instructions") {
		t.Fatal("HasSection(Instructions) = false")
	}
}

// A hand-wrapped Statement or Why must be read whole. Reading only the first
// physical line is the exact mid-sentence truncation that made index rows
// unreadable before the paragraph-joining fixes; this kind joins from the
// outset, and the joined value is what reaches the index row.
func TestParseDetailJoinsWrappedValues(t *testing.T) {
	root := t.TempDir()
	path := writeDetail(t, root, "wrapped", `---
format: `+FormatURL+`
status: Draft
---

# Rule: Wrapped

**Status:** Draft
**Date:** 2026-09-03
**Owner:** alex
**Statement:** Never ship a mocked extension backend — ship real,
dalgo-backed routes, or ship nothing at all.
**Scope:** fleet
**Enforcement:** Stated
**Control:** —
**Sources:** —
**Why:** A mock passes review and then fails in production,
and the failure lands on a user rather than a reviewer.
**Exceptions:** none
**Supersedes:** —
**Superseded By:** —

## Instructions

Do the thing.

## Examples

### Compliant

x

### Violation

y

## Open Questions

None at this time.
`)
	d, err := ParseDetail(path)
	if err != nil {
		t.Fatalf("ParseDetail: %v", err)
	}
	wantStatement := "Never ship a mocked extension backend — ship real, dalgo-backed routes, or ship nothing at all."
	if d.Statement != wantStatement {
		t.Fatalf("statement = %q, want %q", d.Statement, wantStatement)
	}
	wantWhy := "A mock passes review and then fails in production, and the failure lands on a user rather than a reviewer."
	if d.Why != wantWhy {
		t.Fatalf("why = %q, want %q", d.Why, wantWhy)
	}
	if got := RowFromDetail(d).Statement; got != wantStatement {
		t.Fatalf("index row statement = %q, want the whole sentence", got)
	}
}

func TestParseDetailInlineControlOnEnforcementLine(t *testing.T) {
	cases := []struct {
		name           string
		enforcement    string
		control        string
		wantTier       string
		wantControl    string
		wantHasControl bool
	}{
		{name: "inline control lifted", enforcement: "Enforced (control: wb pre-push hook)", control: Sentinel,
			wantTier: "Enforced", wantControl: "wb pre-push hook", wantHasControl: true},
		{name: "explicit control wins", enforcement: "Enforced (control: inline one)", control: "explicit one",
			wantTier: "Enforced", wantControl: "explicit one", wantHasControl: true},
		{name: "no inline control", enforcement: "Stated", control: Sentinel,
			wantTier: "Stated", wantControl: Sentinel, wantHasControl: false},
		{name: "case insensitive", enforcement: "Automated (CONTROL: renovate automerge)", control: Sentinel,
			wantTier: "Automated", wantControl: "renovate automerge", wantHasControl: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			path := writeDetail(t, root, "x", `# Rule: X

**Status:** Draft
**Statement:** s
**Scope:** fleet
**Enforcement:** `+tc.enforcement+`
**Control:** `+tc.control+`

## Open Questions

None at this time.
`)
			d, err := ParseDetail(path)
			if err != nil {
				t.Fatalf("ParseDetail: %v", err)
			}
			if d.Enforcement != tc.wantTier || d.Control != tc.wantControl || d.HasControl() != tc.wantHasControl {
				t.Fatalf("enforcement=%q control=%q hasControl=%v", d.Enforcement, d.Control, d.HasControl())
			}
		})
	}
}

func TestParseDetailRepeatedListFieldsMerge(t *testing.T) {
	root := t.TempDir()
	path := writeDetail(t, root, "x", `# Rule: X

**Status:** Draft
**Scope:** fleet
**Scope:** path:**/*.go, product:sneat
**Sources:** lesson:a
**Sources:** decision:0001

## Open Questions

None at this time.
`)
	d, err := ParseDetail(path)
	if err != nil {
		t.Fatalf("ParseDetail: %v", err)
	}
	if want := []string{"fleet", "path:**/*.go", "product:sneat"}; !reflect.DeepEqual(d.ScopesRaw, want) {
		t.Fatalf("scopes = %v, want %v", d.ScopesRaw, want)
	}
	if want := []string{"lesson:a", "decision:0001"}; !reflect.DeepEqual(d.SourcesRaw, want) {
		t.Fatalf("sources = %v, want %v", d.SourcesRaw, want)
	}
	if d.ScopeText == "" || d.SourcesText == "" {
		t.Fatalf("raw field text must be retained: scope=%q sources=%q", d.ScopeText, d.SourcesText)
	}
}

// A non-rule README under spec/rules must parse without error and report
// HasRuleTitle=false, so callers can tell "not a rule" from "malformed rule".
func TestParseDetailNonRuleFile(t *testing.T) {
	root := t.TempDir()
	path := writeDetail(t, root, "x", "# Something Else\n\ntext\n")
	d, err := ParseDetail(path)
	if err != nil {
		t.Fatalf("ParseDetail: %v", err)
	}
	if d.HasRuleTitle {
		t.Fatal("HasRuleTitle should be false for a non-rule README")
	}
}

func TestParseDetailMissingFile(t *testing.T) {
	if _, err := ParseDetail(filepath.Join(t.TempDir(), "nope", "README.md")); err == nil {
		t.Fatal("ParseDetail of a missing file should error")
	}
}

// A line longer than the parser's 1 MiB scan buffer is a read error, not a
// silently truncated artifact.
func TestParseDetailPropagatesScannerError(t *testing.T) {
	root := t.TempDir()
	path := writeDetail(t, root, "x", "# Rule: X\n**Statement:** "+strings.Repeat("a", 2<<20)+"\n")
	if _, err := ParseDetail(path); err == nil {
		t.Fatal("ParseDetail should fail on a line that exceeds the scan buffer")
	}
}

func TestMirroredValuesAgreeBetweenFormsWhenWrittenTogether(t *testing.T) {
	opts := Options{
		Slug: "x", Date: "2026-09-03", Status: "Active", Statement: "Always x.",
		Scopes: []string{"fleet"}, Enforcement: "Enforced", Control: "wb hook", Sources: []string{"lesson:a"},
	}
	if err := opts.Normalize(); err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	path := scaffoldInto(t, root, opts)
	d, err := ParseDetail(path)
	if err != nil {
		t.Fatal(err)
	}
	want, got := MirroredValuesOf(opts.Row(true)), d.MirroredValues()
	for _, name := range MirroredFields {
		if want[name] != got[name] {
			t.Errorf("mirror %s: row=%q document=%q", name, want[name], got[name])
		}
	}
}

func TestMirroredValuesUnescapeRowCells(t *testing.T) {
	row := NewRow("x", true, "Draft", "Never write a | here", []string{"fleet"}, "Stated", "", nil)
	if got := MirroredValuesOf(row)["Statement"]; got != "Never write a | here" {
		t.Fatalf("mirrored statement = %q, want the unescaped text", got)
	}
	if got := MirroredValuesOf(row)["Control"]; got != Sentinel {
		t.Fatalf("mirrored control = %q, want the sentinel", got)
	}
}

func TestDiscoverDetails(t *testing.T) {
	root := setupRulesTree(t)
	scaffoldInto(t, root, Options{Slug: "zebra", Date: "2026-09-03"})
	scaffoldInto(t, root, Options{Slug: "alpha", Date: "2026-09-03"})
	// Noise that must be ignored: a non-rule README, an invalid slug directory,
	// a slug directory with no README, and a stray file.
	writeDetail(t, root, "not-a-rule", "# Notes\n")
	for _, dir := range []string{"Invalid_Slug", "empty-dir"} {
		if err := os.MkdirAll(filepath.Join(RulesDir(root), dir), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(RulesDir(root), "stray.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	details, err := DiscoverDetails(RulesDir(root))
	if err != nil {
		t.Fatalf("DiscoverDetails: %v", err)
	}
	if len(details) != 2 || details[0].Slug != "alpha" || details[1].Slug != "zebra" {
		t.Fatalf("DiscoverDetails = %v (want alpha, zebra sorted)", detailSlugsOf(details))
	}

	byslug, err := DetailsBySlug(RulesDir(root))
	if err != nil {
		t.Fatalf("DetailsBySlug: %v", err)
	}
	if len(byslug) != 2 || byslug["alpha"] == nil {
		t.Fatalf("DetailsBySlug = %v", byslug)
	}
}

// An absent spec/rules/ directory is not an error: every read verb must work in
// a repository that has recorded no rule yet.
func TestDiscoverDetailsMissingDirectoryIsEmpty(t *testing.T) {
	details, err := DiscoverDetails(filepath.Join(t.TempDir(), "spec", "rules"))
	if err != nil {
		t.Fatalf("DiscoverDetails: %v", err)
	}
	if len(details) != 0 {
		t.Fatalf("DiscoverDetails = %v, want empty", details)
	}
}

func detailSlugsOf(details []*Detail) []string {
	out := make([]string, 0, len(details))
	for _, d := range details {
		out = append(out, d.Slug)
	}
	return out
}

func TestResolveRow(t *testing.T) {
	root := setupRulesTree(t)
	rulesDir := RulesDir(root)
	if err := UpsertRow(rulesDir, NewRow("x", false, "Draft", "s", []string{"fleet"}, "Stated", "", nil)); err != nil {
		t.Fatal(err)
	}
	if row, err := ResolveRow(rulesDir, "x"); err != nil || row.Slug != "x" {
		t.Fatalf("ResolveRow(x) = %+v, %v", row, err)
	}
	if _, err := ResolveRow(rulesDir, "missing"); err == nil {
		t.Fatal("ResolveRow(missing) should error")
	}
	if _, err := ResolveRow(rulesDir, "Bad Slug"); err == nil {
		t.Fatal("ResolveRow with an invalid slug should error")
	}
	if _, err := ResolveRow(filepath.Join(t.TempDir(), "rules"), "x"); err == nil {
		t.Fatal("ResolveRow with no index should error")
	}
}

func TestPathHelpers(t *testing.T) {
	root := "/proj"
	if got := RulesDir(root); got != filepath.Join("/proj", "spec", "rules") {
		t.Fatalf("RulesDir = %q", got)
	}
	if got := DetailPath(RulesDir(root), "x"); !strings.HasSuffix(got, filepath.Join("rules", "x", "README.md")) {
		t.Fatalf("DetailPath = %q", got)
	}
	if got := IndexPath(RulesDir(root)); !strings.HasSuffix(got, filepath.Join("rules", "README.md")) {
		t.Fatalf("IndexPath = %q", got)
	}
}

func TestAppendFieldText(t *testing.T) {
	cases := []struct{ current, value, want string }{
		{"", "a", "a"},
		{"a", "", "a"},
		{"a", "b", "a, b"},
		{"", "", ""},
	}
	for _, tc := range cases {
		if got := appendFieldText(tc.current, tc.value); got != tc.want {
			t.Errorf("appendFieldText(%q,%q) = %q, want %q", tc.current, tc.value, got, tc.want)
		}
	}
}

func TestJoinWrappedFieldValueStopsAtBoundaries(t *testing.T) {
	cases := []struct {
		name     string
		lines    []string
		want     string
		consumed int
	}{
		{name: "stops at a divider", lines: []string{"**Why:** a", "b", "---"}, want: "a b", consumed: 1},
		{name: "stops at a heading", lines: []string{"**Why:** a", "b", "## Next"}, want: "a b", consumed: 1},
		{name: "stops at a blank line", lines: []string{"**Why:** a", "", "b"}, want: "a", consumed: 0},
		{name: "stops at the next field", lines: []string{"**Why:** a", "**Exceptions:** none"}, want: "a", consumed: 0},
		{name: "empty first line still joins", lines: []string{"**Why:**", "b"}, want: "b", consumed: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, consumed := joinWrappedFieldValue(tc.lines, 0, strings.TrimPrefix(tc.lines[0], "**Why:** "))
			if tc.lines[0] == "**Why:**" {
				got, consumed = joinWrappedFieldValue(tc.lines, 0, "")
			}
			if got != tc.want || consumed != tc.consumed {
				t.Fatalf("joinWrappedFieldValue = (%q, %d), want (%q, %d)", got, consumed, tc.want, tc.consumed)
			}
		})
	}
}

func TestMissingSectionsAndSubsections(t *testing.T) {
	d := &Detail{SectionLines: map[string]int{}, SubsectionLines: map[string]int{}}
	if got := d.MissingSections(); len(got) != len(DetailSections) {
		t.Fatalf("MissingSections = %v", got)
	}
	if got := d.MissingExampleSubsections(); len(got) != len(ExampleSubsections) {
		t.Fatalf("MissingExampleSubsections = %v", got)
	}
	for _, s := range DetailSections {
		d.SectionLines[s] = 1
	}
	for _, s := range ExampleSubsections {
		d.SubsectionLines[s] = 1
	}
	if len(d.MissingSections()) != 0 || len(d.MissingExampleSubsections()) != 0 {
		t.Fatal("a complete document must report nothing missing")
	}
}
