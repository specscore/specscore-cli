package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/config"
)

func gitOrSkip(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Skipf("git %v unavailable/failed (%v): %s", args, err, out)
	}
}

func TestRunInit_AddsLocalToGitignore(t *testing.T) {
	dir := t.TempDir()
	gitOrSkip(t, dir, "init")
	if _, _, err := runInitCmd(t, strings.NewReader(""), "--project", dir); err != nil {
		t.Fatalf("init: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	if !strings.Contains(string(data), "specscore.local.yaml") {
		t.Errorf(".gitignore missing specscore.local.yaml:\n%s", data)
	}
	if !strings.Contains(string(data), config.LifecycleTransactionLockIgnorePattern) {
		t.Errorf(".gitignore missing %s:\n%s", config.LifecycleTransactionLockIgnorePattern, data)
	}
	lockPath := filepath.Join("spec", "ideas", ".example.md.lifecycle-transaction.lock")
	cmd := exec.Command("git", "check-ignore", "--quiet", lockPath)
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("generated lifecycle lock is not ignored: %v: %s", err, out)
	}
}

func TestRunInit_WarnsWhenLocalTracked(t *testing.T) {
	dir := t.TempDir()
	gitOrSkip(t, dir, "init")
	if err := os.WriteFile(filepath.Join(dir, "specscore.local.yaml"), []byte("studio:\n  theme: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	gitOrSkip(t, dir, "add", "specscore.local.yaml")

	out, _, err := runInitCmd(t, strings.NewReader(""), "--project", dir)
	if err != nil {
		t.Fatalf("init: %v", err)
	}
	if !strings.Contains(out, "warning") || !strings.Contains(out, "tracked") {
		t.Errorf("expected tracked warning in init output:\n%s", out)
	}
}
