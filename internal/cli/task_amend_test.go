package cli

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

func TestTaskAmend_BoardReplaceAndClearPreservesLifecycleAndAudit(t *testing.T) {
	_, path := stageTaskWithStatus(t, "auth", "complete")
	seed := "**Implemented-by:** repo@abc1234\n**Note:** awaiting merge\n**Evidence:** old-pr\n"
	b, _ := os.ReadFile(path)
	if err := os.WriteFile(path, []byte(strings.Replace(string(b), "\nTask body.", "\n"+seed+"\nTask body.", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	now := func() time.Time { return time.Date(2026, 8, 12, 9, 30, 0, 0, time.UTC) }
	out, _, err := runTaskWithMutationDeps(t, taskMutationDeps{annotationAmendmentNow: now}, "amend", "auth", "--note", "merged", "--evidence", "main-sha, pr-140", "--actor", "codex", "--reason", "landed")
	if err != nil {
		t.Fatal(err)
	}
	if out != "auth: annotations amended\n" {
		t.Fatalf("stdout=%q", out)
	}
	after, _ := os.ReadFile(path)
	text := string(after)
	for _, want := range []string{"**Status:** complete", "**Implemented-by:** repo@abc1234", "**Note:** merged", "**Evidence:** main-sha, pr-140", "**Annotation Amendment:** actor=codex; at=2026-08-12T09:30:00Z; reason=landed; before_sha256=" + fmtDigest(before)} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q:\n%s", want, text)
		}
	}
	if strings.Count(text, "**Note:**") != 1 || strings.Count(text, "**Evidence:**") != 1 {
		t.Fatalf("annotations not singleton:\n%s", text)
	}
	if _, _, err := runTask(t, "amend", "auth", "--clear-note", "--clear-evidence", "--actor", "codex", "--reason", "obsolete"); err != nil {
		t.Fatal(err)
	}
	text = string(mustRead(path))
	if strings.Contains(text, "**Note:**") || strings.Contains(text, "**Evidence:**") || strings.Count(text, "**Annotation Amendment:**") != 2 {
		t.Fatalf("clear/audit failed:\n%s", text)
	}
}

func fmtDigest(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }

func mustRead(path string) []byte { b, _ := os.ReadFile(path); return b }

func TestTaskAmend_StatusAgnosticAndPlanDirectory(t *testing.T) {
	root, path := stagePlanWithTasks(t, "auth", twoTaskPlanBody)
	dir := filepath.Join(root, "spec", "plans", "auth")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	dirPath := filepath.Join(dir, "README.md")
	b, _ := os.ReadFile(path)
	if err := os.WriteFile(dirPath, b, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runTask(t, "amend", "setup", "--plan", "auth", "--note", "waiting", "--actor", "agent", "--reason", "clarify"); err != nil {
		t.Fatal(err)
	}
	got := string(mustRead(dirPath))
	if !strings.Contains(got, "**Status:** in_progress") || !strings.Contains(got, "**Note:** waiting") {
		t.Fatalf("directory plan amend failed:\n%s", got)
	}
}

func TestTaskAmend_RejectsDuplicateMalformedAndInvalidArgsWithoutWrites(t *testing.T) {
	_, path := stageTaskWithStatus(t, "auth", "blocked")
	b, _ := os.ReadFile(path)
	dirty := strings.Replace(string(b), "Task body.", "**Note:** one\n**Note:** two\nTask body.", 1)
	if err := os.WriteFile(path, []byte(dirty), 0o644); err != nil {
		t.Fatal(err)
	}
	before := string(mustRead(path))
	_, _, err := runTask(t, "amend", "auth", "--note", "truth", "--actor", "a", "--reason", "r")
	if exitCodeOfErr(err) != exitcode.InvalidArgs {
		t.Fatalf("duplicate exit=%v", err)
	}
	if string(mustRead(path)) != before {
		t.Fatal("duplicate mutated")
	}
	_, _, err = runTask(t, "amend", "auth", "--note", "", "--actor", "a", "--reason", "r")
	if exitCodeOfErr(err) != exitcode.InvalidArgs {
		t.Fatalf("blank exit=%v", err)
	}
	_, _, err = runTask(t, "amend", "auth", "--clear-note", "--actor", "", "--reason", "r")
	if exitCodeOfErr(err) != exitcode.InvalidArgs {
		t.Fatalf("actor exit=%v", err)
	}
}

func TestTaskAmend_AnnotationBeforeStatusIsFoundAndNeverDuplicated(t *testing.T) {
	_, path := stageTaskWithStatus(t, "auth", "blocked")
	seeded := strings.Replace(string(mustRead(path)), "**Status:** blocked", "**Note:** old\n**Evidence:** old-ref\n**Status:** blocked", 1)
	if err := os.WriteFile(path, []byte(seeded), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runTask(t, "amend", "auth", "--note", "new", "--clear-evidence", "--actor", "a", "--reason", "correct"); err != nil {
		t.Fatal(err)
	}
	got := string(mustRead(path))
	if strings.Count(got, "**Note:**") != 1 || !strings.Contains(got, "**Note:** new") || strings.Contains(got, "**Evidence:**") {
		t.Fatalf("pre-status annotations were not corrected as singletons:\n%s", got)
	}
}

func TestTaskAmend_RejectsPathTraversalSlugsWriteFree(t *testing.T) {
	root, path := stageTaskWithStatus(t, "auth", "blocked")
	before := string(mustRead(path))
	for _, args := range [][]string{
		{"amend", "../auth", "--note", "x", "--actor", "a", "--reason", "r"},
		{"amend", "auth", "--plan", "../release", "--note", "x", "--actor", "a", "--reason", "r", "--project", root},
	} {
		_, _, err := runTask(t, args...)
		if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
			t.Fatalf("args=%v exit=%d err=%v", args, got, err)
		}
	}
	if string(mustRead(path)) != before {
		t.Fatal("invalid slug mutated board task")
	}
}

func TestTaskAmend_FlagAndResolutionFailuresAreWriteFree(t *testing.T) {
	root, path := stageTaskWithStatus(t, "auth", "blocked")
	before := string(mustRead(path))
	cases := [][]string{
		{"--actor", "a", "--reason", "r"},
		{"--note", "x", "--clear-note", "--actor", "a", "--reason", "r"},
		{"--evidence", "x", "--clear-evidence", "--actor", "a", "--reason", "r"},
		{"--note", "x", "--actor", "a", "--reason", "bad\nreason"},
	}
	for _, args := range cases {
		_, _, err := runTask(t, append([]string{"amend", "auth"}, args...)...)
		if exitCodeOfErr(err) != exitcode.InvalidArgs {
			t.Fatalf("args %v: %v", args, err)
		}
	}
	if string(mustRead(path)) != before {
		t.Fatal("invalid args wrote task")
	}
	_, _, err := runTask(t, "amend", "ghost", "--note", "x", "--actor", "a", "--reason", "r")
	if exitCodeOfErr(err) != exitcode.NotFound {
		t.Fatalf("missing=%v", err)
	}
	flat := filepath.Join(root, "tasks", "flat.md")
	if err := os.WriteFile(flat, []byte("# Flat\n\n**Status:** blocked\n\nBody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err = runTask(t, "amend", "flat", "--note", "fixed", "--actor", "a", "--reason", "r"); err != nil {
		t.Fatal(err)
	}
}

func TestTaskAmend_MalformedStatusIsWriteFree(t *testing.T) {
	_, path := stageTaskWithStatus(t, "auth", "blocked")
	for _, tc := range []struct {
		body string
		code int
	}{
		{body: "# Auth\n\nNo status\n", code: exitcode.Unexpected},
		{body: "# Auth\n\n**Status:** blocked\n**Status:** blocked\n", code: exitcode.InvalidArgs},
	} {
		body := tc.body
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		before := string(mustRead(path))
		_, _, err := runTask(t, "amend", "auth", "--note", "fixed", "--actor", "a", "--reason", "r")
		if exitCodeOfErr(err) != tc.code {
			t.Fatalf("status body=%q err=%v", body, err)
		}
		if string(mustRead(path)) != before {
			t.Fatal("malformed status wrote")
		}
	}
}

func TestTaskAmend_PreservesNoFinalNewlineCRLFAndPlanSectionBoundary(t *testing.T) {
	_, path := stageTaskWithStatus(t, "auth", "blocked")
	body := "# Auth\r\n\r\n**Status:** blocked\r\n**Note:** stale"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runTask(t, "amend", "auth", "--note", "truth", "--actor", "a", "--reason", "r"); err != nil {
		t.Fatal(err)
	}
	got := string(mustRead(path))
	if strings.Contains(got, "\n") && !strings.Contains(got, "\r\n") {
		t.Fatalf("CRLF lost: %q", got)
	}
	if !strings.HasSuffix(got, "before_sha256="+fmtDigest([]byte(body))) {
		t.Fatalf("audit was not appended at EOF: %q", got)
	}
	planBody := twoTaskPlanBody + "\n## Open Questions\n\n**Status:** unrelated\n**Note:** preserve\n"
	_, planPath := stagePlanWithTasks(t, "auth", planBody)
	if _, _, err := runTask(t, "amend", "deploy", "--plan", "auth", "--note", "truth", "--actor", "a", "--reason", "r"); err != nil {
		t.Fatal(err)
	}
	got = string(mustRead(planPath))
	if !strings.Contains(got, "**Note:** preserve") || strings.Count(got, "**Status:** unrelated") != 1 {
		t.Fatalf("trailing section was touched: %s", got)
	}
}

func TestTaskAmend_PlanInlineFinalBlockPreservesNoFinalNewline(t *testing.T) {
	body := strings.TrimSuffix(twoTaskPlanBody, "\n")
	_, planPath := stagePlanWithTasks(t, "auth", body)
	before := mustRead(planPath)
	if _, _, err := runTask(t, "amend", "deploy", "--plan", "auth", "--note", "landed", "--actor", "a", "--reason", "r"); err != nil {
		t.Fatal(err)
	}
	got := string(mustRead(planPath))
	if strings.HasSuffix(got, "\n") {
		t.Fatalf("final newline was introduced: %q", got)
	}
	wantAudit := "**Annotation Amendment:** actor=a;"
	if !strings.HasSuffix(got, "before_sha256="+fmtDigest(before)) || !strings.Contains(got, wantAudit) {
		t.Fatalf("audit is not the exact final block line: %q", got)
	}
}

func TestTaskChangeStatus_ConcurrentRewriteReturnsConflict(t *testing.T) {
	_, path := stageTaskWithStatus(t, "auth", "planning")
	rewrite := func(string, lifecycle.Status, []string) (lifecycle.Status, error) {
		return "", lifecycle.ErrConcurrentMutation
	}
	_, _, err := runTaskWithMutationDeps(t, taskMutationDeps{rewriteBoardTask: rewrite}, "change-status", "auth", "--to=queued")
	if exitCodeOfErr(err) != exitcode.Conflict {
		t.Fatalf("board conflict=%v", err)
	}
	if got := taskFileStatus(t, path); got != "planning" {
		t.Fatalf("status=%s", got)
	}

}

func TestTaskChangeStatus_ConcurrentExtraFieldsReturnsConflict(t *testing.T) {
	_, path := stageTaskWithStatus(t, "auth", "in_progress")
	rewrite := func(string, lifecycle.Status, []string) (lifecycle.Status, error) {
		return "", lifecycle.ErrConcurrentMutation
	}
	_, _, err := runTaskWithMutationDeps(t, taskMutationDeps{rewriteBoardTask: rewrite}, "change-status", "auth", "--to=complete", "--note=landed")
	if exitCodeOfErr(err) != exitcode.Conflict {
		t.Fatalf("conflict=%v", err)
	}
	if got := taskFileStatus(t, path); got != "in_progress" {
		t.Fatalf("status=%s", got)
	}
}

// TestTaskWriterFenceMatrix proves every CLI writer of an existing Task body
// fails closed while another lifecycle transaction owns that artifact. New-task
// scaffolding is intentionally absent: it creates a new path, not a mutation
// of an existing Task artifact.
func TestTaskWriterFenceMatrix(t *testing.T) {
	_, boardPath := stageTaskWithStatus(t, "auth", "in_progress")
	_, planPath := stagePlanWithTasks(t, "auth", twoTaskPlanBody)

	cases := []struct {
		name  string
		path  string
		write func() error
	}{
		{
			name: "board status plus fields",
			path: boardPath,
			write: func() error {
				_, err := rewriteBoardTask(boardPath, lifecycle.TaskComplete, []string{"**Note:** landed"})
				return err
			},
		},
		{
			name: "board provenance amendment",
			path: boardPath,
			write: func() error {
				return amendBoardImplementedBy(boardPath, "repo@abc1234")
			},
		},
		{
			name: "board annotation amendment",
			path: boardPath,
			write: func() error {
				return lifecycle.TransformArtifact(boardPath, func(b []byte) ([]byte, error) { return append(b, 'x'), nil })
			},
		},
		{
			name: "plan inline status plus fields",
			path: planPath,
			write: func() error {
				return rewritePlanTaskStatusLine(planPath, 10, lifecycle.TaskComplete, []string{"**Note:** landed"})
			},
		},
		{
			name: "plan inline annotation amendment",
			path: planPath,
			write: func() error {
				return lifecycle.TransformArtifact(planPath, func(b []byte) ([]byte, error) { return append(b, 'x'), nil })
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := lifecycle.WithArtifactTransaction(tc.path, func(*lifecycle.ArtifactTransaction) error {
				if err := tc.write(); !errors.Is(err, lifecycle.ErrConcurrentMutation) {
					t.Fatalf("err=%v, want concurrent mutation", err)
				}
				return nil
			}); err != nil {
				t.Fatal(err)
			}
		})
	}
}
