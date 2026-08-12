package cli

import (
	"bytes"
	"errors"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
	"github.com/specscore/specscore-cli/pkg/plan"
	"github.com/spf13/cobra"
)

// planReconcileCommand returns the "plan reconcile" command. It is a
// deliberately DISTINCT verb from `plan change-status`: change-status walks a
// plan through the human-authored legal-transition matrix one arc at a time;
// reconcile is the out-of-band correction for when work landed outside the
// tracked flow and the record fell behind reality. It never silently re-walks
// intermediate statuses to fake a history that never happened — it writes the
// corrected state directly and marks the artifact (a **Reconciled:** header
// line plus a dated `## Resolution` paragraph) so a reader always knows this
// status was reconciled, not earned through the normal arc.
// See spec/features/cli/plan/reconcile/README.md.
func planReconcileCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "reconcile <slug> --tasks=complete --note=<text> [--evidence=<ref>[,<ref>...]]",
		Short: "Correct a Plan's recorded status to match reality (out-of-band, not a tracked transition)",
		Long: `Reconciles spec/plans/<slug>.md when its recorded **Status:** — and its
embedded tasks' **Status:** lines — fell behind what was actually delivered,
because the work landed outside the tracked ` + "`change-status`" + ` flow.
Unlike ` + "`change-status`" + `, reconcile does not walk the legal-transition
matrix one arc at a time: it writes the corrected state directly, and it
always records that the jump happened out of band — a **Reconciled:** header
marker plus a dated ` + "`## Resolution`" + ` paragraph are written on every
successful run, so a reader can never mistake a reconciled status for one that
followed the normal flow.

--tasks=complete is currently the only supported value: every ` + "`### Task N:`" + `
block's **Status:** is set to complete, and the plan's own **Status:** is set
to the execution band that then derives from the rollup — Implemented, since a
plan with every task complete always derives Implemented.

When a prior reconciliation overstated delivery, ` + "`--reopen-tasks=10,11`" + `
is the deliberately narrow correction path: each named task MUST currently be
complete and is rewritten directly to blocked; every other task is preserved and
the Plan status is re-derived. It cannot be combined with --tasks or
--force-tasks. The mandatory note/evidence record why the earlier claim was false.

--note is REQUIRED: reconcile refuses (exit 2) without a justification
explaining why the record is being corrected. --evidence is optional — a
comma-separated list of commit SHAs, PR URLs, or file paths backing the
claim — and is recorded alongside --note when supplied.

A task recorded as failed or aborted is NEVER silently swept into "every
task": --tasks=complete refuses (exit 4) to touch it unless its number is
named via --force-tasks=<n>[,<n>...], and every task overridden this way is
itemized by number and prior status in the ` + "`## Resolution`" + ` paragraph,
not just folded into the aggregate count. A blocked task has no such
guard — it is not a terminal claim, so completing it needs no acknowledgement.

Reconcile refuses (exit 4) when: the plan has no embedded tasks; any task is
missing an explicit **Status:** line; a task is failed/aborted and not named
in --force-tasks; the plan is in a terminal disposition status
(Rejected/Withdrawn/Superseded/Deprecated — reconcile does not resurrect
those); the plan uses the directory form (spec/plans/<slug>/README.md,
unsupported by this verb); or nothing would actually change — the plan is
already reconciled, so re-running is a no-op refusal rather than a silent
success.

When the plan declares **Coordination:** <owner>/<repo>@<branch>, this verb
refuses (exit 1) unless the current git repo/branch matches — reconciling a
coordinated plan from anywhere else is a process violation, not a merge
chore. --force-coordination bypasses the check for this invocation only,
printing a warning naming what was bypassed.

The Plan status, every selected Task status, the reconciliation marker, and
the Resolution audit are composed under one fail-fast Plan artifact lock and
published with one atomic durable write. Lint/index work begins only after the
lock is released. If it fails, the committed reconciliation remains visible
and the command returns a recovery-required error; it never rolls back stale
bytes.

Examples:

  specscore plan reconcile auth --tasks=complete --note "implemented directly during the incident; tracked flow was skipped"
  specscore plan reconcile auth --tasks=complete --note "shipped ahead of the record" --evidence a1b2c3d,https://github.com/org/repo/pull/42
  specscore plan reconcile auth --tasks=complete --note "task 3 actually landed after all, on a later attempt" --force-tasks=3
`,
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runPlanReconcile,
	}
	cmd.Flags().String("tasks", "", "which embedded tasks to reconcile (required). Only supported value: complete")
	cmd.Flags().String("note", "", "required justification for the reconciliation; written to a ## Resolution section")
	cmd.Flags().String("evidence", "", "optional comma-separated commit SHAs / PR URLs / file paths backing the reconciliation")
	cmd.Flags().String("force-tasks", "", "comma-separated task numbers to explicitly acknowledge overriding from failed/aborted to complete; required only when such tasks exist")
	cmd.Flags().String("reopen-tasks", "", "comma-separated falsely-complete task numbers to correct to blocked; cannot be combined with --tasks")
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().Bool(coordinationForceFlagName, false, coordinationForceFlagUsage)
	return cmd
}

func runPlanReconcile(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return exitcode.InvalidArgsError("missing required positional argument: <slug>")
	}
	if len(args) > 1 {
		return exitcode.InvalidArgsErrorf(
			"too many positional arguments: reconcile accepts exactly one <slug>, got %d", len(args))
	}
	slug := args[0]
	if err := plan.ValidateSlug(slug); err != nil {
		return exitcode.InvalidArgsErrorf("invalid slug %q: %v", slug, err)
	}

	tasksRaw, _ := cmd.Flags().GetString("tasks")
	tasksRaw = strings.TrimSpace(tasksRaw)
	reopenRaw, _ := cmd.Flags().GetString("reopen-tasks")
	reopenRaw = strings.TrimSpace(reopenRaw)
	if tasksRaw == "" && reopenRaw == "" {
		return exitcode.InvalidArgsError("missing required flag: --tasks=complete")
	}
	if tasksRaw != "" && reopenRaw != "" {
		return exitcode.InvalidArgsError("--tasks and --reopen-tasks are mutually exclusive")
	}
	if tasksRaw != "" && !strings.EqualFold(tasksRaw, "complete") {
		return exitcode.InvalidArgsErrorf(
			"unrecognized --tasks value %q; only \"complete\" is currently supported (reconciles every embedded task to complete)", tasksRaw)
	}

	note, _ := cmd.Flags().GetString("note")
	if strings.TrimSpace(note) == "" {
		return exitcode.InvalidArgsError(
			"reconcile requires a reason: pass --note explaining why the plan is being reconciled (what was actually delivered, and why the tracked flow was skipped)")
	}

	evidenceRaw, _ := cmd.Flags().GetString("evidence")
	var evidence []string
	for _, e := range strings.Split(evidenceRaw, ",") {
		e = strings.TrimSpace(e)
		if e != "" {
			evidence = append(evidence, e)
		}
	}

	forceTasksRaw, _ := cmd.Flags().GetString("force-tasks")
	var forceTasks []int
	for _, tok := range strings.Split(forceTasksRaw, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		n, convErr := strconv.Atoi(tok)
		if convErr != nil || n <= 0 {
			return exitcode.InvalidArgsErrorf(
				"invalid --force-tasks value %q: must be a comma-separated list of positive task numbers", tok)
		}
		forceTasks = append(forceTasks, n)
	}
	var reopenTasks []int
	for _, tok := range strings.Split(reopenRaw, ",") {
		tok = strings.TrimSpace(tok)
		if tok == "" {
			continue
		}
		n, convErr := strconv.Atoi(tok)
		if convErr != nil || n <= 0 {
			return exitcode.InvalidArgsErrorf("invalid --reopen-tasks value %q: must be a comma-separated list of positive task numbers", tok)
		}
		reopenTasks = append(reopenTasks, n)
	}
	if len(reopenTasks) > 0 && len(forceTasks) > 0 {
		return exitcode.InvalidArgsError("--force-tasks is only valid with --tasks=complete")
	}

	projectFlag, _ := cmd.Flags().GetString("project")
	specRoot, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}

	forceCoordination, _ := cmd.Flags().GetBool(coordinationForceFlagName)
	var coordinationWarning bytes.Buffer

	result, err := plan.Reconcile(plan.ReconcileOptions{
		SpecRoot:    specRoot,
		Slug:        slug,
		Note:        note,
		Evidence:    evidence,
		ForceTasks:  forceTasks,
		ReopenTasks: reopenTasks,
		ValidateSnapshot: func(path string, before []byte) error {
			parsedPlan, parseErr := plan.ParseBytes(path, before)
			if parseErr != nil {
				return exitcode.UnexpectedErrorCause(fmt.Sprintf("parsing plan %s: %v", slug, parseErr), parseErr)
			}
			return enforceCoordinationBranch(parsedPlan, specRoot, forceCoordination, &coordinationWarning)
		},
		PostMutation: plan.PostMutationHook(lintPostMutationHook(filepath.Join(specRoot, "spec"))),
	})
	if err != nil {
		var committed *lifecycle.CommittedMutationError
		if errors.As(err, &committed) {
			_, _ = cmd.ErrOrStderr().Write(coordinationWarning.Bytes())
			return exitcode.UnexpectedErrorCause(err.Error(), err)
		}
		return err
	}
	_, _ = cmd.ErrOrStderr().Write(coordinationWarning.Bytes())

	msg := fmt.Sprintf("%s: %s → %s (reconciled, %d task(s) marked %s",
		result.Slug, string(result.From), string(result.To), result.TasksReconciled, result.Target)
	if len(result.Overrides) > 0 {
		parts := make([]string, len(result.Overrides))
		for i, o := range result.Overrides {
			parts[i] = fmt.Sprintf("Task %d was %s", o.Number, string(o.From))
		}
		msg += fmt.Sprintf(", including %d forced from a terminal state: %s", len(result.Overrides), strings.Join(parts, "; "))
	}
	msg += ")\n"
	_, _ = fmt.Fprint(cmd.OutOrStdout(), msg)
	return nil
}
