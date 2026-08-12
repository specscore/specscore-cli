// Package plan — Reconcile is the out-of-band correction path for when a
// Plan's recorded **Status:** (and its embedded tasks' **Status:** lines)
// fell behind reality: work landed outside the tracked `change-status` flow,
// and the ONLY supported way to make the record true was, until now, a hand
// edit — the exact anti-pattern SpecScore's change-status verbs exist to
// eliminate.
//
// Reconcile is deliberately NOT `change-status`: it does not walk the plan
// through the legal-transition matrix one arc at a time (that would require
// silently re-enacting a history that never happened), and it does not touch
// the state machine in pkg/lifecycle at all. It performs one direct rewrite —
// every reconciled task's **Status:** to complete, and the plan's own
// **Status:** to the execution band that rollup then derives (Implemented,
// today's only supported outcome — see DeriveExecutionBand) — and it makes
// that jump impossible to mistake for a normal transition: a **Reconciled:**
// header marker plus a dated, clearly-labeled `## Resolution` paragraph are
// written every time, and a caller-supplied --note justification is
// mandatory. There is no silent-bypass path: every field this function
// requires is enforced before any byte is written. A task recorded as failed
// or aborted is never swept into "every task" by default either — --tasks=complete
// refuses to touch it unless the caller names its number via ForceTasks, and
// every such override is itemized (not just aggregated) in the record.
//
// Cross-references:
//
//   - Verb spec: spec/features/cli/plan/reconcile/README.md
//   - Sibling verb: spec/features/cli/plan/change-status/README.md
//   - Canonical lifecycle: spec/features/plan/README.md#req-status-rollup
package plan

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

// reconciledMarkerPrefix is the header field written once, immediately after
// the plan's **Status:** line, on the FIRST successful reconciliation. It is
// the grep-able, always-visible signal that this plan's status did not arrive
// through the normal `change-status` arc — REQ:reconciled-marker.
const reconciledMarkerPrefix = "**Reconciled:**"

// reconcileTodayUTC is a seam over time.Now so tests can pin the date written
// into the **Reconciled:** marker and the Resolution paragraph.
var reconcileTodayUTC = func() string { return time.Now().UTC().Format("2006-01-02") }

// reconcileReadOriginalFn is a testable indirection for the pre-mutation
// snapshot read. Parse() has already proven the file readable moments
// earlier, so a real filesystem race is the only way this call fails in
// production; tests inject a failure here directly rather than relying on
// timing.
var reconcileReadOriginalFn = os.ReadFile

// reconcileAppendNoteUnderLockFn is separate from the ordinary transition
// seam because Reconcile owns its plan artifact lock across its status rewrite
// and its audit append.
var reconcileAppendNoteUnderLockFn = lifecycle.AppendResolutionNoteUnderLock

// ReconcileOptions packages the inputs to Reconcile.
type ReconcileOptions struct {
	// SpecRoot is the project root that contains the `spec/` subtree (NOT the
	// `spec/` directory itself). The Plan is resolved at
	// SpecRoot/spec/plans/<slug>.md; the optional directory form
	// (.../spec/plans/<slug>/README.md) is NOT supported — embedded-task
	// reconciliation requires the single-file parser (pkg/plan.Parse).
	SpecRoot string

	// Slug is the Plan slug, e.g. "user-auth". Caller is expected to have
	// validated it via plan.ValidateSlug.
	Slug string

	// Note is the mandatory justification for the reconciliation: what was
	// actually delivered, and why the tracked change-status flow was
	// skipped. Required — Reconcile refuses (exit 2) when empty or
	// whitespace-only. Written verbatim into the `## Resolution` section
	// alongside the fixed reconciliation preamble.
	Note string

	// Evidence is an optional list of commit SHAs, PR URLs, or file paths
	// backing the claim that the work is actually done. When non-empty it is
	// recorded as a trailing "Evidence: ..." line in the same Resolution
	// paragraph as Note.
	Evidence []string

	// ForceTasks is the caller's explicit, per-task-number acknowledgement
	// that a task recorded as failed or aborted should be overridden to
	// complete anyway. --tasks=complete never silently force-completes a
	// failed/aborted task: a task in either terminal state whose number does
	// NOT appear here blocks the whole reconciliation (exit 4), naming the
	// offending task numbers and their current statuses. Tasks named here
	// that are NOT actually failed/aborted are harmless no-ops. Every task
	// number that IS overridden this way is recorded — by number and prior
	// status — in the `## Resolution` paragraph, never folded silently into
	// the aggregate "N task(s) marked complete" count.
	ForceTasks []int

	// ReopenTasks is an explicit list of falsely-completed embedded tasks to
	// correct to blocked. It is deliberately narrow: only complete tasks may
	// be reopened, and no unlisted task is touched. This is the inverse audit
	// path for an over-stated prior reconciliation, not a general bypass of
	// the task lifecycle.
	ReopenTasks []int

	// PostMutation is the post-rewrite hook (typically a spec-lint pass,
	// which also syncs the frontmatter `status:` mirror and the plans
	// index). Required; Reconcile returns exit 10 if nil.
	PostMutation PostMutationHook
}

// TaskOverride names an embedded task whose recorded terminal failure state
// (failed or aborted) was overridden to complete via --force-tasks, and what
// it was before. Reconcile never invents these silently — a TaskOverride only
// exists because the caller named that exact task number.
type TaskOverride struct {
	Number int
	From   TaskStatus
}

// ReconcileResult is the success payload returned on exit 0.
type ReconcileResult struct {
	Slug string
	// From is the plan's **Status:** value before reconciliation.
	From lifecycle.Status
	// To is the derived execution-band status the plan is reconciled to.
	// Today this is always lifecycle.PlanImplemented — the only supported
	// --tasks value ("complete") forces every task to StatusComplete, and
	// DeriveExecutionBand's complete-rollup case always yields Implemented.
	To lifecycle.Status
	// TasksReconciled is the count of embedded tasks whose **Status:** line
	// was actually rewritten (tasks already at the target status are left
	// byte-untouched and do not count). This INCLUDES any tasks named in
	// Overrides — they are both counted here and itemized there.
	TasksReconciled int
	// Overrides lists every task whose failed/aborted status was overridden
	// via --force-tasks, by number and prior status. Empty when no task was
	// in a terminal failure state (the common case).
	Overrides []TaskOverride
	// Target is the status written to every reconciled task. Complete is the
	// normal all-task delivery reconciliation; Blocked is used only by the
	// explicit ReopenTasks correction path.
	Target TaskStatus
}

// Reconcile performs a Plan-kind out-of-band status correction end-to-end.
//
// Flow:
//
//  1. Resolve <slug> to an existing flat-form Plan file. A missing file
//     returns exit 3; a directory-form-only plan returns exit 4 (unsupported
//     shape for this verb).
//  2. Parse the plan and reject artifact shapes reconcile cannot handle: a
//     terminal disposition status (Rejected/Withdrawn/Superseded/Deprecated —
//     no resurrection via this verb), zero embedded tasks, or any task
//     missing an explicit **Status:** line. Each exits 4. A task recorded as
//     failed or aborted is ALSO refused unless its number appears in
//     ForceTasks — --tasks=complete never silently force-completes a
//     recorded failure (exit 4, naming the offending tasks and statuses).
//  3. Compute the hypothetical all-complete task rollup and its derived
//     execution band. If nothing would actually change — every task is
//     already complete AND the plan is already at the derived band — exit 4
//     (not idempotent; re-running a completed reconciliation is a no-op
//     refusal, mirroring lifecycle-transitions#req:not-idempotent).
//  4. Rewrite every non-complete task's **Status:** line, the plan's own
//     **Status:** line, and (on the first reconciliation only) insert the
//     **Reconciled:** header marker — one atomic-enough write, all indices
//     resolved from the SAME pre-mutation parse.
//  5. Append the `## Resolution` note: a fixed preamble naming the jump
//     explicitly as a reconciliation (never mistakable for a normal
//     transition), the caller's Note, and the Evidence list when supplied.
//  6. Invoke the PostMutation hook (spec lint --fix + verify). Failure → full
//     rollback to the pre-invocation file bytes, exit propagated as-is
//     (typically 10).
func Reconcile(opts ReconcileOptions) (ReconcileResult, error) {
	if opts.SpecRoot == "" {
		return ReconcileResult{}, exitcode.UnexpectedErrorf("Reconcile: SpecRoot required")
	}
	if opts.Slug == "" {
		return ReconcileResult{}, exitcode.InvalidArgsErrorf("Reconcile: slug required")
	}
	if strings.TrimSpace(opts.Note) == "" {
		return ReconcileResult{}, exitcode.InvalidArgsError(
			"Reconcile: --note required — describe why the plan is being reconciled")
	}
	if opts.PostMutation == nil {
		return ReconcileResult{}, exitcode.UnexpectedErrorf("Reconcile: PostMutation hook required")
	}

	plansDir := filepath.Join(opts.SpecRoot, "spec", "plans")
	flatPath := filepath.Join(plansDir, opts.Slug+".md")
	var result ReconcileResult
	err := lifecycle.WithArtifactMutationLock(flatPath, func() error {
		var reconcileErr error
		result, reconcileErr = reconcileUnderLock(opts, plansDir, flatPath)
		return reconcileErr
	})
	return result, err
}

// reconcileUnderLock performs the full plan and embedded-Task transaction
// while the caller owns flatPath's lifecycle fence. It must not call a public
// lifecycle writer that would acquire that fence again.
func reconcileUnderLock(opts ReconcileOptions, plansDir, flatPath string) (ReconcileResult, error) {
	resolved, err := resolvePlanFile(plansDir, opts.Slug)
	if err != nil {
		return ReconcileResult{}, err
	}
	if resolved != flatPath {
		return ReconcileResult{}, exitcode.InvalidStateErrorf(
			"plan %q uses the directory form (spec/plans/%s/README.md); `plan reconcile` only supports the flat single-file form",
			opts.Slug, opts.Slug)
	}

	p, err := Parse(flatPath)
	if err != nil {
		return ReconcileResult{}, exitcode.UnexpectedErrorf("parsing plan %s: %v", flatPath, err)
	}
	if p.StatusLine == 0 {
		return ReconcileResult{}, exitcode.UnexpectedErrorf("plan %s has no **Status:** line", flatPath)
	}
	from := lifecycle.Status(p.Status)

	if lifecycle.IsPlanDisposition(from) {
		return ReconcileResult{}, exitcode.InvalidStateErrorf(
			"plan %q is in disposition status %q; reconcile does not resurrect terminal dispositions — author a new plan, or use `specscore plan change-status` if this disposition was itself recorded in error",
			opts.Slug, p.Status)
	}

	if len(p.Tasks) == 0 {
		return ReconcileResult{}, exitcode.InvalidStateErrorf(
			"plan %q has no embedded tasks; there is nothing for reconcile to derive a status from", opts.Slug)
	}
	for _, t := range p.Tasks {
		if t.StatusLine == 0 {
			return ReconcileResult{}, exitcode.InvalidStateErrorf(
				"plan %q Task %d has no explicit **Status:** line; reconcile requires one on every task", opts.Slug, t.Number)
		}
	}

	// An explicit reopen corrects only a false completion. It must be exact:
	// named task numbers must exist and each target must still be complete.
	// This is intentionally before the all-complete guard below.
	if len(opts.ReopenTasks) > 0 {
		wanted := map[int]bool{}
		for _, n := range opts.ReopenTasks {
			if n <= 0 {
				return ReconcileResult{}, exitcode.InvalidArgsError("Reconcile: reopen task numbers must be positive")
			}
			wanted[n] = true
		}
		hypothetical := append([]Task(nil), p.Tasks...)
		changed := 0
		for i := range hypothetical {
			if !wanted[hypothetical[i].Number] {
				continue
			}
			if hypothetical[i].Status != StatusComplete {
				return ReconcileResult{}, exitcode.InvalidStateErrorf("plan %q Task %d is %s; only falsely complete tasks may be reopened", opts.Slug, hypothetical[i].Number, hypothetical[i].Status)
			}
			hypothetical[i].Status = StatusBlocked
			delete(wanted, hypothetical[i].Number)
			changed++
		}
		if len(wanted) > 0 {
			return ReconcileResult{}, exitcode.InvalidStateErrorf("plan %q has no Task %d to reopen", opts.Slug, firstReconcileTask(wanted))
		}
		band, _ := (&Plan{Tasks: hypothetical}).DeriveExecutionBand()
		to := lifecycle.Status(band)
		original, err := reconcileReadOriginalFn(flatPath)
		if err != nil {
			return ReconcileResult{}, exitcode.UnexpectedErrorf("reading plan: %v", err)
		}
		rollback := func() { _ = os.WriteFile(flatPath, original, 0o644) }
		lines := strings.Split(string(original), "\n")
		for _, t := range p.Tasks {
			if !wantedOriginal(opts.ReopenTasks, t.Number) {
				continue
			}
			lines[t.StatusLine-1] = "**Status:** " + string(StatusBlocked)
		}
		lines[p.StatusLine-1] = "**Status:** " + string(to)
		lines = insertReconciledMarkerIfAbsent(lines, p.StatusLine-1, reconcileTodayUTC())
		if err := os.WriteFile(flatPath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
			return ReconcileResult{}, exitcode.UnexpectedErrorf("writing plan: %v", err)
		}
		noteText := reconcileNoteTextWithTarget(from, to, changed, StatusBlocked, opts.Note, opts.Evidence, nil)
		if _, _, err := reconcileAppendNoteUnderLockFn(flatPath, noteText); err != nil {
			rollback()
			return ReconcileResult{}, exitcode.UnexpectedErrorf("writing resolution note: %v", err)
		}
		if err := opts.PostMutation(); err != nil {
			rollback()
			return ReconcileResult{}, err
		}
		return ReconcileResult{Slug: opts.Slug, From: from, To: to, TasksReconciled: changed, Target: StatusBlocked}, nil
	}

	// A task recorded as failed or aborted is a deliberate, meaningful claim
	// — reconcile must never silently overwrite it just because --tasks=complete
	// asked for "every" task. Only a task number the caller explicitly named
	// via ForceTasks may be overridden; every other failed/aborted task blocks
	// the whole reconciliation. `blocked` are refused; `overrides` are the
	// ones the caller acknowledged, itemized (never just aggregated) in the
	// Resolution paragraph below. Blocked is deliberately excluded from this
	// guard — it has outgoing arcs in the task state machine and is not a
	// terminal claim, so completing it is not the same category of override.
	forceSet := make(map[int]bool, len(opts.ForceTasks))
	for _, n := range opts.ForceTasks {
		forceSet[n] = true
	}
	var blocked []Task
	var overrides []TaskOverride
	for _, t := range p.Tasks {
		if t.Status != StatusFailed && t.Status != StatusAborted {
			continue
		}
		if forceSet[t.Number] {
			overrides = append(overrides, TaskOverride{Number: t.Number, From: t.Status})
			continue
		}
		blocked = append(blocked, t)
	}
	if len(blocked) > 0 {
		descs := make([]string, len(blocked))
		nums := make([]string, len(blocked))
		for i, t := range blocked {
			descs[i] = fmt.Sprintf("Task %d (%s)", t.Number, string(t.Status))
			nums[i] = strconv.Itoa(t.Number)
		}
		return ReconcileResult{}, exitcode.InvalidStateErrorf(
			"plan %q has task(s) recorded as failed/aborted that --tasks=complete would silently overwrite: %s; pass --force-tasks=%s to explicitly acknowledge overriding these specific task(s)",
			opts.Slug, strings.Join(descs, ", "), strings.Join(nums, ","))
	}

	hypothetical := make([]Task, len(p.Tasks))
	copy(hypothetical, p.Tasks)
	changedTasks := 0
	for i := range hypothetical {
		if hypothetical[i].Status != StatusComplete {
			changedTasks++
		}
		hypothetical[i].Status = StatusComplete
	}
	band, _ := (&Plan{Tasks: hypothetical}).DeriveExecutionBand()
	to := lifecycle.Status(band)

	if changedTasks == 0 && from == to {
		return ReconcileResult{}, exitcode.InvalidStateErrorf(
			"plan %q is already reconciled: status is %q and every task is complete; re-running is a no-op", opts.Slug, string(to))
	}

	original, err := reconcileReadOriginalFn(flatPath)
	if err != nil {
		return ReconcileResult{}, exitcode.UnexpectedErrorf("reading plan: %v", err)
	}
	rollback := func() { _ = os.WriteFile(flatPath, original, 0o644) }

	lines := strings.Split(string(original), "\n")
	for _, t := range p.Tasks {
		if t.Status == StatusComplete {
			continue
		}
		lines[t.StatusLine-1] = "**Status:** " + string(StatusComplete)
	}
	lines[p.StatusLine-1] = "**Status:** " + string(to)
	lines = insertReconciledMarkerIfAbsent(lines, p.StatusLine-1, reconcileTodayUTC())

	if err := os.WriteFile(flatPath, []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return ReconcileResult{}, exitcode.UnexpectedErrorf("writing plan: %v", err)
	}

	noteText := reconcileNoteTextWithTarget(from, to, changedTasks, StatusComplete, opts.Note, opts.Evidence, overrides)
	if _, _, err := reconcileAppendNoteUnderLockFn(flatPath, noteText); err != nil {
		rollback()
		return ReconcileResult{}, exitcode.UnexpectedErrorf("writing resolution note: %v", err)
	}

	if err := opts.PostMutation(); err != nil {
		rollback()
		return ReconcileResult{}, err
	}

	return ReconcileResult{Slug: opts.Slug, From: from, To: to, TasksReconciled: changedTasks, Overrides: overrides, Target: StatusComplete}, nil
}

func wantedOriginal(numbers []int, n int) bool {
	for _, v := range numbers {
		if v == n {
			return true
		}
	}
	return false
}
func firstReconcileTask(numbers map[int]bool) int {
	for n := range numbers {
		return n
	}
	return 0
}

// insertReconciledMarkerIfAbsent inserts a `**Reconciled:** <date>` header
// line immediately after lines[statusIdx] (the plan's 0-based **Status:**
// line), unless a `**Reconciled:**` line already exists anywhere in the file.
// The marker records only the date of the FIRST reconciliation; later
// reconciliations (e.g. after more tasks are added) still get their own
// dated paragraph in `## Resolution`, so nothing is lost — the header stays a
// single, stable, grep-able "this plan was reconciled at least once" signal.
func insertReconciledMarkerIfAbsent(lines []string, statusIdx int, date string) []string {
	for _, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), reconciledMarkerPrefix) {
			return lines
		}
	}
	marker := reconciledMarkerPrefix + " " + date
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[:statusIdx+1]...)
	out = append(out, marker)
	out = append(out, lines[statusIdx+1:]...)
	return out
}

// reconcileNoteText composes the `## Resolution` paragraph written by a
// reconciliation: a fixed preamble that names the jump explicitly (so a
// reader can never mistake this for a normal, one-arc-at-a-time
// change-status transition), the caller's own justification, any
// --force-tasks overrides (named individually — NEVER folded silently into
// the aggregate count), and — when supplied — the evidence references.
func reconcileNoteTextWithTarget(from, to lifecycle.Status, tasksReconciled int, target TaskStatus, note string, evidence []string, overrides []TaskOverride) string {
	var b strings.Builder
	fmt.Fprintf(&b,
		"**Reconciled %s → %s outside the tracked `change-status` flow** (%d task(s) marked %s; this did not walk the legal-transition matrix).\n\n",
		string(from), string(to), tasksReconciled, string(target))
	b.WriteString(strings.TrimSpace(note))
	if len(overrides) > 0 {
		parts := make([]string, len(overrides))
		for i, o := range overrides {
			parts[i] = fmt.Sprintf("Task %d (was %s)", o.Number, string(o.From))
		}
		b.WriteString("\n\n**Overridden from a terminal failure state via --force-tasks:** " + strings.Join(parts, ", ") + ".")
	}
	if len(evidence) > 0 {
		b.WriteString("\n\nEvidence: " + strings.Join(evidence, ", "))
	}
	return b.String()
}
