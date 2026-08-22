// Package decision — lifecycle transition orchestration for the Decision
// kind.
//
// This file hosts ChangeStatus, the orchestrator invoked by
// `specscore decision change-status`. It composes pkg/lifecycle/ primitives
// (state-machine validation, status-line rewrite, rollback) with Decision-
// specific behavior that has no analogue in Idea/Plan/Task/Lesson:
//
//   - Archival is NOT an optional axis for a Decision the way it is for an
//     Idea. pkg/lint's D-archived-location rule REQUIRES every Decision whose
//     Status is Rejected, Superseded, or Deprecated to live under
//     spec/decisions/archived/. ChangeStatus therefore relocates the file
//     itself, atomically with the status rewrite, whenever `to` is one of
//     those three dispositions — there is no separate `decision archive` verb
//     to call afterward.
//   - Superseding is bidirectional. pkg/lint's D-supersedes-bidirectional rule
//     requires the OLD decision's `**Superseded By:**` and the NEW
//     (successor) decision's `**Supersedes:**` to point at each other.
//     ChangeStatus writes BOTH sides in the same transition — the successor
//     file is mutated as a side effect of transitioning the superseded one.
//   - The decisions index is NOT self-healing the way the ideas index is.
//     pkg/lint's decisions-index fixer can ADD a missing row to the active
//     table (used by `decision new`) but has no fixer that adds a missing
//     archived-index bullet or removes an active-index row for a decision
//     that just relocated. ChangeStatus performs both edits itself
//     (removeActiveIndexRow / appendArchivedIndexEntry in index.go) before
//     invoking the PostMutation hook, so the tree is lint-clean the moment
//     the verb returns rather than depending on a lint-fixer capability that
//     does not exist today.
//
// LINT INVOCATION lives in the cobra adapter (internal/cli/decision.go), NOT
// here, mirroring every other kind package: the adapter passes a
// PostMutationHook callback into ChangeStatus; this package only knows "run
// the post-mutation hook, and roll back everything if it fails."
//
// Cross-references:
//
//   - Decision status vocabulary + archival/supersession rules:
//     pkg/lint/decision_rules.go (decisionValidStatuses, D-archived-location,
//     D-superseded-requires-successor, D-supersedes-bidirectional,
//     D-supersedes-target-exists).
//   - Sibling shapes: pkg/plan/transitions.go (prep band + disposition band,
//     **Superseded By:** on --to=superseded), pkg/lesson/transitions.go
//     (--note required for disposition targets, --successor required only
//     for Superseded), pkg/idea/archive.go (relocate + collision + full
//     rollback).
package decision

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

// Testable indirections for the pkg/lifecycle body-mutation helpers and a
// couple of OS primitives, so tests can exercise every rollback branch
// without needing to fabricate real failure conditions on disk.
var (
	appendNoteFn        = lifecycle.AppendResolutionNote
	setSupersededByFn   = lifecycle.SetSupersededBy
	setSupersedesFn     = lifecycle.SetSupersedes
	osReadFileFn        = os.ReadFile
	osWriteFileFn       = os.WriteFile
	osRemoveFn          = os.Remove
	osMkdirAllFn        = os.MkdirAll
	osStatFn            = os.Stat
	removeActiveRowFn   = removeActiveIndexRow
	appendArchivedRowFn = appendArchivedIndexEntry
)

// fullSlugRe matches the on-disk Decision identifier: the four-digit sequence
// number plus the bare slug, e.g. "0009-go-wasm-single-engine" — exactly the
// filename minus ".md". This is distinct from ValidateSlug, which validates
// only the BARE slug `decision new` takes as input (the number is assigned by
// the scaffolder, not supplied by the caller). change-status operates on an
// EXISTING file, so it takes and validates the full on-disk identifier, which
// is also the exact value the **Supersedes:**/**Superseded By:** fields carry
// (pkg/lint/decision_rules.go's bySlug keys).
var fullSlugRe = regexp.MustCompile(`^\d{4}-[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

// ValidateFullSlug validates the full on-disk Decision identifier (NNNN-slug)
// used by `decision change-status`'s positional argument and its --successor
// flag. A bare slug (no NNNN- prefix), as accepted by `decision new`, is
// rejected here — change-status resolves directly to a file, and that file is
// always named NNNN-<slug>.md.
func ValidateFullSlug(slug string) error {
	if slug == "" {
		return fmt.Errorf("slug must not be empty")
	}
	if !fullSlugRe.MatchString(slug) {
		return fmt.Errorf("slug %q must be the full on-disk identifier NNNN-<slug> (e.g. 0009-go-wasm-single-engine)", slug)
	}
	return nil
}

// PostMutationHook is the callback the cobra adapter wires to
// `specscore spec lint --fix` (plus a verify pass). It MUST return nil on
// success; a non-nil return triggers full rollback of every on-disk mutation
// ChangeStatus performed, and the error is returned by ChangeStatus.
type PostMutationHook func() error

// ChangeStatusOptions packages the inputs to ChangeStatus.
type ChangeStatusOptions struct {
	// SpecRoot is the project root that contains the `spec/` subtree (NOT the
	// `spec/` directory itself). The Decision is resolved at
	// SpecRoot/spec/decisions/<Slug>.md.
	SpecRoot string

	// Slug is the full on-disk Decision identifier, e.g.
	// "0009-go-wasm-single-engine". Caller is expected to have validated it
	// via ValidateFullSlug.
	Slug string

	// To is the canonical (title-case) target status. The cobra adapter
	// parses the raw --to value via lifecycle.ParseStatus before reaching
	// this function.
	To lifecycle.Status

	// Note is the optional free-form markdown transition note, written as a
	// `## Resolution` section atomically with the status rewrite. The cobra
	// adapter REQUIRES a non-empty Note for every disposition target
	// (Rejected, Superseded, Deprecated) — a Decision moving into archival
	// always carries a reason, both in the artifact body and (flattened to a
	// single line) as the archived-index entry's reason column.
	Note string

	// Successor is the full on-disk identifier of the Decision that
	// supersedes this one. REQUIRED (enforced by the cobra adapter) for
	// --to=Superseded, rejected otherwise. ChangeStatus writes
	// `**Supersedes:** <Slug>` onto the successor file and
	// `**Superseded By:** <Successor>` onto this one — the bidirectional link
	// D-supersedes-bidirectional requires.
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
	// ArchivedPath is non-empty when the transition relocated the file (every
	// disposition target: Rejected, Superseded, Deprecated).
	ArchivedPath string
}

// isDispositionStatus reports whether s is one of the three Decision
// dispositions that D-archived-location requires to live under
// spec/decisions/archived/.
func isDispositionStatus(s lifecycle.Status) bool {
	switch s {
	case lifecycle.DecisionRejected, lifecycle.DecisionSuperseded, lifecycle.DecisionDeprecated:
		return true
	}
	return false
}

// ChangeStatus performs a Decision-kind lifecycle transition end-to-end.
//
// Flow:
//
//  1. Resolve <slug> to spec/decisions/<slug>.md. A missing file returns
//     exit 3.
//  2. lifecycle.Validate against the KindDecision matrix. Illegal transitions
//     return exit 4.
//  3. When `to` is a disposition (Rejected/Superseded/Deprecated): pre-check
//     the archived-path collision (exit 1) and, for Superseded, resolve the
//     successor file (exit 2 if it does not exist) — BEFORE any mutation.
//  4. Rewrite the **Status:** line; capture original for rollback.
//  5. For Superseded: write **Supersedes:** on the successor and
//     **Superseded By:** on this decision.
//  6. Write the optional `## Resolution` note.
//  7. For a disposition: relocate the file to spec/decisions/archived/ and
//     sync both index files (remove the active row, add the archived entry).
//  8. Invoke the PostMutation hook. Failure → full rollback + exit 10.
//
// ChangeStatus performs all rollback internally — by the time it returns an
// error, the on-disk state is byte-identical to its pre-invocation shape.
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

	decisionsDir := filepath.Join(opts.SpecRoot, "spec", "decisions")
	activePath := filepath.Join(decisionsDir, opts.Slug+".md")

	// (1) Slug resolution.
	if _, err := osStatFn(activePath); err != nil {
		if os.IsNotExist(err) {
			return ChangeStatusResult{}, exitcode.NotFoundErrorf("decision not found at %s", activePath)
		}
		return ChangeStatusResult{}, exitcode.UnexpectedErrorf("stat %s: %v", activePath, err)
	}

	// (2) State-machine validation.
	from, err := lifecycle.Validate(lifecycle.KindDecision, activePath, opts.To)
	if err != nil {
		var ite *lifecycle.InvalidTransitionError
		if errors.As(err, &ite) {
			targets := statusNames(ite.LegalTargets)
			if len(targets) == 0 {
				return ChangeStatusResult{}, exitcode.InvalidStateErrorf(
					"invalid transition: decision %q is in status %q; no legal targets from this state",
					opts.Slug, string(ite.From))
			}
			return ChangeStatusResult{}, exitcode.InvalidStateErrorf(
				"invalid transition: decision %q is in status %q; legal targets: %s",
				opts.Slug, string(ite.From), strings.Join(targets, ", "))
		}
		if errors.Is(err, lifecycle.ErrStatusLineNotFound) {
			return ChangeStatusResult{}, exitcode.UnexpectedErrorf(
				"decision %s has no **Status:** line", activePath)
		}
		return ChangeStatusResult{}, exitcode.UnexpectedErrorf("reading decision status: %v", err)
	}

	disposition := isDispositionStatus(opts.To)
	archivedDir := filepath.Join(decisionsDir, "archived")
	archivedPath := filepath.Join(archivedDir, opts.Slug+".md")

	// (3a) Archived-path collision pre-check, BEFORE any mutation.
	if disposition {
		if _, err := osStatFn(archivedPath); err == nil {
			return ChangeStatusResult{}, exitcode.ConflictErrorf(
				"archive collision: %s already exists; aborted move from %s", archivedPath, activePath)
		} else if !os.IsNotExist(err) {
			return ChangeStatusResult{}, exitcode.UnexpectedErrorf("stat %s: %v", archivedPath, err)
		}
	}

	// (3b) Successor resolution, BEFORE any mutation.
	var successorPath string
	if opts.To == lifecycle.DecisionSuperseded {
		successorPath = filepath.Join(decisionsDir, opts.Successor+".md")
		if _, err := osStatFn(successorPath); err != nil {
			if os.IsNotExist(err) {
				return ChangeStatusResult{}, exitcode.InvalidArgsErrorf(
					"successor decision %q does not resolve to an existing decision at spec/decisions/%s.md",
					opts.Successor, opts.Successor)
			}
			return ChangeStatusResult{}, exitcode.UnexpectedErrorf("stat %s: %v", successorPath, err)
		}
	}

	// (4) Status line rewrite.
	origLine, err := lifecycle.Rewrite(activePath, opts.To)
	if err != nil {
		return ChangeStatusResult{}, exitcode.UnexpectedErrorf("rewriting status line: %v", err)
	}
	fullRollback := func() {
		_ = lifecycle.Rollback(activePath, origLine)
	}

	// (5) Bidirectional supersession link.
	if opts.To == lifecycle.DecisionSuperseded {
		origSupersedes, wrote, err := setSupersedesFn(successorPath, opts.Slug)
		if err != nil {
			fullRollback()
			return ChangeStatusResult{}, exitcode.UnexpectedErrorf("writing Supersedes reference on successor: %v", err)
		}
		if wrote {
			prev := fullRollback
			fullRollback = func() {
				_ = lifecycle.RestoreBody(successorPath, origSupersedes)
				prev()
			}
		}

		origSuccBy, wrote, err := setSupersededByFn(activePath, opts.Successor)
		if err != nil {
			fullRollback()
			return ChangeStatusResult{}, exitcode.UnexpectedErrorf("writing Superseded By reference: %v", err)
		}
		if wrote {
			prev := fullRollback
			fullRollback = func() {
				_ = lifecycle.RestoreBody(activePath, origSuccBy)
				prev()
			}
		}
	}

	// (6) Optional transition note → `## Resolution` section.
	origBody, noteWritten, err := appendNoteFn(activePath, opts.Note)
	if err != nil {
		fullRollback()
		return ChangeStatusResult{}, exitcode.UnexpectedErrorf("writing transition note: %v", err)
	}
	if noteWritten {
		prev := fullRollback
		fullRollback = func() {
			_ = lifecycle.RestoreBody(activePath, origBody)
			prev()
		}
	}

	// (7) Relocation + index sync for a disposition target.
	var resultArchivedPath string
	if disposition {
		content, err := osReadFileFn(activePath)
		if err != nil {
			fullRollback()
			return ChangeStatusResult{}, exitcode.UnexpectedErrorf("reading %s: %v", activePath, err)
		}
		if err := osMkdirAllFn(archivedDir, 0o755); err != nil {
			fullRollback()
			return ChangeStatusResult{}, exitcode.UnexpectedErrorf("creating %s: %v", archivedDir, err)
		}
		if err := osWriteFileFn(archivedPath, content, 0o644); err != nil {
			fullRollback()
			return ChangeStatusResult{}, exitcode.UnexpectedErrorf("writing %s: %v", archivedPath, err)
		}
		if err := osRemoveFn(activePath); err != nil {
			_ = osRemoveFn(archivedPath)
			fullRollback()
			return ChangeStatusResult{}, exitcode.UnexpectedErrorf("removing %s: %v", activePath, err)
		}
		// NOTE: each rollback wrapper below uses a UNIQUELY named "prevX"
		// capture rather than the repeated `prev := fullRollback` idiom used
		// elsewhere in this file. That idiom is safe only when each wrap site
		// sits in its OWN block scope (as with the `if wrote {}` /
		// `if noteWritten {}` guards above) — reusing the name "prev" across
		// SIBLING steps in this flatter disposition block would shadow across
		// some sites but plainly reassign (clobber) the same variable at
		// others, and because Go closures capture variables (not values), a
		// later `prev = fullRollback` retroactively rewrites what an EARLIER
		// closure calls, wiring a rollback cycle (infinite recursion) instead
		// of a chain. Unique names make each closure's capture unambiguous.
		prevMove := fullRollback
		fullRollback = func() {
			// Restore the active copy first (in its fully-mutated, pre-move
			// content) so prevMove()'s status/link/note rollbacks — which all
			// address activePath — find the file exactly where they expect it.
			_ = osWriteFileFn(activePath, content, 0o644)
			_ = osRemoveFn(archivedPath)
			prevMove()
		}

		// Index sync: remove the active-table row, add the archived-list entry.
		activeIdxPath := filepath.Join(decisionsDir, "README.md")
		origActiveIdx, changed, err := removeActiveRowFn(activeIdxPath, opts.Slug)
		if err != nil {
			fullRollback()
			return ChangeStatusResult{}, exitcode.UnexpectedErrorf("updating active decisions index: %v", err)
		}
		if changed {
			prevActiveRow := fullRollback
			fullRollback = func() {
				_ = osWriteFileFn(activeIdxPath, origActiveIdx, 0o644)
				prevActiveRow()
			}
		}

		stubCreated, err := EnsureArchivedIndexStub(opts.SpecRoot)
		if err != nil {
			fullRollback()
			return ChangeStatusResult{}, err
		}
		archivedIdxPath := filepath.Join(archivedDir, "README.md")
		if stubCreated {
			prevStub := fullRollback
			fullRollback = func() {
				_ = osRemoveFn(archivedIdxPath)
				prevStub()
			}
		}

		date, status := parseDateAndStatus(content)
		reason := buildArchivedReason(opts.Note)
		origArchivedIdx, err := appendArchivedRowFn(archivedIdxPath, date, opts.Slug, status, reason)
		if err != nil {
			fullRollback()
			return ChangeStatusResult{}, exitcode.UnexpectedErrorf("updating archived decisions index: %v", err)
		}
		prevArchivedRow := fullRollback
		fullRollback = func() {
			_ = osWriteFileFn(archivedIdxPath, origArchivedIdx, 0o644)
			prevArchivedRow()
		}

		resultArchivedPath = archivedPath
	}

	// (8) PostMutation hook — typically `spec lint --fix` + verify.
	if err := opts.PostMutation(); err != nil {
		fullRollback()
		return ChangeStatusResult{}, err
	}

	// (9) Persistence check. The hook returning nil proves only that the
	// derived-index pass did not ERROR, not that the requested status is
	// still the one on disk after its `--fix` rewrites. Verify at the FINAL
	// path (a disposition relocated the file), so the caller never prints
	// `<slug>: <from> → <to>` over a file that says something else.
	finalPath := activePath
	if resultArchivedPath != "" {
		finalPath = resultArchivedPath
	}
	if err := lifecycle.VerifyPersistedStatus(finalPath, opts.To); err != nil {
		fullRollback()
		return ChangeStatusResult{}, exitcode.UnexpectedErrorf("%v (rolled back)", err)
	}

	return ChangeStatusResult{
		Slug:         opts.Slug,
		From:         from,
		To:           opts.To,
		ArchivedPath: resultArchivedPath,
	}, nil
}

// decisionDateFieldRe / decisionStatusFieldRe extract the **Date:** and
// **Status:** header field values from a Decision file's raw content, for
// building the archived-index entry. Deliberately independent of pkg/lint's
// parser (this package must not import pkg/lint; see the package doc).
var (
	decisionDateFieldRe   = regexp.MustCompile(`(?m)^\*\*Date:\*\*[ \t]+(.+?)[ \t]*\r?$`)
	decisionStatusFieldRe = regexp.MustCompile(`(?m)^\*\*Status:\*\*[ \t]+(.+?)[ \t]*\r?$`)
)

// parseDateAndStatus extracts the **Date:** and **Status:** header values
// from a Decision file's content, for the archived-index entry. Both default
// to "—" if somehow absent (defensive; a valid Decision always has both).
func parseDateAndStatus(content []byte) (date, status string) {
	date, status = "—", "—"
	if m := decisionDateFieldRe.FindSubmatch(content); m != nil {
		date = strings.TrimSpace(string(m[1]))
	}
	if m := decisionStatusFieldRe.FindSubmatch(content); m != nil {
		status = strings.TrimSpace(string(m[1]))
	}
	return date, status
}

// buildArchivedReason flattens a (required, non-empty) transition note to a
// single line for the archived-index entry's reason column — index entries
// are one bullet per line, so embedded newlines are joined with a space.
func buildArchivedReason(note string) string {
	fields := strings.Fields(note)
	return strings.Join(fields, " ")
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
// legal --to values, for stderr rendering and help text.
func LegalChangeStatusTargetNames() []string {
	targets := legalChangeStatusTargets()
	out := make([]string, len(targets))
	for i, t := range targets {
		out[i] = string(t)
	}
	return out
}

// legalChangeStatusTargets returns the union of every To column in the
// Decision matrix, sorted alphabetically for deterministic output.
//
// Unlike Idea (whose Draft status is a source-only state, never a legal
// --to value) and Plan (whose execution-band statuses are lint-derived, not
// human-settable), every status in the Decision matrix appears as a To
// somewhere: Draft is reachable via In Review → Draft, so this set is
// identical to lifecycle.LegalStatuses(KindDecision) today. There is
// deliberately no IsLegalChangeStatusTarget guard in the cobra adapter (the
// pattern Idea/Plan use to reject a recognized-but-never-a-target status at
// exit 2 before the state-machine check) — for Decision that guard would
// never fire, since ParseStatus already only returns values from this same
// set.
func legalChangeStatusTargets() []lifecycle.Status {
	seen := map[lifecycle.Status]struct{}{}
	for _, s := range lifecycle.LegalStatuses(lifecycle.KindDecision) {
		if sources := lifecycle.LegalSources(lifecycle.KindDecision, s); len(sources) > 0 {
			seen[s] = struct{}{}
		}
	}
	out := make([]lifecycle.Status, 0, len(seen))
	for s := range seen {
		out = append(out, s)
	}
	sort.Slice(out, func(i, j int) bool { return string(out[i]) < string(out[j]) })
	return out
}

// LegalTransitionMatrix returns a human-readable, ANSI-free rendering of the
// Decision legal-transition matrix, suitable for cobra `Long` help text.
func LegalTransitionMatrix() string {
	statuses := lifecycle.LegalStatuses(lifecycle.KindDecision)

	type row struct {
		from    string
		targets string
	}
	var rows []row
	maxFrom := len("From")
	for _, s := range statuses {
		targets := lifecycle.LegalTargets(lifecycle.KindDecision, s)
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
	sb.WriteString("Legal transitions:\n\n")
	fmt.Fprintf(&sb, "  %-*s  To\n", maxFrom, "From")
	fmt.Fprintf(&sb, "  %s  %s\n", strings.Repeat("-", maxFrom), "--")
	for _, r := range rows {
		fmt.Fprintf(&sb, "  %-*s  %s\n", maxFrom, r.from, r.targets)
	}
	return sb.String()
}
