package lint

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ahEnv writes feature READMEs under a temp spec root for the
// ac-heading-format checker.
type ahEnv struct{ specRoot string }

func newAHEnv(t *testing.T) *ahEnv {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "features"), 0o755); err != nil {
		t.Fatal(err)
	}
	return &ahEnv{specRoot: root}
}

func (e *ahEnv) write(t *testing.T, slug, body string) string {
	t.Helper()
	dir := filepath.Join(e.specRoot, "features", slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	p := filepath.Join(dir, "README.md")
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// ahBody wraps a single AC heading line in a minimal Feature README shell.
func ahBody(slug, acHeadingLine string) string {
	return "# Feature: " + slug + "\n\n**Status:** Approved\n\n## Acceptance Criteria\n\n" +
		acHeadingLine + "\n\n**Given** g **When** w **Then** t\n\n## Open Questions\n\nNone.\n"
}

func TestACHeading_NameSeverity(t *testing.T) {
	c := newACHeadingFormatChecker()
	if c.name() != "ac-heading-format" || c.severity() != "error" {
		t.Fatalf("unexpected name/severity: %s/%s", c.name(), c.severity())
	}
}

func TestACHeading_CanonicalAccepted(t *testing.T) {
	e := newAHEnv(t)
	e.write(t, "ok", ahBody("ok", "### AC: valid-id"))
	e.write(t, "ok2", ahBody("ok2", "### AC: a"))
	e.write(t, "ok3", ahBody("ok3", "### AC: a1-b2-c3"))
	c := newACHeadingFormatChecker()
	vs, err := c.check(e.specRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 0 {
		t.Fatalf("canonical headings should be clean, got %+v", vs)
	}
}

func TestACHeading_MissingSpaceFlagged(t *testing.T) {
	e := newAHEnv(t)
	e.write(t, "nospace", ahBody("nospace", "### AC:x"))
	c := newACHeadingFormatChecker()
	vs, _ := c.check(e.specRoot)
	if hasViolation(vs, "ac-heading-format", "") == nil {
		t.Fatalf("expected a violation for missing space, got %+v", vs)
	}
}

func TestACHeading_MultipleSpacesFlagged(t *testing.T) {
	e := newAHEnv(t)
	e.write(t, "twospace", ahBody("twospace", "### AC:  x"))
	c := newACHeadingFormatChecker()
	vs, _ := c.check(e.specRoot)
	if hasViolation(vs, "ac-heading-format", "") == nil {
		t.Fatalf("expected a violation for multiple spaces, got %+v", vs)
	}
}

func TestACHeading_UppercaseFlagged(t *testing.T) {
	e := newAHEnv(t)
	e.write(t, "upper", ahBody("upper", "### AC: My-Id"))
	c := newACHeadingFormatChecker()
	vs, _ := c.check(e.specRoot)
	v := hasViolation(vs, "ac-heading-format", "not kebab-case")
	if v == nil {
		t.Fatalf("expected a kebab-case violation, got %+v", vs)
	}
}

func TestACHeading_UnderscoreFlagged(t *testing.T) {
	e := newAHEnv(t)
	e.write(t, "under", ahBody("under", "### AC: my_id"))
	c := newACHeadingFormatChecker()
	vs, _ := c.check(e.specRoot)
	if hasViolation(vs, "ac-heading-format", "not kebab-case") == nil {
		t.Fatalf("expected a kebab-case violation, got %+v", vs)
	}
}

func TestACHeading_TrailingTextFlagged(t *testing.T) {
	e := newAHEnv(t)
	e.write(t, "trailing", ahBody("trailing", "### AC: valid-id extra words"))
	c := newACHeadingFormatChecker()
	vs, _ := c.check(e.specRoot)
	if hasViolation(vs, "ac-heading-format", "trailing content") == nil {
		t.Fatalf("expected a trailing-content violation, got %+v", vs)
	}
}

func TestACHeading_VerifiesParentheticalAccepted(t *testing.T) {
	// The `(verifies REQ:...)` annotation is a pre-existing, widely-used
	// convention (this repo's own spec tree and the upstream specscore
	// meta-spec repo both carry it) that predates and is orthogonal to the
	// Chatwright/Sneat `### AC: <id>` convention this rule enforces. A
	// canonically-spaced heading carrying it must not be flagged.
	e := newAHEnv(t)
	e.write(t, "verifies", ahBody("verifies", "### AC: valid-id (verifies REQ:something)"))
	e.write(t, "verifies2", ahBody("verifies2", "### AC: valid-id (verifies REQ:a, REQ:b)"))
	c := newACHeadingFormatChecker()
	vs, err := c.check(e.specRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 0 {
		t.Fatalf("canonically-spaced (verifies ...) headings must be accepted, got %+v", vs)
	}
}

func TestACHeading_VerifiesParentheticalBadWhitespaceFlagged(t *testing.T) {
	e := newAHEnv(t)
	e.write(t, "verifies", ahBody("verifies", "### AC: valid-id  (verifies REQ:x)"))
	c := newACHeadingFormatChecker()
	vs, _ := c.check(e.specRoot)
	if hasViolation(vs, "ac-heading-format", "does not match the canonical") == nil {
		t.Fatalf("expected a whitespace violation for badly-spaced (verifies ...) heading, got %+v", vs)
	}
}

func TestACHeading_NonVerifiesTrailingTextStillFlagged(t *testing.T) {
	e := newAHEnv(t)
	e.write(t, "othertrailing", ahBody("othertrailing", "### AC: valid-id (some other note)"))
	c := newACHeadingFormatChecker()
	vs, _ := c.check(e.specRoot)
	if hasViolation(vs, "ac-heading-format", "trailing content") == nil {
		t.Fatalf("expected non-verifies trailing content to be flagged, got %+v", vs)
	}
}

func TestACHeading_MissingIDFlagged(t *testing.T) {
	e := newAHEnv(t)
	e.write(t, "noid", ahBody("noid", "### AC:"))
	c := newACHeadingFormatChecker()
	vs, _ := c.check(e.specRoot)
	if hasViolation(vs, "ac-heading-format", "missing an id") == nil {
		t.Fatalf("expected a missing-id violation, got %+v", vs)
	}
}

func TestACHeading_ExtraSpaceBeforeACFlagged(t *testing.T) {
	e := newAHEnv(t)
	e.write(t, "prefix", ahBody("prefix", "###  AC: valid-id"))
	c := newACHeadingFormatChecker()
	vs, _ := c.check(e.specRoot)
	if hasViolation(vs, "ac-heading-format", "") == nil {
		t.Fatalf("expected a violation for extra whitespace before AC, got %+v", vs)
	}
}

func TestACHeading_NonFeatureSkipped(t *testing.T) {
	e := newAHEnv(t)
	e.write(t, "notafeature", "# Notes\n\n### AC:x\n")
	c := newACHeadingFormatChecker()
	vs, _ := c.check(e.specRoot)
	if len(vs) != 0 {
		t.Fatalf("non-feature README must be skipped, got %+v", vs)
	}
}

func TestACHeading_UnrelatedHeadingIgnored(t *testing.T) {
	e := newAHEnv(t)
	e.write(t, "unrelated", ahBody("unrelated", "### Not An AC Heading"))
	c := newACHeadingFormatChecker()
	vs, _ := c.check(e.specRoot)
	if len(vs) != 0 {
		t.Fatalf("headings that don't match the AC trigger must be ignored, got %+v", vs)
	}
}

func TestACHeading_TwoViolationsSortedByFileThenLine(t *testing.T) {
	e := newAHEnv(t)
	e.write(t, "bbb", ahBody("bbb", "### AC:x"))
	e.write(t, "aaa", ahBody("aaa", "### AC:y"))
	c := newACHeadingFormatChecker()
	vs, _ := c.check(e.specRoot)
	if len(vs) != 2 {
		t.Fatalf("expected 2 violations, got %d: %+v", len(vs), vs)
	}
	if vs[0].File >= vs[1].File {
		t.Fatalf("violations not sorted by file: %s, %s", vs[0].File, vs[1].File)
	}
}

func TestACHeading_TwoViolationsInOneFileSortByLine(t *testing.T) {
	e := newAHEnv(t)
	e.write(t, "same", ahBody("same", "### AC:x")+"\n### AC:y\n")
	vs, err := newACHeadingFormatChecker().check(e.specRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 2 || vs[0].Line >= vs[1].Line {
		t.Fatalf("violations = %+v, want same-file line order", vs)
	}
}

func TestACHeading_CheckPropagatesWalkerError(t *testing.T) {
	original := walkACFeatureReadmes
	walkACFeatureReadmes = func(string, func(string, []byte)) error { return errors.New("walk boom") }
	t.Cleanup(func() { walkACFeatureReadmes = original })
	if _, err := newACHeadingFormatChecker().check(t.TempDir()); err == nil {
		t.Fatal("check must propagate walker error")
	}
}

// --- fix ---

func TestACHeadingFix_NormalizesWhitespaceOnly(t *testing.T) {
	e := newAHEnv(t)
	nospace := e.write(t, "nospace", ahBody("nospace", "### AC:x"))
	twospace := e.write(t, "twospace", ahBody("twospace", "### AC:  y"))
	prefix := e.write(t, "prefix", ahBody("prefix", "###  AC: z"))

	c := newACHeadingFormatChecker()
	if err := c.fix(e.specRoot); err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		path, want string
	}{
		{nospace, "### AC: x"},
		{twospace, "### AC: y"},
		{prefix, "### AC: z"},
	} {
		got, _ := os.ReadFile(tc.path)
		if !strings.Contains(string(got), tc.want+"\n") {
			t.Fatalf("expected fixed heading %q in %s, got %q", tc.want, tc.path, got)
		}
	}

	// Idempotent + clean afterwards.
	if err := c.fix(e.specRoot); err != nil {
		t.Fatal(err)
	}
	vs, _ := c.check(e.specRoot)
	if len(vs) != 0 {
		t.Fatalf("after fix, check should be clean, got %+v", vs)
	}
}

func TestACHeadingFix_LeavesNonKebabAndTrailingTextUntouched(t *testing.T) {
	e := newAHEnv(t)
	upper := e.write(t, "upper", ahBody("upper", "### AC: My_Id"))
	trailing := e.write(t, "trailing", ahBody("trailing", "### AC: valid-id (some other note)"))
	upperBefore, _ := os.ReadFile(upper)
	trailingBefore, _ := os.ReadFile(trailing)

	c := newACHeadingFormatChecker()
	if err := c.fix(e.specRoot); err != nil {
		t.Fatal(err)
	}

	upperAfter, _ := os.ReadFile(upper)
	trailingAfter, _ := os.ReadFile(trailing)
	if string(upperBefore) != string(upperAfter) {
		t.Fatalf("non-kebab id must not be auto-fixed, got %q", upperAfter)
	}
	if string(trailingBefore) != string(trailingAfter) {
		t.Fatalf("non-verifies trailing content must not be auto-fixed, got %q", trailingAfter)
	}

	vs, _ := c.check(e.specRoot)
	if len(vs) != 2 {
		t.Fatalf("both non-fixable violations should remain after fix, got %+v", vs)
	}
}

func TestACHeadingFix_NormalizesVerifiesParentheticalWhitespace(t *testing.T) {
	e := newAHEnv(t)
	p := e.write(t, "verifies", ahBody("verifies", "### AC:  valid-id  (verifies REQ:x)"))

	c := newACHeadingFormatChecker()
	if err := c.fix(e.specRoot); err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(p)
	if !strings.Contains(string(got), "### AC: valid-id (verifies REQ:x)\n") {
		t.Fatalf("expected normalized (verifies ...) heading, got %q", got)
	}
	vs, _ := c.check(e.specRoot)
	if len(vs) != 0 {
		t.Fatalf("after fix, check should be clean, got %+v", vs)
	}
}

func TestACHeadingFix_SkipsNonFeature(t *testing.T) {
	e := newAHEnv(t)
	nf := e.write(t, "notfeature", "# Notes\n\n### AC:x\n")
	before, _ := os.ReadFile(nf)

	c := newACHeadingFormatChecker()
	if err := c.fix(e.specRoot); err != nil {
		t.Fatal(err)
	}

	after, _ := os.ReadFile(nf)
	if string(before) != string(after) {
		t.Fatal("fix must not touch non-feature READMEs")
	}
}

func TestACHeadingFix_NoOpWhenAlreadyClean(t *testing.T) {
	e := newAHEnv(t)
	p := e.write(t, "clean", ahBody("clean", "### AC: valid-id"))
	before, _ := os.ReadFile(p)

	c := newACHeadingFormatChecker()
	if err := c.fix(e.specRoot); err != nil {
		t.Fatal(err)
	}

	after, _ := os.ReadFile(p)
	if string(before) != string(after) {
		t.Fatal("fix must not modify an already-clean file")
	}
}

// --- classifyACHeading unit coverage ---

func TestClassifyACHeading(t *testing.T) {
	cases := []struct {
		line          string
		wantFixable   bool
		wantSubstr    string
		wantCanonical string
	}{
		{"### AC:x", true, "does not match the canonical", "### AC: x"},
		{"### AC:  x", true, "does not match the canonical", "### AC: x"},
		{"###  AC: x", true, "does not match the canonical", "### AC: x"},
		{"### AC: My_Id", false, "not kebab-case", ""},
		{"### AC: valid-id extra", false, "trailing content", ""},
		{"### AC:", false, "missing an id", ""},
		{"### AC: ", false, "missing an id", ""},
		{"### AC:  id  (verifies REQ:x)", true, "does not match the canonical", "### AC: id (verifies REQ:x)"},
		{"### AC: id (some other note)", false, "trailing content", ""},
	}
	for _, tc := range cases {
		msg, fixable, canonical := classifyACHeading(tc.line)
		if fixable != tc.wantFixable {
			t.Errorf("classifyACHeading(%q) fixable = %v, want %v (msg: %s)", tc.line, fixable, tc.wantFixable, msg)
		}
		if !strings.Contains(msg, tc.wantSubstr) {
			t.Errorf("classifyACHeading(%q) message = %q, want substring %q", tc.line, msg, tc.wantSubstr)
		}
		if canonical != tc.wantCanonical {
			t.Errorf("classifyACHeading(%q) canonical = %q, want %q", tc.line, canonical, tc.wantCanonical)
		}
	}
}
