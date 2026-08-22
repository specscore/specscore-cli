// Package lesson — lifecycle transition orchestration for the Lesson kind.
//
// This file hosts ChangeStatus, the kind-specific orchestrator invoked by
// `specscore lesson change-status`. It composes pkg/lifecycle/ primitives
// (state-machine validation, status-line rewrite, rollback) and adds the
// Lesson-specific structured `**Superseded By:**` successor reference —
// mirroring pkg/plan/transitions.go exactly, minus the execution band (a
// Lesson has none: every rung of the ladder is human-authored).
//
// LINT INVOCATION lives in the cobra adapter (internal/cli/lesson.go), NOT
// here, to avoid an import cycle: pkg/lint imports pkg/lesson for the
// lesson-* lint rules, so pkg/lesson cannot depend back on pkg/lint. The
// adapter passes a PostMutationHook callback into ChangeStatus; this package
// only knows "run the post-mutation hook, and roll back if it fails".
package lesson

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

// appendNoteFn / setSupersededByFn are testable indirections for the
// lifecycle body-mutation helpers, so tests can exercise the
// failure-rollback branches.
var (
	appendNoteFn      = lifecycle.AppendResolutionNote
	setSupersededByFn = lifecycle.SetSupersededBy
)

// PostMutationHook is the callback the cobra adapter wires to its bounded
// index synchronization, durability fence, and read-only validation. Once the
// status rewrite is visible, a hook failure is retained as MutationUncertain;
// restoring a whole-file snapshot could erase a concurrent foreign edit.
type PostMutationHook func() error

// ChangeStatusOptions packages the inputs to ChangeStatus.
type ChangeStatusOptions struct {
	// SpecRoot is the project root that contains the `spec/` subtree (NOT the
	// `spec/` directory itself). The Lesson is resolved at
	// SpecRoot/spec/lessons/<slug>.md.
	SpecRoot string

	// Slug is the Lesson slug. Caller is expected to have validated it via
	// lesson.ValidateSlug.
	Slug string

	// To is the canonical (title-case) target status.
	To lifecycle.Status

	// Note is the optional free-form markdown transition note, written as a
	// `## Resolution` section. REQUIRED (enforced by the cobra adapter) for
	// the Withdrawn and Superseded dispositions.
	Note string

	// Successor is the slug of the lesson that supersedes this one. REQUIRED
	// (enforced by the cobra adapter) for --to=Superseded, rejected
	// otherwise. Written as a `**Superseded By:** <slug>` header line.
	Successor string

	// PostMutation is the post-rewrite hook (typically a spec-lint pass).
	// Required; ChangeStatus returns exit 10 if nil.
	PostMutation PostMutationHook
}

// ChangeStatusResult is the success payload returned on exit 0. The cobra
// adapter formats it as the `<slug>: <from> → <to>` success line.
type ChangeStatusResult struct {
	Slug string
	From lifecycle.Status
	To   lifecycle.Status
}

// ChangeStatus performs a Lesson-kind lifecycle transition end-to-end.
//
// Flow:
//
//  1. Resolve <slug> to an existing Lesson file. A missing file returns
//     exit 3.
//  2. lifecycle.Validate against the KindLesson matrix. Illegal transitions
//     return exit 4.
//  3. lifecycle.Rewrite the **Status:** line.
//  4. Optionally write the `**Superseded By:**` successor reference and the
//     `## Resolution` note.
//  5. Invoke the PostMutation hook. Any failure after step 3 is uncertain and
//     retained for durable recovery; this function never destructively
//     restores a snapshot after publication.
func ChangeStatus(opts ChangeStatusOptions) (ChangeStatusResult, error) {
	if opts.SpecRoot == "" {
		return ChangeStatusResult{}, exitcode.UnexpectedErrorf("ChangeStatus: SpecRoot required")
	}
	if err := ValidateSlug(opts.Slug); err != nil {
		return ChangeStatusResult{}, exitcode.InvalidArgsErrorf("ChangeStatus: invalid slug: %v", err)
	}
	if opts.To == "" {
		return ChangeStatusResult{}, exitcode.InvalidArgsErrorf("ChangeStatus: target status required")
	}
	if opts.PostMutation == nil {
		return ChangeStatusResult{}, exitcode.UnexpectedErrorf("ChangeStatus: PostMutation hook required")
	}
	var result ChangeStatusResult
	err := withLessonMutationLock(opts.SpecRoot, opts.Slug, defaultLessonMutationLockDeps(), func() error {
		var err error
		result, err = changeStatusUnlocked(opts)
		return err
	})
	return result, err
}

func changeStatusUnlocked(opts ChangeStatusOptions) (ChangeStatusResult, error) {
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

	path, err := ResolveLessonFile(filepath.Join(opts.SpecRoot, "spec", "lessons"), opts.Slug)
	if err != nil {
		return ChangeStatusResult{}, err
	}

	from, err := lifecycle.Validate(lifecycle.KindLesson, path, opts.To)
	if err != nil {
		var ite *lifecycle.InvalidTransitionError
		if errors.As(err, &ite) {
			targets := statusNames(ite.LegalTargets)
			if len(targets) == 0 {
				return ChangeStatusResult{}, exitcode.InvalidStateErrorf(
					"invalid transition: lesson %q is in status %q; no legal targets from this state",
					opts.Slug, string(ite.From))
			}
			return ChangeStatusResult{}, exitcode.InvalidStateErrorf(
				"invalid transition: lesson %q is in status %q; legal targets: %s",
				opts.Slug, string(ite.From), strings.Join(targets, ", "))
		}
		if errors.Is(err, lifecycle.ErrStatusLineNotFound) {
			return ChangeStatusResult{}, exitcode.UnexpectedErrorf(
				"lesson %s has no **Status:** line", path)
		}
		return ChangeStatusResult{}, exitcode.UnexpectedErrorf("reading lesson status: %v", err)
	}

	_, err = lifecycle.Rewrite(path, opts.To)
	if err != nil {
		return ChangeStatusResult{}, exitcode.UnexpectedErrorf("rewriting status line: %v", err)
	}

	_, _, err = setSupersededByFn(path, opts.Successor)
	if err != nil {
		return ChangeStatusResult{}, mutationFailure(MutationUncertain, exitcode.UnexpectedErrorf("writing successor reference: %v", err))
	}

	_, _, err = appendNoteFn(path, opts.Note)
	if err != nil {
		return ChangeStatusResult{}, mutationFailure(MutationUncertain, exitcode.UnexpectedErrorf("writing transition note: %v", err))
	}

	if err := opts.PostMutation(); err != nil {
		return ChangeStatusResult{}, mutationFailure(MutationUncertain, err)
	}

	// A hook that returns nil has not proven the requested status survived its
	// own `--fix` rewrites. Verify the artifact actually carries it before the
	// caller prints `<slug>: <from> → <to>`.
	if err := lifecycle.VerifyPersistedStatus(path, opts.To); err != nil {
		return ChangeStatusResult{}, mutationFailure(MutationUncertain,
			exitcode.UnexpectedErrorf("%v", err))
	}

	return ChangeStatusResult{Slug: opts.Slug, From: from, To: opts.To}, nil
}

// ResolveLessonFile resolves a canonical directory Lesson first, then a legacy
// flat file during the compatibility window. Both forms for the same slug are
// a conflict: picking one would make lifecycle changes nondeterministic.
func ResolveLessonFile(lessonsDir, slug string) (string, error) {
	if err := ValidateSlug(slug); err != nil {
		return "", exitcode.InvalidArgsErrorf("invalid lesson slug %q: %v", slug, err)
	}
	canonical := filepath.Join(lessonsDir, slug, "README.md")
	flat := filepath.Join(lessonsDir, slug+".md")
	_, canonicalErr := os.Stat(canonical)
	_, flatErr := os.Stat(flat)
	canonicalExists := canonicalErr == nil
	flatExists := flatErr == nil
	if canonicalExists && flatExists {
		return "", exitcode.ConflictErrorf("lesson %q has both canonical directory and legacy flat forms", slug)
	}
	if canonicalExists {
		return canonical, nil
	}
	if canonicalErr != nil && !os.IsNotExist(canonicalErr) {
		return "", exitcode.UnexpectedErrorf("stat %s: %v", canonical, canonicalErr)
	}
	if flatExists {
		return flat, nil
	}
	if flatErr != nil && !os.IsNotExist(flatErr) {
		return "", exitcode.UnexpectedErrorf("stat %s: %v", flat, flatErr)
	}
	return "", exitcode.NotFoundErrorf("lesson not found: %s", slug)
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

// LegalChangeStatusTargetNames returns the canonical-titled names of the
// legal --to values (every To column in the KindLesson matrix). Filtering
// lifecycle.LegalStatuses — itself already alphabetically sorted — preserves
// that order, so no separate sort step is needed.
func LegalChangeStatusTargetNames() []string {
	var out []string
	for _, s := range lifecycle.LegalStatuses(lifecycle.KindLesson) {
		if sources := lifecycle.LegalSources(lifecycle.KindLesson, s); len(sources) > 0 {
			out = append(out, string(s))
		}
	}
	return out
}

// LegalTransitionMatrix returns a human-readable, ANSI-free rendering of the
// Lesson legal-transition matrix, suitable for cobra `Long` help text.
func LegalTransitionMatrix() string {
	statuses := lifecycle.LegalStatuses(lifecycle.KindLesson)

	type row struct {
		from    string
		targets string
	}
	var rows []row
	maxFrom := len("From")
	for _, s := range statuses {
		targets := lifecycle.LegalTargets(lifecycle.KindLesson, s)
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
	sb.WriteString("Legal transitions (the enforcement ladder climbs Recorded -> Stated ->\n")
	sb.WriteString("Enforced; Withdrawn and Superseded are reachable from every rung):\n\n")
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
