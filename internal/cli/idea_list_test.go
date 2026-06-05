package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestIdeaList_EmptyProject — no ideas exist, output is empty.
func TestIdeaList_EmptyProject(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)

	stdout, _, err := runIdea(t, "list")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("expected empty output, got: %q", stdout)
	}
}

// TestIdeaList_MultipleIdeas — lists slugs sorted alphabetically.
func TestIdeaList_MultipleIdeas(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)

	// Create two ideas (idea new gives them Draft status).
	for _, slug := range []string{"zebra", "alpha"} {
		if _, _, err := runIdea(t, "new", slug, "--owner", "test"); err != nil {
			t.Fatalf("idea new %s: %v", slug, err)
		}
	}

	stdout, _, err := runIdea(t, "list")
	if err != nil {
		t.Fatalf("list failed: %v", err)
	}
	lines := nonEmptyLines(stdout)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d: %q", len(lines), stdout)
	}
	if lines[0] != "alpha" || lines[1] != "zebra" {
		t.Errorf("expected [alpha, zebra], got %v", lines)
	}
}

// TestIdeaList_StatusFilter — --status filters by lifecycle status.
func TestIdeaList_StatusFilter(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)

	// Create features README for lint to pass.
	writeFeaturesReadme(t, root)

	// Create two ideas, then approve one.
	if _, _, err := runIdea(t, "new", "stays-draft", "--owner", "test"); err != nil {
		t.Fatalf("idea new: %v", err)
	}
	if _, _, err := runIdea(t, "new", "gets-approved", "--owner", "test"); err != nil {
		t.Fatalf("idea new: %v", err)
	}
	if _, _, err := runIdea(t, "change-status", "gets-approved", "--to=approved"); err != nil {
		t.Fatalf("change-status: %v", err)
	}

	// Filter for Draft only.
	stdout, _, err := runIdea(t, "list", "--status=Draft")
	if err != nil {
		t.Fatalf("list --status=Draft: %v", err)
	}
	lines := nonEmptyLines(stdout)
	if len(lines) != 1 || lines[0] != "stays-draft" {
		t.Errorf("expected [stays-draft], got %v", lines)
	}

	// Filter for Approved only.
	stdout, _, err = runIdea(t, "list", "--status=approved") // case-insensitive
	if err != nil {
		t.Fatalf("list --status=approved: %v", err)
	}
	lines = nonEmptyLines(stdout)
	if len(lines) != 1 || lines[0] != "gets-approved" {
		t.Errorf("expected [gets-approved], got %v", lines)
	}
}

// TestIdeaList_AllIncludesArchived — --all shows archived ideas.
func TestIdeaList_AllIncludesArchived(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)

	// Create an active idea first (before placing the archived file,
	// so idea new's post-scaffold lint doesn't trip on it).
	if _, _, err := runIdea(t, "new", "active-one", "--owner", "test"); err != nil {
		t.Fatalf("idea new: %v", err)
	}

	// Write an archived idea directly (after idea new, so lint won't see it during scaffold).
	archivedContent := "# Idea: Old Thing\n\n**Status:** Archived\n**Date:** 2025-01-01\n**Owner:** test\n**Promotes To:** —\n**Supersedes:** —\n**Related Ideas:** —\n**Archive Reason:** no longer relevant\n\n## Problem Statement\n\nOld.\n\n## Context\n\nOld.\n\n## Recommended Direction\n\nOld.\n\n## Alternatives Considered\n\n- none\n\n## MVP Scope\n\nN/A.\n\n## Not Doing (and Why)\n\n- nothing — done\n\n## Key Assumptions to Validate\n\n- Must-be-true: this is a test\n\n## SpecScore Integration\n\nN/A.\n\n## Open Questions\n\nNone.\n"
	if err := os.WriteFile(filepath.Join(root, "spec", "ideas", "archived", "old-thing.md"), []byte(archivedContent), 0o644); err != nil {
		t.Fatalf("write archived idea: %v", err)
	}

	// Without --all, only active idea shown.
	stdout, _, err := runIdea(t, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	lines := nonEmptyLines(stdout)
	if len(lines) != 1 || lines[0] != "active-one" {
		t.Errorf("without --all: expected [active-one], got %v", lines)
	}

	// With --all, both shown.
	stdout, _, err = runIdea(t, "list", "--all")
	if err != nil {
		t.Fatalf("list --all: %v", err)
	}
	lines = nonEmptyLines(stdout)
	if len(lines) != 2 {
		t.Fatalf("with --all: expected 2 lines, got %d: %v", len(lines), lines)
	}
	// Sorted: active-one < old-thing
	if lines[0] != "active-one" || lines[1] != "old-thing" {
		t.Errorf("with --all: expected [active-one, old-thing], got %v", lines)
	}
}

// TestIdeaList_IncludeArchivedFlag — --include-archived shows archived ideas.
func TestIdeaList_IncludeArchivedFlag(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)

	if _, _, err := runIdea(t, "new", "active-one", "--owner", "test"); err != nil {
		t.Fatalf("idea new: %v", err)
	}

	archivedContent := "# Idea: Old Thing\n\n**Status:** Archived\n**Date:** 2025-01-01\n**Owner:** test\n**Promotes To:** —\n**Supersedes:** —\n**Related Ideas:** —\n**Archive Reason:** no longer relevant\n\n## Problem Statement\n\nOld.\n\n## Context\n\nOld.\n\n## Recommended Direction\n\nOld.\n\n## Alternatives Considered\n\n- none\n\n## MVP Scope\n\nN/A.\n\n## Not Doing (and Why)\n\n- nothing — done\n\n## Key Assumptions to Validate\n\n- Must-be-true: this is a test\n\n## SpecScore Integration\n\nN/A.\n\n## Open Questions\n\nNone.\n"
	if err := os.WriteFile(filepath.Join(root, "spec", "ideas", "archived", "old-thing.md"), []byte(archivedContent), 0o644); err != nil {
		t.Fatalf("write archived idea: %v", err)
	}

	// Without --include-archived, only active idea shown.
	stdout, _, err := runIdea(t, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	lines := nonEmptyLines(stdout)
	if len(lines) != 1 || lines[0] != "active-one" {
		t.Errorf("without --include-archived: expected [active-one], got %v", lines)
	}

	// With --include-archived, both shown.
	stdout, _, err = runIdea(t, "list", "--include-archived")
	if err != nil {
		t.Fatalf("list --include-archived: %v", err)
	}
	lines = nonEmptyLines(stdout)
	if len(lines) != 2 {
		t.Fatalf("with --include-archived: expected 2 lines, got %d: %v", len(lines), lines)
	}
	if lines[0] != "active-one" || lines[1] != "old-thing" {
		t.Errorf("with --include-archived: expected [active-one, old-thing], got %v", lines)
	}
}

// TestIdeaList_FormatJSON — --format=json emits structured output.
func TestIdeaList_FormatJSON(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)

	if _, _, err := runIdea(t, "new", "json-test", "--owner", "test"); err != nil {
		t.Fatalf("idea new: %v", err)
	}

	stdout, _, err := runIdea(t, "list", "--format=json")
	if err != nil {
		t.Fatalf("list --format=json: %v", err)
	}

	var entries []ideaListEntry
	if err := json.Unmarshal([]byte(stdout), &entries); err != nil {
		t.Fatalf("unmarshal JSON: %v\nraw: %s", err, stdout)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	e := entries[0]
	if e.Slug != "json-test" {
		t.Errorf("slug = %q, want json-test", e.Slug)
	}
	if e.Status != "Draft" {
		t.Errorf("status = %q, want Draft", e.Status)
	}
	if e.Archived {
		t.Errorf("archived = true, want false")
	}
	if !strings.Contains(e.Path, "json-test.md") {
		t.Errorf("path = %q, expected to contain json-test.md", e.Path)
	}
}

// TestIdeaList_FormatYAML — --format=yaml emits structured output.
func TestIdeaList_FormatYAML(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)

	if _, _, err := runIdea(t, "new", "yaml-test", "--owner", "test"); err != nil {
		t.Fatalf("idea new: %v", err)
	}

	stdout, _, err := runIdea(t, "list", "--format=yaml")
	if err != nil {
		t.Fatalf("list --format=yaml: %v", err)
	}

	// Verify YAML contains expected fields.
	if !strings.Contains(stdout, "slug: yaml-test") {
		t.Errorf("YAML missing slug: %s", stdout)
	}
	if !strings.Contains(stdout, "status: Draft") {
		t.Errorf("YAML missing status: %s", stdout)
	}
	if !strings.Contains(stdout, "archived: false") {
		t.Errorf("YAML missing archived: %s", stdout)
	}
}

// TestIdeaList_InvalidFormat — bad --format value is rejected.
func TestIdeaList_InvalidFormat(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)

	_, _, err := runIdea(t, "list", "--format=csv")
	if err == nil {
		t.Fatal("expected error for invalid format")
	}
	if !strings.Contains(err.Error(), "invalid --format") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestIdeaList_NoProjectRoot — error when no spec root found.
func TestIdeaList_NoProjectRoot(t *testing.T) {
	// Use a temp dir with no specscore.yaml or .git — resolveSpecRoot fails.
	tmp := t.TempDir()
	withCwd(t, tmp)

	_, _, err := runIdea(t, "list")
	if err == nil {
		t.Fatal("expected error when no project root found")
	}
}

// TestIdeaList_ProjectFlag — --project flag works.
func TestIdeaList_ProjectFlag(t *testing.T) {
	root := setupSpecRoot(t)
	// Don't chdir — use --project instead.
	tmp := t.TempDir()
	withCwd(t, tmp)

	if _, _, err := runIdea(t, "new", "proj-flag-test", "--owner", "test", "--project", root); err != nil {
		t.Fatalf("idea new: %v", err)
	}

	stdout, _, err := runIdea(t, "list", "--project", root)
	if err != nil {
		t.Fatalf("list --project: %v", err)
	}
	lines := nonEmptyLines(stdout)
	if len(lines) != 1 || lines[0] != "proj-flag-test" {
		t.Errorf("expected [proj-flag-test], got %v", lines)
	}
}

// TestIdeaList_StatusFilterNoMatch — --status with no matching ideas returns empty.
func TestIdeaList_StatusFilterNoMatch(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)

	if _, _, err := runIdea(t, "new", "draft-only", "--owner", "test"); err != nil {
		t.Fatalf("idea new: %v", err)
	}

	stdout, _, err := runIdea(t, "list", "--status=Approved")
	if err != nil {
		t.Fatalf("list --status=Approved: %v", err)
	}
	if strings.TrimSpace(stdout) != "" {
		t.Errorf("expected empty output for non-matching filter, got: %q", stdout)
	}
}

// TestIdeaList_TextFormatNoStatus — text format without status filter uses Discover only (no parse).
func TestIdeaList_TextFormatNoStatus(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)

	for _, slug := range []string{"charlie", "bravo", "alpha"} {
		if _, _, err := runIdea(t, "new", slug, "--owner", "test"); err != nil {
			t.Fatalf("idea new %s: %v", slug, err)
		}
	}

	stdout, _, err := runIdea(t, "list", "--format=text")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	lines := nonEmptyLines(stdout)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d: %v", len(lines), lines)
	}
	// Must be alphabetically sorted.
	if lines[0] != "alpha" || lines[1] != "bravo" || lines[2] != "charlie" {
		t.Errorf("not sorted: %v", lines)
	}
}

// TestIdeaList_DiscoverError — covers the idea.Discover error branch.
func TestIdeaList_DiscoverError(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping as root: chmod won't restrict access")
	}
	root := setupSpecRoot(t)
	withCwd(t, root)

	// Make the ideas directory unreadable so os.ReadDir fails inside Discover.
	ideasDir := filepath.Join(root, "spec", "ideas")
	if err := os.Chmod(ideasDir, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(ideasDir, 0o755) })

	_, _, err := runIdea(t, "list")
	if err == nil {
		t.Fatal("expected error when ideas dir is unreadable")
	}
	if !strings.Contains(err.Error(), "discovering ideas") {
		t.Errorf("unexpected error: %v", err)
	}
}

// TestIdeaList_YAMLEncodeError — covers the yaml.Encode error branch using a failing writer.
func TestIdeaList_YAMLEncodeError(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)

	// Create an idea so there's data to encode.
	if _, _, err := runIdea(t, "new", "yaml-err", "--owner", "test"); err != nil {
		t.Fatalf("idea new: %v", err)
	}

	// Invoke the list command with a writer that errors.
	cmd := ideaCommand()
	cmd.SetOut(&errWriter{})
	cmd.SetErr(&errWriter{})
	cmd.SetArgs([]string{"list", "--format=yaml"})
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error when writer fails during yaml encode")
	}
}

// errWriter is an io.Writer that always returns an error.
type errWriter struct{}

func (ew *errWriter) Write(p []byte) (int, error) {
	return 0, fmt.Errorf("injected write failure")
}

// --- helpers ---

func nonEmptyLines(s string) []string {
	var out []string
	for _, line := range strings.Split(s, "\n") {
		if strings.TrimSpace(line) != "" {
			out = append(out, strings.TrimSpace(line))
		}
	}
	return out
}

func writeFeaturesReadme(t *testing.T, root string) {
	t.Helper()
	content := "---\nformat: https://specscore.md/features-index-specification\n---\n\n# Features\n\n## Index\n\n| Feature | Status |\n|---------|--------|\n\n_No features yet._\n\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(filepath.Join(root, "spec", "features", "README.md"), []byte(content), 0o644); err != nil {
		t.Fatalf("write features README: %v", err)
	}
}
