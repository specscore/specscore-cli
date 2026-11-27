package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Tests for the opt-in feature-ac-test-coverage rule
// (cli/spec/lint/ac-test-coverage).

const acOptIn = "project:\n  title: T\n  host: github.com\n  org: acme\n  repo: orders\nac_test_coverage:\n  required: true\n"

// acFeatureDoc is a Feature README with the given status and one AC block per
// entry of acs (each entry is the AC id followed by optional body lines).
func acFeatureDoc(status string, acs ...string) string {
	var b strings.Builder
	b.WriteString("---\nformat: https://specscore.md/feature-specification\nstatus: " + status + "\n---\n\n")
	b.WriteString("# Feature: Orders\n\n**Status:** " + status + "\n**Source Ideas:** —\n\n## Summary\n\nx\n\n")
	b.WriteString("## Acceptance Criteria\n\n")
	for _, ac := range acs {
		id, body, _ := strings.Cut(ac, "\n")
		b.WriteString("### AC: " + id + "\n\n**Requirements:** orders#req:x\n\nCondition.\n")
		if body != "" {
			b.WriteString("\n" + body + "\n")
		}
		b.WriteString("\n")
	}
	b.WriteString("## Open Questions\n\nNone at this time.\n\n---\n*This document follows the https://specscore.md/feature-specification*\n")
	return b.String()
}

// writeRootFile writes a file relative to the project root (the parent of
// specRoot).
func writeRootFile(t *testing.T, specRoot, rel, content string) string {
	t.Helper()
	return writeMD(t, filepath.Dir(specRoot), rel, content)
}

func acCheck(t *testing.T, specRoot string, explicit bool) []Violation {
	t.Helper()
	vs, err := newACTestCoverageChecker("", "", explicit).check(specRoot)
	if err != nil {
		t.Fatalf("check error: %v", err)
	}
	return vs
}

// acReported returns the AC keys reported, in order.
func acReported(vs []Violation) []string {
	var out []string
	for _, v := range vs {
		start := strings.Index(v.Message, "`")
		end := strings.Index(v.Message[start+1:], "`")
		out = append(out, v.Message[start+1:start+1+end])
	}
	return out
}

func acWant(t *testing.T, vs []Violation, want ...string) {
	t.Helper()
	got := acReported(vs)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("reported %v, want %v (%+v)", got, want, vs)
	}
}

// AC: off-without-opt-in
// specscore:verifies feature/cli/spec/lint/ac-test-coverage#ac:off-without-opt-in
func TestACTestCoverage_OffWithoutOptIn(t *testing.T) {
	for name, cfg := range map[string]string{
		"no block":       "project:\n  title: T\n",
		"required false": "project:\n  title: T\nac_test_coverage:\n  required: false\n",
		"unreadable":     "",
	} {
		t.Run(name, func(t *testing.T) {
			sr := gradeSpecRoot(t, cfg)
			writeMD(t, sr, "features/orders/README.md", acFeatureDoc("Stable", "total"))
			vs, err := Lint(Options{SpecRoot: sr, Severity: "info"})
			if err != nil {
				t.Fatal(err)
			}
			for _, v := range vs {
				if v.Rule == acTestCoverageRule {
					t.Fatalf("default run must not report %s without opt-in: %+v", v.Rule, v)
				}
			}
			vs, err = Lint(Options{SpecRoot: sr, Rules: []string{acTestCoverageRule}})
			if err != nil {
				t.Fatal(err)
			}
			acWant(t, vs, "orders#ac:total")
		})
	}
}

func TestACTestCoverage_RegisteredInCatalog(t *testing.T) {
	var found bool
	for _, r := range AllRules() {
		if r.ID == acTestCoverageRule {
			found = r.Family == "feature" && r.Severity == "error"
		}
	}
	if !found || !AllRuleNames()[acTestCoverageRule] {
		t.Fatal("feature-ac-test-coverage must be registered as a feature-family error rule")
	}
	c := newACTestCoverageChecker("", "", false)
	if c.name() != acTestCoverageRule || c.severity() != "error" {
		t.Fatalf("checker name/severity = %s/%s", c.name(), c.severity())
	}
}

// AC: malformed-config-visible
// specscore:verifies feature/cli/spec/lint/ac-test-coverage#ac:malformed-config-visible
func TestACTestCoverage_MalformedConfigVisible(t *testing.T) {
	for cfg, severity := range map[string]string{
		"project:\n  title: T\nac_test_coverage:\n  required: \"yes\"\n":       "error",
		"project:\n  title: T\nac_test_coverage:\n  required: false\n  x: 1\n": "warning",
	} {
		sr := gradeSpecRoot(t, cfg)
		writeMD(t, sr, "features/orders/README.md", acFeatureDoc("Stable", "total"))
		vs := acCheck(t, sr, false)
		if len(vs) != 1 || vs[0].File != "specscore.yaml" || vs[0].Severity != severity || !strings.Contains(vs[0].Message, "invalid `ac_test_coverage` block") {
			t.Fatalf("config %q: got %+v, want one %s citing specscore.yaml", cfg, vs, severity)
		}
	}
}

// AC: only-implementing-and-stable
// specscore:verifies feature/cli/spec/lint/ac-test-coverage#ac:only-implementing-and-stable
func TestACTestCoverage_OnlyImplementingAndStable(t *testing.T) {
	sr := gradeSpecRoot(t, acOptIn)
	for _, st := range []string{"Draft", "Approved", "Implementing", "Stable", "Deprecated"} {
		writeMD(t, sr, "features/"+strings.ToLower(st)+"/README.md", acFeatureDoc(st, "one"))
	}
	fenced := strings.Replace(acFeatureDoc("stable", "real"), "## Open Questions",
		"```md\n### AC: fenced\n```\n\n<!--\n### AC: commented\n-->\n\n### Notes\n\n**Deferred:** not an AC block\n\n## Open Questions", 1)
	writeMD(t, sr, "features/fenced/README.md", fenced)
	writeMD(t, sr, "features/plain/README.md", "# Not a feature\n\n**Status:** Stable\n\n## Acceptance Criteria\n\n### AC: x\n")
	writeMD(t, sr, "features/empty/README.md", "# Feature: Empty\n\n**Status:** Stable\n\n## Acceptance Criteria\n\nNot defined yet.\n")
	acWant(t, acCheck(t, sr, false), "fenced#ac:real", "implementing#ac:one", "stable#ac:one")
}

func TestACTestCoverage_NoInScopeFeaturesSkipsTestScan(t *testing.T) {
	sr := gradeSpecRoot(t, acOptIn)
	writeMD(t, sr, "features/orders/README.md", acFeatureDoc("Draft", "one"))
	// An unreadable directory would fail the test-file scan; with nothing in
	// scope the scan never runs.
	locked := filepath.Join(filepath.Dir(sr), "locked")
	if err := os.MkdirAll(locked, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
	if vs := acCheck(t, sr, false); len(vs) != 0 {
		t.Fatalf("got %+v", vs)
	}
}

// AC: test-annotation-covers
// specscore:verifies feature/cli/spec/lint/ac-test-coverage#ac:test-annotation-covers
func TestACTestCoverage_TestAnnotationCovers(t *testing.T) {
	sr := gradeSpecRoot(t, acOptIn)
	writeMD(t, sr, "features/orders/README.md", acFeatureDoc("Stable", "a", "b", "c", "d", "e", "f", "g", "h", "i", "j", "k"))
	writeRootFile(t, sr, "orders_test.go", "package orders\n\n// specscore:verifies feature/orders#ac:a\n")
	writeRootFile(t, sr, "tests/test_orders.py", "# specscore:feature/orders#AC:b\n")
	writeRootFile(t, sr, "web/orders.spec.ts", "// specscore:verifies https://specscore.org/github.com/acme/orders/spec/features/orders#ac:c\n"+
		"// specscore:verifies https://specscore.org/github.com/other/repo/spec/features/orders#ac:d\n")
	writeRootFile(t, sr, "orders.go", "// specscore:verifies feature/orders#ac:e\n")
	writeRootFile(t, sr, "x_test.go", "// specscore:implements feature/orders#ac:f\n// specscore:verifies plan/orders#ac:g\n// specscore:plan/orders#ac:g\n// specscore:verifies feature/orders#req:g\n"+
		"// specscore:feature/orders#ac:\n// specscore:feature/orders#g\n// specscore:bogus feature/orders#ac:g\n")
	// Bare canonical URL, cross-repo authority form, and typed references.
	writeRootFile(t, sr, "forms_test.go", "// https://specscore.org/github.com/acme/orders/spec/features/orders#ac:h\n"+
		"// specscore:verifies specscore://github.com/acme/orders/feature/orders#ac:i\n"+
		"// specscore:references feature/orders#ac:j\n"+
		"// specscore:verifies specscore://github.com/other/repo/feature/orders#ac:k\n")
	acWant(t, acCheck(t, sr, false), "orders#ac:d", "orders#ac:e", "orders#ac:f", "orders#ac:g", "orders#ac:k")
}

func TestACTestCoverage_CrossRepoNeedsConfiguredIdentity(t *testing.T) {
	sr := gradeSpecRoot(t, "project:\n  title: T\nac_test_coverage:\n  required: true\n")
	writeMD(t, sr, "features/orders/README.md", acFeatureDoc("Stable", "a", "b"))
	writeRootFile(t, sr, "a_test.go", "// specscore:verifies https://specscore.org/github.com/acme/orders/spec/features/orders#ac:a\n"+
		"// specscore:spec/features/orders#ac:b\n")
	acWant(t, acCheck(t, sr, false), "orders#ac:a")
}

// AC: custom-test-globs
// specscore:verifies feature/cli/spec/lint/ac-test-coverage#ac:custom-test-globs
func TestACTestCoverage_CustomTestGlobs(t *testing.T) {
	sr := gradeSpecRoot(t, "project:\n  title: T\nac_test_coverage:\n  required: true\n  test_files: [\"e2e/**/*.feature\", \"**/*.test.js\"]\n")
	writeMD(t, sr, "features/orders/README.md", acFeatureDoc("Stable", "one", "two", "three", "four"))
	writeRootFile(t, sr, "e2e/checkout/x.feature", "# specscore:verifies feature/orders#ac:one\n")
	writeRootFile(t, sr, "y_test.go", "// specscore:verifies feature/orders#ac:two\n")
	writeRootFile(t, sr, "node_modules/pkg/z.test.js", "// specscore:verifies feature/orders#ac:three\n")
	writeRootFile(t, sr, ".git/z.test.js", "// specscore:verifies feature/orders#ac:three\n")
	// A symlinked test file is not followed: the target is outside
	// test_files and names an AC nothing else names.
	writeRootFile(t, sr, "notes/four.txt", "# specscore:verifies feature/orders#ac:four\n")
	if err := os.Symlink(filepath.Join(filepath.Dir(sr), "notes", "four.txt"), filepath.Join(filepath.Dir(sr), "e2e", "link.feature")); err != nil {
		t.Fatal(err)
	}
	acWant(t, acCheck(t, sr, false), "orders#ac:two", "orders#ac:three", "orders#ac:four")
}

// AC: deferral-counts
// specscore:verifies feature/cli/spec/lint/ac-test-coverage#ac:deferral-counts
func TestACTestCoverage_DeferralCounts(t *testing.T) {
	sr := gradeSpecRoot(t, acOptIn)
	writeMD(t, sr, "features/orders/README.md", acFeatureDoc("Stable",
		"x", "y\n**Deferred:** needs a staging account", "z\n**Deferred:** —", "w\n#### Later\n\n**Deferred:** below another heading",
		"v", "u", "r", "s"))
	writeMD(t, sr, "plans/ship-orders/README.md", "# Plan: Ship orders\n\n**Status:** Draft\n**Source Feature:** orders\n\n## Tasks\n\n### Task 1: Do\n\n**Verifies:** orders#ac:y\n**Status:** planning\n\n"+
		"## Deferred AC Coverage\n\n- orders#ac:x — post-MVP scope\n- orders#ac:v\n- orders#ac:u —\n\n## Open Questions\n\nNone at this time.\n")
	// Retired Plans are historical records: their deferrals do not count.
	for slug, status := range map[string]string{"rejected": "Rejected", "superseded": "Superseded"} {
		ac := map[string]string{"rejected": "r", "superseded": "s"}[slug]
		writeMD(t, sr, "plans/"+slug+"/README.md", "# Plan: Old\n\n**Status:** "+status+"\n**Source Feature:** orders\n\n## Deferred AC Coverage\n\n- orders#ac:"+ac+" — later\n")
	}
	vs := acCheck(t, sr, false)
	acWant(t, vs, "orders#ac:z", "orders#ac:w", "orders#ac:v", "orders#ac:u", "orders#ac:r", "orders#ac:s")
	if !strings.Contains(vs[0].Message, "`**Deferred:**` line with no reason") {
		t.Fatalf("empty deferral must say so: %q", vs[0].Message)
	}
	if strings.Contains(vs[1].Message, "no reason") {
		t.Fatalf("a deferral under another heading is simply missing: %q", vs[1].Message)
	}
}

func TestACTestCoverage_PlansDirOverride(t *testing.T) {
	sr := gradeSpecRoot(t, acOptIn)
	writeMD(t, sr, "features/orders/README.md", acFeatureDoc("Stable", "x"))
	external := t.TempDir()
	writeMD(t, external, "p/README.md", "# Plan: P\n\n**Status:** Draft\n**Source Feature:** orders\n\n## Deferred AC Coverage\n\n- orders#ac:x — later\n")
	vs, err := newACTestCoverageChecker("", external, false).check(sr)
	if err != nil || len(vs) != 0 {
		t.Fatalf("an externally routed Plan's deferral must count: %+v, %v", vs, err)
	}
}

// AC: message-names-feature-ac-and-fix
// specscore:verifies feature/cli/spec/lint/ac-test-coverage#ac:message-names-feature-ac-and-fix
func TestACTestCoverage_MessageNamesFeatureACAndFix(t *testing.T) {
	sr := gradeSpecRoot(t, acOptIn)
	writeMD(t, sr, "features/orders/README.md", acFeatureDoc("Implementing", "total", "later"))
	writeMD(t, sr, "features/a/README.md", acFeatureDoc("Stable", "first"))
	vs, err := Lint(Options{SpecRoot: sr, Rules: []string{acTestCoverageRule}})
	if err != nil {
		t.Fatal(err)
	}
	acWant(t, vs, "a#ac:first", "orders#ac:total", "orders#ac:later")
	v := vs[1]
	if v.File != "features/orders/README.md" || v.Line != 17 || v.Severity != "error" || v.Rule != acTestCoverageRule {
		t.Fatalf("finding = %+v", v)
	}
	for _, part := range []string{"Feature is Implementing", "`orders#ac:total`", "`specscore:verifies feature/orders#ac:total`", "`**Deferred:** <reason>`", "## Deferred AC Coverage"} {
		if !strings.Contains(v.Message, part) {
			t.Fatalf("message %q does not contain %q", v.Message, part)
		}
	}
	if strings.Contains(v.Message, "\n") {
		t.Fatalf("message must be one line: %q", v.Message)
	}
}

func TestACTestCoverage_ScanErrorsFailClosed(t *testing.T) {
	t.Run("unreadable directory", func(t *testing.T) {
		sr := gradeSpecRoot(t, acOptIn)
		writeMD(t, sr, "features/orders/README.md", acFeatureDoc("Stable", "x"))
		locked := filepath.Join(filepath.Dir(sr), "locked")
		if err := os.MkdirAll(locked, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
		if _, err := newACTestCoverageChecker("", "", false).check(sr); err == nil {
			t.Fatal("an unreadable directory must fail the check")
		}
	})
	t.Run("unreadable test file", func(t *testing.T) {
		sr := gradeSpecRoot(t, acOptIn)
		writeMD(t, sr, "features/orders/README.md", acFeatureDoc("Stable", "x"))
		p := writeRootFile(t, sr, "x_test.go", "// specscore:verifies feature/orders#ac:x\n")
		if err := os.Chmod(p, 0o000); err != nil {
			t.Fatal(err)
		}
		if _, err := newACTestCoverageChecker("", "", false).check(sr); err == nil {
			t.Fatal("an unreadable test file must fail the check")
		}
	})
	t.Run("unreadable features tree", func(t *testing.T) {
		sr := gradeSpecRoot(t, acOptIn)
		locked := filepath.Join(sr, "features", "locked")
		if err := os.MkdirAll(locked, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(locked, 0o755) })
		if _, err := newACTestCoverageChecker("", "", false).check(sr); err == nil {
			t.Fatal("an unreadable features tree must fail the check")
		}
	})
}

func TestACTestCoverage_RouteBrokenIsNoOp(t *testing.T) {
	sr := gradeSpecRoot(t, acOptIn)
	writeMD(t, sr, "features/orders/README.md", acFeatureDoc("Stable", "x"))
	rc := newACTestCoverageChecker("", "", true).routeErrorChecker()
	if vs, err := rc.check(sr); err != nil || len(vs) != 0 {
		t.Fatalf("route-error checker must be a no-op, got %+v, %v", vs, err)
	}
	vs, err := Lint(Options{SpecRoot: sr, Rules: []string{acTestCoverageRule, "plan-route-unresolved"}, PlanRouteError: "broken"})
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 1 || vs[0].Rule != "plan-route-unresolved" {
		t.Fatalf("under a broken route only plan-route-unresolved is reported, got %+v", vs)
	}
}

func TestIsATXHeading(t *testing.T) {
	for line, want := range map[string]bool{
		"# A": true, "###### F": true, "####": true, "##\tT": true,
		"####### G": false, "#tag": false, "text": false, "": false,
	} {
		if got := isATXHeading(line); got != want {
			t.Errorf("isATXHeading(%q) = %v, want %v", line, got, want)
		}
	}
}

// AC: only-implementing-and-stable
// specscore:verifies feature/cli/spec/lint/ac-test-coverage#ac:only-implementing-and-stable
func TestACTestCoverage_ThematicBreakKeepsLaterACs(t *testing.T) {
	sr := gradeSpecRoot(t, acOptIn)
	doc := strings.Replace(acFeatureDoc("Stable", "before", "after"), "### AC: after", "---\n\n**Deferred:** a break ends the previous AC block\n\n### AC: after", 1)
	writeMD(t, sr, "features/orders/README.md", doc)
	acWant(t, acCheck(t, sr, false), "orders#ac:before", "orders#ac:after")
}

// AC: test-annotation-covers
// specscore:verifies feature/cli/spec/lint/ac-test-coverage#ac:test-annotation-covers
func TestACTestCoverage_SymlinkedProjectRoot(t *testing.T) {
	sr := gradeSpecRoot(t, acOptIn)
	writeMD(t, sr, "features/orders/README.md", acFeatureDoc("Stable", "a", "b"))
	writeRootFile(t, sr, "orders_test.go", "// specscore:verifies feature/orders#ac:a\n")
	link := filepath.Join(t.TempDir(), "work")
	if err := os.Symlink(filepath.Dir(sr), link); err != nil {
		t.Fatal(err)
	}
	acWant(t, acCheck(t, filepath.Join(link, "spec"), false), "orders#ac:b")
	if _, err := acNamedByTests(filepath.Join(t.TempDir(), "missing"), []string{"**"}, ""); err == nil {
		t.Fatal("an unresolvable project root must fail the scan")
	}
}

// AC: deferral-counts
// specscore:verifies feature/cli/spec/lint/ac-test-coverage#ac:deferral-counts
func TestACTestCoverage_PlanTreeErrors(t *testing.T) {
	t.Run("discover error surfaces", func(t *testing.T) {
		sr := gradeSpecRoot(t, acOptIn)
		writeMD(t, sr, "features/orders/README.md", acFeatureDoc("Stable", "x"))
		target := writeMD(t, t.TempDir(), "real.md", "# Plan: P\n\n**Status:** Draft\n")
		if err := os.MkdirAll(filepath.Join(sr, "plans"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(sr, "plans", "linked.md")); err != nil {
			t.Fatal(err)
		}
		if _, err := newACTestCoverageChecker("", "", false).check(sr); err == nil || !strings.Contains(err.Error(), "reading plans dir") {
			t.Fatalf("a Plan-tree read error must be returned, got %v", err)
		}
	})
	t.Run("plans path is a file", func(t *testing.T) {
		sr := gradeSpecRoot(t, acOptIn)
		writeMD(t, sr, "features/orders/README.md", acFeatureDoc("Stable", "x"))
		writeMD(t, sr, "plans", "not a directory\n")
		acWant(t, acCheck(t, sr, false), "orders#ac:x")
	})
}
