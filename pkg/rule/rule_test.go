package rule

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ----- shared fixtures -----

func readGolden(t *testing.T, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("reading golden %s: %v", name, err)
	}
	return string(b)
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return b
}

// setupRulesTree materializes a project root with a lint-clean rules index.
func setupRulesTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := EnsureIndex(RulesDir(root)); err != nil {
		t.Fatalf("EnsureIndex: %v", err)
	}
	return root
}

func readIndexFile(t *testing.T, root string) string {
	t.Helper()
	return string(mustRead(t, IndexPath(RulesDir(root))))
}

// writeDetail writes body to spec/rules/<slug>/README.md and returns the path.
func writeDetail(t *testing.T, root, slug, body string) string {
	t.Helper()
	dir := filepath.Join(RulesDir(root), slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "README.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	return path
}

func scaffoldInto(t *testing.T, root string, opts Options) string {
	t.Helper()
	body, err := ScaffoldDetail(opts)
	if err != nil {
		t.Fatalf("ScaffoldDetail: %v", err)
	}
	return writeDetail(t, root, opts.Slug, string(body))
}

// ----- vocabularies and helpers -----

func TestValidateSlug(t *testing.T) {
	cases := []struct {
		slug string
		ok   bool
	}{
		{"a", true}, {"a-b", true}, {"a1-b2", true}, {"123", true},
		{"", false}, {"A", false}, {"a_b", false}, {"a/b", false},
		{"-a", false}, {"a-", false}, {"a--b", false}, {"a b", false},
	}
	for _, tc := range cases {
		if err := ValidateSlug(tc.slug); tc.ok != (err == nil) {
			t.Errorf("ValidateSlug(%q) error = %v, want ok=%v", tc.slug, err, tc.ok)
		}
	}
}

func TestStatusAndEnforcementVocabularies(t *testing.T) {
	for _, s := range Statuses {
		if !IsStatus(s) {
			t.Errorf("IsStatus(%q) = false", s)
		}
		if got, ok := ParseStatus(strings.ToLower(s)); !ok || got != s {
			t.Errorf("ParseStatus(%q) = (%q, %v)", strings.ToLower(s), got, ok)
		}
	}
	if IsStatus("Pending") {
		t.Error("IsStatus(Pending) = true")
	}
	if _, ok := ParseStatus("pending"); ok {
		t.Error("ParseStatus(pending) accepted")
	}
	for _, e := range EnforcementTiers {
		if !IsEnforcement(e) {
			t.Errorf("IsEnforcement(%q) = false", e)
		}
		if got, ok := ParseEnforcement(strings.ToUpper(e)); !ok || got != e {
			t.Errorf("ParseEnforcement(%q) = (%q, %v)", strings.ToUpper(e), got, ok)
		}
	}
	if IsEnforcement("Mandatory") {
		t.Error("IsEnforcement(Mandatory) = true")
	}
	if _, ok := ParseEnforcement("mandatory"); ok {
		t.Error("ParseEnforcement(mandatory) accepted")
	}
	// Stated is exactly the tier that needs no control; that is what the tier
	// means, so requiring one would collapse the ladder to two rungs.
	if RequiresControl("Stated") {
		t.Error("Stated must not require a control")
	}
	if !RequiresControl("Enforced") || !RequiresControl("Automated") {
		t.Error("Enforced and Automated must require a control")
	}
	if StatusList() == "" || EnforcementList() == "" {
		t.Error("vocabulary lists must render for error messages")
	}
}

func TestTitleCaseFromSlug(t *testing.T) {
	cases := []struct{ slug, want string }{
		{"never-mock-backends", "Never Mock Backends"},
		{"x", "X"},
		{"a--b", "A  B"},
		{"", ""},
	}
	for _, tc := range cases {
		if got := TitleCaseFromSlug(tc.slug); got != tc.want {
			t.Errorf("TitleCaseFromSlug(%q) = %q, want %q", tc.slug, got, tc.want)
		}
	}
}

func TestValueHelpers(t *testing.T) {
	for _, v := range []string{"", " ", Sentinel, "-"} {
		if isRealValue(v) {
			t.Errorf("isRealValue(%q) = true", v)
		}
	}
	if !isRealValue("x") {
		t.Error("isRealValue(x) = false")
	}
	if got := collapseWhitespace(" a\n b\t c "); got != "a b c" {
		t.Errorf("collapseWhitespace = %q", got)
	}
	if got := splitList(Sentinel); got != nil {
		t.Errorf("splitList(sentinel) = %v", got)
	}
	if got := splitList("a, ,b"); len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("splitList = %v", got)
	}
	if got := joinOrSentinel(nil); got != Sentinel {
		t.Errorf("joinOrSentinel(nil) = %q", got)
	}
	if got := joinOrSentinel([]string{" ", "a", "b"}); got != "a, b" {
		t.Errorf("joinOrSentinel = %q", got)
	}
	if got := sentinelOr("  "); got != Sentinel {
		t.Errorf("sentinelOr(blank) = %q", got)
	}
	if got := sentinelOr(" x "); got != "x" {
		t.Errorf("sentinelOr = %q", got)
	}
}

// ----- ScaffoldDetail -----

// AC: scaffold-needs-only-a-slug — the friction argument for the kind rests on
// `rule new <slug>` alone producing a complete, lint-clean artifact.
func TestScaffoldDetailMinimalMatchesGolden(t *testing.T) {
	got, err := ScaffoldDetail(Options{Slug: "never-mock-backends", Date: "2026-09-03"})
	if err != nil {
		t.Fatalf("ScaffoldDetail: %v", err)
	}
	if want := readGolden(t, "detail_minimal.golden"); string(got) != want {
		t.Fatalf("scaffold does not match golden.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestScaffoldDetailFullMatchesGolden(t *testing.T) {
	got, err := ScaffoldDetail(Options{
		Slug: "format-go-before-building", Title: "Format Go Before Building",
		Owner: "alex", Date: "2026-09-03", Status: "active",
		Statement:   "Always run gofmt on every changed Go file\n   before any build, lint, or test command.",
		Scopes:      []string{"fleet", "path:**/*.go"},
		Enforcement: "enforced", Control: "wb pre-commit hook profile go-standard",
		Sources:      []string{"lesson:kinder-fake-hides-bug", "decision:0012"},
		Why:          "An unformatted file reddens CI on a cosmetic diff and hides the real failure underneath it.",
		Exceptions:   "Generated files under gen/, by the owner of the generator only.",
		Supersedes:   "older-gofmt-rule",
		Instructions: "Run `gofmt -l .`; if it lists anything, run `gofmt -w` on those files before continuing.",
		Compliant:    "```sh\ngofmt -w ./pkg/rule && go build ./...\n```",
		Violation:    "```sh\ngo build ./... # with an unformatted file still staged\n```",
		Skills:       []string{"go-hygiene"},
	})
	if err != nil {
		t.Fatalf("ScaffoldDetail: %v", err)
	}
	if want := readGolden(t, "detail_full.golden"); string(got) != want {
		t.Fatalf("scaffold does not match golden.\n--- got ---\n%s\n--- want ---\n%s", got, want)
	}
}

func TestScaffoldDetailDefaults(t *testing.T) {
	got, err := ScaffoldDetail(Options{Slug: "a-b-c"})
	if err != nil {
		t.Fatalf("ScaffoldDetail: %v", err)
	}
	text := string(got)
	today := time.Now().UTC().Format("2006-01-02")
	for _, want := range []string{
		"# Rule: A B C\n", "**Status:** Draft\n", "**Date:** " + today + "\n",
		"**Owner:** unknown\n", "**Scope:** fleet\n", "**Enforcement:** Stated\n",
		"**Control:** " + Sentinel + "\n", "**Sources:** " + Sentinel + "\n",
		"**Exceptions:** none\n", "## Instructions\n", "### Compliant\n", "### Violation\n",
		"## Open Questions\n", "*This document follows the " + FormatURL + "*\n",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("scaffold missing %q:\n%s", want, text)
		}
	}
}

func TestScaffoldDetailRejects(t *testing.T) {
	cases := []struct {
		name string
		opts Options
	}{
		{name: "empty slug", opts: Options{}},
		{name: "invalid slug", opts: Options{Slug: "Not A Slug"}},
		{name: "slug with slash", opts: Options{Slug: "a/b"}},
		{name: "unknown status", opts: Options{Slug: "x", Status: "Pending"}},
		{name: "unknown enforcement", opts: Options{Slug: "x", Enforcement: "Mandatory"}},
		{name: "invalid scope", opts: Options{Slug: "x", Scopes: []string{"team:platform"}}},
		{name: "invalid source", opts: Options{Slug: "x", Sources: []string{"bogus"}}},
		{name: "enforced without control", opts: Options{Slug: "x", Enforcement: "Enforced"}},
		{name: "automated without control", opts: Options{Slug: "x", Enforcement: "Automated"}},
		{name: "enforced with sentinel control", opts: Options{Slug: "x", Enforcement: "Enforced", Control: Sentinel}},
		{name: "invalid skill name", opts: Options{Slug: "x", Skills: []string{"Not A Skill"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := ScaffoldDetail(tc.opts); err == nil {
				t.Fatalf("ScaffoldDetail(%+v) = nil error, want rejection", tc.opts)
			}
		})
	}
}

// A Statement supplied across several lines must still write exactly one bold
// field, or the document would parse as a field followed by loose prose.
func TestScaffoldDetailCollapsesMultilineValues(t *testing.T) {
	got, err := ScaffoldDetail(Options{Slug: "x", Statement: "one\ntwo\n\nthree", Date: "2026-01-01"})
	if err != nil {
		t.Fatalf("ScaffoldDetail: %v", err)
	}
	if !strings.Contains(string(got), "**Statement:** one two three\n") {
		t.Fatalf("multiline statement was not collapsed:\n%s", got)
	}
}

func TestOptionsRowProjection(t *testing.T) {
	opts := Options{
		Slug: "x", Date: "2026-09-03", Statement: "Always x.",
		Scopes: []string{"fleet"}, Sources: []string{"lesson:a"},
	}
	if err := opts.Normalize(); err != nil {
		t.Fatalf("Normalize: %v", err)
	}
	inline := opts.Row(false)
	if inline.Linked || inline.Slug != "x" || inline.Statement != "Always x." {
		t.Fatalf("inline row = %+v", inline)
	}
	if got := inline.Render(); !strings.HasPrefix(got, "| x | Draft | fleet | Stated | — | lesson:a | Always x. |") {
		t.Fatalf("inline render = %q", got)
	}
	detailed := opts.Row(true)
	if !detailed.Linked || !strings.Contains(detailed.Render(), "[x](x/README.md)") {
		t.Fatalf("detailed render = %q", detailed.Render())
	}
}
