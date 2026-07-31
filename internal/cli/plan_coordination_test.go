package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/plan"
)

// gitInitWithRemoteAndBranch initialises a real git repo in dir on the named
// branch, with one empty commit (so HEAD/branch resolve) and an "origin"
// remote pointing at remoteURL. Skips the calling test if `git` is missing.
func gitInitWithRemoteAndBranch(t *testing.T, dir, remoteURL, branch string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available on PATH")
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", branch},
		{"config", "user.email", "t@example.com"},
		{"config", "user.name", "T"},
		{"remote", "add", "origin", remoteURL},
		{"commit", "--allow-empty", "-q", "-m", "initial"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
}

// gitWorktreeCanonicalAndBranch creates a REAL canonical git repo (origin
// remote = remoteURL, default branch = declaredBranch) containing the given
// files, commits them, then `git worktree add`s a second, linked checkout on
// worktreeBranch (a DIFFERENT branch from declaredBranch). Returns the
// worktree directory — where `.git` is a FILE pointing back at the
// canonical's `.git/worktrees/<name>`, not a directory, exactly like a real
// `git worktree` layout (see gh:specscore/specscore-cli coordination-gate
// verification report, 2026-07-31: reproduction used "a worktree of
// sneat-co/chess on branch test/coord-check2"). Skips if `git` is missing.
func gitWorktreeCanonicalAndBranch(t *testing.T, remoteURL, declaredBranch, worktreeBranch string, files map[string]string) string {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available on PATH")
	}
	root := t.TempDir()
	canonical := filepath.Join(root, "canonical")
	if err := os.MkdirAll(canonical, 0o755); err != nil {
		t.Fatal(err)
	}
	runGitIn := func(dir string, args ...string) {
		t.Helper()
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s (in %s): %v\n%s", strings.Join(args, " "), dir, err, out)
		}
	}
	runGitIn(canonical, "init", "-q", "-b", declaredBranch)
	runGitIn(canonical, "config", "user.email", "t@example.com")
	runGitIn(canonical, "config", "user.name", "T")
	runGitIn(canonical, "remote", "add", "origin", remoteURL)
	for rel, content := range files {
		full := filepath.Join(canonical, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	runGitIn(canonical, "add", "-A")
	runGitIn(canonical, "commit", "-q", "-m", "initial")

	worktree := filepath.Join(root, "worktree-"+worktreeBranch)
	runGitIn(canonical, "worktree", "add", "-q", "-b", worktreeBranch, worktree)
	return worktree
}

// realisticCoordinatedPlanBody mirrors the exact field order the founder's
// worked reproduction used: frontmatter, title, then Status / Source Feature
// / Date / Owner / Supersedes / Coordination (Coordination immediately after
// Supersedes, not the first field) — as opposed to the earlier, structurally
// simpler fixtures in this file that put Coordination right after Source
// Feature.
const realisticCoordinatedPlanBody = `---
format: https://specscore.md/plan-specification
status: Draft
---

# Plan: Full Settings Exposure

**Status:** Draft
**Source Feature:** settings
**Date:** 2026-07-20
**Owner:** alex
**Supersedes:** —
**Coordination:** sneat-co/chess@plans

## Summary

Test fixture.

## Approach

Test fixture.

## Tasks

### Task 4: Wire settings toggle

**Id:** task-4
**Verifies:** settings#ac:toggle
**Status:** planning
**Depends-On:** —

Body.

## Open Questions

None at this time.

---
*This document follows the https://specscore.md/plan-specification*
`

// AC (regression, 2026-07-31 coordination-gate verification report): a
// mismatch is refused end-to-end in a REAL `git worktree` checkout (not a
// plain repo — `.git` is a file, not a directory) with an SSH-form origin
// remote (`git@host:owner/repo.git`), against a realistic full plan document
// where **Coordination:** is the LAST header field rather than the first.
// Neither of these two conditions (worktree layout, SSH remote) was covered
// by any of the other tests in this file before this one — closing that gap.
func TestTaskChangeStatus_PlanInline_CoordinationMismatch_RealWorktree_SSHRemote_CLI(t *testing.T) {
	worktree := gitWorktreeCanonicalAndBranch(t,
		"git@github.com:sneat-co/chess.git", "plans", "test/coord-check2",
		map[string]string{
			"specscore.yaml":                       "name: chess\n",
			"spec/features/settings/README.md":     "# Feature: Settings\n",
			"spec/plans/full-settings-exposure.md": realisticCoordinatedPlanBody,
		})
	withCwd(t, worktree)

	before, err := os.ReadFile(filepath.Join(worktree, "spec", "plans", "full-settings-exposure.md"))
	if err != nil {
		t.Fatal(err)
	}

	_, stderr, err := runTask(t, "change-status", "task-4", "--plan", "full-settings-exposure", "--to=queued")
	if got := exitCodeOfErr(err); got != exitcode.Conflict {
		t.Fatalf("exit = %d, want %d (Conflict); err=%v stderr=%s", got, exitcode.Conflict, err, stderr)
	}
	after, err := os.ReadFile(filepath.Join(worktree, "spec", "plans", "full-settings-exposure.md"))
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("plan file changed despite refusal:\nbefore=%s\nafter=%s", before, after)
	}
}

// AC: the same real-worktree + SSH-remote setup succeeds (task status
// rewritten) when invoked from a worktree ON the declared branch.
func TestTaskChangeStatus_PlanInline_CoordinationMatched_RealWorktree_SSHRemote_CLI(t *testing.T) {
	worktree := gitWorktreeCanonicalAndBranch(t,
		"git@github.com:sneat-co/chess.git", "plans", "plans-clone",
		map[string]string{
			"specscore.yaml":                       "name: chess\n",
			"spec/features/settings/README.md":     "# Feature: Settings\n",
			"spec/plans/full-settings-exposure.md": strings.Replace(realisticCoordinatedPlanBody, "sneat-co/chess@plans", "sneat-co/chess@plans-clone", 1),
		})
	withCwd(t, worktree)

	_, stderr, err := runTask(t, "change-status", "task-4", "--plan", "full-settings-exposure", "--to=queued")
	if err != nil {
		t.Fatalf("task change-status: %v (stderr=%s)", err, stderr)
	}
	if got := planTaskStatus(t, filepath.Join(worktree, "spec", "plans", "full-settings-exposure.md"), "task-4"); got != "queued" {
		t.Errorf("expected task queued, got %q", got)
	}
}

// AC: enforceCoordinationBranch/coordinationCheck match correctly against an
// SSH-form origin remote (git@host:owner/repo.git), not just the HTTPS form
// every other unit test in this file uses.
func TestEnforceCoordinationBranch_Matched_SSHRemote_Passes(t *testing.T) {
	dir := t.TempDir()
	gitInitWithRemoteAndBranch(t, dir, "git@github.com:specscore/specscore-cli.git", "main")

	p := &plan.Plan{Slug: "auth", Coordination: "specscore/specscore-cli@main", CoordinationLine: 6}
	var warn bytes.Buffer
	if err := enforceCoordinationBranch(p, dir, false, &warn); err != nil {
		t.Fatalf("expected nil error for matching SSH-remote repo/branch, got %v", err)
	}
}

// AC: an SSH-form origin remote on a mismatched branch still refuses.
func TestEnforceCoordinationBranch_Mismatch_SSHRemote_Refuses(t *testing.T) {
	dir := t.TempDir()
	gitInitWithRemoteAndBranch(t, dir, "git@github.com:specscore/specscore-cli.git", "some-other-branch")

	p := &plan.Plan{Slug: "auth", Coordination: "specscore/specscore-cli@main", CoordinationLine: 6}
	var warn bytes.Buffer
	err := enforceCoordinationBranch(p, dir, false, &warn)
	if got := exitCodeOfErr(err); got != exitcode.Conflict {
		t.Fatalf("exit = %d, want %d (Conflict); err=%v", got, exitcode.Conflict, err)
	}
}

// ----- enforceCoordinationBranch / coordinationCheck unit tests -----

// AC: a plan with no **Coordination:** field is unrestricted — the check
// passes even in a directory that is not a git repository at all.
func TestEnforceCoordinationBranch_NoField_Passes(t *testing.T) {
	p := &plan.Plan{Slug: "auth"} // CoordinationLine == 0
	var warn bytes.Buffer
	if err := enforceCoordinationBranch(p, t.TempDir(), false, &warn); err != nil {
		t.Fatalf("expected nil error for absent Coordination field, got %v", err)
	}
	if warn.Len() != 0 {
		t.Errorf("expected no warning, got %q", warn.String())
	}
}

// AC: the check passes silently when the current repo/branch matches the
// declared reference.
func TestEnforceCoordinationBranch_Matched_Passes(t *testing.T) {
	dir := t.TempDir()
	gitInitWithRemoteAndBranch(t, dir, "https://github.com/specscore/specscore-cli.git", "feat/plan-coordination-branch")

	p := &plan.Plan{Slug: "auth", Coordination: "specscore/specscore-cli@feat/plan-coordination-branch", CoordinationLine: 6}
	var warn bytes.Buffer
	if err := enforceCoordinationBranch(p, dir, false, &warn); err != nil {
		t.Fatalf("expected nil error for matching repo/branch, got %v", err)
	}
	if warn.Len() != 0 {
		t.Errorf("expected no warning on a match, got %q", warn.String())
	}
}

// AC: owner/repo comparison is case-insensitive (GitHub identity), but branch
// comparison is exact (git branch names are case-sensitive).
func TestEnforceCoordinationBranch_Matched_OwnerRepoCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	gitInitWithRemoteAndBranch(t, dir, "https://github.com/SpecScore/SpecScore-CLI.git", "main")

	p := &plan.Plan{Slug: "auth", Coordination: "specscore/specscore-cli@main", CoordinationLine: 6}
	var warn bytes.Buffer
	if err := enforceCoordinationBranch(p, dir, false, &warn); err != nil {
		t.Fatalf("expected nil error for case-insensitive owner/repo match, got %v", err)
	}
}

// AC: a branch mismatch refuses with a Conflict exit and names both the
// declared and the actual location, and the refusal is NOT bypassed by
// default (force is false).
func TestEnforceCoordinationBranch_BranchMismatch_Refuses(t *testing.T) {
	dir := t.TempDir()
	gitInitWithRemoteAndBranch(t, dir, "https://github.com/specscore/specscore-cli.git", "some-other-branch")

	p := &plan.Plan{Slug: "auth", Coordination: "specscore/specscore-cli@main", CoordinationLine: 6}
	var warn bytes.Buffer
	err := enforceCoordinationBranch(p, dir, false, &warn)
	if got := exitCodeOfErr(err); got != exitcode.Conflict {
		t.Fatalf("exit = %d, want %d (Conflict); err=%v", got, exitcode.Conflict, err)
	}
	for _, want := range []string{"auth", "specscore/specscore-cli@main", "force-coordination"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error message missing %q: %q", want, err.Error())
		}
	}
	if warn.Len() != 0 {
		t.Errorf("expected no warning when refusing (force=false), got %q", warn.String())
	}
}

// AC: a repo mismatch (right branch name, wrong owner/repo) also refuses.
func TestEnforceCoordinationBranch_RepoMismatch_Refuses(t *testing.T) {
	dir := t.TempDir()
	gitInitWithRemoteAndBranch(t, dir, "https://github.com/some-org/some-other-repo.git", "main")

	p := &plan.Plan{Slug: "auth", Coordination: "specscore/specscore-cli@main", CoordinationLine: 6}
	var warn bytes.Buffer
	err := enforceCoordinationBranch(p, dir, false, &warn)
	if got := exitCodeOfErr(err); got != exitcode.Conflict {
		t.Fatalf("exit = %d, want %d (Conflict); err=%v", got, exitcode.Conflict, err)
	}
}

// AC: a non-GitHub origin remote (unparseable by pkg/gitremote's MVP) is
// treated as a mismatch.
func TestEnforceCoordinationBranch_NonGitHubRemote_Refuses(t *testing.T) {
	dir := t.TempDir()
	gitInitWithRemoteAndBranch(t, dir, "https://gitlab.com/specscore/specscore-cli.git", "main")

	p := &plan.Plan{Slug: "auth", Coordination: "specscore/specscore-cli@main", CoordinationLine: 6}
	var warn bytes.Buffer
	err := enforceCoordinationBranch(p, dir, false, &warn)
	if got := exitCodeOfErr(err); got != exitcode.Conflict {
		t.Fatalf("exit = %d, want %d (Conflict); err=%v", got, exitcode.Conflict, err)
	}
}

// AC: a detached HEAD (valid origin, no current branch) is treated as a
// mismatch.
func TestEnforceCoordinationBranch_DetachedHead_Refuses(t *testing.T) {
	dir := t.TempDir()
	gitInitWithRemoteAndBranch(t, dir, "https://github.com/specscore/specscore-cli.git", "main")
	cmd := exec.Command("git", "-C", dir, "checkout", "-q", "--detach", "HEAD")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git checkout --detach: %v\n%s", err, out)
	}

	p := &plan.Plan{Slug: "auth", Coordination: "specscore/specscore-cli@main", CoordinationLine: 6}
	var warn bytes.Buffer
	err := enforceCoordinationBranch(p, dir, false, &warn)
	if got := exitCodeOfErr(err); got != exitcode.Conflict {
		t.Fatalf("exit = %d, want %d (Conflict); err=%v", got, exitcode.Conflict, err)
	}
}

// AC: no git repository at all (no origin remote resolvable) is treated as a
// mismatch — fail closed rather than silently accepting an unverifiable
// invocation.
func TestEnforceCoordinationBranch_NoGitRepo_Refuses(t *testing.T) {
	dir := t.TempDir() // not a git repo
	p := &plan.Plan{Slug: "auth", Coordination: "specscore/specscore-cli@main", CoordinationLine: 6}
	var warn bytes.Buffer
	err := enforceCoordinationBranch(p, dir, false, &warn)
	if got := exitCodeOfErr(err); got != exitcode.Conflict {
		t.Fatalf("exit = %d, want %d (Conflict); err=%v", got, exitcode.Conflict, err)
	}
}

// AC: a malformed **Coordination:** value refuses (it's already a P-010 lint
// violation) rather than silently skipping the check.
func TestEnforceCoordinationBranch_Malformed_Refuses(t *testing.T) {
	p := &plan.Plan{Slug: "auth", Coordination: "not-a-coordination-ref", CoordinationLine: 6}
	var warn bytes.Buffer
	err := enforceCoordinationBranch(p, t.TempDir(), false, &warn)
	if got := exitCodeOfErr(err); got != exitcode.Conflict {
		t.Fatalf("exit = %d, want %d (Conflict); err=%v", got, exitcode.Conflict, err)
	}
	if !strings.Contains(err.Error(), "malformed") || !strings.Contains(err.Error(), "P-010") {
		t.Errorf("expected error to cite malformed value and P-010, got: %q", err.Error())
	}
}

// AC: --force-coordination bypasses a mismatch, prints a warning naming the
// plan and what was bypassed, and returns nil (mutation proceeds).
func TestEnforceCoordinationBranch_ForceBypassesMismatch_Warns(t *testing.T) {
	dir := t.TempDir()
	gitInitWithRemoteAndBranch(t, dir, "https://github.com/specscore/specscore-cli.git", "some-other-branch")

	p := &plan.Plan{Slug: "auth", Coordination: "specscore/specscore-cli@main", CoordinationLine: 6}
	var warn bytes.Buffer
	if err := enforceCoordinationBranch(p, dir, true, &warn); err != nil {
		t.Fatalf("expected nil error with force=true, got %v", err)
	}
	out := warn.String()
	for _, want := range []string{"warning:", "force-coordination", "auth"} {
		if !strings.Contains(out, want) {
			t.Errorf("warning missing %q: %q", want, out)
		}
	}
}

// AC: --force-coordination also bypasses a malformed value.
func TestEnforceCoordinationBranch_ForceBypassesMalformed_Warns(t *testing.T) {
	p := &plan.Plan{Slug: "auth", Coordination: "not-a-coordination-ref", CoordinationLine: 6}
	var warn bytes.Buffer
	if err := enforceCoordinationBranch(p, t.TempDir(), true, &warn); err != nil {
		t.Fatalf("expected nil error with force=true, got %v", err)
	}
	if !strings.Contains(warn.String(), "warning:") {
		t.Errorf("expected a warning, got %q", warn.String())
	}
}

// ----- CLI wiring: plan change-status -----

// coordinatedPlan mirrors stagePlan (plan_change_status_test.go) but also
// injects a **Coordination:** header line and git-inits root on the given
// actualBranch with an origin remote parsed from actualRemote, so the CLI's
// resolveSpecRoot/gitremote calls resolve real ambient state.
func coordinatedPlan(t *testing.T, slug, status, coordination, actualRemote, actualBranch string) string {
	t.Helper()
	root := stagePlan(t, slug, status)
	path := filepath.Join(root, "spec", "plans", slug+".md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read plan: %v", err)
	}
	patched := strings.Replace(string(raw), "**Supersedes:** —\n", "**Supersedes:** —\n**Coordination:** "+coordination+"\n", 1)
	if patched == string(raw) {
		t.Fatalf("failed to inject **Coordination:** line into plan:\n%s", raw)
	}
	if err := os.WriteFile(path, []byte(patched), 0o644); err != nil {
		t.Fatalf("write patched plan: %v", err)
	}
	gitInitWithRemoteAndBranch(t, root, actualRemote, actualBranch)
	return root
}

// AC: `plan change-status` refuses on a coordination-branch mismatch and
// leaves the plan file unchanged.
func TestPlanChangeStatus_CoordinationMismatch_CLI(t *testing.T) {
	root := coordinatedPlan(t, "auth", "Draft",
		"specscore/specscore-cli@main",
		"https://github.com/specscore/specscore-cli.git", "some-other-branch")

	before, _ := os.ReadFile(filepath.Join(root, "spec", "plans", "auth.md"))
	_, stderr, err := runPlan(t, "change-status", "auth", "--to=in review")
	if got := exitCodeOfErr(err); got != exitcode.Conflict {
		t.Fatalf("exit = %d, want %d (Conflict); err=%v stderr=%s", got, exitcode.Conflict, err, stderr)
	}
	after, _ := os.ReadFile(filepath.Join(root, "spec", "plans", "auth.md"))
	if string(before) != string(after) {
		t.Errorf("plan file changed despite refusal:\nbefore=%s\nafter=%s", before, after)
	}
}

// Covers the coordination-branch pre-check's plan.Parse error branch in
// runPlanChangeStatus (resolvePlanFile finds the file, but it becomes
// unreadable before Parse opens it).
func TestPlanChangeStatus_CoordinationParseError_CLI(t *testing.T) {
	root := stagePlan(t, "auth", "Draft")
	path := filepath.Join(root, "spec", "plans", "auth.md")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	_, _, err := runPlan(t, "change-status", "auth", "--to=in review")
	if err == nil {
		t.Skip("filesystem allowed reading a 0-perm file (running as root?)")
	}
	if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d (Unexpected); err=%v", got, exitcode.Unexpected, err)
	}
}

// AC: `plan change-status` succeeds when the current repo/branch matches the
// declared coordination reference.
func TestPlanChangeStatus_CoordinationMatched_CLI(t *testing.T) {
	root := coordinatedPlan(t, "auth", "Draft",
		"specscore/specscore-cli@main",
		"https://github.com/specscore/specscore-cli.git", "main")

	stdout, stderr, err := runPlan(t, "change-status", "auth", "--to=in review")
	if err != nil {
		t.Fatalf("change-status: %v (stderr=%s)", err, stderr)
	}
	if want := "auth: Draft → In Review\n"; stdout != want {
		t.Errorf("stdout = %q; want %q", stdout, want)
	}
	body, _ := os.ReadFile(filepath.Join(root, "spec", "plans", "auth.md"))
	if !strings.Contains(string(body), "**Status:** In Review") {
		t.Errorf("status not rewritten:\n%s", body)
	}
}

// AC: --force-coordination bypasses the mismatch on `plan change-status`,
// prints a warning to stderr, and the transition proceeds.
func TestPlanChangeStatus_CoordinationForce_CLI(t *testing.T) {
	root := coordinatedPlan(t, "auth", "Draft",
		"specscore/specscore-cli@main",
		"https://github.com/specscore/specscore-cli.git", "some-other-branch")

	stdout, stderr, err := runPlan(t, "change-status", "auth", "--to=in review", "--force-coordination")
	if err != nil {
		t.Fatalf("change-status: %v (stderr=%s)", err, stderr)
	}
	if !strings.Contains(stderr, "warning:") || !strings.Contains(stderr, "force-coordination") {
		t.Errorf("expected a --force-coordination warning on stderr, got %q", stderr)
	}
	if want := "auth: Draft → In Review\n"; stdout != want {
		t.Errorf("stdout = %q; want %q", stdout, want)
	}
	body, _ := os.ReadFile(filepath.Join(root, "spec", "plans", "auth.md"))
	if !strings.Contains(string(body), "**Status:** In Review") {
		t.Errorf("status not rewritten:\n%s", body)
	}
}

// A plan with no **Coordination:** field behaves exactly as before this
// feature — change-status succeeds from any repo/branch (including a
// directory that is not a git repository at all).
func TestPlanChangeStatus_NoCoordinationField_UnrestrictedEvenOutsideGit_CLI(t *testing.T) {
	stagePlan(t, "auth", "Draft") // no git init, no Coordination line
	_, stderr, err := runPlan(t, "change-status", "auth", "--to=in review")
	if err != nil {
		t.Fatalf("change-status: %v (stderr=%s)", err, stderr)
	}
}

// ----- CLI wiring: plan reconcile -----

// AC: `plan reconcile` refuses on a coordination-branch mismatch.
func TestPlanReconcile_CoordinationMismatch_CLI(t *testing.T) {
	root := stageReconcilablePlan(t, "auth", "Draft", eightPlanningTasks()...)
	path := filepath.Join(root, "spec", "plans", "auth.md")
	raw, _ := os.ReadFile(path)
	patched := strings.Replace(string(raw), "**Supersedes:** —\n", "**Supersedes:** —\n**Coordination:** specscore/specscore-cli@main\n", 1)
	if err := os.WriteFile(path, []byte(patched), 0o644); err != nil {
		t.Fatalf("write patched plan: %v", err)
	}
	gitInitWithRemoteAndBranch(t, root, "https://github.com/specscore/specscore-cli.git", "some-other-branch")

	_, stderr, err := runPlan(t, "reconcile", "auth", "--tasks=complete", "--note", "shipped ahead of the record")
	if got := exitCodeOfErr(err); got != exitcode.Conflict {
		t.Fatalf("exit = %d, want %d (Conflict); err=%v stderr=%s", got, exitcode.Conflict, err, stderr)
	}
}

// Covers the coordination-branch pre-check's plan.Parse error branch in
// runPlanReconcile (resolvePlanFile finds the file, but it becomes
// unreadable before Parse opens it).
func TestPlanReconcile_CoordinationParseError_CLI(t *testing.T) {
	root := stageReconcilablePlan(t, "auth", "Draft", eightPlanningTasks()...)
	path := filepath.Join(root, "spec", "plans", "auth.md")
	if err := os.Chmod(path, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })

	_, _, err := runPlan(t, "reconcile", "auth", "--tasks=complete", "--note", "x")
	if err == nil {
		t.Skip("filesystem allowed reading a 0-perm file (running as root?)")
	}
	if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d (Unexpected); err=%v", got, exitcode.Unexpected, err)
	}
}

// AC: --force-coordination bypasses the mismatch on `plan reconcile`.
func TestPlanReconcile_CoordinationForce_CLI(t *testing.T) {
	root := stageReconcilablePlan(t, "auth", "Draft", eightPlanningTasks()...)
	path := filepath.Join(root, "spec", "plans", "auth.md")
	raw, _ := os.ReadFile(path)
	patched := strings.Replace(string(raw), "**Supersedes:** —\n", "**Supersedes:** —\n**Coordination:** specscore/specscore-cli@main\n", 1)
	if err := os.WriteFile(path, []byte(patched), 0o644); err != nil {
		t.Fatalf("write patched plan: %v", err)
	}
	gitInitWithRemoteAndBranch(t, root, "https://github.com/specscore/specscore-cli.git", "some-other-branch")

	_, stderr, err := runPlan(t, "reconcile", "auth", "--tasks=complete", "--note", "shipped ahead of the record", "--force-coordination")
	if err != nil {
		t.Fatalf("reconcile: %v (stderr=%s)", err, stderr)
	}
	if !strings.Contains(stderr, "warning:") || !strings.Contains(stderr, "force-coordination") {
		t.Errorf("expected a --force-coordination warning on stderr, got %q", stderr)
	}
}

// ----- CLI wiring: task change-status --plan -----

// coordinatedPlanInlineBody is twoTaskPlanBody with a **Coordination:**
// header line injected after **Source Feature:**.
const coordinatedPlanInlineBody = `# Plan: Auth

**Status:** Executing
**Source Feature:** auth
**Coordination:** specscore/specscore-cli@main

## Tasks

### Task 1: Setup

**Id:** setup
**Status:** in_progress
**Depends-On:** —

Setup body.

### Task 2: Deploy

**Id:** deploy
**Status:** planning
**Depends-On:** 1

Deploy body.
`

// AC: `task change-status --plan` refuses on a coordination-branch mismatch
// and leaves the plan-inline task's status untouched.
func TestTaskChangeStatus_PlanInline_CoordinationMismatch_CLI(t *testing.T) {
	root, planPath := stagePlanWithTasks(t, "auth", coordinatedPlanInlineBody)
	gitInitWithRemoteAndBranch(t, root, "https://github.com/specscore/specscore-cli.git", "some-other-branch")

	_, stderr, err := runTask(t, "change-status", "setup", "--plan", "auth", "--to=complete")
	if got := exitCodeOfErr(err); got != exitcode.Conflict {
		t.Fatalf("exit = %d, want %d (Conflict); err=%v stderr=%s", got, exitcode.Conflict, err, stderr)
	}
	if got := planTaskStatus(t, planPath, "setup"); got != "in_progress" {
		t.Errorf("task status changed despite refusal: %q", got)
	}
}

// AC: --force-coordination bypasses the mismatch on `task change-status --plan`.
func TestTaskChangeStatus_PlanInline_CoordinationForce_CLI(t *testing.T) {
	root, planPath := stagePlanWithTasks(t, "auth", coordinatedPlanInlineBody)
	gitInitWithRemoteAndBranch(t, root, "https://github.com/specscore/specscore-cli.git", "some-other-branch")

	_, stderr, err := runTask(t, "change-status", "setup", "--plan", "auth", "--to=complete", "--force-coordination")
	if err != nil {
		t.Fatalf("task change-status: %v (stderr=%s)", err, stderr)
	}
	if !strings.Contains(stderr, "warning:") || !strings.Contains(stderr, "force-coordination") {
		t.Errorf("expected a --force-coordination warning on stderr, got %q", stderr)
	}
	if got := planTaskStatus(t, planPath, "setup"); got != "complete" {
		t.Errorf("expected task completed after force-bypass, got %q", got)
	}
}

// AC: `task change-status --plan` succeeds without any warning when the
// current repo/branch matches the declared coordination reference.
func TestTaskChangeStatus_PlanInline_CoordinationMatched_CLI(t *testing.T) {
	root, planPath := stagePlanWithTasks(t, "auth", coordinatedPlanInlineBody)
	gitInitWithRemoteAndBranch(t, root, "https://github.com/specscore/specscore-cli.git", "main")

	_, stderr, err := runTask(t, "change-status", "setup", "--plan", "auth", "--to=complete")
	if err != nil {
		t.Fatalf("task change-status: %v (stderr=%s)", err, stderr)
	}
	if strings.Contains(stderr, "warning:") {
		t.Errorf("expected no warning on a coordination match, got %q", stderr)
	}
	if got := planTaskStatus(t, planPath, "setup"); got != "complete" {
		t.Errorf("expected task completed, got %q", got)
	}
}

// AC: --amend-provenance on a plan-inline task also enforces the
// coordination-branch check.
func TestTaskAmendProvenance_PlanInline_CoordinationMismatch_CLI(t *testing.T) {
	body := `# Plan: Auth

**Status:** Executing
**Source Feature:** auth
**Coordination:** specscore/specscore-cli@main

## Tasks

### Task 1: Setup

**Id:** setup
**Status:** complete
**Depends-On:** —

Setup body.
`
	root, _ := stagePlanWithTasks(t, "auth", body)
	gitInitWithRemoteAndBranch(t, root, "https://github.com/specscore/specscore-cli.git", "some-other-branch")

	_, stderr, err := runTask(t, "change-status", "setup", "--plan", "auth", "--amend-provenance", "--repo", "sneat-co/chess", "--commit", "abc1234")
	if got := exitCodeOfErr(err); got != exitcode.Conflict {
		t.Fatalf("exit = %d, want %d (Conflict); err=%v stderr=%s", got, exitcode.Conflict, err, stderr)
	}
}
