package cli

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
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

--note is REQUIRED: reconcile refuses (exit 2) without a justification
explaining why the record is being corrected. --evidence is optional — a
comma-separated list of commit SHAs, PR URLs, or file paths backing the
claim — and is recorded alongside --note when supplied.

Reconcile refuses (exit 4) when: the plan has no embedded tasks; any task is
missing an explicit **Status:** line; the plan is in a terminal disposition
status (Rejected/Withdrawn/Superseded/Deprecated — reconcile does not
resurrect those); the plan uses the directory form (spec/plans/<slug>/README.md,
unsupported by this verb); or nothing would actually change — the plan is
already reconciled, so re-running is a no-op refusal rather than a silent
success.

Examples:

  specscore plan reconcile auth --tasks=complete --note "implemented directly during the incident; tracked flow was skipped"
  specscore plan reconcile auth --tasks=complete --note "shipped ahead of the record" --evidence a1b2c3d,https://github.com/org/repo/pull/42
`,
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runPlanReconcile,
	}
	cmd.Flags().String("tasks", "", "which embedded tasks to reconcile (required). Only supported value: complete")
	cmd.Flags().String("note", "", "required justification for the reconciliation; written to a ## Resolution section")
	cmd.Flags().String("evidence", "", "optional comma-separated commit SHAs / PR URLs / file paths backing the reconciliation")
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
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
	if tasksRaw == "" {
		return exitcode.InvalidArgsError("missing required flag: --tasks=complete")
	}
	if !strings.EqualFold(tasksRaw, "complete") {
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

	projectFlag, _ := cmd.Flags().GetString("project")
	specRoot, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}

	result, err := plan.Reconcile(plan.ReconcileOptions{
		SpecRoot:     specRoot,
		Slug:         slug,
		Note:         note,
		Evidence:     evidence,
		PostMutation: plan.PostMutationHook(lintPostMutationHook(filepath.Join(specRoot, "spec"))),
	})
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s → %s (reconciled, %d task(s) marked complete)\n",
		result.Slug, string(result.From), string(result.To), result.TasksReconciled)
	return nil
}
