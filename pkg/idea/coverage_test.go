package idea

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

// =============================================================================
// parse.go — ArchiveNote at 0%
// =============================================================================

func TestArchiveNote(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "archived-idea.md")
	content := `# Idea: Archived Idea

**Status:** Stale
**Archived:** true
**Date:** 2026-05-01
**Owner:** tester
**Promotes To:** —
**Supersedes:** —
**Related Ideas:** —
**Archive Note:** Superseded by new-idea

## Problem Statement
How Might We test archive reason.

## Context
x

## Recommended Direction
x

## Alternatives Considered
x

## MVP Scope
x

## Not Doing (and Why)
- nothing — placeholder.

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|---|---|---|
| Must-be-true | placeholder | placeholder |

## SpecScore Integration
- placeholder

## Open Questions
None at this time.
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	idea, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}

	got := idea.ArchiveNote()
	if got != "Superseded by new-idea" {
		t.Errorf("ArchiveNote() = %q, want %q", got, "Superseded by new-idea")
	}
}

func TestArchiveNote_Empty(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "no-reason.md")
	content := `# Idea: No Reason

**Status:** Draft
**Date:** 2026-05-01
**Owner:** tester
**Promotes To:** —
**Supersedes:** —
**Related Ideas:** —

## Problem Statement
How Might We test.

## Context
x

## Recommended Direction
x

## Alternatives Considered
x

## MVP Scope
x

## Not Doing (and Why)
- nothing — placeholder.

## Key Assumptions to Validate

| Tier | Assumption | How to validate |
|---|---|---|
| Must-be-true | placeholder | placeholder |

## SpecScore Integration
- placeholder

## Open Questions
None at this time.
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	idea, err := Parse(path)
	if err != nil {
		t.Fatal(err)
	}

	got := idea.ArchiveNote()
	if got != "" {
		t.Errorf("ArchiveNote() = %q, want empty", got)
	}
}

// =============================================================================
// parse.go — SortedStatuses at 0%
// =============================================================================

func TestSortedStatuses(t *testing.T) {
	statuses := SortedStatuses()
	if len(statuses) == 0 {
		t.Fatal("SortedStatuses returned empty slice")
	}
	// Verify sorted
	for i := 1; i < len(statuses); i++ {
		if statuses[i] < statuses[i-1] {
			t.Errorf("not sorted: %q comes after %q", statuses[i], statuses[i-1])
		}
	}
	// Verify known statuses are present
	found := map[string]bool{}
	for _, s := range statuses {
		found[s] = true
	}
	for _, want := range []string{"Draft", "Approved", "Stale"} {
		if !found[want] {
			t.Errorf("missing status %q in SortedStatuses()", want)
		}
	}
}

// =============================================================================
// parse.go — pathBase at 75% (backslash separator case)
// =============================================================================

func TestPathBase_Backslash(t *testing.T) {
	got := pathBase(`C:\Users\test\ideas\my-idea.md`)
	if got != "my-idea.md" {
		t.Errorf("pathBase with backslash = %q, want %q", got, "my-idea.md")
	}
}

func TestPathBase_ForwardSlash(t *testing.T) {
	got := pathBase("/home/user/ideas/my-idea.md")
	if got != "my-idea.md" {
		t.Errorf("pathBase with forward slash = %q, want %q", got, "my-idea.md")
	}
}

func TestPathBase_NoSeparator(t *testing.T) {
	got := pathBase("my-idea.md")
	if got != "my-idea.md" {
		t.Errorf("pathBase with no separator = %q, want %q", got, "my-idea.md")
	}
}

// =============================================================================
// parse.go — ParseTable at 87.5% (edge cases)
// =============================================================================

func TestParseTable_NoTable(t *testing.T) {
	body := "Just some text\nwith no table.\n"
	tab := ParseTable(body)
	if tab != nil {
		t.Errorf("expected nil for no-table body, got %+v", tab)
	}
}

func TestParseTable_HeaderOnly_NoSeparator(t *testing.T) {
	body := "| Col A | Col B |\n| not a separator |\n"
	tab := ParseTable(body)
	if tab != nil {
		t.Errorf("expected nil when separator row is invalid, got %+v", tab)
	}
}

func TestParseTable_PipeLineAtEOF(t *testing.T) {
	// Pipe line as the very last line with nothing following
	body := "some text\n| Col A | Col B |"
	tab := ParseTable(body)
	if tab != nil {
		t.Errorf("expected nil when pipe line is at EOF with no separator, got %+v", tab)
	}
}

func TestParseTable_InvalidSeparator(t *testing.T) {
	body := "| Col A | Col B |\n| not a separator |\n| data | data |\n"
	tab := ParseTable(body)
	if tab != nil {
		t.Errorf("expected nil for invalid separator, got %+v", tab)
	}
}

func TestParseTable_ValidTableWithRows(t *testing.T) {
	body := "| Name | Status |\n|---|---|\n| foo | Draft |\n| bar | Approved |\n"
	tab := ParseTable(body)
	if tab == nil {
		t.Fatal("expected non-nil table")
	}
	if len(tab.Headers) != 2 {
		t.Errorf("headers = %v, want 2 columns", tab.Headers)
	}
	if len(tab.Rows) != 2 {
		t.Errorf("rows = %d, want 2", len(tab.Rows))
	}
}

func TestParseTable_EmptyRows(t *testing.T) {
	body := "| Col |\n|---|\n\nNext paragraph."
	tab := ParseTable(body)
	if tab == nil {
		t.Fatal("expected non-nil table")
	}
	if len(tab.Rows) != 0 {
		t.Errorf("expected 0 data rows, got %d", len(tab.Rows))
	}
}

// =============================================================================
// discover.go ��� Discover at 75% (error branches)
// =============================================================================

func TestDiscover_NoIdeasDir(t *testing.T) {
	root := t.TempDir()
	got, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil for missing ideas dir, got %v", got)
	}
}

func TestDiscover_WithActiveAndArchived(t *testing.T) {
	root := t.TempDir()
	ideasDir := filepath.Join(root, "ideas")
	archivedDir := filepath.Join(ideasDir, "archived")
	if err := os.MkdirAll(archivedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Active ideas
	if err := os.WriteFile(filepath.Join(ideasDir, "alpha.md"), []byte("# Idea: Alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ideasDir, "beta.md"), []byte("# Idea: Beta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Archived idea
	if err := os.WriteFile(filepath.Join(archivedDir, "old.md"), []byte("# Idea: Old\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// A directory inside archived/ (should be skipped)
	if err := os.MkdirAll(filepath.Join(archivedDir, "sub-dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	// README should be skipped
	if err := os.WriteFile(filepath.Join(ideasDir, "README.md"), []byte("# Index\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Non-.md file should be skipped
	if err := os.WriteFile(filepath.Join(ideasDir, "notes.txt"), []byte("notes"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	// Should be sorted: active first (alpha, beta), then archived (old)
	if len(got) != 3 {
		t.Fatalf("expected 3 discovered ideas, got %d: %v", len(got), got)
	}
	if got[0].Slug != "alpha" || got[0].Archived {
		t.Errorf("got[0] = %+v, want alpha/active", got[0])
	}
	if got[1].Slug != "beta" || got[1].Archived {
		t.Errorf("got[1] = %+v, want beta/active", got[1])
	}
	if got[2].Slug != "old" || !got[2].Archived {
		t.Errorf("got[2] = %+v, want old/archived", got[2])
	}
}

func TestDiscover_UnreadableArchivedDir(t *testing.T) {
	root := t.TempDir()
	ideasDir := filepath.Join(root, "ideas")
	archivedDir := filepath.Join(ideasDir, "archived")
	if err := os.MkdirAll(archivedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ideasDir, "good.md"), []byte("# Idea\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Make archived dir unreadable
	if err := os.Chmod(archivedDir, 0o000); err != nil {
		t.Skip("cannot change permissions (maybe running as root)")
	}
	defer func() { _ = os.Chmod(archivedDir, 0o755) }()

	_, err := Discover(root)
	if err == nil {
		t.Error("expected error for unreadable archived dir")
	}
}

// =============================================================================
// discover.go — FindIdeaDirectories at 80%
// =============================================================================

func TestFindIdeaDirectories_SkipsSeedsAndArchived(t *testing.T) {
	root := t.TempDir()
	ideasDir := filepath.Join(root, "ideas")
	if err := os.MkdirAll(filepath.Join(ideasDir, "archived"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(ideasDir, "seeds"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A rogue directory (violation)
	if err := os.MkdirAll(filepath.Join(ideasDir, "some-dir"), 0o755); err != nil {
		t.Fatal(err)
	}

	dirs, err := FindIdeaDirectories(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(dirs) != 1 {
		t.Fatalf("expected 1 directory, got %d: %v", len(dirs), dirs)
	}
	if !strings.Contains(dirs[0], "some-dir") {
		t.Errorf("expected some-dir, got %s", dirs[0])
	}
}

func TestFindIdeaDirectories_NoIdeasDir(t *testing.T) {
	root := t.TempDir()
	dirs, err := FindIdeaDirectories(root)
	if err != nil {
		t.Fatal(err)
	}
	if dirs != nil {
		t.Errorf("expected nil for missing ideas dir, got %v", dirs)
	}
}

// =============================================================================
// transitions.go — ChangeStatus at 62.3% (more scenarios)
// =============================================================================

func TestChangeStatus_MissingSpecRoot(t *testing.T) {
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot:     "",
		Slug:         "foo",
		To:           lifecycle.IdeaApproved,
		PostMutation: noopLint,
	})
	assertExitCode(t, err, exitcode.Unexpected)
}

func TestChangeStatus_MissingSlug(t *testing.T) {
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot:     "/tmp",
		Slug:         "",
		To:           lifecycle.IdeaApproved,
		PostMutation: noopLint,
	})
	assertExitCode(t, err, exitcode.InvalidArgs)
}

func TestChangeStatus_MissingTo(t *testing.T) {
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot:     "/tmp",
		Slug:         "foo",
		To:           "",
		PostMutation: noopLint,
	})
	assertExitCode(t, err, exitcode.InvalidArgs)
}

func TestChangeStatus_MissingPostMutation(t *testing.T) {
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: "/tmp",
		Slug:     "foo",
		To:       lifecycle.IdeaApproved,
	})
	assertExitCode(t, err, exitcode.Unexpected)
}

func TestChangeStatus_LintFailureRollsBackNonArchive(t *testing.T) {
	root := stageIdeaTree(t, "baz", "Draft")

	simulatedErr := exitcode.UnexpectedErrorf("lint failed")
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot:     root,
		Slug:         "baz",
		To:           lifecycle.IdeaApproved,
		PostMutation: failingLint(simulatedErr),
	})
	assertExitCode(t, err, exitcode.Unexpected)

	// File should still be at Draft (rolled back)
	body := readIdea(t, root, "baz")
	if !strings.Contains(body, "**Status:** Draft") {
		t.Errorf("expected status rolled back to Draft, got:\n%s", body)
	}
}

func TestChangeStatus_NoStatusLine(t *testing.T) {
	// Create an idea file without a **Status:** line to trigger ErrStatusLineNotFound
	root := t.TempDir()
	ideasDir := filepath.Join(root, "spec", "ideas")
	if err := os.MkdirAll(ideasDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Write an idea file without a Status line
	content := "# Idea: No Status\n\n**Date:** 2026-05-01\n**Owner:** tester\n"
	if err := os.WriteFile(filepath.Join(ideasDir, "no-status.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot:     root,
		Slug:         "no-status",
		To:           lifecycle.IdeaApproved,
		PostMutation: noopLint,
	})
	if err == nil {
		t.Fatal("expected error for missing status line")
	}
	if !strings.Contains(err.Error(), "Status") {
		t.Errorf("error should mention Status, got: %v", err)
	}
}

// =============================================================================
// scaffold.go — titleCaseFromSlug at 83.3%
// =============================================================================

func TestTitleCaseFromSlug(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"hello-world", "Hello World"},
		{"single", "Single"},
		{"a-b-c", "A B C"},
		{"", ""},
	}
	for _, tt := range tests {
		got := titleCaseFromSlug(tt.input)
		if got != tt.want {
			t.Errorf("titleCaseFromSlug(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

// =============================================================================
// scaffold.go — Scaffold at 90.5% (populated options)
// =============================================================================

func TestScaffold_WithAllOptions(t *testing.T) {
	body, err := Scaffold(ScaffoldOptions{
		Slug:                 "full-options",
		Title:                "Full Options",
		Owner:                "alice",
		Date:                 "2026-01-15",
		Status:               "Approved",
		HMW:                  "How Might We test all options.",
		Context:              "Context paragraph.",
		RecommendedDirection: "Direction paragraph.",
		Alternatives:         []string{"Alt A", "Alt B"},
		MVP:                  "MVP scope.",
		NotDoing:             []string{"Scope X", "Scope Y"},
		Assumptions:          [][3]string{{"Must-be-true", "Users exist", "Check analytics"}},
		NewFeatures:          "feature-a",
		Existing:             "feature-b",
		Dependencies:         "feature-c",
		OpenQuestions:        []string{"Question 1?", "Question 2?"},
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	checks := []string{
		"# Idea: Full Options",
		"**Status:** Approved",
		"**Date:** 2026-01-15",
		"**Owner:** alice",
		"How Might We test all options.",
		"Context paragraph.",
		"Direction paragraph.",
		"- Alt A",
		"- Alt B",
		"MVP scope.",
		"- Scope X",
		"- Scope Y",
		"| Must-be-true | Users exist | Check analytics |",
		"- **New Features this would create:** feature-a",
		"- **Existing Features affected:** feature-b",
		"- **Dependencies:** feature-c",
		"- Question 1?",
		"- Question 2?",
	}
	for _, c := range checks {
		if !strings.Contains(s, c) {
			t.Errorf("scaffold missing %q", c)
		}
	}
}

func TestScaffold_InvalidSlug(t *testing.T) {
	_, err := Scaffold(ScaffoldOptions{Slug: "INVALID"})
	if err == nil {
		t.Error("expected error for invalid slug")
	}
}

func TestScaffold_EmptyItemsInLists(t *testing.T) {
	// Exercise the "continue" branches for empty-string items in lists
	body, err := Scaffold(ScaffoldOptions{
		Slug:          "empty-items",
		Alternatives:  []string{"Alt A", "", "Alt C"}, // empty item filtered
		NotDoing:      []string{"", "Scope X", ""},    // empty items filtered
		OpenQuestions: []string{"Q1?", "", "Q3?"},     // empty item filtered
	})
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	if !strings.Contains(s, "- Alt A") || !strings.Contains(s, "- Alt C") {
		t.Error("missing alternatives")
	}
	if !strings.Contains(s, "- Scope X") {
		t.Error("missing not-doing item")
	}
	if !strings.Contains(s, "- Q1?") || !strings.Contains(s, "- Q3?") {
		t.Error("missing open questions")
	}
}

// =============================================================================
// discover.go — FeatureSourceIdeas at 90%
// =============================================================================

func TestFeatureSourceIdeas_NoFeaturesDir(t *testing.T) {
	root := t.TempDir()
	got, err := FeatureSourceIdeas(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map for missing features dir, got %v", got)
	}
}

func TestFeatureSourceIdeas_WalkError(t *testing.T) {
	root := t.TempDir()
	featuresDir := filepath.Join(root, "features")
	if err := os.MkdirAll(filepath.Join(featuresDir, "unreadable"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Make the feature subdir unreadable to trigger walkErr
	if err := os.Chmod(filepath.Join(featuresDir, "unreadable"), 0o000); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(filepath.Join(featuresDir, "unreadable"), 0o755) }()

	_, err := FeatureSourceIdeas(root)
	if err == nil {
		t.Error("expected walk error for unreadable feature dir")
	}
}

func TestFeatureSourceIdeas_FeatureWithoutReadme(t *testing.T) {
	root := t.TempDir()
	featuresDir := filepath.Join(root, "features")
	// Create a feature dir without README.md
	if err := os.MkdirAll(filepath.Join(featuresDir, "no-readme"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := FeatureSourceIdeas(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("expected empty map for feature without readme, got %v", got)
	}
}

func TestFeatureSourceIdeas_FeatureWithDashSourceIdeas(t *testing.T) {
	root := t.TempDir()
	featuresDir := filepath.Join(root, "features")
	featureDir := filepath.Join(featuresDir, "my-feat")
	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Source Ideas with — (em-dash placeholder)
	content := "# Feature: My Feat\n\n**Status:** Draft\n**Source Ideas:** —\n\n## Summary\n\nTest.\n"
	if err := os.WriteFile(filepath.Join(featureDir, "README.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := FeatureSourceIdeas(root)
	if err != nil {
		t.Fatal(err)
	}
	// "—" normalizes to nil, so no entry should be present
	if len(got) != 0 {
		t.Errorf("expected empty map for — source ideas, got %v", got)
	}
}

func TestDiscover_DirectoriesSkipped(t *testing.T) {
	root := t.TempDir()
	ideasDir := filepath.Join(root, "ideas")
	if err := os.MkdirAll(ideasDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// A directory entry inside ideas (not a file) — should be skipped
	if err := os.MkdirAll(filepath.Join(ideasDir, "some-dir"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A valid idea file
	if err := os.WriteFile(filepath.Join(ideasDir, "valid.md"), []byte("# Idea: Valid\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Discover(root)
	if err != nil {
		t.Fatal(err)
	}
	// Only the .md file should appear, not the directory
	if len(got) != 1 {
		t.Fatalf("expected 1, got %d: %v", len(got), got)
	}
	if got[0].Slug != "valid" {
		t.Errorf("expected slug 'valid', got %q", got[0].Slug)
	}
}

func TestFindIdeaDirectories_UnreadableDir(t *testing.T) {
	root := t.TempDir()
	ideasDir := filepath.Join(root, "ideas")
	if err := os.MkdirAll(ideasDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Make ideas dir unreadable
	if err := os.Chmod(ideasDir, 0o000); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(ideasDir, 0o755) }()

	_, err := FindIdeaDirectories(root)
	if err == nil {
		t.Error("expected error for unreadable ideas dir")
	}
}

func TestDiscover_UnreadableIdeasDir(t *testing.T) {
	root := t.TempDir()
	ideasDir := filepath.Join(root, "ideas")
	if err := os.MkdirAll(ideasDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Make it unreadable but stat-able
	if err := os.Chmod(ideasDir, 0o111); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(ideasDir, 0o755) }()

	_, err := Discover(root)
	if err == nil {
		t.Error("expected error for unreadable ideas dir")
	}
}

// =============================================================================
// discover.go — parseSourceIdeas at 85.7%
// =============================================================================

func TestParseSourceIdeas_NoSourceIdeasField(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "README.md")
	if err := os.WriteFile(path, []byte("# Feature: Test\n\n**Status:** Draft\n\n## Summary\n\nHello.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := parseSourceIdeas(path)
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Errorf("expected nil when no Source Ideas field, got %v", got)
	}
}

func TestParseSourceIdeas_FileNotFound(t *testing.T) {
	_, err := parseSourceIdeas("/nonexistent/path/README.md")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

func TestParseSourceIdeas_WithSourceIdeas(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "README.md")
	content := "# Feature: Test\n\n**Status:** Draft\n**Source Ideas:** alpha, beta\n\n## Summary\n\nHello.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := parseSourceIdeas(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != "alpha" || got[1] != "beta" {
		t.Errorf("expected [alpha, beta], got %v", got)
	}
}

// =============================================================================
// parse.go — splitCSVSlugs at 90.9%
// =============================================================================

// =============================================================================
// parse.go — Parse error path (file not found)
// =============================================================================

func TestParse_FileNotFound(t *testing.T) {
	_, err := Parse("/nonexistent/path/idea.md")
	if err == nil {
		t.Error("expected error for nonexistent file")
	}
}

// =============================================================================
// parse.go — splitCSVSlugs edge cases
// =============================================================================

// =============================================================================
// transitions.go — stat error (not IsNotExist) on active path (line 131)
// =============================================================================

func TestChangeStatus_StatErrorOnActivePath(t *testing.T) {
	root := stageIdeaTree(t, "stat-err", "Draft")
	// Make the idea file unreadable so os.Stat returns a permission error.
	ideaPath := filepath.Join(root, "spec", "ideas", "stat-err.md")
	if err := os.Chmod(ideaPath, 0o000); err != nil {
		t.Skip("cannot change permissions")
	}
	t.Cleanup(func() { _ = os.Chmod(ideaPath, 0o644) })

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot:     root,
		Slug:         "stat-err",
		To:           lifecycle.IdeaApproved,
		PostMutation: noopLint,
	})
	// On some OS this may be NotFound or Unexpected depending on stat behavior.
	if err == nil {
		t.Fatal("expected error for unreadable idea file")
	}
}

// =============================================================================
// transitions.go — lifecycle.Rewrite error (line 158)
// =============================================================================

func TestChangeStatus_RewriteError(t *testing.T) {
	root := t.TempDir()
	ideasDir := filepath.Join(root, "spec", "ideas")
	if err := os.MkdirAll(ideasDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Write an idea with a Status line. Make the DIRECTORY read-only so
	// the atomic-write pattern (create temp file + rename) in lifecycle.Rewrite fails.
	content := "# Idea: Rewrite Error\n\n**Status:** Draft\n**Date:** 2026-01-01\n**Owner:** tester\n**Promotes To:** —\n**Supersedes:** —\n**Related Ideas:** —\n\n## Problem Statement\nHow Might We test.\n\n## Context\nx\n\n## Recommended Direction\nx\n\n## Alternatives Considered\nx\n\n## MVP Scope\nx\n\n## Not Doing (and Why)\n- x — y\n\n## Key Assumptions to Validate\n\n| Tier | Assumption | How to validate |\n|---|---|---|\n| Must-be-true | x | x |\n\n## SpecScore Integration\n- x\n\n## Open Questions\nNone at this time.\n"
	ideaPath := filepath.Join(ideasDir, "rewrite-err.md")
	if err := os.WriteFile(ideaPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	// Make the directory read-only after writing the file so the atomic
	// write (which creates a temp file in the same dir) fails.
	if err := os.Chmod(ideasDir, 0o555); err != nil {
		t.Skip("cannot change permissions")
	}
	t.Cleanup(func() { _ = os.Chmod(ideasDir, 0o755) })

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot:     root,
		Slug:         "rewrite-err",
		To:           lifecycle.IdeaApproved,
		PostMutation: noopLint,
	})
	if err == nil {
		t.Fatal("expected error for read-only directory (atomic write fails)")
	}
	assertExitCode(t, err, exitcode.Unexpected)
}

func TestSplitCSVSlugs_EdgeCases(t *testing.T) {
	tests := []struct {
		input string
		want  []string
	}{
		{"", nil},
		{"—", nil},
		{"-", nil},
		{"  ", nil},
		{"a, b, c", []string{"a", "b", "c"}},
		{"a,,b", []string{"a", "b"}},
	}
	for _, tt := range tests {
		got := splitCSVSlugs(tt.input)
		if tt.want == nil {
			if got != nil {
				t.Errorf("splitCSVSlugs(%q) = %v, want nil", tt.input, got)
			}
		} else {
			if len(got) != len(tt.want) {
				t.Errorf("splitCSVSlugs(%q) = %v, want %v", tt.input, got, tt.want)
			}
		}
	}
}

// =============================================================================
// discover.go — Discover proposals ReadDir error (line 88-89)
// =============================================================================

// TestDiscover_UnreadableProposalsDir covers the branch where os.ReadDir on a
// proposals/ subdirectory fails. The directory passes os.Stat (exists, IsDir),
// but ReadDir is blocked by missing read permission.
func TestDiscover_UnreadableProposalsDir(t *testing.T) {
	root := t.TempDir()
	specRoot := filepath.Join(root, "spec")
	if err := os.MkdirAll(filepath.Join(specRoot, "ideas"), 0o755); err != nil {
		t.Fatal(err)
	}
	proposalsDir := filepath.Join(specRoot, "features", "auth", "proposals")
	if err := os.MkdirAll(proposalsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(proposalsDir, "idea.md"), []byte("# Proposal\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Remove read permission so os.ReadDir fails, but os.Stat still succeeds.
	if err := os.Chmod(proposalsDir, 0o100); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(proposalsDir, 0o755) }()

	got, err := Discover(specRoot)
	// The code does `continue` on ReadDir error, so no error is returned
	// and the proposal is simply skipped.
	if err != nil {
		t.Fatalf("Discover: unexpected error: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 discovered (proposal skipped), got %d: %+v", len(got), got)
	}
}

// =============================================================================
// discover.go — FeatureSourceIdeas fpRel error (lines 186-188)
// =============================================================================

// TestFeatureSourceIdeas_FpRelError covers the branch where fpRel (filepath.Rel)
// returns an error. This is injected via the package-level fpRel seam.
func TestFeatureSourceIdeas_FpRelError(t *testing.T) {
	root := t.TempDir()
	featuresDir := filepath.Join(root, "features")
	featureDir := filepath.Join(featuresDir, "my-feat")
	if err := os.MkdirAll(featureDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "# Feature: My Feat\n\n**Status:** Draft\n**Source Ideas:** alpha\n\n## Summary\n\nTest.\n"
	if err := os.WriteFile(filepath.Join(featureDir, "README.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	old := fpRel
	fpRel = func(basepath, targpath string) (string, error) {
		return "", os.ErrInvalid
	}
	t.Cleanup(func() { fpRel = old })

	got, err := FeatureSourceIdeas(root)
	if err != nil {
		t.Fatalf("FeatureSourceIdeas: unexpected error: %v", err)
	}
	// The slug is skipped (return nil in the walk func), so map is empty.
	if len(got) != 0 {
		t.Errorf("expected empty map when fpRel fails, got %v", got)
	}
}

// =============================================================================
// transitions.go — os.Stat on activePath returns non-ENOENT error (line 135)
// =============================================================================

// TestChangeStatus_StatActivePathPermissionDenied covers the branch where
// os.Stat on the active idea path returns a non-NotExist error (e.g. permission
// denied). Achieved by making the parent directory non-executable so that the
// kernel cannot resolve the path.
func TestChangeStatus_StatActivePathPermissionDenied(t *testing.T) {
	root := stageIdeaTree(t, "perm-denied", "Draft")
	ideasDir := filepath.Join(root, "spec", "ideas")
	// Remove execute permission from the parent so os.Stat on the file returns
	// permission denied (not ENOENT).
	if err := os.Chmod(ideasDir, 0o600); err != nil {
		t.Skip("cannot change permissions")
	}
	t.Cleanup(func() { _ = os.Chmod(ideasDir, 0o755) })

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot:     root,
		Slug:         "perm-denied",
		To:           lifecycle.IdeaApproved,
		PostMutation: noopLint,
	})
	if err == nil {
		t.Fatal("expected error for permission-denied stat on active path")
	}
	assertExitCode(t, err, exitcode.Unexpected)
	if !strings.Contains(err.Error(), "stat") {
		t.Errorf("error should mention 'stat', got: %v", err)
	}
}
