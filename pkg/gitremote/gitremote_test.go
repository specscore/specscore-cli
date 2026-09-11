package gitremote

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// runGit is a tiny helper for spinning up a real git repo inside t.TempDir().
// Tests skip themselves cleanly when git is unavailable on the host.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	// Quiet the porcelain — we only care about exit code / final state.
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
}

// TestHeadSHA initialises a real git repo, makes one commit, and asserts
// HeadSHA returns the matching 40-char hex SHA.
func TestHeadSHA(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available on PATH")
	}
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")
	runGit(t, dir, "config", "user.email", "t@example.com")
	runGit(t, dir, "config", "user.name", "T")
	runGit(t, dir, "commit", "--allow-empty", "-q", "-m", "initial")

	got, err := HeadSHA(dir)
	if err != nil {
		t.Fatalf("HeadSHA returned error: %v", err)
	}
	// Cross-check against `git rev-parse HEAD` directly.
	cmd := exec.Command("git", "-C", dir, "rev-parse", "HEAD")
	expected, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse HEAD failed: %v", err)
	}
	want := strings.TrimSpace(string(expected))
	if got != want {
		t.Errorf("HeadSHA = %q; want %q", got, want)
	}
	if len(got) != 40 {
		t.Errorf("HeadSHA length = %d; want 40 hex chars (got %q)", len(got), got)
	}
}

// TestHeadSHA_NoGitRepo asserts HeadSHA returns an error when invoked
// against a directory that is not a git repo. The error path is what
// the auto-fill caller uses to substitute the literal "uncommitted".
func TestHeadSHA_NoGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available on PATH")
	}
	dir := t.TempDir()
	if _, err := HeadSHA(dir); err == nil {
		t.Fatal("HeadSHA on non-git dir returned nil error; want error")
	}
}

// TestCurrentBranch initialises a real git repo on a named branch and
// asserts CurrentBranch reports that name.
func TestCurrentBranch(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available on PATH")
	}
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "feature/plan-coordination-branch")
	runGit(t, dir, "config", "user.email", "t@example.com")
	runGit(t, dir, "config", "user.name", "T")
	runGit(t, dir, "commit", "--allow-empty", "-q", "-m", "initial")

	got, err := CurrentBranch(dir)
	if err != nil {
		t.Fatalf("CurrentBranch returned error: %v", err)
	}
	if got != "feature/plan-coordination-branch" {
		t.Errorf("CurrentBranch = %q; want %q", got, "feature/plan-coordination-branch")
	}
}

// TestCurrentBranch_DetachedHead asserts CurrentBranch errors rather than
// guessing a name when HEAD is detached.
func TestCurrentBranch_DetachedHead(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available on PATH")
	}
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")
	runGit(t, dir, "config", "user.email", "t@example.com")
	runGit(t, dir, "config", "user.name", "T")
	runGit(t, dir, "commit", "--allow-empty", "-q", "-m", "initial")
	runGit(t, dir, "checkout", "-q", "--detach", "HEAD")

	if _, err := CurrentBranch(dir); err == nil {
		t.Fatal("CurrentBranch on detached HEAD returned nil error; want error")
	}
}

// TestCurrentBranch_NoGitRepo asserts CurrentBranch errors against a
// directory that is not a git repo.
func TestCurrentBranch_NoGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available on PATH")
	}
	dir := t.TempDir()
	if _, err := CurrentBranch(dir); err == nil {
		t.Fatal("CurrentBranch on non-git dir returned nil error; want error")
	}
}

func TestParse(t *testing.T) {
	tests := []struct {
		in        string
		wantOK    bool
		wantOwner string
		wantRepo  string
		wantHost  string
	}{
		{"https://github.com/specscore/specscore-cli.git", true, "specscore", "specscore-cli", "github.com"},
		{"https://github.com/specscore/specscore-cli", true, "specscore", "specscore-cli", "github.com"},
		{"http://github.com/o/r.git", true, "o", "r", "github.com"},
		{"https://GITHUB.COM/O/R.git", true, "O", "R", "github.com"},
		{"git@github.com:specscore/specscore-cli.git", true, "specscore", "specscore-cli", "github.com"},
		{"git@github.com:o/r", true, "o", "r", "github.com"},
		{"ssh://git@github.com/specscore/specscore-cli.git", true, "specscore", "specscore-cli", "github.com"},
		{"ssh://git@github.com/o/r", true, "o", "r", "github.com"},
		{"https://gitlab.com/o/r.git", true, "o", "r", "gitlab.com"},
		{"git@gitlab.com:o/r.git", true, "o", "r", "gitlab.com"},
		{"https://bitbucket.org/o/r", true, "o", "r", "bitbucket.org"},
		// Malformed.
		{"", false, "", "", ""},
		{"not-a-url", false, "", "", ""},
		{"https://github.com/only-owner", false, "", "", ""},
	}
	for _, tt := range tests {
		got, ok := Parse(tt.in)
		if ok != tt.wantOK {
			t.Errorf("Parse(%q) ok = %v, want %v", tt.in, ok, tt.wantOK)
			continue
		}
		if !ok {
			continue
		}
		if got.Owner != tt.wantOwner || got.Repo != tt.wantRepo || got.Host != tt.wantHost {
			t.Errorf("Parse(%q) = %+v, want owner=%q repo=%q host=%q",
				tt.in, got, tt.wantOwner, tt.wantRepo, tt.wantHost)
		}
	}
}

// TestTopLevel initialises a real git repo in a subdirectory and asserts
// TopLevel resolves back to the repository root from a nested path.
func TestTopLevel(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available on PATH")
	}
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")
	sub := filepath.Join(dir, "nested", "deeper")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := TopLevel(sub)
	if err != nil {
		t.Fatalf("TopLevel returned error: %v", err)
	}
	// Compare against `git rev-parse --show-toplevel` directly rather than
	// dir itself: both sides may need OS-level symlink resolution (e.g.
	// macOS /var -> /private/var), and shelling out the same command the
	// production code runs is the only comparison immune to that.
	cmd := exec.Command("git", "-C", sub, "rev-parse", "--show-toplevel")
	want, err := cmd.Output()
	if err != nil {
		t.Fatalf("git rev-parse --show-toplevel failed: %v", err)
	}
	if got != strings.TrimSpace(string(want)) {
		t.Errorf("TopLevel(%q) = %q, want %q", sub, got, strings.TrimSpace(string(want)))
	}
}

// TestTopLevel_NoGitRepo asserts TopLevel returns an error outside any git
// working tree.
func TestTopLevel_NoGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available on PATH")
	}
	dir := t.TempDir()
	if _, err := TopLevel(dir); err == nil {
		t.Error("TopLevel in a non-git directory: expected error, got nil")
	}
}

// TestConfigSet writes a repo-local config value and asserts it round-trips
// through `git config --get`, and that it did NOT touch global config (a
// merge driver installer must never leak into the operator's global git
// config).
func TestConfigSet(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available on PATH")
	}
	dir := t.TempDir()
	runGit(t, dir, "init", "-q", "-b", "main")

	if err := ConfigSet(dir, "merge.specscore-events.driver", "specscore event merge-driver %O %A %B"); err != nil {
		t.Fatalf("ConfigSet returned error: %v", err)
	}

	cmd := exec.Command("git", "-C", dir, "config", "--local", "--get", "merge.specscore-events.driver")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("git config --local --get failed: %v", err)
	}
	got := strings.TrimSpace(string(out))
	want := "specscore event merge-driver %O %A %B"
	if got != want {
		t.Errorf("merge.specscore-events.driver = %q, want %q", got, want)
	}
}

// TestConfigSet_NoGitRepo asserts ConfigSet returns an error outside any
// git working tree rather than silently succeeding or writing elsewhere.
func TestConfigSet_NoGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available on PATH")
	}
	dir := t.TempDir()
	if err := ConfigSet(dir, "merge.specscore-events.driver", "x"); err == nil {
		t.Error("ConfigSet in a non-git directory: expected error, got nil")
	}
}
