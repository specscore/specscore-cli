package cli

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/idea"
	"github.com/specscore/specscore-cli/pkg/ideapromote"
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
		"  host: github.com\n" +
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
	migrateTree(t, root)
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
	// The seed's frontmatter (type/slug/captured_*) is dropped; the promoted
	// Idea instead carries the Idea type's own format:/status: frontmatter per
	// the artifact-frontmatter-convention.
	if !strings.HasPrefix(content, "---\nformat: https://specscore.md/idea-specification\nstatus: Draft\n---") {
		t.Errorf("expected Idea frontmatter at the top; got prefix:\n%s", content[:min(120, len(content))])
	}
	for _, seedKey := range []string{"type: sidekick-seed", "captured_at:", "captured_by:"} {
		if strings.Contains(content, seedKey) {
			t.Errorf("seed frontmatter key %q should be dropped; got:\n%s", seedKey, content)
		}
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

// AC: same-repo-backlinks-reconciled
func TestIdeaPromoteCLI_SameRepoBackLinksReconciled(t *testing.T) {
	parent := t.TempDir()
	source := stagePromoteRepo(t, parent, "src", "src")
	writePromoteSeed(t, source, "bar", "Bar Idea", false)

	// A same-repo Feature with a Sidekick Seeds Generated back-link to
	// the seed (bare relative path → same-repo).
	featDir := filepath.Join(source, "spec", "features", "x")
	if err := os.MkdirAll(featDir, 0o755); err != nil {
		t.Fatalf("mkdir feature: %v", err)
	}
	featBody := "# Feature: X\n\n**Status:** Approved\n\n## Summary\n\nText.\n\n" +
		"## Sidekick Seeds Generated\n\n" +
		"- [bar](../../ideas/seeds/bar.md) — captured 2026-06-01 by specstudio:specify\n\n" +
		"## Open Questions\n\nNone at this time.\n\n" +
		"---\n*This document follows the https://specscore.md/feature-specification*\n"
	featPath := filepath.Join(featDir, "README.md")
	if err := os.WriteFile(featPath, []byte(featBody), 0o644); err != nil {
		t.Fatalf("write feature: %v", err)
	}
	migrateTree(t, source)
	initGitRepoForTest(t, source)

	stdout, _, err := runIdeaPromoteCLI(t, source, "bar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, readErr := os.ReadFile(featPath)
	if readErr != nil {
		t.Fatalf("read feature after: %v", readErr)
	}
	content := string(got)
	if !strings.Contains(content, "(../../ideas/bar.md)") {
		t.Errorf("back-link should point at the new Idea path; got:\n%s", content)
	}
	if strings.Contains(content, "ideas/seeds/bar.md") {
		t.Errorf("no back-link should still reference the seeds path; got:\n%s", content)
	}
	if !strings.Contains(stdout, "reconciled back-link") {
		t.Errorf("stdout should report the reconciled back-link; got: %s", stdout)
	}
	assertLintClean(t, source, "after backlink reconcile")
}

// readIdeaContext returns the ## Context section body of the Idea at path.
func readIdeaContext(t *testing.T, path string) string {
	t.Helper()
	p, err := idea.Parse(path)
	if err != nil {
		t.Fatalf("parse idea %s: %v", path, err)
	}
	s, ok := p.SectionByTitle["Context"]
	if !ok {
		t.Fatalf("no ## Context section in %s", path)
	}
	return s.Body
}

// AC: verdict-carry-forward-modes
func TestIdeaPromoteCLI_VerdictCarryForwardModes(t *testing.T) {
	verdictText := "The panel agreed this is a strong direction worth specifying."

	// Default mode (no flag, no config) → single-line pointer.
	t.Run("default-pointer", func(t *testing.T) {
		parent := t.TempDir()
		source := stagePromoteRepo(t, parent, "src", "src")
		writePromoteSeed(t, source, "vp", "Verdict Pointer Idea", true)
		initGitRepoForTest(t, source)
		if _, _, err := runIdeaPromoteCLI(t, source, "vp"); err != nil {
			t.Fatalf("promote: %v", err)
		}
		ctx := readIdeaContext(t, filepath.Join(source, "spec", "ideas", "vp.md"))
		if !strings.Contains(ctx, "Consilium verdict carried from") {
			t.Errorf("default should write a pointer; ## Context:\n%s", ctx)
		}
		if strings.Contains(ctx, verdictText) {
			t.Errorf("default pointer must NOT copy the full verdict text; ## Context:\n%s", ctx)
		}
	})

	// --verdict=full → copies the whole section text.
	t.Run("flag-full", func(t *testing.T) {
		parent := t.TempDir()
		source := stagePromoteRepo(t, parent, "src", "src")
		writePromoteSeed(t, source, "vf", "Verdict Full Idea", true)
		initGitRepoForTest(t, source)
		if _, _, err := runIdeaPromoteCLI(t, source, "vf", "--verdict=full"); err != nil {
			t.Fatalf("promote: %v", err)
		}
		ctx := readIdeaContext(t, filepath.Join(source, "spec", "ideas", "vf.md"))
		if !strings.Contains(ctx, verdictText) {
			t.Errorf("--verdict=full should copy the verdict body; ## Context:\n%s", ctx)
		}
	})

	// --verdict=drop → omits the verdict entirely.
	t.Run("flag-drop", func(t *testing.T) {
		parent := t.TempDir()
		source := stagePromoteRepo(t, parent, "src", "src")
		writePromoteSeed(t, source, "vd", "Verdict Drop Idea", true)
		initGitRepoForTest(t, source)
		if _, _, err := runIdeaPromoteCLI(t, source, "vd", "--verdict=drop"); err != nil {
			t.Fatalf("promote: %v", err)
		}
		ctx := readIdeaContext(t, filepath.Join(source, "spec", "ideas", "vd.md"))
		if strings.Contains(ctx, verdictText) || strings.Contains(ctx, "Consilium verdict carried") {
			t.Errorf("--verdict=drop should omit the verdict; ## Context:\n%s", ctx)
		}
	})

	// --verdict flag overrides the promote.verdict_carry_forward config.
	t.Run("flag-overrides-config", func(t *testing.T) {
		parent := t.TempDir()
		source := stagePromoteRepo(t, parent, "src", "src")
		// Config says drop; flag says full → flag wins.
		writePromoteConfigVerdict(t, source, "drop")
		writePromoteSeed(t, source, "vo", "Verdict Override Idea", true)
		initGitRepoForTest(t, source)
		if _, _, err := runIdeaPromoteCLI(t, source, "vo", "--verdict=full"); err != nil {
			t.Fatalf("promote: %v", err)
		}
		ctx := readIdeaContext(t, filepath.Join(source, "spec", "ideas", "vo.md"))
		if !strings.Contains(ctx, verdictText) {
			t.Errorf("--verdict=full should override config drop; ## Context:\n%s", ctx)
		}
	})

	// Config alone (no flag) drives the mode.
	t.Run("config-default-drop", func(t *testing.T) {
		parent := t.TempDir()
		source := stagePromoteRepo(t, parent, "src", "src")
		writePromoteConfigVerdict(t, source, "drop")
		writePromoteSeed(t, source, "vc", "Verdict Config Idea", true)
		initGitRepoForTest(t, source)
		if _, _, err := runIdeaPromoteCLI(t, source, "vc"); err != nil {
			t.Fatalf("promote: %v", err)
		}
		ctx := readIdeaContext(t, filepath.Join(source, "spec", "ideas", "vc.md"))
		if strings.Contains(ctx, "Consilium verdict carried") || strings.Contains(ctx, verdictText) {
			t.Errorf("config drop should omit verdict; ## Context:\n%s", ctx)
		}
	})

	// No verdict on the seed → pointer omitted regardless of mode.
	t.Run("omit-when-no-verdict", func(t *testing.T) {
		parent := t.TempDir()
		source := stagePromoteRepo(t, parent, "src", "src")
		writePromoteSeed(t, source, "nv", "No Verdict Idea", false)
		initGitRepoForTest(t, source)
		if _, _, err := runIdeaPromoteCLI(t, source, "nv"); err != nil {
			t.Fatalf("promote: %v", err)
		}
		ctx := readIdeaContext(t, filepath.Join(source, "spec", "ideas", "nv.md"))
		if strings.Contains(ctx, "Consilium verdict carried") {
			t.Errorf("no-verdict seed must not get a pointer; ## Context:\n%s", ctx)
		}
	})
}

// writePromoteConfigVerdict rewrites specscore.yaml to add a
// promote.verdict_carry_forward block.
func writePromoteConfigVerdict(t *testing.T, repoRoot, mode string) {
	t.Helper()
	yaml := "# SpecScore Repo Config Schema: https://specscore.md/repo-config\n" +
		"project:\n" +
		"  title: src\n" +
		"  host: github.com\n" +
		"  org: src\n" +
		"  repo: src\n" +
		"promote:\n" +
		"  verdict_carry_forward: " + mode + "\n"
	if err := os.WriteFile(filepath.Join(repoRoot, "specscore.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
}

// AC: cross-repo-archive + AC: never-deprecated
func TestIdeaPromoteCLI_CrossRepoArchive(t *testing.T) {
	parent := t.TempDir()
	source := stagePromoteRepo(t, parent, "src", "src")
	// A sibling repo so cross-repo discovery has somewhere to look; the
	// cross-repo back-link itself lives in the source repo's Feature with
	// a <repo-slug>:-qualified target.
	stagePromoteRepo(t, parent, "other-repo", "other-repo")
	writePromoteSeed(t, source, "baz", "Baz Idea", false)

	featDir := filepath.Join(source, "spec", "features", "x")
	if err := os.MkdirAll(featDir, 0o755); err != nil {
		t.Fatalf("mkdir feature: %v", err)
	}
	featBody := "# Feature: X\n\n**Status:** Approved\n\n## Summary\n\nText.\n\n" +
		"## Sidekick Seeds Generated\n\n" +
		"- [baz](other-repo:spec/ideas/seeds/baz.md) — captured 2026-06-01 by specstudio:specify\n\n" +
		"## Open Questions\n\nNone at this time.\n\n" +
		"---\n*This document follows the https://specscore.md/feature-specification*\n"
	featPath := filepath.Join(featDir, "README.md")
	featBefore := []byte(featBody)
	if err := os.WriteFile(featPath, featBefore, 0o644); err != nil {
		t.Fatalf("write feature: %v", err)
	}
	initGitRepoForTest(t, source)

	_, _, err := runIdeaPromoteCLI(t, source, "baz")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// New Idea exists.
	ideaPath := filepath.Join(source, "spec", "ideas", "baz.md")
	ideaBody, readErr := os.ReadFile(ideaPath)
	if readErr != nil {
		t.Fatalf("idea not created: %v", readErr)
	}
	if !strings.Contains(string(ideaBody), "# Idea: Baz Idea") {
		t.Errorf("idea should be a lint-clean skeleton; got:\n%s", ideaBody)
	}

	// Seed archived with status: promoted + promoted_to: baz.
	if _, statErr := os.Stat(filepath.Join(source, "spec", "ideas", "seeds", "baz.md")); !os.IsNotExist(statErr) {
		t.Errorf("seed should no longer exist at the seeds path")
	}
	archivedPath := filepath.Join(source, "spec", "ideas", "archived", "baz.md")
	archivedBody, readErr := os.ReadFile(archivedPath)
	if readErr != nil {
		t.Fatalf("archived seed missing: %v", readErr)
	}
	ab := string(archivedBody)
	if !strings.Contains(ab, "status: promoted") {
		t.Errorf("archived seed should have status: promoted; got:\n%s", ab)
	}
	if !strings.Contains(ab, "promoted_to: baz") {
		t.Errorf("archived seed should have promoted_to: baz; got:\n%s", ab)
	}
	if strings.Contains(ab, "deprecated") {
		t.Errorf("archived seed must NEVER be marked deprecated; got:\n%s", ab)
	}

	// Sibling repo's back-link was not rewritten (it lives in the source
	// Feature here; assert that the <repo-slug>: target is untouched).
	featAfter, _ := os.ReadFile(featPath)
	if !strings.Contains(string(featAfter), "other-repo:spec/ideas/seeds/baz.md") {
		t.Errorf("cross-repo back-link must NOT be rewritten; got:\n%s", featAfter)
	}

	// The new Idea is a lint-clean skeleton: full Idea title + every
	// required header field + every required section. (A whole-tree lint
	// pass is confounded by the archived seed sharing the slug, which the
	// idea-lint layer keys on — see the verb's cross-repo lint note.)
	assertValidIdeaSkeleton(t, ideaPath)
}

// assertValidIdeaSkeleton verifies the file at path is a structurally
// valid Idea: `# Idea:` title and all required header fields + sections.
func assertValidIdeaSkeleton(t *testing.T, path string) {
	t.Helper()
	p, err := idea.Parse(path)
	if err != nil {
		t.Fatalf("parse idea %s: %v", path, err)
	}
	if !p.TitleOK || p.TitlePrefix != "Idea" {
		t.Errorf("expected `# Idea:` title; got TitleOK=%v prefix=%q", p.TitleOK, p.TitlePrefix)
	}
	for _, f := range idea.RequiredHeaderFields {
		if _, ok := p.FieldByName[f]; !ok {
			t.Errorf("missing required header field %q", f)
		}
	}
	for _, s := range idea.RequiredSections {
		if _, ok := p.SectionByTitle[s]; !ok {
			t.Errorf("missing required section %q", s)
		}
	}
}

// AC: stdout-summary
func TestIdeaPromoteCLI_StdoutSummary(t *testing.T) {
	parent := t.TempDir()
	source := stagePromoteRepo(t, parent, "src", "src")
	writePromoteSeed(t, source, "bar", "Bar Idea", false)

	featDir := filepath.Join(source, "spec", "features", "x")
	if err := os.MkdirAll(featDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	featBody := "# Feature: X\n\n**Status:** Approved\n\n## Summary\n\nText.\n\n" +
		"## Sidekick Seeds Generated\n\n" +
		"- [bar](../../ideas/seeds/bar.md) — captured 2026-06-01\n\n" +
		"## Open Questions\n\nNone at this time.\n\n" +
		"---\n*This document follows the https://specscore.md/feature-specification*\n"
	if err := os.WriteFile(filepath.Join(featDir, "README.md"), []byte(featBody), 0o644); err != nil {
		t.Fatalf("write feature: %v", err)
	}
	initGitRepoForTest(t, source)

	resolvedSrc, _ := filepath.EvalSymlinks(source)
	ideaPath := filepath.Join(resolvedSrc, "spec", "ideas", "bar.md")

	// Text format: names created Idea path, seed moved, reconciled link.
	stdout, _, err := runIdeaPromoteCLI(t, source, "bar")
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if !strings.Contains(stdout, "created idea: "+ideaPath) {
		t.Errorf("text stdout should name created Idea path; got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "seed moved:") {
		t.Errorf("text stdout should report the seed as moved; got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "reconciled back-link:") {
		t.Errorf("text stdout should list reconciled back-links; got:\n%s", stdout)
	}

	// JSON format: same facts as structured output. Run a fresh promotion
	// (different slug) so the fixture is pristine.
	source2 := stagePromoteRepo(t, parent, "src2", "src2")
	writePromoteSeed(t, source2, "baz2", "Baz Two", false)
	initGitRepoForTest(t, source2)
	jsonOut, _, err := runIdeaPromoteCLI(t, source2, "baz2", "--format=json")
	if err != nil {
		t.Fatalf("promote json: %v", err)
	}
	var got ideapromote.Result
	if err := json.Unmarshal([]byte(jsonOut), &got); err != nil {
		t.Fatalf("json output not parseable: %v\n%s", err, jsonOut)
	}
	if got.Slug != "baz2" {
		t.Errorf("json slug: got %q want baz2", got.Slug)
	}
	if got.SeedFate != ideapromote.FateMoved {
		t.Errorf("json seed_fate: got %q want moved", got.SeedFate)
	}
	resolvedSrc2, _ := filepath.EvalSymlinks(source2)
	if got.IdeaPath != filepath.Join(resolvedSrc2, "spec", "ideas", "baz2.md") {
		t.Errorf("json idea_path: got %q", got.IdeaPath)
	}
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
