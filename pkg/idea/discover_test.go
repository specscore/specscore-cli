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

func TestDiscover_NestedProposalWithoutIdeasTree(t *testing.T) {
	root := t.TempDir()
	specRoot := filepath.Join(root, "spec")
	proposalsDir := filepath.Join(specRoot, "features", "cli", "idea", "proposals")
	if err := os.MkdirAll(proposalsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	proposalPath := filepath.Join(proposalsDir, "nested-change.md")
	if err := os.WriteFile(proposalPath, []byte("# Proposal: Nested Change\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Hidden and underscore-prefixed Feature trees are conventions, not
	// discoverable Feature IDs.
	for _, hidden := range []string{".hidden", "_fixtures"} {
		path := filepath.Join(specRoot, "features", hidden, "proposals")
		if err := os.MkdirAll(path, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(path, "ignored.md"), []byte("ignored"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	got, err := Discover(specRoot)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected one nested proposal, got %+v", got)
	}
	if got[0].Path != proposalPath || got[0].FeatureDir != "cli/idea" || !got[0].IsProposal {
		t.Errorf("unexpected nested proposal: %+v", got[0])
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

// promotedSeed is the canonical content of a promoted sidekick-seed parked
// under spec/ideas/archived/ after `specscore idea promote`'s cross-repo path.
const promotedSeed = `---
type: sidekick-seed
captured_by: specstudio:plan
status: promoted
promoted_to: foo
---
# Foo seed

Original seed prose.
`

// TestDiscover_ArchivedPromotedSeedSkipped verifies that a promoted
// sidekick-seed in archived/ is NOT discovered as an Idea, while the active
// Idea sharing the same slug still is. This locks in the fix for the lint
// bug where the archived seed was mis-parsed as an Idea and collided with
// the active Idea by slug.
func TestDiscover_ArchivedPromotedSeedSkipped(t *testing.T) {
	root := t.TempDir()
	specRoot := filepath.Join(root, "spec")
	ideasDir := filepath.Join(specRoot, "ideas")
	archivedDir := filepath.Join(ideasDir, "archived")
	if err := os.MkdirAll(archivedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Active Idea with slug foo.
	if err := os.WriteFile(filepath.Join(ideasDir, "foo.md"), []byte("# Idea: Foo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Promoted seed (same slug) parked in archived/.
	if err := os.WriteFile(filepath.Join(archivedDir, "foo.md"), []byte(promotedSeed), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Discover(specRoot)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("expected 1 discovered (active Idea only), got %d: %+v", len(got), got)
	}
	if got[0].Slug != "foo" || got[0].Archived {
		t.Errorf("got[0] = %+v, want active foo Idea", got[0])
	}
}

// TestDiscover_ArchivedIdeaStillDiscovered is a regression guard: a genuine
// archived Idea (frontmatter type is NOT sidekick-seed, or no frontmatter at
// all) must still be discovered as an archived Idea.
func TestDiscover_ArchivedIdeaStillDiscovered(t *testing.T) {
	root := t.TempDir()
	specRoot := filepath.Join(root, "spec")
	archivedDir := filepath.Join(specRoot, "ideas", "archived")
	if err := os.MkdirAll(archivedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Plain archived Idea (no frontmatter).
	if err := os.WriteFile(filepath.Join(archivedDir, "alpha.md"), []byte("# Idea: Alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Archived Idea WITH frontmatter whose type is not sidekick-seed.
	withFront := "---\ntype: idea\nslug: beta\n---\n# Idea: Beta\n"
	if err := os.WriteFile(filepath.Join(archivedDir, "beta.md"), []byte(withFront), 0o644); err != nil {
		t.Fatal(err)
	}

	got, err := Discover(specRoot)
	if err != nil {
		t.Fatalf("Discover: %v", err)
	}
	if len(got) != 2 {
		t.Fatalf("expected 2 archived Ideas, got %d: %+v", len(got), got)
	}
	for _, d := range got {
		if !d.Archived {
			t.Errorf("expected archived, got %+v", d)
		}
	}
}

// TestIsSidekickSeedFile_Edges covers the helper's non-seed paths: an
// unreadable path, a file without leading frontmatter, and a frontmatter
// block that closes before declaring a seed type.
func TestIsSidekickSeedFile_Edges(t *testing.T) {
	dir := t.TempDir()

	if isSidekickSeedFile(filepath.Join(dir, "missing.md")) {
		t.Error("missing file should not be a seed")
	}

	noFront := filepath.Join(dir, "nofront.md")
	if err := os.WriteFile(noFront, []byte("# Idea: Plain\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if isSidekickSeedFile(noFront) {
		t.Error("file without frontmatter should not be a seed")
	}

	closedFirst := filepath.Join(dir, "closed.md")
	if err := os.WriteFile(closedFirst, []byte("---\nslug: x\n---\ntype: sidekick-seed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if isSidekickSeedFile(closedFirst) {
		t.Error("type after closing fence is body, not frontmatter")
	}

	// Frontmatter that never closes and never declares the seed type: the
	// scanner reaches EOF and the helper returns false.
	unterminated := filepath.Join(dir, "unterminated.md")
	if err := os.WriteFile(unterminated, []byte("---\nslug: x\ntype: idea\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if isSidekickSeedFile(unterminated) {
		t.Error("unterminated non-seed frontmatter should not be a seed")
	}

	seed := filepath.Join(dir, "seed.md")
	if err := os.WriteFile(seed, []byte(promotedSeed), 0o644); err != nil {
		t.Fatal(err)
	}
	if !isSidekickSeedFile(seed) {
		t.Error("promoted seed should be detected")
	}
}

func TestIsCrossRepoRef(t *testing.T) {
	cross := []string{
		"https://github.com/sneat-co/backstage/blob/main/spec/ideas/sourcer.md",
		"http://example.com/spec/ideas/x.md",
		"[sourcer](https://github.com/sneat-co/backstage/blob/main/spec/ideas/sourcer.md)",
		"[Paywall Bot](https://github.com/sneat-co/backstage/blob/main/spec/ideas/paywallbot.md)",
	}
	for _, c := range cross {
		if !IsCrossRepoRef(c) {
			t.Errorf("IsCrossRepoRef(%q) = false, want true", c)
		}
	}
	local := []string{
		"consilium-command-group",
		"cli-self-update",
		"[local idea](../../ideas/offline-mode.md)", // relative link, not cross-repo
		"—",
		"",
	}
	for _, l := range local {
		if IsCrossRepoRef(l) {
			t.Errorf("IsCrossRepoRef(%q) = true, want false", l)
		}
	}
}
