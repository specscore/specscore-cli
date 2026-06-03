package lint

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilterBySeverity(t *testing.T) {
	violations := []Violation{
		{Severity: "error", Rule: "r1"},
		{Severity: "warning", Rule: "r2"},
		{Severity: "info", Rule: "r3"},
	}
	filtered := FilterBySeverity(violations, "warning")
	if len(filtered) != 2 {
		t.Errorf("expected 2, got %d", len(filtered))
	}
}

func TestFilterBySeverity_ErrorOnly(t *testing.T) {
	violations := []Violation{
		{Severity: "error", Rule: "readme-exists"},
		{Severity: "warning", Rule: "plan-hierarchy"},
		{Severity: "info", Rule: "diag"},
	}

	errOnly := FilterBySeverity(violations, "error")
	if len(errOnly) != 1 {
		t.Errorf("error filter: expected 1, got %d", len(errOnly))
	}

	all := FilterBySeverity(violations, "info")
	if len(all) != 3 {
		t.Errorf("info filter: expected 3, got %d", len(all))
	}
}

func TestAllRuleNames(t *testing.T) {
	rules := AllRuleNames()
	if len(rules) == 0 {
		t.Error("expected non-empty rule names")
	}
	// Verify known rules are present.
	expected := []string{"readme-exists", "oq-section", "plan-hierarchy", "plan-roi-metadata"}
	for _, name := range expected {
		if !rules[name] {
			t.Errorf("expected rule %q to be present", name)
		}
	}
}

func TestAllRuleNames_ReturnsCopy(t *testing.T) {
	rules := AllRuleNames()
	rules["bogus"] = true
	rules2 := AllRuleNames()
	if rules2["bogus"] {
		t.Error("AllRuleNames should return a copy, not the original map")
	}
}

func TestValidateRuleNames(t *testing.T) {
	if err := ValidateRuleNames(nil); err != nil {
		t.Errorf("unexpected error for nil: %v", err)
	}
	if err := ValidateRuleNames([]string{"readme-exists", "oq-section"}); err != nil {
		t.Errorf("valid rules should not error: %v", err)
	}
	if err := ValidateRuleNames([]string{"nonexistent-rule"}); err == nil {
		t.Error("expected error for unknown rule")
	}
	// view-link → studio-toolbar migration message
	// (studio-toolbar#req:studio-toolbar-lint-removes-view-link)
	err := ValidateRuleNames([]string{"view-link"})
	if err == nil {
		t.Fatal("expected migration error for view-link")
	}
	if !strings.Contains(err.Error(), "studio-toolbar") {
		t.Errorf("view-link error should name studio-toolbar as replacement; got %v", err)
	}
}

func TestViolation_JSONRoundTrip(t *testing.T) {
	v := Violation{
		File:     "features/cli/README.md",
		Line:     42,
		Severity: "error",
		Rule:     "oq-section",
		Message:  "Open Questions section not found",
	}
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var got Violation
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got != v {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, v)
	}
}

// --- readmeExistsChecker ---

func TestReadmeExists_AllPresent(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "README.md"), "# Root")
	mkdir(t, filepath.Join(root, "child"))
	writeFile(t, filepath.Join(root, "child", "README.md"), "# Child")

	c := newReadmeExistsChecker()
	v, err := c.check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 0 {
		t.Errorf("expected 0 violations, got %d: %v", len(v), v)
	}
}

func TestReadmeExists_Missing(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "README.md"), "# Root")
	mkdir(t, filepath.Join(root, "child"))

	c := newReadmeExistsChecker()
	v, err := c.check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(v), v)
	}
	if v[0].Rule != "readme-exists" {
		t.Errorf("expected rule readme-exists, got %s", v[0].Rule)
	}
}

func TestReadmeExists_SkipsHiddenDirs(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "README.md"), "# Root")
	mkdir(t, filepath.Join(root, ".hidden"))

	c := newReadmeExistsChecker()
	v, err := c.check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 0 {
		t.Errorf("expected 0 violations for hidden dir, got %d", len(v))
	}
}

// --- oqSectionChecker ---

func TestOQSection_Present(t *testing.T) {
	root := setupSpecTree(t, map[string]string{
		"features/cli/README.md": "# CLI\n\n## Open Questions\n\n- Should we add X?\n",
	})

	c := newOQSectionChecker()
	v, err := c.check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 0 {
		t.Errorf("expected 0 violations, got %d: %v", len(v), v)
	}
}

func TestOQSection_Missing(t *testing.T) {
	root := setupSpecTree(t, map[string]string{
		"features/cli/README.md": "# CLI\n\n## Summary\n\nSome text.\n",
	})

	c := newOQSectionChecker()
	v, err := c.check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(v), v)
	}
	if v[0].Rule != "oq-section" {
		t.Errorf("expected rule oq-section, got %s", v[0].Rule)
	}
}

func TestOQSection_Empty(t *testing.T) {
	root := setupSpecTree(t, map[string]string{
		"features/cli/README.md": "# CLI\n\n## Open Questions\n\n## Next Section\n",
	})

	c := newOQSectionChecker()
	v, err := c.check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 1 {
		t.Fatalf("expected 1 violation (oq-not-empty), got %d: %v", len(v), v)
	}
	if v[0].Rule != "oq-not-empty" {
		t.Errorf("expected rule oq-not-empty, got %s", v[0].Rule)
	}
	if v[0].Severity != "warning" {
		t.Errorf("expected severity warning, got %s", v[0].Severity)
	}
}

func TestOQSection_LegacyHeadingFlagged(t *testing.T) {
	root := setupSpecTree(t, map[string]string{
		"features/cli/README.md": "# CLI\n\n## Outstanding Questions\n\n- Should we add X?\n",
	})

	c := newOQSectionChecker()
	v, err := c.check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(v), v)
	}
	if v[0].Rule != "oq-section" {
		t.Errorf("expected rule oq-section, got %s", v[0].Rule)
	}
	if v[0].Severity != "error" {
		t.Errorf("expected severity error, got %s", v[0].Severity)
	}
	if !strings.Contains(v[0].Message, "Legacy heading") {
		t.Errorf("expected legacy-heading message, got: %s", v[0].Message)
	}
	if !strings.Contains(v[0].Message, "--fix") {
		t.Errorf("expected message to point at --fix, got: %s", v[0].Message)
	}
}

func TestOQSection_FixRewritesLegacyHeading(t *testing.T) {
	// Includes a prose mention of "Outstanding Questions" inside the body
	// to verify the autofix is line-scoped — prose MUST NOT be touched.
	body := "# CLI\n\n## Outstanding Questions\n\n- Should we rename Outstanding Questions?\n"
	root := setupSpecTree(t, map[string]string{
		"features/cli/README.md": body,
	})

	c := newOQSectionChecker().(fixer)
	if err := c.fix(root); err != nil {
		t.Fatalf("fix returned error: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(root, "features/cli/README.md"))
	if err != nil {
		t.Fatal(err)
	}
	want := "# CLI\n\n## Open Questions\n\n- Should we rename Outstanding Questions?\n"
	if string(got) != want {
		t.Errorf("autofix mismatch:\ngot:\n%s\nwant:\n%s", string(got), want)
	}

	// Idempotence: running fix again on the now-canonical file changes nothing.
	if err := c.fix(root); err != nil {
		t.Fatalf("second fix returned error: %v", err)
	}
	got2, err := os.ReadFile(filepath.Join(root, "features/cli/README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got2) != want {
		t.Errorf("second fix mutated file:\ngot:\n%s\nwant:\n%s", string(got2), want)
	}
}

func TestOQSection_ChecksRootSpecReadme(t *testing.T) {
	// Pre-broadening, the check walked spec/features/ and spec/plans/ only;
	// the root spec/README.md and sibling subtrees like spec/research/ were
	// invisible. Now every README.md under spec/ is in scope.
	root := setupSpecTree(t, map[string]string{
		"README.md":              "# Project\n\n## Summary\n\nNo OQ section.\n",
		"research/README.md":     "# Research\n\n## Summary\n\nNo OQ section either.\n",
		"features/cli/README.md": "# CLI\n\n## Open Questions\n\n- ok\n",
	})

	c := newOQSectionChecker()
	v, err := c.check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 2 {
		t.Fatalf("expected 2 violations (root + research), got %d: %v", len(v), v)
	}
	files := map[string]bool{}
	for _, vi := range v {
		files[vi.File] = true
		if vi.Rule != "oq-section" {
			t.Errorf("expected oq-section, got %s", vi.Rule)
		}
		if vi.Message != "Open Questions section not found" {
			t.Errorf("unexpected message: %s", vi.Message)
		}
	}
	if !files["README.md"] || !files["research/README.md"] {
		t.Errorf("expected violations for both root and research READMEs, got files: %v", files)
	}
}

func TestOQSection_FixWalksAllSpecMdFiles(t *testing.T) {
	// Pre-broadening, the fix walked features/plans/ideas only. Now any
	// .md file under spec/ — including spec/README.md, spec/research/foo.md,
	// and arbitrary sibling subtrees — gets the legacy-heading rewrite.
	root := setupSpecTree(t, map[string]string{
		"README.md":              "# Project\n\n## Outstanding Questions\n\n- root oq?\n",
		"research/notes.md":      "# Notes\n\n## Outstanding Questions\n\n- note?\n",
		"decisions/0001-x.md":    "# Decision 1\n\n## Outstanding Questions\n\n- d?\n",
		"features/cli/README.md": "# CLI\n\n## Outstanding Questions\n\n- cli?\n",
	})

	c := newOQSectionChecker().(fixer)
	if err := c.fix(root); err != nil {
		t.Fatalf("fix returned error: %v", err)
	}

	for _, rel := range []string{"README.md", "research/notes.md", "decisions/0001-x.md", "features/cli/README.md"} {
		got, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		if strings.Contains(string(got), "## Outstanding Questions") {
			t.Errorf("%s: legacy heading not rewritten:\n%s", rel, string(got))
		}
		if !strings.Contains(string(got), "## Open Questions") {
			t.Errorf("%s: canonical heading missing:\n%s", rel, string(got))
		}
	}
}

// --- indexEntriesChecker ---

func TestIndexEntries_Valid(t *testing.T) {
	root := setupSpecTree(t, map[string]string{
		"features/cli/README.md":      "# CLI\n\n| Dir | Desc |\n|---|---|\n| [task](task/README.md) | Task mgmt |\n",
		"features/cli/task/README.md": "# Task\n",
	})

	c := newIndexEntriesChecker()
	v, err := c.check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 0 {
		t.Errorf("expected 0 violations, got %d: %v", len(v), v)
	}
}

func TestIndexEntries_NonExistentDir(t *testing.T) {
	root := setupSpecTree(t, map[string]string{
		"features/cli/README.md": "# CLI\n\n| Dir | Desc |\n|---|---|\n| [missing](missing/README.md) | Does not exist |\n",
	})

	c := newIndexEntriesChecker()
	v, err := c.check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(v), v)
	}
	if !strings.Contains(v[0].Message, "missing") {
		t.Errorf("expected message about 'missing', got %s", v[0].Message)
	}
}

// AC: index-entries-fix-removes-phantom-row — `--fix` deletes the row
// whose link target points at a non-existent child dir. Idempotent on
// a second pass. The orphan-child direction is NOT autofixed.
func TestIndexEntries_FixDeletesPhantomRow(t *testing.T) {
	root := setupSpecTree(t, map[string]string{
		"features/cli/README.md": "# CLI\n\n" +
			"| Dir | Desc |\n" +
			"|---|---|\n" +
			"| [real](real/README.md) | Real |\n" +
			"| [ghost](ghost/README.md) | Phantom — to delete |\n" +
			"| [also-real](also-real/README.md) | Also real |\n",
		"features/cli/real/README.md":      "# Real\n",
		"features/cli/also-real/README.md": "# Also Real\n",
	})

	c := newIndexEntriesChecker().(*indexEntriesChecker)
	if err := c.fix(root); err != nil {
		t.Fatalf("fix: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(root, "features", "cli", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(got)
	if strings.Contains(s, "ghost") {
		t.Errorf("phantom row not removed:\n%s", s)
	}
	if !strings.Contains(s, "[real](real/README.md)") || !strings.Contains(s, "[also-real](also-real/README.md)") {
		t.Errorf("real rows must be preserved:\n%s", s)
	}
	if !strings.Contains(s, "|---|---|") {
		t.Errorf("table header/delimiter must be preserved:\n%s", s)
	}

	// fix-is-idempotent: second pass MUST yield no further changes.
	if err := c.fix(root); err != nil {
		t.Fatalf("fix (pass 2): %v", err)
	}
	got2, _ := os.ReadFile(filepath.Join(root, "features", "cli", "README.md"))
	if string(got2) != s {
		t.Errorf("second --fix pass mutated file (idempotency violation):\nbefore:\n%s\nafter:\n%s", s, got2)
	}

	// post-fix check pass MUST report 0 phantom-link violations.
	violations, err := c.check(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range violations {
		if strings.Contains(v.Message, "non-existent") {
			t.Errorf("phantom-link violation survived --fix: %v", v)
		}
	}
}

// AC: index-entries-fix-inserts-orphan-row (root features index) — when a
// top-level feature directory exists on disk but is not linked from
// spec/features/README.md, `--fix` appends a 4-cell row with the parsed
// Status and the standard placeholders for Kind / Description. Mirrors the
// row shape `specscore feature new` already produces, so the autofix never
// invents content the user has authority over.
func TestIndexEntries_FixInsertsOrphanRowAtRoot(t *testing.T) {
	root := setupSpecTree(t, map[string]string{
		"features/README.md": "# Features\n\n" +
			"| Feature | Status | Kind | Description |\n" +
			"|---------|--------|------|-------------|\n" +
			"| [auth](auth/README.md) | Implementing | Command | linked |\n\n" +
			"## Open Questions\n\nNone at this time.\n",
		"features/auth/README.md":    "# Feature: Auth\n\n**Status:** Implementing\n",
		"features/billing/README.md": "# Feature: Billing\n\n**Status:** Stable\n",
	})

	c := newIndexEntriesChecker().(*indexEntriesChecker)
	if err := c.fix(root); err != nil {
		t.Fatalf("fix: %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(root, "features", "README.md"))
	s := string(got)

	// The parsed Status flows through; Kind and Description use the
	// codified placeholders.
	wantRow := "| [billing](billing/README.md) | Stable | — | TODO: Add description. |"
	if !strings.Contains(s, wantRow) {
		t.Errorf("orphan row not inserted with expected shape.\nwant: %s\ngot:\n%s", wantRow, s)
	}
	// Existing rows preserved.
	if !strings.Contains(s, "| [auth](auth/README.md) | Implementing | Command | linked |") {
		t.Errorf("existing row was mutated:\n%s", s)
	}

	// Idempotency: pass 2 is a no-op.
	if err := c.fix(root); err != nil {
		t.Fatalf("fix (pass 2): %v", err)
	}
	got2, _ := os.ReadFile(filepath.Join(root, "features", "README.md"))
	if string(got2) != s {
		t.Errorf("pass 2 mutated the file (idempotency violation):\npass1:\n%s\npass2:\n%s", s, got2)
	}

	// Post-fix check pass: no orphan-child violations remain.
	violations, _ := c.check(root)
	for _, v := range violations {
		if strings.Contains(v.Message, "not listed in index") {
			t.Errorf("orphan-child violation survived --fix: %v", v)
		}
	}
}

// AC: index-entries-fix-inserts-orphan-row (nested feature index) — when a
// nested feature has child directories that are not linked from its
// ## Contents table, `--fix` appends 2-cell rows. The shape matches what
// `feature new --parent` produces.
func TestIndexEntries_FixInsertsOrphanRowNested(t *testing.T) {
	root := setupSpecTree(t, map[string]string{
		"features/cli/README.md": "# Feature: CLI\n\n" +
			"**Status:** Stable\n\n" +
			"## Contents\n\n" +
			"| Child | Description |\n" +
			"|---|---|\n" +
			"| [list](list/README.md) | linked |\n\n" +
			"## Open Questions\n\nNone at this time.\n",
		"features/cli/list/README.md": "# Feature: List\n\n**Status:** Stable\n",
		"features/cli/info/README.md": "# Feature: Info\n\n**Status:** Implementing\n",
	})

	c := newIndexEntriesChecker().(*indexEntriesChecker)
	if err := c.fix(root); err != nil {
		t.Fatalf("fix: %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(root, "features", "cli", "README.md"))
	s := string(got)
	wantRow := "| [info](info/README.md) | TODO: Add description. |"
	if !strings.Contains(s, wantRow) {
		t.Errorf("nested orphan row not inserted with expected shape.\nwant: %s\ngot:\n%s", wantRow, s)
	}

	// Idempotency.
	if err := c.fix(root); err != nil {
		t.Fatalf("fix (pass 2): %v", err)
	}
	got2, _ := os.ReadFile(filepath.Join(root, "features", "cli", "README.md"))
	if string(got2) != s {
		t.Errorf("nested pass 2 mutated the file (idempotency):\npass1:\n%s\npass2:\n%s", s, got2)
	}
}

// Defends against the row that links a real child AND a phantom on the same
// line. The fixer MUST keep the row because deleting it would lose the real
// link.
func TestIndexEntries_FixKeepsRowWithMixedLinks(t *testing.T) {
	root := setupSpecTree(t, map[string]string{
		"features/cli/README.md": "# CLI\n\n" +
			"| Dir | Linked phantom | Desc |\n" +
			"|---|---|---|\n" +
			"| [real](real/README.md) | see also [ghost](ghost/README.md) | Mixed |\n",
		"features/cli/real/README.md": "# Real\n",
	})

	c := newIndexEntriesChecker().(*indexEntriesChecker)
	if err := c.fix(root); err != nil {
		t.Fatalf("fix: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(root, "features", "cli", "README.md"))
	if !strings.Contains(string(got), "[real](real/README.md)") {
		t.Errorf("row linking a real child was deleted: %s", got)
	}
}

// REQ: fix-is-idempotent — when `--fix` derives a new Status / Promotes To
// for an Idea (because a Feature's Source Ideas references it), the
// single-pass run MUST also sync the spec/ideas/README.md row. A SECOND
// `--fix` pass MUST be a no-op. Regression test for the orchestration bug
// where ideaSyncRules rewrote the Idea file on disk but the in-memory
// parsed[] map was not refreshed, so ideaIndexRules in the same pass saw
// stale data and skipped the index rewrite.
func TestIdeaSync_SinglePassUpdatesIdeasIndex(t *testing.T) {
	root := setupSpecTree(t, map[string]string{
		// Feature references the Idea via Source Ideas. Status is Stable so
		// the derivation expects Idea.Status == "Implemented".
		"features/auth/README.md": "# Feature: Auth\n\n" +
			"**Status:** Stable\n" +
			"**Source Ideas:** auth-overhaul\n\n" +
			"## Summary\n\nPlaceholder.\n\n" +
			"## Open Questions\n\nNone at this time.\n\n" +
			"---\n*This document follows the https://specscore.md/feature-specification*\n",

		// Idea is stuck at Approved/—. Expected to be auto-derived to
		// Implemented / auth.
		"ideas/auth-overhaul.md": "# Idea: Auth Overhaul\n\n" +
			"**Status:** Approved\n" +
			"**Date:** 2026-05-18\n" +
			"**Owner:** tester\n" +
			"**Promotes To:** —\n" +
			"**Supersedes:** —\n" +
			"**Related Ideas:** —\n\n" +
			"## Problem Statement\n\nHow might we test idempotency.\n\n" +
			"## Context\n\nx\n\n" +
			"## Recommended Direction\n\nx\n\n" +
			"## Alternatives Considered\n\nx\n\n" +
			"## MVP Scope\n\nx\n\n" +
			"## Not Doing (and Why)\n\n- nothing — placeholder.\n\n" +
			"## Key Assumptions to Validate\n\n" +
			"| Tier | Assumption | How to validate |\n|---|---|---|\n" +
			"| Must-be-true | placeholder | placeholder |\n\n" +
			"## SpecScore Integration\n\n- placeholder\n\n" +
			"## Open Questions\n\nNone at this time.\n\n" +
			"---\n*This document follows the https://specscore.md/idea-specification*\n",

		// Index row is also stale (Approved/—). Must end up Implemented/auth
		// after a SINGLE --fix pass.
		"ideas/README.md": "# Ideas\n\n## Index\n\n" +
			"| Idea | Status | Date | Owner | Promotes To |\n" +
			"|------|--------|------|-------|-------------|\n" +
			"| [auth-overhaul](auth-overhaul.md) | Approved | 2026-05-18 | tester | — |\n\n" +
			"## Open Questions\n\nNone at this time.\n\n" +
			"---\n*This document follows the https://specscore.md/ideas-index-specification*\n",
	})

	// Pass 1: apply --fix. setupSpecTree treats `root` as the spec root.
	if _, err := Lint(Options{SpecRoot: root, Fix: true, Severity: "info"}); err != nil {
		t.Fatalf("lint pass 1: %v", err)
	}

	// Snapshot post-pass-1 state.
	ideaAfter1, _ := os.ReadFile(filepath.Join(root, "ideas", "auth-overhaul.md"))
	indexAfter1, _ := os.ReadFile(filepath.Join(root, "ideas", "README.md"))

	if !strings.Contains(string(ideaAfter1), "**Status:** Implemented") {
		t.Errorf("pass 1: Idea Status not derived to Implemented:\n%s", ideaAfter1)
	}
	if !strings.Contains(string(ideaAfter1), "**Promotes To:** auth") {
		t.Errorf("pass 1: Idea Promotes To not derived to auth:\n%s", ideaAfter1)
	}
	if !strings.Contains(string(indexAfter1), "| Implemented |") {
		t.Errorf("pass 1: ideas index row not synced to Implemented (single-pass idempotency violated):\n%s", indexAfter1)
	}
	if !strings.Contains(string(indexAfter1), "| auth |") {
		t.Errorf("pass 1: ideas index row not synced to auth in Promotes To column:\n%s", indexAfter1)
	}

	// Pass 2 MUST be a no-op.
	if _, err := Lint(Options{SpecRoot: root, Fix: true, Severity: "info"}); err != nil {
		t.Fatalf("lint pass 2: %v", err)
	}
	ideaAfter2, _ := os.ReadFile(filepath.Join(root, "ideas", "auth-overhaul.md"))
	indexAfter2, _ := os.ReadFile(filepath.Join(root, "ideas", "README.md"))
	if string(ideaAfter2) != string(ideaAfter1) {
		t.Errorf("pass 2 mutated the Idea file (idempotency violation):\npass1:\n%s\npass2:\n%s", ideaAfter1, ideaAfter2)
	}
	if string(indexAfter2) != string(indexAfter1) {
		t.Errorf("pass 2 mutated the ideas index (idempotency violation):\npass1:\n%s\npass2:\n%s", indexAfter1, indexAfter2)
	}
}

func TestIndexEntries_ChildNotListed(t *testing.T) {
	// A child directory with a README exists on disk, but the parent index
	// does not link to it. The checker MUST flag the orphan.
	root := setupSpecTree(t, map[string]string{
		"features/README.md":        "# Features\n\n| Feature | Status | Kind | Description |\n|---|---|---|---|\n",
		"features/orphan/README.md": "# Orphan Feature\n",
		"features/listed/README.md": "# Listed\n",
		"features/cli/README.md":    "# CLI\n\n| Dir | Desc |\n|---|---|\n| [listed](listed/README.md) | Linked child |\n",
	})
	// The cli index links 'listed' but not 'orphan'; cli has only the
	// already-listed dir, so the violation belongs to the root index.
	if err := os.WriteFile(filepath.Join(root, "features", "README.md"),
		[]byte("# Features\n\n| Feature | Status | Kind | Description |\n|---|---|---|---|\n| [cli](cli/README.md) | Implementing | Command | parent |\n"),
		0o644); err != nil {
		t.Fatal(err)
	}

	c := newIndexEntriesChecker()
	v, err := c.check(root)
	if err != nil {
		t.Fatal(err)
	}

	// Expect exactly one violation: features/README.md doesn't list 'orphan'
	// or 'listed'. We seeded both for the cli subtree but they sit at root
	// too — so the root README is missing both.
	var orphanFound, listedFound bool
	for _, viol := range v {
		if viol.Rule != "index-entries" {
			t.Errorf("unexpected rule %q in %v", viol.Rule, viol)
		}
		if !strings.Contains(viol.Message, "not listed in index") {
			continue
		}
		if strings.Contains(viol.Message, "orphan") {
			orphanFound = true
		}
		if strings.Contains(viol.Message, "listed") {
			listedFound = true
		}
	}
	if !orphanFound {
		t.Errorf("expected 'not listed in index: orphan' violation, got %v", v)
	}
	if !listedFound {
		t.Errorf("expected 'not listed in index: listed' violation, got %v", v)
	}
}

// --- linter orchestration ---

func TestLinter_RulesFilter(t *testing.T) {
	opts := Options{
		SpecRoot: t.TempDir(),
		Rules:    []string{"oq-section"},
	}
	l := newLinter(opts)
	if l.isRuleEnabled("readme-exists") {
		t.Error("readme-exists should be disabled when Rules=oq-section")
	}
	if !l.isRuleEnabled("oq-section") {
		t.Error("oq-section should be enabled")
	}
}

func TestLinter_IgnoreFilter(t *testing.T) {
	opts := Options{
		SpecRoot: t.TempDir(),
		Ignore:   []string{"plan-hierarchy"},
	}
	l := newLinter(opts)
	if l.isRuleEnabled("plan-hierarchy") {
		t.Error("plan-hierarchy should be disabled when Ignore=plan-hierarchy")
	}
	if !l.isRuleEnabled("readme-exists") {
		t.Error("readme-exists should be enabled")
	}
}

// --- planHierarchyChecker ---

func TestPlanHierarchyChecker_RoadmapWithSteps(t *testing.T) {
	root := setupSpecTree(t, map[string]string{
		"plans/roadmap-a/README.md":            "# Roadmap A\n\n## Steps\n\n- Step 1\n- Step 2\n",
		"plans/roadmap-a/child-plan/README.md": "# Child Plan\n\n## Steps\n\n- Do something\n",
	})

	c := newPlanHierarchyChecker()
	v, err := c.check(root)
	if err != nil {
		t.Fatal(err)
	}

	var stepsViolations []Violation
	for _, viol := range v {
		if strings.Contains(viol.Message, "Steps") {
			stepsViolations = append(stepsViolations, viol)
		}
	}
	if len(stepsViolations) != 1 {
		t.Fatalf("expected 1 Steps violation, got %d: %v", len(stepsViolations), v)
	}
	if stepsViolations[0].Rule != "plan-hierarchy" {
		t.Errorf("expected rule plan-hierarchy, got %s", stepsViolations[0].Rule)
	}
}

func TestPlanHierarchyChecker_ThreeLevelNesting(t *testing.T) {
	root := setupSpecTree(t, map[string]string{
		"plans/roadmap-a/README.md":                       "# Roadmap A\n\n## Child Plans\n\n- child-plan\n",
		"plans/roadmap-a/child-plan/README.md":            "# Child Plan\n\n## Child Plans\n\n- grandchild\n",
		"plans/roadmap-a/child-plan/grandchild/README.md": "# Grandchild\n\n## Steps\n\n- Do something\n",
	})

	c := newPlanHierarchyChecker()
	v, err := c.check(root)
	if err != nil {
		t.Fatal(err)
	}

	var nestingViolations []Violation
	for _, viol := range v {
		if strings.Contains(viol.Message, "nesting") || strings.Contains(viol.Message, "depth") {
			nestingViolations = append(nestingViolations, viol)
		}
	}
	if len(nestingViolations) == 0 {
		t.Fatalf("expected nesting violation, got none; all violations: %v", v)
	}
}

func TestPlanHierarchyChecker_ValidHierarchy(t *testing.T) {
	root := setupSpecTree(t, map[string]string{
		"plans/roadmap-a/README.md":            "# Roadmap A\n\n## Child Plans\n\n- child-plan\n",
		"plans/roadmap-a/child-plan/README.md": "# Child Plan\n\n## Steps\n\n- Do something\n",
		"plans/standalone/README.md":           "# Standalone Plan\n\n## Steps\n\n- Step 1\n",
	})

	c := newPlanHierarchyChecker()
	v, err := c.check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 0 {
		t.Errorf("expected 0 violations for valid hierarchy, got %d: %v", len(v), v)
	}
}

// --- planROIChecker ---

func TestPlanROIChecker_InvalidEffort(t *testing.T) {
	root := setupSpecTree(t, map[string]string{
		"plans/my-plan/README.md": "# My Plan\n\n**Effort:** huge\n**Impact:** high\n\n## Steps\n\n- Step 1\n",
	})

	c := newPlanROIChecker()
	v, err := c.check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 1 {
		t.Fatalf("expected 1 violation, got %d: %v", len(v), v)
	}
	if !strings.Contains(v[0].Message, "Effort") {
		t.Errorf("expected violation to mention Effort, got: %s", v[0].Message)
	}
}

func TestPlanROIChecker_ValidMetadata(t *testing.T) {
	root := setupSpecTree(t, map[string]string{
		"plans/my-plan/README.md": "# My Plan\n\n**Effort:** M\n**Impact:** high\n\n## Steps\n\n- Step 1\n",
	})

	c := newPlanROIChecker()
	v, err := c.check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 0 {
		t.Errorf("expected 0 violations for valid metadata, got %d: %v", len(v), v)
	}
}

func TestPlanROIChecker_NoMetadata(t *testing.T) {
	root := setupSpecTree(t, map[string]string{
		"plans/my-plan/README.md": "# My Plan\n\n## Steps\n\n- Step 1\n",
	})

	c := newPlanROIChecker()
	v, err := c.check(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(v) != 0 {
		t.Errorf("expected 0 violations when metadata absent, got %d: %v", len(v), v)
	}
}

// --- Lint integration ---

func TestLint_InvalidSpecRoot(t *testing.T) {
	_, err := Lint(Options{SpecRoot: "/nonexistent/path"})
	if err == nil {
		t.Error("expected error for nonexistent spec root")
	}
}

func TestLint_EmptyDir(t *testing.T) {
	root := t.TempDir()
	violations, err := Lint(Options{SpecRoot: root, Severity: "info"})
	if err != nil {
		t.Fatal(err)
	}
	// An empty dir with no README.md should produce a readme-exists violation.
	found := false
	for _, v := range violations {
		if v.Rule == "readme-exists" {
			found = true
		}
	}
	if !found {
		t.Error("expected readme-exists violation for empty dir")
	}
}

// --- custom checker registration ---

func TestRegisterChecker(t *testing.T) {
	defer ResetCustomCheckers()

	RegisterChecker(&testChecker{
		n: "custom-rule",
		s: "warning",
		violations: []Violation{{
			File: "test.md", Line: 1, Severity: "warning",
			Rule: "custom-rule", Message: "custom violation",
		}},
	})

	dir := t.TempDir()
	mkdir(t, filepath.Join(dir, "spec", "features"))
	writeFile(t, filepath.Join(dir, "spec", "features", "README.md"),
		"# Features\n\n## Open Questions\n\nNone.\n")

	violations, err := Lint(Options{SpecRoot: dir, Rules: []string{"custom-rule"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(violations) != 1 || violations[0].Rule != "custom-rule" {
		t.Errorf("expected 1 custom violation, got %d", len(violations))
	}
}

type testChecker struct {
	n          string
	s          string
	violations []Violation
}

func (c *testChecker) Name() string                      { return c.n }
func (c *testChecker) Severity() string                  { return c.s }
func (c *testChecker) Check(string) ([]Violation, error) { return c.violations, nil }

// --- helpers ---

func setupSpecTree(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for relPath, content := range files {
		fullPath := filepath.Join(root, relPath)
		mkdir(t, filepath.Dir(fullPath))
		writeFile(t, fullPath, content)
	}
	return root
}

func mkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
