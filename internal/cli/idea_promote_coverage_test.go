package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/idea"
	"github.com/specscore/specscore-cli/pkg/ideapromote"
	"github.com/specscore/specscore-cli/pkg/idearelocate"
	"github.com/specscore/specscore-cli/pkg/lint"
)

// --- runIdeaPromote guard branches --------------------------------------

func TestIdeaPromote_InvalidSlug(t *testing.T) {
	parent := t.TempDir()
	source := stagePromoteRepo(t, parent, "src", "src")
	_, _, err := runIdeaPromoteCLI(t, source, "Bad Slug!!")
	if got := exitCodeFromErr(t, err); got != exitcode.InvalidArgs {
		t.Errorf("invalid slug: got %d want %d", got, exitcode.InvalidArgs)
	}
}

func TestIdeaPromote_InvalidFormat(t *testing.T) {
	parent := t.TempDir()
	source := stagePromoteRepo(t, parent, "src", "src")
	writePromoteSeed(t, source, "foo", "Foo", false)
	_, _, err := runIdeaPromoteCLI(t, source, "foo", "--format=xml")
	if got := exitCodeFromErr(t, err); got != exitcode.InvalidArgs {
		t.Errorf("invalid format: got %d want %d", got, exitcode.InvalidArgs)
	}
}

func TestIdeaPromote_ResolveSpecRootError(t *testing.T) {
	// A bare dir with no specscore.yaml anywhere up the tree → spec-root
	// resolution fails.
	dir := t.TempDir()
	_, _, err := runIdeaPromoteCLI(t, dir, "foo")
	if err == nil {
		t.Fatal("expected spec-root resolution error")
	}
}

func TestIdeaPromote_ScanReposError(t *testing.T) {
	parent := t.TempDir()
	source := stagePromoteRepo(t, parent, "src", "src")
	writePromoteSeed(t, source, "foo", "Foo", false)

	orig := idearelocateDiscoverSiblingsFn
	idearelocateDiscoverSiblingsFn = func(string) ([]idearelocate.TargetRepo, error) {
		return nil, errors.New("siblings boom")
	}
	t.Cleanup(func() { idearelocateDiscoverSiblingsFn = orig })

	_, _, err := runIdeaPromoteCLI(t, source, "foo")
	if err == nil || !strings.Contains(err.Error(), "discovering sibling repos") {
		t.Fatalf("expected sibling-discovery error; got %v", err)
	}
}

func TestIdeaPromote_DiscoverBackLinksError(t *testing.T) {
	parent := t.TempDir()
	source := stagePromoteRepo(t, parent, "src", "src")
	writePromoteSeed(t, source, "foo", "Foo", false)

	orig := ideapromoteDiscoverBackLinksFn
	ideapromoteDiscoverBackLinksFn = func([]ideapromote.RepoRef, string) ([]ideapromote.BackLink, error) {
		return nil, errors.New("discover boom")
	}
	t.Cleanup(func() { ideapromoteDiscoverBackLinksFn = orig })

	_, _, err := runIdeaPromoteCLI(t, source, "foo")
	if err == nil {
		t.Fatal("expected DiscoverBackLinks error")
	}
}

func TestIdeaPromote_ReadSeedError(t *testing.T) {
	parent := t.TempDir()
	source := stagePromoteRepo(t, parent, "src", "src")
	writePromoteSeed(t, source, "foo", "Foo", false)
	// No git init: preflight is vacuously clean for a non-git workspace, so
	// the flow reaches os.ReadFile, which then fails on the unreadable seed.

	seed := filepath.Join(source, "spec", "ideas", "seeds", "foo.md")
	if err := os.Chmod(seed, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(seed, 0o644) })

	_, _, err := runIdeaPromoteCLI(t, source, "foo")
	if err == nil {
		t.Skip("filesystem allowed reading a 0-perm file (running as root?)")
	}
	if !strings.Contains(err.Error(), "reading seed") {
		t.Fatalf("expected read-seed error; got %v", err)
	}
}

func TestIdeaPromote_TransformError(t *testing.T) {
	parent := t.TempDir()
	source := stagePromoteRepo(t, parent, "src", "src")
	writePromoteSeed(t, source, "foo", "Foo", false)
	initGitRepoForTest(t, source)

	orig := ideapromoteTransformFn
	ideapromoteTransformFn = func(ideapromote.SeedContent, ideapromote.TransformOptions) ([]byte, error) {
		return nil, errors.New("transform boom")
	}
	t.Cleanup(func() { ideapromoteTransformFn = orig })

	_, _, err := runIdeaPromoteCLI(t, source, "foo")
	if err == nil || !strings.Contains(err.Error(), "transforming seed") {
		t.Fatalf("expected transform error; got %v", err)
	}
}

func TestIdeaPromote_SameRepoPromoteError(t *testing.T) {
	parent := t.TempDir()
	source := stagePromoteRepo(t, parent, "src", "src")
	writePromoteSeed(t, source, "foo", "Foo", false)
	initGitRepoForTest(t, source)

	orig := ideapromoteSameRepoPromoteFn
	ideapromoteSameRepoPromoteFn = func(string, string, []byte) (string, error) {
		return "", errors.New("samerepo boom")
	}
	t.Cleanup(func() { ideapromoteSameRepoPromoteFn = orig })

	_, _, err := runIdeaPromoteCLI(t, source, "foo")
	if err == nil {
		t.Fatal("expected SameRepoPromote error")
	}
}

func TestIdeaPromote_ReconcileError(t *testing.T) {
	parent := t.TempDir()
	source := stagePromoteRepo(t, parent, "src", "src")
	writePromoteSeed(t, source, "foo", "Foo", false)
	initGitRepoForTest(t, source)

	orig := ideapromoteReconcileFn
	ideapromoteReconcileFn = func([]ideapromote.BackLink, string) ([]ideapromote.ReconcileResult, error) {
		return nil, errors.New("reconcile boom")
	}
	t.Cleanup(func() { ideapromoteReconcileFn = orig })

	_, _, err := runIdeaPromoteCLI(t, source, "foo")
	if err == nil {
		t.Fatal("expected Reconcile error")
	}
}

func TestIdeaPromote_CrossRepoPromoteError(t *testing.T) {
	parent := t.TempDir()
	source := stagePromoteRepo(t, parent, "src", "src")
	stagePromoteRepo(t, parent, "other-repo", "other-repo")
	writePromoteSeed(t, source, "baz", "Baz", false)

	// A cross-repo back-link so HasCrossRepo selects the cross-repo path.
	featDir := filepath.Join(source, "spec", "features", "x")
	if err := os.MkdirAll(featDir, 0o755); err != nil {
		t.Fatal(err)
	}
	featBody := "# Feature: X\n\n**Status:** Approved\n\n## Summary\n\nT.\n\n" +
		"## Sidekick Seeds Generated\n\n" +
		"- [baz](other-repo:spec/ideas/seeds/baz.md) — captured 2026-06-01\n\n" +
		"## Open Questions\n\nNone at this time.\n\n" +
		"---\n*This document follows the https://specscore.md/feature-specification*\n"
	if err := os.WriteFile(filepath.Join(featDir, "README.md"), []byte(featBody), 0o644); err != nil {
		t.Fatal(err)
	}
	initGitRepoForTest(t, source)

	orig := ideapromoteCrossRepoPromoteFn
	ideapromoteCrossRepoPromoteFn = func(string, string, []byte, string) (string, string, error) {
		return "", "", errors.New("crossrepo boom")
	}
	t.Cleanup(func() { ideapromoteCrossRepoPromoteFn = orig })

	_, _, err := runIdeaPromoteCLI(t, source, "baz")
	if err == nil {
		t.Fatal("expected CrossRepoPromote error")
	}
}

func TestIdeaPromote_LintFixError(t *testing.T) {
	parent := t.TempDir()
	source := stagePromoteRepo(t, parent, "src", "src")
	writePromoteSeed(t, source, "foo", "Foo", false)
	initGitRepoForTest(t, source)

	orig := lintLintFn
	lintLintFn = func(opts lint.Options) ([]lint.Violation, error) {
		if opts.Fix {
			return nil, errors.New("lint --fix boom")
		}
		return nil, nil
	}
	t.Cleanup(func() { lintLintFn = orig })

	_, _, err := runIdeaPromoteCLI(t, source, "foo")
	if err == nil || !strings.Contains(err.Error(), "running lint --fix") {
		t.Fatalf("expected lint --fix error; got %v", err)
	}
}

func TestIdeaPromote_LintCheckError(t *testing.T) {
	parent := t.TempDir()
	source := stagePromoteRepo(t, parent, "src", "src")
	writePromoteSeed(t, source, "foo", "Foo", false)
	initGitRepoForTest(t, source)

	orig := lintLintFn
	lintLintFn = func(opts lint.Options) ([]lint.Violation, error) {
		if opts.Fix {
			return nil, nil // fix succeeds
		}
		return nil, errors.New("lint check boom") // verification fails
	}
	t.Cleanup(func() { lintLintFn = orig })

	_, _, err := runIdeaPromoteCLI(t, source, "foo")
	if err == nil || !strings.Contains(err.Error(), "running lint") {
		t.Fatalf("expected lint check error; got %v", err)
	}
}

func TestIdeaPromote_LintViolationsReported(t *testing.T) {
	parent := t.TempDir()
	source := stagePromoteRepo(t, parent, "src", "src")
	writePromoteSeed(t, source, "foo", "Foo", false)
	initGitRepoForTest(t, source)

	orig := lintLintFn
	lintLintFn = func(opts lint.Options) ([]lint.Violation, error) {
		if opts.Fix {
			return nil, nil
		}
		// Return an error-severity violation under ideas/ → promote fails.
		return []lint.Violation{
			{File: "ideas/foo.md", Line: 3, Rule: "X1", Message: "bad", Severity: "error"},
			{File: "features/y/README.md", Line: 1, Rule: "X2", Message: "other", Severity: "error"},
			{File: "ideas/foo.md", Line: 4, Rule: "X3", Message: "warn", Severity: "warning"},
		}, nil
	}
	t.Cleanup(func() { lintLintFn = orig })

	_, _, err := runIdeaPromoteCLI(t, source, "foo")
	if err == nil || !strings.Contains(err.Error(), "promoted Idea failed lint") {
		t.Fatalf("expected lint-violation error; got %v", err)
	}
}

func TestIdeaPromote_YAMLFormat(t *testing.T) {
	parent := t.TempDir()
	source := stagePromoteRepo(t, parent, "src", "src")
	writePromoteSeed(t, source, "foo", "Foo Idea", false)
	initGitRepoForTest(t, source)

	stdout, _, err := runIdeaPromoteCLI(t, source, "foo", "--format=yaml")
	if err != nil {
		t.Fatalf("yaml promote: %v", err)
	}
	if !strings.Contains(stdout, "slug: foo") || !strings.Contains(stdout, "seed_fate: moved") {
		t.Errorf("yaml output missing expected fields; got:\n%s", stdout)
	}
}

func TestIdeaPromote_YAMLEncodeError(t *testing.T) {
	parent := t.TempDir()
	source := stagePromoteRepo(t, parent, "src", "src")
	writePromoteSeed(t, source, "foo", "Foo Idea", false)
	initGitRepoForTest(t, source)

	swapYAML(t, errors.New("yaml encode boom"))

	_, _, err := runIdeaPromoteCLI(t, source, "foo", "--format=yaml")
	if err == nil || !strings.Contains(err.Error(), "encoding yaml") {
		t.Fatalf("expected yaml encode error; got %v", err)
	}
}

// --- promoteScanRepos canon fallback branches ----------------------------

func TestPromoteScanRepos_EvalSymlinksFallback(t *testing.T) {
	parent := t.TempDir()
	source := stagePromoteRepo(t, parent, "src", "src")

	orig := idearelocateDiscoverSiblingsFn
	// Sibling path that does not exist on disk → EvalSymlinks fails →
	// canon falls back to filepath.Clean(abs).
	idearelocateDiscoverSiblingsFn = func(string) ([]idearelocate.TargetRepo, error) {
		return []idearelocate.TargetRepo{{Path: filepath.Join(parent, "ghost-repo")}}, nil
	}
	t.Cleanup(func() { idearelocateDiscoverSiblingsFn = orig })

	repos, err := promoteScanRepos(source)
	if err != nil {
		t.Fatalf("promoteScanRepos: %v", err)
	}
	if len(repos) != 2 {
		t.Errorf("expected source + ghost sibling; got %d", len(repos))
	}
}

func TestPromoteScanRepos_AbsFallback(t *testing.T) {
	parent := t.TempDir()
	source := stagePromoteRepo(t, parent, "src", "src")

	origAbs := filepathAbsFn
	filepathAbsFn = func(string) (string, error) { return "", errors.New("abs boom") }
	t.Cleanup(func() { filepathAbsFn = origAbs })

	// With Abs failing, canon falls back to filepath.Clean(p) for every path.
	repos, err := promoteScanRepos(source)
	if err != nil {
		t.Fatalf("promoteScanRepos: %v", err)
	}
	if len(repos) == 0 {
		t.Error("expected at least the source repo")
	}
}

// --- validateIdeaSkeleton branches ---------------------------------------

func TestValidateIdeaSkeleton_ParseError(t *testing.T) {
	if err := validateIdeaSkeleton(filepath.Join(t.TempDir(), "missing.md")); err == nil {
		t.Fatal("expected parse error for missing file")
	}
}

func TestValidateIdeaSkeleton_MissingFields(t *testing.T) {
	dir := t.TempDir()
	// A file that parses but is not a valid Idea skeleton (wrong title, no
	// fields/sections) → reports missing pieces.
	p := filepath.Join(dir, "bad.md")
	if err := os.WriteFile(p, []byte("# Feature: Not An Idea\n\nbody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	err := validateIdeaSkeleton(p)
	if err == nil || !strings.Contains(err.Error(), "not a lint-clean skeleton") {
		t.Fatalf("expected skeleton-missing error; got %v", err)
	}
}

func TestValidateIdeaSkeleton_Valid(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "ok.md")
	body, err := idea.Scaffold(idea.ScaffoldOptions{Slug: "ok", Title: "OK Idea"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, body, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateIdeaSkeleton(p); err != nil {
		t.Errorf("valid skeleton should pass: %v", err)
	}
}
