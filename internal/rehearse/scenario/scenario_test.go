package scenario_test

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/internal/rehearse/scenario"
)

func write(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "scenario.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestParse_CorpusShapedScenario(t *testing.T) {
	path := write(t, `---
format: https://specscore.md/scenario-specification
---

# Rehearse: sample

**Status:** pending
**Verifies:** cli/studio/index#ac:index-two-repos (REQ: workspace-config), cli/studio/index#ac:other (note, with comma)

Prose describing the scenario.

`+"```bash"+`
echo one
`+"```"+`

More prose.

`+"```sql dsn=sqlite:facts.db mode"+`
SELECT 1;
`+"```"+`
`)
	sc, err := scenario.Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if sc.Path != path {
		t.Errorf("Path = %q", sc.Path)
	}
	if sc.Status != "pending" {
		t.Errorf("Status = %q, want pending", sc.Status)
	}
	wantVerifies := []string{"cli/studio/index#ac:index-two-repos", "cli/studio/index#ac:other"}
	if !reflect.DeepEqual(sc.Verifies, wantVerifies) {
		t.Errorf("Verifies = %v, want %v", sc.Verifies, wantVerifies)
	}
	if len(sc.Blocks) != 2 {
		t.Fatalf("Blocks = %d, want 2", len(sc.Blocks))
	}
	if sc.Blocks[0].Kind != "bash" || sc.Blocks[0].Body != "echo one" {
		t.Errorf("block 0 = %+v", sc.Blocks[0])
	}
	if sc.Blocks[0].Line == 0 {
		t.Error("block 0 has no opening-fence line number")
	}
	if sc.Blocks[1].Kind != "sql" {
		t.Errorf("block 1 kind = %q, want sql", sc.Blocks[1].Kind)
	}
	wantParams := map[string]string{"dsn": "sqlite:facts.db", "mode": ""}
	if !reflect.DeepEqual(sc.Blocks[1].Params, wantParams) {
		t.Errorf("block 1 params = %v, want %v", sc.Blocks[1].Params, wantParams)
	}
}

func TestParse_FirstMetadataOccurrenceWins(t *testing.T) {
	path := write(t, `**Status:** pending
**Verifies:** a#ac:one
**Status:** later
**Verifies:** b#ac:two
`)
	sc, err := scenario.Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if sc.Status != "pending" {
		t.Errorf("Status = %q, want pending", sc.Status)
	}
	if !reflect.DeepEqual(sc.Verifies, []string{"a#ac:one"}) {
		t.Errorf("Verifies = %v", sc.Verifies)
	}
}

func TestParse_MetadataInsideFenceIgnored(t *testing.T) {
	path := write(t, "```bash\n**Verifies:** not/metadata#ac:x\necho hi\n```\n")
	sc, err := scenario.Parse(path)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(sc.Verifies) != 0 {
		t.Errorf("metadata inside a fence leaked: %v", sc.Verifies)
	}
	if !strings.Contains(sc.Blocks[0].Body, "**Verifies:**") {
		t.Errorf("fence body lost a line: %q", sc.Blocks[0].Body)
	}
}

func TestParse_ExpectFail(t *testing.T) {
	sc, err := scenario.ParseBytes("x.md", []byte("**Expect:** fail\n"))
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	if sc.Expect != "fail" {
		t.Errorf("Expect = %q, want \"fail\"", sc.Expect)
	}
}

func TestParse_ExpectDefaultsToPass(t *testing.T) {
	sc, err := scenario.ParseBytes("x.md", []byte("**Verifies:** demo#ac:x\n"))
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	if sc.Expect != "pass" {
		t.Errorf("Expect = %q, want \"pass\" (default)", sc.Expect)
	}
}

func TestParse_ExpectUnknownValueIsPass(t *testing.T) {
	sc, err := scenario.ParseBytes("x.md", []byte("**Expect:** maybe\n"))
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	if sc.Expect != "pass" {
		t.Errorf("Expect = %q, want \"pass\" for an unknown value", sc.Expect)
	}
}

func TestParse_VerifiesPlaceholderMeansNone(t *testing.T) {
	sc, err := scenario.ParseBytes("x.md", []byte("**Verifies:** —\n"))
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	if len(sc.Verifies) != 0 {
		t.Errorf("Verifies = %v, want none", sc.Verifies)
	}
	if sc.Verifies == nil {
		t.Error("Verifies is nil, want empty slice")
	}
}

func TestParse_BareAndLongFences(t *testing.T) {
	sc, err := scenario.ParseBytes("x.md", []byte("````\nplain\n````\n\n```bash\necho hi\n`````\n"))
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	if len(sc.Blocks) != 2 {
		t.Fatalf("Blocks = %d, want 2", len(sc.Blocks))
	}
	if sc.Blocks[0].Kind != "" || sc.Blocks[0].Body != "plain" {
		t.Errorf("bare fence block = %+v", sc.Blocks[0])
	}
	if sc.Blocks[1].Body != "echo hi" {
		t.Errorf("long closing fence mishandled: %+v", sc.Blocks[1])
	}
}

func TestParse_UnclosedFenceErrors(t *testing.T) {
	_, err := scenario.ParseBytes("bad.md", []byte("prose\n```bash\necho hi\n"))
	if err == nil {
		t.Fatal("unclosed fence did not error")
	}
	if !strings.Contains(err.Error(), "unclosed fenced block opened at line 2") {
		t.Errorf("error lacks position detail: %v", err)
	}
}

func TestParse_MissingFileErrors(t *testing.T) {
	_, err := scenario.Parse(filepath.Join(t.TempDir(), "absent.md"))
	if err == nil {
		t.Fatal("missing file did not error")
	}
	if !strings.Contains(err.Error(), "reading scenario") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestParse_NoMetadataNoBlocks(t *testing.T) {
	sc, err := scenario.ParseBytes("empty.md", []byte("# Just prose\n\nNothing else.\n"))
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	if len(sc.Blocks) != 0 || len(sc.Verifies) != 0 || sc.Status != "" {
		t.Errorf("unexpected content parsed: %+v", sc)
	}
}

// --- File assertion tests ---

func TestParseFileAssertions_SingleBlock(t *testing.T) {
	md := "### Assert: file `output/report.txt` contains\n\n```\nHello, world\n```\n"
	sc, err := scenario.ParseBytes("x.md", []byte(md))
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	if len(sc.FileAssertions) != 1 {
		t.Fatalf("FileAssertions = %d, want 1", len(sc.FileAssertions))
	}
	fa := sc.FileAssertions[0]
	if fa.Path != "output/report.txt" {
		t.Errorf("Path = %q, want %q", fa.Path, "output/report.txt")
	}
	if fa.Kind != "contains" {
		t.Errorf("Kind = %q, want contains", fa.Kind)
	}
	if fa.Expected != "Hello, world" {
		t.Errorf("Expected = %q, want %q", fa.Expected, "Hello, world")
	}
}

func TestParseFileAssertions_MultipleBlocks(t *testing.T) {
	md := "### Assert: file `/tmp/a.txt` exists\n\n### Assert: file `/tmp/b.txt` missing\n\n"
	sc, err := scenario.ParseBytes("x.md", []byte(md))
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	if len(sc.FileAssertions) != 2 {
		t.Fatalf("FileAssertions = %d, want 2", len(sc.FileAssertions))
	}
	if sc.FileAssertions[0].Path != "/tmp/a.txt" || sc.FileAssertions[0].Kind != "exists" {
		t.Errorf("assertion 0 = %+v", sc.FileAssertions[0])
	}
	if sc.FileAssertions[1].Path != "/tmp/b.txt" || sc.FileAssertions[1].Kind != "missing" {
		t.Errorf("assertion 1 = %+v", sc.FileAssertions[1])
	}
}

func TestParseFileAssertions_AllKinds(t *testing.T) {
	kinds := []string{"exists", "missing", "contains", "not-contains", "permissions"}
	var sb strings.Builder
	for _, k := range kinds {
		sb.WriteString("### Assert: file `f.txt` " + k + "\n\n")
	}
	sc, err := scenario.ParseBytes("x.md", []byte(sb.String()))
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	if len(sc.FileAssertions) != len(kinds) {
		t.Fatalf("FileAssertions = %d, want %d", len(sc.FileAssertions), len(kinds))
	}
	for i, k := range kinds {
		if sc.FileAssertions[i].Kind != k {
			t.Errorf("assertion[%d].Kind = %q, want %q", i, sc.FileAssertions[i].Kind, k)
		}
	}
}

func TestParseFileAssertions_MultilineContent(t *testing.T) {
	md := "### Assert: file `out.txt` contains\n\n```\nline one\nline two\nline three\n```\n"
	sc, err := scenario.ParseBytes("x.md", []byte(md))
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	if len(sc.FileAssertions) != 1 {
		t.Fatalf("FileAssertions = %d, want 1", len(sc.FileAssertions))
	}
	want := "line one\nline two\nline three"
	if sc.FileAssertions[0].Expected != want {
		t.Errorf("Expected = %q, want %q", sc.FileAssertions[0].Expected, want)
	}
}

func TestParseFileAssertions_NoCodeBlock(t *testing.T) {
	// 'exists' needs no code block; heading alone is sufficient.
	md := "### Assert: file `check.txt` exists\n\n### Assert: file `other.txt` exists\n\n"
	sc, err := scenario.ParseBytes("x.md", []byte(md))
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	if len(sc.FileAssertions) != 2 {
		t.Fatalf("FileAssertions = %d, want 2", len(sc.FileAssertions))
	}
	if sc.FileAssertions[0].Expected != "" {
		t.Errorf("Expected should be empty for exists with no code block, got %q", sc.FileAssertions[0].Expected)
	}
}

func TestParseFileAssertions_InvalidKind(t *testing.T) {
	// An unrecognised kind should still be parsed (stored as-is); callers
	// validate the kind during evaluation — the parser does not error.
	md := "### Assert: file `f.txt` reallybadkind\n\n"
	sc, err := scenario.ParseBytes("x.md", []byte(md))
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	if len(sc.FileAssertions) != 1 {
		t.Fatalf("FileAssertions = %d, want 1", len(sc.FileAssertions))
	}
	if sc.FileAssertions[0].Kind != "reallybadkind" {
		t.Errorf("Kind = %q, want reallybadkind", sc.FileAssertions[0].Kind)
	}
}

func TestParseFileAssertions_EmptyAssertions(t *testing.T) {
	md := "# Rehearse: no file assertions\n\n**Status:** pending\n\n```bash\necho hi\n```\n"
	sc, err := scenario.ParseBytes("x.md", []byte(md))
	if err != nil {
		t.Fatalf("ParseBytes: %v", err)
	}
	if len(sc.FileAssertions) != 0 {
		t.Errorf("FileAssertions = %v, want empty", sc.FileAssertions)
	}
	if sc.FileAssertions == nil {
		t.Error("FileAssertions is nil, want non-nil empty slice")
	}
}
