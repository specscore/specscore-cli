package decision

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

// decisionBody returns a minimal lint-shaped flat Decision body in the given
// status, with the given Supersedes/Superseded By field values (defaulting
// to "—" when empty).
func decisionBody(status, date, supersedes, supersededBy string) string {
	if date == "" {
		date = "2026-08-01"
	}
	if supersedes == "" {
		supersedes = "—"
	}
	if supersededBy == "" {
		supersededBy = "—"
	}
	return "---\nformat: " + FormatURL + "\nstatus: " + status + "\n---\n\n" +
		"# Decision: Test Decision\n\n" +
		"**Status:** " + status + "\n" +
		"**Date:** " + date + "\n" +
		"**Owner:** alex\n" +
		"**Tags:** —\n" +
		"**Source Idea:** —\n" +
		"**Supersedes:** " + supersedes + "\n" +
		"**Superseded By:** " + supersededBy + "\n\n" +
		"## Context\n\nC.\n\n" +
		"## Decision\n\nD.\n\n" +
		"## Rationale\n\nR.\n\n" +
		"## Declined Alternatives\n\n### Alt\n\nNo.\n\n" +
		"## Consequences at Decision Time\n\nC.\n\n" +
		"## Observed Consequences\n\nNone observed yet.\n\n" +
		"## Affected Features\n\nNone at this time.\n\n" +
		"---\n*This document follows the " + FormatURL + "*\n"
}

// activeIndexBody returns a minimal active decisions index carrying a row
// for each of the given (num, slug, title, status) tuples.
func activeIndexBody(rows ...[4]string) string {
	var sb strings.Builder
	sb.WriteString("# Decisions\n\n## Decisions\n\n")
	sb.WriteString("| # | Decision | Status | Date | Tags | Affected |\n")
	sb.WriteString("|---|----------|--------|------|------|----------|\n")
	for _, r := range rows {
		sb.WriteString("| [" + r[0] + "](" + r[1] + ".md) | " + r[2] + " | " + r[3] + " | 2026-08-01 | — | — |\n")
	}
	sb.WriteString("\n## Open Questions\n\nNone at this time.\n\n---\n*This document follows the https://specscore.md/decisions-index-specification*\n")
	return sb.String()
}

// stageDecision writes a flat decision at spec/decisions/<slug>.md under a
// fresh SpecRoot and returns (specRoot, decisionPath).
func stageDecision(t *testing.T, slug, status string) (string, string) {
	t.Helper()
	root := t.TempDir()
	decisionsDir := filepath.Join(root, "spec", "decisions")
	if err := os.MkdirAll(decisionsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(decisionsDir, slug+".md")
	if err := os.WriteFile(path, []byte(decisionBody(status, "", "", "")), 0o644); err != nil {
		t.Fatalf("write decision: %v", err)
	}
	return root, path
}

// stageDecisionWithIndex additionally writes an active decisions index row
// for slug, so tests can assert on index-sync behavior.
func stageDecisionWithIndex(t *testing.T, slug, status, num, title string) (root, path string) {
	t.Helper()
	root, path = stageDecision(t, slug, status)
	idxPath := filepath.Join(root, "spec", "decisions", "README.md")
	if err := os.WriteFile(idxPath, []byte(activeIndexBody([4]string{num, slug, title, status})), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
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

// --- Options validation ---

func TestChangeStatus_GuardErrors(t *testing.T) {
	cases := []struct {
		name string
		opts ChangeStatusOptions
		want int
	}{
		{"no-specroot", ChangeStatusOptions{Slug: "a", To: lifecycle.DecisionApproved, PostMutation: okHook}, exitcode.Unexpected},
		{"no-slug", ChangeStatusOptions{SpecRoot: "/x", To: lifecycle.DecisionApproved, PostMutation: okHook}, exitcode.InvalidArgs},
		{"no-to", ChangeStatusOptions{SpecRoot: "/x", Slug: "a", PostMutation: okHook}, exitcode.InvalidArgs},
		{"no-hook", ChangeStatusOptions{SpecRoot: "/x", Slug: "a", To: lifecycle.DecisionApproved}, exitcode.Unexpected},
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

// --- Happy paths, non-disposition ---

func TestChangeStatus_HappyPath_DraftToInReview(t *testing.T) {
	root, path := stageDecision(t, "0001-auth", "Draft")
	res, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "0001-auth", To: lifecycle.DecisionInReview, PostMutation: okHook,
	})
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}
	if res.From != lifecycle.DecisionDraft || res.To != lifecycle.DecisionInReview {
		t.Errorf("result = %+v", res)
	}
	if res.ArchivedPath != "" {
		t.Errorf("ArchivedPath should be empty for a non-disposition transition, got %q", res.ArchivedPath)
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
	root, _ := stageDecision(t, "0001-auth", "Draft")
	res, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "0001-auth", To: lifecycle.DecisionApproved, PostMutation: okHook,
	})
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}
	if res.From != lifecycle.DecisionDraft || res.To != lifecycle.DecisionApproved {
		t.Errorf("result = %+v", res)
	}
}

func TestChangeStatus_HappyPath_InReviewToDraft_RevisionsRequested(t *testing.T) {
	root, path := stageDecision(t, "0001-auth", "In Review")
	res, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "0001-auth", To: lifecycle.DecisionDraft, PostMutation: okHook,
	})
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}
	if res.From != lifecycle.DecisionInReview || res.To != lifecycle.DecisionDraft {
		t.Errorf("result = %+v", res)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "**Status:** Draft") {
		t.Errorf("status not rewritten:\n%s", body)
	}
}

func TestChangeStatus_HappyPath_InReviewToApproved(t *testing.T) {
	root, _ := stageDecision(t, "0001-auth", "In Review")
	res, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "0001-auth", To: lifecycle.DecisionApproved, PostMutation: okHook,
	})
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}
	if res.From != lifecycle.DecisionInReview {
		t.Errorf("from = %q", res.From)
	}
}

// --- Happy paths, dispositions (relocation + index sync) ---

func TestChangeStatus_InReviewToRejected_RelocatesAndSyncsIndexes(t *testing.T) {
	root, path := stageDecisionWithIndex(t, "0001-auth", "In Review", "0001", "Auth")
	res, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "0001-auth", To: lifecycle.DecisionRejected,
		Note: "no longer needed", PostMutation: okHook,
	})
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}
	archivedPath := filepath.Join(root, "spec", "decisions", "archived", "0001-auth.md")
	if res.ArchivedPath != archivedPath {
		t.Errorf("ArchivedPath = %q, want %q", res.ArchivedPath, archivedPath)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Errorf("active file should no longer exist, stat err = %v", err)
	}
	body, err := os.ReadFile(archivedPath)
	if err != nil {
		t.Fatalf("reading archived file: %v", err)
	}
	for _, want := range []string{"**Status:** Rejected", "## Resolution", "no longer needed"} {
		if !strings.Contains(string(body), want) {
			t.Errorf("missing %q in:\n%s", want, body)
		}
	}

	activeIdx, _ := os.ReadFile(filepath.Join(root, "spec", "decisions", "README.md"))
	if strings.Contains(string(activeIdx), "0001-auth") {
		t.Errorf("active index should no longer reference the relocated decision:\n%s", activeIdx)
	}
	archivedIdx, err := os.ReadFile(filepath.Join(root, "spec", "decisions", "archived", "README.md"))
	if err != nil {
		t.Fatalf("reading archived index: %v", err)
	}
	if !strings.Contains(string(archivedIdx), "- 2026-08-01 — [0001-auth](0001-auth.md) — Rejected — no longer needed") {
		t.Errorf("archived index missing expected entry:\n%s", archivedIdx)
	}
}

func TestChangeStatus_ApprovedToDeprecated_Relocates(t *testing.T) {
	root, _ := stageDecisionWithIndex(t, "0001-auth", "Approved", "0001", "Auth")
	res, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "0001-auth", To: lifecycle.DecisionDeprecated,
		Note: "no longer applies", PostMutation: okHook,
	})
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}
	if res.ArchivedPath == "" {
		t.Error("expected ArchivedPath to be set")
	}
	body, _ := os.ReadFile(res.ArchivedPath)
	if !strings.Contains(string(body), "**Status:** Deprecated") {
		t.Errorf("status not rewritten:\n%s", body)
	}
}

// TestChangeStatus_Superseded_FullRoundTrip is the atomic-supersession
// end-to-end case: relocation, both index surfaces, AND the bidirectional
// **Supersedes:**/**Superseded By:** link.
func TestChangeStatus_Superseded_FullRoundTrip(t *testing.T) {
	root, oldPath := stageDecisionWithIndex(t, "0001-old", "Approved", "0001", "Old Decision")
	decisionsDir := filepath.Join(root, "spec", "decisions")
	newPath := filepath.Join(decisionsDir, "0002-new.md")
	if err := os.WriteFile(newPath, []byte(decisionBody("Approved", "", "", "")), 0o644); err != nil {
		t.Fatalf("write successor: %v", err)
	}
	// Add the successor's row to the active index too.
	idxPath := filepath.Join(decisionsDir, "README.md")
	if err := os.WriteFile(idxPath, []byte(activeIndexBody(
		[4]string{"0001", "0001-old", "Old Decision", "Approved"},
		[4]string{"0002", "0002-new", "New Decision", "Approved"},
	)), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	res, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "0001-old", To: lifecycle.DecisionSuperseded,
		Note: "replaced by the new decision", Successor: "0002-new", PostMutation: okHook,
	})
	if err != nil {
		t.Fatalf("ChangeStatus: %v", err)
	}
	if res.From != lifecycle.DecisionApproved || res.To != lifecycle.DecisionSuperseded {
		t.Errorf("result = %+v", res)
	}

	archivedPath := filepath.Join(decisionsDir, "archived", "0001-old.md")
	if _, err := os.Stat(oldPath); !os.IsNotExist(err) {
		t.Errorf("old decision should have moved out of the active path: %v", err)
	}
	oldBody, err := os.ReadFile(archivedPath)
	if err != nil {
		t.Fatalf("reading archived old decision: %v", err)
	}
	if !strings.Contains(string(oldBody), "**Superseded By:** 0002-new") {
		t.Errorf("old decision missing Superseded By link:\n%s", oldBody)
	}
	if !strings.Contains(string(oldBody), "**Status:** Superseded") {
		t.Errorf("old decision status not rewritten:\n%s", oldBody)
	}

	newBody, err := os.ReadFile(newPath)
	if err != nil {
		t.Fatalf("reading successor: %v", err)
	}
	if !strings.Contains(string(newBody), "**Supersedes:** 0001-old") {
		t.Errorf("successor missing Supersedes link:\n%s", newBody)
	}

	activeIdx, _ := os.ReadFile(idxPath)
	if strings.Contains(string(activeIdx), "0001-old") {
		t.Errorf("active index should no longer list the superseded decision:\n%s", activeIdx)
	}
	if !strings.Contains(string(activeIdx), "0002-new") {
		t.Errorf("active index should still list the successor:\n%s", activeIdx)
	}
	archivedIdx, err := os.ReadFile(filepath.Join(decisionsDir, "archived", "README.md"))
	if err != nil {
		t.Fatalf("reading archived index: %v", err)
	}
	if !strings.Contains(string(archivedIdx), "[0001-old](0001-old.md) — Superseded — replaced by the new decision") {
		t.Errorf("archived index missing expected entry:\n%s", archivedIdx)
	}
}

// A second disposition transition (against a fresh decision) exercises the
// "entries already present, placeholder already replaced" branch of
// appendArchivedIndexEntry through the full ChangeStatus flow, and the
// stubCreated=false branch of EnsureArchivedIndexStub.
func TestChangeStatus_SecondDisposition_AppendsAlongsideExistingEntry(t *testing.T) {
	root, _ := stageDecisionWithIndex(t, "0001-first", "Approved", "0001", "First")
	decisionsDir := filepath.Join(root, "spec", "decisions")
	secondPath := filepath.Join(decisionsDir, "0002-second.md")
	if err := os.WriteFile(secondPath, []byte(decisionBody("Approved", "2026-08-02", "", "")), 0o644); err != nil {
		t.Fatalf("write second: %v", err)
	}
	idxPath := filepath.Join(decisionsDir, "README.md")
	if err := os.WriteFile(idxPath, []byte(activeIndexBody(
		[4]string{"0001", "0001-first", "First", "Approved"},
		[4]string{"0002", "0002-second", "Second", "Approved"},
	)), 0o644); err != nil {
		t.Fatalf("write index: %v", err)
	}

	if _, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "0001-first", To: lifecycle.DecisionDeprecated,
		Note: "first reason", PostMutation: okHook,
	}); err != nil {
		t.Fatalf("first ChangeStatus: %v", err)
	}
	if _, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "0002-second", To: lifecycle.DecisionDeprecated,
		Note: "second reason", PostMutation: okHook,
	}); err != nil {
		t.Fatalf("second ChangeStatus: %v", err)
	}

	archivedIdx, err := os.ReadFile(filepath.Join(decisionsDir, "archived", "README.md"))
	if err != nil {
		t.Fatalf("reading archived index: %v", err)
	}
	for _, want := range []string{
		"[0001-first](0001-first.md) — Deprecated — first reason",
		"[0002-second](0002-second.md) — Deprecated — second reason",
	} {
		if !strings.Contains(string(archivedIdx), want) {
			t.Errorf("missing %q in:\n%s", want, archivedIdx)
		}
	}
	if strings.Contains(string(archivedIdx), "_No archived decisions yet._") {
		t.Errorf("placeholder should have been replaced:\n%s", archivedIdx)
	}
}

// --- Error paths: slug resolution ---

func TestChangeStatus_NotFound(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "spec", "decisions"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "0001-ghost", To: lifecycle.DecisionApproved, PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.NotFound {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.NotFound, err)
	}
}

// A decision already relocated to archived/ is not found at the active
// path — the "already-terminal decision" case for a Decision-kind artifact
// specifically, since a disposition transition always moves the file.
func TestChangeStatus_AlreadyArchivedDecision_NotFoundAtActivePath(t *testing.T) {
	root, _ := stageDecisionWithIndex(t, "0001-old", "Approved", "0001", "Old")
	if _, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "0001-old", To: lifecycle.DecisionDeprecated,
		Note: "retired", PostMutation: okHook,
	}); err != nil {
		t.Fatalf("first ChangeStatus: %v", err)
	}
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "0001-old", To: lifecycle.DecisionSuperseded,
		Note: "x", Successor: "0002-new", PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.NotFound {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.NotFound, err)
	}
}

// TestChangeStatus_StatActivePathError_NonENOENT covers the generic
// "stat %s" branch for the initial slug resolution: making "spec/decisions"
// a regular file (not a directory) makes a stat of any path beneath it fail
// with ENOTDIR rather than ENOENT.
func TestChangeStatus_StatActivePathError_NonENOENT(t *testing.T) {
	root := t.TempDir()
	specDir := filepath.Join(root, "spec")
	if err := os.MkdirAll(specDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(specDir, "decisions"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "0001-auth", To: lifecycle.DecisionApproved, PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.Unexpected, err)
	}
}

// --- Error paths: state-machine validation ---

func TestChangeStatus_IllegalTransition_WithLegalTargets(t *testing.T) {
	root, _ := stageDecision(t, "0001-auth", "Draft")
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "0001-auth", To: lifecycle.DecisionSuperseded, PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.InvalidState {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.InvalidState, err)
	}
	if !strings.Contains(err.Error(), "Draft") {
		t.Errorf("error should name current status: %q", err.Error())
	}
}

// A decision hand-placed at the active path with a disposition Status has no
// legal targets — the terminal-status branch of the state machine.
func TestChangeStatus_NoLegalTargetsMessage(t *testing.T) {
	root, _ := stageDecision(t, "0001-auth", "Rejected")
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "0001-auth", To: lifecycle.DecisionApproved, PostMutation: okHook,
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
	decisionsDir := filepath.Join(root, "spec", "decisions")
	if err := os.MkdirAll(decisionsDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(decisionsDir, "0001-auth.md")
	if err := os.WriteFile(path, []byte("# Decision: Auth\n\nNo status here.\n"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "0001-auth", To: lifecycle.DecisionApproved, PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.Unexpected, err)
	}
	if !strings.Contains(err.Error(), "**Status:**") {
		t.Errorf("error should mention the missing Status line: %q", err.Error())
	}
}

// TestChangeStatus_ReadStatusError covers the generic "reading decision
// status" branch: resolving <slug> to a *directory* named 0001-auth.md lets
// os.Stat succeed (so resolution proceeds) but lifecycle.Validate's
// underlying read then fails with EISDIR (not ErrStatusLineNotFound).
func TestChangeStatus_ReadStatusError(t *testing.T) {
	root := t.TempDir()
	decisionsDir := filepath.Join(root, "spec", "decisions")
	if err := os.MkdirAll(filepath.Join(decisionsDir, "0001-auth.md"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "0001-auth", To: lifecycle.DecisionApproved, PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.Unexpected, err)
	}
	if !strings.Contains(err.Error(), "reading decision status") {
		t.Errorf("expected reading-decision-status error, got: %q", err.Error())
	}
}

// --- Error paths: archival collision + successor resolution (pre-mutation) ---

func TestChangeStatus_ArchiveCollision(t *testing.T) {
	root, _ := stageDecision(t, "0001-auth", "Approved")
	archivedDir := filepath.Join(root, "spec", "decisions", "archived")
	if err := os.MkdirAll(archivedDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(archivedDir, "0001-auth.md"), []byte("collision"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "0001-auth", To: lifecycle.DecisionDeprecated,
		Note: "x", PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.Conflict {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.Conflict, err)
	}
	// Untouched: still Approved at the active path.
	body, _ := os.ReadFile(filepath.Join(root, "spec", "decisions", "0001-auth.md"))
	if !strings.Contains(string(body), "**Status:** Approved") {
		t.Errorf("active decision should be untouched:\n%s", body)
	}
}

func TestChangeStatus_ArchiveCollisionStat_NonENOENT(t *testing.T) {
	root, _ := stageDecision(t, "0001-auth", "Approved")
	// Make "archived" a regular file so stat("archived/0001-auth.md") fails
	// with ENOTDIR rather than ENOENT.
	if err := os.WriteFile(filepath.Join(root, "spec", "decisions", "archived"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "0001-auth", To: lifecycle.DecisionDeprecated,
		Note: "x", PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.Unexpected, err)
	}
}

func TestChangeStatus_SuccessorNotFound(t *testing.T) {
	root, _ := stageDecision(t, "0001-old", "Approved")
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "0001-old", To: lifecycle.DecisionSuperseded,
		Note: "x", Successor: "0002-ghost", PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.InvalidArgs {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.InvalidArgs, err)
	}
	// Untouched.
	body, _ := os.ReadFile(filepath.Join(root, "spec", "decisions", "0001-old.md"))
	if !strings.Contains(string(body), "**Status:** Approved") {
		t.Errorf("decision should be untouched:\n%s", body)
	}
}

func TestChangeStatus_SuccessorStatError_NonENOENT(t *testing.T) {
	root, _ := stageDecision(t, "0001-old", "Approved")
	orig := osStatFn
	successorPath := filepath.Join(root, "spec", "decisions", "0002-new.md")
	osStatFn = func(name string) (os.FileInfo, error) {
		if name == successorPath {
			return nil, errors.New("synthetic stat failure")
		}
		return os.Stat(name)
	}
	t.Cleanup(func() { osStatFn = orig })

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "0001-old", To: lifecycle.DecisionSuperseded,
		Note: "x", Successor: "0002-new", PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.Unexpected, err)
	}
}

// --- Error paths: status rewrite ---

func TestChangeStatus_RewriteError(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("read-only directory is not enforced for root")
	}
	root, _ := stageDecision(t, "0001-auth", "Draft")
	decisionsDir := filepath.Join(root, "spec", "decisions")
	if err := os.Chmod(decisionsDir, 0o555); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(decisionsDir, 0o755) })

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "0001-auth", To: lifecycle.DecisionApproved, PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.Unexpected, err)
	}
	if !strings.Contains(err.Error(), "rewriting status line") {
		t.Errorf("expected rewrite error, got: %q", err.Error())
	}
}

// --- Error paths: bidirectional link writes (Superseded only) ---

func TestChangeStatus_SetSupersedesFails_RollsBack(t *testing.T) {
	root, path := stageDecision(t, "0001-old", "Approved")
	newPath := filepath.Join(root, "spec", "decisions", "0002-new.md")
	if err := os.WriteFile(newPath, []byte(decisionBody("Approved", "", "", "")), 0o644); err != nil {
		t.Fatalf("write successor: %v", err)
	}
	orig := setSupersedesFn
	setSupersedesFn = func(string, string) ([]byte, bool, error) {
		return nil, false, errors.New("supersedes write boom")
	}
	t.Cleanup(func() { setSupersedesFn = orig })

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "0001-old", To: lifecycle.DecisionSuperseded,
		Note: "x", Successor: "0002-new", PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d", got, exitcode.Unexpected)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "**Status:** Approved") {
		t.Errorf("status not rolled back:\n%s", body)
	}
}

func TestChangeStatus_SetSupersededByFails_RollsBack(t *testing.T) {
	root, path := stageDecision(t, "0001-old", "Approved")
	newPath := filepath.Join(root, "spec", "decisions", "0002-new.md")
	if err := os.WriteFile(newPath, []byte(decisionBody("Approved", "", "", "")), 0o644); err != nil {
		t.Fatalf("write successor: %v", err)
	}
	orig := setSupersededByFn
	setSupersededByFn = func(string, string) ([]byte, bool, error) {
		return nil, false, errors.New("superseded-by write boom")
	}
	t.Cleanup(func() { setSupersededByFn = orig })

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "0001-old", To: lifecycle.DecisionSuperseded,
		Note: "x", Successor: "0002-new", PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d", got, exitcode.Unexpected)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "**Status:** Approved") {
		t.Errorf("status not rolled back:\n%s", body)
	}
	newBody, _ := os.ReadFile(newPath)
	if strings.Contains(string(newBody), "**Supersedes:** 0001-old") {
		t.Errorf("successor's Supersedes write should have been rolled back:\n%s", newBody)
	}
}

// --- Error paths: note write ---

func TestChangeStatus_NoteWriteFails_RollsBack(t *testing.T) {
	root, path := stageDecision(t, "0001-auth", "In Review")
	orig := appendNoteFn
	appendNoteFn = func(string, string) ([]byte, bool, error) {
		return nil, false, errors.New("note write boom")
	}
	t.Cleanup(func() { appendNoteFn = orig })

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "0001-auth", To: lifecycle.DecisionRejected,
		Note: "why", PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d", got, exitcode.Unexpected)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "**Status:** In Review") {
		t.Errorf("status not rolled back after note-write failure:\n%s", body)
	}
}

// --- Error paths: relocation ---

func TestChangeStatus_ReadContentFails_RollsBack(t *testing.T) {
	root, path := stageDecision(t, "0001-auth", "In Review")
	orig := osReadFileFn
	osReadFileFn = func(name string) ([]byte, error) {
		if name == path {
			return nil, errors.New("read boom")
		}
		return os.ReadFile(name)
	}
	t.Cleanup(func() { osReadFileFn = orig })

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "0001-auth", To: lifecycle.DecisionRejected,
		Note: "x", PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d", got, exitcode.Unexpected)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "**Status:** In Review") {
		t.Errorf("status not rolled back:\n%s", body)
	}
}

func TestChangeStatus_MkdirArchivedFails_RollsBack(t *testing.T) {
	root, path := stageDecision(t, "0001-auth", "In Review")
	archivedDir := filepath.Join(root, "spec", "decisions", "archived")
	orig := osMkdirAllFn
	osMkdirAllFn = func(name string, perm os.FileMode) error {
		if name == archivedDir {
			return errors.New("mkdir boom")
		}
		return os.MkdirAll(name, perm)
	}
	t.Cleanup(func() { osMkdirAllFn = orig })

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "0001-auth", To: lifecycle.DecisionRejected,
		Note: "x", PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d", got, exitcode.Unexpected)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "**Status:** In Review") {
		t.Errorf("status not rolled back:\n%s", body)
	}
}

func TestChangeStatus_WriteArchivedCopyFails_RollsBack(t *testing.T) {
	root, path := stageDecision(t, "0001-auth", "In Review")
	archivedPath := filepath.Join(root, "spec", "decisions", "archived", "0001-auth.md")
	orig := osWriteFileFn
	osWriteFileFn = func(name string, data []byte, perm os.FileMode) error {
		if name == archivedPath {
			return errors.New("write boom")
		}
		return os.WriteFile(name, data, perm)
	}
	t.Cleanup(func() { osWriteFileFn = orig })

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "0001-auth", To: lifecycle.DecisionRejected,
		Note: "x", PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d", got, exitcode.Unexpected)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "**Status:** In Review") {
		t.Errorf("status not rolled back:\n%s", body)
	}
	if _, err := os.Stat(archivedPath); !os.IsNotExist(err) {
		t.Errorf("archived copy should not exist after rollback: %v", err)
	}
}

func TestChangeStatus_RemoveActivePathFails_RollsBack(t *testing.T) {
	root, path := stageDecision(t, "0001-auth", "In Review")
	orig := osRemoveFn
	osRemoveFn = func(name string) error {
		if name == path {
			return errors.New("remove boom")
		}
		return os.Remove(name)
	}
	t.Cleanup(func() { osRemoveFn = orig })

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "0001-auth", To: lifecycle.DecisionRejected,
		Note: "x", PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d", got, exitcode.Unexpected)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "**Status:** In Review") {
		t.Errorf("status not rolled back; active file should still hold the original status:\n%s", body)
	}
}

// --- Error paths: index sync ---

func TestChangeStatus_RemoveActiveRowFails_RollsBack(t *testing.T) {
	root, path := stageDecisionWithIndex(t, "0001-auth", "In Review", "0001", "Auth")
	orig := removeActiveRowFn
	removeActiveRowFn = func(string, string) ([]byte, bool, error) {
		return nil, false, errors.New("index remove boom")
	}
	t.Cleanup(func() { removeActiveRowFn = orig })

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "0001-auth", To: lifecycle.DecisionRejected,
		Note: "x", PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d", got, exitcode.Unexpected)
	}
	// Active file restored (relocation rolled back too).
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "**Status:** In Review") {
		t.Errorf("active file not restored:\n%s", body)
	}
}

func TestChangeStatus_EnsureArchivedStubFails_RollsBack(t *testing.T) {
	root, path := stageDecisionWithIndex(t, "0001-auth", "In Review", "0001", "Auth")
	orig := osMkdirAllFn
	archivedDir := filepath.Join(root, "spec", "decisions", "archived")
	osMkdirAllFn = func(name string, perm os.FileMode) error {
		if name == archivedDir {
			// Let the FIRST call (the relocation mkdir) succeed so we reach
			// EnsureArchivedIndexStub's own mkdir call, which we fail.
			calls++
			if calls > 1 {
				return errors.New("stub mkdir boom")
			}
		}
		return os.MkdirAll(name, perm)
	}
	t.Cleanup(func() { osMkdirAllFn = orig; calls = 0 })

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "0001-auth", To: lifecycle.DecisionRejected,
		Note: "x", PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.Unexpected, err)
	}
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "**Status:** In Review") {
		t.Errorf("active file not restored:\n%s", body)
	}
}

// calls is a shared counter for TestChangeStatus_EnsureArchivedStubFails_RollsBack.
var calls int

func TestChangeStatus_AppendArchivedRowFails_RollsBack(t *testing.T) {
	root, path := stageDecisionWithIndex(t, "0001-auth", "In Review", "0001", "Auth")
	orig := appendArchivedRowFn
	appendArchivedRowFn = func(string, string, string, string, string) ([]byte, error) {
		return nil, errors.New("archived append boom")
	}
	t.Cleanup(func() { appendArchivedRowFn = orig })

	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "0001-auth", To: lifecycle.DecisionRejected,
		Note: "x", PostMutation: okHook,
	})
	if got := codeOf(t, err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d; err=%v", got, exitcode.Unexpected, err)
	}
	// Full rollback: active file restored, active index restored, archived
	// stub either removed (if freshly created) or left as its pre-call state.
	body, _ := os.ReadFile(path)
	if !strings.Contains(string(body), "**Status:** In Review") {
		t.Errorf("active file not restored:\n%s", body)
	}
	activeIdx, _ := os.ReadFile(filepath.Join(root, "spec", "decisions", "README.md"))
	if !strings.Contains(string(activeIdx), "0001-auth") {
		t.Errorf("active index row not restored:\n%s", activeIdx)
	}
	if _, err := os.Stat(filepath.Join(root, "spec", "decisions", "archived", "README.md")); !os.IsNotExist(err) {
		t.Errorf("freshly-created archived stub should have been removed on rollback, stat err = %v", err)
	}
}

// --- PostMutation failure: full end-to-end rollback ---

func TestChangeStatus_PostMutationFails_RollsBack(t *testing.T) {
	root, oldPath := stageDecisionWithIndex(t, "0001-old", "Approved", "0001", "Old")
	newPath := filepath.Join(root, "spec", "decisions", "0002-new.md")
	if err := os.WriteFile(newPath, []byte(decisionBody("Approved", "", "", "")), 0o644); err != nil {
		t.Fatalf("write successor: %v", err)
	}
	boom := errors.New("lint failed")
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot: root, Slug: "0001-old", To: lifecycle.DecisionSuperseded,
		Note: "replaced", Successor: "0002-new",
		PostMutation: func() error { return boom },
	})
	if !errors.Is(err, boom) {
		t.Fatalf("expected boom error, got %v", err)
	}

	// Full rollback: file back at the active path, in its EXACT original form.
	body, err := os.ReadFile(oldPath)
	if err != nil {
		t.Fatalf("active file should exist again: %v", err)
	}
	if !strings.Contains(string(body), "**Status:** Approved") {
		t.Errorf("status not rolled back:\n%s", body)
	}
	if strings.Contains(string(body), "## Resolution") || strings.Contains(string(body), "**Superseded By:** 0002-new") {
		t.Errorf("body mutations not rolled back:\n%s", body)
	}
	if _, err := os.Stat(filepath.Join(root, "spec", "decisions", "archived", "0001-old.md")); !os.IsNotExist(err) {
		t.Errorf("archived copy should not exist after rollback")
	}
	newBody, _ := os.ReadFile(newPath)
	if strings.Contains(string(newBody), "**Supersedes:**") && !strings.Contains(string(newBody), "**Supersedes:** —") {
		t.Errorf("successor's Supersedes write not rolled back:\n%s", newBody)
	}
	activeIdx, _ := os.ReadFile(filepath.Join(root, "spec", "decisions", "README.md"))
	if !strings.Contains(string(activeIdx), "0001-old") {
		t.Errorf("active index row not restored:\n%s", activeIdx)
	}
}

// --- Pure helper functions ---

func TestValidateFullSlug(t *testing.T) {
	cases := []struct {
		slug    string
		wantErr bool
	}{
		{"0001-auth", false},
		{"0009-go-wasm-single-engine", false},
		{"", true},
		{"auth", true},         // bare slug, no number
		{"1-auth", true},       // number not zero-padded to 4 digits
		{"0001-Auth", true},    // uppercase
		{"0001-", true},        // trailing hyphen, empty slug segment
		{"0001-auth--x", true}, // double hyphen
	}
	for _, tc := range cases {
		err := ValidateFullSlug(tc.slug)
		if (err != nil) != tc.wantErr {
			t.Errorf("ValidateFullSlug(%q) error = %v, wantErr = %v", tc.slug, err, tc.wantErr)
		}
	}
}

func TestParseDateAndStatus(t *testing.T) {
	content := []byte(decisionBody("Rejected", "2026-08-05", "", ""))
	date, status := parseDateAndStatus(content)
	if date != "2026-08-05" || status != "Rejected" {
		t.Errorf("date=%q status=%q, want 2026-08-05/Rejected", date, status)
	}
}

func TestParseDateAndStatus_Defaults(t *testing.T) {
	date, status := parseDateAndStatus([]byte("# Decision: X\n\nNo fields.\n"))
	if date != "—" || status != "—" {
		t.Errorf("date=%q status=%q, want defaults", date, status)
	}
}

func TestBuildArchivedReason(t *testing.T) {
	got := buildArchivedReason("  replaced\nby   a newer\tdecision  ")
	want := "replaced by a newer decision"
	if got != want {
		t.Errorf("got %q, want %q", got, want)
	}
}

func TestIsDispositionStatus(t *testing.T) {
	for _, s := range []lifecycle.Status{lifecycle.DecisionRejected, lifecycle.DecisionSuperseded, lifecycle.DecisionDeprecated} {
		if !isDispositionStatus(s) {
			t.Errorf("%q should be a disposition status", s)
		}
	}
	for _, s := range []lifecycle.Status{lifecycle.DecisionDraft, lifecycle.DecisionInReview, lifecycle.DecisionApproved} {
		if isDispositionStatus(s) {
			t.Errorf("%q should NOT be a disposition status", s)
		}
	}
}

func TestLegalChangeStatusTargetNames(t *testing.T) {
	names := LegalChangeStatusTargetNames()
	want := map[string]bool{
		"Draft": true, "In Review": true, "Approved": true,
		"Rejected": true, "Superseded": true, "Deprecated": true,
	}
	if len(names) != len(want) {
		t.Fatalf("targets = %v, want %d entries", names, len(want))
	}
	for _, n := range names {
		if !want[n] {
			t.Errorf("unexpected target %q", n)
		}
	}
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Errorf("targets not sorted: %v", names)
		}
	}
}

func TestLegalTransitionMatrix_Rendering(t *testing.T) {
	out := LegalTransitionMatrix()
	for _, want := range []string{"Legal transitions", "Draft", "In Review", "Approved", "Rejected", "Superseded", "Deprecated"} {
		if !strings.Contains(out, want) {
			t.Errorf("matrix missing %q:\n%s", want, out)
		}
	}
}
