// Package idea — orthogonal archival for the Idea kind.
//
// Archival is NOT a lifecycle status. An archived Idea keeps its real
// terminal **Status:** (e.g. Rejected, Stale, Implemented) and is
// additionally marked archived via two coordinated facts:
//
//   - a `**Archived:** true` header line in the artifact, and
//   - relocation to spec/ideas/archived/<slug>.md.
//
// `idea archive` sets both; `idea unarchive` reverses both. Neither touches
// the **Status:** line — that is the sole concern of ChangeStatus. The two
// axes are independent: a Rejected Idea may be active or archived; an
// archived Idea may be Rejected, Stale, or Implemented.
//
// Cross-references:
//
//   - Verb spec: spec/features/cli/idea/archive/README.md
//   - Status vocabulary: archival-not-a-status (SpecScore-wide)
package idea

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// ArchiveOptions packages the inputs to Archive.
type ArchiveOptions struct {
	// SpecRoot is the project root that contains the `spec/` subtree.
	SpecRoot string
	// Slug is the Idea slug; the active file is resolved at
	// SpecRoot/spec/ideas/<Slug>.md.
	Slug string
	// Note is an optional free-form `**Archive Note:**` value tied to the
	// archive action. Empty (after trimming) writes no note line. The
	// disposition reason belongs on the terminal status transition, not here.
	Note string
	// PostMutation is the post-relocation hook (typically a spec-lint pass).
	// Required.
	PostMutation PostMutationHook
}

// UnarchiveOptions packages the inputs to Unarchive.
type UnarchiveOptions struct {
	SpecRoot     string
	Slug         string
	PostMutation PostMutationHook
}

// ArchiveResult is the success payload returned by Archive/Unarchive.
type ArchiveResult struct {
	Slug string
	// From is the path the file moved from; To is where it now lives.
	From string
	To   string
}

// Archive files an active Idea out of view: it sets `**Archived:** true`
// (and an optional `**Archive Note:**`) in the header and relocates the file
// from spec/ideas/<slug>.md to spec/ideas/archived/<slug>.md, preserving the
// **Status:** line untouched.
//
// Flow:
//
//  1. Resolve <slug> at the active path (exit 3 if missing).
//  2. Materialize the lint-clean archived-index stub (exit 10 on I/O error).
//  3. Collision check on the archived path (exit 1).
//  4. Snapshot the original bytes, write the flagged content to the archived
//     path, and remove the active file.
//  5. Run the PostMutation hook; any failure rolls back to the byte-identical
//     pre-invocation state (file back at the active path, original content).
func Archive(opts ArchiveOptions) (ArchiveResult, error) {
	if opts.SpecRoot == "" {
		return ArchiveResult{}, exitcode.UnexpectedErrorf("Archive: SpecRoot required")
	}
	if opts.Slug == "" {
		return ArchiveResult{}, exitcode.InvalidArgsErrorf("Archive: slug required")
	}
	if opts.PostMutation == nil {
		return ArchiveResult{}, exitcode.UnexpectedErrorf("Archive: PostMutation hook required")
	}

	activePath := filepath.Join(opts.SpecRoot, "spec", "ideas", opts.Slug+".md")
	archivedPath := filepath.Join(opts.SpecRoot, "spec", "ideas", "archived", opts.Slug+".md")

	// (1) Slug resolution — active path only.
	orig, err := os.ReadFile(activePath)
	if err != nil {
		if os.IsNotExist(err) {
			return ArchiveResult{}, exitcode.NotFoundErrorf("idea not found at %s", activePath)
		}
		return ArchiveResult{}, exitcode.UnexpectedErrorf("reading %s: %v", activePath, err)
	}

	// (2) Materialize a lint-clean archived-index stub on first archive
	// (also ensures spec/ideas/archived/ exists). removeStub undoes the stub
	// iff WE created it (no-op otherwise), so every error/rollback path can
	// call it unconditionally.
	archivedReadme := filepath.Join(filepath.Dir(archivedPath), "README.md")
	stubCreated, err := EnsureArchivedIndexStub(opts.SpecRoot)
	if err != nil {
		return ArchiveResult{}, err
	}
	removeStub := func() {
		if stubCreated {
			_ = os.Remove(archivedReadme)
		}
	}

	// (3) Collision check.
	if _, err := osStatFn(archivedPath); err == nil {
		removeStub()
		return ArchiveResult{}, exitcode.ConflictErrorf(
			"archive collision: %s already exists; aborted move from %s",
			archivedPath, activePath)
	} else if !os.IsNotExist(err) {
		removeStub()
		return ArchiveResult{}, exitcode.UnexpectedErrorf(
			"stat archive target %s: %v", archivedPath, err)
	}

	// (4) Set the archived axis in the header and relocate.
	archivedContent := setArchivedHeader(orig, true, opts.Note)
	if err := os.WriteFile(archivedPath, archivedContent, 0o644); err != nil {
		removeStub()
		return ArchiveResult{}, exitcode.UnexpectedErrorf(
			"writing %s: %v", archivedPath, err)
	}
	if err := os.Remove(activePath); err != nil {
		_ = os.Remove(archivedPath)
		removeStub()
		return ArchiveResult{}, exitcode.UnexpectedErrorf(
			"removing %s: %v", activePath, err)
	}

	rollback := func() {
		_ = os.WriteFile(activePath, orig, 0o644)
		_ = os.Remove(archivedPath)
		removeStub()
	}

	// (5) PostMutation hook.
	if err := opts.PostMutation(); err != nil {
		rollback()
		return ArchiveResult{}, err
	}

	return ArchiveResult{Slug: opts.Slug, From: activePath, To: archivedPath}, nil
}

// Unarchive reverses Archive: it clears the `**Archived:**` axis (removing
// the `**Archived:**` and `**Archive Note:**` header lines) and relocates the
// file from spec/ideas/archived/<slug>.md back to spec/ideas/<slug>.md,
// preserving the **Status:** line. The Idea retains whatever terminal status
// it carried while archived.
//
// Flow mirrors Archive: resolve at the archived path (exit 3 if missing),
// collision check on the active path (exit 1), relocate, then run the
// PostMutation hook with full rollback on failure.
func Unarchive(opts UnarchiveOptions) (ArchiveResult, error) {
	if opts.SpecRoot == "" {
		return ArchiveResult{}, exitcode.UnexpectedErrorf("Unarchive: SpecRoot required")
	}
	if opts.Slug == "" {
		return ArchiveResult{}, exitcode.InvalidArgsErrorf("Unarchive: slug required")
	}
	if opts.PostMutation == nil {
		return ArchiveResult{}, exitcode.UnexpectedErrorf("Unarchive: PostMutation hook required")
	}

	activePath := filepath.Join(opts.SpecRoot, "spec", "ideas", opts.Slug+".md")
	archivedPath := filepath.Join(opts.SpecRoot, "spec", "ideas", "archived", opts.Slug+".md")

	// (1) Slug resolution — archived path only.
	orig, err := os.ReadFile(archivedPath)
	if err != nil {
		if os.IsNotExist(err) {
			return ArchiveResult{}, exitcode.NotFoundErrorf("archived idea not found at %s", archivedPath)
		}
		return ArchiveResult{}, exitcode.UnexpectedErrorf("reading %s: %v", archivedPath, err)
	}

	// (2) Collision check on the active path.
	if _, err := osStatFn(activePath); err == nil {
		return ArchiveResult{}, exitcode.ConflictErrorf(
			"unarchive collision: %s already exists; aborted move from %s",
			activePath, archivedPath)
	} else if !os.IsNotExist(err) {
		return ArchiveResult{}, exitcode.UnexpectedErrorf(
			"stat unarchive target %s: %v", activePath, err)
	}

	// (3) Clear the archived axis and relocate.
	activeContent := setArchivedHeader(orig, false, "")
	if err := os.WriteFile(activePath, activeContent, 0o644); err != nil {
		return ArchiveResult{}, exitcode.UnexpectedErrorf(
			"writing %s: %v", activePath, err)
	}
	if err := os.Remove(archivedPath); err != nil {
		_ = os.Remove(activePath)
		return ArchiveResult{}, exitcode.UnexpectedErrorf(
			"removing %s: %v", archivedPath, err)
	}

	rollback := func() {
		_ = os.WriteFile(archivedPath, orig, 0o644)
		_ = os.Remove(activePath)
	}

	// (4) PostMutation hook.
	if err := opts.PostMutation(); err != nil {
		rollback()
		return ArchiveResult{}, err
	}

	return ArchiveResult{Slug: opts.Slug, From: archivedPath, To: activePath}, nil
}

// setArchivedHeader returns content with the `**Archived:**` axis set to
// archived (and, when archiving with a non-empty note, an `**Archive Note:**`
// line). When archived is false, both the `**Archived:**` and
// `**Archive Note:**` header lines are removed entirely (an absent flag means
// "not archived").
//
// The mutation is confined to the header band — the lines before the first
// `## ` section heading. Existing `**Archived:**` / `**Archive Note:**` lines
// are replaced in place; otherwise the lines are inserted immediately after
// the `**Status:**` line (the axis paired with status). Every other byte,
// including the trailing newline shape, is preserved.
func setArchivedHeader(content []byte, archived bool, note string) []byte {
	note = strings.TrimSpace(note)
	// Split preserving the original line endings is unnecessary here: idea
	// files use \n (enforced elsewhere); we operate on \n-split lines and
	// re-join with \n, preserving a trailing newline if present.
	hadTrailingNewline := strings.HasSuffix(string(content), "\n")
	lines := strings.Split(strings.TrimSuffix(string(content), "\n"), "\n")

	headerEnd := len(lines)
	for i, ln := range lines {
		if strings.HasPrefix(strings.TrimSpace(ln), "## ") {
			headerEnd = i
			break
		}
	}

	// Remove any existing Archived / Archive Note lines within the header.
	var out []string
	statusIdx := -1
	for i := 0; i < len(lines); i++ {
		ln := lines[i]
		if i < headerEnd {
			t := strings.TrimSpace(ln)
			if strings.HasPrefix(t, "**Archived:**") || strings.HasPrefix(t, "**Archive Note:**") {
				continue
			}
			if statusIdx == -1 && strings.HasPrefix(t, "**Status:**") {
				statusIdx = len(out)
			}
		}
		out = append(out, ln)
	}

	if archived {
		ins := []string{"**Archived:** true"}
		if note != "" {
			ins = append(ins, "**Archive Note:** "+note)
		}
		at := statusIdx + 1
		if statusIdx == -1 {
			// No Status line found (malformed); prepend so the flag is at
			// least present. Lint will flag the missing Status separately.
			at = 0
		}
		out = append(out[:at], append(ins, out[at:]...)...)
	}

	joined := strings.Join(out, "\n")
	if hadTrailingNewline {
		joined += "\n"
	}
	return []byte(joined)
}
