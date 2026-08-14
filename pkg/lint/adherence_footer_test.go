package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validFooterURL = "https://specscore.md/feature-specification"

func TestAdherenceFooter_MissingURL_Reports(t *testing.T) {
	tmp := t.TempDir()
	writeFeatureReadme(t, tmp, "bad", "# Feature: Bad\n\n**Status:** Draft\n\n## Summary\nNo footer here.\n")

	violations := runAdherenceFooterCheck(t, tmp)

	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(violations), violations)
	}
	v := violations[0]
	if v.Rule != "adherence-footer" {
		t.Errorf("Rule = %q, want %q", v.Rule, "adherence-footer")
	}
	if v.Severity != "error" {
		t.Errorf("Severity = %q, want %q", v.Severity, "error")
	}
	if v.File != filepath.Join("features", "bad", "README.md") {
		t.Errorf("File = %q, want features/bad/README.md", v.File)
	}
}

func TestAdherenceFooter_URLPresent_NoViolation(t *testing.T) {
	tmp := t.TempDir()
	content := "# Feature: Good\n\n**Status:** Draft\n\n## Summary\nHas footer.\n\n---\n*This document follows the " + validFooterURL + "*\n"
	writeFeatureReadme(t, tmp, "good", content)

	violations := runAdherenceFooterCheck(t, tmp)
	if len(violations) != 0 {
		t.Errorf("expected 0 violations, got %d: %+v", len(violations), violations)
	}
}

func TestAdherenceFooter_TrailingSlashTolerated(t *testing.T) {
	tmp := t.TempDir()
	content := "# Feature: Slash\n\n**Status:** Draft\n\n## Summary\n\n" + validFooterURL + "/\n"
	writeFeatureReadme(t, tmp, "slash", content)

	violations := runAdherenceFooterCheck(t, tmp)
	if len(violations) != 0 {
		t.Errorf("expected 0 violations, got %d: %+v", len(violations), violations)
	}
}

func TestAdherenceFooter_URLAnywhereInDoc_NoViolation(t *testing.T) {
	tmp := t.TempDir()
	// URL cited in the middle of the body rather than as a footer — still counts.
	content := "# Feature: Middle\n\nSee " + validFooterURL + " for the spec.\n\n## Summary\nBody.\n"
	writeFeatureReadme(t, tmp, "middle", content)

	violations := runAdherenceFooterCheck(t, tmp)
	if len(violations) != 0 {
		t.Errorf("expected 0 violations, got %d: %+v", len(violations), violations)
	}
}

func TestAdherenceFooter_PlanReadmeWithoutURL_Warns(t *testing.T) {
	tmp := t.TempDir()
	// A plan README missing the plan-specification URL — consumer-layer
	// check introduced by the adherence-footer-and-doc-type-registry Idea.
	// Initial severity is "warn" per the MVP rollout gate.
	plansDir := filepath.Join(tmp, "plans", "some-plan")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plansDir, "README.md"),
		[]byte("# Plan: Some Plan\n\nNo footer.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	violations := runAdherenceFooterCheck(t, tmp)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation (plan readme without footer), got %d: %+v", len(violations), violations)
	}
	v := violations[0]
	if v.Severity != "warn" {
		t.Errorf("Severity = %q, want %q", v.Severity, "warn")
	}
	if v.Rule != "adherence-footer" {
		t.Errorf("Rule = %q, want %q", v.Rule, "adherence-footer")
	}
}

func TestAdherenceFooter_PlanReadmeWithURL_NoViolation(t *testing.T) {
	tmp := t.TempDir()
	plansDir := filepath.Join(tmp, "plans", "some-plan")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "# Plan: Some Plan\n\n---\n*This document follows the https://specscore.md/plan-specification*\n"
	if err := os.WriteFile(filepath.Join(plansDir, "README.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	violations := runAdherenceFooterCheck(t, tmp)
	if len(violations) != 0 {
		t.Errorf("expected 0 violations, got %d: %+v", len(violations), violations)
	}
}

func TestAdherenceFooter_FlatNonPlanIsIgnored(t *testing.T) {
	tmp := t.TempDir()
	path := filepath.Join(tmp, "plans", "notes.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("# Random notes\n\nNo Plan footer.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if violations := runAdherenceFooterCheck(t, tmp); len(violations) != 0 {
		t.Fatalf("non-Plan flat markdown must not be checked for a Plan footer: %+v", violations)
	}
}

func TestAdherenceFooter_IdeaFileWithoutURL_Warns(t *testing.T) {
	tmp := t.TempDir()
	ideasDir := filepath.Join(tmp, "ideas")
	if err := os.MkdirAll(ideasDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ideasDir, "some-idea.md"),
		[]byte("# Idea: Some Idea\n\nNo footer.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	violations := runAdherenceFooterCheck(t, tmp)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation, got %d: %+v", len(violations), violations)
	}
	if violations[0].Severity != "warn" {
		t.Errorf("Severity = %q, want %q", violations[0].Severity, "warn")
	}
}

func TestAdherenceFooter_IdeasIndexCheckedAgainstIndexURL(t *testing.T) {
	tmp := t.TempDir()
	ideasDir := filepath.Join(tmp, "ideas")
	if err := os.MkdirAll(ideasDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// README.md in ideas/ is the ideas-index — it MUST carry the
	// ideas-index-specification URL (not idea-specification).
	if err := os.WriteFile(filepath.Join(ideasDir, "README.md"),
		[]byte("# Ideas Index\n\nNo footer.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	violations := runAdherenceFooterCheck(t, tmp)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation (ideas-index missing footer), got %d: %+v", len(violations), violations)
	}
	v := violations[0]
	if !strings.Contains(v.Message, "ideas-index-specification") {
		t.Errorf("Message should reference ideas-index-specification, got %q", v.Message)
	}
	if strings.Contains(v.Message, "/idea-specification") {
		t.Errorf("Message must NOT reference idea-specification (the ideas README is an index, not an Idea); got %q", v.Message)
	}
	if v.Severity != "warn" {
		t.Errorf("Severity = %q, want %q", v.Severity, "warn")
	}
}

func TestAdherenceFooter_ArchivedIdeasIgnored(t *testing.T) {
	tmp := t.TempDir()
	archived := filepath.Join(tmp, "ideas", "archived")
	if err := os.MkdirAll(archived, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(archived, "old.md"),
		[]byte("# Idea: Old\n\nNo footer, archived.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	violations := runAdherenceFooterCheck(t, tmp)
	if len(violations) != 0 {
		t.Errorf("expected 0 violations (archived/ excluded), got %d: %+v", len(violations), violations)
	}
}

func TestAdherenceFooter_UnderscoreSubtreeNotTreatedAsFeature(t *testing.T) {
	tmp := t.TempDir()
	// A valid feature with the footer.
	writeFeatureReadme(t, tmp, "auth", "# Feature: Auth\n\n**Status:** Draft\n\n"+validFooterURL+"\n")
	// A README inside _tests/ without the footer — must NOT be flagged as a
	// feature (feature-specification is a feature-only check). It IS flagged
	// separately as a scenarios-index missing its own URL.
	testsDir := filepath.Join(tmp, "features", "auth", "_tests")
	if err := os.MkdirAll(testsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(testsDir, "README.md"),
		[]byte("# Scenarios: Auth\n\nNo footer.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	violations := runAdherenceFooterCheck(t, tmp)
	if len(violations) != 1 {
		t.Fatalf("expected 1 violation (scenarios-index missing footer), got %d: %+v", len(violations), violations)
	}
	v := violations[0]
	if !strings.Contains(v.Message, "scenarios-index-specification") {
		t.Errorf("Message should reference scenarios-index-specification, got %q", v.Message)
	}
	if strings.Contains(v.Message, "feature-specification") {
		t.Errorf("Message must NOT reference feature-specification (the _tests/README.md is a scenarios-index, not a feature); got %q", v.Message)
	}
	if v.Severity != "warn" {
		t.Errorf("Severity = %q, want %q", v.Severity, "warn")
	}
}

func TestAdherenceFooter_ScenariosIndexWithURL_NoViolation(t *testing.T) {
	tmp := t.TempDir()
	testsDir := filepath.Join(tmp, "features", "auth", "_tests")
	if err := os.MkdirAll(testsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "# Scenarios: Auth\n\n---\n*This document follows the https://specscore.md/scenarios-index-specification*\n"
	if err := os.WriteFile(filepath.Join(testsDir, "README.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	violations := runAdherenceFooterCheck(t, tmp)
	if len(violations) != 0 {
		t.Errorf("expected 0 violations, got %d: %+v", len(violations), violations)
	}
}

func TestAdherenceFooter_RegisteredAsKnownRule(t *testing.T) {
	rules := AllRuleNames()
	if !rules["adherence-footer"] {
		t.Error("expected adherence-footer to be a known rule")
	}
}

func TestAdherenceFooterFix_RewritesTrailingWrongURL_InPlace(t *testing.T) {
	tmp := t.TempDir()
	writeFeaturesIndexReadme(t, tmp, "# specscore-cli Features\n\n---\n*This document follows the https://specscore.md/feature-specification*\n")

	runAdherenceFooterFix(t, tmp)

	b, err := os.ReadFile(filepath.Join(tmp, "features", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if strings.Contains(got, "https://specscore.md/feature-specification") {
		t.Fatalf("wrong URL should be replaced, got:\n%s", got)
	}
	if !strings.Contains(got, "https://specscore.md/features-index-specification") {
		t.Fatalf("expected rewritten features-index URL, got:\n%s", got)
	}
	if strings.Count(got, "*This document follows the https://specscore.md/") != 1 {
		t.Fatalf("expected exactly one adherence footer, got:\n%s", got)
	}
}

func TestAdherenceFooterFix_RewritesTrailingWrongURL_EvenIfCanonicalURLAppearsElsewhere(t *testing.T) {
	tmp := t.TempDir()
	content := "# specscore-cli Features\n\nSee https://specscore.md/features-index-specification for details.\n\n---\n*This document follows the https://specscore.md/feature-specification*\n"
	writeFeaturesIndexReadme(t, tmp, content)

	runAdherenceFooterFix(t, tmp)

	b, err := os.ReadFile(filepath.Join(tmp, "features", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	got := string(b)
	if strings.Count(got, "*This document follows the https://specscore.md/") != 1 {
		t.Fatalf("expected exactly one trailing adherence footer, got:\n%s", got)
	}
	if !strings.HasSuffix(got, "\n---\n*This document follows the https://specscore.md/features-index-specification*\n") {
		t.Fatalf("expected trailing footer to be rewritten, got:\n%s", got)
	}
}

// writeFeatureReadme writes a feature README under specRoot/features/<slug>/README.md.
func writeFeatureReadme(t *testing.T, specRoot, slug, content string) {
	t.Helper()
	dir := filepath.Join(specRoot, "features", slug)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// runAdherenceFooterCheck runs only the adherence-footer checker against specRoot.
func runAdherenceFooterCheck(t *testing.T, specRoot string) []Violation {
	t.Helper()
	c := newAdherenceFooterChecker()
	violations, err := c.check(specRoot)
	if err != nil {
		t.Fatalf("check returned error: %v", err)
	}
	return violations
}

func runAdherenceFooterFix(t *testing.T, specRoot string) {
	t.Helper()
	c := newAdherenceFooterChecker()
	f, ok := c.(fixer)
	if !ok {
		t.Fatal("adherence-footer checker does not implement fixer")
	}
	if err := f.fix(specRoot); err != nil {
		t.Fatalf("fix returned error: %v", err)
	}
}

func writeFeaturesIndexReadme(t *testing.T, specRoot, content string) {
	t.Helper()
	dir := filepath.Join(specRoot, "features")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
