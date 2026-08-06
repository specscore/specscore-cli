package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validDecisionsIndex() string {
	return `# SpecScore Decisions

Some intro text.

## Decisions

| # | Decision | Status | Date | Tags | Affected |
|---|----------|--------|------|------|----------|
| [0001](0001-test.md) | Test Decision | Approved | 2026-05-20 | — | — |

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/decisions-index-specification*
`
}

func TestDecisionsIndexListSectionHeading(t *testing.T) {
	t.Run("valid heading passes", func(t *testing.T) {
		root := setupDecisionTestTree(t, map[string]string{
			"decisions/README.md":    validDecisionsIndex(),
			"decisions/0001-test.md": acceptedDecisionContent(),
		})
		vs, err := checkDecisionsIndex(root, false)
		if err != nil {
			t.Fatal(err)
		}
		if hasDecisionViolation(vs, "DI-list-section-heading", "") {
			t.Error("expected no DI-list-section-heading violation")
		}
	})

	t.Run("wrong heading rejected", func(t *testing.T) {
		content := `# Decisions

## Decision Records

| # | Decision | Status | Date | Tags | Affected |
|---|----------|--------|------|------|----------|

## Open Questions

None.
`
		root := setupDecisionTestTree(t, map[string]string{
			"decisions/README.md": content,
		})
		vs, err := checkDecisionsIndex(root, false)
		if err != nil {
			t.Fatal(err)
		}
		if !hasDecisionViolation(vs, "DI-list-section-heading", "## Decisions") {
			t.Error("expected DI-list-section-heading violation for wrong heading")
		}
	})
}

func TestDecisionsIndexColumns(t *testing.T) {
	t.Run("missing table rejected", func(t *testing.T) {
		content := `# Decisions

## Decisions

No table here.

## Open Questions

None.
`
		root := setupDecisionTestTree(t, map[string]string{
			"decisions/README.md": content,
		})
		vs, err := checkDecisionsIndex(root, false)
		if err != nil {
			t.Fatal(err)
		}
		if !hasDecisionViolation(vs, "DI-index-columns", "") {
			t.Error("expected DI-index-columns violation for missing table")
		}
	})
}

func TestDecisionsIndexStatusExcludesArchived(t *testing.T) {
	t.Run("Superseded row rejected", func(t *testing.T) {
		content := `# Decisions

## Decisions

| # | Decision | Status | Date | Tags | Affected |
|---|----------|--------|------|------|----------|
| [0001](0001-test.md) | Old Decision | Superseded | 2026-05-20 | — | — |

## Open Questions

None.
`
		root := setupDecisionTestTree(t, map[string]string{
			"decisions/README.md": content,
		})
		vs, err := checkDecisionsIndex(root, false)
		if err != nil {
			t.Fatal(err)
		}
		if !hasDecisionViolation(vs, "DI-status-excludes-archived", "Superseded") {
			t.Error("expected DI-status-excludes-archived violation")
		}
	})
}

func TestDecisionsIndexNumericOrdering(t *testing.T) {
	t.Run("out of order rejected", func(t *testing.T) {
		content := `# Decisions

## Decisions

| # | Decision | Status | Date | Tags | Affected |
|---|----------|--------|------|------|----------|
| [0002](0002-second.md) | Second | Draft | 2026-05-26 | — | — |
| [0001](0001-first.md) | First | Approved | 2026-05-20 | — | — |

## Open Questions

None.
`
		root := setupDecisionTestTree(t, map[string]string{
			"decisions/README.md":      content,
			"decisions/0001-first.md":  acceptedDecisionContent(),
			"decisions/0002-second.md": validDecisionContent(),
		})
		vs, err := checkDecisionsIndex(root, false)
		if err != nil {
			t.Fatal(err)
		}
		if !hasDecisionViolation(vs, "DI-numeric-ordering", "ascending") {
			t.Error("expected DI-numeric-ordering violation")
		}
	})

	t.Run("fix reorders rows", func(t *testing.T) {
		content := `# Decisions

## Decisions

| # | Decision | Status | Date | Tags | Affected |
|---|----------|--------|------|------|----------|
| [0002](0002-second.md) | Second | Draft | 2026-05-26 | — | — |
| [0001](0001-first.md) | First | Approved | 2026-05-20 | — | — |

## Open Questions

None.
`
		root := setupDecisionTestTree(t, map[string]string{
			"decisions/README.md":      content,
			"decisions/0001-first.md":  acceptedDecisionContent(),
			"decisions/0002-second.md": validDecisionContent(),
		})
		vs, err := checkDecisionsIndex(root, true)
		if err != nil {
			t.Fatal(err)
		}
		if hasDecisionViolation(vs, "DI-numeric-ordering", "") {
			t.Error("expected fix to resolve DI-numeric-ordering violation")
		}

		// Verify the file was rewritten
		fixed, err := os.ReadFile(filepath.Join(root, "decisions", "README.md"))
		if err != nil {
			t.Fatal(err)
		}
		fixedStr := string(fixed)
		idx1 := strings.Index(fixedStr, "0001")
		idx2 := strings.Index(fixedStr, "0002")
		if idx1 < 0 || idx2 < 0 || idx1 > idx2 {
			t.Error("expected 0001 before 0002 after fix")
		}
	})
}

func TestDecisionsIndexCompleteness(t *testing.T) {
	t.Run("missing entry detected", func(t *testing.T) {
		content := `# Decisions

## Decisions

| # | Decision | Status | Date | Tags | Affected |
|---|----------|--------|------|------|----------|

## Open Questions

None.
`
		root := setupDecisionTestTree(t, map[string]string{
			"decisions/README.md":    content,
			"decisions/0001-test.md": validDecisionContent(),
		})
		vs, err := checkDecisionsIndex(root, false)
		if err != nil {
			t.Fatal(err)
		}
		if !hasDecisionViolation(vs, "DI-completeness", "0001-test") {
			t.Error("expected DI-completeness violation for missing entry")
		}
	})

	t.Run("archived decisions not required in active index", func(t *testing.T) {
		content := `# Decisions

## Decisions

| # | Decision | Status | Date | Tags | Affected |
|---|----------|--------|------|------|----------|

## Open Questions

None.
`
		archivedContent := `# Decision: Archived

**Status:** Superseded
**Date:** 2026-05-20
**Owner:** test
**Tags:** —
**Source Idea:** —
**Supersedes:** —
**Superseded By:** 0002-new

## Context

C.

## Decision

D.

## Rationale

R.

## Declined Alternatives

### Alt

No.

## Consequences at Decision Time

C.

## Observed Consequences

None observed yet.

## Affected Features

None at this time.

---
*This document follows the https://specscore.md/decision-specification*
`
		root := setupDecisionTestTree(t, map[string]string{
			"decisions/README.md":            content,
			"decisions/archived/0001-old.md": archivedContent,
		})
		vs, err := checkDecisionsIndex(root, false)
		if err != nil {
			t.Fatal(err)
		}
		if hasDecisionViolation(vs, "DI-completeness", "") {
			t.Error("archived decisions should not be required in active index")
		}
	})
}

func TestActiveDecisionsIndexFixSynchronisesRenamedDecisionRow(t *testing.T) {
	decision := strings.Replace(validDecisionContent(), "# Decision: Test Decision", "# Decision: Competios Naming", 1)
	decision = strings.Replace(decision, "**Date:** 2026-05-26", "**Date:** 2026-08-06", 1)
	decision = strings.Replace(decision, "**Tags:** —", "**Tags:** naming, migration", 1)
	decision = strings.Replace(decision, "None at this time.\n\n---", "- cli\n- website\n\n---", 1)
	index := strings.Replace(validDecisionsIndex(), "Test Decision | Approved | 2026-05-20 | — | —", "Old Product Name | Draft | 2020-01-01 | old | legacy", 1)
	root := setupDecisionTestTree(t, map[string]string{"decisions/README.md": index, "decisions/0001-test.md": decision})
	vs, err := checkDecisionsIndex(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if !hasDecisionViolation(vs, "DI-row-content-sync", "0001-test") {
		t.Fatalf("expected stale decision row violation, got %+v", vs)
	}
	vs, err = checkDecisionsIndex(root, true)
	if err != nil {
		t.Fatal(err)
	}
	if hasDecisionViolation(vs, "DI-row-content-sync", "") {
		t.Fatalf("fix left row sync violation: %+v", vs)
	}
	data, err := os.ReadFile(filepath.Join(root, "decisions", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Competios Naming | Draft | 2026-08-06 | naming, migration | cli, website") {
		t.Fatalf("decision index row was not canonicalised:\n%s", data)
	}
	vs, err = checkDecisionsIndex(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if hasDecisionViolation(vs, "DI-row-content-sync", "") {
		t.Fatalf("re-lint found stale row: %+v", vs)
	}
}

func TestActiveDecisionsIndexFixEscapesDerivedPipes(t *testing.T) {
	const index = `# Decisions

## Decisions

| # | Decision | Status | Date | Tags | Affected |
|---|----------|--------|------|------|----------|

## Open Questions

None.
`
	decision := strings.Replace(validDecisionContent(), "# Decision: Test Decision", "# Decision: Creator | Competitor", 1)
	decision = strings.Replace(decision, "**Status:** Draft", "**Status:** Draft | Scheduled", 1)
	decision = strings.Replace(decision, "**Date:** 2026-05-26", "**Date:** 2026-05-26 | provisional", 1)
	decision = strings.Replace(decision, "**Tags:** —", "**Tags:** creators | chess", 1)
	decision = strings.Replace(decision, "None at this time.\n\n---", "- products | creators\n\n---", 1)
	root := setupDecisionTestTree(t, map[string]string{"decisions/README.md": index, "decisions/0001-test.md": decision})

	if _, err := checkDecisionsIndex(root, true); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(root, "decisions", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "Creator \\| Competitor | Draft \\| Scheduled | 2026-05-26 \\| provisional | creators \\| chess | products \\| creators |") {
		t.Fatalf("derived decision cells did not escape pipes:\n%s", data)
	}
	vs, err := checkDecisionsIndex(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if hasDecisionViolation(vs, "DI-row-content-sync", "") {
		t.Fatalf("escaped row should be clean on re-lint, got %+v", vs)
	}
}

func TestDecisionsIndexArchivedCompleteness(t *testing.T) {
	// An archived decision file with no row in the archived index.
	archivedIndex := "# Archived Decisions\n\n"
	decision := strings.Replace(acceptedDecisionContent(), "**Status:** Approved", "**Status:** Superseded", 1)
	root := setupDecisionTestTree(t, map[string]string{
		"decisions/archived/README.md":   archivedIndex,
		"decisions/archived/0001-old.md": decision,
	})
	vs, err := checkDecisionsIndex(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if !hasDecisionViolation(vs, "DI-completeness", "archived decisions index missing") {
		t.Errorf("expected archived DI-completeness violation, got: %+v", vs)
	}
}

func TestDecisionsIndexArchivedStatusExcludesActive(t *testing.T) {
	// An active (Approved) decision wrongly listed in the archived index.
	archivedIndex := "# Archived Decisions\n\n" +
		"- 2026-05-20 — [0001-active](0001-active.md) — Approved — listed by mistake\n"
	root := setupDecisionTestTree(t, map[string]string{
		"decisions/archived/README.md": archivedIndex,
		"decisions/0001-active.md":     acceptedDecisionContent(),
	})
	vs, err := checkDecisionsIndex(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if !hasDecisionViolation(vs, "DI-archived-status-excludes-active", "Approved") {
		t.Errorf("expected DI-archived-status-excludes-active violation, got: %+v", vs)
	}
}

func TestDecisionsIndexArchivedChronological(t *testing.T) {
	t.Run("out of order rejected", func(t *testing.T) {
		content := `# Archived Decisions

- 2026-05-26 — [0002-newer](0002-newer.md) — Superseded — → D-0003
- 2026-05-20 — [0001-older](0001-older.md) — Deprecated — no longer applies
`
		root := setupDecisionTestTree(t, map[string]string{
			"decisions/archived/README.md": content,
		})
		vs, err := checkDecisionsIndex(root, false)
		if err != nil {
			t.Fatal(err)
		}
		if !hasDecisionViolation(vs, "DI-archived-index-chronological", "chronological") {
			t.Error("expected DI-archived-index-chronological violation")
		}
	})

	t.Run("fix reorders entries", func(t *testing.T) {
		content := `# Archived Decisions

- 2026-05-26 — [0002-newer](0002-newer.md) — Superseded — → D-0003
- 2026-05-20 — [0001-older](0001-older.md) — Deprecated — no longer applies
`
		root := setupDecisionTestTree(t, map[string]string{
			"decisions/archived/README.md": content,
		})
		vs, err := checkDecisionsIndex(root, true)
		if err != nil {
			t.Fatal(err)
		}
		if hasDecisionViolation(vs, "DI-archived-index-chronological", "") {
			t.Error("expected fix to resolve chronological violation")
		}

		fixed, err := os.ReadFile(filepath.Join(root, "decisions", "archived", "README.md"))
		if err != nil {
			t.Fatal(err)
		}
		fixedStr := string(fixed)
		idx1 := strings.Index(fixedStr, "2026-05-20")
		idx2 := strings.Index(fixedStr, "2026-05-26")
		if idx1 < 0 || idx2 < 0 || idx1 > idx2 {
			t.Error("expected 2026-05-20 before 2026-05-26 after fix")
		}
	})
}

func TestDecisionsIndexNoDecisionsDir(t *testing.T) {
	root := t.TempDir()
	vs, err := checkDecisionsIndex(root, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) > 0 {
		t.Errorf("expected no violations when no decisions dir, got %d", len(vs))
	}
}

func TestParseDecisionsIndexRow_RejectsMalformedReferences(t *testing.T) {
	tests := []string{
		"| 0001](0001-test.md) | Title | Draft | 2026-08-06 | — | — |",
		"| [0001](0001-test.txt) | Title | Draft | 2026-08-06 | — | — |",
		"| [abcd](0001-test.md) | Title | Draft | 2026-08-06 | — | — |",
	}
	for _, line := range tests {
		if _, ok := parseDecisionsIndexRow(line); ok {
			t.Errorf("malformed decision reference parsed: %q", line)
		}
	}
}
