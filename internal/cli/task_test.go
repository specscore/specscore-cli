package cli

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// setupTaskProject creates a minimal SpecScore project with a tasks/ directory,
// a board (tasks/README.md), and one individual task (tasks/setup/README.md).
func setupTaskProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	// specscore.yaml for root detection
	_ = os.WriteFile(filepath.Join(root, "specscore.yaml"), []byte("name: test\n"), 0o644)

	// tasks directory with board (7-column format with emoji+backtick status)
	tasksDir := filepath.Join(root, "tasks")
	_ = os.MkdirAll(tasksDir, 0o755)
	board := "# Tasks\n\n" +
		"| Task | Status | Depends on | Branch | Agent | Requester | Time |\n" +
		"|---|---|---|---|---|---|---|\n" +
		"| [setup](setup/) | \U0001f4cb `planning` | — | — | — | — | — |\n" +
		"| [deploy](deploy/) | ⏳ `queued` | setup | — | — | — | — |\n"
	_ = os.WriteFile(filepath.Join(tasksDir, "README.md"), []byte(board), 0o644)

	// Individual task file for "setup"
	setupDir := filepath.Join(tasksDir, "setup")
	_ = os.MkdirAll(setupDir, 0o755)
	setupFile := "# Setup\n\nInitial project setup.\n\n## Dependencies\n\nNone\n\n## Summary\n\nNone\n"
	_ = os.WriteFile(filepath.Join(setupDir, "README.md"), []byte(setupFile), 0o644)

	return root
}

func runTask(t *testing.T, args ...string) (string, string, error) {
	return runTaskWithMutationDeps(t, taskMutationDeps{}, args...)
}

func runTaskWithMutationDeps(t *testing.T, deps taskMutationDeps, args ...string) (string, string, error) {
	t.Helper()
	cmd := taskCommandWithDeps(deps)
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

// --- task list ---

func TestTaskList_YAML(t *testing.T) {
	root := setupTaskProject(t)
	withCwd(t, root)

	out, _, err := runTask(t, "list")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "setup") {
		t.Errorf("expected output to contain 'setup', got:\n%s", out)
	}
	if !strings.Contains(out, "deploy") {
		t.Errorf("expected output to contain 'deploy', got:\n%s", out)
	}
	if !strings.Contains(out, "planning") {
		t.Errorf("expected output to contain 'planning', got:\n%s", out)
	}
}

func TestTaskList_JSON(t *testing.T) {
	root := setupTaskProject(t)
	withCwd(t, root)

	out, _, err := runTask(t, "list", "--format=json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var rows []map[string]interface{}
	if err := json.Unmarshal([]byte(out), &rows); err != nil {
		t.Fatalf("invalid JSON output: %v\n%s", err, out)
	}
	if len(rows) != 2 {
		t.Errorf("expected 2 rows, got %d", len(rows))
	}
}

func TestTaskList_MD(t *testing.T) {
	root := setupTaskProject(t)
	withCwd(t, root)

	out, _, err := runTask(t, "list", "--format=md")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "| Task |") {
		t.Errorf("expected markdown table header, got:\n%s", out)
	}
	if !strings.Contains(out, "setup") {
		t.Errorf("expected output to contain 'setup', got:\n%s", out)
	}
}

func TestTaskList_StatusFilter(t *testing.T) {
	root := setupTaskProject(t)
	withCwd(t, root)

	out, _, err := runTask(t, "list", "--status=planning")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "setup") {
		t.Errorf("expected 'setup' in filtered output, got:\n%s", out)
	}
	if strings.Contains(out, "deploy") {
		t.Errorf("did not expect 'deploy' in planning-filtered output, got:\n%s", out)
	}
}

func TestTaskList_InvalidStatus(t *testing.T) {
	root := setupTaskProject(t)
	withCwd(t, root)

	_, _, err := runTask(t, "list", "--status=banana")
	if err == nil {
		t.Fatal("expected error for invalid status, got nil")
	}
	if !strings.Contains(err.Error(), "invalid status") {
		t.Errorf("expected 'invalid status' in error, got: %v", err)
	}
}

func TestTaskList_InvalidFormat(t *testing.T) {
	root := setupTaskProject(t)
	withCwd(t, root)

	_, _, err := runTask(t, "list", "--format=csv")
	if err == nil {
		t.Fatal("expected error for invalid format, got nil")
	}
	if !strings.Contains(err.Error(), "invalid format") {
		t.Errorf("expected 'invalid format' in error, got: %v", err)
	}
}

func TestTaskList_NoProject(t *testing.T) {
	// Use an empty temp dir with no specscore.yaml
	empty := t.TempDir()
	withCwd(t, empty)

	_, _, err := runTask(t, "list")
	if err == nil {
		t.Fatal("expected error when no project root found, got nil")
	}
}

func TestTaskList_NoTasksDir(t *testing.T) {
	// Project root exists (specscore.yaml) but no tasks/ directory
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "specscore.yaml"), []byte("name: test\n"), 0o644)
	withCwd(t, root)

	_, _, err := runTask(t, "list")
	if err == nil {
		t.Fatal("expected error when tasks/ dir missing, got nil")
	}
	if !strings.Contains(err.Error(), "tasks directory not found") {
		t.Errorf("expected 'tasks directory not found' in error, got: %v", err)
	}
}

// --- task info ---

func TestTaskInfo_YAML(t *testing.T) {
	root := setupTaskProject(t)
	withCwd(t, root)

	out, _, err := runTask(t, "info", "--task=setup")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "slug: setup") {
		t.Errorf("expected 'slug: setup' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "title: Setup") {
		t.Errorf("expected 'title: Setup' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "status: planning") {
		t.Errorf("expected 'status: planning' in output, got:\n%s", out)
	}
}

func TestTaskInfo_JSON(t *testing.T) {
	root := setupTaskProject(t)
	withCwd(t, root)

	out, _, err := runTask(t, "info", "--task=setup", "--format=json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var info map[string]interface{}
	if err := json.Unmarshal([]byte(out), &info); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if info["slug"] != "setup" {
		t.Errorf("expected slug=setup, got %v", info["slug"])
	}
	if info["status"] != "planning" {
		t.Errorf("expected status=planning, got %v", info["status"])
	}
}

func TestTaskInfo_MissingFlag(t *testing.T) {
	root := setupTaskProject(t)
	withCwd(t, root)

	_, _, err := runTask(t, "info")
	if err == nil {
		t.Fatal("expected error for missing --task flag, got nil")
	}
	if !strings.Contains(err.Error(), "missing required flag: --task") {
		t.Errorf("expected 'missing required flag: --task' in error, got: %v", err)
	}
}

func TestTaskInfo_NotFound(t *testing.T) {
	root := setupTaskProject(t)
	withCwd(t, root)

	_, _, err := runTask(t, "info", "--task=nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent task, got nil")
	}
	if !strings.Contains(err.Error(), "task not found") {
		t.Errorf("expected 'task not found' in error, got: %v", err)
	}
}

func TestTaskInfo_InvalidFormat(t *testing.T) {
	root := setupTaskProject(t)
	withCwd(t, root)

	_, _, err := runTask(t, "info", "--task=setup", "--format=csv")
	if err == nil {
		t.Fatal("expected error for invalid format, got nil")
	}
	if !strings.Contains(err.Error(), "invalid format") {
		t.Errorf("expected 'invalid format' in error, got: %v", err)
	}
}

// --- task new ---

func TestTaskNew_YAML(t *testing.T) {
	root := setupTaskProject(t)
	withCwd(t, root)

	out, _, err := runTask(t, "new", "--task=migrate", "--title=DB Migration")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "slug: migrate") {
		t.Errorf("expected 'slug: migrate' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "title: DB Migration") {
		t.Errorf("expected 'title: DB Migration' in output, got:\n%s", out)
	}
	if !strings.Contains(out, "status: planning") {
		t.Errorf("expected 'status: planning' in output, got:\n%s", out)
	}

	// Verify task file was created
	taskFile := filepath.Join(root, "tasks", "migrate", "README.md")
	if _, err := os.Stat(taskFile); err != nil {
		t.Errorf("expected task file to exist at %s: %v", taskFile, err)
	}

	// Verify board was updated
	boardData, _ := os.ReadFile(filepath.Join(root, "tasks", "README.md"))
	if !strings.Contains(string(boardData), "migrate") {
		t.Errorf("expected board to contain 'migrate', got:\n%s", string(boardData))
	}
}

func TestTaskNew_JSON(t *testing.T) {
	root := setupTaskProject(t)
	withCwd(t, root)

	out, _, err := runTask(t, "new", "--task=migrate", "--title=DB Migration", "--format=json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var info map[string]interface{}
	if err := json.Unmarshal([]byte(out), &info); err != nil {
		t.Fatalf("invalid JSON: %v\n%s", err, out)
	}
	if info["slug"] != "migrate" {
		t.Errorf("expected slug=migrate, got %v", info["slug"])
	}
	if info["status"] != "planning" {
		t.Errorf("expected status=planning, got %v", info["status"])
	}
}

func TestTaskNew_WithDeps(t *testing.T) {
	root := setupTaskProject(t)
	withCwd(t, root)

	out, _, err := runTask(t, "new", "--task=migrate", "--title=DB Migration", "--depends-on=setup,deploy")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "setup") {
		t.Errorf("expected 'setup' in depends_on output, got:\n%s", out)
	}
	if !strings.Contains(out, "deploy") {
		t.Errorf("expected 'deploy' in depends_on output, got:\n%s", out)
	}

	// Verify board has deps
	boardData, _ := os.ReadFile(filepath.Join(root, "tasks", "README.md"))
	if !strings.Contains(string(boardData), "setup") {
		t.Errorf("expected board to contain dependency 'setup', got:\n%s", string(boardData))
	}
}

func TestTaskNew_MissingTask(t *testing.T) {
	root := setupTaskProject(t)
	withCwd(t, root)

	_, _, err := runTask(t, "new", "--title=Something")
	if err == nil {
		t.Fatal("expected error for missing --task flag, got nil")
	}
	if !strings.Contains(err.Error(), "missing required flag: --task") {
		t.Errorf("expected 'missing required flag: --task' in error, got: %v", err)
	}
}

func TestTaskNew_MissingTitle(t *testing.T) {
	root := setupTaskProject(t)
	withCwd(t, root)

	_, _, err := runTask(t, "new", "--task=migrate")
	if err == nil {
		t.Fatal("expected error for missing --title flag, got nil")
	}
	if !strings.Contains(err.Error(), "missing required flag: --title") {
		t.Errorf("expected 'missing required flag: --title' in error, got: %v", err)
	}
}

func TestTaskNew_Duplicate(t *testing.T) {
	root := setupTaskProject(t)
	withCwd(t, root)

	// "setup" already exists in the fixture
	_, _, err := runTask(t, "new", "--task=setup", "--title=Duplicate Setup")
	if err == nil {
		t.Fatal("expected error for duplicate task, got nil")
	}
	if !strings.Contains(err.Error(), "task already exists") {
		t.Errorf("expected 'task already exists' in error, got: %v", err)
	}
}

func TestTaskNew_InvalidFormat(t *testing.T) {
	root := setupTaskProject(t)
	withCwd(t, root)

	_, _, err := runTask(t, "new", "--task=migrate", "--title=X", "--format=csv")
	if err == nil {
		t.Fatal("expected error for invalid format, got nil")
	}
	if !strings.Contains(err.Error(), "invalid format") {
		t.Errorf("expected 'invalid format' in error, got: %v", err)
	}
}
