package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// stagePromoteRepo creates a SpecScore-managed repo at <parent>/<name>
// with a spec/ideas/seeds tree and a project.repo=<repoSlug> config.
// Returns the repo root.
func stagePromoteRepo(t *testing.T, parent, name, repoSlug string) string {
	t.Helper()
	root := filepath.Join(parent, name)
	if err := os.MkdirAll(filepath.Join(root, "spec", "ideas", "seeds"), 0o755); err != nil {
		t.Fatalf("mkdir spec tree: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "spec", "ideas", "archived"), 0o755); err != nil {
		t.Fatalf("mkdir archived: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(root, "spec", "features"), 0o755); err != nil {
		t.Fatalf("mkdir features: %v", err)
	}
	specReadme := "# Spec\n\n## Contents\n\n_No features yet._\n\n## Open Questions\n\nNone at this time.\n"
	_ = os.WriteFile(filepath.Join(root, "spec", "README.md"), []byte(specReadme), 0o644)
	featReadme := "# Features\n\n## Contents\n\n_No features yet._\n\n## Open Questions\n\nNone at this time.\n"
	_ = os.WriteFile(filepath.Join(root, "spec", "features", "README.md"), []byte(featReadme), 0o644)
	yaml := "# SpecScore Repo Config Schema: https://specscore.md/repo-config\n" +
		"project:\n" +
		"  title: " + name + "\n" +
		"  org: " + repoSlug + "\n" +
		"  repo: " + repoSlug + "\n"
	if err := os.WriteFile(filepath.Join(root, "specscore.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatalf("write specscore.yaml: %v", err)
	}
	// Index READMEs so the tree is lint-clean apart from the new artifact.
	idx := "# Ideas\n\n## Index\n\n| Idea | Status | Date | Owner | Promotes To |\n|------|--------|------|-------|-------------|\n\n_No active ideas yet._\n\n## Open Questions\n\nNone at this time.\n"
	_ = os.WriteFile(filepath.Join(root, "spec", "ideas", "README.md"), []byte(idx), 0o644)
	arch := "# Archived\n\n_No archived ideas yet._\n\n## Open Questions\n\nNone at this time.\n"
	_ = os.WriteFile(filepath.Join(root, "spec", "ideas", "archived", "README.md"), []byte(arch), 0o644)
	return root
}

// promoteSeedBody returns a sidekick-seed file body with frontmatter and
// a one-line H1 title plus prose. When verdict is true a ## Consilium
// Verdict section is appended.
func promoteSeedBody(slug, title string, verdict bool) string {
	body := "---\n" +
		"type: sidekick-seed\n" +
		"slug: " + slug + "\n" +
		"captured_at: 2026-06-01T12:00:00Z\n" +
		"captured_by: specscore:test\n" +
		"status: queued\n" +
		"---\n" +
		"# " + title + "\n\n" +
		"This is the seed prose describing the idea in a paragraph.\n\n" +
		"A second paragraph with more detail.\n"
	if verdict {
		body += "\n## Consilium Verdict\n\n" +
			"**Verdict:** worth-pursuing\n\n" +
			"The panel agreed this is a strong direction worth specifying.\n"
	}
	return body
}

func writePromoteSeed(t *testing.T, repoRoot, slug, title string, verdict bool) {
	t.Helper()
	p := filepath.Join(repoRoot, "spec", "ideas", "seeds", slug+".md")
	if err := os.WriteFile(p, []byte(promoteSeedBody(slug, title, verdict)), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
}

func runIdeaPromoteCLI(t *testing.T, sourceDir string, args ...string) (string, string, error) {
	t.Helper()
	withCwd(t, sourceDir)
	full := append([]string{"promote"}, args...)
	return runIdea(t, full...)
}

// AC: seed-not-found
func TestIdeaPromoteCLI_SeedNotFound(t *testing.T) {
	parent := t.TempDir()
	source := stagePromoteRepo(t, parent, "src", "src")

	_, _, err := runIdeaPromoteCLI(t, source, "ghost")
	if got := exitCodeFromErr(t, err); got != exitcode.NotFound {
		t.Errorf("exit code: got %d want %d (NotFound)", got, exitcode.NotFound)
	}
	if !strings.Contains(err.Error(), filepath.Join("spec", "ideas", "seeds", "ghost.md")) {
		t.Errorf("error should name the missing seed path; got: %v", err)
	}
	// No file created.
	if _, statErr := os.Stat(filepath.Join(source, "spec", "ideas", "ghost.md")); !os.IsNotExist(statErr) {
		t.Errorf("no Idea file should be created on not-found")
	}
}

// AC: collision-without-force
func TestIdeaPromoteCLI_CollisionWithoutForce(t *testing.T) {
	parent := t.TempDir()
	source := stagePromoteRepo(t, parent, "src", "src")
	writePromoteSeed(t, source, "foo", "Foo Idea", false)
	// Pre-existing destination Idea.
	ideaPath := filepath.Join(source, "spec", "ideas", "foo.md")
	if err := os.WriteFile(ideaPath, []byte("# Idea: Foo\n\nexisting\n"), 0o644); err != nil {
		t.Fatalf("write idea: %v", err)
	}
	seedPath := filepath.Join(source, "spec", "ideas", "seeds", "foo.md")
	seedBefore, _ := os.ReadFile(seedPath)
	ideaBefore, _ := os.ReadFile(ideaPath)

	_, _, err := runIdeaPromoteCLI(t, source, "foo")
	if got := exitCodeFromErr(t, err); got != exitcode.Conflict {
		t.Errorf("exit code: got %d want %d (Conflict)", got, exitcode.Conflict)
	}
	seedAfter, _ := os.ReadFile(seedPath)
	ideaAfter, _ := os.ReadFile(ideaPath)
	if string(seedBefore) != string(seedAfter) {
		t.Errorf("seed must be unchanged on collision")
	}
	if string(ideaBefore) != string(ideaAfter) {
		t.Errorf("existing Idea must be unchanged on collision")
	}
}

// --verdict enum validation (exit 2).
func TestIdeaPromoteCLI_InvalidVerdictRejected(t *testing.T) {
	parent := t.TempDir()
	source := stagePromoteRepo(t, parent, "src", "src")
	writePromoteSeed(t, source, "foo", "Foo Idea", false)

	_, _, err := runIdeaPromoteCLI(t, source, "foo", "--verdict=bogus")
	if got := exitCodeFromErr(t, err); got != exitcode.InvalidArgs {
		t.Errorf("exit code: got %d want %d (InvalidArgs)", got, exitcode.InvalidArgs)
	}
}

// AC: same-repo-promote-happy-path
func TestIdeaPromoteCLI_SameRepoHappyPath(t *testing.T) {
	parent := t.TempDir()
	source := stagePromoteRepo(t, parent, "src", "src")
	writePromoteSeed(t, source, "bar", "Bar Feature Idea", false)
	initGitRepoForTest(t, source)

	stdout, _, err := runIdeaPromoteCLI(t, source, "bar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Seed gone, Idea present.
	seedPath := filepath.Join(source, "spec", "ideas", "seeds", "bar.md")
	if _, statErr := os.Stat(seedPath); !os.IsNotExist(statErr) {
		t.Errorf("seed should be moved away; still at %s", seedPath)
	}
	ideaPath := filepath.Join(source, "spec", "ideas", "bar.md")
	body, readErr := os.ReadFile(ideaPath)
	if readErr != nil {
		t.Fatalf("idea not created: %v", readErr)
	}
	content := string(body)
	if !strings.Contains(content, "# Idea: Bar Feature Idea") {
		t.Errorf("expected `# Idea: <title>` heading; got:\n%s", content)
	}
	if !strings.HasPrefix(content, "# Idea:") {
		t.Errorf("frontmatter should be dropped; got prefix:\n%s", content[:min(80, len(content))])
	}
	for _, field := range []string{"**Status:** Draft", "**Date:**", "**Owner:**",
		"**Promotes To:** —", "**Supersedes:** —", "**Related Ideas:** —"} {
		if !strings.Contains(content, field) {
			t.Errorf("missing Idea body-metadata %q; got:\n%s", field, content)
		}
	}
	// Seed prose folded into ## Context.
	ctxIdx := strings.Index(content, "## Context")
	if ctxIdx < 0 {
		t.Fatalf("no ## Context section; got:\n%s", content)
	}
	if !strings.Contains(content[ctxIdx:], "This is the seed prose") {
		t.Errorf("seed prose should fold into ## Context; got:\n%s", content[ctxIdx:])
	}
	// stdout names the created Idea path.
	if !strings.Contains(stdout, ideaPath) {
		t.Errorf("stdout should name created Idea path %q; got: %s", ideaPath, stdout)
	}

	// The verb commits the pure rename, so git log --follow reaches the
	// seed's original path across the subsequent transform.
	out, logErr := exec.Command("git", "-C", source, "log", "--follow", "--name-only",
		"--pretty=format:", "--", filepath.Join("spec", "ideas", "bar.md")).Output()
	if logErr != nil {
		t.Fatalf("git log --follow: %v", logErr)
	}
	if !strings.Contains(string(out), filepath.Join("spec", "ideas", "seeds", "bar.md")) {
		t.Errorf("git log --follow should reach the original seed path; got:\n%s", out)
	}

	// specscore spec lint passes (no error-severity violations).
	assertLintClean(t, source, "after promote bar")
}

// AC: dirty-tree-rejected
func TestIdeaPromoteCLI_DirtyTreeRejected(t *testing.T) {
	parent := t.TempDir()
	source := stagePromoteRepo(t, parent, "src", "src")
	writePromoteSeed(t, source, "foo", "Foo Idea", false)
	initGitRepoForTest(t, source)

	// Dirty the seed after the initial commit.
	seedPath := filepath.Join(source, "spec", "ideas", "seeds", "foo.md")
	if err := os.WriteFile(seedPath, []byte(promoteSeedBody("foo", "Foo Idea", false)+"\nedited\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	seedBefore, _ := os.ReadFile(seedPath)

	_, _, err := runIdeaPromoteCLI(t, source, "foo")
	if got := exitCodeFromErr(t, err); got != exitcode.DirtyTree {
		t.Errorf("exit code: got %d want %d (DirtyTree)", got, exitcode.DirtyTree)
	}
	// Nothing mutated.
	seedAfter, _ := os.ReadFile(seedPath)
	if string(seedBefore) != string(seedAfter) {
		t.Errorf("seed must be unchanged on dirty-tree rejection")
	}
	if _, statErr := os.Stat(filepath.Join(source, "spec", "ideas", "foo.md")); !os.IsNotExist(statErr) {
		t.Errorf("no Idea file should be created on dirty-tree rejection")
	}
}
