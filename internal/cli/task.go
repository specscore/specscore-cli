package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/feature"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
	"github.com/specscore/specscore-cli/pkg/plan"
	"github.com/specscore/specscore-cli/pkg/task"
	"github.com/spf13/cobra"
)

func taskCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "task",
		Short: "Task management — list, info, and create tasks",
	}
	cmd.AddCommand(
		taskListCommand(),
		taskInfoCommand(),
		taskNewCommand(),
		taskChangeStatusCommand(),
	)
	return cmd
}

// --- task change-status ---

// taskChangeStatusCommand transitions a task's **Status:** field via the shared
// lifecycle state-machine contract (KindTask). It is a pure single-actor
// mutation: read the task file, validate the (from, to) arc against the strict
// task legal-transition matrix, and rewrite the **Status:** line. There is no
// claim/release, lock acquisition, or conflict-resolution step
// (cli/task/change-status#ac:single-actor-no-coordination).
//
// Only board-mode resolution (tasks/<task>/README.md) is wired here; the
// plan-inline (--plan) source and the provenance flags are later tasks.
func taskChangeStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "change-status <task> --to=<status>",
		Short: "Transition a task's Status (single-actor, no coordination)",
		Long: `Transitions tasks/<task>/README.md from its current **Status:** to the
value named by --to. The transition is validated against the strict task
legal-transition matrix:

  planning    → queued, aborted
  queued      → in_progress, aborted
  in_progress → blocked, complete, failed, aborted
  blocked     → in_progress, aborted
  complete, failed, aborted are terminal (no outgoing transitions)

--to is case-insensitive and must name one of the seven task statuses
(planning, queued, in_progress, blocked, complete, failed, aborted); a missing
or unrecognized value exits 2. An illegal (from, to) pair — including re-running
on the current status — exits 4. On success the verb performs a pure file
rewrite of the **Status:** line and prints "<task>: <from> → <to>".`,
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runTaskChangeStatus,
	}
	cmd.Flags().String("to", "", "target status (required), case-insensitive. One of: "+
		strings.Join(taskStatusNames(), ", "))
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().String("plan", "", "resolve <task> as a plan-inline task by its **Id:** inside spec/plans/<plan>.md (instead of the tasks board)")
	// Provenance flags are accepted here so the AC command parses cleanly; the
	// provenance write itself is a later task and these are not read yet.
	cmd.Flags().String("commit", "", "provenance: implementing commit SHA (reserved; written by a later task)")
	cmd.Flags().String("repo", "", "provenance: implementing repo (reserved; written by a later task)")
	cmd.Flags().String("branch", "", "provenance: implementing branch (reserved; written by a later task)")
	return cmd
}

// taskStatusNames returns the recognized task status names for help/error text.
func taskStatusNames() []string {
	statuses := lifecycle.LegalStatuses(lifecycle.KindTask)
	names := make([]string, len(statuses))
	for i, s := range statuses {
		names[i] = string(s)
	}
	return names
}

func runTaskChangeStatus(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return exitcode.InvalidArgsError("missing required positional argument: <task>")
	}
	if len(args) > 1 {
		return exitcode.InvalidArgsErrorf(
			"too many positional arguments: change-status accepts exactly one <task>, got %d", len(args))
	}
	taskSlug := args[0]

	toRaw, _ := cmd.Flags().GetString("to")
	if strings.TrimSpace(toRaw) == "" {
		return exitcode.InvalidArgsError("missing required flag: --to=<status>")
	}
	to, ok := lifecycle.ParseStatus(lifecycle.KindTask, toRaw)
	if !ok {
		return exitcode.InvalidArgsErrorf(
			"unrecognized --to value %q for task; legal values: %s",
			toRaw, strings.Join(taskStatusNames(), ", "))
	}

	// --plan selects plan-inline mode: the task is addressed by its **Id:**
	// field on a `### Task N:` block inside spec/plans/<plan>.md. Without --plan
	// the verb stays in board mode (tasks/<task>/README.md), unchanged.
	if planSlug, _ := cmd.Flags().GetString("plan"); strings.TrimSpace(planSlug) != "" {
		return runTaskChangeStatusPlanInline(cmd, taskSlug, strings.TrimSpace(planSlug), to)
	}

	projectFlag, _ := cmd.Flags().GetString("project")
	tasksDir, err := resolveTasksDir(projectFlag)
	if err != nil {
		return err
	}

	taskFilePath := filepath.Join(tasksDir, taskSlug, "README.md")
	from, err := lifecycle.Validate(lifecycle.KindTask, taskFilePath, to)
	if err != nil {
		switch {
		case errors.Is(err, lifecycle.ErrInvalidTransition):
			return exitcode.InvalidStateErrorf("%v", err)
		case errors.Is(err, lifecycle.ErrStatusLineNotFound):
			return exitcode.UnexpectedErrorf("task %s has no **Status:** line", taskSlug)
		default:
			return exitcode.NotFoundErrorf("task not found: %s", taskSlug)
		}
	}

	// Pure single-actor mutation: rewrite the **Status:** line (and the
	// frontmatter status: mirror if present). No claim/release, lock, or
	// conflict-resolution step (cli/task/change-status#ac:single-actor-no-coordination).
	if _, err := lifecycle.Rewrite(taskFilePath, to); err != nil {
		return exitcode.UnexpectedErrorf("rewriting status: %v", err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s → %s\n", taskSlug, string(from), string(to))
	return nil
}

// runTaskChangeStatusPlanInline resolves <task> to the `### Task N:` block in
// spec/plans/<planSlug>.md whose **Id:** equals taskSlug, validates the
// (from, to) arc against the single KindTask matrix (illegal arcs exit 4), and
// rewrites that block's **Status:** line in place — every other line is
// preserved byte-for-byte. A missing plan file or no matching **Id:** exits 3.
//
// Provenance writing (--repo/--commit/--branch) is the next task; this function
// returns the resolved block as the rewrite target so that follow-up can hang
// the provenance write off the same StatusLine locator.
func runTaskChangeStatusPlanInline(cmd *cobra.Command, taskSlug, planSlug string, to lifecycle.Status) error {
	projectFlag, _ := cmd.Flags().GetString("project")
	specRoot, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}

	planPath := filepath.Join(specRoot, "spec", "plans", planSlug+".md")
	p, err := plan.Parse(planPath)
	if err != nil {
		if os.IsNotExist(err) {
			return exitcode.NotFoundErrorf("plan not found: %s", planSlug)
		}
		return exitcode.UnexpectedErrorf("parsing plan %s: %v", planSlug, err)
	}

	// Resolve the task block by its stable **Id:** field.
	var target *plan.Task
	for i := range p.Tasks {
		if p.Tasks[i].IdPresent && p.Tasks[i].Id == taskSlug {
			target = &p.Tasks[i]
			break
		}
	}
	if target == nil {
		return exitcode.NotFoundErrorf("task %q not found by **Id:** in plan %s", taskSlug, planSlug)
	}

	// Validate through the same single-actor KindTask matrix as board mode.
	from := lifecycle.Status(string(target.Status))
	if err := lifecycle.Transition(lifecycle.KindTask, from, to); err != nil {
		return exitcode.InvalidStateErrorf("%v", err)
	}

	if target.StatusLine == 0 {
		return exitcode.UnexpectedErrorf("task %q in plan %s has no **Status:** line", taskSlug, planSlug)
	}
	if err := rewritePlanTaskStatusLine(planPath, target.StatusLine, to); err != nil {
		return exitcode.UnexpectedErrorf("rewriting status: %v", err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s → %s\n", taskSlug, string(from), string(to))
	return nil
}

// rewritePlanTaskStatusLine rewrites the 1-based statusLine of a plan file to
// `**Status:** <to>`, preserving every other line verbatim. The line number
// comes from the parse pass over the same file, so it is always in range. This
// mirrors the per-block status rewrite in pkg/lint (rewriteTaskStatusLines).
func rewritePlanTaskStatusLine(path string, statusLine int, to lifecycle.Status) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	lines[statusLine-1] = "**Status:** " + string(to)
	return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
}

// resolveTasksDir resolves the tasks directory from a --project flag or CWD.
func resolveTasksDir(projectFlag string) (string, error) {
	var startDir string
	if projectFlag != "" {
		abs, err := filepathAbsFn(projectFlag)
		if err != nil {
			return "", exitcode.InvalidArgsErrorf("resolving --project path: %v", err)
		}
		startDir = abs
	} else {
		cwd, err := osGetwdFn()
		if err != nil {
			return "", exitcode.UnexpectedErrorf("cannot determine working directory: %v", err)
		}
		startDir = cwd
	}

	root, err := feature.FindSpecRepoRoot(startDir)
	if err != nil {
		return "", err
	}

	tasksDir := filepath.Join(root, "tasks")
	info, err := os.Stat(tasksDir)
	if err != nil || !info.IsDir() {
		return "", exitcode.NotFoundErrorf("tasks directory not found: %s", tasksDir)
	}
	return tasksDir, nil
}

// --- task list ---

func taskListCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List tasks from the board",
		Long: `Lists tasks from the project's tasks/README.md board. Optionally filter
by status. Default output format is YAML.`,
		Args: cobra.NoArgs,
		RunE: runTaskList,
	}
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().String("status", "", "filter by status (e.g., planning, queued, in_progress, completed)")
	cmd.Flags().String("format", "yaml", "output format: yaml, json, md")
	return cmd
}

func runTaskList(cmd *cobra.Command, _ []string) error {
	projectFlag, _ := cmd.Flags().GetString("project")
	statusFlag, _ := cmd.Flags().GetString("status")
	formatFlag, _ := cmd.Flags().GetString("format")

	if formatFlag != "yaml" && formatFlag != "json" && formatFlag != "md" {
		return exitcode.InvalidArgsErrorf("invalid format: %s (supported: yaml, json, md)", formatFlag)
	}

	tasksDir, err := resolveTasksDir(projectFlag)
	if err != nil {
		return err
	}

	boardPath := filepath.Join(tasksDir, "README.md")
	data, err := osReadFileFn(boardPath)
	if err != nil {
		return exitcode.NotFoundErrorf("reading board: %v", err)
	}

	bv, err := task.ParseBoard(data)
	if err != nil {
		return exitcode.UnexpectedErrorf("parsing board: %v", err)
	}

	// Filter by status if requested.
	rows := bv.Rows
	if statusFlag != "" {
		filterStatus := task.TaskStatus(statusFlag)
		// Validate status value.
		switch filterStatus {
		case task.StatusPlanning, task.StatusQueued, task.StatusClaimed,
			task.StatusInProgress, task.StatusCompleted, task.StatusFailed,
			task.StatusBlocked, task.StatusAborted:
		default:
			return exitcode.InvalidArgsErrorf("invalid status: %s (valid: planning, queued, claimed, in_progress, completed, failed, blocked, aborted)", statusFlag)
		}
		var filtered []task.BoardRow
		for _, r := range rows {
			if r.Status == filterStatus {
				filtered = append(filtered, r)
			}
		}
		rows = filtered
	}

	w := cmd.OutOrStdout()

	switch formatFlag {
	case "yaml":
		enc := newYAMLEnc(w)
		if err := enc.Encode(rows); err != nil {
			return exitcode.UnexpectedErrorf("encoding yaml: %v", err)
		}
		return enc.Close()
	case "json":
		return newJSONEnc(w).Encode(rows)
	default:
		filtered := &task.BoardView{Rows: rows}
		_, err := w.Write(task.RenderBoard(filtered))
		return err
	}
}

// --- task info ---

func taskInfoCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "info",
		Short: "Show detailed task information",
		Long: `Shows detailed information for a task by reading its tasks/{slug}/README.md
file and status from the board. Output includes title, status, description,
dependencies, and summary.`,
		Args: cobra.NoArgs,
		RunE: runTaskInfo,
	}
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().String("task", "", "task slug (required)")
	cmd.Flags().String("format", "yaml", "output format: yaml, json")
	return cmd
}

// taskInfoOutput is the combined output for task info.
type taskInfoOutput struct {
	Slug        string   `yaml:"slug" json:"slug"`
	Title       string   `yaml:"title" json:"title"`
	Status      string   `yaml:"status" json:"status"`
	Description string   `yaml:"description,omitempty" json:"description,omitempty"`
	DependsOn   []string `yaml:"depends_on,omitempty" json:"depends_on,omitempty"`
	Summary     string   `yaml:"summary,omitempty" json:"summary,omitempty"`
}

func runTaskInfo(cmd *cobra.Command, _ []string) error {
	projectFlag, _ := cmd.Flags().GetString("project")
	taskFlag, _ := cmd.Flags().GetString("task")
	formatFlag, _ := cmd.Flags().GetString("format")

	if taskFlag == "" {
		return exitcode.InvalidArgsError("missing required flag: --task")
	}
	if formatFlag != "yaml" && formatFlag != "json" {
		return exitcode.InvalidArgsErrorf("invalid format: %s (supported: yaml, json)", formatFlag)
	}

	tasksDir, err := resolveTasksDir(projectFlag)
	if err != nil {
		return err
	}

	// Read the task file.
	taskFilePath := filepath.Join(tasksDir, taskFlag, "README.md")
	data, err := osReadFileFn(taskFilePath)
	if err != nil {
		return exitcode.NotFoundErrorf("task not found: %s", taskFlag)
	}

	tfd, err := task.ParseTaskFile(data)
	if err != nil {
		return exitcode.UnexpectedErrorf("parsing task file: %v", err)
	}

	// Read status from board.
	status := string(task.StatusPlanning) // default
	boardPath := filepath.Join(tasksDir, "README.md")
	boardData, err := osReadFileFn(boardPath)
	if err == nil {
		bv, parseErr := task.ParseBoard(boardData)
		if parseErr == nil {
			for _, r := range bv.Rows {
				if r.Task == taskFlag {
					status = string(r.Status)
					break
				}
			}
		}
	}

	out := taskInfoOutput{
		Slug:        taskFlag,
		Title:       tfd.Title,
		Status:      status,
		Description: tfd.Description,
		DependsOn:   tfd.DependsOn,
		Summary:     tfd.Summary,
	}

	w := cmd.OutOrStdout()
	switch formatFlag {
	case "yaml":
		enc := newYAMLEnc(w)
		if err := enc.Encode(out); err != nil {
			return exitcode.UnexpectedErrorf("encoding yaml: %v", err)
		}
		return enc.Close()
	default:
		return newJSONEnc(w).Encode(out)
	}
}

// --- task new ---

func taskNewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new",
		Short: "Create a new task",
		Long: `Creates a new task by writing tasks/{slug}/README.md and adding a row
to the tasks/README.md board. New tasks are always created in "planning" status.`,
		Args: cobra.NoArgs,
		RunE: runTaskNew,
	}
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().String("task", "", "task slug (required)")
	cmd.Flags().String("title", "", "task title (required)")
	cmd.Flags().String("description", "", "task description")
	cmd.Flags().String("depends-on", "", "comma-separated list of dependency slugs")
	cmd.Flags().String("format", "yaml", "output format: yaml, json")
	return cmd
}

func runTaskNew(cmd *cobra.Command, _ []string) error {
	projectFlag, _ := cmd.Flags().GetString("project")
	taskFlag, _ := cmd.Flags().GetString("task")
	titleFlag, _ := cmd.Flags().GetString("title")
	descFlag, _ := cmd.Flags().GetString("description")
	depsFlag, _ := cmd.Flags().GetString("depends-on")
	formatFlag, _ := cmd.Flags().GetString("format")

	if taskFlag == "" {
		return exitcode.InvalidArgsError("missing required flag: --task")
	}
	if titleFlag == "" {
		return exitcode.InvalidArgsError("missing required flag: --title")
	}
	if formatFlag != "yaml" && formatFlag != "json" {
		return exitcode.InvalidArgsErrorf("invalid format: %s (supported: yaml, json)", formatFlag)
	}

	// Parse --depends-on
	deps := []string{}
	if depsFlag != "" {
		for _, d := range strings.Split(depsFlag, ",") {
			d = strings.TrimSpace(d)
			if d != "" {
				deps = append(deps, d)
			}
		}
	}

	tasksDir, err := resolveTasksDir(projectFlag)
	if err != nil {
		return err
	}

	// Check if task already exists.
	taskDir := filepath.Join(tasksDir, taskFlag)
	if _, err := os.Stat(taskDir); err == nil {
		return exitcode.InvalidArgsErrorf("task already exists: %s", taskFlag)
	}

	// Create task directory and README.
	if err := os.MkdirAll(taskDir, 0o755); err != nil {
		return exitcode.UnexpectedErrorf("creating task directory: %v", err)
	}

	tfd := task.TaskFileData{
		Title:       titleFlag,
		Description: descFlag,
		DependsOn:   deps,
	}
	taskFilePath := filepath.Join(taskDir, "README.md")
	if err := osWriteFileFn(taskFilePath, task.RenderTaskFile(tfd), 0o644); err != nil {
		return exitcode.UnexpectedErrorf("writing task file: %v", err)
	}

	// Update board: read, append row, write back.
	boardPath := filepath.Join(tasksDir, "README.md")
	boardData, err := osReadFileFn(boardPath)
	if err != nil {
		return exitcode.NotFoundErrorf("reading board: %v", err)
	}

	bv, err := task.ParseBoard(boardData)
	if err != nil {
		return exitcode.UnexpectedErrorf("parsing board: %v", err)
	}

	newRow := task.BoardRow{
		Task:      taskFlag,
		Status:    task.StatusPlanning,
		DependsOn: deps,
	}
	bv.Rows = append(bv.Rows, newRow)

	if err := os.WriteFile(boardPath, task.RenderBoard(bv), 0o644); err != nil {
		return exitcode.UnexpectedErrorf("writing board: %v", err)
	}

	// Output the created task info.
	out := taskInfoOutput{
		Slug:        taskFlag,
		Title:       titleFlag,
		Status:      string(task.StatusPlanning),
		Description: descFlag,
		DependsOn:   deps,
	}

	w := cmd.OutOrStdout()
	switch formatFlag {
	case "yaml":
		enc := newYAMLEnc(w)
		if err := enc.Encode(out); err != nil {
			return exitcode.UnexpectedErrorf("encoding yaml: %v", err)
		}
		return enc.Close()
	default:
		return newJSONEnc(w).Encode(out)
	}
}
