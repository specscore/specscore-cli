package cli

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// stageRelocateRepo creates a SpecScore-managed repo dir at
// <parent>/<name> with project.org=project.repo=<repoSlug>. Returns
// the repo's absolute path. The shared org=repo convention matches the
// single-org SpecScore workspaces these tests model; Task 4's cross-
// repo link rewrite reads project.org from the target repo, so setting
// it here lets every test fixture build well-formed GitHub URLs without
// per-test boilerplate.
func stageRelocateRepo(t *testing.T, parent, name, repoSlug string) string {
	t.Helper()
	root := filepath.Join(parent, name)
	if err := os.MkdirAll(filepath.Join(root, "spec", "ideas", "seeds"), 0o755); err != nil {
		t.Fatalf("mkdir spec tree: %v", err)
	}
	yaml := "# SpecScore Repo Config Schema: https://specscore.md/repo-config\n" +
		"project:\n" +
		"  title: " + name + "\n" +
		"  org: " + repoSlug + "\n" +
		"  repo: " + repoSlug + "\n"
	if err := os.WriteFile(filepath.Join(root, "specscore.yaml"), []byte(yaml), 0o644); err != nil {
		t.Fatalf("write specscore.yaml: %v", err)
	}
	return root
}

func writeIdeaFile(t *testing.T, repoRoot, slug string) {
	t.Helper()
	body := "# Idea: " + slug + "\n\nplaceholder\n"
	if err := os.WriteFile(filepath.Join(repoRoot, "spec", "ideas", slug+".md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write idea: %v", err)
	}
}

func writeSeedFile(t *testing.T, repoRoot, slug string) {
	t.Helper()
	body := "# Seed: " + slug + "\n\nplaceholder\n"
	if err := os.WriteFile(filepath.Join(repoRoot, "spec", "ideas", "seeds", slug+".md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
}

func exitCodeFromErr(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	var ec *exitcode.Error
	if !errors.As(err, &ec) {
		t.Fatalf("expected *exitcode.Error, got %T: %v", err, err)
	}
	return ec.ExitCode()
}

// runIdeaRelocateCLI invokes `idea relocate` via the cobra command tree.
// Uses a shared root parent dir so sibling-dir scan finds the configured
// siblings.
func runIdeaRelocateCLI(t *testing.T, sourceDir string, args ...string) (string, string, error) {
	t.Helper()
	withCwd(t, sourceDir)
	full := append([]string{"relocate"}, args...)
	return runIdea(t, full...)
}

// -------- AC tests --------

// specscore:verifies feature/cli/idea/relocate#ac:ambiguous-slug-rejected
func TestIdeaRelocateCLI_AmbiguousSlugRejected(t *testing.T) {
	parent := t.TempDir()
	source := stageRelocateRepo(t, parent, "src", "src")
	stageRelocateRepo(t, parent, "tgt", "tgt")
	writeIdeaFile(t, source, "foo")
	writeSeedFile(t, source, "foo")

	_, _, err := runIdeaRelocateCLI(t, source, "foo", "--to-repo=tgt")
	if got := exitCodeFromErr(t, err); got != exitcode.AmbiguousSlug {
		t.Errorf("exit code: got %d want %d (AmbiguousSlug)", got, exitcode.AmbiguousSlug)
	}
}

// specscore:verifies feature/cli/idea/relocate#ac:slug-not-found
func TestIdeaRelocateCLI_SlugNotFound(t *testing.T) {
	parent := t.TempDir()
	source := stageRelocateRepo(t, parent, "src", "src")
	stageRelocateRepo(t, parent, "tgt", "tgt")

	_, _, err := runIdeaRelocateCLI(t, source, "nonexistent", "--to-repo=tgt")
	if got := exitCodeFromErr(t, err); got != exitcode.NotFound {
		t.Errorf("exit code: got %d want %d (NotFound)", got, exitcode.NotFound)
	}
}

// specscore:verifies feature/cli/idea/relocate#ac:to-repo-slug-form-resolves-via-sibling-scan
func TestIdeaRelocateCLI_ToRepoSlugFormResolvesViaScan(t *testing.T) {
	parent := t.TempDir()
	source := stageRelocateRepo(t, parent, "specstudio-skills", "specstudio-skills")
	target := stageRelocateRepo(t, parent, "specscore", "specscore")
	stageRelocateRepo(t, parent, "specscore-cli", "specscore-cli") // bystander
	writeIdeaFile(t, source, "foo")

	stdout, _, err := runIdeaRelocateCLI(t, source, "foo", "--to-repo=specscore")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = target
	// Stdout's per-repo lines use project.repo, not the directory path —
	// the target appears under its repo slug "specscore".
	if !strings.Contains(stdout, "specscore: received idea foo") {
		t.Errorf("stdout should contain target's per-repo line; got: %s", stdout)
	}
}

// specscore:verifies feature/cli/idea/relocate#ac:to-repo-path-form-bypasses-scan
func TestIdeaRelocateCLI_ToRepoPathFormBypassesScan(t *testing.T) {
	parent := t.TempDir()
	source := stageRelocateRepo(t, parent, "src", "src")
	target := stageRelocateRepo(t, parent, "tgt", "tgt-name-differs-from-dirname")
	writeIdeaFile(t, source, "foo")

	// Path form: contains "/". Should be honored verbatim even though
	// the target's project.repo differs from "tgt".
	stdout, _, err := runIdeaRelocateCLI(t, source, "foo", "--to-repo=../tgt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	_ = target
	// Path-form resolution still drives the stdout per-repo line via
	// the target's project.repo value — not the directory name.
	if !strings.Contains(stdout, "tgt-name-differs-from-dirname: received idea foo") {
		t.Errorf("stdout should name target by its project.repo value; got: %s", stdout)
	}
}

// specscore:verifies feature/cli/idea/relocate#ac:to-repo-without-specscore-yaml-rejected
func TestIdeaRelocateCLI_ToRepoWithoutSpecScoreYamlRejected(t *testing.T) {
	parent := t.TempDir()
	source := stageRelocateRepo(t, parent, "src", "src")
	bareDir := filepath.Join(parent, "bare")
	if err := os.MkdirAll(bareDir, 0o755); err != nil {
		t.Fatalf("mkdir bare: %v", err)
	}
	writeIdeaFile(t, source, "foo")

	_, _, err := runIdeaRelocateCLI(t, source, "foo", "--to-repo=../bare")
	if got := exitCodeFromErr(t, err); got != exitcode.TargetNotSpecScore {
		t.Errorf("exit code: got %d want %d (TargetNotSpecScore)", got, exitcode.TargetNotSpecScore)
	}
}

// specscore:verifies feature/cli/idea/relocate#ac:to-repo-slug-multiple-matches-rejected
func TestIdeaRelocateCLI_ToRepoSlugMultipleMatchesRejected(t *testing.T) {
	parent := t.TempDir()
	source := stageRelocateRepo(t, parent, "src", "src")
	stageRelocateRepo(t, parent, "specscore-a", "specscore")
	stageRelocateRepo(t, parent, "specscore-b", "specscore")
	writeIdeaFile(t, source, "foo")

	_, _, err := runIdeaRelocateCLI(t, source, "foo", "--to-repo=specscore")
	if got := exitCodeFromErr(t, err); got != exitcode.InvalidArgs {
		t.Errorf("exit code: got %d want %d (InvalidArgs)", got, exitcode.InvalidArgs)
	}
}

// Smoke: --to-repo is required at flag-parse time.
func TestIdeaRelocateCLI_MissingToRepoRejected(t *testing.T) {
	parent := t.TempDir()
	source := stageRelocateRepo(t, parent, "src", "src")
	writeIdeaFile(t, source, "foo")

	_, _, err := runIdeaRelocateCLI(t, source, "foo")
	if err == nil {
		t.Fatal("expected error when --to-repo is missing")
	}
}

// initGitRepoForTest runs git init + commit-all in root. After this
// call the working tree is clean from git's perspective.
func initGitRepoForTest(t *testing.T, root string) {
	t.Helper()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = root
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), root, err, out)
		}
	}
	run("init", "--initial-branch=main")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "Test")
	// Disable signing locally so the verb's own `git commit -m …`
	// invocations (without --no-gpg-sign) work regardless of the
	// developer's global gpg config.
	run("config", "commit.gpgsign", "false")
	run("add", "-A")
	run("commit", "--no-gpg-sign", "-m", "initial")
}

// specscore:verifies feature/cli/idea/relocate#ac:preflight-source-dirty
func TestIdeaRelocateCLI_PreflightSourceDirty(t *testing.T) {
	parent := t.TempDir()
	source := stageRelocateRepo(t, parent, "src", "src")
	target := stageRelocateRepo(t, parent, "tgt", "tgt")
	writeIdeaFile(t, source, "foo")
	initGitRepoForTest(t, source)
	initGitRepoForTest(t, target)

	// Modify foo.md after the initial commit — unstaged dirty.
	if err := os.WriteFile(
		filepath.Join(source, "spec", "ideas", "foo.md"),
		[]byte("# Idea: foo\n\nedited after commit\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, _, err := runIdeaRelocateCLI(t, source, "foo", "--to-repo=tgt")
	if got := exitCodeFromErr(t, err); got != exitcode.DirtyTree {
		t.Errorf("exit code: got %d want %d (DirtyTree)", got, exitcode.DirtyTree)
	}
	if !strings.Contains(err.Error(), "spec/ideas/foo.md") {
		t.Errorf("error should name the dirty source path; got: %v", err)
	}
}

// specscore:verifies feature/cli/idea/relocate#ac:preflight-sibling-with-references-dirty
func TestIdeaRelocateCLI_PreflightSiblingWithReferencesDirty(t *testing.T) {
	parent := t.TempDir()
	source := stageRelocateRepo(t, parent, "src", "src")
	target := stageRelocateRepo(t, parent, "tgt", "tgt")
	sib := stageRelocateRepo(t, parent, "sib", "sib")
	writeIdeaFile(t, source, "foo")

	// Sibling has a Feature that references the slug via markdown link.
	if err := os.MkdirAll(filepath.Join(sib, "spec", "features", "x"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	refFile := filepath.Join(sib, "spec", "features", "x", "README.md")
	if err := os.WriteFile(refFile,
		[]byte("# Feature: X\n\nSee [the Idea](../../../src/spec/ideas/foo.md).\n"), 0o644); err != nil {
		t.Fatalf("write ref: %v", err)
	}

	initGitRepoForTest(t, source)
	initGitRepoForTest(t, target)
	initGitRepoForTest(t, sib)

	// Now dirty the sibling reference file.
	if err := os.WriteFile(refFile,
		[]byte("# Feature: X\n\nSee [the Idea](../../../src/spec/ideas/foo.md). modified\n"), 0o644); err != nil {
		t.Fatalf("modify ref: %v", err)
	}

	_, _, err := runIdeaRelocateCLI(t, source, "foo", "--to-repo=tgt")
	if got := exitCodeFromErr(t, err); got != exitcode.DirtyTree {
		t.Errorf("exit code: got %d want %d (DirtyTree)", got, exitcode.DirtyTree)
	}
	// Error message should name the sibling repo path AND the affected file.
	msg := err.Error()
	if !strings.Contains(msg, sib) {
		t.Errorf("error should name sibling repo %q; got: %s", sib, msg)
	}
	if !strings.Contains(msg, filepath.Join("spec", "features", "x", "README.md")) {
		t.Errorf("error should name sibling ref path; got: %s", msg)
	}
}

// Sanity: when everything is git-clean, preflight passes and the verb
// reaches Task 1's resolution-summary output. After Task 3 the verb
// also performs the file mutation, so the source artifact is gone
// after this call.
func TestIdeaRelocateCLI_PreflightCleanAllowsThrough(t *testing.T) {
	parent := t.TempDir()
	source := stageRelocateRepo(t, parent, "src", "src")
	target := stageRelocateRepo(t, parent, "tgt", "tgt")
	writeIdeaFile(t, source, "foo")
	initGitRepoForTest(t, source)
	initGitRepoForTest(t, target)

	stdout, _, err := runIdeaRelocateCLI(t, source, "foo", "--to-repo=tgt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// New stdout-format: per-repo lines for source and target plus the
	// summary line. With git initialized in both, each per-repo line
	// includes a 7-char SHA bracket.
	if !strings.Contains(stdout, "src: moved idea foo") {
		t.Errorf("stdout should contain source per-repo line; got: %s", stdout)
	}
	if !strings.Contains(stdout, "tgt: received idea foo") {
		t.Errorf("stdout should contain target per-repo line; got: %s", stdout)
	}
	if !strings.Contains(stdout, "relocate complete: 2 repos affected") {
		t.Errorf("stdout should contain summary line; got: %s", stdout)
	}
}

// specscore:verifies feature/cli/idea/relocate#ac:destination-collision
func TestIdeaRelocateCLI_DestinationCollision(t *testing.T) {
	parent := t.TempDir()
	source := stageRelocateRepo(t, parent, "src", "src")
	target := stageRelocateRepo(t, parent, "tgt", "tgt")
	writeIdeaFile(t, source, "foo")
	// Pre-existing file at the destination → collision.
	writeIdeaFile(t, target, "foo")

	srcPath := filepath.Join(source, "spec", "ideas", "foo.md")
	tgtPath := filepath.Join(target, "spec", "ideas", "foo.md")
	srcBefore, err := os.ReadFile(srcPath)
	if err != nil {
		t.Fatalf("read src: %v", err)
	}
	tgtBefore, err := os.ReadFile(tgtPath)
	if err != nil {
		t.Fatalf("read tgt: %v", err)
	}

	_, _, err = runIdeaRelocateCLI(t, source, "foo", "--to-repo=tgt")
	if got := exitCodeFromErr(t, err); got != exitcode.Conflict {
		t.Errorf("exit code: got %d want %d (Conflict)", got, exitcode.Conflict)
	}
	srcAfter, _ := os.ReadFile(srcPath)
	tgtAfter, _ := os.ReadFile(tgtPath)
	if string(srcBefore) != string(srcAfter) {
		t.Errorf("source artifact must be unchanged on collision; before=%q after=%q", srcBefore, srcAfter)
	}
	if string(tgtBefore) != string(tgtAfter) {
		t.Errorf("target artifact must be unchanged on collision; before=%q after=%q", tgtBefore, tgtAfter)
	}
}

// specscore:verifies feature/cli/idea/relocate#ac:in-file-rewrite-org-rename
func TestIdeaRelocateCLI_InFileRewriteOrgRename(t *testing.T) {
	parent := t.TempDir()
	source := stageRelocateRepo(t, parent, "src", "src")
	target := stageRelocateRepo(t, parent, "tgt", "tgt")
	body := "# Idea: foo\n\nSee synchestra-io/specscore for the worked example, and synchestra-io/specscore-cli for the verb.\n"
	srcPath := filepath.Join(source, "spec", "ideas", "foo.md")
	if err := os.WriteFile(srcPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, _, err := runIdeaRelocateCLI(t, source, "foo", "--to-repo=tgt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tgtPath := filepath.Join(target, "spec", "ideas", "foo.md")
	got, err := os.ReadFile(tgtPath)
	if err != nil {
		t.Fatalf("read tgt: %v", err)
	}
	if !strings.Contains(string(got), "specscore/specscore") {
		t.Errorf("expected synchestra-io/specscore → specscore/specscore; got:\n%s", got)
	}
	if !strings.Contains(string(got), "specscore/specscore-cli") {
		t.Errorf("expected synchestra-io/specscore-cli → specscore/specscore-cli; got:\n%s", got)
	}
	if strings.Contains(string(got), "synchestra-io/") {
		t.Errorf("expected all synchestra-io/ replaced; got:\n%s", got)
	}
	// Source artifact deleted post-copy.
	if _, err := os.Stat(srcPath); !os.IsNotExist(err) {
		t.Errorf("source artifact must be removed after successful relocate; stat err: %v", err)
	}
}

// specscore:verifies feature/cli/idea/relocate#ac:in-file-rewrite-this-repo
func TestIdeaRelocateCLI_InFileRewriteThisRepo(t *testing.T) {
	parent := t.TempDir()
	source := stageRelocateRepo(t, parent, "src", "src")
	target := stageRelocateRepo(t, parent, "tgt", "tgt")
	body := "# Idea: foo\n\n" +
		"Existing artifacts in this repo are migrated by a one-shot script.\n\n" +
		"```\n" +
		"git -C this-repo log\n" +
		"this repo intact inside fence\n" +
		"```\n"
	srcPath := filepath.Join(source, "spec", "ideas", "foo.md")
	if err := os.WriteFile(srcPath, []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, _, err := runIdeaRelocateCLI(t, source, "foo", "--to-repo=tgt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tgtPath := filepath.Join(target, "spec", "ideas", "foo.md")
	got, err := os.ReadFile(tgtPath)
	if err != nil {
		t.Fatalf("read tgt: %v", err)
	}
	if !strings.Contains(string(got), "Existing artifacts in tgt are migrated") {
		t.Errorf("expected 'this repo' rewritten to 'tgt' in body prose; got:\n%s", got)
	}
	if !strings.Contains(string(got), "git -C this-repo log") {
		t.Errorf("expected fenced 'this-repo' content preserved; got:\n%s", got)
	}
	if !strings.Contains(string(got), "this repo intact inside fence") {
		t.Errorf("expected fenced 'this repo' phrase preserved; got:\n%s", got)
	}
}

// specscore:verifies feature/cli/idea/relocate#ac:cross-repo-link-cleanup-markdown-link
func TestIdeaRelocateCLI_CrossRepoLinkCleanupMarkdownLink(t *testing.T) {
	parent := t.TempDir()
	source := stageRelocateRepo(t, parent, "specstudio-skills", "specstudio-skills")
	stageRelocateRepo(t, parent, "specscore", "specscore")
	sib := stageRelocateRepo(t, parent, "specscore-cli", "specscore-cli")
	writeIdeaFile(t, source, "foo")

	// Sibling repo has a Feature linking to the source artifact.
	if err := os.MkdirAll(filepath.Join(sib, "spec", "features", "x"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	refPath := filepath.Join(sib, "spec", "features", "x", "README.md")
	if err := os.WriteFile(refPath,
		[]byte("# Feature: X\n\nSee [the Idea](../../../specstudio-skills/spec/ideas/foo.md) for context.\n"),
		0o644); err != nil {
		t.Fatalf("write ref: %v", err)
	}

	_, _, err := runIdeaRelocateCLI(t, source, "foo", "--to-repo=specscore")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(refPath)
	if err != nil {
		t.Fatalf("read ref after: %v", err)
	}
	wantURL := "https://github.com/specscore/specscore/blob/main/spec/ideas/foo.md"
	if !strings.Contains(string(got), wantURL) {
		t.Errorf("expected sibling link rewritten to %q; got:\n%s", wantURL, got)
	}
	if !strings.Contains(string(got), "[the Idea]") {
		t.Errorf("expected display text 'the Idea' preserved; got:\n%s", got)
	}
	if strings.Contains(string(got), "../../../specstudio-skills/spec/ideas/foo.md") {
		t.Errorf("expected old relative link replaced; got:\n%s", got)
	}
}

// specscore:verifies feature/cli/idea/relocate#ac:cross-repo-link-cleanup-preserves-slug-metadata
func TestIdeaRelocateCLI_CrossRepoLinkCleanupPreservesSlugMetadata(t *testing.T) {
	parent := t.TempDir()
	source := stageRelocateRepo(t, parent, "specstudio-skills", "specstudio-skills")
	stageRelocateRepo(t, parent, "specscore", "specscore")
	sib := stageRelocateRepo(t, parent, "specscore-cli", "specscore-cli")
	writeIdeaFile(t, source, "foo")

	// Sibling Feature has a slug-only metadata reference. After
	// relocate, that line must be byte-for-byte unchanged.
	if err := os.MkdirAll(filepath.Join(sib, "spec", "features", "y"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	refPath := filepath.Join(sib, "spec", "features", "y", "README.md")
	original := "# Feature: Y\n\n**Source Ideas:** foo\n\nBody text.\n"
	if err := os.WriteFile(refPath, []byte(original), 0o644); err != nil {
		t.Fatalf("write ref: %v", err)
	}

	_, _, err := runIdeaRelocateCLI(t, source, "foo", "--to-repo=specscore")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	got, err := os.ReadFile(refPath)
	if err != nil {
		t.Fatalf("read ref after: %v", err)
	}
	if string(got) != original {
		t.Errorf("expected slug-only metadata line preserved;\nwant:\n%s\ngot:\n%s", original, got)
	}
}

// gitHEADSubject returns the subject line of HEAD in repoRoot.
func gitHEADSubject(t *testing.T, repoRoot string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", repoRoot, "log", "-1", "--pretty=%s")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git log in %s: %v", repoRoot, err)
	}
	return strings.TrimRight(string(out), "\n")
}

// gitWorkingTreeStatus returns the porcelain output of `git status` in repoRoot.
func gitWorkingTreeStatus(t *testing.T, repoRoot string) string {
	t.Helper()
	cmd := exec.Command("git", "-C", repoRoot, "status", "--porcelain")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git status in %s: %v", repoRoot, err)
	}
	return string(out)
}

// specscore:verifies feature/cli/idea/relocate#ac:auto-commit-three-repo-flow
func TestIdeaRelocateCLI_AutoCommitThreeRepoFlow(t *testing.T) {
	parent := t.TempDir()
	source := stageRelocateRepo(t, parent, "specstudio-skills", "specstudio-skills")
	target := stageRelocateRepo(t, parent, "specscore", "specscore")
	sib := stageRelocateRepo(t, parent, "specscore-cli", "specscore-cli")
	writeIdeaFile(t, source, "foo")

	// Sibling has a link to the source artifact — so link cleanup will
	// modify it and it'll be an affected repo.
	if err := os.MkdirAll(filepath.Join(sib, "spec", "features", "x"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	refPath := filepath.Join(sib, "spec", "features", "x", "README.md")
	if err := os.WriteFile(refPath,
		[]byte("# Feature: X\n\nSee [the Idea](../../../specstudio-skills/spec/ideas/foo.md).\n"),
		0o644); err != nil {
		t.Fatalf("write ref: %v", err)
	}

	initGitRepoForTest(t, source)
	initGitRepoForTest(t, target)
	initGitRepoForTest(t, sib)

	stdout, _, err := runIdeaRelocateCLI(t, source, "foo", "--to-repo=specscore")
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout:\n%s", err, stdout)
	}

	// Each repo has a new commit at HEAD with the canonical subject.
	if got, want := gitHEADSubject(t, source), "chore(relocate): move idea foo to specscore"; got != want {
		t.Errorf("source HEAD subject: got %q want %q", got, want)
	}
	if got, want := gitHEADSubject(t, target), "chore(relocate): receive idea foo from specstudio-skills"; got != want {
		t.Errorf("target HEAD subject: got %q want %q", got, want)
	}
	if got, want := gitHEADSubject(t, sib), "chore(relocate): update links for foo (specstudio-skills → specscore)"; got != want {
		t.Errorf("sibling HEAD subject: got %q want %q", got, want)
	}

	// Stdout per-repo lines in commit order (source, target, alphabetical
	// siblings) — "specscore-cli" sorts after "specscore" alphabetically.
	wantLines := []string{
		"specstudio-skills: moved idea foo",
		"specscore: received idea foo",
		"specscore-cli: updated-links idea foo",
		"relocate complete: 3 repos affected",
	}
	for _, want := range wantLines {
		if !strings.Contains(stdout, want) {
			t.Errorf("stdout should contain %q; got:\n%s", want, stdout)
		}
	}

	// Verify the per-repo lines each end with a 7-char SHA in brackets.
	for _, prefix := range []string{"specstudio-skills:", "specscore:", "specscore-cli:"} {
		for _, line := range strings.Split(stdout, "\n") {
			if !strings.HasPrefix(line, prefix) {
				continue
			}
			// Expect "  [<sha7>]" suffix.
			if !strings.Contains(line, "  [") || !strings.HasSuffix(line, "]") {
				t.Errorf("expected SHA bracket on per-repo line; got: %q", line)
			}
		}
	}
}

// specscore:verifies feature/cli/idea/relocate#ac:no-commit-flag-stages-everywhere
func TestIdeaRelocateCLI_NoCommitFlagStagesEverywhere(t *testing.T) {
	parent := t.TempDir()
	source := stageRelocateRepo(t, parent, "specstudio-skills", "specstudio-skills")
	target := stageRelocateRepo(t, parent, "specscore", "specscore")
	sib := stageRelocateRepo(t, parent, "specscore-cli", "specscore-cli")
	writeIdeaFile(t, source, "foo")

	if err := os.MkdirAll(filepath.Join(sib, "spec", "features", "x"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(sib, "spec", "features", "x", "README.md"),
		[]byte("# Feature: X\n\nSee [the Idea](../../../specstudio-skills/spec/ideas/foo.md).\n"),
		0o644); err != nil {
		t.Fatalf("write ref: %v", err)
	}

	initGitRepoForTest(t, source)
	initGitRepoForTest(t, target)
	initGitRepoForTest(t, sib)

	// Snapshot HEAD SHAs before relocate to verify no new commits.
	headBefore := func(repo string) string {
		out, err := exec.Command("git", "-C", repo, "rev-parse", "HEAD").Output()
		if err != nil {
			t.Fatalf("rev-parse HEAD in %s: %v", repo, err)
		}
		return strings.TrimSpace(string(out))
	}
	srcHEAD, tgtHEAD, sibHEAD := headBefore(source), headBefore(target), headBefore(sib)

	stdout, _, err := runIdeaRelocateCLI(t, source, "foo", "--to-repo=specscore", "--no-commit")
	if err != nil {
		t.Fatalf("unexpected error: %v\nstdout:\n%s", err, stdout)
	}

	// No new commits.
	if got := headBefore(source); got != srcHEAD {
		t.Errorf("source HEAD advanced under --no-commit: %s → %s", srcHEAD, got)
	}
	if got := headBefore(target); got != tgtHEAD {
		t.Errorf("target HEAD advanced under --no-commit: %s → %s", tgtHEAD, got)
	}
	if got := headBefore(sib); got != sibHEAD {
		t.Errorf("sibling HEAD advanced under --no-commit: %s → %s", sibHEAD, got)
	}

	// Each repo has staged (index-set) changes — porcelain output has
	// non-space chars in column 1.
	checkStaged := func(repo, label string) {
		porcelain := gitWorkingTreeStatus(t, repo)
		if porcelain == "" {
			t.Errorf("%s has no working-tree changes; expected staged mutations", label)
			return
		}
		// Every non-empty line should have a non-space char in column 1
		// (status of the index entry).
		for _, line := range strings.Split(strings.TrimRight(porcelain, "\n"), "\n") {
			if len(line) < 2 {
				continue
			}
			if line[0] == ' ' || line[0] == '?' {
				t.Errorf("%s: expected staged status (col 1 ≠ space/?); got line %q in:\n%s",
					label, line, porcelain)
			}
		}
	}
	checkStaged(source, "source")
	checkStaged(target, "target")
	checkStaged(sib, "sibling")

	// Stdout has per-repo lines WITHOUT [<sha>] brackets.
	if !strings.Contains(stdout, "specstudio-skills: moved idea foo") {
		t.Errorf("stdout missing source line; got:\n%s", stdout)
	}
	if strings.Contains(stdout, "  [") {
		t.Errorf("stdout should NOT contain SHA brackets under --no-commit; got:\n%s", stdout)
	}
	if !strings.Contains(stdout, "relocate complete: 3 repos affected") {
		t.Errorf("stdout missing summary; got:\n%s", stdout)
	}
}

// specscore:verifies feature/cli/idea/relocate#ac:idea-relocate-happy-path
// specscore:verifies feature/cli/idea/relocate#ac:seed-relocate-happy-path
//
// End-to-end happy-path test (Task 7). Stages a three-repo workspace
// mirroring the AC's worked example, then runs two relocates back-to-
// back (Idea form first, then seed form), asserting exit 0, file
// movement, canonical commit subjects, and stdout-format compliance
// for both.
func TestIdeaRelocateCLI_HappyPathE2E(t *testing.T) {
	parent := t.TempDir()
	source := stageRelocateRepo(t, parent, "specstudio-skills", "specstudio-skills")
	target := stageRelocateRepo(t, parent, "specscore", "specscore")
	sib := stageRelocateRepo(t, parent, "specscore-cli", "specscore-cli")

	// Source artifacts: an Idea (foo) and a seed (bar), each with
	// substitution-worthy content so the in-file rewrite is exercised
	// alongside the cross-repo move.
	ideaBody := "# Idea: foo\n\nFrom synchestra-io/specscore in this repo.\n"
	if err := os.WriteFile(filepath.Join(source, "spec", "ideas", "foo.md"), []byte(ideaBody), 0o644); err != nil {
		t.Fatalf("write idea: %v", err)
	}
	seedBody := "# Seed: bar\n\nLegacy ref to synchestra-io/specscore-cli.\n"
	if err := os.WriteFile(filepath.Join(source, "spec", "ideas", "seeds", "bar.md"), []byte(seedBody), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}

	// Sibling has a cross-repo link to foo.md so Task-4 cleanup is
	// exercised in the Idea flow.
	if err := os.MkdirAll(filepath.Join(sib, "spec", "features", "x"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	sibRef := filepath.Join(sib, "spec", "features", "x", "README.md")
	if err := os.WriteFile(sibRef,
		[]byte("# Feature: X\n\nSee [the Idea](../../../specstudio-skills/spec/ideas/foo.md).\n"),
		0o644); err != nil {
		t.Fatalf("write sib ref: %v", err)
	}

	initGitRepoForTest(t, source)
	initGitRepoForTest(t, target)
	initGitRepoForTest(t, sib)

	// ----- Flow 1: Idea (foo) -----
	stdout, _, err := runIdeaRelocateCLI(t, source, "foo", "--to-repo=specscore")
	if err != nil {
		t.Fatalf("idea relocate: unexpected error: %v\nstdout:\n%s", err, stdout)
	}

	// Source artifact removed.
	if _, err := os.Stat(filepath.Join(source, "spec", "ideas", "foo.md")); !os.IsNotExist(err) {
		t.Errorf("source spec/ideas/foo.md should be deleted; stat err: %v", err)
	}
	// Target artifact present with org-rename + this-repo rewrites.
	tgtIdea, err := os.ReadFile(filepath.Join(target, "spec", "ideas", "foo.md"))
	if err != nil {
		t.Fatalf("target idea missing: %v", err)
	}
	if !strings.Contains(string(tgtIdea), "specscore/specscore") {
		t.Errorf("idea: expected org-rename applied; got:\n%s", tgtIdea)
	}
	if !strings.Contains(string(tgtIdea), "in specscore.") {
		t.Errorf("idea: expected 'this repo' rewritten to 'specscore'; got:\n%s", tgtIdea)
	}
	// Sibling link rewritten to GitHub URL.
	sibBody, err := os.ReadFile(sibRef)
	if err != nil {
		t.Fatalf("read sibling: %v", err)
	}
	if !strings.Contains(string(sibBody), "https://github.com/specscore/specscore/blob/main/spec/ideas/foo.md") {
		t.Errorf("sibling link not rewritten; got:\n%s", sibBody)
	}
	// Canonical commit subjects in each affected repo.
	if got, want := gitHEADSubject(t, source), "chore(relocate): move idea foo to specscore"; got != want {
		t.Errorf("source HEAD subject: got %q want %q", got, want)
	}
	if got, want := gitHEADSubject(t, target), "chore(relocate): receive idea foo from specstudio-skills"; got != want {
		t.Errorf("target HEAD subject: got %q want %q", got, want)
	}
	if got, want := gitHEADSubject(t, sib), "chore(relocate): update links for foo (specstudio-skills → specscore)"; got != want {
		t.Errorf("sibling HEAD subject: got %q want %q", got, want)
	}
	// Stdout format compliance: per-repo lines + summary.
	for _, want := range []string{
		"specstudio-skills: moved idea foo",
		"specscore: received idea foo",
		"specscore-cli: updated-links idea foo",
		"relocate complete: 3 repos affected",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("idea stdout missing %q; got:\n%s", want, stdout)
		}
	}

	// ----- Flow 2: seed (bar) -----
	stdout, _, err = runIdeaRelocateCLI(t, source, "bar", "--to-repo=specscore")
	if err != nil {
		t.Fatalf("seed relocate: unexpected error: %v\nstdout:\n%s", err, stdout)
	}

	if _, err := os.Stat(filepath.Join(source, "spec", "ideas", "seeds", "bar.md")); !os.IsNotExist(err) {
		t.Errorf("source spec/ideas/seeds/bar.md should be deleted; stat err: %v", err)
	}
	tgtSeed, err := os.ReadFile(filepath.Join(target, "spec", "ideas", "seeds", "bar.md"))
	if err != nil {
		t.Fatalf("target seed missing: %v", err)
	}
	if !strings.Contains(string(tgtSeed), "specscore/specscore-cli") {
		t.Errorf("seed: expected org-rename applied; got:\n%s", tgtSeed)
	}
	// Seed has no sibling links → no sibling commit, but source +
	// target both got the seed-kind subject lines.
	if got, want := gitHEADSubject(t, source), "chore(relocate): move seed bar to specscore"; got != want {
		t.Errorf("source HEAD subject (seed): got %q want %q", got, want)
	}
	if got, want := gitHEADSubject(t, target), "chore(relocate): receive seed bar from specstudio-skills"; got != want {
		t.Errorf("target HEAD subject (seed): got %q want %q", got, want)
	}
	for _, want := range []string{
		"specstudio-skills: moved seed bar",
		"specscore: received seed bar",
		"relocate complete: 2 repos affected",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("seed stdout missing %q; got:\n%s", want, stdout)
		}
	}
	// No sibling line for seed (no links matched).
	if strings.Contains(stdout, "specscore-cli: updated-links seed bar") {
		t.Errorf("seed stdout should NOT include sibling line (no matching links); got:\n%s", stdout)
	}
}

// specscore:verifies feature/cli/idea/relocate#ac:io-failure-rollback-pre-commit
func TestIdeaRelocateCLI_IOFailureRollbackPreCommit(t *testing.T) {
	parent := t.TempDir()
	source := stageRelocateRepo(t, parent, "specstudio-skills", "specstudio-skills")
	target := stageRelocateRepo(t, parent, "specscore", "specscore")
	writeIdeaFile(t, source, "foo")

	srcArtifact := filepath.Join(source, "spec", "ideas", "foo.md")
	srcBefore, err := os.ReadFile(srcArtifact)
	if err != nil {
		t.Fatalf("read source: %v", err)
	}

	// Make target's spec/ideas/ writable-by-nobody so WriteFile in
	// ApplyMutation fails. t.Cleanup restores perms so t.TempDir can
	// remove the tree.
	targetIdeas := filepath.Join(target, "spec", "ideas")
	if err := os.Chmod(targetIdeas, 0o555); err != nil {
		t.Fatalf("chmod 555: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(targetIdeas, 0o755) })

	_, _, relocateErr := runIdeaRelocateCLI(t, source, "foo", "--to-repo=specscore")
	if got := exitCodeFromErr(t, relocateErr); got != exitcode.Unexpected {
		t.Errorf("exit code: got %d want %d (IOFailure)", got, exitcode.Unexpected)
	}

	// Source artifact remains at original path with unchanged content.
	srcAfter, err := os.ReadFile(srcArtifact)
	if err != nil {
		t.Fatalf("source artifact missing after rollback: %v", err)
	}
	if string(srcAfter) != string(srcBefore) {
		t.Errorf("source artifact content changed by failed relocate; before=%q after=%q", srcBefore, srcAfter)
	}

	// No partial copy at target.
	tgtArtifact := filepath.Join(target, "spec", "ideas", "foo.md")
	if _, err := os.Stat(tgtArtifact); err == nil {
		t.Errorf("target should have no partial copy at %s", tgtArtifact)
	}

	// Stderr names the failed step.
	msg := relocateErr.Error()
	if !strings.Contains(msg, "pre-commit-phase I/O failure") {
		t.Errorf("stderr should name the failed step; got: %s", msg)
	}
}

// specscore:verifies feature/cli/idea/relocate#ac:commit-failure-mid-flight
func TestIdeaRelocateCLI_CommitFailureMidFlight(t *testing.T) {
	parent := t.TempDir()
	source := stageRelocateRepo(t, parent, "specstudio-skills", "specstudio-skills")
	target := stageRelocateRepo(t, parent, "specscore", "specscore")
	writeIdeaFile(t, source, "foo")
	initGitRepoForTest(t, source)
	initGitRepoForTest(t, target)

	// Install a failing pre-commit hook in the target repo.
	hookPath := filepath.Join(target, ".git", "hooks", "pre-commit")
	if err := os.WriteFile(hookPath, []byte("#!/bin/sh\necho 'reject' 1>&2\nexit 1\n"), 0o755); err != nil {
		t.Fatalf("write hook: %v", err)
	}

	_, _, err := runIdeaRelocateCLI(t, source, "foo", "--to-repo=specscore")
	if got := exitCodeFromErr(t, err); got != exitcode.Unexpected {
		t.Errorf("exit code: got %d want %d (Unexpected/CommitFailed)", got, exitcode.Unexpected)
	}

	// Source already committed; target failed; both reflected in stderr msg.
	msg := err.Error()
	if !strings.Contains(msg, "specstudio-skills") {
		t.Errorf("stderr should name committed source repo; got: %s", msg)
	}
	if !strings.Contains(msg, "specscore") {
		t.Errorf("stderr should name failing target repo; got: %s", msg)
	}
	if !strings.Contains(msg, "reject") {
		t.Errorf("stderr should include hook's failure output; got: %s", msg)
	}
	if !strings.Contains(msg, "reset HEAD~1 --hard") {
		t.Errorf("stderr should include rollback command for source; got: %s", msg)
	}
	if !strings.Contains(msg, "checkout -- .") {
		t.Errorf("stderr should include checkout rollback for target; got: %s", msg)
	}
}
