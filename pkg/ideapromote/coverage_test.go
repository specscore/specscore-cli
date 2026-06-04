package ideapromote

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/idea"
)

// gitInit initializes a git repo at root with a committed-friendly identity
// so the git-backed promote funcs record real renames/commits.
func gitInit(t *testing.T, root string) {
	t.Helper()
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "t@example.com"},
		{"config", "user.name", "Test"},
		{"config", "commit.gpgsign", "false"},
	} {
		cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
}

func gitCommitAll(t *testing.T, root, msg string) {
	t.Helper()
	if out, err := exec.Command("git", "-C", root, "add", "-A").CombinedOutput(); err != nil {
		t.Fatalf("git add: %v\n%s", err, out)
	}
	if out, err := exec.Command("git", "-C", root, "commit", "--no-gpg-sign", "-q", "-m", msg).CombinedOutput(); err != nil {
		t.Fatalf("git commit: %v\n%s", err, out)
	}
}

// --- resolve.go -----------------------------------------------------------

func TestPathsFor(t *testing.T) {
	p := PathsFor("foo")
	if p.SeedRel != "spec/ideas/seeds/foo.md" {
		t.Errorf("SeedRel: %q", p.SeedRel)
	}
	if p.IdeaRel != "spec/ideas/foo.md" {
		t.Errorf("IdeaRel: %q", p.IdeaRel)
	}
	if p.ArchivedSeedRel != "spec/ideas/archived/foo.md" {
		t.Errorf("ArchivedSeedRel: %q", p.ArchivedSeedRel)
	}
}

func TestResolveSeed(t *testing.T) {
	root := t.TempDir()
	// Missing → NotFound.
	if _, err := ResolveSeed(root, "ghost"); err == nil {
		t.Fatal("expected error for missing seed")
	} else if ec, ok := err.(*exitcode.Error); !ok || ec.ExitCode() != exitcode.NotFound {
		t.Errorf("want NotFound exitcode; got %v", err)
	}
	// Present → returns path.
	seed := filepath.Join(root, "spec", "ideas", "seeds", "foo.md")
	writeFileT(t, seed, "# Foo\n")
	got, err := ResolveSeed(root, "foo")
	if err != nil {
		t.Fatalf("ResolveSeed: %v", err)
	}
	if got != seed {
		t.Errorf("path: got %q want %q", got, seed)
	}
}

func TestCheckCollision(t *testing.T) {
	root := t.TempDir()
	// No destination → ok.
	if err := CheckCollision(root, "foo", false); err != nil {
		t.Fatalf("no collision should be ok: %v", err)
	}
	ideaPath := filepath.Join(root, "spec", "ideas", "foo.md")
	writeFileT(t, ideaPath, "# Idea: Foo\n")
	// Exists, no force → Conflict.
	if err := CheckCollision(root, "foo", false); err == nil {
		t.Fatal("expected collision error")
	} else if ec, ok := err.(*exitcode.Error); !ok || ec.ExitCode() != exitcode.Conflict {
		t.Errorf("want Conflict; got %v", err)
	}
	// Exists, force → ok.
	if err := CheckCollision(root, "foo", true); err != nil {
		t.Errorf("force should permit collision: %v", err)
	}
}

func TestFileExists(t *testing.T) {
	root := t.TempDir()
	if fileExists(filepath.Join(root, "nope")) {
		t.Error("missing path should be false")
	}
	if fileExists(root) {
		t.Error("directory should be false")
	}
	f := filepath.Join(root, "f.md")
	writeFileT(t, f, "x")
	if !fileExists(f) {
		t.Error("regular file should be true")
	}
}

// --- transform.go ---------------------------------------------------------

func TestParseSeed_Full(t *testing.T) {
	content := "---\n" +
		"type: sidekick-seed\n" +
		"captured_by: alice\n" +
		"captured_by: alice2\n" + // duplicate key keeps order once, last wins
		"novalueline\n" + // no colon → skipped
		": noKey\n" + // empty key → skipped
		"---\n" +
		"\n" +
		"# My Title\n" +
		"\n" +
		"Body paragraph one.\n" +
		"\n" +
		"## Consilium Verdict\n" +
		"\n" +
		"**Verdict:** go\n" +
		"\n" +
		"## Other Section\n" +
		"after verdict\n"
	sc := ParseSeed(content)
	if sc.Title != "My Title" {
		t.Errorf("Title: %q", sc.Title)
	}
	if sc.Frontmatter["captured_by"] != "alice2" {
		t.Errorf("captured_by last-wins: %q", sc.Frontmatter["captured_by"])
	}
	if len(sc.FrontmatterOrder) != 2 { // type, captured_by
		t.Errorf("FrontmatterOrder: %v", sc.FrontmatterOrder)
	}
	if !strings.Contains(sc.Body, "Body paragraph one.") {
		t.Errorf("Body: %q", sc.Body)
	}
	// Lines after a non-verdict H2 fall back into the prose body (the H2
	// heading line itself is dropped, but following content is captured).
	if !strings.Contains(sc.Body, "after verdict") {
		t.Errorf("post-other-H2 content should fall into body: %q", sc.Body)
	}
	if !strings.Contains(sc.Verdict, "## Consilium Verdict") || !strings.Contains(sc.Verdict, "**Verdict:** go") {
		t.Errorf("Verdict: %q", sc.Verdict)
	}
}

func TestParseSeed_NoFrontmatterNoTitle(t *testing.T) {
	// Body starts immediately (non-blank, non-title line before any H1).
	sc := ParseSeed("just prose here\nmore prose\n")
	if sc.Title != "" {
		t.Errorf("expected empty title; got %q", sc.Title)
	}
	if !strings.Contains(sc.Body, "just prose here") {
		t.Errorf("Body: %q", sc.Body)
	}
}

func TestTransform_VerdictModesAndScaffold(t *testing.T) {
	seed := SeedContent{
		Title:   "T",
		Body:    "ctx body",
		Verdict: "## Consilium Verdict\n\n**Verdict:** go\n",
	}

	// Pointer (default) with explicit ref.
	out, err := Transform(seed, TransformOptions{Slug: "s", VerdictMode: VerdictPointer, SeedRefForPointer: "spec/ideas/seeds/s.md"})
	if err != nil {
		t.Fatalf("Transform pointer: %v", err)
	}
	if !strings.Contains(string(out), "Consilium verdict carried from spec/ideas/seeds/s.md") {
		t.Errorf("pointer text missing:\n%s", out)
	}

	// Pointer with empty ref → fallback wording.
	out, _ = Transform(seed, TransformOptions{Slug: "s", VerdictMode: VerdictPointer})
	if !strings.Contains(string(out), "the originating seed") {
		t.Errorf("pointer fallback missing:\n%s", out)
	}

	// Full → demoted heading.
	out, _ = Transform(seed, TransformOptions{Slug: "s", VerdictMode: VerdictFull})
	if !strings.Contains(string(out), "### Consilium Verdict") {
		t.Errorf("full verdict demote missing:\n%s", out)
	}

	// Drop → no verdict text.
	out, _ = Transform(seed, TransformOptions{Slug: "s", VerdictMode: VerdictDrop})
	if strings.Contains(string(out), "Consilium verdict carried") || strings.Contains(string(out), "Consilium Verdict") {
		t.Errorf("drop should omit verdict:\n%s", out)
	}

	// Empty body + verdict → verdict becomes the context.
	empty := SeedContent{Title: "T", Body: "", Verdict: seed.Verdict}
	out, _ = Transform(empty, TransformOptions{Slug: "s", VerdictMode: VerdictFull})
	if !strings.Contains(string(out), "### Consilium Verdict") {
		t.Errorf("verdict-as-context missing:\n%s", out)
	}

	// No verdict at all → carryVerdict returns "".
	noVd := SeedContent{Title: "T", Body: "ctx"}
	out, _ = Transform(noVd, TransformOptions{Slug: "s", VerdictMode: VerdictPointer})
	if strings.Contains(string(out), "Consilium") {
		t.Errorf("no-verdict seed should carry nothing:\n%s", out)
	}
}

func TestTransform_ScaffoldError(t *testing.T) {
	orig := ideaScaffoldFn
	ideaScaffoldFn = func(_ idea.ScaffoldOptions) ([]byte, error) {
		return nil, errors.New("boom")
	}
	defer func() { ideaScaffoldFn = orig }()
	if _, err := Transform(SeedContent{Title: "T"}, TransformOptions{Slug: "s"}); err == nil {
		t.Fatal("expected scaffold error to propagate")
	}
}

func TestDemoteVerdictHeading_NoHeading(t *testing.T) {
	// No `## ` heading present → returned unchanged (loop finds nothing).
	in := "plain line\nanother\n"
	got := demoteVerdictHeading(in)
	if got != strings.TrimRight(in, "\n") {
		t.Errorf("expected unchanged; got %q", got)
	}
}

// --- verdict.go -----------------------------------------------------------

func TestValidateVerdict(t *testing.T) {
	for _, raw := range []string{"", "pointer", "full", "drop"} {
		if _, err := ValidateVerdict(raw); err != nil {
			t.Errorf("ValidateVerdict(%q): %v", raw, err)
		}
	}
	if _, err := ValidateVerdict("bogus"); err == nil {
		t.Fatal("expected error for bogus")
	} else if ec, ok := err.(*exitcode.Error); !ok || ec.ExitCode() != exitcode.InvalidArgs {
		t.Errorf("want InvalidArgs; got %v", err)
	}
}

func TestResolveVerdictMode(t *testing.T) {
	// Flag set → wins.
	if got := ResolveVerdictMode(t.TempDir(), VerdictFull); got != VerdictFull {
		t.Errorf("flag should win; got %q", got)
	}

	// No flag, no config → default pointer.
	root := writeRepoConfig(t, "")
	if got := ResolveVerdictMode(root, ""); got != DefaultVerdictMode {
		t.Errorf("default expected; got %q", got)
	}

	// Config provides a valid value.
	rootCfg := writeRepoConfig(t, "promote:\n  verdict_carry_forward: drop\n")
	if got := ResolveVerdictMode(rootCfg, ""); got != VerdictDrop {
		t.Errorf("config drop expected; got %q", got)
	}

	// Config provides an invalid value → falls back to default.
	rootBad := writeRepoConfig(t, "promote:\n  verdict_carry_forward: nonsense\n")
	if got := ResolveVerdictMode(rootBad, ""); got != DefaultVerdictMode {
		t.Errorf("invalid config should default; got %q", got)
	}
}

func TestReadConfigVerdictMode_NoConfig(t *testing.T) {
	// No specscore.yaml at all → ReadSpecConfig errors → (",", false).
	if _, ok := readConfigVerdictMode(t.TempDir()); ok {
		t.Error("missing config should return ok=false")
	}
}

// writeRepoConfig writes a minimal specscore.yaml (plus optional extra block)
// into a fresh temp dir and returns the dir.
func writeRepoConfig(t *testing.T, extra string) string {
	t.Helper()
	root := t.TempDir()
	yaml := "# SpecScore Repo Config Schema: https://specscore.md/repo-config\n" +
		"project:\n" +
		"  title: x\n" +
		"  host: github.com\n" +
		"  org: x\n" +
		"  repo: x\n" + extra
	if err := os.WriteFile(filepath.Join(root, "specscore.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return root
}

// --- mutate.go ------------------------------------------------------------

func TestGitMV_GitRepo(t *testing.T) {
	root := t.TempDir()
	gitInit(t, root)
	src := filepath.Join(root, "spec", "ideas", "seeds", "foo.md")
	writeFileT(t, src, "seed\n")
	gitCommitAll(t, root, "seed")
	if err := gitMV(root, "spec/ideas/seeds/foo.md", "spec/ideas/foo.md"); err != nil {
		t.Fatalf("gitMV: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "spec", "ideas", "foo.md")); err != nil {
		t.Errorf("dest missing after gitMV: %v", err)
	}
}

func TestGitMV_GitError(t *testing.T) {
	root := t.TempDir()
	gitInit(t, root)
	// Source not tracked / not present → git mv fails.
	if err := gitMV(root, "spec/ideas/seeds/missing.md", "spec/ideas/missing.md"); err == nil {
		t.Fatal("expected git mv error for missing source")
	}
}

func TestGitMV_NonGitFallback(t *testing.T) {
	root := t.TempDir()
	src := filepath.Join(root, "spec", "ideas", "seeds", "foo.md")
	writeFileT(t, src, "seed\n")
	if err := gitMV(root, "spec/ideas/seeds/foo.md", "spec/ideas/foo.md"); err != nil {
		t.Fatalf("non-git gitMV: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "spec", "ideas", "foo.md")); err != nil {
		t.Errorf("dest missing after os.Rename: %v", err)
	}
	// Rename error path: source absent.
	if err := gitMV(root, "spec/ideas/seeds/gone.md", "spec/ideas/gone.md"); err == nil {
		t.Fatal("expected rename error for missing source")
	}
}

func TestWriteFileAbs(t *testing.T) {
	root := t.TempDir()
	p := filepath.Join(root, "a", "b", "c.md")
	if err := writeFileAbs(p, []byte("hi")); err != nil {
		t.Fatalf("writeFileAbs: %v", err)
	}
	got, _ := os.ReadFile(p)
	if string(got) != "hi" {
		t.Errorf("content: %q", got)
	}
	// MkdirAll error: parent is a file.
	bad := filepath.Join(p, "sub", "x.md")
	if err := writeFileAbs(bad, []byte("x")); err == nil {
		t.Fatal("expected mkdir error when parent is a file")
	}
}

func TestWriteFileAbs_WriteError(t *testing.T) {
	root := t.TempDir()
	// Path is an existing directory → WriteFile fails.
	dir := filepath.Join(root, "d")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeFileAbs(dir, []byte("x")); err == nil {
		t.Fatal("expected write error when path is a directory")
	}
}

func TestSameRepoPromote_GitRepo(t *testing.T) {
	root := t.TempDir()
	gitInit(t, root)
	writeFileT(t, filepath.Join(root, "spec", "ideas", "seeds", "foo.md"), "seed\n")
	gitCommitAll(t, root, "seed")

	dst, err := SameRepoPromote(root, "foo", []byte("# Idea: Foo\n"))
	if err != nil {
		t.Fatalf("SameRepoPromote: %v", err)
	}
	if dst != filepath.Join(root, "spec", "ideas", "foo.md") {
		t.Errorf("dst: %q", dst)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != "# Idea: Foo\n" {
		t.Errorf("transformed body not written: %q", got)
	}
	// The rename commit links history via --follow.
	out, err := exec.Command("git", "-C", root, "log", "--follow", "--name-only",
		"--pretty=format:", "--", "spec/ideas/foo.md").Output()
	if err != nil {
		t.Fatalf("git log --follow: %v", err)
	}
	if !strings.Contains(string(out), "spec/ideas/seeds/foo.md") {
		t.Errorf("--follow should reach seed path:\n%s", out)
	}
}

func TestSameRepoPromote_NonGit(t *testing.T) {
	root := t.TempDir()
	writeFileT(t, filepath.Join(root, "spec", "ideas", "seeds", "foo.md"), "seed\n")
	dst, err := SameRepoPromote(root, "foo", []byte("# Idea: Foo\n"))
	if err != nil {
		t.Fatalf("SameRepoPromote non-git: %v", err)
	}
	got, _ := os.ReadFile(dst)
	if string(got) != "# Idea: Foo\n" {
		t.Errorf("body: %q", got)
	}
}

func TestSameRepoPromote_GitMVError(t *testing.T) {
	root := t.TempDir()
	gitInit(t, root)
	// No seed → gitMV fails → SameRepoPromote returns the error.
	if _, err := SameRepoPromote(root, "missing", []byte("x")); err == nil {
		t.Fatal("expected error when seed is absent in git repo")
	}
}

func TestGitCommitStage_NonGitNoops(t *testing.T) {
	root := t.TempDir()
	if err := gitCommit(root, "msg"); err != nil {
		t.Errorf("gitCommit non-git should be noop: %v", err)
	}
	if err := gitStageAll(root); err != nil {
		t.Errorf("gitStageAll non-git should be noop: %v", err)
	}
	if err := gitStage(root, "x"); err != nil {
		t.Errorf("gitStage non-git should be noop: %v", err)
	}
	// gitStage with no paths → noop even in a git repo.
	gitInit(t, root)
	if err := gitStage(root); err != nil {
		t.Errorf("gitStage no-paths should be noop: %v", err)
	}
}

func TestGitCommitStage_GitHappy(t *testing.T) {
	root := t.TempDir()
	gitInit(t, root)
	writeFileT(t, filepath.Join(root, "a.md"), "a\n")
	if err := gitStage(root, "a.md"); err != nil {
		t.Fatalf("gitStage: %v", err)
	}
	if err := gitStageAll(root); err != nil {
		t.Fatalf("gitStageAll: %v", err)
	}
	if err := gitCommit(root, "init"); err != nil {
		t.Fatalf("gitCommit: %v", err)
	}
}

func TestGitCommit_Error(t *testing.T) {
	root := t.TempDir()
	gitInit(t, root)
	// Nothing to commit → git commit exits non-zero.
	if err := gitCommit(root, "empty"); err == nil {
		t.Fatal("expected commit error on empty tree")
	}
}

func TestGitStage_Error(t *testing.T) {
	root := t.TempDir()
	gitInit(t, root)
	// Pathspec with no match and a bad option style: stage a path containing
	// a NUL is impossible; instead use a pathspec that git rejects.
	if err := gitStage(root, "../escape.md"); err == nil {
		t.Fatal("expected git add error for outside-repo pathspec")
	}
}

func TestIsGitRepoFn(t *testing.T) {
	if isGitRepoFn(t.TempDir()) {
		t.Error("fresh temp dir is not a git repo")
	}
	root := t.TempDir()
	gitInit(t, root)
	if !isGitRepoFn(root) {
		t.Error("git-init'd dir should be a repo")
	}
}

// --- preflight.go ---------------------------------------------------------

func TestPreflight_CleanAndDirty(t *testing.T) {
	root := t.TempDir()
	gitInit(t, root)
	writeFileT(t, filepath.Join(root, "spec", "ideas", "seeds", "foo.md"), "seed\n")
	gitCommitAll(t, root, "seed")

	// Clean (with an empty subject and a duplicate to exercise skip paths).
	if err := Preflight(root, []string{"", "spec/ideas/seeds/foo.md", "spec/ideas/seeds/foo.md"}); err != nil {
		t.Fatalf("clean preflight: %v", err)
	}

	// Dirty: modify the committed seed.
	writeFileT(t, filepath.Join(root, "spec", "ideas", "seeds", "foo.md"), "seed edited\n")
	err := Preflight(root, []string{"spec/ideas/seeds/foo.md"})
	if err == nil {
		t.Fatal("expected dirty-tree error")
	}
	if ec, ok := err.(*exitcode.Error); !ok || ec.ExitCode() != exitcode.DirtyTree {
		t.Errorf("want DirtyTree; got %v", err)
	}
}

func TestPreflight_NonGitVacuouslyClean(t *testing.T) {
	root := t.TempDir()
	if err := Preflight(root, []string{"spec/ideas/seeds/foo.md"}); err != nil {
		t.Errorf("non-git should be vacuously clean: %v", err)
	}
}

func TestPreflight_CheckError(t *testing.T) {
	orig := isPathCleanFn
	isPathCleanFn = func(_, _ string) (bool, error) { return false, errors.New("boom") }
	defer func() { isPathCleanFn = orig }()
	if err := Preflight(t.TempDir(), []string{"x"}); err == nil {
		t.Fatal("expected error from clean-check failure")
	}
}

// --- output.go ------------------------------------------------------------

func TestFormatText(t *testing.T) {
	r := Result{
		IdeaPath:            "/repo/spec/ideas/foo.md",
		SeedFate:            FateMoved,
		SeedPath:            "/repo/spec/ideas/foo.md",
		ReconciledBackLinks: []string{"spec/features/x/README.md"},
	}
	out := r.FormatText()
	for _, want := range []string{"created idea: /repo/spec/ideas/foo.md",
		"seed moved: /repo/spec/ideas/foo.md", "reconciled back-link: spec/features/x/README.md"} {
		if !strings.Contains(out, want) {
			t.Errorf("FormatText missing %q; got:\n%s", want, out)
		}
	}
	// No back-links branch.
	r2 := Result{IdeaPath: "/i", SeedFate: FateArchived, SeedPath: "/a"}
	if strings.Contains(r2.FormatText(), "reconciled back-link") {
		t.Errorf("no back-links should print none: %s", r2.FormatText())
	}
}

// --- archive.go -----------------------------------------------------------

func TestCrossRepoPromote(t *testing.T) {
	root := t.TempDir()
	gitInit(t, root)
	writeFileT(t, filepath.Join(root, "spec", "ideas", "seeds", "baz.md"),
		"---\nstatus: queued\n---\n# Baz\n\nprose\n")
	gitCommitAll(t, root, "seed")

	ideaAbs, archivedAbs, err := CrossRepoPromote(root, "baz", []byte("# Idea: Baz\n"),
		"---\nstatus: queued\n---\n# Baz\n\nprose\n")
	if err != nil {
		t.Fatalf("CrossRepoPromote: %v", err)
	}
	ib, _ := os.ReadFile(ideaAbs)
	if string(ib) != "# Idea: Baz\n" {
		t.Errorf("idea body: %q", ib)
	}
	ab, _ := os.ReadFile(archivedAbs)
	if !strings.Contains(string(ab), "status: promoted") || !strings.Contains(string(ab), "promoted_to: baz") {
		t.Errorf("archived marking missing:\n%s", ab)
	}
	if _, err := os.Stat(filepath.Join(root, "spec", "ideas", "seeds", "baz.md")); !os.IsNotExist(err) {
		t.Error("seed should be moved out of seeds/")
	}
}

func TestCrossRepoPromote_GitMVError(t *testing.T) {
	root := t.TempDir()
	gitInit(t, root)
	// No seed present → gitMV of the seed fails (Idea write succeeds first).
	if _, _, err := CrossRepoPromote(root, "ghost", []byte("# Idea: G\n"), "body"); err == nil {
		t.Fatal("expected gitMV error when seed is absent")
	}
}

func TestMarkArchivedSeed_MalformedNoClosingFence(t *testing.T) {
	// Opening fence but no closing fence → keys prepended after opener.
	seed := "---\nstatus: queued\n# Title\nprose\n"
	got := markArchivedSeed(seed, "baz")
	if !strings.Contains(got, "status: promoted") || !strings.Contains(got, "promoted_to: baz") {
		t.Errorf("malformed-frontmatter marking missing:\n%s", got)
	}
}

func TestFrontmatterKey(t *testing.T) {
	if got := frontmatterKey("status: x"); got != "status" {
		t.Errorf("got %q", got)
	}
	if got := frontmatterKey("no colon here"); got != "" {
		t.Errorf("expected empty; got %q", got)
	}
}

// --- backlink.go ----------------------------------------------------------

func TestDiscoverBackLinks_RepoError(t *testing.T) {
	// A spec/ path that is a FILE (not a dir) → discoverBackLinksInRepo
	// stat says not-a-dir → returns nil, nil. Exercises the !IsDir branch.
	root := t.TempDir()
	writeFileT(t, filepath.Join(root, "spec"), "i am a file")
	links, err := DiscoverBackLinks([]RepoRef{{Path: root}}, "foo")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(links) != 0 {
		t.Errorf("expected no links; got %d", len(links))
	}
}

func TestDiscoverBackLinks_NoSpecDir(t *testing.T) {
	links, err := DiscoverBackLinks([]RepoRef{{Path: t.TempDir()}}, "foo")
	if err != nil || links != nil {
		t.Errorf("missing spec dir → nil,nil; got %v, %v", links, err)
	}
}

func TestDiscoverBackLinksInRepo_SkipsNonMarkdownAndNoLinks(t *testing.T) {
	root := t.TempDir()
	// Non-markdown file in spec → skipped.
	writeFileT(t, filepath.Join(root, "spec", "data.txt"), "## Sidekick Seeds Generated\n- [x](ideas/seeds/foo.md)\n")
	// Markdown in section but no markdown link on the line → continue.
	writeFileT(t, filepath.Join(root, "spec", "features", "a", "README.md"),
		"## Sidekick Seeds Generated\n\nplain line no link\n")
	links, err := discoverBackLinksInRepo(root, "foo")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(links) != 0 {
		t.Errorf("expected 0 links; got %+v", links)
	}
}

func TestHasCrossRepo_AllSameRepo(t *testing.T) {
	if HasCrossRepo([]BackLink{{CrossRepo: false}}) {
		t.Error("no cross-repo links → false")
	}
}

func TestReconcileSameRepoBackLinks_ReadError(t *testing.T) {
	// A back-link pointing at a file that does not exist → read error.
	links := []BackLink{{RepoRoot: t.TempDir(), RelPath: "spec/features/ghost/README.md", Line: 0, Target: "../../ideas/seeds/foo.md"}}
	if _, err := ReconcileSameRepoBackLinks(links, "foo"); err == nil {
		t.Fatal("expected read error for missing back-link file")
	}
}

func TestReconcileSameRepoBackLinks_NoChangeBranches(t *testing.T) {
	root := t.TempDir()
	rel := "spec/features/a/README.md"
	body := "## Sidekick Seeds Generated\n- [x](../../ideas/seeds/foo.md)\n"
	writeFileT(t, filepath.Join(root, rel), body)

	// Line out of range → skipped; Line where target unchanged → skipped.
	links := []BackLink{
		{RepoRoot: root, RelPath: rel, Line: 999, Target: "../../ideas/seeds/foo.md"},
		// Target that does not contain the seeds suffix → rewrite returns same → no change.
		{RepoRoot: root, RelPath: rel, Line: 0, Target: "unrelated"},
	}
	results, err := ReconcileSameRepoBackLinks(links, "foo")
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(results) != 0 {
		t.Errorf("no modifications expected; got %d", len(results))
	}
	// Cross-repo links are skipped entirely.
	cross := []BackLink{{RepoRoot: root, RelPath: rel, Line: 0, Target: "other:ideas/seeds/foo.md", CrossRepo: true}}
	results, err = ReconcileSameRepoBackLinks(cross, "foo")
	if err != nil || len(results) != 0 {
		t.Errorf("cross-repo only → no results; got %v, %v", results, err)
	}
}

func TestRewriteSeedTargetToIdea_NoMatch(t *testing.T) {
	if got := rewriteSeedTargetToIdea("nothing/here.md", "foo"); got != "nothing/here.md" {
		t.Errorf("non-matching target should be unchanged; got %q", got)
	}
}

// forceGitRepo overrides isGitRepoFn to always report true so the git-backed
// funcs take their exec path even against a non-git dir — where the actual
// git command then fails, exercising the error branches.
func forceGitRepo(t *testing.T) {
	t.Helper()
	orig := isGitRepoFn
	isGitRepoFn = func(string) bool { return true }
	t.Cleanup(func() { isGitRepoFn = orig })
}

// --- additional error-branch coverage ------------------------------------

func TestGitMV_MkdirAllError(t *testing.T) {
	root := t.TempDir()
	// Make a file where the destination's parent dir would need to be.
	writeFileT(t, filepath.Join(root, "blocker"), "x")
	if err := gitMV(root, "spec/ideas/seeds/foo.md", "blocker/sub/foo.md"); err == nil {
		t.Fatal("expected MkdirAll error when a parent path is a file")
	}
}

func TestGitStageAll_ExecError(t *testing.T) {
	forceGitRepo(t)
	// Non-git dir + forced isGitRepo → `git add -A` fails.
	if err := gitStageAll(t.TempDir()); err == nil {
		t.Fatal("expected git add -A exec error")
	}
}

func TestGitCommit_StageAllError(t *testing.T) {
	forceGitRepo(t)
	// gitStageAll fails first inside gitCommit.
	if err := gitCommit(t.TempDir(), "msg"); err == nil {
		t.Fatal("expected gitCommit to surface gitStageAll error")
	}
}

func TestSameRepoPromote_CommitError(t *testing.T) {
	root := t.TempDir()
	gitInit(t, root)
	writeFileT(t, filepath.Join(root, "spec", "ideas", "seeds", "foo.md"), "seed\n")
	gitCommitAll(t, root, "seed")

	// Install a failing pre-commit hook so the post-mv rename commit fails
	// (gitMV succeeds first; gitCommit then returns an error).
	hookDir := filepath.Join(root, ".git", "hooks")
	if err := os.MkdirAll(hookDir, 0o755); err != nil {
		t.Fatal(err)
	}
	hook := filepath.Join(hookDir, "pre-commit")
	if err := os.WriteFile(hook, []byte("#!/bin/sh\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, err := SameRepoPromote(root, "foo", []byte("# Idea: Foo\n")); err == nil {
		t.Skip("pre-commit hook not honored in this environment")
	}
}

func TestSameRepoPromote_WriteError(t *testing.T) {
	root := t.TempDir()
	gitInit(t, root)
	writeFileT(t, filepath.Join(root, "spec", "ideas", "seeds", "foo.md"), "seed\n")
	gitCommitAll(t, root, "seed")

	orig := writeFileAbsFn
	writeFileAbsFn = func(string, []byte) error { return errors.New("write boom") }
	t.Cleanup(func() { writeFileAbsFn = orig })

	if _, err := SameRepoPromote(root, "foo", []byte("# Idea: Foo\n")); err == nil {
		t.Fatal("expected writeFileAbs error from SameRepoPromote")
	}
}

func TestSameRepoPromote_StageError(t *testing.T) {
	root := t.TempDir()
	gitInit(t, root)
	writeFileT(t, filepath.Join(root, "spec", "ideas", "seeds", "foo.md"), "seed\n")
	gitCommitAll(t, root, "seed")

	orig := gitStageFn
	gitStageFn = func(string, ...string) error { return errors.New("stage boom") }
	t.Cleanup(func() { gitStageFn = orig })

	if _, err := SameRepoPromote(root, "foo", []byte("# Idea: Foo\n")); err == nil {
		t.Fatal("expected gitStage error from SameRepoPromote")
	}
}

func TestCrossRepoPromote_WriteIdeaError(t *testing.T) {
	root := t.TempDir()
	// Pre-create a directory at the Idea path so writeFileAbs(ideaAbs) fails.
	if err := os.MkdirAll(filepath.Join(root, "spec", "ideas", "baz.md"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := CrossRepoPromote(root, "baz", []byte("x"), "body"); err == nil {
		t.Fatal("expected writeFileAbs(idea) error")
	}
}

func TestCrossRepoPromote_StageIdeaError(t *testing.T) {
	forceGitRepo(t)
	// Non-git dir but forced isGitRepo: writeFileAbs(idea) succeeds, then
	// gitStage(idea) execs `git add --` which fails.
	root := t.TempDir()
	if _, _, err := CrossRepoPromote(root, "baz", []byte("# Idea: Baz\n"), "body"); err == nil {
		t.Fatal("expected gitStage(idea) exec error")
	}
}

func TestCrossRepoPromote_WriteArchivedError(t *testing.T) {
	root := t.TempDir()
	gitInit(t, root)
	writeFileT(t, filepath.Join(root, "spec", "ideas", "seeds", "baz.md"), "---\n---\n# Baz\n")
	gitCommitAll(t, root, "seed")

	// Only the archived write goes through writeFileAbsFn (the Idea write
	// uses writeFileAbs directly), so failing the seam fails exactly that
	// step, after the Idea write and seed mv have succeeded.
	orig := writeFileAbsFn
	writeFileAbsFn = func(string, []byte) error { return errors.New("archived write boom") }
	t.Cleanup(func() { writeFileAbsFn = orig })

	if _, _, err := CrossRepoPromote(root, "baz", []byte("# Idea: Baz\n"), "body"); err == nil {
		t.Fatal("expected writeFileAbs(archived) error")
	}
}

func TestCrossRepoPromote_StageArchivedError(t *testing.T) {
	root := t.TempDir()
	gitInit(t, root)
	writeFileT(t, filepath.Join(root, "spec", "ideas", "seeds", "baz.md"), "---\n---\n# Baz\n")
	gitCommitAll(t, root, "seed")

	// Only the archived stage goes through gitStageFn (the Idea stage uses
	// gitStage directly), so failing the seam fails exactly that step.
	orig := gitStageFn
	gitStageFn = func(string, ...string) error { return errors.New("archived stage boom") }
	t.Cleanup(func() { gitStageFn = orig })

	if _, _, err := CrossRepoPromote(root, "baz", []byte("# Idea: Baz\n"), "body"); err == nil {
		t.Fatal("expected gitStage(archived) error")
	}
}

func TestCrossRepoPromote_EnsureStubError(t *testing.T) {
	root := t.TempDir()
	gitInit(t, root)
	writeFileT(t, filepath.Join(root, "spec", "ideas", "seeds", "baz.md"), "---\n---\n# Baz\n")
	gitCommitAll(t, root, "seed")

	orig := ensureArchivedIndexStubFn
	ensureArchivedIndexStubFn = func(string) (bool, error) { return false, errors.New("stub boom") }
	t.Cleanup(func() { ensureArchivedIndexStubFn = orig })

	if _, _, err := CrossRepoPromote(root, "baz", []byte("# Idea: Baz\n"), "body"); err == nil {
		t.Fatal("expected EnsureArchivedIndexStub error to propagate")
	}
}

func TestCrossRepoPromote_StageArchivedReadmeError(t *testing.T) {
	root := t.TempDir()
	gitInit(t, root)
	writeFileT(t, filepath.Join(root, "spec", "ideas", "seeds", "baz.md"), "---\n---\n# Baz\n")
	gitCommitAll(t, root, "seed")

	// Force the stub to report it was created so the README-stage branch runs,
	// and make gitStageFn fail only for the README path (the seed stage at the
	// earlier step must succeed so we reach the README stage).
	origEnsure := ensureArchivedIndexStubFn
	ensureArchivedIndexStubFn = func(string) (bool, error) { return true, nil }
	t.Cleanup(func() { ensureArchivedIndexStubFn = origEnsure })

	origStage := gitStageFn
	gitStageFn = func(repoRoot string, relPaths ...string) error {
		for _, p := range relPaths {
			if strings.HasSuffix(p, "archived/README.md") {
				return errors.New("readme stage boom")
			}
		}
		return gitStage(repoRoot, relPaths...)
	}
	t.Cleanup(func() { gitStageFn = origStage })

	if _, _, err := CrossRepoPromote(root, "baz", []byte("# Idea: Baz\n"), "body"); err == nil {
		t.Fatal("expected gitStage(README) error to propagate")
	}
}

func TestMarkArchivedSeed_KeysAlreadyPresent(t *testing.T) {
	// Both status and promoted_to present → in-place rewrite, no inserts.
	seed := "---\nstatus: queued\npromoted_to: old\n---\n# Baz\n"
	got := markArchivedSeed(seed, "baz")
	if !strings.Contains(got, "status: promoted") {
		t.Errorf("status not rewritten:\n%s", got)
	}
	if !strings.Contains(got, "promoted_to: baz") || strings.Contains(got, "promoted_to: old") {
		t.Errorf("promoted_to not rewritten:\n%s", got)
	}
}

func TestDiscoverBackLinksInRepo_ReadFileError(t *testing.T) {
	root := t.TempDir()
	// An unreadable .md file in a Sidekick section → ReadFile errs → skipped.
	f := filepath.Join(root, "spec", "f", "a.md")
	writeFileT(t, f, "## Sidekick Seeds Generated\n- [x](../../ideas/seeds/foo.md)\n")
	if err := os.Chmod(f, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(f, 0o644) })
	links, err := discoverBackLinksInRepo(root, "foo")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(links) != 0 {
		t.Skip("filesystem allowed reading a 0-perm file (running as root?)")
	}
}

func TestDiscoverBackLinks_WalkError(t *testing.T) {
	orig := filepathWalkFn
	filepathWalkFn = func(_ string, _ filepath.WalkFunc) error { return errors.New("walk boom") }
	t.Cleanup(func() { filepathWalkFn = orig })

	root := t.TempDir()
	writeFileT(t, filepath.Join(root, "spec", "f", "a.md"), "x")
	// discoverBackLinksInRepo returns the wrapped walk error; DiscoverBackLinks
	// propagates it (covers both the walkErr branch and the propagation).
	if _, err := DiscoverBackLinks([]RepoRef{{Path: root}}, "foo"); err == nil {
		t.Fatal("expected walk error to propagate")
	}
}

func TestDiscoverBackLinksInRepo_RelError(t *testing.T) {
	orig := filepathRelFn
	filepathRelFn = func(_, _ string) (string, error) { return "", errors.New("rel boom") }
	t.Cleanup(func() { filepathRelFn = orig })

	root := t.TempDir()
	// A matching back-link so the Rel call is reached and its error skips it.
	writeFileT(t, filepath.Join(root, "spec", "f", "a.md"),
		"## Sidekick Seeds Generated\n- [x](../../ideas/seeds/foo.md)\n")
	links, err := discoverBackLinksInRepo(root, "foo")
	if err != nil {
		t.Fatalf("unexpected: %v", err)
	}
	if len(links) != 0 {
		t.Errorf("Rel error should skip the entry; got %d links", len(links))
	}
}

func TestReconcileSameRepoBackLinks_WriteError(t *testing.T) {
	root := t.TempDir()
	rel := "spec/features/a/README.md"
	abs := filepath.Join(root, rel)
	writeFileT(t, abs, "## Sidekick Seeds Generated\n- [x](../../ideas/seeds/foo.md)\n")
	// Make the file read-only AND its dir read-only so WriteFile fails.
	if err := os.Chmod(abs, 0o444); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(filepath.Dir(abs), 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = os.Chmod(filepath.Dir(abs), 0o755)
		_ = os.Chmod(abs, 0o644)
	})
	links := []BackLink{{RepoRoot: root, RelPath: rel, Line: 1, Target: "../../ideas/seeds/foo.md"}}
	if _, err := ReconcileSameRepoBackLinks(links, "foo"); err == nil {
		t.Skip("filesystem allowed write to read-only file (running as root?)")
	}
}
