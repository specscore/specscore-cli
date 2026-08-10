package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// AC: index-entries-ignores-nonidentity-cell-links. Feature row-sync copies a
// Feature's canonical Summary into the root index. Relative child links in
// that Summary describe the Feature; they do not declare root-level children.
func TestIndexEntries_IgnoresSummaryLinksAfterFeatureRowSync(t *testing.T) {
	root := setupSpecTree(t, map[string]string{
		"features/README.md": "# Features\n\n## Index\n\n" +
			"| Feature | Status | Description |\n" +
			"|---|---|---|\n" +
			"| [Old Parent](parent/README.md) | Draft | stale |\n",
		"features/parent/README.md": "# Feature: Parent\n\n" +
			"**Status:** Draft\n\n" +
			"## Summary\n\n" +
			"Owns a [nested capability](nested/README.md).\n\n" +
			"## Contents\n\n" +
			"| Feature | Description |\n" +
			"|---|---|\n" +
			"| [Nested](nested/README.md) | Nested capability. |\n",
		"features/parent/nested/README.md": "# Feature: Nested\n",
	})

	violations, fixed := featureIndexRules(root, true)
	if !fixed || len(violations) != 0 {
		t.Fatalf("row-sync fix = (%v, %+v), want fixed with no violations", fixed, violations)
	}
	index, err := os.ReadFile(filepath.Join(root, "features", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), "[nested capability](nested/README.md)") {
		t.Fatalf("row-sync output did not retain the link-rich Summary:\n%s", index)
	}

	violations, err = newIndexEntriesChecker().check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("Summary link became a root child reference: %+v", violations)
	}
}

func TestIndexEntries_SummaryLinkCannotMaskWrongIdentityCell(t *testing.T) {
	root := setupSpecTree(t, map[string]string{
		"features/README.md": "# Features\n\n## Index\n\n" +
			"| Feature | Status | Description |\n" +
			"|---|---|---|\n" +
			"| [Ghost](ghost/README.md) | Draft | See [Real](real/README.md). |\n",
		"features/real/README.md": "# Feature: Real\n",
	})

	violations, err := newIndexEntriesChecker().check(root)
	if err != nil {
		t.Fatal(err)
	}
	var ghostMissing, realUnlisted bool
	for _, violation := range violations {
		ghostMissing = ghostMissing || strings.Contains(violation.Message, "non-existent directory: ghost")
		realUnlisted = realUnlisted || strings.Contains(violation.Message, "not listed in index: real")
	}
	if !ghostMissing || !realUnlisted {
		t.Fatalf("first-cell identity was masked by Summary link: %+v", violations)
	}
}

func TestIndexEntries_SummaryLinkCannotMaskMissingIdentityLink(t *testing.T) {
	root := setupSpecTree(t, map[string]string{
		"features/README.md": "# Features\n\n## Index\n\n" +
			"| Feature | Status | Description |\n" +
			"|---|---|---|\n" +
			"| Real | Draft | See [Real](real/README.md). |\n",
		"features/real/README.md": "# Feature: Real\n",
	})

	violations, err := newIndexEntriesChecker().check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 1 || !strings.Contains(violations[0].Message, "not listed in index: real") {
		t.Fatalf("missing identity link was masked by Summary link: %+v", violations)
	}
}

func TestIndexEntries_FixerPreservesIdentityRowWithSummaryLinks(t *testing.T) {
	const index = "# Features\n\n## Index\n\n" +
		"| Feature | Status | Description |\n" +
		"|---|---|---|\n" +
		"| [Real](real/README.md) | Draft | See [Ghost](ghost/README.md). |\n"
	root := setupSpecTree(t, map[string]string{
		"features/README.md":      index,
		"features/real/README.md": "# Feature: Real\n",
	})

	checker := newIndexEntriesChecker().(*indexEntriesChecker)
	if err := checker.fix(root); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "features", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != index {
		t.Fatalf("fixer mutated a valid identity row because of its Summary link:\n%s", got)
	}
}

func TestIndexEntries_UsesFeatureIdentityColumnAfterOrdinal(t *testing.T) {
	const index = "# Features\n\n## Index\n\n" +
		"| # | Feature | Description |\n" +
		"|---|---|---|\n" +
		"| 1 | [Real](real/README.md) | See [Ghost](ghost/README.md). |\n"
	root := setupSpecTree(t, map[string]string{
		"features/README.md":      index,
		"features/real/README.md": "# Feature: Real\n",
	})

	violations, err := newIndexEntriesChecker().check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("Feature identity in column two was not recognized: %+v", violations)
	}
	checker := newIndexEntriesChecker().(*indexEntriesChecker)
	if err := checker.fix(root); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "features", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != index {
		t.Fatalf("fixer rewrote a valid legacy column order:\n%s", got)
	}
}

func TestIndexEntries_FixerUsesFeatureIdentityColumnAfterOrdinal(t *testing.T) {
	const index = "# Features\n\n## Index\n\n" +
		"| # | Feature | Description |\n" +
		"|---|---|---|\n" +
		"| 1 | [Ghost](ghost/README.md) | See [Real](real/README.md). |\n" +
		"| 2 | [Real](real/README.md) | valid |\n"
	root := setupSpecTree(t, map[string]string{
		"features/README.md":      index,
		"features/real/README.md": "# Feature: Real\n",
	})

	checker := newIndexEntriesChecker().(*indexEntriesChecker)
	if err := checker.fix(root); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "features", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(got), "[Ghost](ghost/README.md)") {
		t.Fatalf("fixer retained phantom identity row:\n%s", got)
	}
	if !strings.Contains(string(got), "[Real](real/README.md)") {
		t.Fatalf("fixer removed valid identity row:\n%s", got)
	}
}

func TestIndexEntries_UsesUnambiguousCustomArtifactColumn(t *testing.T) {
	root := setupSpecTree(t, map[string]string{
		"features/parent/README.md": "# Feature: Parent\n\n## Contents\n\n" +
			"| # | Mini-product | Domain | Captures | Status |\n" +
			"|---|---|---|---|---|\n" +
			"| 1 | [Real](real/README.md) | example.test | people | Draft |\n",
		"features/parent/real/README.md": "# Feature: Real\n",
	})

	violations, err := newIndexEntriesChecker().check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("unambiguous custom artifact column was not recognized: %+v", violations)
	}
}

func TestIndexEntries_FixerPreservesCustomSchemaForMissingChild(t *testing.T) {
	const parent = "# Feature: Parent\n\n## Contents\n\n" +
		"| # | Mini-product | Domain | Captures | Status |\n" +
		"|---|---|---|---|---|\n" +
		"| 1 | [Existing](existing/README.md) | example.test | people | Draft |\n"
	root := setupSpecTree(t, map[string]string{
		"features/parent/README.md":          parent,
		"features/parent/existing/README.md": "# Feature: Existing\n\n**Status:** Draft\n",
		"features/parent/missing/README.md":  "# Feature: Missing\n\n**Status:** Stable\n",
	})

	checker := newIndexEntriesChecker().(*indexEntriesChecker)
	if err := checker.fix(root); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "features", "parent", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	wantRow := "| — | [missing](missing/README.md) | — | — | Stable |"
	if !strings.Contains(string(got), wantRow) {
		t.Fatalf("fixer did not preserve the five-column identity schema; want %q:\n%s", wantRow, got)
	}
	if strings.Contains(string(got), "| [missing](missing/README.md) | TODO: Add description. |") {
		t.Fatalf("fixer appended its canonical two-column row to a custom table:\n%s", got)
	}
	violations, err := checker.check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 0 {
		t.Fatalf("schema-aware fix left violations: %+v", violations)
	}
}

func TestIndexEntries_InvalidIdentitySchemasFailClosed(t *testing.T) {
	tests := []struct {
		name       string
		header     string
		separator  string
		row        string
		wantReason string
	}{
		{
			name:       "missing",
			header:     "| Status | Summary |",
			separator:  "|---|---|",
			row:        "| Draft | See [Real](real/README.md). |",
			wantReason: "missing child identity column",
		},
		{
			name:       "duplicate",
			header:     "| Feature | Child | Summary |",
			separator:  "|---|---|---|",
			row:        "| [Ghost](ghost/README.md) | [Real](real/README.md) | linked |",
			wantReason: "multiple child identity columns",
		},
		{
			name:       "ambiguous custom",
			header:     "| Product | Service | Status |",
			separator:  "|---|---|---|",
			row:        "| [Real](real/README.md) | service | Draft |",
			wantReason: "ambiguous child identity columns",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			const prefix = "# Feature: Parent\n\n## Contents\n\n"
			index := prefix + tt.header + "\n" + tt.separator + "\n" + tt.row + "\n"
			root := setupSpecTree(t, map[string]string{
				"features/parent/README.md":      index,
				"features/parent/real/README.md": "# Feature: Real\n",
			})

			checker := newIndexEntriesChecker().(*indexEntriesChecker)
			violations, err := checker.check(root)
			if err != nil {
				t.Fatal(err)
			}
			if len(violations) != 1 || !strings.Contains(violations[0].Message, tt.wantReason) {
				t.Fatalf("invalid schema did not fail closed with %q: %+v", tt.wantReason, violations)
			}

			if err := checker.fix(root); err != nil {
				t.Fatal(err)
			}
			got, err := os.ReadFile(filepath.Join(root, "features", "parent", "README.md"))
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != index {
				t.Fatalf("fixer mutated unsafe schema:\n%s", got)
			}
		})
	}
}

func TestIndexEntries_FixerScaffoldsRootRowsOnceInSortedOrder(t *testing.T) {
	root := setupSpecTree(t, map[string]string{
		"features/README.md":       "# Features\n",
		"features/zulu/README.md":  "# Feature: Zulu\n\n**Status:** Stable\n",
		"features/alpha/README.md": "# Feature: Alpha\n\n**Status:** Draft\n",
	})

	checker := newIndexEntriesChecker().(*indexEntriesChecker)
	if err := checker.fix(root); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "features", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	alpha := "| [alpha](alpha/README.md) | Draft | — | TODO: Add description. |"
	zulu := "| [zulu](zulu/README.md) | Stable | — | TODO: Add description. |"
	if strings.Count(string(got), alpha) != 1 || strings.Count(string(got), zulu) != 1 ||
		strings.Index(string(got), alpha) > strings.Index(string(got), zulu) {
		t.Fatalf("root scaffold rows are missing, duplicated, or unsorted:\n%s", got)
	}
}

func TestIndexEntries_FixerRendersKnownColumnsInExistingSchema(t *testing.T) {
	root := setupSpecTree(t, map[string]string{
		"features/parent/README.md": "# Feature: Parent\n\n## Contents\n\n" +
			"| Child | Kind | URL | Description | Status |\n" +
			"|---|---|---|---|---|\n",
		"features/parent/alpha/README.md": "# Feature: Alpha\n\n**Status:** Draft\n",
		"features/parent/catalog-index/README.md": "# Feature: Catalog\n\n**Status:** Stable\n\n" +
			"---\n*This document follows the https://specscore.md/feature-specification*\n",
	})

	checker := newIndexEntriesChecker().(*indexEntriesChecker)
	if err := checker.fix(root); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "features", "parent", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	wants := []string{
		"| [alpha](alpha/README.md) | — | — | TODO: Add description. | Draft |",
		"| [catalog-index](catalog-index/README.md) | Index | https://specscore.md/feature-specification | TODO: Add description. | Stable |",
	}
	for _, want := range wants {
		if !strings.Contains(string(got), want) {
			t.Fatalf("schema-shaped row %q missing:\n%s", want, got)
		}
	}
}

func TestIndexEntryHelpersRejectUnsafeCellsAndParseSpecURLs(t *testing.T) {
	if _, ok := directChildRefFromIndexRow("| [real](real/README.md) |", -1); ok {
		t.Fatal("negative identity column must be rejected")
	}
	if _, ok := directChildRefFromIndexRow("| [real](real/README.md) |", 2); ok {
		t.Fatal("out-of-range identity column must be rejected")
	}

	actual := map[string]bool{"real": true}
	content := "## Contents\n\n| Child | Description |\n|---|---|\n" +
		"| [ghost](ghost/README.md) | phantom |\n" +
		"| [real](real/README.md) | valid |\n"
	rewritten, changed := dropPhantomIndexRows(content, actual)
	if !changed || strings.Contains(rewritten, "ghost") || !strings.Contains(rewritten, "real") {
		t.Fatalf("schema-aware phantom wrapper did not preserve only the real row:\n%s", rewritten)
	}
	unchanged, changed := dropPhantomIndexRows(rewritten, actual)
	if changed || unchanged != rewritten {
		t.Fatalf("phantom wrapper must be idempotent:\n%s", unchanged)
	}

	dir := t.TempDir()
	write := func(name, body string) string {
		t.Helper()
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
	tests := []struct {
		name string
		path string
		want string
	}{
		{name: "missing file", path: filepath.Join(dir, "missing.md")},
		{name: "missing marker", path: write("no-marker.md", "# Feature\n")},
		{name: "unterminated marker", path: write("unterminated.md", "*This document follows the https://example.test/spec\n")},
		{name: "invalid scheme", path: write("invalid.md", "*This document follows the ftp://example.test/spec*\n")},
		{name: "valid", path: write("valid.md", "*This document follows the https://example.test/spec*\n"), want: "https://example.test/spec"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := childSpecURL(tt.path); got != tt.want {
				t.Fatalf("childSpecURL() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestIndexEntries_NoTableWriteFailureIsReturned(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("permission behavior requires a non-root user")
	}
	root := setupSpecTree(t, map[string]string{
		"features/README.md":       "# Features\n",
		"features/child/README.md": "# Feature: Child\n\n**Status:** Draft\n",
	})
	indexPath := filepath.Join(root, "features", "README.md")
	if err := os.Chmod(indexPath, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(indexPath, 0o644) })

	err := newIndexEntriesChecker().(*indexEntriesChecker).fix(root)
	if err == nil {
		t.Fatal("write failure for a newly scaffolded index was swallowed")
	}
}

func TestACHeadingCheck_PropagatesFeatureWalkFailure(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("permission behavior requires a non-root user")
	}
	root := setupSpecTree(t, map[string]string{
		"features/blocked/README.md": "# Feature: Blocked\n",
		"features/blocked/child.txt": "forces directory enumeration\n",
	})
	blocked := filepath.Join(root, "features", "blocked")
	if err := os.Chmod(blocked, 0o111); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(blocked, 0o755) })

	if _, err := newACHeadingFormatChecker().check(root); err == nil {
		t.Fatal("feature-walk failure was swallowed")
	}
}
