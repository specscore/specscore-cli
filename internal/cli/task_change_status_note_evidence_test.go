package cli

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// taskFieldValue re-reads path and returns the trimmed value of the first
// line whose bold field name matches fieldName (e.g. "Note", "Evidence"), or
// "" when no such line is present.
func taskFieldValue(t *testing.T, path, fieldName string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	prefix := "**" + fieldName + ":**"
	for _, line := range strings.Split(string(data), "\n") {
		s := strings.TrimSpace(line)
		if strings.HasPrefix(s, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(s, prefix))
		}
	}
	return ""
}

// --- board mode ---

// --note alone, on a NON-complete transition: written even though provenance
// flags would be rejected here — --note is not restricted to --to=complete.
func TestTaskChangeStatus_Board_NoteOnNonCompleteTransition(t *testing.T) {
	_, taskFile := stageTaskWithStatus(t, "auth", "queued")
	stdout, _, err := runTask(t, "change-status", "auth", "--to=in_progress",
		"--note", "picking this up now")
	if err != nil {
		t.Fatalf("change-status: %v", err)
	}
	if want := "auth: queued → in_progress\n"; stdout != want {
		t.Errorf("stdout = %q; want %q", stdout, want)
	}
	if got := taskFieldValue(t, taskFile, "Note"); got != "picking this up now" {
		t.Errorf("note = %q; want %q", got, "picking this up now")
	}
}

func TestTaskChangeStatus_Board_AnnotationUpsertRemainsSingleton(t *testing.T) {
	_, taskFile := stageTaskWithStatus(t, "auth", "queued")
	before, _ := os.ReadFile(taskFile)
	seeded := strings.Replace(string(before), "**Status:** queued", "**Status:** queued\n**Note:** old", 1)
	if err := os.WriteFile(taskFile, []byte(seeded), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runTask(t, "change-status", "auth", "--to=in_progress", "--note", "new"); err != nil {
		t.Fatal(err)
	}
	got := string(mustRead(taskFile))
	if strings.Count(got, "**Note:**") != 1 || !strings.Contains(got, "**Note:** new") {
		t.Fatalf("annotation was not upserted as a singleton:\n%s", got)
	}
}

func TestTaskChangeStatus_Board_DuplicateSingletonRejectedWriteFree(t *testing.T) {
	_, taskFile := stageTaskWithStatus(t, "auth", "in_progress")
	before, _ := os.ReadFile(taskFile)
	seeded := strings.Replace(string(before), "**Status:** in_progress", "**Status:** in_progress\n**Note:** one\n**Note:** two", 1)
	if err := os.WriteFile(taskFile, []byte(seeded), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := runTask(t, "change-status", "auth", "--to=blocked", "--note", "new")
	if exitCodeOfErr(err) != exitcode.InvalidArgs {
		t.Fatalf("duplicate singleton err=%v", err)
	}
	if got := string(mustRead(taskFile)); got != seeded {
		t.Fatalf("duplicate singleton mutation was not write-free:\n%s", got)
	}
}

// --evidence alone, comma-separated refs, joined with ", " in the written field.
func TestTaskChangeStatus_Board_EvidenceMultipleRefs(t *testing.T) {
	_, taskFile := stageTaskWithStatus(t, "auth", "in_progress")
	_, _, err := runTask(t, "change-status", "auth", "--to=complete",
		"--evidence", "cfabf5e, https://chessraiders.com/board/ , deploy-log-42")
	if err != nil {
		t.Fatalf("change-status: %v", err)
	}
	want := "cfabf5e, https://chessraiders.com/board/, deploy-log-42"
	if got := taskFieldValue(t, taskFile, "Evidence"); got != want {
		t.Errorf("evidence = %q; want %q", got, want)
	}
}

// --note + --evidence + provenance together on completion: all three fields
// land in ONE atomic write, in the fixed order Implemented-by, Note, Evidence.
func TestTaskChangeStatus_Board_NoteEvidenceAndProvenanceOrdering(t *testing.T) {
	_, taskFile := stageTaskWithStatus(t, "auth", "in_progress")
	_, _, err := runTask(t, "change-status", "auth", "--to=complete",
		"--repo", "sneat-co/chess", "--commit", "cfabf5e",
		"--note", "verified live", "--evidence", "https://chessraiders.com/board/")
	if err != nil {
		t.Fatalf("change-status: %v", err)
	}
	if got := taskFieldValue(t, taskFile, "Implemented-by"); got != "sneat-co/chess@cfabf5e" {
		t.Errorf("implemented-by = %q", got)
	}
	if got := taskFieldValue(t, taskFile, "Note"); got != "verified live" {
		t.Errorf("note = %q", got)
	}
	if got := taskFieldValue(t, taskFile, "Evidence"); got != "https://chessraiders.com/board/" {
		t.Errorf("evidence = %q", got)
	}
	data, _ := os.ReadFile(taskFile)
	body := string(data)
	statusIdx := strings.Index(body, "**Status:** complete")
	implIdx := strings.Index(body, "**Implemented-by:**")
	noteIdx := strings.Index(body, "**Note:**")
	evidenceIdx := strings.Index(body, "**Evidence:**")
	if statusIdx >= implIdx || implIdx >= noteIdx || noteIdx >= evidenceIdx {
		t.Errorf("fields out of order (status=%d, implemented-by=%d, note=%d, evidence=%d):\n%s",
			statusIdx, implIdx, noteIdx, evidenceIdx, body)
	}
}

// Blank/whitespace-only --note and --evidence write nothing (same as omitting
// the flags entirely).
func TestTaskChangeStatus_Board_BlankNoteAndEvidenceWriteNothing(t *testing.T) {
	_, taskFile := stageTaskWithStatus(t, "auth", "in_progress")
	_, _, err := runTask(t, "change-status", "auth", "--to=complete",
		"--note", "   ", "--evidence", " , ,  ")
	if err != nil {
		t.Fatalf("change-status: %v", err)
	}
	if got := taskFieldValue(t, taskFile, "Note"); got != "" {
		t.Errorf("note = %q; want none", got)
	}
	if got := taskFieldValue(t, taskFile, "Evidence"); got != "" {
		t.Errorf("evidence = %q; want none", got)
	}
}

// An illegal transition rejects BEFORE any write — --note/--evidence must not
// leak onto an unchanged task.
func TestTaskChangeStatus_Board_NoteNotWrittenOnIllegalTransition(t *testing.T) {
	_, taskFile := stageTaskWithStatus(t, "auth", "planning")
	_, _, err := runTask(t, "change-status", "auth", "--to=complete", "--note", "should not land")
	if got := exitCodeOfErr(err); got != exitcode.InvalidState {
		t.Errorf("exit = %d, want %d (InvalidState); err=%v", got, exitcode.InvalidState, err)
	}
	if got := taskFieldValue(t, taskFile, "Note"); got != "" {
		t.Errorf("note leaked on rejected transition: %q", got)
	}
}

// --note/--evidence combined with --amend-provenance is rejected (exit 2); the
// task is left byte-unchanged.
func TestTaskChangeStatus_Board_NoteWithAmendProvenanceRejected(t *testing.T) {
	_, taskFile := stageTaskWithStatus(t, "auth", "complete")
	before, _ := os.ReadFile(taskFile)
	_, _, err := runTask(t, "change-status", "auth", "--amend-provenance",
		"--commit", "a1b2c3d", "--note", "not allowed here")
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit = %d, want %d (InvalidArgs); err=%v", got, exitcode.InvalidArgs, err)
	}
	after, _ := os.ReadFile(taskFile)
	if string(before) != string(after) {
		t.Errorf("task file changed despite rejection:\n%s", after)
	}
}

func TestTaskChangeStatus_Board_EvidenceWithAmendProvenanceRejected(t *testing.T) {
	_, taskFile := stageTaskWithStatus(t, "auth", "complete")
	before, _ := os.ReadFile(taskFile)
	_, _, err := runTask(t, "change-status", "auth", "--amend-provenance",
		"--commit", "a1b2c3d", "--evidence", "cfabf5e")
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit = %d, want %d (InvalidArgs); err=%v", got, exitcode.InvalidArgs, err)
	}
	after, _ := os.ReadFile(taskFile)
	if string(before) != string(after) {
		t.Errorf("task file changed despite rejection:\n%s", after)
	}
}

// --- plan-inline mode ---

// --note + --evidence on a plan-inline task land adjacent to that block's
// **Status:**, and the sibling block is byte-untouched.
func TestTaskChangeStatus_PlanInline_NoteAndEvidence(t *testing.T) {
	_, planPath := stagePlanWithTasks(t, "auth", twoTaskPlanBody)
	_, _, err := runTask(t, "change-status", "setup", "--plan", "auth", "--to=complete",
		"--commit", "cfabf5e", "--note", "verified live", "--evidence", "cfabf5e,https://chessraiders.com/board/")
	if err != nil {
		t.Fatalf("change-status: %v", err)
	}
	if got := planTaskStatus(t, planPath, "setup"); got != "complete" {
		t.Errorf("setup status = %q; want complete", got)
	}
	if got := taskFieldValue(t, planPath, "Note"); got != "verified live" {
		t.Errorf("note = %q", got)
	}
	if got := taskFieldValue(t, planPath, "Evidence"); got != "cfabf5e, https://chessraiders.com/board/" {
		t.Errorf("evidence = %q", got)
	}
	// Sibling block untouched.
	if got := planTaskStatus(t, planPath, "deploy"); got != "planning" {
		t.Errorf("deploy changed: %q", got)
	}
	if got := taskFieldValue(t, planPath, "Implemented-by"); got != "cfabf5e" {
		t.Errorf("implemented-by = %q", got)
	}
}

// --note on a plan-inline task is valid on transitions OTHER than complete
// (e.g. blocked), unlike provenance flags.
func TestTaskChangeStatus_PlanInline_NoteOnBlocked(t *testing.T) {
	_, planPath := stagePlanWithTasks(t, "auth", twoTaskPlanBody)
	_, _, err := runTask(t, "change-status", "setup", "--plan", "auth", "--to=blocked",
		"--note", "waiting on Firebase console access")
	if err != nil {
		t.Fatalf("change-status: %v", err)
	}
	if got := planTaskStatus(t, planPath, "setup"); got != "blocked" {
		t.Errorf("setup status = %q; want blocked", got)
	}
	if got := taskFieldValue(t, planPath, "Note"); got != "waiting on Firebase console access" {
		t.Errorf("note = %q", got)
	}
}

func TestTaskChangeStatus_PlanInline_AnnotationUpsertAndDuplicateRejection(t *testing.T) {
	t.Run("upsert", func(t *testing.T) {
		seeded := strings.Replace(twoTaskPlanBody, "**Status:** in_progress", "**Status:** in_progress\n**Note:** old", 1)
		_, planPath := stagePlanWithTasks(t, "auth", seeded)
		if _, _, err := runTask(t, "change-status", "setup", "--plan", "auth", "--to=blocked", "--note", "new"); err != nil {
			t.Fatal(err)
		}
		got := string(mustRead(planPath))
		if strings.Count(got, "**Note:**") != 1 || !strings.Contains(got, "**Note:** new") {
			t.Fatalf("plan annotation was not upserted:\n%s", got)
		}
	})
	t.Run("duplicate", func(t *testing.T) {
		seeded := strings.Replace(twoTaskPlanBody, "**Status:** in_progress", "**Status:** in_progress\n**Implemented-by:** one\n**Implemented-by:** two", 1)
		_, planPath := stagePlanWithTasks(t, "auth", seeded)
		before := mustRead(planPath)
		_, _, err := runTask(t, "change-status", "setup", "--plan", "auth", "--to=blocked")
		if exitCodeOfErr(err) != exitcode.InvalidArgs {
			t.Fatalf("duplicate provenance err=%v", err)
		}
		if got := mustRead(planPath); !bytes.Equal(got, before) {
			t.Fatal("duplicate plan singleton mutation was not write-free")
		}
	})
}

// --- chess-shaped fixture: 7 tasks, one completing with note+evidence+provenance ---

// sevenTaskPlanBody mirrors the shape of the real
// sneat-co/chess spec/plans/browser-play-surface.md motivating scenario: 7
// tasks, each **Id:**-addressed, all starting in planning.
const sevenTaskPlanBody = `# Plan: Playable browser multiplayer

**Status:** Approved
**Source Feature:** rts-multilayer-chess/browser-play-surface

## Tasks

### Task 1: Serve the live webapp at /board/

**Id:** task-1
**Depends-On:** —
**Status:** planning

Body 1.

### Task 2: Add email and Telegram sign-in

**Id:** task-2
**Depends-On:** —
**Status:** planning

Body 2.

### Task 3: Board-first lobby

**Id:** task-3
**Depends-On:** —
**Status:** planning

Body 3.

### Task 4: Results panel

**Id:** task-4
**Depends-On:** —
**Status:** planning

Body 4.

### Task 5: Forecast engine

**Id:** task-5
**Depends-On:** 1
**Status:** planning

Body 5.

### Task 6: Two-browser playability proof

**Id:** task-6
**Depends-On:** 1, 2, 3, 4, 5
**Status:** planning

Body 6.

### Task 7: Rollout

**Id:** task-7
**Depends-On:** 2, 6
**Status:** planning

Body 7.
`

// Acceptance scenario: Task 1 of 7 ships to production and is marked complete
// with evidence while the rest of the plan stays mid-execution. Mirrors the
// real chess browser-play-surface.md plan shape.
func TestTaskChangeStatus_PlanInline_SevenTaskFixture_Task1CompleteWithEvidence(t *testing.T) {
	_, planPath := stagePlanWithTasks(t, "browser-play-surface", sevenTaskPlanBody)

	// Tasks 2-6 move to in_progress (mid-execution); Task 7 stays planning.
	for _, id := range []string{"task-2", "task-3", "task-4", "task-5", "task-6"} {
		if _, _, err := runTask(t, "change-status", id, "--plan", "browser-play-surface", "--to=queued"); err != nil {
			t.Fatalf("%s → queued: %v", id, err)
		}
		if _, _, err := runTask(t, "change-status", id, "--plan", "browser-play-surface", "--to=in_progress"); err != nil {
			t.Fatalf("%s → in_progress: %v", id, err)
		}
	}

	// Task 1 ships: walk it through the matrix (planning → queued → in_progress
	// → complete) and mark complete with commit provenance + note + evidence.
	if _, _, err := runTask(t, "change-status", "task-1", "--plan", "browser-play-surface", "--to=queued"); err != nil {
		t.Fatalf("task-1 → queued: %v", err)
	}
	if _, _, err := runTask(t, "change-status", "task-1", "--plan", "browser-play-surface", "--to=in_progress"); err != nil {
		t.Fatalf("task-1 → in_progress: %v", err)
	}
	stdout, _, err := runTask(t, "change-status", "task-1", "--plan", "browser-play-surface", "--to=complete",
		"--repo", "sneat-co/chess", "--commit", "cfabf5e",
		"--note", "shipped to production, verified live",
		"--evidence", "cfabf5e,https://chessraiders.com/board/")
	if err != nil {
		t.Fatalf("task-1 → complete: %v", err)
	}
	if want := "task-1: in_progress → complete\n"; stdout != want {
		t.Errorf("stdout = %q; want %q", stdout, want)
	}

	if got := planTaskStatus(t, planPath, "task-1"); got != "complete" {
		t.Errorf("task-1 status = %q; want complete", got)
	}
	if got := planTaskStatus(t, planPath, "task-7"); got != "planning" {
		t.Errorf("task-7 status = %q; want planning (untouched)", got)
	}
	for _, id := range []string{"task-2", "task-3", "task-4", "task-5", "task-6"} {
		if got := planTaskStatus(t, planPath, id); got != "in_progress" {
			t.Errorf("%s status = %q; want in_progress", id, got)
		}
	}

	// `plan info` rollup reflects exactly 1 complete, 5 in_progress, 1 planning.
	stdout, stderr, err := runPlan(t, "info", "browser-play-surface", "--format=json")
	if err != nil {
		t.Fatalf("plan info: %v (stderr=%s)", err, stderr)
	}
	for _, want := range []string{`"total": 7`, `"complete": 1`, `"in_progress": 5`, `"planning": 1`, `"queued": 0`, `"blocked": 0`} {
		if !strings.Contains(stdout, want) {
			t.Errorf("plan info output missing %q:\n%s", want, stdout)
		}
	}
}

// A repeat completion attempt on the already-complete Task 1 (re-running
// change-status) is refused per the strict non-idempotent matrix — the note
// mechanism does not create a loophole to re-annotate a terminal task via a
// transition.
func TestTaskChangeStatus_PlanInline_SevenTaskFixture_RepeatCompleteRefused(t *testing.T) {
	_, planPath := stagePlanWithTasks(t, "browser-play-surface", sevenTaskPlanBody)
	if _, _, err := runTask(t, "change-status", "task-1", "--plan", "browser-play-surface", "--to=queued"); err != nil {
		t.Fatalf("task-1 → queued: %v", err)
	}
	if _, _, err := runTask(t, "change-status", "task-1", "--plan", "browser-play-surface", "--to=in_progress"); err != nil {
		t.Fatalf("task-1 → in_progress: %v", err)
	}
	if _, _, err := runTask(t, "change-status", "task-1", "--plan", "browser-play-surface",
		"--to=complete", "--commit", "cfabf5e"); err != nil {
		t.Fatalf("first completion: %v", err)
	}
	_, _, err := runTask(t, "change-status", "task-1", "--plan", "browser-play-surface",
		"--to=complete", "--note", "re-annotating")
	if got := exitCodeOfErr(err); got != exitcode.InvalidState {
		t.Errorf("exit = %d, want %d (InvalidState); err=%v", got, exitcode.InvalidState, err)
	}
	if got := taskFieldValue(t, planPath, "Note"); got != "" {
		t.Errorf("note leaked via rejected re-completion: %q", got)
	}
}
