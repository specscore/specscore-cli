package lint

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// taskBoard builds a tasks/README.md board with one row per (slug, status)
// pair, using the exact 7-column shape task.RenderBoard/ParseBoard expect.
func taskBoard(rows ...[2]string) string {
	var b strings.Builder
	b.WriteString("# Tasks\n\n| Task | Status | Depends on | Branch | Agent | Requester | Time |\n|---|---|---|---|---|---|---|\n")
	emoji := map[string]string{
		"planning": "\U0001f4cb", "queued": "⏳", "in_progress": "\U0001f535",
		"blocked": "\U0001f7e1", "failed": "❌", "aborted": "⛔",
	}
	for _, r := range rows {
		slug, status := r[0], r[1]
		b.WriteString("| [" + slug + "](" + slug + "/) | " + emoji[status] + " `" + status + "` | — | — | — | — | — |\n")
	}
	return b.String()
}

// taskReadmeWithStatus builds a minimal task README carrying the given
// **Status:** line (or none, when status == "").
func taskReadmeWithStatus(title, status string) string {
	s := "# " + title + "\n\n"
	if status != "" {
		s += "**Status:** " + status + "\n\n"
	}
	s += "Body.\n\n## Dependencies\n\nNone\n\n## Summary\n\nNone\n"
	return s
}

// TestTaskIndex_CleanCase asserts a board whose Status cells already match
// their task READMEs produces zero violations, and --fix is a byte-level
// no-op.
func TestTaskIndex_CleanCase(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{})
	projectRoot := writeTaskBoard(t, specRoot, map[string]string{
		"README.md":         taskBoard([2]string{"auth", "queued"}, [2]string{"billing", "in_progress"}),
		"auth/README.md":    taskReadmeWithStatus("Auth", "queued"),
		"billing/README.md": taskReadmeWithStatus("Billing", "in_progress"),
	})

	vs, _, _ := taskIndexRules(projectRoot, specRoot, false)
	if len(vs) != 0 {
		t.Fatalf("expected 0 violations on clean board, got %d: %+v", len(vs), vs)
	}

	boardPath := filepath.Join(projectRoot, "tasks", "README.md")
	before, err := os.ReadFile(boardPath)
	if err != nil {
		t.Fatal(err)
	}
	vs, fixed, rc := taskIndexRules(projectRoot, specRoot, true)
	if len(vs) != 0 || fixed || len(rc) != 0 {
		t.Fatalf("expected no-op on clean board, got vs=%v fixed=%v rc=%v", vs, fixed, rc)
	}
	after, err := os.ReadFile(boardPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Fatalf("expected --fix to leave a clean board untouched")
	}
}

// TestTaskIndex_DriftReportedAndFixed proves the file wins: a task whose
// README status disagrees with its board row is reported in check mode and
// reconciled (board rewritten from the file, loudly, via a Reconciliation) in
// fix mode — this is the same-shape rule feature-index-row-sync/DI-row-
// content-sync already enforce for Features and Decisions, closing the gap
// `task change-status` deliberately leaves (it writes only the task file).
func TestTaskIndex_DriftReportedAndFixed(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{})
	projectRoot := writeTaskBoard(t, specRoot, map[string]string{
		"README.md":      taskBoard([2]string{"auth", "planning"}), // stale: board says planning
		"auth/README.md": taskReadmeWithStatus("Auth", "queued"),   // file says queued (after a real transition)
	})

	vs, _, _ := taskIndexRules(projectRoot, specRoot, false)
	if len(vs) != 1 {
		t.Fatalf("expected exactly 1 violation, got %d: %+v", len(vs), vs)
	}
	if vs[0].Rule != "task-index-row-sync" {
		t.Errorf("rule = %q, want task-index-row-sync", vs[0].Rule)
	}

	vs2, fixed, rc := taskIndexRules(projectRoot, specRoot, true)
	if len(vs2) != 0 || !fixed {
		t.Fatalf("expected a clean fix, got vs=%v fixed=%v", vs2, fixed)
	}
	if len(rc) != 1 {
		t.Fatalf("expected exactly 1 reconciliation, got %d: %+v", len(rc), rc)
	}
	if rc[0].Artifact != "auth" || len(rc[0].Changes) != 1 ||
		rc[0].Changes[0].Field != "status" || rc[0].Changes[0].IndexValue != "planning" || rc[0].Changes[0].FileValue != "queued" {
		t.Errorf("unexpected reconciliation: %+v", rc[0])
	}

	board := readFile(t, filepath.Join(projectRoot, "tasks", "README.md"))
	if !strings.Contains(board, "`queued`") || strings.Contains(board, "`planning`") {
		t.Fatalf("board row not rewritten to the file's status:\n%s", board)
	}

	// A second fix pass is a no-op — idempotent.
	vs3, fixed3, rc3 := taskIndexRules(projectRoot, specRoot, true)
	if len(vs3) != 0 || fixed3 || len(rc3) != 0 {
		t.Fatalf("expected idempotent second pass, got vs=%v fixed=%v rc=%v", vs3, fixed3, rc3)
	}
}

// TestTaskIndex_UnmigratedTaskIsSkipped proves a task file with no
// **Status:** line at all (not yet migrated) is left alone by this rule —
// that gap is specscore migrate's job, never task-index-row-sync's, so it
// must never invent or infer a value.
func TestTaskIndex_UnmigratedTaskIsSkipped(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{})
	projectRoot := writeTaskBoard(t, specRoot, map[string]string{
		"README.md":      taskBoard([2]string{"auth", "planning"}),
		"auth/README.md": taskReadmeWithStatus("Auth", ""), // no Status line
	})
	vs, fixed, rc := taskIndexRules(projectRoot, specRoot, true)
	if len(vs) != 0 || fixed || len(rc) != 0 {
		t.Fatalf("expected an unmigrated task to be left alone, got vs=%v fixed=%v rc=%v", vs, fixed, rc)
	}
}

// TestTaskIndex_UnrecognizedFileStatusIsSkipped proves a file status the
// legacy board vocabulary cannot represent (lifecycle's "complete" vs. the
// board's "completed" — the separately tracked unify-task-status-vocabulary
// gap) is skipped rather than written as a cell the board's own parser would
// then reject on the next read.
func TestTaskIndex_UnrecognizedFileStatusIsSkipped(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{})
	projectRoot := writeTaskBoard(t, specRoot, map[string]string{
		"README.md":      taskBoard([2]string{"auth", "queued"}),
		"auth/README.md": taskReadmeWithStatus("Auth", "complete"), // lifecycle vocabulary
	})
	vs, fixed, rc := taskIndexRules(projectRoot, specRoot, true)
	if len(vs) != 0 || fixed || len(rc) != 0 {
		t.Fatalf("expected an unrecognized file status to be left alone, got vs=%v fixed=%v rc=%v", vs, fixed, rc)
	}
	board := readFile(t, filepath.Join(projectRoot, "tasks", "README.md"))
	if !strings.Contains(board, "`queued`") {
		t.Fatalf("board must be left byte-unchanged when the file status is unrecognized:\n%s", board)
	}
}

// TestTaskIndex_OrphanRowIsSkipped proves a board row with no matching task
// directory is left alone (a different concern from row-sync).
func TestTaskIndex_OrphanRowIsSkipped(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{})
	projectRoot := writeTaskBoard(t, specRoot, map[string]string{
		"README.md": taskBoard([2]string{"ghost", "planning"}),
	})
	vs, fixed, rc := taskIndexRules(projectRoot, specRoot, true)
	if len(vs) != 0 || fixed || len(rc) != 0 {
		t.Fatalf("expected an orphan row to be left alone, got vs=%v fixed=%v rc=%v", vs, fixed, rc)
	}
}

// TestTaskIndex_NoTasksDirOrNoBoard proves the rule is a silent no-op absent
// a tasks/ directory or its board file, matching every other index rule's
// behavior when its target document doesn't exist.
func TestTaskIndex_NoTasksDirOrNoBoard(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{})
	if vs, fixed, rc := taskIndexRules(filepath.Dir(specRoot), specRoot, true); len(vs) != 0 || fixed || len(rc) != 0 {
		t.Fatalf("expected no-op with no tasks/ dir, got vs=%v fixed=%v rc=%v", vs, fixed, rc)
	}
	projectRoot := filepath.Dir(specRoot)
	if err := os.MkdirAll(filepath.Join(projectRoot, "tasks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if vs, fixed, rc := taskIndexRules(projectRoot, specRoot, true); len(vs) != 0 || fixed || len(rc) != 0 {
		t.Fatalf("expected no-op with a tasks/ dir but no board README, got vs=%v fixed=%v rc=%v", vs, fixed, rc)
	}
}

// TestTaskIndex_Severity is a direct call covering the trivial accessor
// (mirrors every other checker's severity() test in this package).
func TestTaskIndex_Severity(t *testing.T) {
	if got := newTaskIndexChecker("").severity(); got != "error" {
		t.Errorf("severity() = %q, want %q", got, "error")
	}
}

// TestTaskIndex_EmptyProjectRootDerivesFromSpecRoot covers the "" fallback:
// callers (e.g. a direct taskIndexRules invocation without a linter-supplied
// projectRoot) get filepath.Dir(specRoot), matching every other rule's
// lintProjectRoot("", specRoot) convention.
func TestTaskIndex_EmptyProjectRootDerivesFromSpecRoot(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{})
	projectRoot := writeTaskBoard(t, specRoot, map[string]string{
		"README.md":      taskBoard([2]string{"auth", "planning"}),
		"auth/README.md": taskReadmeWithStatus("Auth", "queued"),
	})
	_ = projectRoot
	vs, _, _ := taskIndexRules("", specRoot, false)
	if len(vs) != 1 {
		t.Fatalf("expected the derived projectRoot to find the board, got %d violations: %+v", len(vs), vs)
	}
}

// TestTaskIndex_MalformedBoardIsSkipped proves a board task.ParseBoard cannot
// parse (a different rule, board-format, owns that) is left alone rather than
// erroring this rule out.
func TestTaskIndex_MalformedBoardIsSkipped(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{})
	projectRoot := writeTaskBoard(t, specRoot, map[string]string{
		"README.md":      "# Tasks\n\nnot a table at all\n",
		"auth/README.md": taskReadmeWithStatus("Auth", "queued"),
	})
	vs, fixed, rc := taskIndexRules(projectRoot, specRoot, true)
	if len(vs) != 0 || fixed || len(rc) != 0 {
		t.Fatalf("expected a malformed board to be left alone, got vs=%v fixed=%v rc=%v", vs, fixed, rc)
	}
}

// TestTaskIndex_MalformedRowIsSkippedDuringRewrite mixes a well-formed
// drifted row with a malformed one (wrong cell count) directly in
// rewriteTaskBoardRows, proving the malformed row is left untouched rather
// than corrupting the table.
func TestTaskIndex_MalformedRowIsSkippedDuringRewrite(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "README.md")
	content := "# Tasks\n\n| Task | Status | Depends on | Branch | Agent | Requester | Time |\n|---|---|---|---|---|---|---|\n" +
		"| [auth](auth/) | ⏳ `queued` | — | — | — | — | — |\n" +
		"| not a real row |\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := rewriteTaskBoardRows(path, map[string]string{"auth": "in_progress"}); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, path)
	if !strings.Contains(got, "`in_progress`") {
		t.Fatalf("well-formed row not rewritten:\n%s", got)
	}
	if !strings.Contains(got, "| not a real row |") {
		t.Fatalf("malformed row must be preserved byte-for-byte:\n%s", got)
	}
}

// TestRewriteTaskBoardRows_ReadError covers the os.ReadFile error path for a
// nonexistent board.
func TestRewriteTaskBoardRows_ReadError(t *testing.T) {
	if err := rewriteTaskBoardRows(filepath.Join(t.TempDir(), "does-not-exist.md"), map[string]string{"auth": "queued"}); err == nil {
		t.Fatal("expected a read error for a nonexistent board")
	}
}

// TestTaskIndex_FixWriteFailureReportsViolation covers the rewrite-failure
// branch: the board file itself is made read-only so the fix's rewrite fails
// and the drift is reported as a violation instead of silently disappearing.
func TestTaskIndex_FixWriteFailureReportsViolation(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{})
	projectRoot := writeTaskBoard(t, specRoot, map[string]string{
		"README.md":      taskBoard([2]string{"auth", "planning"}),
		"auth/README.md": taskReadmeWithStatus("Auth", "queued"),
	})
	boardPath := filepath.Join(projectRoot, "tasks", "README.md")
	if err := os.Chmod(boardPath, 0o444); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(boardPath, 0o644) }()

	vs, fixed, rc := taskIndexRules(projectRoot, specRoot, true)
	if fixed || len(rc) != 0 {
		t.Fatalf("expected the write failure to prevent a clean fix, got fixed=%v rc=%v", fixed, rc)
	}
	if len(vs) != 1 || !strings.Contains(vs[0].Message, "fix failed") {
		t.Fatalf("expected a 'fix failed' violation, got %+v", vs)
	}
}

// TestTaskIndex_RegisteredWithLinter proves the rule runs through the
// standard specscore spec lint / --fix pipeline (not just its exported
// helper function), populating lint.Result.Reconciled the same way
// feature-index-row-sync does.
func TestTaskIndex_RegisteredWithLinter(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{})
	if err := os.MkdirAll(specRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	projectRoot := writeTaskBoard(t, specRoot, map[string]string{
		"README.md":      taskBoard([2]string{"auth", "planning"}),
		"auth/README.md": taskReadmeWithStatus("Auth", "queued"),
	})

	res, err := LintWithResult(Options{SpecRoot: specRoot, ProjectRoot: projectRoot, Fix: true, Severity: "error"})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, r := range res.Reconciled {
		if r.Rule == "task-index-row-sync" && r.Artifact == "auth" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected task-index-row-sync reconciliation in lint Result, got %+v", res.Reconciled)
	}
}
