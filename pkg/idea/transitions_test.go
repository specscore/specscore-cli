package idea

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

// Each test below maps 1:1 to an AC in
// spec/features/cli/idea/change-status/README.md. The AC ID is named in
// the test name (snake-cased) and again in a Run-subtest comment when
// table-driven. Per-AC mapping:
//
//   TestChangeStatus_DraftToApprovedHappyPath        -> draft-to-approved-happy-path
//   TestChangeStatus_CaseInsensitiveToFlag           -> case-insensitive-to-flag
//   TestChangeStatus_IllegalTargetRejected           -> illegal-target-rejected
//   TestChangeStatus_AlreadyApprovedRejected         -> already-approved-rejected
//   TestChangeStatus_UnrecognizedToValueRejected     -> unrecognized-to-value-rejected
//   TestChangeStatus_MissingSlugRejected             -> missing-slug-rejected           (CLI-level, see internal/cli/idea_test.go)
//   TestChangeStatus_MissingToFlagRejected           -> missing-to-flag-rejected        (CLI-level, see internal/cli/idea_test.go)
//   TestChangeStatus_SlugNotFound                    -> slug-not-found
//   TestChangeStatus_LintFailureRollsBack            -> lint-failure-rolls-back
//
// Archival is no longer a change-status target — those ACs live with the
// `idea archive`/`idea unarchive` verbs (archive_test.go).

// noopLint is a PostMutationHook that always succeeds. Used for ACs
// that don't exercise the lint-failure path; we test the lint path
// end-to-end in TestChangeStatus_LintFailureRollsBack via a hook that
// returns an error.
func noopLint() error { return nil }

// failingLint returns a PostMutationHook that always returns the given
// error. Used to simulate an error-severity lint violation after the
// status rewrite.
func failingLint(e error) PostMutationHook {
	return func() error { return e }
}

// stageIdeaTree creates a minimal lint-clean spec tree at root and writes
// a single Idea file at spec/ideas/<slug>.md with the given status.
// Returns the project root.
//
// The function does NOT run lint — the index README only needs to be
// well-formed for the ChangeStatus paths these tests exercise. Tests
// that want lint integration use the failingLint hook to simulate that
// surface from the orchestrator's perspective.
func stageIdeaTree(t *testing.T, slug, status string) string {
	t.Helper()
	root := t.TempDir()
	ideasDir := filepath.Join(root, "spec", "ideas")
	if err := os.MkdirAll(filepath.Join(ideasDir, "archived"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Idea body. Use the scaffold so the file is lint-clean modulo
	// status, then patch the status line.
	body, err := Scaffold(ScaffoldOptions{Slug: slug, Status: status})
	if err != nil {
		t.Fatalf("scaffold: %v", err)
	}
	if err := os.WriteFile(filepath.Join(ideasDir, slug+".md"), body, 0o644); err != nil {
		t.Fatalf("write idea: %v", err)
	}
	// Minimal index README files. Real index sync happens via lint --fix
	// — these are well-formed placeholders sufficient for tests that
	// don't exercise the lint pass.
	idx := "# Ideas\n\n## Index\n\n| Idea | Status | Date | Owner | Promotes To |\n|------|--------|------|-------|-------------|\n\n_No active ideas yet._\n\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(filepath.Join(ideasDir, "README.md"), []byte(idx), 0o644); err != nil {
		t.Fatalf("write idx: %v", err)
	}
	arch := "# Archived\n\n_No archived ideas yet._\n\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(filepath.Join(ideasDir, "archived", "README.md"), []byte(arch), 0o644); err != nil {
		t.Fatalf("write archived idx: %v", err)
	}
	return root
}

// readIdea returns the file contents at spec/ideas/<slug>.md (active path).
func readIdea(t *testing.T, root, slug string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "spec", "ideas", slug+".md"))
	if err != nil {
		t.Fatalf("read idea: %v", err)
	}
	return string(b)
}

// assertExitCode unwraps an *exitcode.Error and asserts ExitCode() == want.
// Fails the test if err is nil or not an *exitcode.Error.
func assertExitCode(t *testing.T, err error, want int) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error with exit code %d, got nil", want)
	}
	type exitCoder interface{ ExitCode() int }
	var ec exitCoder
	if !errors.As(err, &ec) {
		t.Fatalf("error %v does not carry an ExitCode()", err)
	}
	if got := ec.ExitCode(); got != want {
		t.Fatalf("exit code = %d, want %d (err: %v)", got, want, err)
	}
}

// AC: draft-to-approved-happy-path
func TestChangeStatus_DraftToApprovedHappyPath(t *testing.T) {
	root := stageIdeaTree(t, "foo", "Draft")

	result, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot:     root,
		Slug:         "foo",
		To:           lifecycle.IdeaApproved,
		PostMutation: noopLint,
	})
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}
	if result.Slug != "foo" || result.From != lifecycle.IdeaDraft || result.To != lifecycle.IdeaApproved {
		t.Errorf("result = %+v; want {foo Draft Approved}", result)
	}
	body := readIdea(t, root, "foo")
	if !strings.Contains(body, "**Status:** Approved") {
		t.Errorf("status line not rewritten:\n%s", body)
	}
	if strings.Contains(body, "**Status:** Draft") {
		t.Errorf("old status line still present:\n%s", body)
	}
}

// AC: in-review-to-rejected-happy-path — change-status writes the terminal
// status in place; it never relocates a file (that is `idea archive`).
func TestChangeStatus_InReviewToRejectedHappyPath(t *testing.T) {
	root := stageIdeaTree(t, "foo", "In Review")

	result, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot:     root,
		Slug:         "foo",
		To:           lifecycle.IdeaRejected,
		PostMutation: noopLint,
	})
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}
	if result.From != lifecycle.IdeaInReview || result.To != lifecycle.IdeaRejected {
		t.Errorf("result = %+v; want from=In Review to=Rejected", result)
	}
	// File stays at the active path — change-status does not relocate.
	body := readIdea(t, root, "foo")
	if !strings.Contains(body, "**Status:** Rejected") {
		t.Errorf("status line not rewritten:\n%s", body)
	}
	if _, err := os.Stat(filepath.Join(root, "spec", "ideas", "archived", "foo.md")); !os.IsNotExist(err) {
		t.Errorf("change-status must not relocate the file: err=%v", err)
	}
}

// AC: case-insensitive-to-flag — testing through ParseStatus (CLI parses
// the flag value before reaching ChangeStatus). We verify that the
// canonical title-case value is what gets written when the lower/upper
// variant is passed via ParseStatus → ChangeStatus, AND that the
// canonical value is what's emitted in the result.
func TestChangeStatus_CaseInsensitiveToFlag(t *testing.T) {
	cases := []struct {
		input string
		want  lifecycle.Status
	}{
		{"approved", lifecycle.IdeaApproved},
		{"Approved", lifecycle.IdeaApproved},
		{"APPROVED", lifecycle.IdeaApproved},
	}
	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			root := stageIdeaTree(t, "foo", "Draft")

			to, ok := lifecycle.ParseStatus(lifecycle.KindIdea, c.input)
			if !ok || to != c.want {
				t.Fatalf("ParseStatus(%q) = (%q, %v); want (%q, true)", c.input, to, ok, c.want)
			}
			result, err := ChangeStatus(ChangeStatusOptions{
				SpecRoot:     root,
				Slug:         "foo",
				To:           to,
				PostMutation: noopLint,
			})
			if err != nil {
				t.Fatalf("ChangeStatus: %v", err)
			}
			// Result MUST carry the canonical title-case value, not the
			// input case.
			if result.To != c.want {
				t.Errorf("result.To = %q; want %q (input was %q)", result.To, c.want, c.input)
			}
			body := readIdea(t, root, "foo")
			if !strings.Contains(body, "**Status:** "+string(c.want)) {
				t.Errorf("canonical status not written for input %q:\n%s", c.input, body)
			}
		})
	}
}

// AC: illegal-target-rejected
func TestChangeStatus_IllegalTargetRejected(t *testing.T) {
	root := stageIdeaTree(t, "foo", "Draft")

	// `Implementing` is a recognized Idea status that is legal from
	// Specified, but NOT from Draft. The state-machine check returns
	// ErrInvalidTransition (exit 4) BEFORE any mutation.
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot:     root,
		Slug:         "foo",
		To:           lifecycle.IdeaImplementing,
		PostMutation: noopLint,
	})
	assertExitCode(t, err, exitcode.InvalidState)

	// Stderr message MUST name the current status (Draft) and the
	// legal targets from Draft (Approved, In Review, Stale).
	msg := err.Error()
	if !strings.Contains(msg, "Draft") {
		t.Errorf("error message missing current status %q: %s", "Draft", msg)
	}
	for _, want := range []string{"Approved", "In Review", "Stale"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error message missing legal target %q: %s", want, msg)
		}
	}

	// File MUST be unchanged.
	body := readIdea(t, root, "foo")
	if !strings.Contains(body, "**Status:** Draft") {
		t.Errorf("file should be unchanged on illegal transition:\n%s", body)
	}
}

// AC: already-approved-rejected — re-running on the target state is an
// illegal transition per the strict state-machine (REQ: not-idempotent).
func TestChangeStatus_AlreadyApprovedRejected(t *testing.T) {
	root := stageIdeaTree(t, "foo", "Approved")

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot:     root,
		Slug:         "foo",
		To:           lifecycle.IdeaApproved,
		PostMutation: noopLint,
	})
	assertExitCode(t, err, exitcode.InvalidState)

	// File MUST remain at Approved (no rewrite).
	body := readIdea(t, root, "foo")
	if !strings.Contains(body, "**Status:** Approved") {
		t.Errorf("file should remain at Approved:\n%s", body)
	}
}

// AC: unrecognized-to-value-rejected — testing the flag-parse layer.
// ParseStatus returns (_, false) for a bogus value; the cobra adapter
// turns that into exit 2 BEFORE invoking ChangeStatus. We assert both
// the ParseStatus rejection AND that IsLegalChangeStatusTarget rejects
// recognized-but-not-user-settable values (e.g., "Draft").
func TestChangeStatus_UnrecognizedToValueRejected(t *testing.T) {
	// Wholly unrecognized — even ParseStatus rejects.
	if _, ok := lifecycle.ParseStatus(lifecycle.KindIdea, "banana"); ok {
		t.Errorf("ParseStatus accepted bogus value %q", "banana")
	}

	// Recognized as an Idea status but not a user-facing --to target:
	// IsLegalChangeStatusTarget rejects so the cobra adapter exits 2.
	// "Draft" is the only pure source state with no incoming arcs (never
	// a To in the matrix).
	for _, raw := range []string{"draft"} {
		s, ok := lifecycle.ParseStatus(lifecycle.KindIdea, raw)
		if !ok {
			t.Fatalf("ParseStatus(%q) failed; expected recognition", raw)
		}
		if IsLegalChangeStatusTarget(s) {
			t.Errorf("IsLegalChangeStatusTarget(%q) should be false (not a user-facing target)", s)
		}
	}
	// Sanity — statuses that appear as To in the matrix are accepted.
	for _, raw := range []string{"in review", "approved", "rejected", "stale", "specifying", "specified", "implementing", "implemented"} {
		s, ok := lifecycle.ParseStatus(lifecycle.KindIdea, raw)
		if !ok {
			t.Fatalf("ParseStatus(%q) failed", raw)
		}
		if !IsLegalChangeStatusTarget(s) {
			t.Errorf("IsLegalChangeStatusTarget(%q) should be true", s)
		}
	}
}

// AC: slug-not-found — active path missing, including the case where
// an archived copy exists (archived MUST NOT satisfy the active
// lookup per REQ: slug-resolves-to-active-idea).
func TestChangeStatus_SlugNotFound(t *testing.T) {
	root := t.TempDir()
	// Create only the archived subtree, with an archived file at the
	// slug. Active path is intentionally absent.
	archDir := filepath.Join(root, "spec", "ideas", "archived")
	if err := os.MkdirAll(archDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(archDir, "nonexistent.md"),
		[]byte("# archived\n**Status:** Stale\n**Archived:** true\n"), 0o644); err != nil {
		t.Fatalf("write archived: %v", err)
	}

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot:     root,
		Slug:         "nonexistent",
		To:           lifecycle.IdeaApproved,
		PostMutation: noopLint,
	})
	assertExitCode(t, err, exitcode.NotFound)

	// Error message names the expected active path.
	wantPath := filepath.Join(root, "spec", "ideas", "nonexistent.md")
	if !strings.Contains(err.Error(), wantPath) {
		t.Errorf("error message missing expected path %q: %v", wantPath, err)
	}
}

// Sanity tests for the helpers ChangeStatus relies on.

func TestLegalTransitionMatrix_IncludesAllSources(t *testing.T) {
	m := LegalTransitionMatrix()
	// Must include the heading + table header.
	if !strings.Contains(m, "Legal transitions:") {
		t.Errorf("missing heading:\n%s", m)
	}
	if !strings.Contains(m, "From") || !strings.Contains(m, "To") {
		t.Errorf("missing table headers:\n%s", m)
	}
	// Every source status with ≥1 outgoing target MUST appear.
	for _, src := range []string{"Draft", "In Review", "Approved", "Specifying", "Specified", "Implementing"} {
		if !strings.Contains(m, src) {
			t.Errorf("matrix missing source %q:\n%s", src, m)
		}
	}
	// Must NOT include ANSI escape sequences.
	if strings.Contains(m, "\x1b[") {
		t.Errorf("matrix contains ANSI escapes:\n%s", m)
	}
}

// AC: no-status-line-transitions
func TestChangeStatus_NoStatusLineTransitions(t *testing.T) {
	root := t.TempDir()
	ideasDir := filepath.Join(root, "spec", "ideas")
	if err := os.MkdirAll(filepath.Join(ideasDir, "archived"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	// Write an idea file WITHOUT a **Status:** line
	body := "# Idea: No Status\n\nNo status line here.\n"
	if err := os.WriteFile(filepath.Join(ideasDir, "no-status.md"), []byte(body), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot:     root,
		Slug:         "no-status",
		To:           lifecycle.IdeaApproved,
		PostMutation: noopLint,
	})
	assertExitCode(t, err, exitcode.Unexpected)
	if !strings.Contains(err.Error(), "no **Status:** line") {
		t.Errorf("expected 'no **Status:** line' in error, got: %v", err)
	}
}

// AC: lint-failure-rolls-back — a lint failure simulated via a hook that
// returns an error AFTER the rewrite. The verb MUST exit 10 and restore
// the file with its original status.
func TestChangeStatus_LintFailureRollsBack(t *testing.T) {
	root := stageIdeaTree(t, "lint-fail", "Draft")
	simulatedErr := exitcode.UnexpectedErrorf("lint failed: oq-section error")

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot:     root,
		Slug:         "lint-fail",
		To:           lifecycle.IdeaApproved,
		PostMutation: failingLint(simulatedErr),
	})
	assertExitCode(t, err, exitcode.Unexpected)

	// File should be rolled back to Draft
	body := readIdea(t, root, "lint-fail")
	if !strings.Contains(body, "**Status:** Draft") {
		t.Errorf("status should be rolled back to Draft; got:\n%s", body)
	}
}

// AC: legal-targets-empty — test state with no outgoing transitions
// (Stale is terminal: no legal targets).
func TestChangeStatus_NoLegalTargets(t *testing.T) {
	root := stageIdeaTree(t, "stale-idea", "Stale")
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot:     root,
		Slug:         "stale-idea",
		To:           lifecycle.IdeaApproved,
		PostMutation: noopLint,
	})
	assertExitCode(t, err, exitcode.InvalidState)
	if !strings.Contains(err.Error(), "no legal targets") {
		t.Errorf("expected 'no legal targets' message, got: %v", err)
	}
}

func TestLegalChangeStatusTargetNames_Stable(t *testing.T) {
	got := LegalChangeStatusTargetNames()
	want := []string{"Approved", "Implemented", "Implementing", "In Review", "Rejected", "Specified", "Specifying", "Stale"}
	if fmt.Sprintf("%v", got) != fmt.Sprintf("%v", want) {
		t.Errorf("LegalChangeStatusTargetNames = %v; want %v", got, want)
	}
}

// Note plumbing: a positive transition with --note writes a `## Resolution`
// section into the body verbatim, atomically with the status rewrite.
func TestChangeStatus_NoteWritesResolution(t *testing.T) {
	root := stageIdeaTree(t, "baz", "In Review")

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot:     root,
		Slug:         "baz",
		To:           lifecycle.IdeaRejected,
		PostMutation: noopLint,
		Note:         "Turned down at the v2 review",
	})
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}
	body := readIdea(t, root, "baz")
	if !strings.Contains(body, "## Resolution") {
		t.Errorf("body missing ## Resolution section:\n%s", body)
	}
	if !strings.Contains(body, "Turned down at the v2 review") {
		t.Errorf("body missing note content verbatim:\n%s", body)
	}
}

// Without --note, no `## Resolution` section is written (today's behavior).
func TestChangeStatus_NoNoteNoResolution(t *testing.T) {
	root := stageIdeaTree(t, "baz", "In Review")

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot:     root,
		Slug:         "baz",
		To:           lifecycle.IdeaRejected,
		PostMutation: noopLint,
	})
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}
	if body := readIdea(t, root, "baz"); strings.Contains(body, "## Resolution") {
		t.Errorf("unexpected ## Resolution section without --note:\n%s", body)
	}
}

// A post-mutation failure after a note write rolls back BOTH the status line
// and the note body, leaving the active file byte-identical to pre-invocation.
func TestChangeStatus_NoteRollsBackOnFailure(t *testing.T) {
	root := stageIdeaTree(t, "baz", "Draft")
	before := readIdea(t, root, "baz")

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot:     root,
		Slug:         "baz",
		To:           lifecycle.IdeaApproved,
		PostMutation: failingLint(exitcode.UnexpectedErrorf("lint boom")),
		Note:         "this note must not survive rollback",
	})
	assertExitCode(t, err, 10)

	after := readIdea(t, root, "baz")
	if after != before {
		t.Errorf("rollback not byte-identical.\nbefore:\n%s\nafter:\n%s", before, after)
	}
	if strings.Contains(after, "## Resolution") {
		t.Errorf("## Resolution survived rollback:\n%s", after)
	}
}
