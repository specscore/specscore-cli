package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
	"github.com/specscore/specscore-cli/pkg/lint"
)

// stagePlan bootstraps a spec repo, scaffolds a lint-clean plan via `plan new`,
// patches its **Status:** to the requested value, runs lint --fix to sync the
// plans index, and returns the project root (cwd is set to it). The fixture is
// verified lint-clean before the test runs so retained post-mutation failure
// only on the verb under test.
func stagePlan(t *testing.T, slug, status string) string {
	t.Helper()
	root := setupSpecRoot(t)
	withCwd(t, root)
	if _, _, err := runPlan(t, "new", slug, "--owner", "tester"); err != nil {
		t.Fatalf("plan new: %v", err)
	}
	// Minimal lint-clean Features index (init's job; plan new does not write it).
	featuresReadme := "# Features\n\n## Index\n\n| Feature | Status |\n|---------|--------|\n\n_No features yet._\n\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(filepath.Join(root, "spec", "features", "README.md"), []byte(featuresReadme), 0o644); err != nil {
		t.Fatalf("write features README: %v", err)
	}

	path := filepath.Join(root, "spec", "plans", slug+".md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read plan: %v", err)
	}
	patched := strings.Replace(string(raw), "**Status:** Draft", "**Status:** "+status, 1)
	patched = strings.Replace(patched, "status: Draft", "status: "+status, 1)
	if err := os.WriteFile(path, []byte(patched), 0o644); err != nil {
		t.Fatalf("write patched plan: %v", err)
	}

	// Backfill frontmatter on the hand-written features index so the tree is
	// lint-clean before the verb under test runs.
	migrateTree(t, root)

	if _, err := lint.Lint(lint.Options{SpecRoot: filepath.Join(root, "spec"), Fix: true}); err != nil {
		t.Fatalf("initial lint --fix: %v", err)
	}
	vs, err := lint.Lint(lint.Options{SpecRoot: filepath.Join(root, "spec")})
	if err != nil {
		t.Fatalf("verify lint: %v", err)
	}
	for _, v := range vs {
		if v.Severity == "error" {
			t.Fatalf("pre-existing lint error in fixture: %s:%d [%s] %s", v.File, v.Line, v.Rule, v.Message)
		}
	}
	return root
}

// AC: draft-to-in-review-happy-path
func TestPlanChangeStatus_DraftToInReview_CLI(t *testing.T) {
	root := stagePlan(t, "auth", "Draft")
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

// AC: draft-to-approved-direct-happy-path
func TestPlanChangeStatus_DraftToApprovedDirect_CLI(t *testing.T) {
	root := stagePlan(t, "auth", "Draft")
	stdout, stderr, err := runPlan(t, "change-status", "auth", "--to=approved")
	if err != nil {
		t.Fatalf("change-status: %v (stderr=%s)", err, stderr)
	}
	if want := "auth: Draft → Approved\n"; stdout != want {
		t.Errorf("stdout = %q; want %q", stdout, want)
	}
	body, _ := os.ReadFile(filepath.Join(root, "spec", "plans", "auth.md"))
	if !strings.Contains(string(body), "**Status:** Approved") {
		t.Errorf("status not rewritten:\n%s", body)
	}
}

// AC: in-review-to-approved-happy-path + case-insensitive
func TestPlanChangeStatus_InReviewToApproved_CaseInsensitive_CLI(t *testing.T) {
	root := stagePlan(t, "auth", "In Review")
	stdout, stderr, err := runPlan(t, "change-status", "auth", "--to=APPROVED")
	if err != nil {
		t.Fatalf("change-status: %v (stderr=%s)", err, stderr)
	}
	if want := "auth: In Review → Approved\n"; stdout != want {
		t.Errorf("stdout = %q; want %q", stdout, want)
	}
	body, _ := os.ReadFile(filepath.Join(root, "spec", "plans", "auth.md"))
	if !strings.Contains(string(body), "**Status:** Approved") {
		t.Errorf("canonical title-case not written:\n%s", body)
	}
}

// AC: withdrawn-with-reason-writes-resolution
func TestPlanChangeStatus_Withdrawn_CLI(t *testing.T) {
	root := stagePlan(t, "auth", "Approved")
	_, stderr, err := runPlan(t, "change-status", "auth", "--to=withdrawn", "--note", "abandoned after pivot")
	if err != nil {
		t.Fatalf("change-status: %v (stderr=%s)", err, stderr)
	}
	body, _ := os.ReadFile(filepath.Join(root, "spec", "plans", "auth.md"))
	if !strings.Contains(string(body), "**Status:** Withdrawn") || !strings.Contains(string(body), "abandoned after pivot") {
		t.Errorf("withdrawn/resolution not written:\n%s", body)
	}
}

// AC: superseded-writes-successor-reference
func TestPlanChangeStatus_Superseded_CLI(t *testing.T) {
	root := stagePlan(t, "auth", "Approved")
	if _, _, err := runPlan(t, "new", "auth-v2", "--owner", "tester"); err != nil {
		t.Fatalf("plan new auth-v2: %v", err)
	}
	_, stderr, err := runPlan(t, "change-status", "auth", "--to=superseded", "--note", "replaced", "--successor", "auth-v2")
	if err != nil {
		t.Fatalf("change-status: %v (stderr=%s)", err, stderr)
	}
	body, _ := os.ReadFile(filepath.Join(root, "spec", "plans", "auth.md"))
	for _, want := range []string{"**Status:** Superseded", "**Superseded By:** auth-v2", "replaced"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("missing %q:\n%s", want, body)
		}
	}
}

func TestPlanChangeStatus_ArgErrors_CLI(t *testing.T) {
	stagePlan(t, "auth", "Approved")
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"missing-slug", []string{"change-status", "--to=approved"}, exitcode.InvalidArgs},
		{"too-many-args", []string{"change-status", "auth", "extra", "--to=approved"}, exitcode.InvalidArgs},
		{"bad-slug", []string{"change-status", "Bad_Slug", "--to=approved"}, exitcode.InvalidArgs},
		{"missing-to", []string{"change-status", "auth"}, exitcode.InvalidArgs},
		{"unrecognized-to", []string{"change-status", "auth", "--to=banana"}, exitcode.InvalidArgs},
		{"execution-band-to", []string{"change-status", "auth", "--to=executing"}, exitcode.InvalidArgs},
		{"withdrawn-no-note", []string{"change-status", "auth", "--to=withdrawn"}, exitcode.InvalidArgs},
		{"superseded-no-note", []string{"change-status", "auth", "--to=superseded"}, exitcode.InvalidArgs},
		{"superseded-no-successor", []string{"change-status", "auth", "--to=superseded", "--note", "x"}, exitcode.InvalidArgs},
		{"successor-on-non-superseded", []string{"change-status", "auth", "--to=deprecated", "--note", "x", "--successor", "auth"}, exitcode.InvalidArgs},
		{"illegal-transition", []string{"change-status", "auth", "--to=draft"}, exitcode.InvalidState},
		{"not-found", []string{"change-status", "ghost", "--to=approved"}, exitcode.NotFound},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := runPlan(t, tc.args...)
			if got := exitCodeOfErr(err); got != tc.want {
				t.Errorf("exit = %d, want %d; err=%v", got, tc.want, err)
			}
		})
	}
}

// AC: superseded-requires-successor — unresolvable successor exits 2.
func TestPlanChangeStatus_SupersededUnresolvableSuccessor_CLI(t *testing.T) {
	stagePlan(t, "auth", "Approved")
	_, _, err := runPlan(t, "change-status", "auth", "--to=superseded", "--note", "x", "--successor", "nope")
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.InvalidArgs, err)
	}
}

// AC: post-commit-callback-failure-is-recovery-required — derived lint failure
// retains the durable Plan transaction and exposes the original cause.
func TestPlanChangeStatus_LintFailureRetainsCommittedTransaction_CLI(t *testing.T) {
	root := stagePlan(t, "auth", "Draft")
	orig := lintLintFn
	lintLintFn = func(opts lint.Options) ([]lint.Violation, error) {
		if opts.Fix {
			return nil, nil
		}
		return []lint.Violation{{File: "x", Line: 1, Rule: "P-001", Severity: "error", Message: "boom"}}, nil
	}
	t.Cleanup(func() { lintLintFn = orig })

	_, _, err := runPlan(t, "change-status", "auth", "--to=in review")
	if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.Unexpected, err)
	}
	var committed *lifecycle.CommittedMutationError
	if !errors.As(err, &committed) || committed.Phase != "post-mutation callback" {
		t.Fatalf("error = %T %v, want committed callback failure", err, err)
	}
	body, _ := os.ReadFile(filepath.Join(root, "spec", "plans", "auth.md"))
	if !strings.Contains(string(body), "**Status:** In Review") {
		t.Errorf("committed status was not retained after lint failure:\n%s", body)
	}
}

// A --project that does not resolve to a spec repo surfaces the resolveSpecRoot
// error (covers the error-return branch after flag validation passes).
func TestPlanChangeStatus_ProjectResolveError_CLI(t *testing.T) {
	bare := t.TempDir() // no spec/ subtree
	_, _, err := runPlan(t, "change-status", "auth", "--to=approved", "--project", bare)
	if err == nil {
		t.Fatal("expected resolveSpecRoot error, got nil")
	}
}

// resolvePlanFile (CLI helper) resolves the directory form.
func TestResolvePlanFile_CLI_DirectoryForm(t *testing.T) {
	plansDir := setupPlansSpec(t)
	dir := filepath.Join(plansDir, "auth")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# Plan: Auth\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	got, err := resolvePlanFile(plansDir, "auth")
	if err != nil {
		t.Fatalf("resolvePlanFile: %v", err)
	}
	if got != filepath.Join(dir, "README.md") {
		t.Errorf("got %q", got)
	}
}

// resolvePlanFile (CLI helper) returns NotFound when neither form exists.
func TestResolvePlanFile_CLI_NotFound(t *testing.T) {
	plansDir := setupPlansSpec(t)
	_, err := resolvePlanFile(plansDir, "ghost")
	if got := exitCodeOfErr(err); got != exitcode.NotFound {
		t.Errorf("exit = %d, want %d", got, exitcode.NotFound)
	}
}
