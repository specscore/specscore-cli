package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// AC: plan-completed-rewrite / decision-legacy-rewrite — the core map lookup.
func TestRewriteLegacyBodyStatus(t *testing.T) {
	in := "# Plan: x\n\n**Status:** Completed\n**Source:** none\n"
	out, changed := rewriteLegacyBodyStatus([]byte(in), legacyPlanStatusMap)
	if !changed {
		t.Fatal("expected changed=true for legacy token")
	}
	if !strings.Contains(string(out), "**Status:** Implemented") {
		t.Fatalf("status not rewritten: %q", out)
	}

	// Non-legacy / prose value is untouched (closed-set-only).
	prose := "# Plan: x\n\n**Status:** Not started.\n"
	out2, changed2 := rewriteLegacyBodyStatus([]byte(prose), legacyPlanStatusMap)
	if changed2 || string(out2) != prose {
		t.Fatalf("prose status must be untouched, changed=%v", changed2)
	}
}

// AC: decision-frontmatter-mirror — frontmatter status: is synced in one pass.
func TestRewriteLegacyBodyStatus_FrontmatterMirror(t *testing.T) {
	in := "---\nstatus: Accepted\n---\n# Decision: x\n\n**Status:** Accepted\n"
	out, changed := rewriteLegacyBodyStatus([]byte(in), legacyDecisionStatusMap)
	if !changed {
		t.Fatal("expected change")
	}
	s := string(out)
	if !strings.Contains(s, "status: Approved") || !strings.Contains(s, "**Status:** Approved") {
		t.Fatalf("frontmatter and body must both read Approved: %q", s)
	}
}

// ensureArchivedFlag: an existing **Archived:** line is forced to true.
func TestEnsureArchivedFlag_ExistingLine(t *testing.T) {
	in := "# Idea: x\n\n**Status:** Stale\n**Archived:** false\n\n## Summary\n"
	out := string(ensureArchivedFlag([]byte(in)))
	if !strings.Contains(out, "**Archived:** true") || strings.Contains(out, "**Archived:** false") {
		t.Fatalf("existing Archived line must become true: %q", out)
	}
}

// ensureArchivedFlag: inserted immediately after the Status line when absent.
func TestEnsureArchivedFlag_Insert(t *testing.T) {
	in := "# Idea: x\n\n**Status:** Stale\n\n## Summary\n"
	out := string(ensureArchivedFlag([]byte(in)))
	want := "**Status:** Stale\n**Archived:** true\n"
	if !strings.Contains(out, want) {
		t.Fatalf("Archived flag must follow Status line: %q", out)
	}
}

// ensureArchivedFlag: no body Status line (scan breaks at the first ## heading)
// leaves content unchanged.
func TestEnsureArchivedFlag_NoStatus(t *testing.T) {
	in := "# Idea: x\n\n## Summary\n\n**Status:** Stale\n"
	out := string(ensureArchivedFlag([]byte(in)))
	if out != in {
		t.Fatalf("content must be unchanged when no body Status precedes a heading: %q", out)
	}
}

// fixLegacyStatusesInTree: a missing directory is a no-op (returns nil).
func TestFixLegacyStatusesInTree_MissingDir(t *testing.T) {
	if err := fixLegacyStatusesInTree(filepath.Join(t.TempDir(), "nope"), legacyPlanStatusMap, false); err != nil {
		t.Fatalf("missing dir must be a no-op, got %v", err)
	}
}

// fixLegacyStatusesInTree: README.md and non-markdown files are skipped; only
// legacy-token .md files are rewritten.
func TestFixLegacyStatusesInTree_SkipsNonTargets(t *testing.T) {
	dir := t.TempDir()
	readme := filepath.Join(dir, "README.md")
	other := filepath.Join(dir, "notes.txt")
	plan := filepath.Join(dir, "p.md")
	mustWrite(t, readme, "# R\n\n**Status:** Completed\n")
	mustWrite(t, other, "**Status:** Completed\n")
	mustWrite(t, plan, "# Plan: p\n\n**Status:** Completed\n")
	if err := fixLegacyStatusesInTree(dir, legacyPlanStatusMap, false); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, readme); !strings.Contains(got, "Completed") {
		t.Fatal("README.md must be skipped")
	}
	if got := readFile(t, other); !strings.Contains(got, "Completed") {
		t.Fatal(".txt must be skipped")
	}
	if got := readFile(t, plan); !strings.Contains(got, "Implemented") {
		t.Fatalf("plan .md must be rewritten: %q", got)
	}
}

// fixLegacyStatusesInTree: a WalkDir error (unreadable subdirectory) propagates.
func TestFixLegacyStatusesInTree_WalkError(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(sub, 0o000); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(sub, 0o755) }()
	if err := fixLegacyStatusesInTree(dir, legacyPlanStatusMap, false); err == nil {
		t.Error("expected error for unreadable subdirectory")
	}
}

// fixLegacyStatusesInTree: a ReadFile error on a listed .md propagates.
func TestFixLegacyStatusesInTree_ReadError(t *testing.T) {
	dir := t.TempDir()
	plan := filepath.Join(dir, "p.md")
	mustWrite(t, plan, "# Plan: p\n\n**Status:** Completed\n")
	if err := os.Chmod(plan, 0o000); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(plan, 0o644) }()
	if err := fixLegacyStatusesInTree(dir, legacyPlanStatusMap, false); err == nil {
		t.Error("expected error for unreadable .md file")
	}
}

// decisionRulesChecker.fix is a no-op when fixLegacy is unset.
func TestDecisionRulesChecker_FixDisabled(t *testing.T) {
	if err := newDecisionRulesChecker().fix(t.TempDir()); err != nil {
		t.Fatalf("fix with fixLegacy=false must be a no-op, got %v", err)
	}
}

// planRulesChecker.fix surfaces a P-006 legacy-fix error (unreadable plans dir).
func TestPlanRulesChecker_FixP006LegacyError(t *testing.T) {
	root := t.TempDir()
	plansDir := filepath.Join(root, "plans")
	if err := os.MkdirAll(filepath.Join(plansDir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(plansDir, "sub"), 0o000); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(filepath.Join(plansDir, "sub"), 0o755) }()
	c := newPlanRulesChecker()
	c.fixP006Legacy = true
	if err := c.fix(root); err == nil {
		t.Error("expected P-006 legacy fix error for unreadable plans subdir")
	}
}

// CheckIdeas surfaces a legacy-fix error when the ideas tree is unreadable.
func TestCheckIdeas_LegacyFixError(t *testing.T) {
	root := t.TempDir()
	ideasDir := filepath.Join(root, "spec", "ideas")
	if err := os.MkdirAll(filepath.Join(ideasDir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Join(ideasDir, "sub"), 0o000); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(filepath.Join(ideasDir, "sub"), 0o755) }()
	if _, err := CheckIdeas(filepath.Join(root, "spec"), true); err == nil {
		t.Error("expected legacy-fix error for unreadable ideas subdir")
	}
}
