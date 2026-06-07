package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/projectdef"
)

// AC-test helpers — fixtures referenced by TestAC_* tests below
// (studio-toolbar Feature, 14 Acceptance Criteria).

const (
	acHost  = "github.com"
	acOwner = "specscore"
	acRepo  = "specscore"
)

// acDefaultExpectedLine is the canonical toolbar bytes for a feature at
// spec/features/repo-config under specscore/specscore with defaults,
// per AC: toolbar-rendered-with-defaults. Used as the byte-equality target
// for several AC tests.
const acDefaultExpectedLine = "> [SpecScore.**Studio**](https://specscore.studio): | " +
	"[Explore](https://specscore.studio/app/github.com/specscore/specscore/spec/features/repo-config?op=explore) | " +
	"[Edit](https://specscore.studio/app/github.com/specscore/specscore/spec/features/repo-config?op=edit) | " +
	"[Ask question](https://specscore.studio/app/github.com/specscore/specscore/spec/features/repo-config?op=ask) | " +
	"[Request change](https://specscore.studio/app/github.com/specscore/specscore/spec/features/repo-config?op=request-change) |"

// setupDefaultStudioProject writes a header-only specscore.yaml so the
// studio defaults apply (name=SpecScore.Studio, url=https://specscore.studio/),
// with explicit project host/org/repo so URL composition doesn't depend
// on git remote discovery.
func setupDefaultStudioProject(t *testing.T) string {
	t.Helper()
	return setupStudioProjectWithIdentity(t, nil, acHost, acOwner, acRepo)
}

// setupStudioProjectWithIdentity inits a tmpdir with specscore.yaml
// carrying the supplied studio config (nil = defaults) and the given
// project identity (host/org/repo). Creates spec/features/.
func setupStudioProjectWithIdentity(t *testing.T, studio *projectdef.StudioConfig, host, org, repo string) string {
	t.Helper()
	root := t.TempDir()
	cfg := projectdef.SpecConfig{
		Project: &projectdef.ProjectConfig{
			Title: "Test",
			Host:  host,
			Org:   org,
			Repo:  repo,
		},
	}
	if studio != nil {
		cfg.Studio = studio
	}
	if err := projectdef.WriteSpecConfig(root, cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "spec", "features"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

// setupOptOutProject writes specscore.yaml with `studio: null` and the
// canonical AC-suite project identity.
func setupOptOutProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	body := projectdef.SchemaHeader + "\nproject:\n  title: Test\n  host: " + acHost +
		"\n  org: " + acOwner + "\n  repo: " + acRepo + "\nstudio: null\n"
	if err := os.WriteFile(filepath.Join(root, projectdef.SpecConfigFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "spec", "features"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

// setupViewerStillPresentProject writes a raw specscore.yaml whose body
// (between the schema header and EOF) is exactly bodyYAML. Used for the
// three forms of the viewer: block (mapping, null, bare key).
func setupViewerStillPresentProject(t *testing.T, bodyYAML string) string {
	t.Helper()
	root := t.TempDir()
	body := projectdef.SchemaHeader + "\n" + bodyYAML
	if err := os.WriteFile(filepath.Join(root, projectdef.SpecConfigFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "spec", "features"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

// writeStudioFeatureReadme writes spec/features/<slug>/README.md with the given
// content under root. Returns the absolute README path.
func writeStudioFeatureReadme(t *testing.T, root, slug, content string) string {
	t.Helper()
	dir := filepath.Join(root, "spec", "features", slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	readme := filepath.Join(dir, "README.md")
	if err := os.WriteFile(readme, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return readme
}

// findStudioToolbarViolations filters the slice down to rule==studio-toolbar.
func findStudioToolbarViolations(vs []Violation) []Violation {
	var out []Violation
	for _, v := range vs {
		if v.Rule == "studio-toolbar" {
			out = append(out, v)
		}
	}
	return out
}

// TestRenderStudioToolbar_DefaultsMatchAC mirrors the byte string from
// the studio-toolbar Feature's AC: toolbar-rendered-with-defaults.
func TestRenderStudioToolbar_DefaultsMatchAC(t *testing.T) {
	got := RenderStudioToolbar(
		"SpecScore.Studio",
		"https://specscore.studio/",
		"github.com",
		"specscore",
		"specscore",
		"spec/features/repo-config",
	)
	want := "> [SpecScore.**Studio**](https://specscore.studio): | " +
		"[Explore](https://specscore.studio/app/github.com/specscore/specscore/spec/features/repo-config?op=explore) | " +
		"[Edit](https://specscore.studio/app/github.com/specscore/specscore/spec/features/repo-config?op=edit) | " +
		"[Ask question](https://specscore.studio/app/github.com/specscore/specscore/spec/features/repo-config?op=ask) | " +
		"[Request change](https://specscore.studio/app/github.com/specscore/specscore/spec/features/repo-config?op=request-change) |\n"
	if got != want {
		t.Errorf("renderer mismatch.\n got: %q\nwant: %q", got, want)
	}
}

// TestRenderStudioToolbar_MultiDotName covers brand-attribution-multi-dot —
// only the segment after the LAST `.` is bolded.
func TestRenderStudioToolbar_MultiDotName(t *testing.T) {
	got := RenderStudioToolbar(
		"Acme.Internal.Studio",
		"https://acme.internal.studio/",
		"github.com", "bar", "baz",
		"spec/features/foo",
	)
	if !strings.HasPrefix(got, "> [Acme.Internal.**Studio**](https://acme.internal.studio):") {
		t.Errorf("multi-dot brand prefix wrong: %q", got)
	}
}

// TestRenderStudioToolbar_NoDotName covers brand-attribution-no-dot —
// no `**` markers when the name contains no `.`.
func TestRenderStudioToolbar_NoDotName(t *testing.T) {
	got := RenderStudioToolbar(
		"AcmeSpecs",
		"https://specs.acme.internal/",
		"github.com", "bar", "baz",
		"spec/features/foo",
	)
	if !strings.HasPrefix(got, "> [AcmeSpecs](https://specs.acme.internal):") {
		t.Errorf("no-dot brand prefix wrong: %q", got)
	}
	if strings.Contains(got, "**") {
		t.Errorf("no `**` should appear when name has no dot; got: %q", got)
	}
}

// TestRenderStudioToolbar_TrailingSlashStripped covers
// url-grammar-trailing-slash — exactly one `/` between studio URL and
// `app`, never two.
func TestRenderStudioToolbar_TrailingSlashStripped(t *testing.T) {
	got := RenderStudioToolbar(
		"X.Y",
		"https://x.example/",
		"github.com", "bar", "baz",
		"spec/features/foo",
	)
	if strings.Contains(got, "//app") {
		t.Errorf("double slash before /app/: %q", got)
	}
	want := "https://x.example/app/github.com/bar/baz/spec/features/foo?op=edit"
	if !strings.Contains(got, want) {
		t.Errorf("expected canonical edit URL %q in render; got: %q", want, got)
	}
	// The brand-link target must also be stripped to just "https://x.example"
	if !strings.Contains(got, "(https://x.example):") {
		t.Errorf("brand link target should be stripped: %q", got)
	}
}

// TestRenderStudioToolbar_PathPreservesSlashes asserts that the artifact
// path's "/" separators survive escaping (url-grammar-path).
func TestRenderStudioToolbar_PathPreservesSlashes(t *testing.T) {
	got := RenderStudioToolbar(
		"S",
		"https://s.example/",
		"github.com", "o", "r",
		"spec/features/a-b_c.d/nested",
	)
	want := "https://s.example/app/github.com/o/r/spec/features/a-b_c.d/nested?op=explore"
	if !strings.Contains(got, want) {
		t.Errorf("path with `-`, `_`, `.`, `/` should survive escaping intact; got: %q", got)
	}
}

// setupStudioProject creates a project root with explicit project
// host/org/repo (so the tests don't depend on git origin discovery).
func setupStudioProject(t *testing.T, studio *projectdef.StudioConfig) string {
	t.Helper()
	root := t.TempDir()
	cfg := projectdef.SpecConfig{
		Project: &projectdef.ProjectConfig{
			Title: "Test",
			Host:  "github.com",
			Org:   "specscore",
			Repo:  "specscore",
		},
	}
	if studio != nil {
		cfg.Studio = studio
	}
	if err := projectdef.WriteSpecConfig(root, cfg); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "spec", "features"), 0o755); err != nil {
		t.Fatal(err)
	}
	return root
}

// TestStudioToolbarFix_RewritesLegacyLine covers
// studio-toolbar#req:studio-toolbar-autofix-artifact-line.
func TestStudioToolbarFix_RewritesLegacyLine(t *testing.T) {
	root := setupStudioProject(t, nil)
	featureDir := filepath.Join(root, "spec", "features", "studio-toolbar")
	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := "# Feature: Studio Toolbar\n\n" +
		"> [View in SpecStudio](https://specscore.studio/project/features?id=specscore@specscore@github.com&path=spec%2Ffeatures%2Fstudio-toolbar) — graph, discussions, approvals\n" +
		"\n**Status:** Approved\n"
	readme := filepath.Join(featureDir, "README.md")
	if err := os.WriteFile(readme, []byte(legacy), 0o644); err != nil {
		t.Fatal(err)
	}
	c := newStudioToolbarChecker().(*studioToolbarChecker)
	if err := c.fix(filepath.Join(root, "spec")); err != nil {
		t.Fatalf("fix: %v", err)
	}
	data, _ := os.ReadFile(readme)
	lines := strings.Split(string(data), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected >= 3 lines after fix; got %q", data)
	}
	wantLine := strings.TrimRight(RenderStudioToolbar(
		"SpecScore.Studio", "https://specscore.studio/",
		"github.com", "specscore", "specscore",
		"spec/features/studio-toolbar"), "\n")
	if lines[2] != wantLine {
		t.Errorf("line 3 not rewritten to canonical form.\n got: %q\nwant: %q", lines[2], wantLine)
	}
	// Lines 1, 2, 4, 5 untouched.
	if lines[0] != "# Feature: Studio Toolbar" {
		t.Errorf("title line mutated: %q", lines[0])
	}
	if lines[1] != "" {
		t.Errorf("blank separator line mutated: %q", lines[1])
	}
	if lines[3] != "" {
		t.Errorf("line 4 (blank after toolbar) mutated: %q", lines[3])
	}
	if lines[4] != "**Status:** Approved" {
		t.Errorf("status line mutated: %q", lines[4])
	}
	// Idempotency.
	before, _ := os.ReadFile(readme)
	if err := c.fix(filepath.Join(root, "spec")); err != nil {
		t.Fatalf("fix pass 2: %v", err)
	}
	after, _ := os.ReadFile(readme)
	if string(before) != string(after) {
		t.Errorf("autofix not idempotent.\nbefore: %q\nafter:  %q", before, after)
	}
}

// TestStudioToolbarFix_InsertsWhenMissing covers the "fewer than 3 lines"
// edge case of studio-toolbar-autofix-artifact-line.
func TestStudioToolbarFix_InsertsWhenMissing(t *testing.T) {
	root := setupStudioProject(t, nil)
	featureDir := filepath.Join(root, "spec", "features", "foo")
	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	readme := filepath.Join(featureDir, "README.md")
	if err := os.WriteFile(readme, []byte("# Feature: Foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	c := newStudioToolbarChecker().(*studioToolbarChecker)
	if err := c.fix(filepath.Join(root, "spec")); err != nil {
		t.Fatalf("fix: %v", err)
	}
	data, _ := os.ReadFile(readme)
	lines := strings.Split(string(data), "\n")
	if len(lines) < 3 {
		t.Fatalf("expected file padded to >= 3 lines; got: %q", data)
	}
	if lines[0] != "# Feature: Foo" {
		t.Errorf("title line mutated: %q", lines[0])
	}
	wantLine := strings.TrimRight(RenderStudioToolbar(
		"SpecScore.Studio", "https://specscore.studio/",
		"github.com", "specscore", "specscore",
		"spec/features/foo"), "\n")
	if lines[2] != wantLine {
		t.Errorf("toolbar not inserted at line 3.\n got: %q\nwant: %q", lines[2], wantLine)
	}
}

// TestStudioToolbarFix_OptOutRemovesToolbar covers
// studio-toolbar#req:studio-toolbar-opt-out — when studio: null, --fix
// strips any pre-existing toolbar at line 3.
func TestStudioToolbarFix_OptOutRemovesToolbar(t *testing.T) {
	// Build a project with studio: null. We do it by writing raw YAML
	// because StudioConfig{} can't represent the null state through the
	// struct.
	root := t.TempDir()
	body := projectdef.SchemaHeader + "\nproject:\n  host: github.com\n  org: o\n  repo: r\nstudio: null\n"
	if err := os.WriteFile(filepath.Join(root, projectdef.SpecConfigFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "spec", "features", "foo"), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "# Feature: Foo\n\n" +
		"> [SpecScore.**Studio**](https://specscore.studio): | [Explore](x) | [Edit](x) | [Ask question](x) | [Request change](x) |\n" +
		"\n**Status:** Stable\n"
	readme := filepath.Join(root, "spec", "features", "foo", "README.md")
	if err := os.WriteFile(readme, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	c := newStudioToolbarChecker().(*studioToolbarChecker)
	if err := c.fix(filepath.Join(root, "spec")); err != nil {
		t.Fatalf("fix: %v", err)
	}
	got, _ := os.ReadFile(readme)
	lines := strings.Split(string(got), "\n")
	if lines[0] != "# Feature: Foo" {
		t.Errorf("title line mutated: %q", lines[0])
	}
	for _, l := range lines {
		if strings.HasPrefix(l, "> [SpecScore.") {
			t.Errorf("toolbar should have been stripped; still present: %q", got)
		}
	}
}

// TestStudioToolbarFix_BlockedByViewer covers
// studio-toolbar#req:studio-toolbar-autofix-blocked-by-viewer — the
// fixer surfaces the parser's viewer-rejection error and modifies no
// files.
func TestStudioToolbarFix_BlockedByViewer(t *testing.T) {
	root := t.TempDir()
	body := projectdef.SchemaHeader + "\nproject:\n  host: github.com\n  org: o\n  repo: r\nviewer: null\n"
	if err := os.WriteFile(filepath.Join(root, projectdef.SpecConfigFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "spec", "features", "foo"), 0o755); err != nil {
		t.Fatal(err)
	}
	readme := filepath.Join(root, "spec", "features", "foo", "README.md")
	original := "# Feature: Foo\n\n> [whatever](url) — old\n"
	if err := os.WriteFile(readme, []byte(original), 0o644); err != nil {
		t.Fatal(err)
	}
	c := newStudioToolbarChecker().(*studioToolbarChecker)
	err := c.fix(filepath.Join(root, "spec"))
	if err == nil {
		t.Fatal("expected fix to refuse with viewer: present")
	}
	if !strings.Contains(err.Error(), "viewer: block is no longer supported") {
		t.Errorf("error should cite the viewer rejection; got %v", err)
	}
	after, _ := os.ReadFile(readme)
	if string(after) != original {
		t.Errorf("README must not be modified when viewer: blocks the fix.\nbefore: %q\nafter:  %q", original, after)
	}
}

// TestStudioToolbarCheck_FlagsViewerBlock asserts the check path
// surfaces the viewer: rejection as a single rule violation at
// specscore.yaml (studio-toolbar#req:studio-toolbar-lint-no-viewer-backcompat).
func TestStudioToolbarCheck_FlagsViewerBlock(t *testing.T) {
	root := t.TempDir()
	body := projectdef.SchemaHeader + "\nviewer:\n  name: X\n  url: https://x/\n"
	if err := os.WriteFile(filepath.Join(root, projectdef.SpecConfigFile), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "spec", "features"), 0o755); err != nil {
		t.Fatal(err)
	}
	c := newStudioToolbarChecker()
	violations, err := c.check(filepath.Join(root, "spec"))
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(violations), violations)
	}
	if violations[0].Rule != "studio-toolbar" {
		t.Errorf("expected rule=studio-toolbar; got %q", violations[0].Rule)
	}
	if !strings.Contains(violations[0].Message, "viewer: block is no longer supported") {
		t.Errorf("expected viewer-rejection in message; got %q", violations[0].Message)
	}
}

// =============================================================================
// AC tests — studio-toolbar Feature, 14 Acceptance Criteria
// =============================================================================

// TestAC_ToolbarRenderedWithDefaults covers
// AC: toolbar-rendered-with-defaults. With no studio: block in yaml,
// defaults apply and --fix produces the canonical line byte-for-byte
// against the AC's expected string.
func TestAC_ToolbarRenderedWithDefaults(t *testing.T) {
	root := setupDefaultStudioProject(t)
	readme := writeStudioFeatureReadme(t, root, "repo-config",
		"# Feature: Repo Config\n\n**Status:** Approved\n")

	c := newStudioToolbarChecker().(*studioToolbarChecker)
	if err := c.fix(filepath.Join(root, "spec")); err != nil {
		t.Fatalf("fix: %v", err)
	}
	data, _ := os.ReadFile(readme)
	lines := strings.Split(string(data), "\n")
	if len(lines) < 3 {
		t.Fatalf("file too short after fix: %q", data)
	}
	if lines[2] != acDefaultExpectedLine {
		t.Errorf("line 3 not canonical.\n got: %q\nwant: %q", lines[2], acDefaultExpectedLine)
	}
}

// TestAC_BrandAttributionBoldsLastSegment covers
// AC: brand-attribution-bolds-last-segment. With studio.name =
// "Acme.Internal.Studio", only "Studio" is wrapped in **.
func TestAC_BrandAttributionBoldsLastSegment(t *testing.T) {
	studio := &projectdef.StudioConfig{
		Name: "Acme.Internal.Studio",
		URL:  "https://acme.internal.studio/",
	}
	root := setupStudioProjectWithIdentity(t, studio, acHost, acOwner, acRepo)
	readme := writeStudioFeatureReadme(t, root, "foo", "# Feature: Foo\n\n**Status:** Draft\n")
	c := newStudioToolbarChecker().(*studioToolbarChecker)
	if err := c.fix(filepath.Join(root, "spec")); err != nil {
		t.Fatalf("fix: %v", err)
	}
	data, _ := os.ReadFile(readme)
	lines := strings.Split(string(data), "\n")
	want := "[Acme.Internal.**Studio**](https://acme.internal.studio):"
	if !strings.Contains(lines[2], want) {
		t.Errorf("expected brand prefix %q in line 3; got %q", want, lines[2])
	}
}

// TestAC_BrandAttributionNoDotNoBold covers
// AC: brand-attribution-no-dot-no-bold. With studio.name = "AcmeSpecs"
// (no dot), no ** appears anywhere in the line.
func TestAC_BrandAttributionNoDotNoBold(t *testing.T) {
	studio := &projectdef.StudioConfig{
		Name: "AcmeSpecs",
		URL:  "https://specs.acme.internal/",
	}
	root := setupStudioProjectWithIdentity(t, studio, acHost, acOwner, acRepo)
	readme := writeStudioFeatureReadme(t, root, "foo", "# Feature: Foo\n\n**Status:** Draft\n")
	c := newStudioToolbarChecker().(*studioToolbarChecker)
	if err := c.fix(filepath.Join(root, "spec")); err != nil {
		t.Fatalf("fix: %v", err)
	}
	data, _ := os.ReadFile(readme)
	lines := strings.Split(string(data), "\n")
	want := "[AcmeSpecs](https://specs.acme.internal):"
	if !strings.Contains(lines[2], want) {
		t.Errorf("expected brand prefix %q in line 3; got %q", want, lines[2])
	}
	if strings.Contains(lines[2], "**") {
		t.Errorf("no-dot brand attribution must not contain **; got %q", lines[2])
	}
}

// TestAC_UrlGrammarStripsTrailingSlash covers
// AC: url-grammar-strips-trailing-slash. Trailing / on studio.url is
// stripped exactly once before joining with the path grammar.
func TestAC_UrlGrammarStripsTrailingSlash(t *testing.T) {
	studio := &projectdef.StudioConfig{
		Name: "X.Y",
		URL:  "https://x.example/",
	}
	root := setupStudioProjectWithIdentity(t, studio, "github.com", "bar", "baz")
	readme := writeStudioFeatureReadme(t, root, "foo", "# Feature: Foo\n\n**Status:** Draft\n")
	c := newStudioToolbarChecker().(*studioToolbarChecker)
	if err := c.fix(filepath.Join(root, "spec")); err != nil {
		t.Fatalf("fix: %v", err)
	}
	data, _ := os.ReadFile(readme)
	lines := strings.Split(string(data), "\n")
	wantEdit := "[Edit](https://x.example/app/github.com/bar/baz/spec/features/foo?op=edit)"
	if !strings.Contains(lines[2], wantEdit) {
		t.Errorf("expected canonical Edit URL %q in line 3; got %q", wantEdit, lines[2])
	}
	// Also check no double-slash anywhere on line 3 (the // in "https://" is
	// the only legitimate occurrence, and the URL grammar prevents another).
	// We count occurrences of "//" — must be exactly 4 (one per URL: brand
	// link + 4 verbs, but brand has https:// + each verb URL has https://,
	// so 5 total).
	doubles := strings.Count(lines[2], "//")
	if doubles != 5 {
		t.Errorf("expected exactly 5 // occurrences (one per URL); got %d in line %q", doubles, lines[2])
	}
}

// TestAC_OptOutSuppressesToolbar covers AC: opt-out-suppresses-toolbar.
// With studio: null and a feature README that has no toolbar at line 3,
// the rule emits zero violations.
func TestAC_OptOutSuppressesToolbar(t *testing.T) {
	root := setupOptOutProject(t)
	writeStudioFeatureReadme(t, root, "foo",
		"# Feature: Foo\n\n**Status:** Draft\n## Summary\nhello\n")
	c := newStudioToolbarChecker()
	violations, err := c.check(filepath.Join(root, "spec"))
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	st := findStudioToolbarViolations(violations)
	if len(st) != 0 {
		t.Errorf("expected zero studio-toolbar violations under opt-out; got %d: %v", len(st), st)
	}
}

// TestAC_LintErrorsOnByteDrift covers AC: lint-errors-on-byte-drift —
// any byte-level deviation from the canonical form yields exactly one
// error-severity violation at line 3.
func TestAC_LintErrorsOnByteDrift(t *testing.T) {
	// Canonical line we'll mutate to produce drifts.
	canonical := acDefaultExpectedLine

	cases := []struct {
		name string
		// transform mutates the canonical line into a drifted version.
		transform func(string) string
	}{
		{
			name: "separator_pipe_no_spaces",
			transform: func(s string) string {
				// Replace the first " | " with "|".
				return strings.Replace(s, " | ", "|", 1)
			},
		},
		{
			name: "lowercase_explore_label",
			transform: func(s string) string {
				return strings.Replace(s, "[Explore]", "[explore]", 1)
			},
		},
		{
			name: "missing_trailing_pipe",
			transform: func(s string) string {
				return strings.TrimSuffix(s, " |")
			},
		},
		{
			name: "missing_closing_colon_on_brand",
			transform: func(s string) string {
				// Remove the colon right after the brand link's closing ).
				return strings.Replace(s, "studio): |", "studio) |", 1)
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := setupDefaultStudioProject(t)
			drifted := tc.transform(canonical)
			if drifted == canonical {
				t.Fatalf("transform did not change line; bug in test")
			}
			writeStudioFeatureReadme(t, root, "repo-config",
				"# Feature: Repo Config\n\n"+drifted+"\n\n**Status:** Approved\n")
			c := newStudioToolbarChecker()
			violations, err := c.check(filepath.Join(root, "spec"))
			if err != nil {
				t.Fatalf("check: %v", err)
			}
			st := findStudioToolbarViolations(violations)
			if len(st) != 1 {
				t.Fatalf("expected exactly 1 violation, got %d: %v", len(st), st)
			}
			if st[0].Severity != "error" {
				t.Errorf("expected severity=error; got %q", st[0].Severity)
			}
			if st[0].Line != 3 {
				t.Errorf("expected line=3; got %d", st[0].Line)
			}
		})
	}
}

// TestAC_ViewerBlockIsHardError covers AC: viewer-block-is-hard-error.
// Any form of viewer: block in specscore.yaml produces a single error
// violation at specscore.yaml directing the user to rename to studio:.
func TestAC_ViewerBlockIsHardError(t *testing.T) {
	cases := []struct {
		name string
		body string
	}{
		{
			name: "mapping",
			body: "viewer:\n  name: SpecStudio\n  url: https://specscore.studio/\n",
		},
		{
			name: "null",
			body: "viewer: null\n",
		},
		{
			name: "bare",
			body: "viewer:\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := setupViewerStillPresentProject(t, tc.body)
			c := newStudioToolbarChecker()
			violations, err := c.check(filepath.Join(root, "spec"))
			if err != nil {
				t.Fatalf("check: %v", err)
			}
			st := findStudioToolbarViolations(violations)
			if len(st) != 1 {
				t.Fatalf("expected 1 violation, got %d: %v", len(st), st)
			}
			v := st[0]
			if v.Severity != "error" {
				t.Errorf("severity=%q; want error", v.Severity)
			}
			if v.File != projectdef.SpecConfigFile {
				t.Errorf("file=%q; want %s", v.File, projectdef.SpecConfigFile)
			}
			if !strings.Contains(v.Message, "viewer:") {
				t.Errorf("message should mention viewer:; got %q", v.Message)
			}
			if !strings.Contains(v.Message, "studio:") {
				t.Errorf("message should direct rename to studio:; got %q", v.Message)
			}
		})
	}
}

// TestAC_AutofixRewritesLegacyLine covers
// AC: autofix-rewrites-legacy-line. The legacy "View in SpecStudio"
// line is replaced byte-for-byte with the canonical form; all other
// lines are preserved.
func TestAC_AutofixRewritesLegacyLine(t *testing.T) {
	root := setupDefaultStudioProject(t)
	legacy := "> [View in SpecStudio](https://specscore.studio/project/features?id=specscore@specscore@github.com&path=spec%2Ffeatures%2Fstudio-toolbar) — graph, discussions, approvals"
	content := "# Feature: Studio Toolbar\n\n" + legacy + "\n\n**Status:** Approved\n## Summary\nsome text\n"
	readme := writeStudioFeatureReadme(t, root, "studio-toolbar", content)

	c := newStudioToolbarChecker().(*studioToolbarChecker)
	if err := c.fix(filepath.Join(root, "spec")); err != nil {
		t.Fatalf("fix: %v", err)
	}
	data, _ := os.ReadFile(readme)
	lines := strings.Split(string(data), "\n")

	wantLine := strings.TrimRight(RenderStudioToolbar(
		"SpecScore.Studio", "https://specscore.studio/",
		acHost, acOwner, acRepo, "spec/features/studio-toolbar"), "\n")
	if lines[2] != wantLine {
		t.Errorf("line 3 not canonical.\n got: %q\nwant: %q", lines[2], wantLine)
	}
	// Other lines untouched.
	if lines[0] != "# Feature: Studio Toolbar" {
		t.Errorf("line 1 mutated: %q", lines[0])
	}
	if lines[1] != "" {
		t.Errorf("line 2 mutated: %q", lines[1])
	}
	if lines[3] != "" {
		t.Errorf("line 4 mutated: %q", lines[3])
	}
	if lines[4] != "**Status:** Approved" {
		t.Errorf("line 5 mutated: %q", lines[4])
	}
	if lines[5] != "## Summary" {
		t.Errorf("line 6 mutated: %q", lines[5])
	}
	if lines[6] != "some text" {
		t.Errorf("line 7 mutated: %q", lines[6])
	}
}

// TestAC_AutofixBlockedWhenViewerStillPresent covers
// AC: autofix-blocked-when-viewer-still-present. With viewer: present
// in specscore.yaml, --fix modifies nothing and emits the rejection.
func TestAC_AutofixBlockedWhenViewerStillPresent(t *testing.T) {
	root := setupViewerStillPresentProject(t,
		"project:\n  host: "+acHost+"\n  org: "+acOwner+"\n  repo: "+acRepo+"\nviewer:\n  name: SpecStudio\n  url: https://specscore.studio/\n")
	original := "# Feature: Foo\n\n> [whatever](url) — drifted line\n\n**Status:** Draft\n"
	readme := writeStudioFeatureReadme(t, root, "foo", original)

	before, _ := os.ReadFile(readme)
	c := newStudioToolbarChecker().(*studioToolbarChecker)
	err := c.fix(filepath.Join(root, "spec"))
	if err == nil {
		t.Fatal("expected fix to refuse with viewer: present")
	}
	if !strings.Contains(err.Error(), "viewer:") {
		t.Errorf("error should mention viewer:; got %v", err)
	}
	after, _ := os.ReadFile(readme)
	if string(after) != string(before) {
		t.Errorf("README must be byte-identical after blocked fix.\nbefore: %q\nafter:  %q", before, after)
	}
	// Also: check() surfaces the same rejection.
	violations, err := c.check(filepath.Join(root, "spec"))
	if err != nil {
		t.Fatalf("check after blocked fix: %v", err)
	}
	st := findStudioToolbarViolations(violations)
	if len(st) != 1 {
		t.Fatalf("expected 1 violation from check; got %d", len(st))
	}
	if !strings.Contains(st[0].Message, "viewer:") {
		t.Errorf("check violation should cite viewer:; got %q", st[0].Message)
	}
}

// TestAC_ViewLinkRuleRemoved covers AC: view-link-rule-removed.
// ValidateRuleNames(["view-link"]) returns an error naming both the
// removed rule and the studio-toolbar replacement.
func TestAC_ViewLinkRuleRemoved(t *testing.T) {
	err := ValidateRuleNames([]string{"view-link"})
	if err == nil {
		t.Fatal("expected error for view-link rule reference")
	}
	msg := err.Error()
	if !strings.Contains(msg, "view-link") {
		t.Errorf("error should mention view-link; got %q", msg)
	}
	if !strings.Contains(msg, "studio-toolbar") {
		t.Errorf("error should name studio-toolbar as replacement; got %q", msg)
	}
}

// TestAC_OptOutStripsExistingToolbar covers
// AC: opt-out-strips-existing-toolbar. With studio: null and a
// pre-existing canonical toolbar at line 3, --fix removes only line 3.
func TestAC_OptOutStripsExistingToolbar(t *testing.T) {
	root := setupOptOutProject(t)
	toolbar := strings.TrimRight(RenderStudioToolbar(
		"SpecScore.Studio", "https://specscore.studio/",
		acHost, acOwner, acRepo, "spec/features/foo"), "\n")
	content := "# Feature: Foo\n\n" + toolbar + "\n\n**Status:** Stable\n## Summary\nbody\n"
	readme := writeStudioFeatureReadme(t, root, "foo", content)

	c := newStudioToolbarChecker().(*studioToolbarChecker)
	if err := c.fix(filepath.Join(root, "spec")); err != nil {
		t.Fatalf("fix: %v", err)
	}
	data, _ := os.ReadFile(readme)
	lines := strings.Split(string(data), "\n")
	// Title and blank separator preserved.
	if lines[0] != "# Feature: Foo" {
		t.Errorf("line 1 mutated: %q", lines[0])
	}
	if lines[1] != "" {
		t.Errorf("line 2 mutated: %q", lines[1])
	}
	// Original line 4 ("") is now at position 3 (the "blank-after-toolbar"
	// blank line), and the **Status:** line follows.
	if lines[2] != "" {
		t.Errorf("expected blank at new line 3 (toolbar stripped); got %q", lines[2])
	}
	if lines[3] != "**Status:** Stable" {
		t.Errorf("expected **Status:** Stable at line 4; got %q", lines[3])
	}
	// No toolbar line anywhere in the file.
	for _, l := range lines {
		if strings.HasPrefix(l, "> [SpecScore.") {
			t.Errorf("toolbar should be stripped; still present: %q", data)
		}
	}
}

// TestAC_NoSectionOrReqScopeTokens covers
// AC: no-section-or-req-scope-tokens. A toolbar URL with &section=
// or &req= produces an error citing toolbar-file-scope.
func TestAC_NoSectionOrReqScopeTokens(t *testing.T) {
	root := setupDefaultStudioProject(t)
	// Take the canonical line and append a scope token inside the Edit URL.
	drifted := strings.Replace(acDefaultExpectedLine,
		"?op=edit)",
		"?op=edit&section=behavior)", 1)
	writeStudioFeatureReadme(t, root, "repo-config",
		"# Feature: Repo Config\n\n"+drifted+"\n\n**Status:** Approved\n")

	c := newStudioToolbarChecker()
	violations, err := c.check(filepath.Join(root, "spec"))
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	st := findStudioToolbarViolations(violations)
	if len(st) == 0 {
		t.Fatal("expected at least one studio-toolbar violation")
	}
	found := false
	for _, v := range st {
		if v.Severity == "error" && strings.Contains(v.Message, "toolbar-file-scope") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected an error violation citing toolbar-file-scope; got %v", st)
	}
}

// TestAC_NoUtmParameters covers AC: no-utm-parameters. A toolbar URL
// with utm_source=foo (etc.) produces an error citing url-grammar-no-utm.
func TestAC_NoUtmParameters(t *testing.T) {
	root := setupDefaultStudioProject(t)
	drifted := strings.Replace(acDefaultExpectedLine,
		"?op=edit)",
		"?op=edit&utm_source=foo)", 1)
	writeStudioFeatureReadme(t, root, "repo-config",
		"# Feature: Repo Config\n\n"+drifted+"\n\n**Status:** Approved\n")

	c := newStudioToolbarChecker()
	violations, err := c.check(filepath.Join(root, "spec"))
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	st := findStudioToolbarViolations(violations)
	if len(st) == 0 {
		t.Fatal("expected at least one studio-toolbar violation")
	}
	found := false
	for _, v := range st {
		if v.Severity == "error" && strings.Contains(v.Message, "url-grammar-no-utm") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected an error violation citing url-grammar-no-utm; got %v", st)
	}
}

// TestAC_NoBranchOrTagSuffix covers AC: no-branch-or-tag-suffix. A
// toolbar URL containing @branch or ?ref=tag produces an error citing
// url-grammar-no-branch-tag.
func TestAC_NoBranchOrTagSuffix(t *testing.T) {
	cases := []struct {
		name      string
		transform func(string) string
	}{
		{
			name: "at_branch_in_path",
			transform: func(s string) string {
				// Inject @main before the ?op= for the Edit URL.
				return strings.Replace(s,
					"spec/features/repo-config?op=edit",
					"spec/features/repo-config@main?op=edit", 1)
			},
		},
		{
			name: "ref_query_param",
			transform: func(s string) string {
				return strings.Replace(s,
					"?op=edit)",
					"?op=edit&ref=tag)", 1)
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := setupDefaultStudioProject(t)
			drifted := tc.transform(acDefaultExpectedLine)
			if drifted == acDefaultExpectedLine {
				t.Fatalf("transform did not change line; bug in test")
			}
			writeStudioFeatureReadme(t, root, "repo-config",
				"# Feature: Repo Config\n\n"+drifted+"\n\n**Status:** Approved\n")
			c := newStudioToolbarChecker()
			violations, err := c.check(filepath.Join(root, "spec"))
			if err != nil {
				t.Fatalf("check: %v", err)
			}
			st := findStudioToolbarViolations(violations)
			if len(st) == 0 {
				t.Fatal("expected at least one studio-toolbar violation")
			}
			found := false
			for _, v := range st {
				if v.Severity == "error" && strings.Contains(v.Message, "url-grammar-no-branch-tag") {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("expected an error violation citing url-grammar-no-branch-tag; got %v", st)
			}
		})
	}
}
