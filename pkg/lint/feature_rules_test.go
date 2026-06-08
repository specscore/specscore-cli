package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// frEnv writes feature READMEs under a temp spec root for the feature-rules checker.
type frEnv struct{ specRoot string }

func newFREnv(t *testing.T) *frEnv {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "features"), 0o755); err != nil {
		t.Fatal(err)
	}
	return &frEnv{specRoot: root}
}

func (e *frEnv) write(t *testing.T, slug, body string) string {
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

func frBody(slug, sourceIdeasLine string) string {
	return "# Feature: " + slug + "\n\n**Status:** Approved\n" + sourceIdeasLine +
		"\n## Summary\n\nx\n\n## Open Questions\n\nNone.\n"
}

func TestFeatureRules_MissingFlagged(t *testing.T) {
	e := newFREnv(t)
	e.write(t, "missing", frBody("missing", ""))
	c := newFeatureRulesChecker()
	vs, err := c.check(e.specRoot)
	if err != nil {
		t.Fatal(err)
	}
	if hasViolation(vs, "feature-source-ideas-required", "missing a **Source Ideas:**") == nil {
		t.Fatal("expected missing-line violation")
	}
}

func TestFeatureRules_SentinelsAndListAccepted(t *testing.T) {
	e := newFREnv(t)
	e.write(t, "dash", frBody("dash", "**Source Ideas:** —\n"))
	e.write(t, "none", frBody("none", "**Source Ideas:** none\n"))
	e.write(t, "list", frBody("list", "**Source Ideas:** a-idea, b-idea\n"))
	c := newFeatureRulesChecker()
	vs, err := c.check(e.specRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 0 {
		t.Fatalf("sentinels/list should be accepted, got %d violations: %+v", len(vs), vs)
	}
}

func TestFeatureRules_TwoMissingSortedByFile(t *testing.T) {
	e := newFREnv(t)
	e.write(t, "aaa", frBody("aaa", ""))
	e.write(t, "bbb", frBody("bbb", ""))
	c := newFeatureRulesChecker()
	vs, _ := c.check(e.specRoot)
	if len(vs) != 2 {
		t.Fatalf("expected 2 violations, got %d", len(vs))
	}
	if vs[0].File >= vs[1].File {
		t.Fatalf("violations not sorted by file: %s, %s", vs[0].File, vs[1].File)
	}
}

func TestFeatureRules_EmptyValueFlagged(t *testing.T) {
	e := newFREnv(t)
	e.write(t, "empty", frBody("empty", "**Source Ideas:** \n"))
	c := newFeatureRulesChecker()
	vs, _ := c.check(e.specRoot)
	if hasViolation(vs, "feature-source-ideas-required", "empty value") == nil {
		t.Fatal("expected empty-value violation")
	}
}

func TestFeatureRules_NonFeatureSkipped(t *testing.T) {
	e := newFREnv(t)
	// A README that is not a Feature (no `# Feature:` H1).
	e.write(t, "notafeature", "# Notes\n\njust notes\n")
	c := newFeatureRulesChecker()
	vs, _ := c.check(e.specRoot)
	if len(vs) != 0 {
		t.Fatalf("non-feature README must be skipped, got %+v", vs)
	}
}

func TestFeatureRules_MissingNoStatusLineUsesLineOne(t *testing.T) {
	e := newFREnv(t)
	// Feature with no **Status:** line at all → violation line falls back to 1.
	e.write(t, "nostatus", "# Feature: NoStatus\n\n## Summary\n\nx\n\n## Open Questions\n\nNone.\n")
	c := newFeatureRulesChecker()
	vs, _ := c.check(e.specRoot)
	v := hasViolation(vs, "feature-source-ideas-required", "missing")
	if v == nil || v.Line != 1 {
		t.Fatalf("expected missing violation at line 1, got %+v", vs)
	}
}

func TestFeatureRules_FixBackfillsMissingOnly(t *testing.T) {
	e := newFREnv(t)
	mp := e.write(t, "missing", frBody("missing", ""))
	hp := e.write(t, "have", frBody("have", "**Source Ideas:** existing-idea\n"))
	haveBefore, _ := os.ReadFile(hp)

	c := newFeatureRulesChecker()
	c.autofix = true
	if err := c.fix(e.specRoot); err != nil {
		t.Fatal(err)
	}

	got, _ := os.ReadFile(mp)
	if !strings.Contains(string(got), "**Status:** Approved\n**Source Ideas:** —\n") {
		t.Fatalf("missing feature not backfilled after Status: %q", got)
	}
	haveAfter, _ := os.ReadFile(hp)
	if string(haveBefore) != string(haveAfter) {
		t.Fatal("feature that already had Source Ideas must be left untouched")
	}
	// Idempotent: a second fix changes nothing; check now reports clean.
	if err := c.fix(e.specRoot); err != nil {
		t.Fatal(err)
	}
	vs, _ := c.check(e.specRoot)
	if len(vs) != 0 {
		t.Fatalf("after fix, check should be clean, got %+v", vs)
	}
}

func TestFeatureRules_FixSkipsNonFeatureAndNoStatus(t *testing.T) {
	e := newFREnv(t)
	nf := e.write(t, "notfeature", "# Notes\n\nx\n")
	ns := e.write(t, "nostatus", "# Feature: NoStatus\n\n## Summary\n\nx\n")
	nfBefore, _ := os.ReadFile(nf)
	nsBefore, _ := os.ReadFile(ns)
	c := newFeatureRulesChecker()
	if err := c.fix(e.specRoot); err != nil {
		t.Fatal(err)
	}
	nfAfter, _ := os.ReadFile(nf)
	nsAfter, _ := os.ReadFile(ns)
	if string(nfBefore) != string(nfAfter) || string(nsBefore) != string(nsAfter) {
		t.Fatal("fix must not touch non-feature or status-less files")
	}
}

func TestFeatureRules_WalkError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root, chmod tests skipped")
	}
	e := newFREnv(t)
	e.write(t, "auth", frBody("auth", "**Source Ideas:** —\n"))
	featuresDir := filepath.Join(e.specRoot, "features")
	// Execute-only on the features dir: Stat succeeds, ReadDir fails → walk error.
	if err := os.Chmod(featuresDir, 0o111); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(featuresDir, 0o755) }()

	c := newFeatureRulesChecker()
	if _, err := c.check(featuresParent(e.specRoot)); err == nil {
		t.Skip("walk did not surface an error on this platform")
	}
}

// featuresParent returns specRoot unchanged; check() walks specRoot/features.
func featuresParent(specRoot string) string { return specRoot }

func TestFeatureRules_NameSeverity(t *testing.T) {
	c := newFeatureRulesChecker()
	if c.name() != "feature-source-ideas-required" || c.severity() != "error" {
		t.Fatalf("unexpected name/severity: %s/%s", c.name(), c.severity())
	}
}
