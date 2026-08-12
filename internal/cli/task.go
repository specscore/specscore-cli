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
		taskAmendCommand(),
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
// Both board-mode (tasks/<task>/README.md) and plan-inline (--plan) resolution
// are wired here. On --to=complete the --repo/--commit/--branch flags assemble
// an **Implemented-by:** provenance field written alongside the status.
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
rewrite of the **Status:** line and prints "<task>: <from> → <to>".

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
		RunE:          runTaskChangeStatus,
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

func runTaskChangeStatus(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return exitcode.InvalidArgsError("missing required positional argument: <task>")
	}
	if len(args) > 1 {
		return exitcode.InvalidArgsErrorf(
			"too many positional arguments: change-status accepts exactly one <task>, got %d", len(args))
	}
	taskSlug := args[0]

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
		if planSlug, _ := cmd.Flags().GetString("plan"); strings.TrimSpace(planSlug) != "" {
			return runTaskAmendProvenancePlanInline(cmd, taskSlug, strings.TrimSpace(planSlug))
		}
		return runTaskAmendProvenanceBoard(cmd, taskSlug)
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
	if planSlug, _ := cmd.Flags().GetString("plan"); strings.TrimSpace(planSlug) != "" {
		return runTaskChangeStatusPlanInline(cmd, taskSlug, strings.TrimSpace(planSlug), to)
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
	from, err := rewriteBoardTaskFn(taskFilePath, to, fields)
	if err != nil {
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
			return exitcode.UnexpectedErrorf("rewriting task: %v", err)
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
	return lifecycle.WithArtifactMutationLock(path, func() error {
		return writeBoardExtraFieldsUnlocked(path, fields)
	})
}

// rewriteBoardTask applies status, provenance, and transition annotations in
// one lifecycle critical section. The Validate read is deliberately inside the
// fence: no writer may validate an old task body then overwrite an amendment.
func rewriteBoardTask(path string, to lifecycle.Status, fields []string) (lifecycle.Status, error) {
	var from lifecycle.Status
	err := lifecycle.WithArtifactMutationLock(path, func() error {
		var err error
		from, err = lifecycle.Validate(lifecycle.KindTask, path, to)
		if err != nil {
			return err
		}
		if _, err = lifecycle.RewriteUnderLock(path, to); err != nil {
			return err
		}
		if len(fields) == 0 {
			return nil
		}
		return writeBoardExtraFieldsUnlocked(path, fields)
	})
	return from, err
}

func writeBoardExtraFieldsUnlocked(path string, fields []string) error {
	data, err := osReadFileFn(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	idx := -1
	for i, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), "**Status:**") {
			idx = i
			break
		}
	}
	if idx < 0 {
		return errNoStatusForProvenance
	}
	lines = withExtraFieldLines(lines, idx, fields)
	return osWriteFileFn(path, []byte(strings.Join(lines, "\n")), 0o644)
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

	// Coordination-branch enforcement: when the plan declares
	// **Coordination:**, mutating one of its inline tasks is authoritative
	// only on the declared repo/branch
	// (spec/features/plan/README.md#coordination-branch, upstream).
	forceCoordination, _ := cmd.Flags().GetBool(coordinationForceFlagName)
	if cerr := enforceCoordinationBranch(p, specRoot, forceCoordination, cmd.ErrOrStderr()); cerr != nil {
		return cerr
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
	if err := rewritePlanTaskStatusLineFn(planPath, target.StatusLine, to, extraFields); err != nil {
		if errors.Is(err, lifecycle.ErrConcurrentMutation) {
			return exitcode.ConflictErrorf("task %s changed concurrently; re-read before changing status", taskSlug)
		}
		return exitcode.UnexpectedErrorf("rewriting status: %v", err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s → %s\n", taskSlug, string(from), string(to))
	return nil
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
	return lifecycle.WithArtifactMutationLock(path, func() error {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines := strings.Split(string(data), "\n")
		lines[statusLine-1] = "**Status:** " + string(to)
		lines = withExtraFieldLines(lines, statusLine-1, extraFields)
		return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
	})
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
func boardTaskStatus(path string) (string, error) {
	data, err := osReadFileFn(path)
	if err != nil {
		return "", err
	}
	for _, ln := range strings.Split(string(data), "\n") {
		s := strings.TrimSpace(ln)
		if strings.HasPrefix(s, "**Status:**") {
			return strings.TrimSpace(strings.TrimPrefix(s, "**Status:**")), nil
		}
	}
	return "", errNoStatusForProvenance
}

// amendBoardImplementedBy re-stamps (or clears) the `**Implemented-by:**` field
// in the board task file at path, anchored to its `**Status:**` line.
func amendBoardImplementedBy(path, ref string) error {
	return lifecycle.WithArtifactMutationLock(path, func() error {
		return amendBoardImplementedByUnlocked(path, ref)
	})
}

func amendBoardImplementedByUnlocked(path, ref string) error {
	data, err := osReadFileFn(path)
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	idx := -1
	for i, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), "**Status:**") {
			idx = i
			break
		}
	}
	if idx < 0 {
		return errNoStatusForProvenance
	}
	lines = withAmendedImplementedBy(lines, idx, ref)
	return osWriteFileFn(path, []byte(strings.Join(lines, "\n")), 0o644)
}

// runTaskAmendProvenanceBoard re-stamps provenance on a board task that is
// ALREADY complete, without a status transition. The task must be in complete
// (else exit 4); the ref is assembled from the same flags as the completion
// path, and an empty ref clears the field. Prints "<task>: provenance amended".
func runTaskAmendProvenanceBoard(cmd *cobra.Command, taskSlug string) error {
	projectFlag, _ := cmd.Flags().GetString("project")
	tasksDir, err := resolveTasksDir(projectFlag)
	if err != nil {
		return err
	}

	taskFilePath := filepath.Join(tasksDir, taskSlug, "README.md")
	status, err := boardTaskStatus(taskFilePath)
	if err != nil {
		if errors.Is(err, errNoStatusForProvenance) {
			return exitcode.UnexpectedErrorf("task %s has no **Status:** line", taskSlug)
		}
		return exitcode.NotFoundErrorf("task not found: %s", taskSlug)
	}
	if status != string(lifecycle.TaskComplete) {
		return exitcode.InvalidStateErrorf(
			"--amend-provenance requires task %s to be complete, but it is %s", taskSlug, status)
	}

	if err := amendBoardImplementedBy(taskFilePath, implementedByRefFromFlags(cmd)); err != nil {
		return exitcode.UnexpectedErrorf("amending provenance: %v", err)
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: provenance amended\n", taskSlug)
	return nil
}

// runTaskAmendProvenancePlanInline re-stamps provenance on the plan-inline task
// block whose **Id:** equals taskSlug, without a status transition. The block
// must be in complete (else exit 4); an empty assembled ref clears the field.
func runTaskAmendProvenancePlanInline(cmd *cobra.Command, taskSlug, planSlug string) error {
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

	// Coordination-branch enforcement: see runTaskChangeStatusPlanInline.
	forceCoordination, _ := cmd.Flags().GetBool(coordinationForceFlagName)
	if cerr := enforceCoordinationBranch(p, specRoot, forceCoordination, cmd.ErrOrStderr()); cerr != nil {
		return cerr
	}

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

	if string(target.Status) != string(lifecycle.TaskComplete) {
		return exitcode.InvalidStateErrorf(
			"--amend-provenance requires task %q to be complete, but it is %s", taskSlug, string(target.Status))
	}

	if err := amendPlanImplementedBy(planPath, target.StatusLine, implementedByRefFromFlags(cmd)); err != nil {
		return exitcode.UnexpectedErrorf("amending provenance: %v", err)
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: provenance amended\n", taskSlug)
	return nil
}

// amendPlanImplementedBy re-stamps (or clears) the `**Implemented-by:**` field
// adjacent to the 1-based statusLine of a plan file, preserving every other line.
func amendPlanImplementedBy(path string, statusLine int, ref string) error {
	return lifecycle.WithArtifactMutationLock(path, func() error {
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		lines := strings.Split(string(data), "\n")
		if statusLine < 1 || statusLine > len(lines) || !strings.HasPrefix(strings.TrimSpace(lines[statusLine-1]), "**Status:**") {
			return lifecycle.ErrConcurrentMutation
		}
		lines = withAmendedImplementedBy(lines, statusLine-1, ref)
		return os.WriteFile(path, []byte(strings.Join(lines, "\n")), 0o644)
	})
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
