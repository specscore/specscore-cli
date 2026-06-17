package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func validDecisionContent() string {
	return `# Decision: Test Decision

**Status:** Draft
**Date:** 2026-05-26
**Owner:** test@example.com
**Tags:** —
**Source Idea:** —
**Supersedes:** —
**Superseded By:** —

## Context

Some context here.

## Decision

We chose option A.

## Rationale

Because reasons.

## Declined Alternatives

### Option B

Too expensive.

## Consequences at Decision Time

Expected good things.

## Observed Consequences

None observed yet.

## Affected Features

None at this time.

---
*This document follows the https://specscore.md/decision-specification*
`
}

func acceptedDecisionContent() string {
	return `# Decision: Accepted Decision

**Status:** Approved
**Date:** 2026-05-20
**Owner:** test@example.com
**Tags:** tag1, tag2
**Source Idea:** —
**Supersedes:** —
**Superseded By:** —

## Context

Some context.

## Decision

We chose A.

## Rationale

Good reasons.

## Declined Alternatives

### Option B

Not good enough.

## Consequences at Decision Time

Expected positive outcomes.

## Observed Consequences

None observed yet.

## Affected Features

None at this time.

---
*This document follows the https://specscore.md/decision-specification*
`
}

// TestDecisionReverseCheckSkipsDecisionWithoutSupersededByField covers the
// reverse-supersession loop's branch where a decision file omits the
// "Superseded By" field entirely (the fieldByName lookup is !ok → continue),
// so it must not trigger any D-supersedes-bidirectional violation.
func TestDecisionReverseCheckSkipsDecisionWithoutSupersededByField(t *testing.T) {
	content := strings.Replace(validDecisionContent(), "**Superseded By:** —\n", "", 1)
	root := setupDecisionTestTree(t, map[string]string{
		"decisions/0001-a.md": content,
	})
	vs, err := checkDecisions(root)
	if err != nil {
		t.Fatal(err)
	}
	if hasDecisionViolation(vs, "D-supersedes-bidirectional", "") {
		t.Errorf("a decision without a Superseded By field must be skipped by the reverse check, got: %+v", vs)
	}
}

// TestDecisionSupersededByReverse covers the reverse direction of the
// bidirectional supersession check: a "Superseded By: X" reference whose
// target is missing (orphan) or does not point back (mismatch). The forward
// check is driven by Supersedes and does not cover these.
func TestDecisionSupersededByReverse(t *testing.T) {
	t.Run("orphan Superseded By rejected", func(t *testing.T) {
		content := strings.Replace(validDecisionContent(), "**Status:** Draft", "**Status:** Superseded", 1)
		content = strings.Replace(content, "**Superseded By:** —", "**Superseded By:** 0099-ghost", 1)
		root := setupDecisionTestTree(t, map[string]string{
			"decisions/archived/0001-a.md": content,
		})
		vs, err := checkDecisions(root)
		if err != nil {
			t.Fatal(err)
		}
		if !hasDecisionViolation(vs, "D-supersedes-bidirectional", "does not exist") {
			t.Errorf("expected orphan D-supersedes-bidirectional violation, got: %+v", vs)
		}
	})

	t.Run("back-reference mismatch rejected", func(t *testing.T) {
		// A claims it was superseded by B, but B's Supersedes does not point back.
		a := strings.Replace(validDecisionContent(), "**Status:** Draft", "**Status:** Superseded", 1)
		a = strings.Replace(a, "**Superseded By:** —", "**Superseded By:** 0002-b", 1)
		b := validDecisionContent() // B.Supersedes stays "—" — does not point back to A.
		root := setupDecisionTestTree(t, map[string]string{
			"decisions/archived/0001-a.md": a,
			"decisions/0002-b.md":          b,
		})
		vs, err := checkDecisions(root)
		if err != nil {
			t.Fatal(err)
		}
		if !hasDecisionViolation(vs, "D-supersedes-bidirectional", "expected \"0001-a\"") {
			t.Errorf("expected mismatch D-supersedes-bidirectional violation, got: %+v", vs)
		}
	})
}

func setupDecisionTestTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for relPath, content := range files {
		fullPath := filepath.Join(root, relPath)
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func hasDecisionViolation(vs []Violation, rule, substr string) bool {
	for _, v := range vs {
		if v.Rule == rule && (substr == "" || contains(v.Message, substr)) {
			return true
		}
	}
	return false
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (sub == "" || containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

func TestDecisionTitleFormat(t *testing.T) {
	t.Run("valid title passes", func(t *testing.T) {
		root := setupDecisionTestTree(t, map[string]string{
			"decisions/0001-test.md": validDecisionContent(),
		})
		vs, err := checkDecisions(root)
		if err != nil {
			t.Fatal(err)
		}
		if hasDecisionViolation(vs, "D-title-format", "") {
			t.Error("expected no D-title-format violation for valid title")
		}
	})

	t.Run("missing Decision prefix rejected", func(t *testing.T) {
		content := "# Some Title Without Prefix\n\n**Status:** Draft\n**Date:** 2026-05-26\n**Owner:** test\n**Tags:** —\n**Source Idea:** —\n**Supersedes:** —\n**Superseded By:** —\n\n## Context\n\nCtx.\n\n## Decision\n\nD.\n\n## Rationale\n\nR.\n\n## Declined Alternatives\n\n### Alt\n\nNo.\n\n## Consequences at Decision Time\n\nC.\n\n## Observed Consequences\n\nNone observed yet.\n\n## Affected Features\n\nNone at this time.\n\n---\n*This document follows the https://specscore.md/decision-specification*\n"
		root := setupDecisionTestTree(t, map[string]string{
			"decisions/0001-test.md": content,
		})
		vs, err := checkDecisions(root)
		if err != nil {
			t.Fatal(err)
		}
		if !hasDecisionViolation(vs, "D-title-format", "Decision:") {
			t.Error("expected D-title-format violation for missing prefix")
		}
	})
}

func TestDecisionHeaderFields(t *testing.T) {
	t.Run("all fields present passes", func(t *testing.T) {
		root := setupDecisionTestTree(t, map[string]string{
			"decisions/0001-test.md": validDecisionContent(),
		})
		vs, err := checkDecisions(root)
		if err != nil {
			t.Fatal(err)
		}
		if hasDecisionViolation(vs, "D-header-fields", "") {
			t.Error("expected no D-header-fields violation")
		}
	})

	t.Run("missing Tags field rejected", func(t *testing.T) {
		content := `# Decision: Test

**Status:** Draft
**Date:** 2026-05-26
**Owner:** test
**Source Idea:** —
**Supersedes:** —
**Superseded By:** —

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
			"decisions/0001-test.md": content,
		})
		vs, err := checkDecisions(root)
		if err != nil {
			t.Fatal(err)
		}
		if !hasDecisionViolation(vs, "D-header-fields", "Tags") {
			t.Error("expected D-header-fields violation for missing Tags")
		}
	})

	t.Run("fields out of order rejected", func(t *testing.T) {
		content := `# Decision: Test

**Date:** 2026-05-26
**Status:** Draft
**Owner:** test
**Tags:** —
**Source Idea:** —
**Supersedes:** —
**Superseded By:** —

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
			"decisions/0001-test.md": content,
		})
		vs, err := checkDecisions(root)
		if err != nil {
			t.Fatal(err)
		}
		if !hasDecisionViolation(vs, "D-header-fields", "canonical order") {
			t.Error("expected D-header-fields violation for wrong order")
		}
	})
}

func TestDecisionStatusValues(t *testing.T) {
	t.Run("valid status passes", func(t *testing.T) {
		root := setupDecisionTestTree(t, map[string]string{
			"decisions/0001-test.md": validDecisionContent(),
		})
		vs, err := checkDecisions(root)
		if err != nil {
			t.Fatal(err)
		}
		if hasDecisionViolation(vs, "D-status-values", "") {
			t.Error("expected no D-status-values violation")
		}
	})

	t.Run("invalid status rejected", func(t *testing.T) {
		content := `# Decision: Test

**Status:** Bogus
**Date:** 2026-05-26
**Owner:** test
**Tags:** —
**Source Idea:** —
**Supersedes:** —
**Superseded By:** —

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
			"decisions/0001-test.md": content,
		})
		vs, err := checkDecisions(root)
		if err != nil {
			t.Fatal(err)
		}
		if !hasDecisionViolation(vs, "D-status-values", "Bogus") {
			t.Error("expected D-status-values violation for Bogus")
		}
	})
}

func TestDecisionFilenameFormat(t *testing.T) {
	t.Run("valid filename passes", func(t *testing.T) {
		root := setupDecisionTestTree(t, map[string]string{
			"decisions/0001-test-slug.md": validDecisionContent(),
		})
		vs, err := checkDecisions(root)
		if err != nil {
			t.Fatal(err)
		}
		if hasDecisionViolation(vs, "D-filename-format", "") {
			t.Error("expected no D-filename-format violation")
		}
	})

	t.Run("invalid filename rejected", func(t *testing.T) {
		root := setupDecisionTestTree(t, map[string]string{
			"decisions/bad-name.md": validDecisionContent(),
		})
		vs, err := checkDecisions(root)
		if err != nil {
			t.Fatal(err)
		}
		if !hasDecisionViolation(vs, "D-filename-format", "") {
			t.Error("expected D-filename-format violation for bad filename")
		}
	})
}

func TestDecisionSingleFile(t *testing.T) {
	t.Run("directory rejected", func(t *testing.T) {
		root := setupDecisionTestTree(t, map[string]string{
			"decisions/0001-test/README.md": "# Decision: Bad\n",
		})
		vs, err := checkDecisions(root)
		if err != nil {
			t.Fatal(err)
		}
		if !hasDecisionViolation(vs, "D-single-file", "directory") {
			t.Error("expected D-single-file violation for directory")
		}
	})
}

func TestDecisionRequiredSections(t *testing.T) {
	t.Run("all sections present passes", func(t *testing.T) {
		root := setupDecisionTestTree(t, map[string]string{
			"decisions/0001-test.md": validDecisionContent(),
		})
		vs, err := checkDecisions(root)
		if err != nil {
			t.Fatal(err)
		}
		if hasDecisionViolation(vs, "D-required-sections", "") {
			t.Error("expected no D-required-sections violation")
		}
	})

	t.Run("missing Rationale rejected", func(t *testing.T) {
		content := `# Decision: Test

**Status:** Draft
**Date:** 2026-05-26
**Owner:** test
**Tags:** —
**Source Idea:** —
**Supersedes:** —
**Superseded By:** —

## Context

C.

## Decision

D.

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
			"decisions/0001-test.md": content,
		})
		vs, err := checkDecisions(root)
		if err != nil {
			t.Fatal(err)
		}
		if !hasDecisionViolation(vs, "D-required-sections", "Rationale") {
			t.Error("expected D-required-sections violation for missing Rationale")
		}
	})
}

func TestDecisionDeclinedAlternatives(t *testing.T) {
	t.Run("no h3 entries rejected", func(t *testing.T) {
		content := `# Decision: Test

**Status:** Draft
**Date:** 2026-05-26
**Owner:** test
**Tags:** —
**Source Idea:** —
**Supersedes:** —
**Superseded By:** —

## Context

C.

## Decision

D.

## Rationale

R.

## Declined Alternatives

We didn't consider any alternatives.

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
			"decisions/0001-test.md": content,
		})
		vs, err := checkDecisions(root)
		if err != nil {
			t.Fatal(err)
		}
		if !hasDecisionViolation(vs, "D-declined-alternatives-non-empty", "") {
			t.Error("expected D-declined-alternatives-non-empty violation")
		}
	})
}

func TestDecisionObservedConsequencesPlaceholder(t *testing.T) {
	t.Run("Draft with placeholder passes", func(t *testing.T) {
		root := setupDecisionTestTree(t, map[string]string{
			"decisions/0001-test.md": validDecisionContent(),
		})
		vs, err := checkDecisions(root)
		if err != nil {
			t.Fatal(err)
		}
		if hasDecisionViolation(vs, "D-observed-consequences-placeholder", "") {
			t.Error("expected no D-observed-consequences-placeholder violation")
		}
	})

	t.Run("Draft without placeholder rejected", func(t *testing.T) {
		content := `# Decision: Test

**Status:** Draft
**Date:** 2026-05-26
**Owner:** test
**Tags:** —
**Source Idea:** —
**Supersedes:** —
**Superseded By:** —

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

Something already observed.

## Affected Features

None at this time.

---
*This document follows the https://specscore.md/decision-specification*
`
		root := setupDecisionTestTree(t, map[string]string{
			"decisions/0001-test.md": content,
		})
		vs, err := checkDecisions(root)
		if err != nil {
			t.Fatal(err)
		}
		if !hasDecisionViolation(vs, "D-observed-consequences-placeholder", "None observed yet") {
			t.Error("expected D-observed-consequences-placeholder violation")
		}
	})

	t.Run("Approved without placeholder is fine", func(t *testing.T) {
		root := setupDecisionTestTree(t, map[string]string{
			"decisions/0001-test.md": acceptedDecisionContent(),
		})
		vs, err := checkDecisions(root)
		if err != nil {
			t.Fatal(err)
		}
		if hasDecisionViolation(vs, "D-observed-consequences-placeholder", "") {
			t.Error("Approved decisions should not require the placeholder")
		}
	})
}

func TestDecisionArchivedLocation(t *testing.T) {
	t.Run("Superseded in active rejected", func(t *testing.T) {
		content := `# Decision: Test

**Status:** Superseded
**Date:** 2026-05-20
**Owner:** test
**Tags:** —
**Source Idea:** —
**Supersedes:** —
**Superseded By:** 0002-replacement

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
			"decisions/0001-test.md": content,
		})
		vs, err := checkDecisions(root)
		if err != nil {
			t.Fatal(err)
		}
		if !hasDecisionViolation(vs, "D-archived-location", "archived") {
			t.Error("expected D-archived-location violation for Superseded in active dir")
		}
	})

	t.Run("Draft in archived rejected", func(t *testing.T) {
		root := setupDecisionTestTree(t, map[string]string{
			"decisions/archived/0001-test.md": validDecisionContent(),
		})
		vs, err := checkDecisions(root)
		if err != nil {
			t.Fatal(err)
		}
		if !hasDecisionViolation(vs, "D-archived-location", "") {
			t.Error("expected D-archived-location violation for Draft in archived dir")
		}
	})
}

func TestDecisionSupersededRequiresSuccessor(t *testing.T) {
	t.Run("Superseded without successor rejected", func(t *testing.T) {
		content := `# Decision: Test

**Status:** Superseded
**Date:** 2026-05-20
**Owner:** test
**Tags:** —
**Source Idea:** —
**Supersedes:** —
**Superseded By:** —

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
			"decisions/archived/0001-test.md": content,
		})
		vs, err := checkDecisions(root)
		if err != nil {
			t.Fatal(err)
		}
		if !hasDecisionViolation(vs, "D-superseded-requires-successor", "non-empty") {
			t.Error("expected D-superseded-requires-successor violation")
		}
	})
}

func TestDecisionSupersedesTargetExists(t *testing.T) {
	t.Run("missing target rejected", func(t *testing.T) {
		content := `# Decision: Replacement

**Status:** Draft
**Date:** 2026-05-26
**Owner:** test
**Tags:** —
**Source Idea:** —
**Supersedes:** 0001-old-decision
**Superseded By:** —

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
			"decisions/0002-replacement.md": content,
		})
		vs, err := checkDecisions(root)
		if err != nil {
			t.Fatal(err)
		}
		if !hasDecisionViolation(vs, "D-supersedes-target-exists", "0001-old-decision") {
			t.Error("expected D-supersedes-target-exists violation")
		}
	})
}

func TestDecisionSupersedesBidirectional(t *testing.T) {
	t.Run("bidirectional consistency enforced", func(t *testing.T) {
		old := `# Decision: Old

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
		new := `# Decision: New

**Status:** Draft
**Date:** 2026-05-26
**Owner:** test
**Tags:** —
**Source Idea:** —
**Supersedes:** 0001-old
**Superseded By:** —

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
			"decisions/archived/0001-old.md": old,
			"decisions/0002-new.md":          new,
		})
		vs, err := checkDecisions(root)
		if err != nil {
			t.Fatal(err)
		}
		// Should pass — 0001 has Superseded By: 0002-new and is in archived/
		if hasDecisionViolation(vs, "D-supersedes-bidirectional", "") {
			t.Errorf("expected no bidirectional violation, got: %+v", vs)
		}
	})

	t.Run("drift detected", func(t *testing.T) {
		old := `# Decision: Old

**Status:** Approved
**Date:** 2026-05-20
**Owner:** test
**Tags:** —
**Source Idea:** —
**Supersedes:** —
**Superseded By:** —

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
		new := `# Decision: New

**Status:** Draft
**Date:** 2026-05-26
**Owner:** test
**Tags:** —
**Source Idea:** —
**Supersedes:** 0001-old
**Superseded By:** —

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
			"decisions/0001-old.md": old,
			"decisions/0002-new.md": new,
		})
		vs, err := checkDecisions(root)
		if err != nil {
			t.Fatal(err)
		}
		if !hasDecisionViolation(vs, "D-supersedes-bidirectional", "") {
			t.Error("expected D-supersedes-bidirectional violation for drift")
		}
	})
}

func TestDecisionAffectedFeaturesTargetExists(t *testing.T) {
	t.Run("nonexistent feature slug rejected", func(t *testing.T) {
		content := `# Decision: Test

**Status:** Draft
**Date:** 2026-05-26
**Owner:** test
**Tags:** —
**Source Idea:** —
**Supersedes:** —
**Superseded By:** —

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

- nonexistent-feature

---
*This document follows the https://specscore.md/decision-specification*
`
		root := setupDecisionTestTree(t, map[string]string{
			"decisions/0001-test.md": content,
		})
		vs, err := checkDecisions(root)
		if err != nil {
			t.Fatal(err)
		}
		if !hasDecisionViolation(vs, "D-affected-features-target-exists", "nonexistent-feature") {
			t.Error("expected D-affected-features-target-exists violation")
		}
	})

	t.Run("existing feature passes", func(t *testing.T) {
		content := `# Decision: Test

**Status:** Draft
**Date:** 2026-05-26
**Owner:** test
**Tags:** —
**Source Idea:** —
**Supersedes:** —
**Superseded By:** —

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

- my-feature

---
*This document follows the https://specscore.md/decision-specification*
`
		root := setupDecisionTestTree(t, map[string]string{
			"decisions/0001-test.md":        content,
			"features/my-feature/README.md": "# Feature: My Feature\n",
		})
		vs, err := checkDecisions(root)
		if err != nil {
			t.Fatal(err)
		}
		if hasDecisionViolation(vs, "D-affected-features-target-exists", "") {
			t.Error("expected no D-affected-features-target-exists violation for existing feature")
		}
	})

	t.Run("None at this time passes", func(t *testing.T) {
		root := setupDecisionTestTree(t, map[string]string{
			"decisions/0001-test.md": validDecisionContent(),
		})
		vs, err := checkDecisions(root)
		if err != nil {
			t.Fatal(err)
		}
		if hasDecisionViolation(vs, "D-affected-features-target-exists", "") {
			t.Error("expected no violation for 'None at this time.'")
		}
	})
}

func TestDecisionSourceIdeaOptional(t *testing.T) {
	t.Run("dash value passes", func(t *testing.T) {
		root := setupDecisionTestTree(t, map[string]string{
			"decisions/0001-test.md": validDecisionContent(),
		})
		vs, err := checkDecisions(root)
		if err != nil {
			t.Fatal(err)
		}
		if hasDecisionViolation(vs, "D-source-idea-optional", "") {
			t.Error("expected no violation for Source Idea: —")
		}
	})

	t.Run("nonexistent idea rejected", func(t *testing.T) {
		content := `# Decision: Test

**Status:** Draft
**Date:** 2026-05-26
**Owner:** test
**Tags:** —
**Source Idea:** nonexistent-idea
**Supersedes:** —
**Superseded By:** —

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
			"decisions/0001-test.md": content,
		})
		vs, err := checkDecisions(root)
		if err != nil {
			t.Fatal(err)
		}
		if !hasDecisionViolation(vs, "D-source-idea-optional", "nonexistent-idea") {
			t.Error("expected D-source-idea-optional violation")
		}
	})

	t.Run("existing idea passes", func(t *testing.T) {
		content := `# Decision: Test

**Status:** Draft
**Date:** 2026-05-26
**Owner:** test
**Tags:** —
**Source Idea:** my-idea
**Supersedes:** —
**Superseded By:** —

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
			"decisions/0001-test.md": content,
			"ideas/my-idea.md":       "# Idea: My Idea\n",
		})
		vs, err := checkDecisions(root)
		if err != nil {
			t.Fatal(err)
		}
		if hasDecisionViolation(vs, "D-source-idea-optional", "") {
			t.Error("expected no violation for existing idea")
		}
	})
}

func TestDecisionNumberAssignment(t *testing.T) {
	t.Run("sequential numbers pass", func(t *testing.T) {
		root := setupDecisionTestTree(t, map[string]string{
			"decisions/0001-first.md":  validDecisionContent(),
			"decisions/0002-second.md": validDecisionContent(),
		})
		vs, err := checkDecisions(root)
		if err != nil {
			t.Fatal(err)
		}
		if hasDecisionViolation(vs, "D-number-assignment", "") {
			t.Error("expected no D-number-assignment violation for sequential numbers")
		}
	})
}

func TestDecisionValidFilePassesAllRules(t *testing.T) {
	root := setupDecisionTestTree(t, map[string]string{
		"decisions/0001-test.md": validDecisionContent(),
	})
	vs, err := checkDecisions(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range vs {
		if v.Severity == "error" {
			t.Errorf("unexpected error violation: [%s] %s", v.Rule, v.Message)
		}
	}
}

func TestDecisionNoDecisionsDir(t *testing.T) {
	root := t.TempDir()
	vs, err := checkDecisions(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) > 0 {
		t.Errorf("expected no violations when no decisions dir, got %d", len(vs))
	}
}

func TestDecisionDeprecatedRequiresDashSupersededBy(t *testing.T) {
	content := `# Decision: Old

**Status:** Deprecated
**Date:** 2026-05-20
**Owner:** test
**Tags:** —
**Source Idea:** —
**Supersedes:** —
**Superseded By:** 0002-something

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
		"decisions/archived/0001-old.md": content,
	})
	vs, err := checkDecisions(root)
	if err != nil {
		t.Fatal(err)
	}
	if !hasDecisionViolation(vs, "D-superseded-requires-successor", "Deprecated") {
		t.Error("expected violation: Deprecated must have Superseded By: —")
	}
}

func TestDecisionCrossRepoAffectedFeaturesSkipped(t *testing.T) {
	content := `# Decision: Test

**Status:** Approved
**Date:** 2026-05-22
**Owner:** test
**Tags:** —
**Source Idea:** —
**Supersedes:** —
**Superseded By:** —

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

- specscore-studio-app/spec/features/studio-url-scheme — cross-repo reference

---
*This document follows the https://specscore.md/decision-specification*
`
	root := setupDecisionTestTree(t, map[string]string{
		"decisions/0001-test.md": content,
	})
	vs, err := checkDecisions(root)
	if err != nil {
		t.Fatal(err)
	}
	if hasDecisionViolation(vs, "D-affected-features-target-exists", "") {
		t.Error("cross-repo references should be skipped, not flagged")
	}
}
