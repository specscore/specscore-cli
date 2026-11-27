package idearelocate

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// stageRepo creates a repo dir at <parent>/<name> with the minimum
// SpecScore structure: a specscore.yaml with project.org=project.repo=
// <repoSlug>, and an empty spec/ tree. Returns the repo's absolute
// path. Setting org by default lets Task-4 link-cleanup tests share
// the helper.
func stageRepo(t *testing.T, parent, name, repoSlug string) string {
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

// writeIdea writes spec/ideas/<slug>.md inside repoRoot.
func writeIdea(t *testing.T, repoRoot, slug, body string) {
	t.Helper()
	path := filepath.Join(repoRoot, "spec", "ideas", slug+".md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write idea: %v", err)
	}
}

// writeSeed writes spec/ideas/seeds/<slug>.md inside repoRoot.
func writeSeed(t *testing.T, repoRoot, slug, body string) {
	t.Helper()
	path := filepath.Join(repoRoot, "spec", "ideas", "seeds", slug+".md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write seed: %v", err)
	}
}

func exitCodeOf(t *testing.T, err error) int {
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

// -------- ResolveSourceArtifact --------

func TestResolveSourceArtifact_IdeaFirst(t *testing.T) {
	parent := t.TempDir()
	source := stageRepo(t, parent, "src", "src")
	writeIdea(t, source, "foo", "# Idea: Foo\n")

	got, err := ResolveSourceArtifact(source, "foo")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Kind != KindIdea {
		t.Errorf("kind: got %q want %q", got.Kind, KindIdea)
	}
	wantPath := filepath.Join(source, "spec", "ideas", "foo.md")
	if got.Path != wantPath {
		t.Errorf("path: got %q want %q", got.Path, wantPath)
	}
}

func TestResolveSourceArtifact_SeedFallback(t *testing.T) {
	parent := t.TempDir()
	source := stageRepo(t, parent, "src", "src")
	writeSeed(t, source, "bar", "# Seed bar\n")

	got, err := ResolveSourceArtifact(source, "bar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Kind != KindSeed {
		t.Errorf("kind: got %q want %q", got.Kind, KindSeed)
	}
	wantPath := filepath.Join(source, "spec", "ideas", "seeds", "bar.md")
	if got.Path != wantPath {
		t.Errorf("path: got %q want %q", got.Path, wantPath)
	}
}

// specscore:verifies feature/cli/idea/relocate#ac:ambiguous-slug-rejected
func TestResolveSourceArtifact_AmbiguousRejected(t *testing.T) {
	parent := t.TempDir()
	source := stageRepo(t, parent, "src", "src")
	writeIdea(t, source, "foo", "")
	writeSeed(t, source, "foo", "")

	_, err := ResolveSourceArtifact(source, "foo")
	if got := exitCodeOf(t, err); got != exitcode.AmbiguousSlug {
		t.Errorf("exit code: got %d want %d (AmbiguousSlug)", got, exitcode.AmbiguousSlug)
	}
	if !strings.Contains(err.Error(), "foo") {
		t.Errorf("error should name the offending slug: %v", err)
	}
}

// specscore:verifies feature/cli/idea/relocate#ac:slug-not-found
func TestResolveSourceArtifact_NotFound(t *testing.T) {
	parent := t.TempDir()
	source := stageRepo(t, parent, "src", "src")

	_, err := ResolveSourceArtifact(source, "nonexistent")
	if got := exitCodeOf(t, err); got != exitcode.NotFound {
		t.Errorf("exit code: got %d want %d (NotFound)", got, exitcode.NotFound)
	}
}

// -------- ResolveTargetRepo (slug form) --------

// specscore:verifies feature/cli/idea/relocate#ac:to-repo-slug-form-resolves-via-sibling-scan
func TestResolveTargetRepo_SlugForm_ResolvesViaScan(t *testing.T) {
	parent := t.TempDir()
	source := stageRepo(t, parent, "specstudio-skills", "specstudio-skills")
	target := stageRepo(t, parent, "specscore", "specscore")
	stageRepo(t, parent, "specscore-cli", "specscore-cli") // bystander

	got, err := ResolveTargetRepo(source, "specscore")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Path != target {
		t.Errorf("path: got %q want %q", got.Path, target)
	}
	if got.RepoName != "specscore" {
		t.Errorf("repo name: got %q want %q", got.RepoName, "specscore")
	}
}

// specscore:verifies feature/cli/idea/relocate#ac:to-repo-slug-multiple-matches-rejected
func TestResolveTargetRepo_SlugForm_MultipleMatchesRejected(t *testing.T) {
	parent := t.TempDir()
	source := stageRepo(t, parent, "specstudio-skills", "specstudio-skills")
	stageRepo(t, parent, "specscore-a", "specscore") // duplicate project.repo
	stageRepo(t, parent, "specscore-b", "specscore")

	_, err := ResolveTargetRepo(source, "specscore")
	if got := exitCodeOf(t, err); got != exitcode.InvalidArgs {
		t.Errorf("exit code: got %d want %d (InvalidArgs)", got, exitcode.InvalidArgs)
	}
	if !strings.Contains(err.Error(), "specscore-a") || !strings.Contains(err.Error(), "specscore-b") {
		t.Errorf("error should name each matching repo, got: %v", err)
	}
}

func TestResolveTargetRepo_SlugForm_NoMatchesIsNotFound(t *testing.T) {
	parent := t.TempDir()
	source := stageRepo(t, parent, "specstudio-skills", "specstudio-skills")
	stageRepo(t, parent, "specscore", "specscore")

	_, err := ResolveTargetRepo(source, "nonexistent-repo")
	if got := exitCodeOf(t, err); got != exitcode.NotFound {
		t.Errorf("exit code: got %d want %d (NotFound)", got, exitcode.NotFound)
	}
}

func TestResolveTargetRepo_SlugForm_SkipsHiddenSiblings(t *testing.T) {
	parent := t.TempDir()
	source := stageRepo(t, parent, "src", "src")
	stageRepo(t, parent, ".hidden", "specscore") // hidden; should be skipped
	stageRepo(t, parent, "visible", "specscore") // single visible match wins

	got, err := ResolveTargetRepo(source, "specscore")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if filepath.Base(got.Path) != "visible" {
		t.Errorf("expected to resolve to 'visible', got %q", got.Path)
	}
}

// -------- ResolveTargetRepo (path form) --------

// specscore:verifies feature/cli/idea/relocate#ac:to-repo-path-form-bypasses-scan
func TestResolveTargetRepo_PathForm_Relative(t *testing.T) {
	parent := t.TempDir()
	source := stageRepo(t, parent, "src", "src")
	target := stageRepo(t, parent, "tgt", "tgt-different-name") // slug mismatch on purpose

	got, err := ResolveTargetRepo(source, "../tgt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Resolve both to absolute via EvalSymlinks on macOS to handle /private/ prefix.
	wantAbs, _ := filepath.EvalSymlinks(target)
	gotAbs, _ := filepath.EvalSymlinks(got.Path)
	if gotAbs != wantAbs {
		t.Errorf("path: got %q want %q", gotAbs, wantAbs)
	}
	// Path-form does NOT use sibling-scan slug matching; the path was
	// honored verbatim regardless of the target's project.repo value.
	if got.RepoName != "tgt-different-name" {
		t.Errorf("repo name (read from target's specscore.yaml): got %q want %q",
			got.RepoName, "tgt-different-name")
	}
}

func TestResolveTargetRepo_PathForm_Absolute(t *testing.T) {
	parent := t.TempDir()
	source := stageRepo(t, parent, "src", "src")
	target := stageRepo(t, parent, "tgt", "tgt")

	got, err := ResolveTargetRepo(source, target) // absolute path
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	wantAbs, _ := filepath.EvalSymlinks(target)
	gotAbs, _ := filepath.EvalSymlinks(got.Path)
	if gotAbs != wantAbs {
		t.Errorf("path: got %q want %q", gotAbs, wantAbs)
	}
}

// specscore:verifies feature/cli/idea/relocate#ac:to-repo-without-specscore-yaml-rejected
func TestResolveTargetRepo_PathForm_MissingYamlRejected(t *testing.T) {
	parent := t.TempDir()
	source := stageRepo(t, parent, "src", "src")
	bareDir := filepath.Join(parent, "bare")
	if err := os.MkdirAll(bareDir, 0o755); err != nil {
		t.Fatalf("mkdir bare: %v", err)
	}

	_, err := ResolveTargetRepo(source, "../bare")
	if got := exitCodeOf(t, err); got != exitcode.TargetNotSpecScore {
		t.Errorf("exit code: got %d want %d (TargetNotSpecScore)", got, exitcode.TargetNotSpecScore)
	}
}

func TestResolveTargetRepo_PathForm_NonexistentDirIsNotFound(t *testing.T) {
	parent := t.TempDir()
	source := stageRepo(t, parent, "src", "src")

	_, err := ResolveTargetRepo(source, "../does-not-exist")
	if got := exitCodeOf(t, err); got != exitcode.NotFound {
		t.Errorf("exit code: got %d want %d (NotFound)", got, exitcode.NotFound)
	}
}

func TestResolveTargetRepo_EmptyValueRejected(t *testing.T) {
	parent := t.TempDir()
	source := stageRepo(t, parent, "src", "src")

	_, err := ResolveTargetRepo(source, "")
	if got := exitCodeOf(t, err); got != exitcode.InvalidArgs {
		t.Errorf("exit code: got %d want %d (InvalidArgs)", got, exitcode.InvalidArgs)
	}
}
