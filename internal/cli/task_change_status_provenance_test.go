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

// runGit runs a git subcommand in dir, failing the test on error.
func runGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	c := exec.Command("git", append([]string{"-C", dir}, args...)...)
	if out, err := c.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
}

// implementedByField re-reads a file and returns the value of the first
// `**Implemented-by:**` line, or "" when none is present.
func implementedByField(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		s := strings.TrimSpace(line)
		if strings.HasPrefix(s, "**Implemented-by:**") {
			return strings.TrimSpace(strings.TrimPrefix(s, "**Implemented-by:**"))
		}
	}
	return ""
}

// AC complete-with-provenance (board): full ref repo@sha (branch).
func TestTaskChangeStatus_Board_CompleteWithProvenance(t *testing.T) {
	_, taskFile := stageTaskWithStatus(t, "auth", "in_progress")
	stdout, stderr, err := runTask(t, "change-status", "auth", "--to=complete",
		"--repo", "backstage", "--commit", "a1b2c3d", "--branch", "feat/auth")
	if err != nil {
		t.Fatalf("change-status: %v (stderr=%s)", err, stderr)
	}
	if want := "auth: in_progress → complete\n"; stdout != want {
		t.Errorf("stdout = %q; want %q", stdout, want)
	}
	if got := taskFileStatus(t, taskFile); got != "complete" {
		t.Errorf("status = %q; want complete", got)
	}
	if got := implementedByField(t, taskFile); got != "backstage@a1b2c3d (feat/auth)" {
		t.Errorf("implemented-by = %q; want backstage@a1b2c3d (feat/auth)", got)
	}
}

// AC complete-with-provenance: full clone URL as <repo>.
func TestTaskChangeStatus_Board_CompleteWithCloneURL(t *testing.T) {
	_, taskFile := stageTaskWithStatus(t, "auth", "in_progress")
	url := "https://github.com/sneat-co/backstage.git"
	_, _, err := runTask(t, "change-status", "auth", "--to=complete",
		"--repo", url, "--commit", "a1b2c3d")
	if err != nil {
		t.Fatalf("change-status: %v", err)
	}
	if got := implementedByField(t, taskFile); got != url+"@a1b2c3d" {
		t.Errorf("implemented-by = %q; want %q", got, url+"@a1b2c3d")
	}
}

// AC bare-sha-same-repo-assembly (board): no --repo → bare sha, no `<repo>@`.
func TestTaskChangeStatus_Board_BareSha(t *testing.T) {
	_, taskFile := stageTaskWithStatus(t, "auth", "in_progress")
	_, _, err := runTask(t, "change-status", "auth", "--to=complete", "--commit", "a1b2c3d")
	if err != nil {
		t.Fatalf("change-status: %v", err)
	}
	if got := implementedByField(t, taskFile); got != "a1b2c3d" {
		t.Errorf("implemented-by = %q; want a1b2c3d (bare sha)", got)
	}
}

// Bare sha with a branch: `<sha> (<branch>)`.
func TestTaskChangeStatus_Board_BareShaWithBranch(t *testing.T) {
	_, taskFile := stageTaskWithStatus(t, "auth", "in_progress")
	_, _, err := runTask(t, "change-status", "auth", "--to=complete",
		"--commit", "a1b2c3d", "--branch", "feat/auth")
	if err != nil {
		t.Fatalf("change-status: %v", err)
	}
	if got := implementedByField(t, taskFile); got != "a1b2c3d (feat/auth)" {
		t.Errorf("implemented-by = %q; want a1b2c3d (feat/auth)", got)
	}
}

// AC complete-without-provenance-is-valid (board): no flags → no field written.
func TestTaskChangeStatus_Board_CompleteNoProvenance(t *testing.T) {
	_, taskFile := stageTaskWithStatus(t, "auth", "in_progress")
	stdout, _, err := runTask(t, "change-status", "auth", "--to=complete")
	if err != nil {
		t.Fatalf("change-status: %v", err)
	}
	if want := "auth: in_progress → complete\n"; stdout != want {
		t.Errorf("stdout = %q; want %q", stdout, want)
	}
	if got := taskFileStatus(t, taskFile); got != "complete" {
		t.Errorf("status = %q; want complete", got)
	}
	if got := implementedByField(t, taskFile); got != "" {
		t.Errorf("implemented-by = %q; want none", got)
	}
}

// Provenance is written ONLY on --to=complete: a non-complete transition with
// provenance flags writes no field (rejection wiring is Task 4).
func TestTaskChangeStatus_Board_ProvenanceIgnoredOnNonComplete(t *testing.T) {
	_, taskFile := stageTaskWithStatus(t, "auth", "queued")
	_, _, err := runTask(t, "change-status", "auth", "--to=in_progress",
		"--repo", "backstage", "--commit", "a1b2c3d")
	if err != nil {
		t.Fatalf("change-status: %v", err)
	}
	if got := implementedByField(t, taskFile); got != "" {
		t.Errorf("implemented-by = %q; want none on non-complete", got)
	}
}

// Provenance flags without --commit assemble nothing (Task 4 rejects this); for
// now no field is written even on complete.
func TestTaskChangeStatus_Board_NoCommitNoProvenance(t *testing.T) {
	_, taskFile := stageTaskWithStatus(t, "auth", "in_progress")
	_, _, err := runTask(t, "change-status", "auth", "--to=complete", "--repo", "backstage")
	if err != nil {
		t.Fatalf("change-status: %v", err)
	}
	if got := implementedByField(t, taskFile); got != "" {
		t.Errorf("implemented-by = %q; want none without --commit", got)
	}
}

// AC provenance-not-derived-from-head: completing with no provenance flags
// inside a git work tree whose HEAD is a commit writes NO field — the verb
// never reads ambient git HEAD.
func TestTaskChangeStatus_Board_NotDerivedFromHead(t *testing.T) {
	root, taskFile := stageTaskWithStatus(t, "auth", "in_progress")
	// Make root a real git work tree with a HEAD commit.
	runGit(t, root, "init")
	runGit(t, root, "config", "user.email", "t@example.com")
	runGit(t, root, "config", "user.name", "t")
	runGit(t, root, "add", ".")
	runGit(t, root, "commit", "-m", "initial")

	_, _, err := runTask(t, "change-status", "auth", "--to=complete")
	if err != nil {
		t.Fatalf("change-status: %v", err)
	}
	if got := implementedByField(t, taskFile); got != "" {
		t.Errorf("implemented-by = %q; want none (HEAD must not be read)", got)
	}
}

// A provenance-write I/O failure surfaces Unexpected (10). The status Rewrite
// (which uses os directly) succeeds; the seamed provenance write is forced to
// fail.
func TestTaskChangeStatus_Board_ProvenanceWriteFailure(t *testing.T) {
	stageTaskWithStatus(t, "auth", "in_progress")
	orig := osWriteFileFn
	osWriteFileFn = func(string, []byte, os.FileMode) error { return errors.New("boom") }
	t.Cleanup(func() { osWriteFileFn = orig })

	_, _, err := runTask(t, "change-status", "auth", "--to=complete", "--commit", "a1b2c3d")
	if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d (Unexpected); err=%v", got, exitcode.Unexpected, err)
	}
}

// writeBoardImplementedBy unit: a file with no **Status:** line is an error
// (defensive; unreachable via the verb because Rewrite guarantees the line).
func TestWriteBoardImplementedBy_NoStatusLine(t *testing.T) {
	p := filepath.Join(t.TempDir(), "README.md")
	if err := os.WriteFile(p, []byte("# Auth\n\nNo status.\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if err := writeBoardImplementedBy(p, "backstage@a1b2c3d"); err == nil {
		t.Fatal("expected error for missing status line, got nil")
	}
}

// writeBoardImplementedBy unit: a missing file surfaces the ReadFile error.
func TestWriteBoardImplementedBy_ReadError(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope.md")
	if err := writeBoardImplementedBy(missing, "x@y"); err == nil {
		t.Fatal("expected ReadFile error, got nil")
	}
}

// --- plan-inline provenance ---

// AC complete-with-provenance (plan-inline): full ref onto the right block; the
// sibling block is byte-untouched.
func TestTaskChangeStatus_PlanInline_CompleteWithProvenance(t *testing.T) {
	_, planPath := stagePlanWithTasks(t, "auth", twoTaskPlanBody)
	_, _, err := runTask(t, "change-status", "setup", "--plan", "auth", "--to=complete",
		"--repo", "backstage", "--commit", "a1b2c3d", "--branch", "feat/auth")
	if err != nil {
		t.Fatalf("change-status: %v", err)
	}
	if got := planTaskStatus(t, planPath, "setup"); got != "complete" {
		t.Errorf("setup status = %q; want complete", got)
	}
	if got := implementedByField(t, planPath); got != "backstage@a1b2c3d (feat/auth)" {
		t.Errorf("implemented-by = %q; want backstage@a1b2c3d (feat/auth)", got)
	}
	// The provenance line must sit inside the setup block, adjacent to its
	// **Status:** — between **Status:** and **Depends-On:**.
	data, _ := os.ReadFile(planPath)
	body := string(data)
	statusIdx := strings.Index(body, "**Status:** complete")
	implIdx := strings.Index(body, "**Implemented-by:**")
	depsIdx := strings.Index(body, "**Depends-On:** —")
	if !(statusIdx < implIdx && implIdx < depsIdx) {
		t.Errorf("provenance not placed adjacent to Status in setup block:\n%s", body)
	}
	// Sibling deploy block untouched.
	if got := planTaskStatus(t, planPath, "deploy"); got != "planning" {
		t.Errorf("deploy changed: %q", got)
	}
}

// AC bare-sha-same-repo-assembly (plan-inline): no --repo → bare sha.
func TestTaskChangeStatus_PlanInline_BareSha(t *testing.T) {
	_, planPath := stagePlanWithTasks(t, "auth", twoTaskPlanBody)
	_, _, err := runTask(t, "change-status", "setup", "--plan", "auth",
		"--to=complete", "--commit", "deadbee")
	if err != nil {
		t.Fatalf("change-status: %v", err)
	}
	if got := implementedByField(t, planPath); got != "deadbee" {
		t.Errorf("implemented-by = %q; want deadbee (bare sha)", got)
	}
}

// AC complete-without-provenance-is-valid (plan-inline): no flags → no field.
func TestTaskChangeStatus_PlanInline_CompleteNoProvenance(t *testing.T) {
	_, planPath := stagePlanWithTasks(t, "auth", twoTaskPlanBody)
	_, _, err := runTask(t, "change-status", "setup", "--plan", "auth", "--to=complete")
	if err != nil {
		t.Fatalf("change-status: %v", err)
	}
	if got := planTaskStatus(t, planPath, "setup"); got != "complete" {
		t.Errorf("setup status = %q; want complete", got)
	}
	if got := implementedByField(t, planPath); got != "" {
		t.Errorf("implemented-by = %q; want none", got)
	}
}

// assembleImplementedByRef unit table.
func TestAssembleImplementedByRef(t *testing.T) {
	cases := []struct {
		repo, commit, branch, want string
	}{
		{"backstage", "a1b2c3d", "feat/auth", "backstage@a1b2c3d (feat/auth)"},
		{"backstage", "a1b2c3d", "", "backstage@a1b2c3d"},
		{"", "a1b2c3d", "", "a1b2c3d"},
		{"", "a1b2c3d", "feat/auth", "a1b2c3d (feat/auth)"},
		{"backstage", "", "feat/auth", ""}, // no commit → nothing
		{"", "", "", ""},
		{"  backstage  ", "  a1b2c3d  ", "  feat/auth  ", "backstage@a1b2c3d (feat/auth)"}, // trimmed
	}
	for _, tc := range cases {
		if got := assembleImplementedByRef(tc.repo, tc.commit, tc.branch); got != tc.want {
			t.Errorf("assemble(%q,%q,%q) = %q; want %q", tc.repo, tc.commit, tc.branch, got, tc.want)
		}
	}
}
