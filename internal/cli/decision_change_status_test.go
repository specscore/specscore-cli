package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lint"
)

// stageDecisionCLIRoot bootstraps a fresh spec repo with a lint-clean minimal
// Features index (setupDecisionRoot creates spec/features/ but not its
// README, and the tree-wide lint pass stageDecisionInto runs also walks
// spec/features/ — mirrors stagePlan in plan_change_status_test.go) and sets
// the test's CWD to it.
func stageDecisionCLIRoot(t *testing.T) string {
	t.Helper()
	root := setupDecisionRoot(t)
	withCwd(t, root)
	featuresReadme := "# Features\n\n## Index\n\n| Feature | Status |\n|---------|--------|\n\n_No features yet._\n\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(filepath.Join(root, "spec", "features", "README.md"), []byte(featuresReadme), 0o644); err != nil {
		t.Fatalf("write features README: %v", err)
	}
	migrateTree(t, root)
	return root
}

// stageDecisionInto scaffolds a lint-clean decision via `decision new`
// (forcing the embedded scaffolder via --tags so the fixture is
// deterministic regardless of network reachability — see
// cli-template-runtime-fetch in internal/cli/decision.go) into an
// already-staged root (see stageDecisionCLIRoot), patches its **Status:** to
// the requested value, and returns its full on-disk slug. The tree is
// verified lint-clean afterward so a later rollback assertion fires only on
// the verb under test, not on an already-dirty fixture.
func stageDecisionInto(t *testing.T, root, bareSlug, status string) (fullSlug string) {
	t.Helper()
	if _, _, err := runDecision(t, "new", bareSlug, "--title", strings.ToUpper(bareSlug[:1])+bareSlug[1:], "--tags", "test"); err != nil {
		t.Fatalf("decision new: %v", err)
	}
	entries, err := os.ReadDir(filepath.Join(root, "spec", "decisions"))
	if err != nil {
		t.Fatalf("reading decisions dir: %v", err)
	}
	for _, e := range entries {
		if strings.HasSuffix(e.Name(), "-"+bareSlug+".md") {
			fullSlug = strings.TrimSuffix(e.Name(), ".md")
			break
		}
	}
	if fullSlug == "" {
		t.Fatalf("could not find scaffolded decision for slug %q", bareSlug)
	}

	path := filepath.Join(root, "spec", "decisions", fullSlug+".md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read decision: %v", err)
	}
	patched := strings.Replace(string(raw), "**Status:** Draft", "**Status:** "+status, 1)
	patched = strings.Replace(patched, "status: Draft", "status: "+status, 1)
	if err := os.WriteFile(path, []byte(patched), 0o644); err != nil {
		t.Fatalf("write patched decision: %v", err)
	}

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
	return fullSlug
}

// stageDecisionCLI is the single-decision convenience wrapper: a fresh root
// carrying exactly one staged decision. Tests that need a second decision in
// the SAME root (e.g. a supersession successor) call stageDecisionCLIRoot
// once and stageDecisionInto per decision instead.
func stageDecisionCLI(t *testing.T, bareSlug, status string) (root, fullSlug string) {
	t.Helper()
	root = stageDecisionCLIRoot(t)
	fullSlug = stageDecisionInto(t, root, bareSlug, status)
	return root, fullSlug
}

// --- Happy paths ---

// AC: draft-to-in-review-happy-path
func TestDecisionChangeStatus_DraftToInReview_CLI(t *testing.T) {
	root, slug := stageDecisionCLI(t, "auth", "Draft")
	stdout, stderr, err := runDecision(t, "change-status", slug, "--to=in review")
	if err != nil {
		t.Fatalf("change-status: %v (stderr=%s)", err, stderr)
	}
	if want := slug + ": Draft → In Review\n"; stdout != want {
		t.Errorf("stdout = %q; want %q", stdout, want)
	}
	body, _ := os.ReadFile(filepath.Join(root, "spec", "decisions", slug+".md"))
	if !strings.Contains(string(body), "**Status:** In Review") {
		t.Errorf("status not rewritten:\n%s", body)
	}
}

// AC: draft-to-approved-direct-happy-path
func TestDecisionChangeStatus_DraftToApprovedDirect_CLI(t *testing.T) {
	_, slug := stageDecisionCLI(t, "auth", "Draft")
	stdout, stderr, err := runDecision(t, "change-status", slug, "--to=approved")
	if err != nil {
		t.Fatalf("change-status: %v (stderr=%s)", err, stderr)
	}
	if want := slug + ": Draft → Approved\n"; stdout != want {
		t.Errorf("stdout = %q; want %q", stdout, want)
	}
}

// AC: in-review-to-approved-happy-path + case-insensitive
func TestDecisionChangeStatus_InReviewToApproved_CaseInsensitive_CLI(t *testing.T) {
	_, slug := stageDecisionCLI(t, "auth", "In Review")
	stdout, stderr, err := runDecision(t, "change-status", slug, "--to=APPROVED")
	if err != nil {
		t.Fatalf("change-status: %v (stderr=%s)", err, stderr)
	}
	if want := slug + ": In Review → Approved\n"; stdout != want {
		t.Errorf("stdout = %q; want %q", stdout, want)
	}
}

// AC: rejected-with-note-relocates
func TestDecisionChangeStatus_Rejected_CLI(t *testing.T) {
	root, slug := stageDecisionCLI(t, "auth", "In Review")
	_, stderr, err := runDecision(t, "change-status", slug, "--to=rejected", "--note", "no longer needed")
	if err != nil {
		t.Fatalf("change-status: %v (stderr=%s)", err, stderr)
	}
	if _, err := os.Stat(filepath.Join(root, "spec", "decisions", slug+".md")); !os.IsNotExist(err) {
		t.Errorf("active file should have relocated, stat err = %v", err)
	}
	body, err := os.ReadFile(filepath.Join(root, "spec", "decisions", "archived", slug+".md"))
	if err != nil {
		t.Fatalf("reading archived file: %v", err)
	}
	if !strings.Contains(string(body), "**Status:** Rejected") || !strings.Contains(string(body), "no longer needed") {
		t.Errorf("rejected/resolution not written:\n%s", body)
	}
	assertDecisionTreeLintClean(t, root)
}

// AC: supersession-full-round-trip — the atomic case: relocation, both
// index files, and BOTH directions of the **Supersedes:**/**Superseded By:**
// link, ending lint-clean.
func TestDecisionChangeStatus_Superseded_FullRoundTrip_CLI(t *testing.T) {
	root := stageDecisionCLIRoot(t)
	oldSlug := stageDecisionInto(t, root, "old-approach", "Approved")
	newSlug := stageDecisionInto(t, root, "new-approach", "Approved")

	stdout, stderr, err := runDecision(t, "change-status", oldSlug,
		"--to=superseded", "--note", "replaced by the new approach", "--successor", newSlug)
	if err != nil {
		t.Fatalf("change-status: %v (stderr=%s)", err, stderr)
	}
	if want := oldSlug + ": Approved → Superseded\n"; stdout != want {
		t.Errorf("stdout = %q; want %q", stdout, want)
	}

	oldBody, err := os.ReadFile(filepath.Join(root, "spec", "decisions", "archived", oldSlug+".md"))
	if err != nil {
		t.Fatalf("reading archived old decision: %v", err)
	}
	for _, want := range []string{"**Status:** Superseded", "**Superseded By:** " + newSlug, "replaced by the new approach"} {
		if !strings.Contains(string(oldBody), want) {
			t.Errorf("old decision missing %q:\n%s", want, oldBody)
		}
	}
	newBody, err := os.ReadFile(filepath.Join(root, "spec", "decisions", newSlug+".md"))
	if err != nil {
		t.Fatalf("reading successor: %v", err)
	}
	if !strings.Contains(string(newBody), "**Supersedes:** "+oldSlug) {
		t.Errorf("successor missing Supersedes link:\n%s", newBody)
	}

	activeIdx, _ := os.ReadFile(filepath.Join(root, "spec", "decisions", "README.md"))
	if strings.Contains(string(activeIdx), oldSlug) {
		t.Errorf("active index should no longer list the superseded decision:\n%s", activeIdx)
	}
	if !strings.Contains(string(activeIdx), newSlug) {
		t.Errorf("active index should still list the successor:\n%s", activeIdx)
	}
	archivedIdx, err := os.ReadFile(filepath.Join(root, "spec", "decisions", "archived", "README.md"))
	if err != nil {
		t.Fatalf("reading archived index: %v", err)
	}
	if !strings.Contains(string(archivedIdx), "["+oldSlug+"]("+oldSlug+".md) — Superseded — replaced by the new approach") {
		t.Errorf("archived index missing expected entry:\n%s", archivedIdx)
	}

	assertDecisionTreeLintClean(t, root)
}

// assertDecisionTreeLintClean fails the test if `specscore spec lint` reports
// any error-severity violation for the fixture tree.
func assertDecisionTreeLintClean(t *testing.T, root string) {
	t.Helper()
	vs, err := lint.Lint(lint.Options{SpecRoot: filepath.Join(root, "spec")})
	if err != nil {
		t.Fatalf("lint: %v", err)
	}
	for _, v := range vs {
		if v.Severity == "error" {
			t.Errorf("lint error after change-status: %s:%d [%s] %s", v.File, v.Line, v.Rule, v.Message)
		}
	}
}

// --- Error paths ---

// AC: unrecognized-to-value-rejected
func TestDecisionChangeStatus_InvalidStatus_CLI(t *testing.T) {
	_, slug := stageDecisionCLI(t, "auth", "Draft")
	_, _, err := runDecision(t, "change-status", slug, "--to=bogus")
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit = %d, want %d (InvalidArgs); err=%v", got, exitcode.InvalidArgs, err)
	}
}

// AC: unknown-slug-not-found
func TestDecisionChangeStatus_UnknownSlug_CLI(t *testing.T) {
	root := setupDecisionRoot(t)
	withCwd(t, root)
	_, errOut, err := runDecision(t, "change-status", "9999-ghost", "--to=approved")
	if got := exitCodeOfErr(err); got != exitcode.NotFound {
		t.Errorf("exit = %d, want %d (NotFound); err=%v", got, exitcode.NotFound, err)
	}
	if !strings.Contains(err.Error()+errOut, filepath.Join("spec", "decisions", "9999-ghost.md")) {
		t.Errorf("error should name expected path; got %v / %q", err, errOut)
	}
}

// AC: already-terminal-decision-no-legal-targets — a decision hand-placed at
// the active path with a disposition Status has no legal outgoing arcs. This
// fixture is deliberately NOT lint-clean (D-archived-location would flag a
// Rejected decision sitting outside archived/) — that's fine, since the verb
// under test rejects the transition at the state-machine check, before any
// lint pass runs, and never mutates the file.
func TestDecisionChangeStatus_AlreadyTerminal_NoLegalTargets_CLI(t *testing.T) {
	root := setupDecisionRoot(t)
	withCwd(t, root)
	if _, _, err := runDecision(t, "new", "auth", "--title", "Auth", "--tags", "test"); err != nil {
		t.Fatalf("decision new: %v", err)
	}
	path := filepath.Join(root, "spec", "decisions", "0001-auth.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read decision: %v", err)
	}
	patched := strings.Replace(string(raw), "**Status:** Draft", "**Status:** Rejected", 1)
	if err := os.WriteFile(path, []byte(patched), 0o644); err != nil {
		t.Fatalf("write patched decision: %v", err)
	}

	_, _, err = runDecision(t, "change-status", "0001-auth", "--to=approved")
	if got := exitCodeOfErr(err); got != exitcode.InvalidState {
		t.Errorf("exit = %d, want %d (InvalidState); err=%v", got, exitcode.InvalidState, err)
	}
}

// Once a disposition transition has actually relocated a decision, its slug
// no longer resolves at the active path — a second change-status call
// against the same slug is a NotFound, not an InvalidState.
func TestDecisionChangeStatus_AlreadyArchived_NotFound_CLI(t *testing.T) {
	root, slug := stageDecisionCLI(t, "auth", "Approved")
	if _, _, err := runDecision(t, "change-status", slug, "--to=deprecated", "--note", "retired"); err != nil {
		t.Fatalf("first change-status: %v", err)
	}
	withCwd(t, root)
	_, _, err := runDecision(t, "change-status", slug, "--to=superseded", "--note", "x", "--successor", "0099-nope")
	if got := exitCodeOfErr(err); got != exitcode.NotFound {
		t.Errorf("exit = %d, want %d (NotFound); err=%v", got, exitcode.NotFound, err)
	}
}

func TestDecisionChangeStatus_ArgErrors_CLI(t *testing.T) {
	stageDecisionCLI(t, "auth", "Approved")
	cases := []struct {
		name string
		args []string
		want int
	}{
		{"missing-slug", []string{"change-status", "--to=approved"}, exitcode.InvalidArgs},
		{"too-many-args", []string{"change-status", "0001-auth", "extra", "--to=approved"}, exitcode.InvalidArgs},
		{"bad-slug-bare", []string{"change-status", "auth", "--to=approved"}, exitcode.InvalidArgs},
		{"missing-to", []string{"change-status", "0001-auth"}, exitcode.InvalidArgs},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := runDecision(t, tc.args...)
			if got := exitCodeOfErr(err); got != tc.want {
				t.Errorf("exit = %d, want %d; err=%v", got, tc.want, err)
			}
		})
	}
}

// AC: reason-required-for-every-disposition
func TestDecisionChangeStatus_DispositionRequiresNote_CLI(t *testing.T) {
	for _, to := range []string{"rejected", "superseded", "deprecated"} {
		t.Run(to, func(t *testing.T) {
			_, slug := stageDecisionCLI(t, "auth", "Approved")
			args := []string{"change-status", slug, "--to=" + to}
			if to == "superseded" {
				args = append(args, "--successor", "0099-nope")
			}
			_, _, err := runDecision(t, args...)
			if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
				t.Errorf("exit = %d, want %d (InvalidArgs); err=%v", got, exitcode.InvalidArgs, err)
			}
			if !strings.Contains(err.Error(), "--note") {
				t.Errorf("error should mention --note: %v", err)
			}
		})
	}
}

// AC: successor-required-for-superseded
func TestDecisionChangeStatus_SupersededRequiresSuccessor_CLI(t *testing.T) {
	_, slug := stageDecisionCLI(t, "auth", "Approved")
	_, _, err := runDecision(t, "change-status", slug, "--to=superseded", "--note", "x")
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit = %d, want %d (InvalidArgs); err=%v", got, exitcode.InvalidArgs, err)
	}
	if !strings.Contains(err.Error(), "--successor") {
		t.Errorf("error should mention --successor: %v", err)
	}
}

// AC: successor-rejected-outside-superseded
func TestDecisionChangeStatus_SuccessorRejectedOutsideSuperseded_CLI(t *testing.T) {
	_, slug := stageDecisionCLI(t, "auth", "Draft")
	_, _, err := runDecision(t, "change-status", slug, "--to=approved", "--successor", "0002-x")
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit = %d, want %d (InvalidArgs); err=%v", got, exitcode.InvalidArgs, err)
	}
	if !strings.Contains(err.Error(), "--successor") {
		t.Errorf("error should mention --successor: %v", err)
	}
}

// Successor value that is not a well-formed full identifier is rejected at
// flag-validation time, before any mutation.
func TestDecisionChangeStatus_SuccessorBadFormat_CLI(t *testing.T) {
	_, slug := stageDecisionCLI(t, "auth", "Approved")
	_, _, err := runDecision(t, "change-status", slug, "--to=superseded", "--note", "x", "--successor", "not-a-full-slug")
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit = %d, want %d (InvalidArgs); err=%v", got, exitcode.InvalidArgs, err)
	}
}

// A successor slug that is well-formed but does not resolve to an existing
// decision surfaces InvalidArgs from pkg/decision, unchanged.
func TestDecisionChangeStatus_SuccessorNotFound_CLI(t *testing.T) {
	_, slug := stageDecisionCLI(t, "auth", "Approved")
	before, _ := os.ReadFile(filepath.Join(filepath.Dir(mustDecisionPath(t, slug)), slug+".md"))
	_, _, err := runDecision(t, "change-status", slug, "--to=superseded", "--note", "x", "--successor", "0099-ghost")
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit = %d, want %d (InvalidArgs); err=%v", got, exitcode.InvalidArgs, err)
	}
	after, _ := os.ReadFile(filepath.Join(filepath.Dir(mustDecisionPath(t, slug)), slug+".md"))
	if string(before) != string(after) {
		t.Errorf("decision should be untouched on a bad successor")
	}
}

// mustDecisionPath resolves the absolute path of the currently-staged
// decision fixture under the process's current working directory (set by
// stageDecisionCLI via withCwd), for before/after byte comparisons.
func mustDecisionPath(t *testing.T, slug string) string {
	t.Helper()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Join(cwd, "spec", "decisions", slug+".md")
}

func TestDecisionChangeStatus_ProjectFlagInvalid_CLI(t *testing.T) {
	_, _, err := runDecision(t, "change-status", "0001-auth", "--to=approved", "--project", "/nonexistent/path/that/does/not/exist")
	if err == nil {
		t.Fatal("expected error when --project doesn't exist")
	}
}

// A lint failure after the mutation rolls back the entire transition,
// including the relocation.
func TestDecisionChangeStatus_LintFailure_RollsBack_CLI(t *testing.T) {
	root, slug := stageDecisionCLI(t, "auth", "In Review")

	orig := lintLintFn
	lintLintFn = func(lint.Options) ([]lint.Violation, error) {
		return nil, errors.New("synthetic lint failure")
	}
	t.Cleanup(func() { lintLintFn = orig })

	_, _, err := runDecision(t, "change-status", slug, "--to=rejected", "--note", "x")
	if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d (Unexpected); err=%v", got, exitcode.Unexpected, err)
	}
	body, rerr := os.ReadFile(filepath.Join(root, "spec", "decisions", slug+".md"))
	if rerr != nil {
		t.Fatalf("active file should still exist after rollback: %v", rerr)
	}
	if !strings.Contains(string(body), "**Status:** In Review") {
		t.Errorf("status not rolled back:\n%s", body)
	}
	if _, statErr := os.Stat(filepath.Join(root, "spec", "decisions", "archived", slug+".md")); !os.IsNotExist(statErr) {
		t.Errorf("archived copy should not exist after rollback")
	}
}
