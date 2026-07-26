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

// PostMutationHook is the callback the cobra adapter wires to
// `specscore spec lint --fix`. It MUST return nil on success; a non-nil
// return triggers full rollback of every on-disk mutation and the error is
// returned by ChangeStatus.
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
//  3. lifecycle.Rewrite the **Status:** line; capture original for rollback.
//  4. Optionally write the `**Superseded By:**` successor reference and the
//     `## Resolution` note, each rolled back together with the status line.
//  5. Invoke the PostMutation hook. Failure → full rollback + exit 10.
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

	origLine, err := lifecycle.Rewrite(path, opts.To)
	if err != nil {
		return ChangeStatusResult{}, exitcode.UnexpectedErrorf("rewriting status line: %v", err)
	}
	fullRollback := func() {
		_ = lifecycle.Rollback(path, origLine)
	}

	origSucc, succWritten, err := setSupersededByFn(path, opts.Successor)
	if err != nil {
		fullRollback()
		return ChangeStatusResult{}, exitcode.UnexpectedErrorf("writing successor reference: %v", err)
	}
	if succWritten {
		prev := fullRollback
		fullRollback = func() {
			_ = lifecycle.RestoreBody(path, origSucc)
			prev()
		}
	}

	origBody, noteWritten, err := appendNoteFn(path, opts.Note)
	if err != nil {
		fullRollback()
		return ChangeStatusResult{}, exitcode.UnexpectedErrorf("writing transition note: %v", err)
	}
	if noteWritten {
		prev := fullRollback
		fullRollback = func() {
			_ = lifecycle.RestoreBody(path, origBody)
			prev()
		}
	}

	if err := opts.PostMutation(); err != nil {
		fullRollback()
		return ChangeStatusResult{}, err
	}

	return ChangeStatusResult{Slug: opts.Slug, From: from, To: opts.To}, nil
}

// ResolveLessonFile resolves <slug> to an existing Lesson file under
// lessonsDir (spec/lessons/<slug>.md). A slug that does not resolve returns
// exit 3 (NotFound), naming the canonical path.
func ResolveLessonFile(lessonsDir, slug string) (string, error) {
	flat := filepath.Join(lessonsDir, slug+".md")
	if _, err := os.Stat(flat); err == nil {
		return flat, nil
	} else if !os.IsNotExist(err) {
		return "", exitcode.UnexpectedErrorf("stat %s: %v", flat, err)
	}
	return "", exitcode.NotFoundErrorf("lesson not found at %s", flat)
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
