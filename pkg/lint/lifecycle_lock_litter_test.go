package lint

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func gitAddCommit(t *testing.T, root string, args ...string) {
	t.Helper()
	addArgs := append([]string{"add"}, args...)
	cmd := exec.Command("git", addArgs...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git add %v failed: %v\n%s", args, err, out)
	}
	cmd = exec.Command("git", "commit", "-m", "add lock file")
	cmd.Dir = root
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test", "GIT_AUTHOR_EMAIL=test@test.com",
		"GIT_COMMITTER_NAME=Test", "GIT_COMMITTER_EMAIL=test@test.com",
	)
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git commit failed: %v\n%s", err, out)
	}
}

// TestLifecycleLockLitter_CommittedFails reproduces the sneat-co/backstage
// incident: a lifecycle-transaction lock file left behind by
// pkg/lifecycle.TransformArtifact gets swept into a commit. Before the fix in
// this change, nothing in `spec lint` ever inspected these files, so this
// case reported zero violations.
func TestLifecycleLockLitter_CommittedFails(t *testing.T) {
	root := setupGitRepo(t, map[string]string{
		"spec/ideas/README.md": "# Ideas\n",
		"spec/ideas/thing.md":  "# Idea: Thing\n",
	})
	lockRel := filepath.Join("spec", "ideas", ".thing.md.lifecycle-transaction.lock")
	if err := os.WriteFile(filepath.Join(root, lockRel), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	gitAddCommit(t, root, lockRel)

	c := newLifecycleLockLitterChecker()
	vs, err := c.check(filepath.Join(root, "spec"))
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 1 {
		t.Fatalf("expected exactly 1 violation for a committed lock file, got %d: %+v", len(vs), vs)
	}
	if vs[0].Rule != "lifecycle-lock-committed" {
		t.Errorf("expected rule lifecycle-lock-committed, got %q", vs[0].Rule)
	}
	wantFile := filepath.ToSlash(filepath.Join("ideas", ".thing.md.lifecycle-transaction.lock"))
	if vs[0].File != wantFile {
		t.Errorf("expected file %q, got %q", wantFile, vs[0].File)
	}
}

// TestLifecycleLockLitter_UntrackedIsSilent proves the common, designed-for
// case — a lock file left on disk by a completed or in-flight transaction
// that was never staged/committed — produces no violation. Flagging every
// untracked lock file would make this rule fire on ordinary, harmless
// lifecycle-cli disk state.
func TestLifecycleLockLitter_UntrackedIsSilent(t *testing.T) {
	root := setupGitRepo(t, map[string]string{
		"spec/ideas/README.md": "# Ideas\n",
		"spec/ideas/thing.md":  "# Idea: Thing\n",
	})
	lockRel := filepath.Join("spec", "ideas", ".thing.md.lifecycle-transaction.lock")
	if err := os.WriteFile(filepath.Join(root, lockRel), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	// Deliberately not staged/committed.

	c := newLifecycleLockLitterChecker()
	vs, err := c.check(filepath.Join(root, "spec"))
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 0 {
		t.Fatalf("expected no violations for an untracked lock file, got %+v", vs)
	}
}

// TestLifecycleLockLitter_NoGitRepoIsSilent proves the checker no-ops rather
// than errors when specRoot isn't inside a git repository.
func TestLifecycleLockLitter_NoGitRepoIsSilent(t *testing.T) {
	root := t.TempDir()
	ideasDir := filepath.Join(root, "spec", "ideas")
	if err := os.MkdirAll(ideasDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(ideasDir, ".thing.md.lifecycle-transaction.lock"), nil, 0o644); err != nil {
		t.Fatal(err)
	}

	c := newLifecycleLockLitterChecker()
	vs, err := c.check(filepath.Join(root, "spec"))
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 0 {
		t.Fatalf("expected no violations outside a git repo, got %+v", vs)
	}
}

func TestLifecycleLockLitter_NameAndSeverity(t *testing.T) {
	c := newLifecycleLockLitterChecker()
	if got := c.name(); got != "lifecycle-lock-committed" {
		t.Errorf("name() = %q", got)
	}
	if got := c.severity(); got != "error" {
		t.Errorf("severity() = %q", got)
	}
}

// TestLifecycleLockLitter_GitFileTrackedHardError covers gitFileTracked's
// non-ExitError branch: a git invocation that fails to even run (as opposed
// to running and reporting "not tracked" via a non-zero exit) must surface,
// not be silently treated as untracked.
func TestLifecycleLockLitter_GitFileTrackedHardError(t *testing.T) {
	_, err := gitFileTracked(filepath.Join(t.TempDir(), "does-not-exist"), "foo.md")
	if err == nil {
		t.Fatal("expected an error when git cannot even start in a missing directory")
	}
}

// TestLifecycleLockLitter_ReadDirErrorPropagates covers both the checker's
// own os.ReadDir failure branch and check()'s walkErr-propagation branch in
// one shot: filepath.Walk aborts as soon as our callback returns an error.
func TestLifecycleLockLitter_ReadDirErrorPropagates(t *testing.T) {
	root := setupGitRepo(t, map[string]string{
		"spec/ideas/README.md": "# Ideas\n",
	})
	orig := lifecycleLockReadDir
	boom := errors.New("boom")
	lifecycleLockReadDir = func(dir string) ([]os.DirEntry, error) {
		if filepath.Base(dir) == "ideas" {
			return nil, boom
		}
		return orig(dir)
	}
	t.Cleanup(func() { lifecycleLockReadDir = orig })

	c := newLifecycleLockLitterChecker()
	_, err := c.check(filepath.Join(root, "spec"))
	if !errors.Is(err, boom) {
		t.Fatalf("expected the injected ReadDir error to propagate, got %v", err)
	}
}

// TestLifecycleLockLitter_EvalSymlinksFallback covers the canonFull=full
// fallback: when resolving symlinks fails, the checker still computes a
// correct repo-relative path and reports the violation instead of losing it.
func TestLifecycleLockLitter_EvalSymlinksFallback(t *testing.T) {
	root := setupGitRepo(t, map[string]string{
		"spec/ideas/README.md": "# Ideas\n",
		"spec/ideas/thing.md":  "# Idea: Thing\n",
	})
	// t.TempDir() resolves through a symlink on macOS (/var/folders ->
	// /private/var/folders); canonicalize before building expectations so
	// this test isolates the fallback statement instead of also exercising
	// the mismatched-prefix limitation documented above check()'s EvalSymlinks
	// call.
	if real, evalErr := filepath.EvalSymlinks(root); evalErr == nil {
		root = real
	}
	lockRel := filepath.Join("spec", "ideas", ".thing.md.lifecycle-transaction.lock")
	if err := os.WriteFile(filepath.Join(root, lockRel), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	gitAddCommit(t, root, lockRel)

	orig := lifecycleLockEvalSymlinks
	lifecycleLockEvalSymlinks = func(string) (string, error) { return "", errors.New("no symlink resolution") }
	t.Cleanup(func() { lifecycleLockEvalSymlinks = orig })

	c := newLifecycleLockLitterChecker()
	vs, err := c.check(filepath.Join(root, "spec"))
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 1 {
		t.Fatalf("expected the violation to still be reported via the fallback path, got %+v", vs)
	}
}

// TestLifecycleLockLitter_RelErrorSkipsEntry covers the relErr continue
// branch: an entry whose repo-relative path cannot be computed is skipped
// rather than failing the whole check.
func TestLifecycleLockLitter_RelErrorSkipsEntry(t *testing.T) {
	root := setupGitRepo(t, map[string]string{
		"spec/ideas/README.md": "# Ideas\n",
		"spec/ideas/thing.md":  "# Idea: Thing\n",
	})
	lockRel := filepath.Join("spec", "ideas", ".thing.md.lifecycle-transaction.lock")
	if err := os.WriteFile(filepath.Join(root, lockRel), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	gitAddCommit(t, root, lockRel)

	orig := lifecycleLockRel
	lifecycleLockRel = func(string, string) (string, error) { return "", errors.New("unrelated paths") }
	t.Cleanup(func() { lifecycleLockRel = orig })

	c := newLifecycleLockLitterChecker()
	vs, err := c.check(filepath.Join(root, "spec"))
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 0 {
		t.Fatalf("expected the entry to be skipped rather than reported, got %+v", vs)
	}
}

// TestLifecycleLockLitter_IsTrackedErrorPropagates proves a failure probing
// git tracking status (not just a "not tracked" result) fails the check
// rather than being swallowed as "not tracked".
func TestLifecycleLockLitter_IsTrackedErrorPropagates(t *testing.T) {
	root := setupGitRepo(t, map[string]string{
		"spec/ideas/README.md": "# Ideas\n",
		"spec/ideas/thing.md":  "# Idea: Thing\n",
	})
	lockRel := filepath.Join("spec", "ideas", ".thing.md.lifecycle-transaction.lock")
	if err := os.WriteFile(filepath.Join(root, lockRel), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	gitAddCommit(t, root, lockRel)

	boom := errors.New("git unavailable")
	c := &lifecycleLockLitterChecker{isTracked: func(string, string) (bool, error) { return false, boom }}
	_, err := c.check(filepath.Join(root, "spec"))
	if !errors.Is(err, boom) {
		t.Fatalf("expected the injected isTracked error to propagate, got %v", err)
	}
}

// TestLifecycleLockLitter_MultipleViolationsSorted proves multiple committed
// lock files are reported in deterministic, sorted order.
func TestLifecycleLockLitter_MultipleViolationsSorted(t *testing.T) {
	root := setupGitRepo(t, map[string]string{
		"spec/ideas/README.md": "# Ideas\n",
		"spec/ideas/zzz.md":    "# Idea: Zzz\n",
		"spec/ideas/aaa.md":    "# Idea: Aaa\n",
	})
	zLock := filepath.Join("spec", "ideas", ".zzz.md.lifecycle-transaction.lock")
	aLock := filepath.Join("spec", "ideas", ".aaa.md.lifecycle-transaction.lock")
	for _, rel := range []string{zLock, aLock} {
		if err := os.WriteFile(filepath.Join(root, rel), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	gitAddCommit(t, root, zLock, aLock)

	c := newLifecycleLockLitterChecker()
	vs, err := c.check(filepath.Join(root, "spec"))
	if err != nil {
		t.Fatal(err)
	}
	if len(vs) != 2 {
		t.Fatalf("expected 2 violations, got %+v", vs)
	}
	if vs[0].File >= vs[1].File {
		t.Errorf("expected sorted violations, got %q then %q", vs[0].File, vs[1].File)
	}
}
