// Package idea — lifecycle transition orchestration for the Idea kind.
//
// This file hosts ChangeStatus, the kind-specific orchestrator invoked by
// `specscore idea change-status`. It composes pkg/lifecycle/ primitives
// (state-machine validation, status-line rewrite, rollback) with the
// Idea-specific archive file-relocation side effect.
//
// LINT INVOCATION lives in the cobra adapter (internal/cli/idea.go), NOT
// here, to avoid an import cycle: pkg/lint already imports pkg/idea for
// the idea-* lint rules, so pkg/idea cannot depend back on pkg/lint.
// The adapter passes a Linter callback into ChangeStatus; this package
// only knows "run the post-mutation hook, and roll back if it fails".
//
// Cross-references:
//
//   - Verb spec: spec/features/cli/idea/change-status/README.md
//   - Meta contract: spec/features/cli/lifecycle-transitions/README.md
//
// All behavioral REQs cited in those documents are enforced here; the cobra
// adapter is a thin shim over this function.
package idea

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

// osStatFn is a testable indirection for os.Stat used in the archive
// collision check. Tests can replace it to inject non-ENOENT errors.
var osStatFn = os.Stat

// archivedIndexStub is the minimal lint-clean content the verb writes to
// spec/ideas/archived/README.md when that file does not already exist on
// the first archive transition. lint --fix will subsequently rewrite the
// body to list the just-archived idea, but the stub itself satisfies
// readme-exists, oq-section, and the (warn-severity) adherence-footer
// rules on its own.
const archivedIndexStub = `# Archived Ideas

_No archived ideas yet._

## Open Questions

None at this time.
`

// EnsureArchivedIndexStub guarantees that spec/ideas/archived/ exists and
// contains a lint-clean README.md index. It creates the directory if absent
// and writes archivedIndexStub to archived/README.md ONLY when that file
// does not already exist. It returns created=true iff it wrote the stub
// (so callers can stage exactly the file they materialized).
//
// `specscore init` does not create spec/ideas/archived/ — the directory
// comes into existence on the first archive/promote, and a directory
// without README.md fires the readme-exists rule (error severity). Writing
// the stub keeps the archive/promote operation's output lint-clean.
//
// specRoot is the project root that contains the spec/ subtree (NOT the
// spec/ directory itself).
func EnsureArchivedIndexStub(specRoot string) (created bool, err error) {
	archivedDir := filepath.Join(ResolveIdeasDir(filepath.Join(specRoot, "spec")), "archived")
	if err := os.MkdirAll(archivedDir, 0o755); err != nil {
		return false, exitcode.UnexpectedErrorf(
			"creating archived directory %s: %v", archivedDir, err)
	}
	archivedReadme := filepath.Join(archivedDir, "README.md")
	if _, statErr := os.Stat(archivedReadme); os.IsNotExist(statErr) {
		if werr := os.WriteFile(archivedReadme, []byte(archivedIndexStub), 0o644); werr != nil {
			return false, exitcode.UnexpectedErrorf(
				"creating archived index stub %s: %v", archivedReadme, werr)
		}
		return true, nil
	} else if statErr != nil {
		return false, exitcode.UnexpectedErrorf(
			"stat archived index %s: %v", archivedReadme, statErr)
	}
	return false, nil
}

// PostMutationHook is the callback ChangeStatus invokes after a successful
// status rewrite (and, for archive transitions, file move). It is the
// integration point for `specscore spec lint --fix` plus the verify pass.
//
// The hook MUST return nil on success. A non-nil return triggers full
// rollback (status line restored AND, for archive transitions, file moved
// back) and the error is wrapped and returned by ChangeStatus.
//
// The cobra adapter is responsible for wiring this to pkg/lint; tests can
// supply a fake to exercise the rollback paths without invoking lint.
type PostMutationHook func() error

// ChangeStatusOptions packages the inputs to ChangeStatus.
type ChangeStatusOptions struct {
	// SpecRoot is the project root that contains the `spec/` subtree
	// (NOT the `spec/` directory itself). The Idea file is resolved at
	// SpecRoot/spec/ideas/<Slug>.md.
	SpecRoot string

	// Slug is the Idea slug, e.g. "payment-fraud". Caller is expected
	// to have validated it via idea.ValidateSlug.
	Slug string

	// To is the canonical (title-case) target status, e.g. "Approved"
	// or "Archived". The cobra adapter parses the raw --to value via
	// lifecycle.ParseStatus before reaching this function.
	To lifecycle.Status

	// PostMutation is the post-rewrite hook (typically a spec-lint
	// pass). Required; ChangeStatus returns exit 10 if nil.
	PostMutation PostMutationHook

	// Note is an optional free-form markdown transition note. When non-
	// empty after trimming, it is written into the artifact body as a
	// `## Resolution` section, atomically with the status rewrite, per
	// lifecycle-transitions#REQ:optional-transition-note. An empty or
	// whitespace-only value is a no-op (today's behavior). The note write
	// participates in the same rollback guarantee as the status rewrite.
	Note string
}

// ChangeStatusResult is the success payload returned by ChangeStatus on
// exit 0. The cobra adapter formats it as the success-output line.
type ChangeStatusResult struct {
	Slug string
	From lifecycle.Status
	To   lifecycle.Status
}

// ChangeStatus performs an Idea-kind lifecycle transition end-to-end.
//
// Flow (matches the verb spec step-list):
//
//  1. Resolve <slug> to an active file at spec/ideas/<slug>.md. A missing
//     active file (even if an archived copy exists) returns exit 3.
//  2. lifecycle.Validate against the Idea matrix. Illegal transitions
//     return exit 4.
//  3. lifecycle.Rewrite the **Status:** line; capture original for
//     rollback.
//  4. If To == Archived: check collision, mkdir-p + os.Rename. Collision
//     returns exit 1; mkdir/rename failure rolls back and returns exit 10.
//  5. Invoke the PostMutation hook. Failure → full rollback + exit 10.
//
// ChangeStatus performs all rollback internally — by the time it returns
// an error, the on-disk state is byte-identical to its pre-invocation
// shape per lifecycle-transitions#REQ:rollback-on-lint-failure and
// cli/idea/change-status#REQ:rollback-includes-relocation.
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

	activePath := filepath.Join(opts.SpecRoot, "spec", "ideas", opts.Slug+".md")
	archivedPath := filepath.Join(opts.SpecRoot, "spec", "ideas", "archived", opts.Slug+".md")

	// (1) Slug resolution — active path only. The archived path is NEVER
	// a fallback per REQ:slug-resolves-to-active-idea.
	if _, err := os.Stat(activePath); err != nil {
		if os.IsNotExist(err) {
			return ChangeStatusResult{}, exitcode.NotFoundErrorf("idea not found at %s", activePath)
		}
		return ChangeStatusResult{}, exitcode.UnexpectedErrorf("stat %s: %v", activePath, err)
	}

	// (2) State-machine validation.
	from, err := lifecycle.Validate(lifecycle.KindIdea, activePath, opts.To)
	if err != nil {
		var ite *lifecycle.InvalidTransitionError
		if errors.As(err, &ite) {
			targets := statusNames(ite.LegalTargets)
			if len(targets) == 0 {
				return ChangeStatusResult{}, exitcode.InvalidStateErrorf(
					"invalid transition: idea %q is in status %q; no legal targets from this state",
					opts.Slug, string(ite.From))
			}
			return ChangeStatusResult{}, exitcode.InvalidStateErrorf(
				"invalid transition: idea %q is in status %q; legal targets: %s",
				opts.Slug, string(ite.From), strings.Join(targets, ", "))
		}
		if errors.Is(err, lifecycle.ErrStatusLineNotFound) {
			return ChangeStatusResult{}, exitcode.UnexpectedErrorf(
				"idea %s has no **Status:** line", activePath)
		}
		return ChangeStatusResult{}, exitcode.UnexpectedErrorf("reading idea status: %v", err)
	}

	// (3) Status line rewrite.
	origLine, err := lifecycle.Rewrite(activePath, opts.To)
	if err != nil {
		return ChangeStatusResult{}, exitcode.UnexpectedErrorf("rewriting status line: %v", err)
	}

	// fileAtArchived tracks whether the rename to archived/ succeeded.
	var fileAtArchived bool

	// fullRollback restores the pre-invocation state. Safe to call multiple
	// times (idempotent) and best-effort: if move-back fails we still try
	// to restore the status line so callers see the most-recoverable state.
	fullRollback := func() {
		if fileAtArchived {
			if err := os.Rename(archivedPath, activePath); err == nil {
				fileAtArchived = false
			}
		}
		if !fileAtArchived {
			_ = lifecycle.Rollback(activePath, origLine)
		}
	}

	// (3a) Optional transition note → `## Resolution` section, written
	// into the still-active file so it travels with any subsequent
	// archive move. The note write is rolled back together with the
	// status line on any later failure
	// (lifecycle-transitions#REQ:optional-transition-note).
	origBody, noteWritten, err := lifecycle.AppendResolutionNote(activePath, opts.Note)
	if err != nil {
		fullRollback()
		return ChangeStatusResult{}, exitcode.UnexpectedErrorf("writing transition note: %v", err)
	}
	if noteWritten {
		prev := fullRollback
		fullRollback = func() {
			// Restore the body first so the status-line rollback in prev()
			// operates on the original byte content.
			restorePath := activePath
			if fileAtArchived {
				restorePath = archivedPath
			}
			_ = lifecycle.RestoreBody(restorePath, origBody)
			prev()
		}
	}

	// (4) Archive side effect — only for --to=archived.
	if opts.To == lifecycle.IdeaArchived {
		// Materialize a lint-clean archived-index stub on first archive
		// (also ensures spec/ideas/archived/ exists). `specscore init`
		// does not create that directory — it comes into existence here,
		// and a directory without README.md fires the readme-exists rule
		// (error severity), which would in turn fire the post-mutation
		// rollback. Writing the stub keeps the verb's atomic-mutation
		// contract: the verb itself does not leave the spec tree in a
		// lint-failing state.
		archivedReadme := filepath.Join(filepath.Dir(archivedPath), "README.md")
		archivedReadmeCreated, err := EnsureArchivedIndexStub(opts.SpecRoot)
		if err != nil {
			fullRollback()
			return ChangeStatusResult{}, err
		}
		// Augment fullRollback to also remove the stub if WE created it.
		// (If it pre-existed, leave it alone.)
		if archivedReadmeCreated {
			prev := fullRollback
			fullRollback = func() {
				prev()
				_ = os.Remove(archivedReadme)
			}
		}

		// Collision check. If a stale archived file already exists,
		// exit 1 without overwriting and roll back the status rewrite.
		if _, err := osStatFn(archivedPath); err == nil {
			fullRollback()
			return ChangeStatusResult{}, exitcode.ConflictErrorf(
				"archive collision: %s already exists; aborted move from %s",
				archivedPath, activePath)
		} else if !os.IsNotExist(err) {
			fullRollback()
			return ChangeStatusResult{}, exitcode.UnexpectedErrorf(
				"stat archive target %s: %v", archivedPath, err)
		}

		if err := os.Rename(activePath, archivedPath); err != nil {
			fullRollback()
			return ChangeStatusResult{}, exitcode.UnexpectedErrorf(
				"moving %s → %s: %v", activePath, archivedPath, err)
		}
		fileAtArchived = true
	}

	// (5) PostMutation hook — typically `spec lint --fix` + verify.
	if err := opts.PostMutation(); err != nil {
		fullRollback()
		return ChangeStatusResult{}, err
	}

	return ChangeStatusResult{
		Slug: opts.Slug,
		From: from,
		To:   opts.To,
	}, nil
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

// LegalTransitionMatrix returns a human-readable, ANSI-free rendering of
// the Idea legal-transition matrix, suitable for inclusion in cobra
// `Long` help text. Built from lifecycle.LegalTargets so the help stays
// current as the matrix grows.
//
// Rows are emitted in the order returned by lifecycle.LegalStatuses
// (alphabetical) but only for statuses that have ≥1 outgoing legal
// target. The output is intentionally simple two-column ASCII — no box
// characters or color codes.
func LegalTransitionMatrix() string {
	statuses := lifecycle.LegalStatuses(lifecycle.KindIdea)

	type row struct {
		from    string
		targets string
	}
	var rows []row
	maxFrom := len("From")
	for _, s := range statuses {
		targets := lifecycle.LegalTargets(lifecycle.KindIdea, s)
		if len(targets) == 0 {
			continue
		}
		names := statusNames(targets)
		r := row{from: string(s), targets: strings.Join(names, ", ")}
		if len(r.from) > maxFrom {
			maxFrom = len(r.from)
		}
		rows = append(rows, r)
	}

	var sb strings.Builder
	sb.WriteString("Legal transitions:\n\n")
	fmt.Fprintf(&sb, "  %-*s  To\n", maxFrom, "From")
	fmt.Fprintf(&sb, "  %s  %s\n", strings.Repeat("-", maxFrom), "--")
	for _, r := range rows {
		fmt.Fprintf(&sb, "  %-*s  %s\n", maxFrom, r.from, r.targets)
	}
	return sb.String()
}

// IsLegalChangeStatusTarget reports whether status is one of the
// user-facing --to values for `specscore idea change-status`. The CLI
// uses this for early flag validation (exit 2 InvalidArgs) BEFORE the
// state-machine check, so unrecognized values like --to=draft fail fast
// with a clear message rather than slipping through to a state-machine
// rejection at exit 4. The legal --to set is the union of every To
// column in the Idea matrix.
func IsLegalChangeStatusTarget(s lifecycle.Status) bool {
	for _, t := range legalChangeStatusTargets() {
		if t == s {
			return true
		}
	}
	return false
}

// LegalChangeStatusTargetNames returns the canonical-titled names of
// the legal --to values, for stderr rendering when a user supplies an
// unrecognized value.
func LegalChangeStatusTargetNames() []string {
	targets := legalChangeStatusTargets()
	out := make([]string, len(targets))
	for i, t := range targets {
		out[i] = string(t)
	}
	return out
}

// legalChangeStatusTargets returns the union of every To column in the
// Idea matrix — these are exactly the values that can be supplied as
// --to and ever produce a legal transition. Computed from the lifecycle
// package's matrix so it stays in sync if the matrix grows.
//
// Today: {Approved, Archived}.
func legalChangeStatusTargets() []lifecycle.Status {
	seen := map[lifecycle.Status]struct{}{}
	for _, s := range lifecycle.LegalStatuses(lifecycle.KindIdea) {
		if sources := lifecycle.LegalSources(lifecycle.KindIdea, s); len(sources) > 0 {
			seen[s] = struct{}{}
		}
	}
	out := make([]lifecycle.Status, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	// Stable order — alphabetical by string value.
	for i := 0; i < len(out); i++ {
		for j := i + 1; j < len(out); j++ {
			if string(out[j]) < string(out[i]) {
				out[i], out[j] = out[j], out[i]
			}
		}
	}
	return out
}
