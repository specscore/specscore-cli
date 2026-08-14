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
	"errors"
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

	// ValidateSnapshot runs under the Plan artifact lock before reconciliation
	// validation/transformation. It must not mutate the artifact.
	ValidateSnapshot SnapshotValidator

	// PostMutation is the post-rewrite hook (typically a spec-lint pass that
	// syncs the plans index). The lifecycle write itself keeps the frontmatter
	// `status:` mirror in lockstep with the canonical Plan body status.
	// Required; Reconcile returns exit 10 if nil.
	PostMutation PostMutationHook

	// transformArtifact is an instance-scoped fault dependency for package
	// tests. Production callers leave it nil and use lifecycle.TransformArtifact.
	transformArtifact artifactTransform
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

// PreviewReconcile runs the complete read-only reconciliation validation and
// composition against the current Plan bytes. It is used by whole-tree
// transactions to reject an invalid command before creating recovery state;
// it never acquires a write lock, writes the Plan, or invokes PostMutation.
func PreviewReconcile(opts ReconcileOptions) (ReconcileResult, error) {
	if opts.SpecRoot == "" {
		return ReconcileResult{}, exitcode.UnexpectedErrorf("PreviewReconcile: SpecRoot required")
	}
	if opts.Slug == "" {
		return ReconcileResult{}, exitcode.InvalidArgsErrorf("PreviewReconcile: slug required")
	}
	if strings.TrimSpace(opts.Note) == "" {
		return ReconcileResult{}, exitcode.InvalidArgsError(
			"PreviewReconcile: --note required — describe why the plan is being reconciled")
	}
	plansDir := filepath.Join(opts.SpecRoot, "spec", "plans")
	flatPath := filepath.Join(plansDir, opts.Slug+".md")
	resolved, err := resolvePlanFile(plansDir, opts.Slug)
	if err != nil {
		return ReconcileResult{}, err
	}
	if resolved != flatPath {
		return ReconcileResult{}, exitcode.InvalidStateErrorf(
			"plan %q uses the directory form (spec/plans/%s/README.md); `plan reconcile` only supports the flat single-file form",
			opts.Slug, opts.Slug)
	}
	before, err := os.ReadFile(flatPath)
	if err != nil {
		return ReconcileResult{}, exitcode.UnexpectedErrorf("reading plan %s for reconciliation preview: %v", opts.Slug, err)
	}
	if opts.ValidateSnapshot != nil {
		if err := opts.ValidateSnapshot(flatPath, before); err != nil {
			return ReconcileResult{}, err
		}
	}
	result, _, err := reconcileBytes(opts, flatPath, before)
	return result, err
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
//  4. Under one fail-fast per-artifact lock, read the exact bytes, resolve and
//     validate every target against that snapshot, then compose every task
//     status, the Plan status, the first-reconciliation marker, and the
//     `## Resolution` audit into one atomic durable replacement.
//  5. Release the lock, then invoke PostMutation (spec lint --fix + verify).
//     Callback failure reports a typed committed/recovery-required error; the
//     already-visible canonical bytes are deliberately never rolled back.
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
	resolved, err := resolvePlanFile(plansDir, opts.Slug)
	if err != nil {
		return ReconcileResult{}, err
	}
	if resolved != flatPath {
		return ReconcileResult{}, exitcode.InvalidStateErrorf(
			"plan %q uses the directory form (spec/plans/%s/README.md); `plan reconcile` only supports the flat single-file form",
			opts.Slug, opts.Slug)
	}
	var result ReconcileResult
	err = transformPlanArtifact(opts.transformArtifact, flatPath, func(before []byte) ([]byte, error) {
		if opts.ValidateSnapshot != nil {
			if validateErr := opts.ValidateSnapshot(flatPath, before); validateErr != nil {
				return nil, validateErr
			}
		}
		var after []byte
		var reconcileErr error
		result, after, reconcileErr = reconcileBytes(opts, flatPath, before)
		return after, reconcileErr
	})
	if err != nil {
		if errors.Is(err, lifecycle.ErrConcurrentMutation) {
			return result, exitcode.Wrap(exitcode.Conflict,
				fmt.Sprintf("plan %s is busy; retry reconciliation from fresh state", opts.Slug), err)
		}
		var coded *exitcode.Error
		var committed *lifecycle.CommittedMutationError
		if errors.As(err, &coded) || errors.As(err, &committed) {
			return result, err
		}
		return result, exitcode.UnexpectedErrorCause(fmt.Sprintf("reconciling plan artifact transaction %s: %v", flatPath, err), err)
	}
	// Like ChangeStatus, the artifact transaction is durable before derived
	// lint/index work begins. The ordinary hook may acquire Plan locks itself,
	// so it must never run while flatPath's non-reentrant fence is held.
	if err := opts.PostMutation(); err != nil {
		return result, lifecycle.CommittedError(flatPath, "post-mutation callback", err)
	}
	return result, nil
}

// reconcileBytes validates and transforms one exact Plan snapshot. It has no
// filesystem side effects; status, task rows, marker, and Resolution audit are
// composed into the one byte slice committed by TransformArtifact.
func reconcileBytes(opts ReconcileOptions, flatPath string, original []byte) (ReconcileResult, []byte, error) {
	p, err := ParseBytes(flatPath, original)
	if err != nil {
		return ReconcileResult{}, nil, exitcode.UnexpectedErrorf("parsing plan %s: %v", flatPath, err)
	}
	if err := requirePlanArtifact(p, "target"); err != nil {
		return ReconcileResult{}, nil, err
	}
	if p.StatusCount == 0 {
		return ReconcileResult{}, nil, exitcode.UnexpectedErrorf("plan %s has no **Status:** line", flatPath)
	}
	if p.StatusCount > 1 {
		return ReconcileResult{}, nil, exitcode.InvalidArgsErrorf("plan %q must have exactly one header **Status:** field; found %d", opts.Slug, p.StatusCount)
	}
	from := lifecycle.Status(p.Status)

	if lifecycle.IsPlanDisposition(from) {
		return ReconcileResult{}, nil, exitcode.InvalidStateErrorf(
			"plan %q is in disposition status %q; reconcile does not resurrect terminal dispositions — author a new plan, or use `specscore plan change-status` if this disposition was itself recorded in error",
			opts.Slug, p.Status)
	}

	if len(p.Tasks) == 0 {
		return ReconcileResult{}, nil, exitcode.InvalidStateErrorf(
			"plan %q has no embedded tasks; there is nothing for reconcile to derive a status from", opts.Slug)
	}
	taskNumbers := make(map[int]int, len(p.Tasks))
	for _, t := range p.Tasks {
		taskNumbers[t.Number]++
		if t.StatusCount == 0 {
			return ReconcileResult{}, nil, exitcode.InvalidStateErrorf(
				"plan %q Task %d has no explicit **Status:** line; reconcile requires one on every task", opts.Slug, t.Number)
		}
		if t.StatusCount > 1 {
			return ReconcileResult{}, nil, exitcode.InvalidArgsErrorf(
				"plan %q Task %d must have exactly one **Status:** field; found %d", opts.Slug, t.Number, t.StatusCount)
		}
	}
	for number, count := range taskNumbers {
		if count > 1 {
			return ReconcileResult{}, nil, exitcode.InvalidArgsErrorf(
				"plan %q has duplicate Task number %d; reconciliation targets must be unique", opts.Slug, number)
		}
	}

	// An explicit reopen corrects only a false completion. It must be exact:
	// named task numbers must exist and each target must still be complete.
	// This is intentionally before the all-complete guard below.
	if len(opts.ReopenTasks) > 0 {
		wanted := map[int]bool{}
		for _, n := range opts.ReopenTasks {
			if n <= 0 {
				return ReconcileResult{}, nil, exitcode.InvalidArgsError("Reconcile: reopen task numbers must be positive")
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
				return ReconcileResult{}, nil, exitcode.InvalidStateErrorf("plan %q Task %d is %s; only falsely complete tasks may be reopened", opts.Slug, hypothetical[i].Number, hypothetical[i].Status)
			}
			hypothetical[i].Status = StatusBlocked
			delete(wanted, hypothetical[i].Number)
			changed++
		}
		if len(wanted) > 0 {
			return ReconcileResult{}, nil, exitcode.InvalidStateErrorf("plan %q has no Task %d to reopen", opts.Slug, firstReconcileTask(wanted))
		}
		band, _ := (&Plan{Tasks: hypothetical}).DeriveExecutionBand()
		to := lifecycle.Status(band)
		lines := strings.Split(string(original), "\n")
		for _, t := range p.Tasks {
			if !wantedOriginal(opts.ReopenTasks, t.Number) {
				continue
			}
			lines[t.StatusLine-1] = "**Status:** " + string(StatusBlocked)
		}
		lines[p.StatusLine-1] = "**Status:** " + string(to)
		var mirrorErr error
		lines, mirrorErr = setReconcileFrontmatterStatus(lines, string(to))
		if mirrorErr != nil {
			return ReconcileResult{}, nil, exitcode.UnexpectedErrorf("preparing Plan frontmatter mirror: %v", mirrorErr)
		}
		lines = insertReconciledMarkerIfAbsent(lines, p.TitleLine-1, p.StatusLine-1, reconcileTodayUTC())
		noteText := reconcileNoteTextWithTarget(from, to, changed, StatusBlocked, opts.Note, opts.Evidence, nil)
		updated, _, _ := lifecycle.AppendResolutionNoteAfterLineBytes([]byte(strings.Join(lines, "\n")), noteText, p.TitleLine)
		return ReconcileResult{Slug: opts.Slug, From: from, To: to, TasksReconciled: changed, Target: StatusBlocked}, updated, nil
	}

	// Reconcile writes the Plan directly to the Implemented execution band.
	// It is an out-of-band history correction, not a prerequisite bypass: the
	// same readiness evaluator used by dispatch entrypoints must pass before
	// any transformed bytes are produced or written, preserving atomic refusal.
	readiness, err := p.PrerequisiteReadiness(filepath.Join(opts.SpecRoot, "spec", "plans"))
	if err != nil {
		return ReconcileResult{}, nil, preserveReadinessError(opts.Slug, err)
	}
	if !readiness.Ready {
		return ReconcileResult{}, nil, exitcode.InvalidStateErrorf(
			"plan %q is not ready to reconcile to Implemented; unmet prerequisite plan(s): %s; each must derive Implemented from its task rollup",
			opts.Slug, readiness.UnmetMessage())
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
		return ReconcileResult{}, nil, exitcode.InvalidStateErrorf(
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
		return ReconcileResult{}, nil, exitcode.InvalidStateErrorf(
			"plan %q is already reconciled: status is %q and every task is complete; re-running is a no-op", opts.Slug, string(to))
	}

	updated, err := reconcilePlanContent(original, p, to, reconcileTodayUTC())
	if err != nil {
		return ReconcileResult{}, nil, exitcode.UnexpectedErrorf("preparing plan reconciliation: %v", err)
	}
	noteText := reconcileNoteTextWithTarget(from, to, changedTasks, StatusComplete, opts.Note, opts.Evidence, overrides)
	updated, _, _ = lifecycle.AppendResolutionNoteAfterLineBytes(updated, noteText, p.TitleLine)

	return ReconcileResult{Slug: opts.Slug, From: from, To: to, TasksReconciled: changedTasks, Overrides: overrides, Target: StatusComplete}, updated, nil
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

// reconcilePlanContent derives every first-write change from one original byte
// snapshot: each incomplete task becomes complete, the canonical Plan Status
// becomes Implemented, the leading-frontmatter status mirror follows it, and
// the reconciliation marker is inserted once. No source line is selected by a
// broad text search: Parse supplied every body line from the same structural
// Markdown mask used for lifecycle admission.
func reconcilePlanContent(original []byte, p *Plan, to lifecycle.Status, date string) ([]byte, error) {
	lines := strings.Split(string(original), "\n")
	for _, t := range p.Tasks {
		if t.Status == StatusComplete {
			continue
		}
		if t.StatusLine <= 0 || t.StatusLine > len(lines) {
			return nil, fmt.Errorf("Task %d status line %d is outside 1..%d", t.Number, t.StatusLine, len(lines))
		}
		updated, err := replaceReconciledStatusLine(lines[t.StatusLine-1], string(StatusComplete))
		if err != nil {
			return nil, fmt.Errorf("Task %d status line: %w", t.Number, err)
		}
		lines[t.StatusLine-1] = updated
	}
	if p.StatusLine <= 0 || p.StatusLine > len(lines) {
		return nil, fmt.Errorf("Plan status line %d is outside 1..%d", p.StatusLine, len(lines))
	}
	updatedStatus, err := replaceReconciledStatusLine(lines[p.StatusLine-1], string(to))
	if err != nil {
		return nil, fmt.Errorf("Plan status line: %w", err)
	}
	lines[p.StatusLine-1] = updatedStatus
	lines = insertReconciledMarkerIfAbsent(lines, p.TitleLine-1, p.StatusLine-1, date)

	// A reconciled flat Plan must leave both status surfaces synchronized in
	// this very write. A legacy/malformed frontmatter block is rejected before
	// persistence rather than allowing body-only status drift for a later lint
	// pass to repair.
	lines, err = setReconcileFrontmatterStatus(lines, string(to))
	if err != nil {
		return nil, err
	}
	return []byte(strings.Join(lines, "\n")), nil
}

// replaceReconciledStatusLine preserves the header spelling, post-colon
// whitespace, trailing horizontal whitespace, and CR byte of a parsed Plan
// Status field. The parser only records structurally canonical, column-zero
// Plan/task fields; this defensive check makes any read/write disagreement a
// fail-closed error before the compound write happens.
func replaceReconciledStatusLine(line, status string) (string, error) {
	const prefix = "**Status:**"
	cr := ""
	if strings.HasSuffix(line, "\r") {
		cr = "\r"
		line = strings.TrimSuffix(line, cr)
	}
	if !strings.HasPrefix(line, prefix) {
		return "", fmt.Errorf("expected %q field", prefix)
	}
	rest := line[len(prefix):]
	valueStart := 0
	for valueStart < len(rest) && (rest[valueStart] == ' ' || rest[valueStart] == '\t') {
		valueStart++
	}
	if valueStart == len(rest) {
		return "", fmt.Errorf("missing status value")
	}
	valueEnd := len(rest)
	for valueEnd > valueStart && (rest[valueEnd-1] == ' ' || rest[valueEnd-1] == '\t') {
		valueEnd--
	}
	return prefix + rest[:valueStart] + status + rest[valueEnd:] + cr, nil
}

// setReconcileFrontmatterStatus updates the unique top-level `status:` field
// in a complete leading frontmatter block, or inserts it immediately before
// the closing fence when absent. A BOM-prefixed opening fence is recognized
// but never normalized, and a missing/malformed block fails closed before any
// mutation is persisted.
func setReconcileFrontmatterStatus(lines []string, status string) ([]string, error) {
	if len(lines) == 0 {
		return nil, fmt.Errorf("Plan has no leading frontmatter block")
	}
	first := strings.TrimSuffix(lines[0], "\r")
	if !lifecycle.IsLeadingFrontmatterFence(first) {
		return nil, fmt.Errorf("Plan has no complete leading frontmatter block")
	}
	closing := -1
	statusLine := -1
	for i := 1; i < len(lines); i++ {
		body := strings.TrimSuffix(lines[i], "\r")
		if lifecycle.IsFrontmatterFence(body) {
			closing = i
			break
		}
		if isTopLevelFrontmatterKey(body, "status") {
			if statusLine >= 0 {
				return nil, fmt.Errorf("Plan frontmatter has duplicate top-level status fields")
			}
			statusLine = i
		}
	}
	if closing < 0 {
		return nil, fmt.Errorf("Plan frontmatter opening fence is not closed")
	}
	if statusLine >= 0 {
		lines[statusLine] = replaceReconcileFrontmatterStatus(lines[statusLine], status)
		return lines, nil
	}

	terminator := reconcileInsertionTerminator(lines, closing)
	inserted := append([]string{}, lines[:closing]...)
	inserted = append(inserted, "status: "+status+terminator)
	inserted = append(inserted, lines[closing:]...)
	return inserted, nil
}

func isTopLevelFrontmatterKey(line, key string) bool {
	if line == "" || line[0] == ' ' || line[0] == '\t' || line[0] == '#' {
		return false
	}
	colon := strings.IndexByte(line, ':')
	return colon > 0 && strings.TrimSpace(line[:colon]) == key
}

func replaceReconcileFrontmatterStatus(line, status string) string {
	cr := ""
	if strings.HasSuffix(line, "\r") {
		cr = "\r"
		line = strings.TrimSuffix(line, cr)
	}
	colon := strings.IndexByte(line, ':')
	rest := line[colon+1:]
	valueStart := 0
	for valueStart < len(rest) && (rest[valueStart] == ' ' || rest[valueStart] == '\t') {
		valueStart++
	}
	valueEnd := len(rest)
	for valueEnd > valueStart && (rest[valueEnd-1] == ' ' || rest[valueEnd-1] == '\t') {
		valueEnd--
	}
	return line[:colon+1] + rest[:valueStart] + status + rest[valueEnd:] + cr
}

func reconcileInsertionTerminator(lines []string, around int) string {
	for _, index := range []int{around, around - 1, 0} {
		if index >= 0 && index < len(lines) && strings.HasSuffix(lines[index], "\r") {
			return "\r"
		}
	}
	return ""
}

// preserveReadinessError keeps contract-level refusals such as a non-Plan
// prerequisite at InvalidState. Only untyped filesystem/parser failures are
// unexpected runtime failures. Call this before a reconcile mutation so an
// execution gate cannot accidentally erase its own machine-readable reason.
func preserveReadinessError(slug string, err error) error {
	var coded interface{ ExitCode() int }
	if errors.As(err, &coded) {
		return err
	}
	return exitcode.UnexpectedErrorf("checking prerequisite readiness for plan %q: %v", slug, err)
}

// insertReconciledMarkerIfAbsent inserts a `**Reconciled:** <date>` header
// line immediately after lines[statusIdx] (the plan's 0-based **Status:**
// line), unless the canonical post-title Plan header already contains a
// `**Reconciled:**` line. A pre-title sample or a body example must not
// suppress the actual audit marker.
// The marker records only the date of the FIRST reconciliation; later
// reconciliations (e.g. after more tasks are added) still get their own
// dated paragraph in `## Resolution`, so nothing is lost — the header stays a
// single, stable, grep-able "this plan was reconciled at least once" signal.
func insertReconciledMarkerIfAbsent(lines []string, titleIdx, statusIdx int, date string) []string {
	structure := scanStructuralMarkdown(lines)
	for i := titleIdx + 1; i < len(lines); i++ {
		if !structure.isStructural(i) {
			continue
		}
		body := strings.TrimSuffix(lines[i], "\r")
		if _, ok := canonicalUnindentedATXH2(body); ok {
			break
		}
		name, _, ok := matchBoldField(body)
		if ok && name == "Reconciled" {
			return lines
		}
	}
	marker := reconciledMarkerPrefix + " " + date + reconcileInsertionTerminator(lines, statusIdx)
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
