package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// featureIndexHeader is the canonical column header + separator used by
// every synthetic features-index README in these tests. Keeps the
// fixtures aligned with the format the production rule expects.
const featureIndexHeader = `# Features

| Feature | Status | Kind | Description |
|---------|--------|------|-------------|
`

// featuresIndex builds a features-index README with one row per (slug,
// status) pair. The Kind and Description cells are populated with
// placeholder values that the rule MUST preserve verbatim across
// `--fix` so the test can assert non-Status cells are never rewritten.
func featuresIndex(rows ...[2]string) string {
	var b strings.Builder
	b.WriteString(featureIndexHeader)
	for _, r := range rows {
		slug, status := r[0], r[1]
		b.WriteString("| [")
		b.WriteString(featureFixtureTitle(slug))
		b.WriteString("](")
		b.WriteString(slug)
		b.WriteString("/README.md) | ")
		b.WriteString(status)
		b.WriteString(" | Command | desc-")
		b.WriteString(slug)
		b.WriteString(" |\n")
	}
	b.WriteString("\n## Open Questions\n\nNone at this time.\n")
	return b.String()
}

// featureFixtureTitle is deliberately limited to the lowercase ASCII slugs
// used by these test fixtures. It keeps test data readable without depending
// on the deprecated, locale-sensitive strings.Title.
func featureFixtureTitle(slug string) string {
	words := strings.Split(slug, "-")
	for i, word := range words {
		if word != "" {
			words[i] = strings.ToUpper(word[:1]) + word[1:]
		}
	}
	return strings.Join(words, " ")
}

// featureReadme builds a minimal feature README declaring the given
// status. Only the `**Status:**` line matters for these tests; the rest
// is a stable shell so the file is non-empty.
func featureReadme(name, status string) string {
	return "# Feature: " + name + "\n\n" +
		"**Status:** " + status + "\n\n" +
		"## Summary\n\ndesc-" + strings.ToLower(strings.ReplaceAll(name, " ", "-")) + "\n"
}

// TestFeatureIndex_CleanCase asserts that an index whose Status cells
// already match the corresponding feature READMEs produces zero
// violations, and that `--fix` is a no-op against the same tree.
func TestFeatureIndex_CleanCase(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"features/README.md":         featuresIndex([2]string{"auth", "Approved"}, [2]string{"billing", "Implementing"}),
		"features/auth/README.md":    featureReadme("Auth", "Approved"),
		"features/billing/README.md": featureReadme("Billing", "Implementing"),
	})

	vs, _, _ := featureIndexRules(specRoot, false)
	if len(vs) != 0 {
		t.Fatalf("expected 0 violations on clean tree, got %d: %+v", len(vs), vs)
	}

	// Snapshot the index before --fix so we can prove --fix is a no-op
	// on the byte level (no whitespace or column reshuffling).
	indexPath := filepath.Join(specRoot, "features", "README.md")
	before, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	vs, fixed, _ := featureIndexRules(specRoot, true)
	if len(vs) != 0 {
		t.Fatalf("expected 0 violations with --fix on clean tree, got %d: %+v", len(vs), vs)
	}
	if fixed {
		t.Fatalf("expected fixed=false on clean tree, got true")
	}
	after, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("expected --fix to leave clean index untouched, but file changed")
	}
}

// TestFeatureIndex_DriftReported builds the canonical drift case from
// the task spec — `spec/features/auth/README.md` says `Approved`, the
// index says `Draft` — and asserts the rule emits a
// `feature-index-row-sync` violation pointing at the index file.
func TestFeatureIndex_DriftReported(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"features/README.md":      featuresIndex([2]string{"auth", "Draft"}),
		"features/auth/README.md": featureReadme("Auth", "Approved"),
	})

	vs, _, _ := featureIndexRules(specRoot, false)
	if !hasRule(vs, "feature-index-row-sync") {
		t.Fatalf("expected feature-index-row-sync violation, got: %+v", vs)
	}

	// The violation must point at the index file, not the feature
	// README, because that is the file --fix will rewrite.
	for _, v := range vs {
		if v.Rule != "feature-index-row-sync" {
			continue
		}
		if !strings.HasSuffix(v.File, filepath.Join("features", "README.md")) {
			t.Errorf("violation File = %q, want features/README.md suffix", v.File)
		}
		if v.Severity != "error" {
			t.Errorf("violation Severity = %q, want error", v.Severity)
		}
		if !strings.Contains(v.Message, "auth") {
			t.Errorf("violation Message = %q, expected to mention auth", v.Message)
		}
	}
}

// TestFeatureIndex_FixRewritesRow exercises the full --fix loop: detect
// drift, run --fix, then re-lint and confirm zero violations. The
// post-fix index MUST contain the rewritten Status cell AND preserve
// the original Kind and Description cells verbatim (per the meta-spec
// contract: those cells are hand-maintained).
func TestFeatureIndex_FixRewritesRow(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"features/README.md":      featuresIndex([2]string{"auth", "Draft"}),
		"features/auth/README.md": featureReadme("Auth", "Approved"),
	})

	vs, fixed, _ := featureIndexRules(specRoot, true)
	if !fixed {
		t.Fatalf("expected fixed=true after running --fix on drifted tree")
	}
	// `--fix` rewrites and returns no violations on the same pass —
	// the post-fix scan is the caller's re-lint step.
	if len(vs) != 0 {
		t.Fatalf("expected 0 violations returned from --fix pass, got %d: %+v", len(vs), vs)
	}

	indexPath := filepath.Join(specRoot, "features", "README.md")
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)

	// Status cell now reads Approved.
	if !strings.Contains(body, "| Approved |") {
		t.Errorf("expected rewritten index to contain `| Approved |`, got:\n%s", body)
	}
	// Original Kind and Description cells preserved verbatim.
	if !strings.Contains(body, "| Command | desc-auth |") {
		t.Errorf("expected --fix to preserve Kind and Description cells, got:\n%s", body)
	}
	// Original drifted value gone.
	if strings.Contains(body, "| Draft |") {
		t.Errorf("expected --fix to remove the stale `| Draft |` cell, got:\n%s", body)
	}

	// Re-lint must report 0 violations.
	vs2, _, _ := featureIndexRules(specRoot, false)
	if len(vs2) != 0 {
		t.Fatalf("expected 0 violations after --fix + re-lint, got %d: %+v", len(vs2), vs2)
	}
}

func TestFeatureIndex_FixRepairsRenamedTitleAndSummary(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"features/README.md":      featureIndexHeader + "| [Old Product Name](auth/README.md) | Draft | Command | Old summary |\n",
		"features/auth/README.md": "# Feature: Competios Discovery\n\n**Status:** Approved\n\n## Summary\n\nLists public competitions.\n",
	})
	vs, _, _ := featureIndexRules(specRoot, false)
	if !hasRule(vs, "feature-index-row-sync") {
		t.Fatalf("expected stale rename violation, got %+v", vs)
	}
	vs, fixed, _ := featureIndexRules(specRoot, true)
	if !fixed || len(vs) != 0 {
		t.Fatalf("fix = (%v, %+v)", fixed, vs)
	}
	data, err := os.ReadFile(filepath.Join(specRoot, "features", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "[Competios Discovery](auth/README.md) | Approved | Command | Lists public competitions.") {
		t.Fatalf("rename was not synchronised:\n%s", data)
	}
}

func TestFeatureIndex_FixPreservesTailCellsWithoutDescription(t *testing.T) {
	const index = `# Features

| Feature | Status | Kind | URL | Index |
|---------|--------|------|-----|-------|
| [Old Name](auth/README.md) | Draft | Command | https://example.com/a\|b | external-auth |
`
	specRoot := writeSpec(t, map[string]string{
		"features/README.md":      index,
		"features/auth/README.md": "# Feature: Competios Discovery\n\n**Status:** Approved\n\n## Summary\n\nLists public competitions.\n",
	})

	vs, fixed, _ := featureIndexRules(specRoot, true)
	if !fixed || len(vs) != 0 {
		t.Fatalf("fix = (%v, %+v)", fixed, vs)
	}
	data, err := os.ReadFile(filepath.Join(specRoot, "features", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, "[Competios Discovery](auth/README.md) | Approved | Command | https://example.com/a\\|b | external-auth |") {
		t.Fatalf("fix did not preserve non-Description URL and Index cells:\n%s", body)
	}
	if strings.Contains(body, "Lists public competitions.") {
		t.Fatalf("summary must not overwrite a non-Description tail cell:\n%s", body)
	}
}

func TestFeatureIndex_FixUsesNonfinalDescriptionColumn(t *testing.T) {
	const index = `# Features

| Feature | Status | Description | URL | Index |
|---------|--------|-------------|-----|-------|
| [Old Name](auth/README.md) | Draft | Old description | https://example.com/a\|b | external-auth |
`
	specRoot := writeSpec(t, map[string]string{
		"features/README.md":      index,
		"features/auth/README.md": "# Feature: Competios Discovery\n\n**Status:** Approved\n\n## Summary\n\nLists public competitions.\n",
	})

	vs, fixed, _ := featureIndexRules(specRoot, true)
	if !fixed || len(vs) != 0 {
		t.Fatalf("fix = (%v, %+v)", fixed, vs)
	}
	data, err := os.ReadFile(filepath.Join(specRoot, "features", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "[Competios Discovery](auth/README.md) | Approved | Lists public competitions. | https://example.com/a\\|b | external-auth |") {
		t.Fatalf("fix did not target the explicit Description column:\n%s", data)
	}
}

func TestFeatureIndex_FixEscapesPipesInDerivedCells(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"features/README.md":      featureIndexHeader + "| [Old Name](auth/README.md) | Draft | Command | Old description |\n",
		"features/auth/README.md": "# Feature: Creator | Competitor\n\n**Status:** Approved\n\n## Summary\n\nA creator | competitor event.\n",
	})

	vs, fixed, _ := featureIndexRules(specRoot, true)
	if !fixed || len(vs) != 0 {
		t.Fatalf("fix = (%v, %+v)", fixed, vs)
	}
	data, err := os.ReadFile(filepath.Join(specRoot, "features", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	body := string(data)
	if !strings.Contains(body, "[Creator \\| Competitor](auth/README.md) | Approved | Command | A creator \\| competitor event. |") {
		t.Fatalf("derived pipe values were not escaped safely:\n%s", body)
	}
	if vs, _, _ := featureIndexRules(specRoot, false); len(vs) != 0 {
		t.Fatalf("escaped row should be clean on re-lint, got %+v", vs)
	}
}

func TestFeatureIndexHelpers_RejectMalformedRowsAndLinks(t *testing.T) {
	if _, _, ok := parseFeatureIndexLink("[Auth](auth/README.md"); ok {
		t.Fatal("malformed feature link must not parse")
	}

	indexPath := filepath.Join(t.TempDir(), "README.md")
	content := "| Feature | Status | Description |\n|---|---|---|\n| [Auth](auth/README.md) | Draft |\n"
	if err := os.WriteFile(indexPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	rows, err := readFeatureIndexRows(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 {
		t.Fatalf("malformed row must be ignored, got %+v", rows)
	}
	if err := rewriteFeatureIndexRows(indexPath, map[string]featureIndexValue{"auth": {title: "Auth", status: "Stable"}}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != content {
		t.Fatalf("malformed row must round-trip untouched:\n%s", data)
	}
}

func TestFeatureIndexHelpers_HandleMissingDerivedInput(t *testing.T) {
	if _, err := parseFeatureIndexSummary(filepath.Join(t.TempDir(), "missing.md")); err == nil {
		t.Fatal("missing feature README must return an error")
	}

	path := filepath.Join(t.TempDir(), "README.md")
	if err := os.WriteFile(path, []byte("# Feature: Auth\n\n## Summary\n\n## Details\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if summary, err := parseFeatureIndexSummary(path); err != nil || summary != "" {
		t.Fatalf("summary before next heading = (%q, %v), want empty, nil", summary, err)
	}

	if err := rewriteFeatureIndexRows(filepath.Join(t.TempDir(), "missing.md"), nil); err == nil {
		t.Fatal("missing index must return an error")
	}
	plain := filepath.Join(t.TempDir(), "plain.md")
	if err := os.WriteFile(plain, []byte("# No index\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := rewriteFeatureIndexRows(plain, nil); err != nil {
		t.Fatalf("index without a Feature/Status table must be a no-op: %v", err)
	}
}

func TestFeatureIndexRules_SkipsFeatureWithoutParseableTitle(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"features/README.md": featureIndexHeader + "| [Auth](auth/README.md) | Draft | Command | desc-auth |\n",
		// Status is available before the oversized line; ParseFeatureTitle then
		// reports the scanner error instead of silently deriving an empty title.
		"features/auth/README.md": "**Status:** Draft\n" + strings.Repeat("x", 70*1024),
	})
	violations, fixed, _ := featureIndexRules(specRoot, true)
	if fixed || len(violations) != 0 {
		t.Fatalf("unparseable feature title must be skipped, got fixed=%v violations=%+v", fixed, violations)
	}
}

func TestFeatureIndexRules_SkipsFeatureWithUnreadableSummary(t *testing.T) {
	original := parseFeatureIndexSummaryFile
	parseFeatureIndexSummaryFile = func(string) (string, error) { return "", os.ErrPermission }
	t.Cleanup(func() { parseFeatureIndexSummaryFile = original })
	specRoot := writeSpec(t, map[string]string{
		"features/README.md":      featureIndexHeader + "| [Auth](auth/README.md) | Draft | Command | desc-auth |\n",
		"features/auth/README.md": "# Feature: Auth\n\n**Status:** Draft\n\n## Summary\n\nDescription.\n",
	})
	violations, fixed, _ := featureIndexRules(specRoot, true)
	if fixed || len(violations) != 0 {
		t.Fatalf("unreadable summary must be skipped, got fixed=%v violations=%+v", fixed, violations)
	}
}

// TestFeatureIndex_TopLevelOnly asserts the rule never fires for
// sub-features. The features-index lists only top-level rows; rows
// whose slug contains "/" point into nested directories and are not
// part of the row-sync contract. A drifted sub-feature row must NOT
// produce a violation.
func TestFeatureIndex_TopLevelOnly(t *testing.T) {
	// Index row for a sub-feature `cli/idea/change-status` with a wrong
	// Status. The corresponding feature README declares `Approved` but
	// the index shows `Draft` — for a top-level slug this would be
	// drift; for a sub-feature it MUST be ignored.
	indexBody := featureIndexHeader +
		"| [cli/idea/change-status](cli/idea/change-status/README.md) | Draft | Command | sub-feature row |\n" +
		"\n## Open Questions\n\nNone at this time.\n"
	specRoot := writeSpec(t, map[string]string{
		"features/README.md":                        indexBody,
		"features/cli/idea/change-status/README.md": featureReadme("Change Status", "Approved"),
	})

	vs, _, _ := featureIndexRules(specRoot, false)
	if hasRule(vs, "feature-index-row-sync") {
		t.Fatalf("expected no feature-index-row-sync violation for sub-feature rows, got: %+v", vs)
	}
}

// TestFeatureIndex_RegisteredWithLinter wires the rule through the
// public Lint() entry point to prove it participates in every default
// `specscore spec lint` invocation — not just direct calls to
// featureIndexRules. Uses the same drift fixture as
// TestFeatureIndex_DriftReported.
func TestFeatureIndex_RegisteredWithLinter(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"features/README.md":      featuresIndex([2]string{"auth", "Draft"}),
		"features/auth/README.md": featureReadme("Auth", "Approved"),
	})

	vs, err := Lint(Options{SpecRoot: specRoot})
	if err != nil {
		t.Fatal(err)
	}
	if !hasRule(vs, "feature-index-row-sync") {
		t.Fatalf("expected feature-index-row-sync from Lint(), got: %+v", vs)
	}
}

// TestFeatureIndex_LintFixIntegration drives the fix path through the
// public Lint() entry point with Fix=true and confirms a follow-up
// Lint() reports zero feature-index-row-sync violations.
func TestFeatureIndex_LintFixIntegration(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"features/README.md":      featuresIndex([2]string{"auth", "Draft"}),
		"features/auth/README.md": featureReadme("Auth", "Approved"),
	})

	// First pass with Fix=true mutates the index.
	if _, err := Lint(Options{SpecRoot: specRoot, Fix: true}); err != nil {
		t.Fatal(err)
	}

	// Second pass without Fix must report no row-sync violations.
	vs, err := Lint(Options{SpecRoot: specRoot})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range vs {
		if v.Rule == "feature-index-row-sync" {
			t.Fatalf("expected feature-index-row-sync to be silent after --fix, got: %+v", v)
		}
	}
}

// TestFeatureIndex_AllRuleNames guards against silent registry drift:
// the rule name must be listed in the canonical AllRuleNames map so
// `--rules feature-index-row-sync` and `--ignore feature-index-row-sync`
// validate.
func TestFeatureIndex_AllRuleNames(t *testing.T) {
	names := AllRuleNames()
	if !names["feature-index-row-sync"] {
		t.Fatalf("feature-index-row-sync missing from AllRuleNames()")
	}
}
