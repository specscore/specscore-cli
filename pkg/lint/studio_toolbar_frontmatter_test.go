package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/projectdef"
)

// expectedFooToolbar is the canonical toolbar line for spec/features/foo under
// the default studio config + AC project identity.
func expectedFooToolbar() string {
	return strings.TrimRight(RenderStudioToolbar(
		"SpecScore.Studio", "https://specscore.studio/",
		acHost, acOwner, acRepo, "spec/features/foo"), "\n")
}

func writeFooFeature(t *testing.T, root, content string) string {
	t.Helper()
	dir := filepath.Join(root, "spec", "features", "foo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "README.md")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

const fooFM = "---\nformat: https://specscore.md/feature-specification\nstatus: Draft\n---\n\n"

// A feature carrying leading frontmatter with the toolbar correctly placed two
// lines below the title passes (the rule tracks the title, not file line 3).
func TestStudioToolbar_FrontmatterTolerated(t *testing.T) {
	root := setupStudioProject(t, nil)
	writeFooFeature(t, root, fooFM+"# Feature: Foo\n\n"+expectedFooToolbar()+"\n\n**Status:** Draft\n")
	c := newStudioToolbarChecker()
	v, err := c.check(filepath.Join(root, "spec"))
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 0 {
		t.Errorf("expected no violation for frontmatter+correct toolbar, got %+v", v)
	}
}

// A drifted toolbar below frontmatter is flagged at the title+2 line (8), not 3.
func TestStudioToolbar_FrontmatterDriftFlagged(t *testing.T) {
	root := setupStudioProject(t, nil)
	writeFooFeature(t, root, fooFM+"# Feature: Foo\n\n> [SpecScore.**Studio**](https://specscore.studio): | wrong |\n\n**Status:** Draft\n")
	c := newStudioToolbarChecker()
	v, err := c.check(filepath.Join(root, "spec"))
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 1 || v[0].Rule != "studio-toolbar" {
		t.Fatalf("expected one studio-toolbar violation, got %+v", v)
	}
	if v[0].Line != 8 {
		t.Errorf("expected violation at line 8 (title+2 below frontmatter), got %d", v[0].Line)
	}
}

// A feature with frontmatter + title but no toolbar (position past EOF) is
// flagged as missing.
func TestStudioToolbar_FrontmatterMissingFlagged(t *testing.T) {
	root := setupStudioProject(t, nil)
	writeFooFeature(t, root, fooFM+"# Feature: Foo\n")
	c := newStudioToolbarChecker()
	v, err := c.check(filepath.Join(root, "spec"))
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 1 || !strings.Contains(v[0].Message, "missing studio toolbar") {
		t.Fatalf("expected a missing-toolbar violation, got %+v", v)
	}
}

// --fix inserts the toolbar below the frontmatter + title (line 8), leaving the
// frontmatter block intact.
func TestStudioToolbarFix_FrontmatterInsert(t *testing.T) {
	root := setupStudioProject(t, nil)
	p := writeFooFeature(t, root, fooFM+"# Feature: Foo\n\n**Status:** Draft\n")
	c := newStudioToolbarChecker().(fixer)
	if err := c.fix(filepath.Join(root, "spec")); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	lines := strings.Split(string(got), "\n")
	// Frontmatter intact at the top.
	if lines[0] != "---" || lines[1] != "format: https://specscore.md/feature-specification" {
		t.Errorf("frontmatter mangled by fix:\n%s", got)
	}
	if lines[5] != "# Feature: Foo" {
		t.Errorf("title moved unexpectedly:\n%s", got)
	}
	if lines[7] != expectedFooToolbar() {
		t.Errorf("toolbar not inserted at title+2 (line 8).\n got: %q\nwant: %q", lines[7], expectedFooToolbar())
	}
	// Re-check is clean.
	v, _ := newStudioToolbarChecker().check(filepath.Join(root, "spec"))
	if len(v) != 0 {
		t.Errorf("still flagged after fix: %+v", v)
	}
}

// Opt-out --fix strips a toolbar sitting below frontmatter.
func TestStudioToolbarFix_FrontmatterOptOutRemoves(t *testing.T) {
	root := t.TempDir()
	body := projectdef.SchemaHeader + "\nproject:\n  host: github.com\n  org: o\n  repo: r\nstudio: null\n"
	if err := os.WriteFile(filepath.Join(root, projectdef.SpecConfigFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	p := writeFooFeature(t, root, fooFM+"# Feature: Foo\n\n> [SpecScore.**Studio**](x): | [Explore](x) |\n\n**Status:** Draft\n")
	c := newStudioToolbarChecker().(fixer)
	if err := c.fix(filepath.Join(root, "spec")); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if strings.Contains(string(got), "> [SpecScore.") {
		t.Errorf("toolbar below frontmatter should have been stripped:\n%s", got)
	}
	if !strings.HasPrefix(string(got), "---\nformat:") {
		t.Errorf("frontmatter mangled:\n%s", got)
	}
}

// A feature README lacking any `# ` title falls back to the legacy position 3
// (other rules flag the missing title; studio-toolbar still operates).
func TestStudioToolbar_NoTitleFallsBackToPosition3(t *testing.T) {
	root := setupStudioProject(t, nil)
	p := writeFooFeature(t, root, "no title here\nplain body\n")
	c := newStudioToolbarChecker()
	v, err := c.check(filepath.Join(root, "spec"))
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 1 || v[0].Line != 3 {
		t.Fatalf("expected one violation at line 3 (legacy fallback), got %+v", v)
	}
	if err := c.(fixer).fix(filepath.Join(root, "spec")); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	lines := strings.Split(string(got), "\n")
	if len(lines) < 3 || lines[2] != expectedFooToolbar() {
		t.Errorf("toolbar not inserted at fallback position 3:\n%s", got)
	}
}

func TestStudioToolbarLineIndex(t *testing.T) {
	cases := []struct {
		name    string
		content string
		want    int
	}{
		{"no frontmatter, title at 0", "# Feature: Foo\n\n> tb\n", 2},
		{"frontmatter then title", "---\nformat: x\n---\n\n# Feature: Foo\n", 6},
		{"dotted frontmatter closer then title", "---\nformat: x\n...\n\n# Feature: Foo\n", 6},
		{"no title", "just text here\n", -1},
		{"opening fence never closed, no title", "---\nformat: x\nno close\n", -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := studioToolbarLineIndex(strings.Split(tc.content, "\n"))
			if got != tc.want {
				t.Errorf("studioToolbarLineIndex = %d, want %d", got, tc.want)
			}
		})
	}
}
