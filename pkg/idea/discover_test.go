package idea

import (
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"testing"
)

// TestFeatureSourceIdeas_NestedFeatures locks in the contract that
// **Source Ideas:** headers on nested feature READMEs are discovered.
// Regression test for the prior bug where only `spec/features/<slug>/`
// (one level deep) was scanned, making nested-feature promotion silently
// no-op in the idea-sync-lint-strict rule.
func TestFeatureSourceIdeas_NestedFeatures(t *testing.T) {
	root := t.TempDir()
	specRoot := filepath.Join(root, "spec")

	files := map[string]string{
		// Top-level feature with a Source Ideas reference.
		"features/auth/README.md": "# Feature: Auth\n\n" +
			"**Status:** Approved\n" +
			"**Source Ideas:** auth-overhaul\n\n",

		// Nested two levels deep — the case the original walker missed.
		"features/cli/lifecycle-transitions/README.md": "# Feature: Lifecycle Transitions\n\n" +
			"**Status:** Approved\n" +
			"**Source Ideas:** lifecycle-verbs-for-idea-and-feature\n\n",

		// Nested three levels deep — also must be picked up.
		"features/cli/spec/lint/README.md": "# Feature: Spec Lint\n\n" +
			"**Status:** Stable\n" +
			"**Source Ideas:** index-entries-autofix\n\n",

		// Feature without Source Ideas — must be omitted from the map.
		"features/cli/README.md": "# Feature: CLI\n\n**Status:** Stable\n\n",

		// Dir convention prefixes that must be skipped entirely.
		"features/_args/README.md":   "# Args\n\n**Source Ideas:** ignored-conventional-dir\n",
		"features/.hidden/README.md": "# Hidden\n\n**Source Ideas:** ignored-hidden\n",
	}
	for path, content := range files {
		full := filepath.Join(specRoot, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
	}

	got, err := FeatureSourceIdeas(specRoot)
	if err != nil {
		t.Fatalf("FeatureSourceIdeas: %v", err)
	}

	want := map[string][]string{
		"auth":                      {"auth-overhaul"},
		"cli/lifecycle-transitions": {"lifecycle-verbs-for-idea-and-feature"},
		"cli/spec/lint":             {"index-entries-autofix"},
	}

	if len(got) != len(want) {
		gotKeys := make([]string, 0, len(got))
		for k := range got {
			gotKeys = append(gotKeys, k)
		}
		sort.Strings(gotKeys)
		t.Fatalf("map size = %d, want %d. got keys = %v", len(got), len(want), gotKeys)
	}
	for slug, wantIdeas := range want {
		gotIdeas, ok := got[slug]
		if !ok {
			t.Errorf("missing slug %q in result", slug)
			continue
		}
		if !reflect.DeepEqual(gotIdeas, wantIdeas) {
			t.Errorf("slug %q: got %v, want %v", slug, gotIdeas, wantIdeas)
		}
	}
	for _, badSlug := range []string{"_args", ".hidden", "cli"} {
		if _, present := got[badSlug]; present {
			t.Errorf("slug %q must NOT be in result", badSlug)
		}
	}
}

// TestFeatureSourceIdeas_FencedHeadingBeforeField verifies that a `## ` line
// inside a fenced code block in the preamble does NOT stop the Source Ideas
// scan early. Without fence tracking, the scanner breaks at the fenced `## `
// and silently drops the feature -> idea reference.
func TestFeatureSourceIdeas_FencedHeadingBeforeField(t *testing.T) {
	root := t.TempDir()
	specRoot := filepath.Join(root, "spec")

	content := "# Feature: Fenced\n\n" +
		"**Status:** Approved\n\n" +
		"```yaml\n" +
		"## not a real heading\n" +
		"```\n\n" +
		"**Source Ideas:** my-idea\n\n" +
		"## Summary\n\nStuff.\n"

	full := filepath.Join(specRoot, "features", "fenced", "README.md")
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	got, err := FeatureSourceIdeas(specRoot)
	if err != nil {
		t.Fatalf("FeatureSourceIdeas: %v", err)
	}
	if !reflect.DeepEqual(got["fenced"], []string{"my-idea"}) {
		t.Errorf("source ideas dropped by fenced `## `: got %v, want [my-idea]", got["fenced"])
	}
}

// TestDiscover_Proposals exercises the proposal discovery path, which
// scans spec/features/*/proposals/*.md and returns them with IsProposal=true.
func TestDiscover_Proposals(t *testing.T) {
	root := t.TempDir()
	specRoot := filepath.Join(root, "spec")

	// Required: ideas/ directory must exist for Discover to proceed.
	ideasDir := filepath.Join(specRoot, "ideas")
	if err := os.MkdirAll(ideasDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Create a feature with a proposals/ subdirectory containing proposals.
	proposalsDir := filepath.Join(specRoot, "features", "auth", "proposals")
	if err := os.MkdirAll(proposalsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Valid proposal
	if err := os.WriteFile(filepath.Join(proposalsDir, "add-mfa.md"), []byte("# Proposal: Add MFA\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Another valid proposal
	if err := os.WriteFile(filepath.Join(proposalsDir, "add-sso.md"), []byte("# Proposal: Add SSO\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// README.md should be skipped
	if err := os.WriteFile(filepath.Join(proposalsDir, "README.md"), []byte("# Proposals\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Non-.md file should be skipped
	if err := os.WriteFile(filepath.Join(proposalsDir, "notes.txt"), []byte("notes"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Subdirectory inside proposals/ should be skipped
	subDir := filepath.Join(proposalsDir, "drafts")
	if err := os.MkdirAll(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(subDir, "ignored.md"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Discover(specRoot)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}

	// Should find exactly 2 proposals.
	if len(got) != 2 {
		t.Fatalf("expected 2 discovered, got %d: %+v", len(got), got)
	}

	slugs := map[string]Discovered{}
	for _, d := range got {
		slugs[d.Slug] = d
	}

	for _, slug := range []string{"add-mfa", "add-sso"} {
		d, ok := slugs[slug]
		if !ok {
			t.Errorf("missing slug %q", slug)
			continue
		}
		if !d.IsProposal {
			t.Errorf("%q: IsProposal should be true", slug)
		}
		if d.FeatureDir != "auth" {
			t.Errorf("%q: FeatureDir = %q, want %q", slug, d.FeatureDir, "auth")
		}
		if d.Archived {
			t.Errorf("%q: should not be archived", slug)
		}
	}
}

// TestDiscover_NoProposalsDir verifies that a feature without a proposals/
// subdirectory is silently skipped.
func TestDiscover_NoProposalsDir(t *testing.T) {
	root := t.TempDir()
	specRoot := filepath.Join(root, "spec")
	if err := os.MkdirAll(filepath.Join(specRoot, "ideas"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Feature directory without proposals/
	if err := os.MkdirAll(filepath.Join(specRoot, "features", "auth"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(specRoot, "features", "auth", "README.md"), []byte("# Feature: Auth\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Discover(specRoot)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 discovered, got %d: %+v", len(got), got)
	}
}

// TestDiscover_FeatureHasNoProposalFiles verifies that an empty proposals/
// directory produces no results.
func TestDiscover_FeatureHasNoProposalFiles(t *testing.T) {
	root := t.TempDir()
	specRoot := filepath.Join(root, "spec")
	if err := os.MkdirAll(filepath.Join(specRoot, "ideas"), 0o755); err != nil {
		t.Fatal(err)
	}
	proposalsDir := filepath.Join(specRoot, "features", "auth", "proposals")
	if err := os.MkdirAll(proposalsDir, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := Discover(specRoot)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 discovered, got %d: %+v", len(got), got)
	}
}

// TestDiscover_MixedActiveArchivedAndProposals verifies that active ideas,
// archived ideas, and proposals are all returned and sorted correctly
// (active+proposals before archived, alphabetically within each group).
func TestDiscover_MixedActiveArchivedAndProposals(t *testing.T) {
	root := t.TempDir()
	specRoot := filepath.Join(root, "spec")
	ideasDir := filepath.Join(specRoot, "ideas")
	archivedDir := filepath.Join(ideasDir, "archived")
	proposalsDir := filepath.Join(specRoot, "features", "auth", "proposals")
	for _, d := range []string{ideasDir, archivedDir, proposalsDir} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Active idea
	if err := os.WriteFile(filepath.Join(ideasDir, "beta.md"), []byte("# Idea: Beta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Archived idea
	if err := os.WriteFile(filepath.Join(archivedDir, "alpha.md"), []byte("# Idea: Alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Proposal
	if err := os.WriteFile(filepath.Join(proposalsDir, "add-mfa.md"), []byte("# Proposal: Add MFA\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Discover(specRoot)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d: %+v", len(got), got)
	}

	// Non-archived should come first (active + proposals), sorted by slug.
	// add-mfa < beta (alphabetical among non-archived), then alpha (archived).
	if got[0].Slug != "add-mfa" || got[0].IsProposal != true {
		t.Errorf("got[0] = %+v, want add-mfa proposal", got[0])
	}
	if got[1].Slug != "beta" || got[1].Archived != false {
		t.Errorf("got[1] = %+v, want beta active", got[1])
	}
	if got[2].Slug != "alpha" || got[2].Archived != true {
		t.Errorf("got[2] = %+v, want alpha archived", got[2])
	}
}

// TestDiscover_NonDirFeatureEntrySkipped verifies that non-directory entries
// inside spec/features/ are skipped when scanning for proposals.
func TestDiscover_NonDirFeatureEntrySkipped(t *testing.T) {
	root := t.TempDir()
	specRoot := filepath.Join(root, "spec")
	if err := os.MkdirAll(filepath.Join(specRoot, "ideas"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(specRoot, "features"), 0o755); err != nil {
		t.Fatal(err)
	}
	// A file (not directory) at features/ level
	if err := os.WriteFile(filepath.Join(specRoot, "features", "README.md"), []byte("# Features\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Discover(specRoot)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected 0 discovered, got %d", len(got))
	}
}
