// Package plan — lifecycle transition orchestration for the Plan kind.
//
// This file hosts ChangeStatus, the kind-specific orchestrator invoked by
// `specscore plan change-status`. It composes pkg/lifecycle/ primitives
// (state-machine validation and one atomic artifact transaction) and adds the
// Plan-specific structured `**Superseded By:**` successor reference.
//
// `plan change-status` owns ONLY the human-authored arcs: the prep band
// (Draft/In Review/Approved) plus the dispositions (Rejected/Withdrawn/
// Superseded/Deprecated). The execution band (Executing/Blocked/Implemented/
// Failed) is LINT-DERIVED from the task-status rollup (rule P-007) and MUST
// NOT be settable here — the lifecycle KindPlan matrix encodes that by
// omitting every execution-band-entering arc. Plans are flat single files
// (or the optional directory form); change-status NEVER relocates a file.
//
// LINT INVOCATION lives in the cobra adapter (internal/cli/plan.go), NOT
// here, to avoid an import cycle: pkg/lint imports pkg/plan for the plan-*
// lint rules, so pkg/plan cannot depend back on pkg/lint. The adapter passes
// a PostMutationHook callback into ChangeStatus. It runs after the Plan lock is
// released; failure retains the committed Plan and reports recovery required.
//
// Cross-references:
//
//   - Verb spec: spec/features/cli/plan/change-status/README.md
//   - Meta contract: spec/features/cli/lifecycle-transitions/README.md
//   - Canonical lifecycle: spec/features/plan/README.md#req-status-transitions
package plan

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

// PostMutationHook is the callback the cobra adapter wires to
// `specscore spec lint --fix` (plus a verify pass). It MUST return nil on
// success; a non-nil return is wrapped as a committed/recovery-required error.
type PostMutationHook func() error

type artifactTransform func(path string, transform func([]byte) ([]byte, error)) error

func transformPlanArtifact(transform artifactTransform, path string, fn func([]byte) ([]byte, error)) error {
	if transform == nil {
		transform = lifecycle.TransformArtifact
	}
	return transform(path, fn)
}

// SnapshotValidator checks caller-specific preconditions against the exact
// bytes read under the Plan artifact lock. It must not mutate the artifact.
type SnapshotValidator func(path string, before []byte) error

// ChangeStatusOptions packages the inputs to ChangeStatus.
type ChangeStatusOptions struct {
	// SpecRoot is the project root that contains the `spec/` subtree
	// (NOT the `spec/` directory itself). The Plan is resolved at
	// SpecRoot/spec/plans/<slug>.md (flat) or .../spec/plans/<slug>/README.md
	// (the optional directory form).
	SpecRoot string

	// Slug is the Plan slug, e.g. "user-auth". Caller is expected to have
	// validated it via plan.ValidateSlug.
	Slug string

	// To is the canonical (title-case) target status. The cobra adapter parses
	// the raw --to value via lifecycle.ParseStatus and rejects execution-band /
	// unrecognized values BEFORE reaching this function.
	To lifecycle.Status

	// Note is the optional free-form markdown transition note. When non-empty
	// it is written as a `## Resolution` section, atomically with the status
	// rewrite. REQUIRED (enforced by the cobra adapter) for the Withdrawn and
	// Superseded dispositions.
	Note string

	// Successor is the slug of the plan that supersedes this one. REQUIRED
	// (enforced by the cobra adapter) for --to=Superseded, rejected otherwise.
	// It is written as a `**Superseded By:** <slug>` header line.
	Successor string

	// ValidateSnapshot runs under the Plan artifact lock before lifecycle
	// validation and transformation. CLI coordination-branch enforcement is
	// supplied here so it cannot authorize one snapshot and mutate another.
	ValidateSnapshot SnapshotValidator

	// PostMutation is the post-rewrite hook (typically a spec-lint pass).
	// Required; ChangeStatus returns exit 10 if nil.
	PostMutation PostMutationHook

	// transformArtifact is an instance-scoped fault dependency for package
	// tests. Production callers leave it nil and use lifecycle.TransformArtifact.
	transformArtifact artifactTransform
}

// ChangeStatusResult is the success payload returned on exit 0. The cobra
// adapter formats it as the `<slug>: <from> → <to>` success line.
type ChangeStatusResult struct {
	Slug string
	From lifecycle.Status
	To   lifecycle.Status
}

// ChangeStatus performs a Plan-kind lifecycle transition end-to-end.
//
// Flow:
//
//  1. Resolve <slug> to an existing Plan file (flat or directory form). A
//     missing file returns exit 3.
//  2. Under the Plan artifact lock, read the exact bytes, validate against the
//     KindPlan matrix, and compose Status, successor, and Resolution in memory.
//  3. Commit those bytes with one atomic durable replacement and release.
//  4. Invoke PostMutation. Failure retains the committed Plan and returns a
//     typed recovery-required error.
func ChangeStatus(opts ChangeStatusOptions) (ChangeStatusResult, error) {
	if opts.SpecRoot == "" {
		return ChangeStatusResult{}, exitcode.UnexpectedErrorf("ChangeStatus: SpecRoot required")
	}
	if opts.Slug == "" {
		return ChangeStatusResult{}, exitcode.InvalidArgsErrorf("ChangeStatus: slug required")
	}
	if opts.To == "" {
		return ChangeStatusResult{}, exitcode.InvalidArgsErrorf("ChangeStatus: target status required")
	}
	if opts.PostMutation == nil {
		return ChangeStatusResult{}, exitcode.UnexpectedErrorf("ChangeStatus: PostMutation hook required")
	}

	// (1) Slug resolution — flat file first, then the optional directory form.
	plansDir := filepath.Join(opts.SpecRoot, "spec", "plans")
	path, err := resolvePlanFile(plansDir, opts.Slug)
	if err != nil {
		return ChangeStatusResult{}, err
	}

	var from lifecycle.Status
	err = transformPlanArtifact(opts.transformArtifact, path, func(before []byte) ([]byte, error) {
		// Validation occurs only after the artifact fence is held and is derived
		// from the exact bytes this callback transforms.
		if opts.ValidateSnapshot != nil {
			if validateErr := opts.ValidateSnapshot(path, before); validateErr != nil {
				return nil, validateErr
			}
		}
		parsed, parseErr := ParseBytes(path, before)
		if parseErr != nil {
			return nil, parseErr
		}
		if artifactErr := requirePlanArtifact(parsed, "target"); artifactErr != nil {
			return nil, artifactErr
		}
		if parsed.StatusCount == 0 {
			return nil, lifecycle.ErrStatusLineNotFound
		}
		if parsed.StatusCount > 1 {
			return nil, exitcode.InvalidArgsErrorf("plan %q must have exactly one header **Status:** field; found %d", opts.Slug, parsed.StatusCount)
		}
		// RewriteBytes intentionally understands several artifact kinds and
		// therefore selects the first raw body Status line. Refuse if that line
		// is an example/comment rather than the structural Plan header selected
		// from this exact transaction snapshot.
		if firstRawPlanStatusLineBytes(before) != parsed.StatusLine {
			return nil, exitcode.InvalidStateErrorf(
				"plan %q has a non-structural **Status:** line before its canonical header status; remove or relocate the example before changing status", opts.Slug)
		}
		from = lifecycle.Status(parsed.Status)
		var validateErr error
		if validateErr = lifecycle.Transition(lifecycle.KindPlan, from, opts.To); validateErr != nil {
			return nil, validateErr
		}
		updated, _, transformErr := lifecycle.RewriteBytes(before, opts.To)
		if transformErr != nil {
			return nil, transformErr
		}
		if opts.Successor != "" {
			updated, _, _ = lifecycle.SetSupersededByBytes(updated, opts.Successor)
		}
		updated, _, _ = lifecycle.AppendResolutionNoteAfterLineBytes(updated, opts.Note, parsed.TitleLine)
		return updated, nil
	})
	if err != nil {
		var committed *lifecycle.CommittedMutationError
		if errors.As(err, &committed) {
			return ChangeStatusResult{Slug: opts.Slug, From: from, To: opts.To}, err
		}
		if errors.Is(err, lifecycle.ErrConcurrentMutation) {
			return ChangeStatusResult{}, exitcode.Wrap(exitcode.Conflict,
				fmt.Sprintf("plan %s is busy; retry from fresh state", opts.Slug), err)
		}
		var coded *exitcode.Error
		if errors.As(err, &coded) {
			return ChangeStatusResult{}, err
		}
		var ite *lifecycle.InvalidTransitionError
		if errors.As(err, &ite) {
			targets := statusNames(ite.LegalTargets)
			if len(targets) == 0 {
				return ChangeStatusResult{}, exitcode.InvalidStateErrorf(
					"invalid transition: plan %q is in status %q; no legal targets from this state",
					opts.Slug, string(ite.From))
			}
			return ChangeStatusResult{}, exitcode.InvalidStateErrorf(
				"invalid transition: plan %q is in status %q; legal targets: %s",
				opts.Slug, string(ite.From), strings.Join(targets, ", "))
		}
		if errors.Is(err, lifecycle.ErrStatusLineNotFound) {
			return ChangeStatusResult{}, exitcode.UnexpectedErrorCause(
				fmt.Sprintf("plan %s has no **Status:** line", path), err)
		}
		return ChangeStatusResult{}, exitcode.UnexpectedErrorCause(fmt.Sprintf("reading plan status: %v", err), err)
	}

	// The artifact commit is now durable. Post-mutation derived work runs after
	// release so it may take its own ordered artifact/index locks. A failure is
	// deliberately retained rather than attempting an unsafe split rollback.
	if err := opts.PostMutation(); err != nil {
		return ChangeStatusResult{Slug: opts.Slug, From: from, To: opts.To}, lifecycle.CommittedError(path, "post-mutation callback", err)
	}

	return ChangeStatusResult{Slug: opts.Slug, From: from, To: opts.To}, nil
}

// firstRawPlanStatusLineBytes mirrors the broad line selection used by
// lifecycle.RewriteBytes. Keeping this check byte-local means validation and
// transformation describe the same locked snapshot; no second filesystem read
// can race the transaction.
func firstRawPlanStatusLineBytes(data []byte) int {
	for i, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSuffix(line, "\r")
		if strings.HasPrefix(strings.TrimSpace(line), "**Status:**") {
			return i + 1
		}
	}
	return 0
}

// resolvePlanFile resolves <slug> to an existing Plan file under plansDir,
// supporting both the flat form (spec/plans/<slug>.md) and the optional
// directory form (spec/plans/<slug>/README.md). The flat form takes
// precedence. A slug that resolves to neither returns exit 3 (NotFound),
// naming the canonical flat path.
func resolvePlanFile(plansDir, slug string) (string, error) {
	flat := filepath.Join(plansDir, slug+".md")
	if _, err := os.Stat(flat); err == nil {
		return flat, nil
	} else if !os.IsNotExist(err) {
		return "", exitcode.UnexpectedErrorCause(fmt.Sprintf("stat %s: %v", flat, err), err)
	}

	dir := filepath.Join(plansDir, slug, "README.md")
	if _, err := os.Stat(dir); err == nil {
		return dir, nil
	} else if !os.IsNotExist(err) {
		return "", exitcode.UnexpectedErrorCause(fmt.Sprintf("stat %s: %v", dir, err), err)
	}

	return "", exitcode.NotFoundErrorf("plan not found at %s", flat)
}

// statusNames converts a slice of Status values to plain strings, for
// rendering in error messages.
func statusNames(s []lifecycle.Status) []string {
	out := make([]string, len(s))
	for i, st := range s {
		out[i] = string(st)
	}
	return out
}

// IsLegalChangeStatusTarget reports whether status is one of the human-
// settable --to values for `specscore plan change-status`. This is the union
// of every To column in the KindPlan matrix MINUS the execution-band values
// (which appear in the matrix only as disposition From-states, never as a
// human-settable target). The cobra adapter uses this for the exit-2 early
// flag check BEFORE the state-machine check.
func IsLegalChangeStatusTarget(s lifecycle.Status) bool {
	for _, t := range legalChangeStatusTargets() {
		if t == s {
			return true
		}
	}
	return false
}

// IsExecutionBandStatus reports whether s is one of the lint-derived
// execution-band statuses. The cobra adapter rejects these as --to values
// with a dedicated message that points the user at `spec lint --fix`.
func IsExecutionBandStatus(s lifecycle.Status) bool {
	switch s {
	case lifecycle.PlanExecuting, lifecycle.PlanBlocked,
		lifecycle.PlanImplemented, lifecycle.PlanFailed:
		return true
	}
	return false
}

// LegalChangeStatusTargetNames returns the canonical-titled names of the legal
// --to values, for stderr rendering and help text.
func LegalChangeStatusTargetNames() []string {
	targets := legalChangeStatusTargets()
	out := make([]string, len(targets))
	for i, t := range targets {
		out[i] = string(t)
	}
	return out
}

// legalChangeStatusTargets returns every To column in the KindPlan matrix that
// is human-settable — i.e., excluding the execution-band states. Computed from
// the lifecycle matrix so it stays in sync if the matrix grows, sorted
// alphabetically for deterministic output.
func legalChangeStatusTargets() []lifecycle.Status {
	seen := map[lifecycle.Status]struct{}{}
	for _, s := range lifecycle.LegalStatuses(lifecycle.KindPlan) {
		if IsExecutionBandStatus(s) {
			continue
		}
		if sources := lifecycle.LegalSources(lifecycle.KindPlan, s); len(sources) > 0 {
			seen[s] = struct{}{}
		}
	}
	out := make([]lifecycle.Status, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if string(out[j]) < string(out[i]) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}

// LegalTransitionMatrix returns a human-readable, ANSI-free rendering of the
// Plan legal-transition matrix, suitable for cobra `Long` help text. Built
// from lifecycle.LegalTargets so the help stays current as the matrix grows.
func LegalTransitionMatrix() string {
	statuses := lifecycle.LegalStatuses(lifecycle.KindPlan)

	type row struct {
		from    string
		targets string
	}
	var rows []row
	maxFrom := len("From")
	for _, s := range statuses {
		targets := lifecycle.LegalTargets(lifecycle.KindPlan, s)
		if len(targets) == 0 {
			continue
		}
		r := row{from: string(s), targets: strings.Join(statusNames(targets), ", ")}
		if len(r.from) > maxFrom {
			maxFrom = len(r.from)
		}
		rows = append(rows, r)
	}

	var sb strings.Builder
	sb.WriteString("Legal transitions (human-authored only; the execution band\n")
	sb.WriteString("Executing/Blocked/Implemented/Failed is derived by `spec lint --fix`):\n\n")
	sb.WriteString("  From")
	sb.WriteString(strings.Repeat(" ", maxFrom-len("From")))
	sb.WriteString("  To\n")
	sb.WriteString("  " + strings.Repeat("-", maxFrom) + "  --\n")
	for _, r := range rows {
		sb.WriteString("  " + r.from)
		sb.WriteString(strings.Repeat(" ", maxFrom-len(r.from)))
		sb.WriteString("  " + r.targets + "\n")
	}
	return sb.String()
}
