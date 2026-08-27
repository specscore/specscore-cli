package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readGitignore(t *testing.T, repo string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(repo, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestEnsureLocalGitignored_AddsWhenMissing(t *testing.T) {
	repo := t.TempDir() // not a git repo -> real git reports untracked
	added, warning, err := EnsureLocalGitignored(repo)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !added {
		t.Error("added = false, want true")
	}
	if warning != "" {
		t.Errorf("warning = %q, want empty", warning)
	}
	if !strings.Contains(readGitignore(t, repo), LocalFile) {
		t.Errorf(".gitignore missing %s", LocalFile)
	}
	if !strings.Contains(readGitignore(t, repo), LifecycleTransactionLockIgnorePattern) {
		t.Errorf(".gitignore missing %s", LifecycleTransactionLockIgnorePattern)
	}
	if !strings.Contains(readGitignore(t, repo), LifecycleProjectLockIgnorePattern) {
		t.Errorf(".gitignore missing %s", LifecycleProjectLockIgnorePattern)
	}
}

func TestEnsureLocalGitignored_AlreadyPresent(t *testing.T) {
	repo := t.TempDir()
	writeLayer(t, filepath.Join(repo, ".gitignore"), "node_modules\n"+LocalFile+"\n"+LifecycleTransactionLockIgnorePattern+"\n"+LifecycleProjectLockIgnorePattern+"\n")
	added, _, err := EnsureLocalGitignored(repo)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if added {
		t.Error("added = true, want false (already present)")
	}
}

func TestEnsureLocalGitignored_AppendsNoTrailingNewline(t *testing.T) {
	repo := t.TempDir()
	writeLayer(t, filepath.Join(repo, ".gitignore"), "node_modules")
	added, _, err := EnsureLocalGitignored(repo)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !added {
		t.Error("added = false")
	}
	if got := readGitignore(t, repo); got != "node_modules\n"+LocalFile+"\n"+LifecycleTransactionLockIgnorePattern+"\n"+LifecycleProjectLockIgnorePattern+"\n" {
		t.Errorf("content = %q", got)
	}
}

func TestEnsureLocalGitignored_AppendsWithTrailingNewline(t *testing.T) {
	repo := t.TempDir()
	writeLayer(t, filepath.Join(repo, ".gitignore"), "node_modules\n")
	if _, _, err := EnsureLocalGitignored(repo); err != nil {
		t.Fatalf("err: %v", err)
	}
	if got := readGitignore(t, repo); got != "node_modules\n"+LocalFile+"\n"+LifecycleTransactionLockIgnorePattern+"\n"+LifecycleProjectLockIgnorePattern+"\n" {
		t.Errorf("content = %q", got)
	}
}

func TestEnsureLocalGitignored_AddsOnlyMissingEntry(t *testing.T) {
	repo := t.TempDir()
	writeLayer(t, filepath.Join(repo, ".gitignore"), LocalFile+"\n")
	if _, _, err := EnsureLocalGitignored(repo); err != nil {
		t.Fatalf("err: %v", err)
	}
	if got := readGitignore(t, repo); got != LocalFile+"\n"+LifecycleTransactionLockIgnorePattern+"\n"+LifecycleProjectLockIgnorePattern+"\n" {
		t.Errorf("content = %q", got)
	}
}

func TestEnsureLocalGitignored_TrackedWarns(t *testing.T) {
	repo := t.TempDir()
	orig := runGitFn
	runGitFn = func(string, ...string) error { return nil } // simulate tracked
	defer func() { runGitFn = orig }()

	_, warning, err := EnsureLocalGitignored(repo)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if warning == "" {
		t.Error("warning empty, want tracked warning")
	}
}

func TestEnsureLocalGitignored_GitignoreIsDirError(t *testing.T) {
	repo := t.TempDir()
	if err := os.Mkdir(filepath.Join(repo, ".gitignore"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := EnsureLocalGitignored(repo); err == nil {
		t.Fatal("expected error reading a directory as .gitignore")
	}
}

func TestEnsureLocalGitignoredMsg_CleanIsEmpty(t *testing.T) {
	repo := t.TempDir() // no git repo -> untracked, clean add
	if msg := EnsureLocalGitignoredMsg(repo); msg != "" {
		t.Errorf("msg = %q, want empty on a clean add", msg)
	}
}

func TestEnsureLocalGitignoredMsg_TrackedWarning(t *testing.T) {
	repo := t.TempDir()
	orig := runGitFn
	runGitFn = func(string, ...string) error { return nil } // tracked
	defer func() { runGitFn = orig }()

	if msg := EnsureLocalGitignoredMsg(repo); !strings.Contains(msg, "tracked") {
		t.Errorf("msg = %q, want a tracked warning", msg)
	}
}

func TestEnsureLocalGitignoredMsg_ErrorReported(t *testing.T) {
	repo := t.TempDir()
	orig := writeFileFn
	writeFileFn = func(string, []byte, os.FileMode) error { return errors.New("boom") }
	defer func() { writeFileFn = orig }()

	if msg := EnsureLocalGitignoredMsg(repo); !strings.Contains(msg, ".gitignore") {
		t.Errorf("msg = %q, want a .gitignore failure message", msg)
	}
}

func TestEnsureLocalGitignored_WriteError(t *testing.T) {
	repo := t.TempDir()
	orig := writeFileFn
	writeFileFn = func(string, []byte, os.FileMode) error { return errors.New("boom") }
	defer func() { writeFileFn = orig }()

	if _, _, err := EnsureLocalGitignored(repo); err == nil {
		t.Fatal("expected write error to surface")
	}
}
