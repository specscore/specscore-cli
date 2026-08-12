package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// stageTaskWithStatus creates a SpecScore project with a tasks/ board and a
// single board task whose tasks/<slug>/README.md carries a **Status:** line at
// the given status. It returns the project root (cwd is set to it) and the
// task-file path.
func stageTaskWithStatus(t *testing.T, slug, status string) (string, string) {
	t.Helper()
	root := t.TempDir()
	withCwd(t, root)

	_ = os.WriteFile(filepath.Join(root, "specscore.yaml"), []byte("name: test\n"), 0o644)

	tasksDir := filepath.Join(root, "tasks")
	_ = os.MkdirAll(tasksDir, 0o755)
	board := "# Tasks\n\n" +
		"| Task | Status | Depends on | Branch | Agent | Requester | Time |\n" +
		"|---|---|---|---|---|---|---|\n" +
		"| [" + slug + "](" + slug + "/) | \U0001f4cb `" + status + "` | — | — | — | — | — |\n"
	_ = os.WriteFile(filepath.Join(tasksDir, "README.md"), []byte(board), 0o644)

	taskDir := filepath.Join(tasksDir, slug)
	_ = os.MkdirAll(taskDir, 0o755)
	taskFile := filepath.Join(taskDir, "README.md")
	content := "# " + strings.ToUpper(slug[:1]) + slug[1:] + "\n\n" +
		"**Status:** " + status + "\n\n" +
		"Task body.\n\n## Dependencies\n\nNone\n\n## Summary\n\nNone\n"
	_ = os.WriteFile(taskFile, []byte(content), 0o644)

	return root, taskFile
}

func taskFileStatus(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read task file: %v", err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		s := strings.TrimSpace(line)
		if strings.HasPrefix(s, "**Status:**") {
			return strings.TrimSpace(strings.TrimPrefix(s, "**Status:**"))
		}
	}
	t.Fatalf("no **Status:** line in %s", path)
	return ""
}

// AC single-actor-no-coordination (happy path) + success-line output.
func TestTaskChangeStatus_LegalTransitions(t *testing.T) {
	cases := []struct {
		from, to string
		toFlag   string
	}{
		{"planning", "queued", "queued"},
		{"planning", "aborted", "aborted"},
		{"queued", "in_progress", "in_progress"},
		{"queued", "aborted", "aborted"},
		{"in_progress", "blocked", "blocked"},
		{"in_progress", "complete", "COMPLETE"}, // case-insensitive
		{"in_progress", "failed", "failed"},
		{"in_progress", "aborted", "aborted"},
		{"blocked", "in_progress", "in_progress"},
		{"blocked", "aborted", "aborted"},
	}
	for _, tc := range cases {
		t.Run(tc.from+"_to_"+tc.to, func(t *testing.T) {
			_, taskFile := stageTaskWithStatus(t, "auth", tc.from)
			stdout, stderr, err := runTask(t, "change-status", "auth", "--to="+tc.toFlag)
			if err != nil {
				t.Fatalf("change-status: %v (stderr=%s)", err, stderr)
			}
			want := "auth: " + tc.from + " → " + tc.to + "\n"
			if stdout != want {
				t.Errorf("stdout = %q; want %q", stdout, want)
			}
			if got := taskFileStatus(t, taskFile); got != tc.to {
				t.Errorf("task status = %q; want %q", got, tc.to)
			}
		})
	}
}

// AC illegal-transition-rejected: illegal / terminal pairs exit 4, non-idempotent.
func TestTaskChangeStatus_IllegalTransitions(t *testing.T) {
	cases := []struct {
		from, to string
	}{
		{"planning", "complete"},    // the AC's named example
		{"planning", "in_progress"}, // skips queued
		{"planning", "planning"},    // non-idempotent (self)
		{"queued", "complete"},
		{"queued", "blocked"},
		{"in_progress", "queued"},   // backwards
		{"blocked", "complete"},     // blocked only reaches in_progress/aborted
		{"complete", "in_progress"}, // terminal
		{"failed", "in_progress"},   // terminal
		{"aborted", "planning"},     // terminal
		{"complete", "complete"},    // terminal + self
	}
	for _, tc := range cases {
		t.Run(tc.from+"_to_"+tc.to, func(t *testing.T) {
			_, taskFile := stageTaskWithStatus(t, "auth", tc.from)
			_, _, err := runTask(t, "change-status", "auth", "--to="+tc.to)
			if got := exitCodeOfErr(err); got != exitcode.InvalidState {
				t.Errorf("exit = %d, want %d (InvalidState); err=%v", got, exitcode.InvalidState, err)
			}
			if got := taskFileStatus(t, taskFile); got != tc.from {
				t.Errorf("task changed on rejection: status = %q; want %q", got, tc.from)
			}
		})
	}
}

// AC to-flag-validation: missing --to exits 2 and leaves the task unchanged.
func TestTaskChangeStatus_MissingTo(t *testing.T) {
	_, taskFile := stageTaskWithStatus(t, "auth", "in_progress")
	_, _, err := runTask(t, "change-status", "auth")
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit = %d, want %d (InvalidArgs); err=%v", got, exitcode.InvalidArgs, err)
	}
	if !strings.Contains(err.Error(), "--to") {
		t.Errorf("error should name the missing flag: %v", err)
	}
	if got := taskFileStatus(t, taskFile); got != "in_progress" {
		t.Errorf("task changed: status = %q", got)
	}
}

// AC to-flag-validation: an unrecognized status value (not one of the seven)
// exits 2 and leaves the task unchanged.
func TestTaskChangeStatus_UnknownTo(t *testing.T) {
	_, taskFile := stageTaskWithStatus(t, "auth", "in_progress")
	_, _, err := runTask(t, "change-status", "auth", "--to=shipped")
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit = %d, want %d (InvalidArgs); err=%v", got, exitcode.InvalidArgs, err)
	}
	if !strings.Contains(err.Error(), "shipped") {
		t.Errorf("error should name the unrecognized value: %v", err)
	}
	if got := taskFileStatus(t, taskFile); got != "in_progress" {
		t.Errorf("task changed: status = %q", got)
	}
}

func TestTaskChangeStatus_MissingSlug(t *testing.T) {
	stageTaskWithStatus(t, "auth", "planning")
	_, _, err := runTask(t, "change-status", "--to=queued")
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit = %d, want %d (InvalidArgs); err=%v", got, exitcode.InvalidArgs, err)
	}
}

func TestTaskChangeStatus_TooManyArgs(t *testing.T) {
	stageTaskWithStatus(t, "auth", "planning")
	_, _, err := runTask(t, "change-status", "auth", "extra", "--to=queued")
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("exit = %d, want %d (InvalidArgs); err=%v", got, exitcode.InvalidArgs, err)
	}
}

func TestTaskChangeStatus_InvalidSlugsAreWriteFree(t *testing.T) {
	_, taskFile := stageTaskWithStatus(t, "auth", "planning")
	before, err := os.ReadFile(taskFile)
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"change-status", "../auth", "--to=queued"},
		{"change-status", "auth", "--plan=../escape", "--to=queued"},
	} {
		_, _, err := runTask(t, args...)
		if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
			t.Fatalf("args=%v exit=%d err=%v", args, got, err)
		}
	}
	after, err := os.ReadFile(taskFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(after, before) {
		t.Fatal("invalid slug mutated task")
	}
}

func TestTaskChangeStatus_TaskNotFound(t *testing.T) {
	stageTaskWithStatus(t, "auth", "planning")
	_, _, err := runTask(t, "change-status", "ghost", "--to=queued")
	if got := exitCodeOfErr(err); got != exitcode.NotFound {
		t.Errorf("exit = %d, want %d (NotFound); err=%v", got, exitcode.NotFound, err)
	}
}

// A task file with no **Status:** line surfaces an Unexpected (10) error.
func TestTaskChangeStatus_NoStatusLine(t *testing.T) {
	root, taskFile := stageTaskWithStatus(t, "auth", "planning")
	_ = root
	noStatus := "# Auth\n\nBody only.\n\n## Dependencies\n\nNone\n\n## Summary\n\nNone\n"
	if err := os.WriteFile(taskFile, []byte(noStatus), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _, err := runTask(t, "change-status", "auth", "--to=queued")
	if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d (Unexpected); err=%v", got, exitcode.Unexpected, err)
	}
}

// A post-validation Rewrite I/O failure surfaces an Unexpected (10) error. The
// task directory is made unwritable so the atomic temp-write inside Rewrite
// fails while Validate (a read) still succeeds.
func TestTaskChangeStatus_RewriteFailure(t *testing.T) {
	_, taskFile := stageTaskWithStatus(t, "auth", "planning")
	taskDir := filepath.Dir(taskFile)
	if err := os.Chmod(taskDir, 0o500); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(taskDir, 0o755) })

	_, _, err := runTask(t, "change-status", "auth", "--to=queued")
	if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d (Unexpected); err=%v", got, exitcode.Unexpected, err)
	}
}

// A --project that does not resolve to a spec repo surfaces the resolve error.
func TestTaskChangeStatus_ProjectResolveError(t *testing.T) {
	stageTaskWithStatus(t, "auth", "planning")
	bare := t.TempDir() // no specscore.yaml / tasks dir
	_, _, err := runTask(t, "change-status", "auth", "--to=queued", "--project", bare)
	if err == nil {
		t.Fatal("expected resolve error, got nil")
	}
}
