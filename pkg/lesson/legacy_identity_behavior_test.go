package lesson

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveLegacySourceIdentityRejectsWorkingTreeDivergence(t *testing.T) {
	repo := t.TempDir()
	source := filepath.Join(repo, "history", "LESSONS-LEARNED.md")
	if err := os.MkdirAll(filepath.Dir(source), 0o755); err != nil {
		t.Fatal(err)
	}
	committed := []byte("## L1 — preserve durable provenance\n")
	if err := os.WriteFile(source, committed, 0o644); err != nil {
		t.Fatal(err)
	}
	run := func(args ...string) string {
		t.Helper()
		out, err := exec.Command("git", append([]string{"-C", repo}, args...)...).CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v: %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	run("init")
	run("config", "user.email", "test@example.test")
	run("config", "user.name", "Test")
	run("remote", "add", "origin", "https://github.com/example/legacy.git")
	run("add", "history/LESSONS-LEARNED.md")
	run("commit", "-m", "legacy")
	physical, err := filepath.EvalSymlinks(source)
	if err != nil {
		t.Fatal(err)
	}
	ref, err := resolveLegacySourceIdentity(physical, committed)
	if err != nil || ref.Repository != "github.com/example/legacy" || ref.Path != "history/LESSONS-LEARNED.md" {
		t.Fatalf("ref=%#v err=%v", ref, err)
	}
	if err := os.WriteFile(source, []byte("changed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := resolveLegacySourceIdentity(physical, []byte("changed\n")); err == nil || !strings.Contains(err.Error(), "bytes do not match") {
		t.Fatalf("err=%v", err)
	}
}
