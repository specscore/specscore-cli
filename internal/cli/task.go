package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
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

func taskNewMarkerPath(tasksDir, slug string) string {
	return filepath.Join(tasksDir, ".task-new-"+slug+".prepared")
}

type taskNewPrepared struct {
	SchemaVersion     int    `json:"schema_version"`
	TaskID            string `json:"task_id"`
	TaskPath          string `json:"task_path"`
	ContentSHA256     string `json:"content_sha256"`
	BoardBeforeSHA256 string `json:"board_before_sha256"`
	BoardAfterSHA256  string `json:"board_after_sha256"`
}

func (p taskNewPrepared) bytes() []byte {
	b, _ := json.Marshal(p)
	return append(b, '\n')
}

func taskNewDigest(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }

func taskCommand() *cobra.Command {
	return taskCommandWithDeps(defaultTaskMutationDeps())
}

func taskCommandWithDeps(deps taskMutationDeps) *cobra.Command {
	deps = deps.withDefaults()
	cmd := &cobra.Command{
		Use:   "task",
		Short: "Task management — list, info, and create tasks",
	}
	cmd.AddCommand(
		taskListCommand(),
		taskInfoCommand(),
		taskNewCommand(deps),
		taskChangeStatusCommand(deps),
		taskAmendCommand(deps),
		taskTransitionsCommand(),
	)
	return cmd
}

// taskTransitionsCommand registers `specscore task transitions [<task>]`,
// the read-only counterpart to change-status. It covers board-mode tasks
// (tasks/<task>/README.md) only: a plan-inline task's status lives inside a
// `### Task N:` block partway down its plan file rather than as that file's
// leading `**Status:**` line, so resolving one requires the plan block
// parser change-status's plan-inline path uses — out of scope for this
// read-only query verb. Use `plan info <plan>` to inspect a plan-inline
// task's current status in the meantime.
func taskTransitionsCommand() *cobra.Command {
	return transitionsCommand(lifecycle.KindTask, "task", "Show the Task status matrix, or one board-mode task's legal next statuses",
		func(projectFlag, taskSlug string) (string, error) {
			tasksDir, err := resolveTasksDir(projectFlag)
			if err != nil {
				return "", err
			}
			taskFilePath := filepath.Join(tasksDir, taskSlug, "README.md")
			if _, statErr := os.Stat(taskFilePath); statErr != nil {
				return "", exitcode.NotFoundErrorf("task not found: %s", taskSlug)
			}
			return taskFilePath, nil
		})
}

// --- task change-status ---

// taskChangeStatusCommand transitions a task's **Status:** field via the shared
// lifecycle state-machine contract (KindTask). It is one fail-fast artifact
// transaction: lock, read exact bytes, validate the (from, to) arc, compose
// status and adjacent fields in memory, then make one atomic durable write.
// It acquires no distributed coordination claim.
//
// Both board-mode (tasks/<task>/README.md) and plan-inline (--plan) resolution
// are wired here. On --to=complete the --repo/--commit/--branch flags assemble
// an **Implemented-by:** provenance field written alongside the status.
func taskChangeStatusCommand(deps taskMutationDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "change-status <task> --to=<status>",
		Short: "Transition a task's Status (local transaction; no distributed coordination)",
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
on the current status — exits 4. The verb uses one fail-fast local artifact
transaction for Status and every supplied adjacent field. Lock contention or
a changed preimage exits 1; on success it prints "<task>: <from> → <to>".
It does not invoke a derived lint/index callback.

--note and --evidence are optional annotations, valid on ANY transition (not
restricted to --to=complete like the provenance flags): --note records a
free-text justification, --evidence records a comma-separated list of
supporting references (commit SHAs, PR URLs, file paths, deploy or monitoring
links). Both are written as their own field ("**Note:**" / "**Evidence:**")
immediately after **Status:** (and after **Implemented-by:** when provenance
is also written in the same call), in the same atomic rewrite. Neither is
required, and — unlike ImplementationCommit — neither is syntactically
validated. Not supported together with --amend-provenance.

With --plan, when the target plan declares **Coordination:**
<owner>/<repo>@<branch>, this verb refuses (exit 1) unless the current git
repo/branch matches — mutating a plan-inline task on a coordinated plan from
anywhere else is a process violation, not a merge chore. --force-coordination
bypasses the check for this invocation only, printing a warning naming what
was bypassed. Board-mode tasks (no --plan) carry no Coordination field and
are never subject to this check.`,
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          func(cmd *cobra.Command, args []string) error { return runTaskChangeStatus(cmd, args, deps) },
	}
	cmd.Flags().String("to", "", "target status (required), case-insensitive. One of: "+
		strings.Join(taskStatusNames(), ", "))
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().String("plan", "", "resolve <task> as a plan-inline task by its **Id:** inside spec/plans/<plan>.md (instead of the tasks board)")
	// Provenance flags: on --to=complete these are assembled into an
	// **Implemented-by:** field on the task. Values come ONLY from these flags;
	// the verb never reads ambient git HEAD.
	cmd.Flags().String("commit", "", "provenance: implementing commit SHA (written as **Implemented-by:** on --to=complete)")
	cmd.Flags().String("repo", "", "provenance: implementing repo slug or clone URL (omitted for same-repo bare sha)")
	cmd.Flags().String("branch", "", "provenance: implementing branch (optional trailing \"(<branch>)\")")
	cmd.Flags().Bool("amend-provenance", false, "re-stamp **Implemented-by:** on an already-complete task WITHOUT a status transition (mutually exclusive with --to)")
	// Annotation flags: optional on any transition, independent of --to value.
	cmd.Flags().String("note", "", "optional free-text annotation written as **Note:** (any transition; not supported with --amend-provenance)")
	cmd.Flags().String("evidence", "", "optional comma-separated supporting references written as **Evidence:** (any transition; not supported with --amend-provenance)")
	cmd.Flags().Bool(coordinationForceFlagName, false, coordinationForceFlagUsage+" (only applies with --plan, when the target plan declares **Coordination:**)")
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

func runTaskChangeStatus(cmd *cobra.Command, args []string, deps taskMutationDeps) error {
	if len(args) == 0 {
		return exitcode.InvalidArgsError("missing required positional argument: <task>")
	}
	if len(args) > 1 {
		return exitcode.InvalidArgsErrorf(
			"too many positional arguments: change-status accepts exactly one <task>, got %d", len(args))
	}
	taskSlug := args[0]
	if err := task.ValidateSlug(taskSlug); err != nil {
		return exitcode.InvalidArgsErrorf("invalid task slug: %v", err)
	}
	planSlug, _ := cmd.Flags().GetString("plan")
	planSlug = strings.TrimSpace(planSlug)
	if planSlug != "" {
		if err := plan.ValidateSlug(planSlug); err != nil {
			return exitcode.InvalidArgsErrorf("invalid plan slug: %v", err)
		}
	}

	// --amend-provenance corrects/clears the **Implemented-by:** field on an
	// already-complete task WITHOUT a status transition. It is mutually exclusive
	// with --to, and the command requires exactly one of the two.
	amend, _ := cmd.Flags().GetBool("amend-provenance")
	toRaw, _ := cmd.Flags().GetString("to")
	toRaw = strings.TrimSpace(toRaw)
	if amend {
		if toRaw != "" {
			return exitcode.InvalidArgsError(
				"--amend-provenance is mutually exclusive with --to")
		}
		// --note/--evidence annotate a status transition; --amend-provenance is
		// not one (it corrects a field on an already-complete task without
		// moving Status), so the two are rejected together rather than silently
		// dropping the annotation.
		if noteFromFlags(cmd) != "" || len(evidenceFromFlags(cmd)) > 0 {
			return exitcode.InvalidArgsError(
				"--note/--evidence are not supported together with --amend-provenance")
		}
		// Reuse the shared provenance-flag validation: when any provenance flag is
		// present an explicit --commit is required (the "complete" predicate is
		// trivially satisfied here, so only the --commit rule applies).
		if err := validateProvenanceFlags(cmd, lifecycle.TaskComplete); err != nil {
			return err
		}
		if planSlug != "" {
			return runTaskAmendProvenancePlanInline(cmd, taskSlug, planSlug, deps)
		}
		return runTaskAmendProvenanceBoard(cmd, taskSlug, deps)
	}
	if toRaw == "" {
		return exitcode.InvalidArgsError("missing required flag: --to=<status>")
	}
	to, ok := lifecycle.ParseStatus(lifecycle.KindTask, toRaw)
	if !ok {
		return exitcode.InvalidArgsErrorf(
			"unrecognized --to value %q for task; legal values: %s",
			toRaw, strings.Join(taskStatusNames(), ", "))
	}

	// Provenance flags (--repo/--commit/--branch) are valid only when completing
	// and only alongside an explicit --commit. Reject invalid combinations up
	// front — before any file mutation in either board or plan-inline mode — so a
	// rejected task is left UNCHANGED
	// (cli/task/change-status#ac:provenance-flag-without-complete-rejected,
	// cli/task/change-status#ac:provenance-flag-without-commit-rejected).
	if err := validateProvenanceFlags(cmd, to); err != nil {
		return err
	}

	// --plan selects plan-inline mode: the task is addressed by its **Id:**
	// field on a `### Task N:` block inside spec/plans/<plan>.md. Without --plan
	// the verb stays in board mode (tasks/<task>/README.md), unchanged.
	if planSlug != "" {
		return runTaskChangeStatusPlanInline(cmd, taskSlug, planSlug, to, deps)
	}

	projectFlag, _ := cmd.Flags().GetString("project")
	tasksDir, err := resolveTasksDir(projectFlag)
	if err != nil {
		return err
	}

	taskFilePath := filepath.Join(tasksDir, taskSlug, "README.md")
	ref := ""
	if to == lifecycle.TaskComplete {
		ref = implementedByRefFromFlags(cmd)
	}
	fields := buildExtraTaskFieldLines(ref, noteFromFlags(cmd), evidenceFromFlags(cmd))
	from, err := deps.rewriteBoardTask(taskFilePath, to, fields)
	if err != nil {
		var coded *exitcode.Error
		if errors.As(err, &coded) {
			return err
		}
		switch {
		case errors.Is(err, lifecycle.ErrConcurrentMutation):
			return exitcode.ConflictErrorf("task %s changed concurrently; re-read before changing status", taskSlug)
		case errors.Is(err, lifecycle.ErrInvalidTransition):
			return exitcode.InvalidStateErrorf("%v", err)
		case errors.Is(err, lifecycle.ErrStatusLineNotFound):
			return exitcode.UnexpectedErrorf("task %s has no **Status:** line", taskSlug)
		case errors.Is(err, os.ErrNotExist):
			return exitcode.NotFoundErrorf("task not found: %s", taskSlug)
		default:
			return exitcode.UnexpectedErrorCause(fmt.Sprintf("rewriting task: %v", err), err)
		}
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s → %s\n", taskSlug, string(from), string(to))
	return nil
}

// validateProvenanceFlags rejects invalid provenance-flag combinations before
// any file is touched. When any of --repo/--commit/--branch is supplied, the
// transition must target complete and an explicit --commit must be present;
// otherwise it returns an InvalidArgs (exit 2) error and the task is left
// unchanged. With no provenance flags it is a no-op.
func validateProvenanceFlags(cmd *cobra.Command, to lifecycle.Status) error {
	repo, _ := cmd.Flags().GetString("repo")
	commit, _ := cmd.Flags().GetString("commit")
	branch, _ := cmd.Flags().GetString("branch")
	repo, commit, branch = strings.TrimSpace(repo), strings.TrimSpace(commit), strings.TrimSpace(branch)

	if repo == "" && commit == "" && branch == "" {
		return nil
	}
	if to != lifecycle.TaskComplete {
		return exitcode.InvalidArgsError(
			"provenance flags (--repo/--commit/--branch) are valid only with --to=complete")
	}
	if commit == "" {
		return exitcode.InvalidArgsError(
			"--commit is required when provenance flags (--repo/--branch) are supplied")
	}
	return nil
}

// implementedByRefFromFlags assembles the **Implemented-by:** reference from the
// --repo/--commit/--branch flags. It returns "" when no reference can be formed
// (i.e. --commit is absent), so the caller writes no provenance. Values come
// ONLY from these flags; ambient git state is never consulted.
func implementedByRefFromFlags(cmd *cobra.Command) string {
	repo, _ := cmd.Flags().GetString("repo")
	commit, _ := cmd.Flags().GetString("commit")
	branch, _ := cmd.Flags().GetString("branch")
	return assembleImplementedByRef(repo, commit, branch)
}

// assembleImplementedByRef builds the implementation-commit reference written as
// the value of `**Implemented-by:**`. The shape is `<repo>@<sha> (<branch>)`,
// where `<repo>` (a slug OR a full clone URL) is omitted for a same-repo commit
// (a bare `<sha>`), and the trailing ` (<branch>)` is omitted when no branch is
// given. A reference requires a commit: with no --commit this returns "" (Task
// 4 turns provenance-without-commit into a hard rejection).
func assembleImplementedByRef(repo, commit, branch string) string {
	commit = strings.TrimSpace(commit)
	if commit == "" {
		return ""
	}
	ref := commit
	if repo = strings.TrimSpace(repo); repo != "" {
		ref = repo + "@" + commit
	}
	if branch = strings.TrimSpace(branch); branch != "" {
		ref += " (" + branch + ")"
	}
	return ref
}

// noteFromFlags returns the trimmed --note value, or "" when absent/blank.
// --note is an optional free-text annotation, valid on ANY transition —
// unlike the provenance flags it is not restricted to --to=complete.
func noteFromFlags(cmd *cobra.Command) string {
	note, _ := cmd.Flags().GetString("note")
	return strings.TrimSpace(note)
}

// evidenceFromFlags parses --evidence into its comma-separated, trimmed,
// non-empty refs, in source order. Returns nil when absent/blank. Like
// --note, --evidence is valid on ANY transition and carries unstructured
// references (never syntactically validated, unlike **Implemented-by:**).
func evidenceFromFlags(cmd *cobra.Command) []string {
	raw, _ := cmd.Flags().GetString("evidence")
	var out []string
	for _, e := range strings.Split(raw, ",") {
		e = strings.TrimSpace(e)
		if e != "" {
			out = append(out, e)
		}
	}
	return out
}

// buildExtraTaskFieldLines formats the optional Implemented-by/Note/Evidence
// field lines to be inserted immediately after a task's **Status:** line, in
// this fixed order, in ONE atomic pass. An empty ref/note, or an empty
// evidence slice, contributes no line — the common case (no flags supplied)
// returns nil, so callers can skip the write entirely. Evidence entries are
// joined with ", " (mirroring `plan reconcile`'s Evidence line).
func buildExtraTaskFieldLines(ref, note string, evidence []string) []string {
	var out []string
	if ref != "" {
		out = append(out, "**Implemented-by:** "+ref)
	}
	if note != "" {
		out = append(out, "**Note:** "+note)
	}
	if len(evidence) > 0 {
		out = append(out, "**Evidence:** "+strings.Join(evidence, ", "))
	}
	return out
}

// errNoStatusForProvenance is returned by writeBoardExtraFields when the file
// has no `**Status:**` line to anchor the extra fields to. This is defensive:
// board mode always runs lifecycle.Rewrite first, which guarantees a status
// line.
var errNoStatusForProvenance = errors.New("task file has no **Status:** line for provenance placement")

// writeBoardExtraFields inserts fields (already formatted "**Name:** value"
// lines, in caller-supplied order — see buildExtraTaskFieldLines) into the
// board task file at path, immediately after its `**Status:**` line, in ONE
// atomic write. It shares the lifecycle mutation fence with status rewrites
// and annotation amendments, so a stale read-modify-write can never erase a
// concurrent amendment. Every other byte is preserved. Covers provenance
// (**Implemented-by:**) and the optional **Note:**/**Evidence:** annotations.
func writeBoardExtraFields(path string, fields []string) error {
	return lifecycle.TransformArtifact(path, func(before []byte) ([]byte, error) {
		return boardExtraFieldsBytes(before, fields)
	})
}

// rewriteBoardTask applies status, provenance, and transition annotations in
// one lifecycle critical section. The Validate read is deliberately inside the
// fence: no writer may validate an old task body then overwrite an amendment.
func rewriteBoardTask(path string, to lifecycle.Status, fields []string) (lifecycle.Status, error) {
	return rewriteBoardTaskWithTransforms(path, to, fields, lifecycle.RewriteBytes, boardExtraFieldsBytes)
}

func rewriteBoardTaskWithTransforms(path string, to lifecycle.Status, fields []string, rewriteStatus func([]byte, lifecycle.Status) ([]byte, string, error), addFields func([]byte, []string) ([]byte, error)) (lifecycle.Status, error) {
	var from lifecycle.Status
	err := lifecycle.TransformArtifact(path, func(before []byte) ([]byte, error) {
		lines := strings.Split(string(before), "\n")
		statusIdx, validateErr := validateTaskSingletonLines(lines, 0, len(lines), "board task")
		if validateErr != nil {
			return nil, validateErr
		}
		from = lifecycle.Status(strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(lines[statusIdx]), "**Status:**")))
		var transformErr error
		if transformErr = lifecycle.Transition(lifecycle.KindTask, from, to); transformErr != nil {
			return nil, transformErr
		}
		updated, _, transformErr := rewriteStatus(before, to)
		if transformErr != nil {
			return nil, transformErr
		}
		if len(fields) > 0 {
			updated, transformErr = addFields(updated, fields)
			if transformErr != nil {
				return nil, transformErr
			}
		}
		return updated, nil
	})
	return from, err
}

func boardExtraFieldsBytes(data []byte, fields []string) ([]byte, error) {
	lines := strings.Split(string(data), "\n")
	idx, err := validateTaskSingletonLines(lines, 0, len(lines), "board task")
	if err != nil {
		return nil, err
	}
	lines = upsertTaskFieldLines(lines, idx, len(lines), fields)
	return []byte(strings.Join(lines, "\n")), nil
}

// writeBoardImplementedBy inserts an `**Implemented-by:** <ref>` line into the
// board task file at path, immediately after its `**Status:**` line. Every
// other byte is preserved. Kept as a thin single-field wrapper over
// writeBoardExtraFields for callers that only ever write provenance.
func writeBoardImplementedBy(path, ref string) error {
	return writeBoardExtraFields(path, []string{"**Implemented-by:** " + ref})
}

// withExtraFieldLines returns lines with fieldLines (already formatted
// "**Name:** value" strings, in caller-supplied order) inserted immediately
// after lines[anchorIdx], keeping them adjacent to the **Status:** line in
// both board files and plan-inline task blocks. A nil/empty fieldLines is a
// no-op (returns lines unchanged).
func withExtraFieldLines(lines []string, anchorIdx int, fieldLines []string) []string {
	if len(fieldLines) == 0 {
		return lines
	}
	out := make([]string, 0, len(lines)+len(fieldLines))
	out = append(out, lines[:anchorIdx+1]...)
	out = append(out, fieldLines...)
	out = append(out, lines[anchorIdx+1:]...)
	return out
}

// withImplementedByLine returns lines with `**Implemented-by:** ref` inserted
// immediately after lines[statusIdx]. Kept as a thin single-field wrapper over
// withExtraFieldLines; still used by the corrective-restamp path
// (withAmendedImplementedBy), which manages ONLY the Implemented-by field.
func withImplementedByLine(lines []string, statusIdx int, ref string) []string {
	return withExtraFieldLines(lines, statusIdx, []string{"**Implemented-by:** " + ref})
}

// runTaskChangeStatusPlanInline resolves <task> to the `### Task N:` block in
// spec/plans/<planSlug>.md whose **Id:** equals taskSlug, validates the
// (from, to) arc against the single KindTask matrix (illegal arcs exit 4), and
// rewrites that block's **Status:** line in place — every other line is
// preserved byte-for-byte. A missing plan file or no matching **Id:** exits 3.
//
// On --to=complete the provenance flags (--repo/--commit/--branch) are
// assembled into an **Implemented-by:** line written adjacent to the block's
// **Status:** in the same atomic write.
func runTaskChangeStatusPlanInline(cmd *cobra.Command, taskSlug, planSlug string, to lifecycle.Status, deps taskMutationDeps) error {
	projectFlag, _ := cmd.Flags().GetString("project")
	specRoot, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}

	planPath := filepath.Join(specRoot, "spec", "plans", planSlug+".md")
	var from lifecycle.Status
	var coordinationWarning bytes.Buffer
	err = deps.transformArtifact(planPath, func(before []byte) ([]byte, error) {
		return changePlanTaskStatusBytes(cmd, planPath, taskSlug, planSlug, specRoot, to, &from, before, &coordinationWarning, deps.rewritePlanTaskStatus)
	})
	if err != nil {
		var committed *lifecycle.CommittedMutationError
		if errors.As(err, &committed) {
			_, _ = cmd.ErrOrStderr().Write(coordinationWarning.Bytes())
			return exitcode.UnexpectedErrorCause(fmt.Sprintf("changing plan-inline task status: %v", err), err)
		}
		if errors.Is(err, lifecycle.ErrConcurrentMutation) {
			return exitcode.ConflictErrorf("task %s changed concurrently; re-read before changing status", taskSlug)
		}
		if os.IsNotExist(err) {
			return exitcode.NotFoundErrorf("plan not found: %s", planSlug)
		}
		var coded *exitcode.Error
		if errors.As(err, &coded) {
			return err
		}
		return exitcode.UnexpectedErrorCause(fmt.Sprintf("changing plan-inline task status: %v", err), err)
	}
	_, _ = cmd.ErrOrStderr().Write(coordinationWarning.Bytes())
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s → %s\n", taskSlug, string(from), string(to))
	return nil
}

func changePlanTaskStatusBytes(cmd *cobra.Command, planPath, taskSlug, planSlug, specRoot string, to lifecycle.Status, fromOut *lifecycle.Status, before []byte, coordinationWarning io.Writer, rewriteStatus func([]byte, int, lifecycle.Status, []string) ([]byte, error)) ([]byte, error) {
	p, err := plan.ParseBytes(planPath, before)
	if err != nil {
		// ParseBytes consumes the supplied snapshot only and never performs I/O.
		return nil, exitcode.UnexpectedErrorf("parsing plan %s: %v", planSlug, err)
	}

	// Coordination-branch enforcement: when the plan declares
	// **Coordination:**, mutating one of its inline tasks is authoritative
	// only on the declared repo/branch
	// (spec/features/plan/README.md#coordination-branch, upstream).
	forceCoordination, _ := cmd.Flags().GetBool(coordinationForceFlagName)
	if cerr := enforceCoordinationBranch(p, specRoot, forceCoordination, coordinationWarning); cerr != nil {
		return nil, cerr
	}

	// Resolve the task block by its stable **Id:** field, rejecting duplicates.
	target, err := uniquePlanTaskByID(p, taskSlug)
	if err != nil {
		return nil, err
	}

	// Validate through the same single-actor KindTask matrix as board mode.
	from := lifecycle.Status(string(target.Status))
	if err := lifecycle.Transition(lifecycle.KindTask, from, to); err != nil {
		return nil, exitcode.InvalidStateErrorf("%v", err)
	}

	if target.StatusLine == 0 {
		return nil, exitcode.UnexpectedErrorf("task %q in plan %s has no **Status:** line", taskSlug, planSlug)
	}
	// Moving a plan-inline task into in_progress is the first execution-band
	// entrypoint. Keep the check before the file rewrite so an unmet
	// prerequisite leaves the task byte-for-byte untouched.
	if to == lifecycle.TaskInProgress {
		readiness, readinessErr := p.PrerequisiteReadiness(filepath.Join(specRoot, "spec", "plans"))
		if readinessErr != nil {
			return exitcode.UnexpectedErrorf("checking prerequisite readiness for plan %q: %v", planSlug, readinessErr)
		}
		if !readiness.Ready {
			return exitcode.InvalidStateErrorf(
				"plan %q is not ready to begin execution; unmet prerequisite plan(s): %s; each must derive Implemented from its task rollup",
				planSlug, readiness.UnmetMessage())
		}
	}
	// Provenance is written ONLY when completing, in the same atomic write that
	// sets the status; values come ONLY from flags (never ambient git HEAD).
	// --note/--evidence are independent optional annotations, valid on ANY
	// transition, written in the same atomic pass alongside provenance when
	// both are given.
	implementedBy := ""
	if to == lifecycle.TaskComplete {
		implementedBy = implementedByRefFromFlags(cmd)
	}
	extraFields := buildExtraTaskFieldLines(implementedBy, noteFromFlags(cmd), evidenceFromFlags(cmd))
	after, err := rewriteStatus(before, target.StatusLine, to, extraFields)
	if err != nil {
		if errors.Is(err, lifecycle.ErrConcurrentMutation) {
			return nil, err
		}
		return nil, exitcode.UnexpectedErrorf("rewriting status: %v", err)
	}
	*fromOut = from
	return after, nil
}

// rewritePlanTaskStatusLine rewrites the 1-based statusLine of a plan file to
// `**Status:** <to>`, preserving every other line verbatim. The line number
// comes from the parse pass over the same file, so it is always in range. This
// mirrors the per-block status rewrite in pkg/lint (rewriteTaskStatusLines).
//
// extraFields (already formatted "**Name:** value" strings, in caller-supplied
// order — see buildExtraTaskFieldLines) are inserted immediately after the
// status line, in the same atomic write, so the status flip and any
// provenance/note/evidence writes land together. A nil/empty extraFields is a
// pure status rewrite.
func rewritePlanTaskStatusLine(path string, statusLine int, to lifecycle.Status, extraFields []string) error {
	return lifecycle.TransformArtifact(path, func(before []byte) ([]byte, error) {
		return rewritePlanTaskStatusLineBytes(before, statusLine, to, extraFields)
	})
}

func rewritePlanTaskStatusLineBytes(data []byte, statusLine int, to lifecycle.Status, extraFields []string) ([]byte, error) {
	lines := strings.Split(string(data), "\n")
	if statusLine < 1 || statusLine > len(lines) || !strings.HasPrefix(strings.TrimSpace(lines[statusLine-1]), "**Status:**") {
		return nil, lifecycle.ErrConcurrentMutation
	}
	lines[statusLine-1] = "**Status:** " + string(to)
	end := len(lines)
	for i := statusLine; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "### Task ") || strings.HasPrefix(trimmed, "## ") {
			end = i
			break
		}
	}
	lines = upsertTaskFieldLines(lines, statusLine-1, end, extraFields)
	return []byte(strings.Join(lines, "\n")), nil
}

func taskFieldName(line string) string {
	trimmed := strings.TrimSpace(line)
	for _, name := range []string{"Implemented-by", "Note", "Evidence"} {
		if strings.HasPrefix(trimmed, "**"+name+":**") {
			return name
		}
	}
	return ""
}

// upsertTaskFieldLines replaces only the singleton fields explicitly supplied
// by this transaction, within one already-resolved task block.
func upsertTaskFieldLines(lines []string, statusIdx, end int, fields []string) []string {
	if len(fields) == 0 {
		return lines
	}
	replace := map[string]bool{}
	for _, field := range fields {
		if name := taskFieldName(field); name != "" {
			replace[name] = true
		}
	}
	out := make([]string, 0, len(lines)+len(fields))
	for i, line := range lines {
		if i > statusIdx && i < end && replace[taskFieldName(line)] {
			continue
		}
		out = append(out, line)
	}
	return withExtraFieldLines(out, statusIdx, fields)
}

func validateTaskSingletonLines(lines []string, start, end int, label string) (int, error) {
	counts := map[string]int{}
	statusIdx := -1
	for i := start; i < end; i++ {
		trimmed := strings.TrimSpace(lines[i])
		if strings.HasPrefix(trimmed, "**Status:**") {
			counts["Status"]++
			statusIdx = i
		}
		if name := taskFieldName(trimmed); name != "" {
			counts[name]++
		}
	}
	if counts["Status"] == 0 {
		return -1, errNoStatusForProvenance
	}
	for _, name := range []string{"Status", "Implemented-by", "Note", "Evidence"} {
		if counts[name] > 1 {
			return -1, exitcode.InvalidArgsErrorf("%s has duplicate **%s:** fields", label, name)
		}
	}
	return statusIdx, nil
}

// isImplementedByLine reports whether ln is an `**Implemented-by:**` field line.
func isImplementedByLine(ln string) bool {
	return strings.HasPrefix(strings.TrimSpace(ln), "**Implemented-by:**")
}

// withAmendedImplementedBy re-stamps the provenance field anchored to
// lines[statusIdx]: it drops an existing `**Implemented-by:**` line sitting
// immediately after the status line (where both the board and plan-inline
// completion paths write it), then inserts `**Implemented-by:** ref` when ref is
// non-empty. An empty ref therefore CLEARS the field.
func withAmendedImplementedBy(lines []string, statusIdx int, ref string) []string {
	if statusIdx+1 < len(lines) && isImplementedByLine(lines[statusIdx+1]) {
		out := make([]string, 0, len(lines)-1)
		out = append(out, lines[:statusIdx+1]...)
		out = append(out, lines[statusIdx+2:]...)
		lines = out
	}
	if ref != "" {
		lines = withImplementedByLine(lines, statusIdx, ref)
	}
	return lines
}

// boardTaskStatus returns the value text of the board task file's first
// `**Status:**` line. A read error is returned verbatim; a file with no status
// line returns errNoStatusForProvenance.
func boardTaskStatusBytes(data []byte) (string, error) {
	lines := strings.Split(string(data), "\n")
	idx, err := validateTaskSingletonLines(lines, 0, len(lines), "board task")
	if err != nil {
		return "", err
	}
	s := strings.TrimSpace(lines[idx])
	return strings.TrimSpace(strings.TrimPrefix(s, "**Status:**")), nil
}

// amendBoardImplementedBy re-stamps (or clears) the `**Implemented-by:**` field
// in the board task file at path, anchored to its `**Status:**` line.
func amendBoardImplementedBy(path, ref string) error {
	return lifecycle.TransformArtifact(path, func(before []byte) ([]byte, error) {
		return amendBoardImplementedByBytes(before, ref)
	})
}

func amendBoardImplementedByBytes(data []byte, ref string) ([]byte, error) {
	lines := strings.Split(string(data), "\n")
	idx := -1
	for i, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), "**Status:**") {
			idx = i
			break
		}
	}
	if idx < 0 {
		return nil, errNoStatusForProvenance
	}
	lines = withAmendedImplementedBy(lines, idx, ref)
	return []byte(strings.Join(lines, "\n")), nil
}

// runTaskAmendProvenanceBoard re-stamps provenance on a board task that is
// ALREADY complete, without a status transition. The task must be in complete
// (else exit 4); the ref is assembled from the same flags as the completion
// path, and an empty ref clears the field. Prints "<task>: provenance amended".
func runTaskAmendProvenanceBoard(cmd *cobra.Command, taskSlug string, deps taskMutationDeps) error {
	projectFlag, _ := cmd.Flags().GetString("project")
	tasksDir, err := resolveTasksDir(projectFlag)
	if err != nil {
		return err
	}

	taskFilePath := filepath.Join(tasksDir, taskSlug, "README.md")
	if err := deps.transformArtifact(taskFilePath, func(before []byte) ([]byte, error) {
		status, err := boardTaskStatusBytes(before)
		if err != nil {
			return nil, err
		}
		if status != string(lifecycle.TaskComplete) {
			return nil, exitcode.InvalidStateErrorf("--amend-provenance requires task %s to be complete, but it is %s", taskSlug, status)
		}
		return amendBoardImplementedByBytes(before, implementedByRefFromFlags(cmd))
	}); err != nil {
		if errors.Is(err, errNoStatusForProvenance) {
			return exitcode.UnexpectedErrorf("task %s has no **Status:** line", taskSlug)
		}
		if errors.Is(err, os.ErrNotExist) {
			return exitcode.NotFoundErrorf("task not found: %s", taskSlug)
		}
		var stateErr *exitcode.Error
		if errors.As(err, &stateErr) {
			return err
		}
		return exitcode.UnexpectedErrorCause(fmt.Sprintf("amending provenance: %v", err), err)
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: provenance amended\n", taskSlug)
	return nil
}

// runTaskAmendProvenancePlanInline re-stamps provenance on the plan-inline task
// block whose **Id:** equals taskSlug, without a status transition. The block
// must be in complete (else exit 4); an empty assembled ref clears the field.
func runTaskAmendProvenancePlanInline(cmd *cobra.Command, taskSlug, planSlug string, deps taskMutationDeps) error {
	projectFlag, _ := cmd.Flags().GetString("project")
	specRoot, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}

	planPath := filepath.Join(specRoot, "spec", "plans", planSlug+".md")
	var coordinationWarning bytes.Buffer
	err = deps.transformArtifact(planPath, func(before []byte) ([]byte, error) {
		p, err := plan.ParseBytes(planPath, before)
		if err != nil {
			return nil, err
		}
		forceCoordination, _ := cmd.Flags().GetBool(coordinationForceFlagName)
		if err := enforceCoordinationBranch(p, specRoot, forceCoordination, &coordinationWarning); err != nil {
			return nil, err
		}
		target, err := uniquePlanTaskByID(p, taskSlug)
		if err != nil {
			return nil, err
		}
		if string(target.Status) != string(lifecycle.TaskComplete) {
			return nil, exitcode.InvalidStateErrorf("--amend-provenance requires task %q to be complete, but it is %s", taskSlug, target.Status)
		}
		return deps.amendPlanProvenance(before, target.StatusLine, implementedByRefFromFlags(cmd))
	})
	if err != nil {
		var committed *lifecycle.CommittedMutationError
		if errors.As(err, &committed) {
			_, _ = cmd.ErrOrStderr().Write(coordinationWarning.Bytes())
			return exitcode.UnexpectedErrorCause(fmt.Sprintf("amending provenance: %v", err), err)
		}
		if errors.Is(err, os.ErrNotExist) {
			return exitcode.NotFoundErrorf("plan not found: %s", planSlug)
		}
		var coded *exitcode.Error
		if errors.As(err, &coded) {
			return err
		}
		return exitcode.UnexpectedErrorCause(fmt.Sprintf("amending provenance: %v", err), err)
	}
	_, _ = cmd.ErrOrStderr().Write(coordinationWarning.Bytes())
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: provenance amended\n", taskSlug)
	return nil
}

// amendPlanImplementedBy re-stamps (or clears) the `**Implemented-by:**` field
// adjacent to the 1-based statusLine of a plan file, preserving every other line.
func amendPlanImplementedBy(path string, statusLine int, ref string) error {
	return lifecycle.TransformArtifact(path, func(before []byte) ([]byte, error) {
		return amendPlanImplementedByBytes(before, statusLine, ref)
	})
}

func amendPlanImplementedByBytes(data []byte, statusLine int, ref string) ([]byte, error) {
	lines := strings.Split(string(data), "\n")
	if statusLine < 1 || statusLine > len(lines) || !strings.HasPrefix(strings.TrimSpace(lines[statusLine-1]), "**Status:**") {
		return nil, lifecycle.ErrConcurrentMutation
	}
	lines = withAmendedImplementedBy(lines, statusLine-1, ref)
	return []byte(strings.Join(lines, "\n")), nil
}

func uniquePlanTaskByID(p *plan.Plan, id string) (*plan.Task, error) {
	var found *plan.Task
	for i := range p.Tasks {
		if p.Tasks[i].IdCount > 1 {
			return nil, exitcode.InvalidArgsErrorf("plan Task %d has duplicate **Id:** fields", p.Tasks[i].Number)
		}
		for _, field := range []struct {
			name  string
			count int
		}{
			{"Status", p.Tasks[i].StatusCount}, {"Implemented-by", p.Tasks[i].ImplementedByCount},
			{"Note", p.Tasks[i].NoteCount}, {"Evidence", p.Tasks[i].EvidenceCount},
		} {
			if field.count > 1 {
				return nil, exitcode.InvalidArgsErrorf("plan Task %d has duplicate **%s:** fields", p.Tasks[i].Number, field.name)
			}
		}
		if p.Tasks[i].IdPresent && p.Tasks[i].Id == id {
			if found != nil {
				return nil, exitcode.InvalidArgsErrorf("plan has duplicate Task **Id:** %q", id)
			}
			found = &p.Tasks[i]
		}
	}
	if found == nil {
		return nil, exitcode.NotFoundErrorf("task %q not found by **Id:** in plan", id)
	}
	return found, nil
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

func taskNewCommand(deps taskMutationDeps) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new",
		Short: "Create a new task",
		Long: `Creates a new task by writing tasks/{slug}/README.md and adding a row
to the tasks/README.md board. New tasks are always created in "planning" status.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error { return runTaskNew(cmd, args, deps) },
	}
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().String("task", "", "task slug (required)")
	cmd.Flags().String("title", "", "task title (required)")
	cmd.Flags().String("description", "", "task description")
	cmd.Flags().String("depends-on", "", "comma-separated list of dependency slugs")
	cmd.Flags().String("format", "yaml", "output format: yaml, json")
	return cmd
}

func runTaskNew(cmd *cobra.Command, _ []string, mutationDeps taskMutationDeps) error {
	projectFlag, _ := cmd.Flags().GetString("project")
	taskFlag, _ := cmd.Flags().GetString("task")
	titleFlag, _ := cmd.Flags().GetString("title")
	descFlag, _ := cmd.Flags().GetString("description")
	depsFlag, _ := cmd.Flags().GetString("depends-on")
	formatFlag, _ := cmd.Flags().GetString("format")

	if taskFlag == "" {
		return exitcode.InvalidArgsError("missing required flag: --task")
	}
	if err := task.ValidateSlug(taskFlag); err != nil {
		return exitcode.InvalidArgsErrorf("invalid --task: %v", err)
	}
	if titleFlag == "" {
		return exitcode.InvalidArgsError("missing required flag: --title")
	}
	if formatFlag != "yaml" && formatFlag != "json" {
		return exitcode.InvalidArgsErrorf("invalid format: %s (supported: yaml, json)", formatFlag)
	}

	// Parse --depends-on
	dependencySlugs := []string{}
	if depsFlag != "" {
		for _, d := range strings.Split(depsFlag, ",") {
			d = strings.TrimSpace(d)
			if d != "" {
				if err := task.ValidateSlug(d); err != nil {
					return exitcode.InvalidArgsErrorf("invalid --depends-on task %q: %v", d, err)
				}
				dependencySlugs = append(dependencySlugs, d)
			}
		}
	}

	tasksDir, err := resolveTasksDir(projectFlag)
	if err != nil {
		return err
	}

	taskDir := filepath.Join(tasksDir, taskFlag)
	tfd := task.TaskFileData{
		Title:       titleFlag,
		Description: descFlag,
		DependsOn:   dependencySlugs,
	}
	taskFilePath := filepath.Join(taskDir, "README.md")
	boardPath := filepath.Join(tasksDir, "README.md")
	taskBytes := task.RenderTaskFile(tfd)
	markerPath := taskNewMarkerPath(tasksDir, taskFlag)
	var marker taskNewPrepared
	var markerBytes []byte
	out := taskInfoOutput{
		Slug: taskFlag, Title: titleFlag, Status: string(task.StatusPlanning),
		Description: descFlag, DependsOn: dependencySlugs,
	}
	// The board path lock exists even before its README does. It fences the
	// complete allocation/publication transaction, so concurrent creators cannot
	// leave an unlisted README or lose an appended row.
	if err := mutationDeps.withArtifactTx(boardPath, func(tx *lifecycle.ArtifactTransaction) error {
		boardData := tx.Before()
		bv, err := task.ParseBoard(boardData)
		if err != nil {
			return err
		}
		rowCount := 0
		for _, row := range bv.Rows {
			if row.Task == taskFlag {
				rowCount++
			}
		}
		if rowCount > 1 {
			return exitcode.ConflictErrorf("task board contains duplicate rows for %s", taskFlag)
		}
		if err := validateTaskNewDependencies(tasksDir, taskFlag, dependencySlugs, bv.Rows, mutationDeps); err != nil {
			return err
		}
		intendedRow := task.BoardRow{Task: taskFlag, Status: task.StatusPlanning, DependsOn: dependencySlugs}
		intendedBoard := *bv
		intendedBoard.Rows = append(append([]task.BoardRow(nil), bv.Rows...), intendedRow)
		boardAfter := task.RenderBoard(&intendedBoard)

		existingMarker, markerExists, committedReceiptExists, markerErr := readTaskNewRecoveryMarker(markerPath)
		if markerErr != nil {
			return markerErr
		}
		_, taskDirErr := os.Stat(taskDir)
		taskDirExists := taskDirErr == nil
		if taskDirErr != nil && !os.IsNotExist(taskDirErr) {
			if markerExists {
				return lifecycle.CommittedError(markerPath, "checking prepared task directory", taskDirErr)
			}
			return taskDirErr
		}
		readme, readmeErr := os.ReadFile(taskFilePath)
		readmeExists := readmeErr == nil
		if readmeErr != nil && !os.IsNotExist(readmeErr) {
			if markerExists {
				return lifecycle.CommittedError(markerPath, "reading prepared task README", readmeErr)
			}
			return readmeErr
		}

		if !markerExists {
			// Fresh conflicts are rejected before a prepared record is created.
			if rowCount != 0 || taskDirExists || readmeExists {
				return exitcode.ConflictErrorf("task already exists: %s", taskFlag)
			}
			marker = taskNewPrepared{SchemaVersion: 2, TaskID: taskFlag, TaskPath: filepath.ToSlash(filepath.Join(taskFlag, "README.md")), ContentSHA256: taskNewDigest(taskBytes), BoardBeforeSHA256: taskNewDigest(boardData), BoardAfterSHA256: taskNewDigest(boardAfter)}
			markerBytes = marker.bytes()
			if err := mutationDeps.publishExclusive(markerPath, markerBytes, 0o600); err != nil {
				return fmt.Errorf("publishing prepared task marker: %w", err)
			}
		} else {
			if err := json.Unmarshal(existingMarker, &marker); err != nil || marker.SchemaVersion != 2 || marker.TaskID != taskFlag || marker.TaskPath != filepath.ToSlash(filepath.Join(taskFlag, "README.md")) || marker.ContentSHA256 != taskNewDigest(taskBytes) {
				return exitcode.ConflictErrorf("task creation recovery marker belongs to a different intent: %s", taskFlag)
			}
			markerBytes = existingMarker
		}

		// A committed receipt means finalization began only after the board row
		// was visible. It is cleanup evidence, never authority to reconstruct a
		// missing or changed commit. Fail closed unless every visible byte still
		// matches the postimage bound into that receipt.
		if committedReceiptExists && rowCount != 1 {
			return exitcode.ConflictErrorf("committed task %s is missing its exact board postimage", taskFlag)
		}
		if rowCount == 1 {
			if !readmeExists || taskNewDigest(readme) != marker.ContentSHA256 || !taskNewRowMatches(bv.Rows, intendedRow) || taskNewDigest(boardData) != marker.BoardAfterSHA256 {
				return exitcode.ConflictErrorf("committed task %s does not match its prepared content", taskFlag)
			}
			return finishTaskNewVisibleTransaction(cmd, formatFlag, out, markerPath, markerBytes, mutationDeps)
		}
		if taskNewDigest(boardData) != marker.BoardBeforeSHA256 {
			return exitcode.ConflictErrorf("task board changed after preparing %s; recovery required", taskFlag)
		}
		if readmeExists {
			if taskNewDigest(readme) != marker.ContentSHA256 {
				return exitcode.ConflictErrorf("task %s was replaced outside prepared creation", taskFlag)
			}
		} else if err := mutationDeps.publishExclusive(taskFilePath, taskBytes, 0o644); err != nil {
			return lifecycle.CommittedError(markerPath, "publishing prepared task README", err)
		}
		// Directory-only dependencies are a legacy fallback outside the board's
		// lock domain. Revalidate them immediately before the board commit; board
		// rows themselves remain authoritative under this held lock.
		if err := validateTaskNewDependencies(tasksDir, taskFlag, dependencySlugs, bv.Rows, mutationDeps); err != nil {
			return lifecycle.CommittedError(markerPath, "revalidating prepared task dependencies", err)
		}
		if taskNewDigest(boardAfter) != marker.BoardAfterSHA256 {
			return lifecycle.CommittedError(markerPath, "validating prepared task board postimage", lifecycle.ErrConcurrentMutation)
		}
		if err := mutationDeps.commitBoard(tx, boardAfter); err != nil {
			return lifecycle.CommittedError(markerPath, "committing prepared task board", err)
		}
		return finishTaskNewVisibleTransaction(cmd, formatFlag, out, markerPath, markerBytes, mutationDeps)
	}); err != nil {
		var committed *lifecycle.CommittedMutationError
		if errors.As(err, &committed) {
			return exitcode.UnexpectedErrorCause(err.Error(), err)
		}
		var coded *exitcode.Error
		if errors.As(err, &coded) {
			return err
		}
		if errors.Is(err, lifecycle.ErrConcurrentMutation) {
			return exitcode.ConflictErrorf("task board is busy; retry creation of %s from fresh state", taskFlag)
		}
		if errors.Is(err, os.ErrNotExist) {
			return exitcode.NotFoundErrorf("reading board: %v", err)
		}
		return exitcode.UnexpectedErrorCause(fmt.Sprintf("writing board: %v", err), err)
	}

	return nil
}

func taskNewRowMatches(rows []task.BoardRow, expected task.BoardRow) bool {
	for _, row := range rows {
		if row.Task != expected.Task {
			continue
		}
		if row.Status != expected.Status || row.Branch != "" || row.Agent != "" || row.Requester != "" || row.Time != "" || len(row.DependsOn) != len(expected.DependsOn) {
			return false
		}
		for i := range row.DependsOn {
			if row.DependsOn[i] != expected.DependsOn[i] {
				return false
			}
		}
		return true
	}
	return false
}

func readTaskNewRecoveryMarker(markerPath string) ([]byte, bool, bool, error) {
	prepared, preparedErr := os.ReadFile(markerPath)
	committed, committedErr := os.ReadFile(committedMarkerPath(markerPath))
	preparedExists := preparedErr == nil
	committedExists := committedErr == nil
	if preparedErr != nil && !os.IsNotExist(preparedErr) {
		return nil, false, false, preparedErr
	}
	if committedErr != nil && !os.IsNotExist(committedErr) {
		return nil, false, false, committedErr
	}
	if preparedExists && committedExists && !bytes.Equal(prepared, committed) {
		return nil, false, false, exitcode.ConflictError("task creation prepared and committed receipts disagree")
	}
	if preparedExists {
		return prepared, true, committedExists, nil
	}
	if committedExists {
		return committed, true, true, nil
	}
	return nil, false, false, nil
}

func validateTaskNewDependencies(tasksDir, taskSlug string, deps []string, rows []task.BoardRow, mutationDeps taskMutationDeps) error {
	boardTasks := make(map[string]struct{}, len(rows))
	for _, row := range rows {
		boardTasks[row.Task] = struct{}{}
	}
	for _, dep := range deps {
		if dep == taskSlug {
			return exitcode.NotFoundErrorf("dependency task not found: %s", dep)
		}
		if _, ok := boardTasks[dep]; ok {
			continue
		}
		info, err := mutationDeps.stat(filepath.Join(tasksDir, dep))
		if err == nil && info.IsDir() {
			continue
		}
		if err == nil {
			return exitcode.NotFoundErrorf("dependency task %s is not a directory", dep)
		}
		if os.IsNotExist(err) {
			return exitcode.NotFoundErrorf("dependency task not found: %s", dep)
		}
		return exitcode.UnexpectedErrorCause(fmt.Sprintf("checking dependency task %s: %v", dep, err), err)
	}
	return nil
}

// finishTaskNewVisibleTransaction runs only after the task row is visible,
// while the board transaction lock is still held. Output failure therefore
// retains the exact prepared marker for retry; successful output is followed
// by exact-owned durable marker finalization.
func finishTaskNewVisibleTransaction(cmd *cobra.Command, formatFlag string, out taskInfoOutput, markerPath string, markerBytes []byte, deps taskMutationDeps) error {
	w := cmd.OutOrStdout()
	var outputErr error
	switch formatFlag {
	case "yaml":
		enc := deps.newYAMLEncoder(w)
		if err := enc.Encode(out); err != nil {
			outputErr = fmt.Errorf("encoding yaml: %w", err)
		} else if err := enc.Close(); err != nil {
			outputErr = fmt.Errorf("closing yaml output: %w", err)
		}
	default:
		if err := deps.newJSONEncoder(w).Encode(out); err != nil {
			outputErr = fmt.Errorf("encoding json: %w", err)
		}
	}
	if outputErr != nil {
		return outputErr
	}
	if err := deps.removeMarker(markerPath, markerBytes); err != nil {
		return fmt.Errorf("finalizing prepared task marker: %w", err)
	}
	return nil
}
