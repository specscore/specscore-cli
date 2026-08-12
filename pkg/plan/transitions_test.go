package plan

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

// planBody returns a minimal lint-shaped flat Plan body in the given status.
func planBody(status string) string {
	return "---\nformat: https://specscore.md/plan-specification\nstatus: " + status + "\n---\n\n" +
		"# Plan: Auth\n\n" +
		"**Status:** " + status + "\n" +
		"**Source:** none\n" +
		"**Date:** 2026-06-17\n" +
		"**Owner:** alex\n" +
		"**Supersedes:** —\n\n" +
		"## Summary\n\nAuth.\n\n" +
		"## Approach\n\nOne task.\n\n" +
		"## Tasks\n\n### Task 1: Do it\n\n**Verifies:** —\n\nBody.\n\n" +
		"## Open Questions\n\nNone at this time.\n\n" +
		"---\n*This document follows the https://specscore.md/plan-specification*\n"
}

// stageFlatPlan writes a flat plan at spec/plans/<slug>.md under a fresh
// SpecRoot and returns (specRoot, planPath).
func stageFlatPlan(t *testing.T, slug, status string) (string, string) {
	t.Helper()
	root := t.TempDir()
	plansDir := filepath.Join(root, "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(plansDir, slug+".md")
	if err := os.WriteFile(path, []byte(planBody(status)), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	return root, path
}

func okHook() error { return nil }

func codeOf(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var e *exitcode.Error
	if !errors.As(err, &e) {
		t.Fatalf("error is not *exitcode.Error: %T (%v)", err, err)
	}
	return e.ExitCode()
}

func TestChangeStatus_HappyPath_DraftToInReview(t *testing.T) {
	root, path := stageFlatPlan(t, "auth", "Draft")
	res, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "auth", To: lifecycle.PlanInReview, PostMutation: okHook,
	})
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}
	if res.From != lifecycle.PlanDraft || res.To != lifecycle.PlanInReview {
		t.Errorf("result = %+v", res)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "**Status:** In Review") {
		t.Errorf("status not rewritten:\n%s", body)
	}
	if !strings.Contains(string(body), "status: In Review") {
		t.Errorf("frontmatter mirror not rewritten:\n%s", body)
	}
}

func TestChangeStatus_HappyPath_DraftToApprovedDirect(t *testing.T) {
	root, path := stageFlatPlan(t, "auth", "Draft")
	res, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "auth", To: lifecycle.PlanApproved, PostMutation: okHook,
	})
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}
	if res.From != lifecycle.PlanDraft || res.To != lifecycle.PlanApproved {
		t.Errorf("result = %+v", res)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "**Status:** Approved") {
		t.Errorf("status not rewritten:\n%s", body)
	}
	if !strings.Contains(string(body), "status: Approved") {
		t.Errorf("frontmatter mirror not rewritten:\n%s", body)
	}
}

func TestChangeStatus_Withdrawn_WritesResolution(t *testing.T) {
	root, path := stageFlatPlan(t, "auth", "Approved")
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "auth", To: lifecycle.PlanWithdrawn,
		Note: "abandoned after pivot", PostMutation: okHook,
	})
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "**Status:** Withdrawn") {
		t.Errorf("status not rewritten:\n%s", body)
	}
	if !strings.Contains(string(body), "## Resolution") ||
		!strings.Contains(string(body), "abandoned after pivot") {
		t.Errorf("resolution note not written:\n%s", body)
	}
}

func TestChangeStatus_Superseded_WritesSuccessorAndNote(t *testing.T) {
	root, path := stageFlatPlan(t, "auth", "Approved")
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "auth", To: lifecycle.PlanSuperseded,
		Note: "replaced by v2", Successor: "auth-v2", PostMutation: okHook,
	})
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}
	body, _ := os.ReadFile(path)
	for _, want := range []string{"**Status:** Superseded", "**Superseded By:** auth-v2", "## Resolution", "replaced by v2"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("missing %q in:\n%s", want, body)
		}
	}
}

func TestChangeStatus_DirectoryForm(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "spec", "plans", "auth")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(dir, "README.md")
	if err := os.WriteFile(path, []byte(planBody("Draft")), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	res, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "auth", To: lifecycle.PlanInReview, PostMutation: okHook,
	})
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}
	if res.From != lifecycle.PlanDraft {
		t.Errorf("from = %q", res.From)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "**Status:** In Review") {
		t.Errorf("directory-form plan not rewritten:\n%s", body)
	}
}

func TestChangeStatus_NotFound(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "spec", "plans"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "ghost", To: lifecycle.PlanApproved, PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.NotFound {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.NotFound, err)
	}
}

func TestChangeStatus_IllegalTransition(t *testing.T) {
	root, _ := stageFlatPlan(t, "auth", "Approved")
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "auth", To: lifecycle.PlanDraft, PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.InvalidState {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.InvalidState, err)
	}
	if !strings.Contains(err.Error(), "Approved") {
		t.Errorf("error should name current status: %q", err.Error())
	}
}

func TestChangeStatus_NoLegalTargetsMessage(t *testing.T) {
	// Rejected is terminal — no legal targets from it.
	root, _ := stageFlatPlan(t, "auth", "Rejected")
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "auth", To: lifecycle.PlanApproved, PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.InvalidState {
		t.Errorf("exit = %d, want %d", got, exitcode.InvalidState)
	}
	if !strings.Contains(err.Error(), "no legal targets") {
		t.Errorf("error should state no legal targets: %q", err.Error())
	}
}

func TestChangeStatus_NoStatusLine(t *testing.T) {
	root := t.TempDir()
	plansDir := filepath.Join(root, "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(plansDir, "auth.md")
	if err := os.WriteFile(path, []byte("# Plan: Auth\n\nNo status here.\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "auth", To: lifecycle.PlanApproved, PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.Unexpected, err)
	}
}

// TestChangeStatus_ReadStatusError covers the generic "reading plan status"
// branch: resolving <slug> to a *directory* named auth.md lets os.Stat
// succeed (so resolution picks it) but lifecycle.Validate's status read then
// fails with a non-ENOENT, non-ErrStatusLineNotFound os error (EISDIR).
func TestChangeStatus_ReadStatusError(t *testing.T) {
	root := t.TempDir()
	plansDir := filepath.Join(root, "spec", "plans")
	if err := os.MkdirAll(filepath.Join(plansDir, "auth.md"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "auth", To: lifecycle.PlanApproved, PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.Unexpected, err)
	}
	if !strings.Contains(err.Error(), "reading plan status") {
		t.Errorf("expected reading-plan-status error, got: %q", err.Error())
	}
}

// TestChangeStatus_RewriteError covers the Rewrite-failure branch: Validate
// reads the file fine, but a read-only plans directory makes Rewrite's atomic
// temp-write + rename fail.
func TestChangeStatus_RewriteError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("read-only directory is not enforced for root")
	}
	root, _ := stageFlatPlan(t, "auth", "Draft")
	plansDir := filepath.Join(root, "spec", "plans")
	if err := os.Chmod(plansDir, 0o555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(plansDir, 0o755) })

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "auth", To: lifecycle.PlanInReview, PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.Unexpected, err)
	}
	if !strings.Contains(err.Error(), "reading plan status") {
		t.Errorf("expected transaction error, got: %q", err.Error())
	}
}

func TestChangeStatus_PostMutationFails_RetainsCommittedTransaction(t *testing.T) {
	root, path := stageFlatPlan(t, "auth", "Approved")
	boom := errors.New("lint failed")
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "auth", To: lifecycle.PlanSuperseded,
		Note: "replaced", Successor: "auth-v2",
		PostMutation: func() error { return boom },
	})
	if !errors.Is(err, boom) {
		t.Fatalf("expected boom error, got %v", err)
	}
	body, _ := os.ReadFile(path)
	// The one-write artifact transaction is durable before derived work starts;
	// a callback failure is retained for explicit recovery rather than a split
	// rollback that could erase another writer.
	if !strings.Contains(string(body), "**Status:** Superseded") {
		t.Errorf("status not retained:\n%s", body)
	}
	if !strings.Contains(string(body), "## Resolution") || !strings.Contains(string(body), "Superseded By") {
		t.Errorf("transaction fields not retained:\n%s", body)
	}
}

func TestChangeStatus_SuccessorTransformSucceedsInOneWrite(t *testing.T) {
	root, path := stageFlatPlan(t, "auth", "Approved")
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "auth", To: lifecycle.PlanSuperseded,
		Note: "replaced", Successor: "auth-v2", PostMutation: okHook,
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "**Superseded By:** auth-v2") {
		t.Errorf("missing successor:\n%s", body)
	}
}

func TestChangeStatus_NoteTransformSucceedsInOneWrite(t *testing.T) {
	root, path := stageFlatPlan(t, "auth", "Approved")
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "auth", To: lifecycle.PlanWithdrawn,
		Note: "why", PostMutation: okHook,
	})
	if err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "## Resolution") {
		t.Errorf("missing note:\n%s", body)
	}
}

func TestChangeStatus_GuardErrors(t *testing.T) {
	cases := []struct {
		name string
		opts ChangeStatusOptions
		want int
	}{
		{"no-specroot", ChangeStatusOptions{Slug: "a", To: lifecycle.PlanApproved, PostMutation: okHook}, exitcode.Unexpected},
		{"no-slug", ChangeStatusOptions{SpecRoot: "/x", To: lifecycle.PlanApproved, PostMutation: okHook}, exitcode.InvalidArgs},
		{"no-to", ChangeStatusOptions{SpecRoot: "/x", Slug: "a", PostMutation: okHook}, exitcode.InvalidArgs},
		{"no-hook", ChangeStatusOptions{SpecRoot: "/x", Slug: "a", To: lifecycle.PlanApproved}, exitcode.Unexpected},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ChangeStatus(tc.opts)
			if got := codeOf(t, err); got != tc.want {
				t.Errorf("exit = %d, want %d", got, tc.want)
			}
		})
	}
}

// TestResolvePlanFile_FlatStatError covers the non-ENOENT stat branch on the
// flat path: making spec/plans itself a regular file makes a stat of
// spec/plans/auth.md fail with ENOTDIR (not IsNotExist).
func TestResolvePlanFile_FlatStatError(t *testing.T) {
	root := t.TempDir()
	specDir := filepath.Join(root, "spec")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// "plans" is a FILE where a directory is expected.
	if err := os.WriteFile(filepath.Join(specDir, "plans"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := resolvePlanFile(filepath.Join(specDir, "plans"), "auth")
	if got := codeOf(t, err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.Unexpected, err)
	}
}

// TestResolvePlanFile_DirStatError covers the non-ENOENT stat branch on the
// directory-form path: the flat file is absent (ENOENT), but spec/plans/auth
// is a regular file, so a stat of spec/plans/auth/README.md fails with
// ENOTDIR.
func TestResolvePlanFile_DirStatError(t *testing.T) {
	root := t.TempDir()
	plansDir := filepath.Join(root, "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// "auth" is a FILE; auth.md is absent. Stat(auth.md) → ENOENT;
	// Stat(auth/README.md) → ENOTDIR.
	if err := os.WriteFile(filepath.Join(plansDir, "auth"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := resolvePlanFile(plansDir, "auth")
	if got := codeOf(t, err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.Unexpected, err)
	}
}

func TestIsExecutionBandStatus(t *testing.T) {
	for _, s := range []lifecycle.Status{
		lifecycle.PlanExecuting, lifecycle.PlanBlocked, lifecycle.PlanImplemented, lifecycle.PlanFailed,
	} {
		if !IsExecutionBandStatus(s) {
			t.Errorf("%q should be execution-band", s)
		}
	}
	for _, s := range []lifecycle.Status{
		lifecycle.PlanDraft, lifecycle.PlanInReview, lifecycle.PlanApproved,
		lifecycle.PlanRejected, lifecycle.PlanWithdrawn, lifecycle.PlanSuperseded, lifecycle.PlanDeprecated,
	} {
		if IsExecutionBandStatus(s) {
			t.Errorf("%q should NOT be execution-band", s)
		}
	}
}

func TestLegalChangeStatusTargets(t *testing.T) {
	names := LegalChangeStatusTargetNames()
	want := map[string]bool{
		"Draft": true, "In Review": true, "Approved": true, "Rejected": true,
		"Withdrawn": true, "Superseded": true, "Deprecated": true,
	}
	if len(names) != len(want) {
		t.Fatalf("targets = %v, want %d entries", names, len(want))
	}
	for _, n := range names {
		if !want[n] {
			t.Errorf("unexpected target %q", n)
		}
		if !IsLegalChangeStatusTarget(lifecycle.Status(n)) {
			t.Errorf("IsLegalChangeStatusTarget(%q) = false", n)
		}
	}
	// Execution-band values are NOT legal targets even though they appear in
	// the matrix as disposition From-states.
	if IsLegalChangeStatusTarget(lifecycle.PlanExecuting) {
		t.Error("Executing must not be a legal change-status target")
	}
	// Sorted alphabetically.
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Errorf("targets not sorted: %v", names)
		}
	}
}

func TestLegalTransitionMatrix_Rendering(t *testing.T) {
	out := LegalTransitionMatrix()
	for _, want := range []string{"Legal transitions", "Draft", "In Review", "Approved", "lint --fix"} {
		if !strings.Contains(out, want) {
			t.Errorf("matrix missing %q:\n%s", want, out)
		}
	}
	// Execution-band states have no human From-arc except dispositions; they
	// appear as From-rows (e.g. Executing → Deprecated/Superseded/Withdrawn).
	if !strings.Contains(out, "Executing") {
		t.Errorf("matrix should list Executing as a disposition From-state:\n%s", out)
	}
}
