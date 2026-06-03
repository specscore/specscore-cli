package cli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/specscore/specscore-cli/pkg/lint"
)

// withGetwd points the osGetwdFn seam at fn for the duration of the test.
func withGetwd(t *testing.T, fn func() (string, error)) {
	t.Helper()
	orig := osGetwdFn
	osGetwdFn = fn
	t.Cleanup(func() { osGetwdFn = orig })
}

func TestRunRules_CheckGetwdError(t *testing.T) {
	withGetwd(t, func() (string, error) { return "", errors.New("boom") })
	if _, _, err := runRulesCmd(t, "--check"); err == nil {
		t.Fatal("expected error when getwd fails under --check")
	}
}

func TestRunRules_WriteGetwdError(t *testing.T) {
	withGetwd(t, func() (string, error) { return "", errors.New("boom") })
	if _, _, err := runRulesCmd(t, "--write"); err == nil {
		t.Fatal("expected error when getwd fails under --write")
	}
}

func TestRunRules_CheckNoConfigRoot(t *testing.T) {
	dir := t.TempDir() // no specscore.yaml up the tree
	withGetwd(t, func() (string, error) { return dir, nil })
	if _, _, err := runRulesCmd(t, "--check"); err == nil {
		t.Fatal("expected findRepoConfigRoot error under --check with no config")
	}
}

func TestRunRules_WriteNoConfigRoot(t *testing.T) {
	dir := t.TempDir()
	withGetwd(t, func() (string, error) { return dir, nil })
	if _, _, err := runRulesCmd(t, "--write"); err == nil {
		t.Fatal("expected findRepoConfigRoot error under --write with no config")
	}
}

func TestRunRules_WriteCatalogError(t *testing.T) {
	dir := newTempRepo(t)
	// A regular file at the docs path makes WriteCatalog's MkdirAll fail.
	if err := os.WriteFile(filepath.Join(dir, "docs"), []byte("x"), 0o644); err != nil {
		t.Fatalf("seeding docs file: %v", err)
	}
	if _, _, err := runRulesCmd(t, "--write"); err == nil {
		t.Fatal("expected WriteCatalog error under --write when docs path is a file")
	}
}

func TestRunRules_CheckReadError(t *testing.T) {
	dir := newTempRepo(t)
	// A directory at the catalog path makes os.ReadFile return a non-NotExist error.
	if err := os.MkdirAll(filepath.Join(dir, lint.CatalogPath), 0o755); err != nil {
		t.Fatalf("seeding catalog dir: %v", err)
	}
	if _, _, err := runRulesCmd(t, "--check"); err == nil {
		t.Fatal("expected read error under --check when catalog path is a directory")
	}
}
