package rule

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func TestEnsureIndexIsIdempotentAndNeverOverwrites(t *testing.T) {
	root := setupRulesTree(t)
	if got := readIndexFile(t, root); got != IndexContent() {
		t.Fatal("stub index does not match IndexContent()")
	}
	// A hand-edited index must survive: EnsureIndex only materializes what is
	// missing, so a second call can never discard an author's prologue.
	custom := strings.Replace(IndexContent(), "# Rules", "# Rules\n\nHand-written prologue.", 1)
	if err := os.WriteFile(IndexPath(RulesDir(root)), []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := EnsureIndex(RulesDir(root)); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}
	if got := readIndexFile(t, root); got != custom {
		t.Fatal("EnsureIndex overwrote an existing index")
	}
}

func TestParseIdentityCell(t *testing.T) {
	cases := []struct {
		name       string
		cell       string
		wantSlug   string
		wantLinked bool
		wantOK     bool
	}{
		{name: "bare slug is inline", cell: "never-mock-backends", wantSlug: "never-mock-backends", wantOK: true},
		{name: "link is detailed", cell: "[x](x/README.md)", wantSlug: "x", wantLinked: true, wantOK: true},
		{name: "whitespace tolerated", cell: "  x  ", wantSlug: "x", wantOK: true},
		{name: "non-slug bare cell", cell: "Not A Slug"},
		{name: "label and link disagree", cell: "[y](x/README.md)"},
		{name: "flat link shape", cell: "[x](x.md)"},
		{name: "unterminated link", cell: "[x](x/README.md"},
		{name: "empty label", cell: "[](x/README.md)"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			slug, linked, ok := parseIdentityCell(tc.cell)
			if ok != tc.wantOK || slug != tc.wantSlug || linked != tc.wantLinked {
				t.Fatalf("parseIdentityCell(%q) = (%q, %v, %v), want (%q, %v, %v)",
					tc.cell, slug, linked, ok, tc.wantSlug, tc.wantLinked, tc.wantOK)
			}
		})
	}
}

func TestRowRenderAndEquals(t *testing.T) {
	row := NewRow("x", false, "Draft", "Always x.", []string{"fleet"}, "Stated", "", nil)
	if got := row.Render(); got != "| x | Draft | fleet | Stated | — | — | Always x. |" {
		t.Fatalf("render = %q", got)
	}
	if !row.Equals(row) {
		t.Fatal("a row must equal itself")
	}
	// Line is provenance, not content: two rows that say the same thing on
	// different lines are the same row.
	shifted := row
	shifted.Line = 42
	if !row.Equals(shifted) {
		t.Fatal("Equals must ignore the source line")
	}
	other := row
	other.Status = "Active"
	if row.Equals(other) {
		t.Fatal("rows differing in a cell must not compare equal")
	}
	if row.Detailed() || row.HasControl() {
		t.Fatal("an inline row with a sentinel control must report neither")
	}
	withControl := NewRow("x", true, "Draft", "s", []string{"fleet"}, "Enforced", "wb hook", []string{"lesson:a"})
	if !withControl.Detailed() || !withControl.HasControl() {
		t.Fatal("a linked row with a control must report both")
	}
	if got := withControl.DetailLink(); got != "x/README.md" {
		t.Fatalf("DetailLink = %q", got)
	}
	if want := []string{"lesson:a"}; !reflect.DeepEqual(withControl.SourceList(), want) {
		t.Fatalf("SourceList = %v", withControl.SourceList())
	}
}

// A pipe in a statement must be escaped so it cannot fabricate a column, and
// must survive a read/write round trip as one cell.
func TestRowEscapesPipes(t *testing.T) {
	root := setupRulesTree(t)
	row := NewRow("x", false, "Draft", "Never write a | in a table", []string{"fleet"}, "Stated", "", nil)
	if !strings.Contains(row.Statement, `\|`) {
		t.Fatalf("pipe not escaped: %q", row.Statement)
	}
	if err := UpsertRow(RulesDir(root), row); err != nil {
		t.Fatalf("UpsertRow: %v", err)
	}
	report, err := ReadIndex(IndexPath(RulesDir(root)))
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Rows) != 1 || report.Rows[0].Statement != row.Statement {
		t.Fatalf("round trip lost the escaped pipe: %+v", report.Rows)
	}
	if unescapeCell(report.Rows[0].Statement) != "Never write a | in a table" {
		t.Fatalf("unescape = %q", unescapeCell(report.Rows[0].Statement))
	}
}

func TestWriteIndexRowsMatchesGolden(t *testing.T) {
	root := setupRulesTree(t)
	rows := []Row{
		NewRow("beta-rule", false, "Draft", "Never do the beta thing without the alpha thing.", []string{"path:**/*.go"}, "Stated", "", nil),
		NewRow("alpha-rule", true, "Active", "Always do the alpha thing before the beta thing.", []string{"fleet"}, "Enforced", "wb pre-commit hook", []string{"lesson:a"}),
	}
	path := IndexPath(RulesDir(root))
	if err := WriteIndexRows(path, rows); err != nil {
		t.Fatalf("WriteIndexRows: %v", err)
	}
	if got, want := readIndexFile(t, root), readGolden(t, "index_two_rules.golden"); got != want {
		t.Fatalf("index does not match golden.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
	// Regeneration is idempotent: a second pass must be byte-for-byte identical.
	if err := WriteIndexRows(path, rows); err != nil {
		t.Fatalf("WriteIndexRows (second pass): %v", err)
	}
	if got, want := readIndexFile(t, root), readGolden(t, "index_two_rules.golden"); got != want {
		t.Fatal("WriteIndexRows is not idempotent")
	}
}

func TestWriteIndexRowsEmptySetRestoresPlaceholder(t *testing.T) {
	root := setupRulesTree(t)
	if err := WriteIndexRows(IndexPath(RulesDir(root)), nil); err != nil {
		t.Fatalf("WriteIndexRows: %v", err)
	}
	if !strings.Contains(readIndexFile(t, root), IndexEmptyPlaceholder) {
		t.Fatalf("empty index lost its placeholder:\n%s", readIndexFile(t, root))
	}
}

func TestWriteIndexRowsPreservesPrologueAndTrailer(t *testing.T) {
	root := setupRulesTree(t)
	custom := strings.Replace(IndexContent(), "# Rules\n", "# Rules\n\nHand-written prologue paragraph.\n", 1)
	custom = strings.Replace(custom, "None at this time.", "Should the fleet scope be implicit?", 1)
	if err := os.WriteFile(IndexPath(RulesDir(root)), []byte(custom), 0o644); err != nil {
		t.Fatal(err)
	}
	rows := []Row{NewRow("x", false, "Draft", "s", []string{"fleet"}, "Stated", "", nil)}
	if err := WriteIndexRows(IndexPath(RulesDir(root)), rows); err != nil {
		t.Fatalf("WriteIndexRows: %v", err)
	}
	got := readIndexFile(t, root)
	for _, want := range []string{"Hand-written prologue paragraph.", "Should the fleet scope be implicit?", "| x | Draft |"} {
		if !strings.Contains(got, want) {
			t.Fatalf("rewrite lost %q:\n%s", want, got)
		}
	}
}

func TestWriteIndexRowsWithoutHeadingErrors(t *testing.T) {
	root := setupRulesTree(t)
	if err := os.WriteFile(IndexPath(RulesDir(root)), []byte("# Rules\n\nno table\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteIndexRows(IndexPath(RulesDir(root)), nil); err == nil {
		t.Fatal("WriteIndexRows should refuse an index with no `## Rules` heading")
	}
}

func TestUpsertRow(t *testing.T) {
	root := setupRulesTree(t)
	rulesDir := RulesDir(root)
	row := NewRow("x", false, "Draft", "Always x.", []string{"fleet"}, "Stated", "", nil)
	if err := UpsertRow(rulesDir, row); err != nil {
		t.Fatalf("UpsertRow: %v", err)
	}
	got := readIndexFile(t, root)
	if !strings.Contains(got, "| x | Draft | fleet | Stated | — | — | Always x. |") {
		t.Fatalf("row not inserted:\n%s", got)
	}
	if strings.Contains(got, IndexEmptyPlaceholder) {
		t.Fatalf("placeholder retained alongside a real row:\n%s", got)
	}

	// A second upsert replaces the row in place rather than appending.
	row.Status = "Active"
	if err := UpsertRow(rulesDir, row); err != nil {
		t.Fatalf("UpsertRow (second): %v", err)
	}
	got = readIndexFile(t, root)
	if strings.Count(got, "| x |") != 1 || !strings.Contains(got, "| x | Active |") {
		t.Fatalf("upsert appended instead of replacing:\n%s", got)
	}

	// Other rows are preserved untouched.
	if err := UpsertRow(rulesDir, NewRow("a", false, "Draft", "Always a.", []string{"fleet"}, "Stated", "", nil)); err != nil {
		t.Fatal(err)
	}
	got = readIndexFile(t, root)
	if !strings.Contains(got, "| a | Draft |") || !strings.Contains(got, "| x | Active |") {
		t.Fatalf("upsert lost a sibling row:\n%s", got)
	}
	// Rows stay sorted by slug, so a reviewer sees a stable diff.
	if strings.Index(got, "| a |") > strings.Index(got, "| x |") {
		t.Fatalf("rows are not sorted:\n%s", got)
	}
}

func TestUpsertRowRejects(t *testing.T) {
	root := setupRulesTree(t)
	rulesDir := RulesDir(root)
	row := NewRow("x", false, "Draft", "s", []string{"fleet"}, "Stated", "", nil)

	t.Run("no canonical table", func(t *testing.T) {
		if err := os.WriteFile(IndexPath(rulesDir), []byte("# Rules\n\n## Rules\n\nno table\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := UpsertRow(rulesDir, row); err == nil {
			t.Fatal("UpsertRow should refuse an index with no canonical table")
		}
	})

	t.Run("duplicate rows", func(t *testing.T) {
		body := "# Rules\n\n" + IndexHeading + "\n\n" + IndexHeaderRow + "\n" + IndexSeparatorRow +
			"\n" + row.Render() + "\n" + row.Render() + "\n\n## Open Questions\n\nNone.\n"
		if err := os.WriteFile(IndexPath(rulesDir), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := UpsertRow(rulesDir, row); err == nil {
			t.Fatal("UpsertRow should refuse a duplicated row rather than pick one")
		}
	})

	t.Run("missing index", func(t *testing.T) {
		if err := UpsertRow(filepath.Join(t.TempDir(), "rules"), row); err == nil {
			t.Fatal("UpsertRow should fail with no index file")
		}
	})
}

func TestRemoveRow(t *testing.T) {
	root := setupRulesTree(t)
	rulesDir := RulesDir(root)
	for _, slug := range []string{"a", "b"} {
		if err := UpsertRow(rulesDir, NewRow(slug, false, "Draft", "s", []string{"fleet"}, "Stated", "", nil)); err != nil {
			t.Fatal(err)
		}
	}
	if err := RemoveRow(rulesDir, "a"); err != nil {
		t.Fatalf("RemoveRow: %v", err)
	}
	got := readIndexFile(t, root)
	if strings.Contains(got, "| a |") || !strings.Contains(got, "| b |") {
		t.Fatalf("wrong row removed:\n%s", got)
	}
	if strings.Contains(got, IndexEmptyPlaceholder) {
		t.Fatalf("placeholder restored while a row remains:\n%s", got)
	}
	if err := RemoveRow(rulesDir, "b"); err != nil {
		t.Fatalf("RemoveRow: %v", err)
	}
	if !strings.Contains(readIndexFile(t, root), IndexEmptyPlaceholder) {
		t.Fatalf("placeholder not restored on an empty table:\n%s", readIndexFile(t, root))
	}
	// Removing an absent row is a no-op, so `rule delete` still finishes on a
	// tree whose index had already drifted.
	if err := RemoveRow(rulesDir, "never-existed"); err != nil {
		t.Fatalf("RemoveRow(absent): %v", err)
	}
	if err := RemoveRow(filepath.Join(t.TempDir(), "rules"), "x"); err != nil {
		t.Fatalf("RemoveRow with no index file should be a no-op: %v", err)
	}
}

func TestReadIndexReportsShapeProblems(t *testing.T) {
	cases := []struct {
		name          string
		table         string
		wantRows      int
		wantHeader    bool
		wantMalformed bool
		wantDuplicate bool
	}{
		{
			name:       "canonical inline row",
			table:      IndexHeaderRow + "\n" + IndexSeparatorRow + "\n| x | Draft | fleet | Stated | — | — | s |\n",
			wantRows:   1,
			wantHeader: true,
		},
		{
			name:       "canonical linked row",
			table:      IndexHeaderRow + "\n" + IndexSeparatorRow + "\n| [x](x/README.md) | Draft | fleet | Stated | — | — | s |\n",
			wantRows:   1,
			wantHeader: true,
		},
		{
			name:          "wrong column count",
			table:         IndexHeaderRow + "\n" + IndexSeparatorRow + "\n| x | Draft |\n",
			wantHeader:    true,
			wantMalformed: true,
		},
		{
			name:          "unparsable identity cell",
			table:         IndexHeaderRow + "\n" + IndexSeparatorRow + "\n| Not A Slug | Draft | fleet | Stated | — | — | s |\n",
			wantHeader:    true,
			wantMalformed: true,
		},
		{
			name:          "duplicate slug",
			table:         IndexHeaderRow + "\n" + IndexSeparatorRow + "\n| x | Draft | fleet | Stated | — | — | s |\n| x | Draft | fleet | Stated | — | — | s |\n",
			wantRows:      2,
			wantHeader:    true,
			wantDuplicate: true,
		},
		{
			name:     "no header",
			table:    "| x | Draft | fleet | Stated | — | — | s |\n",
			wantRows: 1,
		},
		{
			name:       "prose inside the section is ignored",
			table:      "A sentence of prose.\n\n" + IndexHeaderRow + "\n" + IndexSeparatorRow + "\n| x | Draft | fleet | Stated | — | — | s |\n",
			wantRows:   1,
			wantHeader: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			dir := RulesDir(root)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			body := "# Rules\n\n" + IndexHeading + "\n\n" + tc.table + "\n## Open Questions\n\nNone.\n"
			if err := os.WriteFile(IndexPath(dir), []byte(body), 0o644); err != nil {
				t.Fatal(err)
			}
			rep, err := ReadIndex(IndexPath(dir))
			if err != nil {
				t.Fatalf("ReadIndex: %v", err)
			}
			if len(rep.Rows) != tc.wantRows || rep.HeaderSeen != tc.wantHeader ||
				rep.Malformed != tc.wantMalformed || (len(rep.Duplicates) > 0) != tc.wantDuplicate {
				t.Fatalf("rows=%d header=%v malformed=%v duplicates=%v", len(rep.Rows), rep.HeaderSeen, rep.Malformed, rep.Duplicates)
			}
			if tc.wantMalformed && len(rep.MalformedLines) == 0 {
				t.Fatal("a malformed row must report its source line")
			}
			for _, row := range rep.Rows {
				if row.Line == 0 {
					t.Fatal("every parsed row must carry its source line")
				}
			}
		})
	}
}

func TestIndexReportAccessors(t *testing.T) {
	rep := IndexReport{Rows: []Row{
		{Slug: "b", Status: "Draft"},
		{Slug: "a", Status: "Active"},
		{Slug: "b", Status: "Superseded"},
	}}
	byslug := rep.BySlug()
	if len(byslug) != 2 || byslug["b"].Status != "Draft" {
		t.Fatalf("BySlug = %+v (the first row of a duplicated slug wins)", byslug)
	}
	if want := []string{"a", "b"}; !reflect.DeepEqual(rep.Slugs(), want) {
		t.Fatalf("Slugs = %v, want %v", rep.Slugs(), want)
	}
}

func TestReadIndexMissingFile(t *testing.T) {
	if _, err := ReadIndex(filepath.Join(t.TempDir(), "README.md")); err == nil {
		t.Fatal("ReadIndex of a missing file should error")
	}
}

func TestSplitMarkdownRowEdgeCases(t *testing.T) {
	if got := splitMarkdownRow("||"); len(got) > 1 {
		t.Fatalf("splitMarkdownRow(||) = %v", got)
	}
	if got := splitMarkdownRow(""); got != nil {
		t.Fatalf("splitMarkdownRow(empty) = %v", got)
	}
	if got := splitMarkdownRow(`| a \| b | c |`); len(got) != 2 || got[0] != `a \| b` {
		t.Fatalf("splitMarkdownRow = %v", got)
	}
}

func TestEscapeCellEmptyIsSentinel(t *testing.T) {
	if got := escapeCell("   "); got != Sentinel {
		t.Fatalf("escapeCell(blank) = %q", got)
	}
}

func TestRowFromDetail(t *testing.T) {
	root := setupRulesTree(t)
	path := scaffoldInto(t, root, Options{
		Slug: "x", Date: "2026-09-03", Status: "Active", Statement: "Always x.",
		Scopes: []string{"fleet"}, Enforcement: "Enforced", Control: "wb hook", Sources: []string{"lesson:a"},
	})
	d, err := ParseDetail(path)
	if err != nil {
		t.Fatal(err)
	}
	row := RowFromDetail(d)
	if !row.Linked || row.Slug != "x" || row.Status != "Active" || row.Control != "wb hook" {
		t.Fatalf("RowFromDetail = %+v", row)
	}
}

func TestWriteFileAtomicPreservesMode(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "f.md")
	if err := os.WriteFile(path, []byte("a"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := WriteFileAtomic(path, []byte("b")); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %o, want 600", info.Mode().Perm())
	}
	if string(mustRead(t, path)) != "b" {
		t.Fatal("content not published")
	}
}
