package sidekick

// Seed terminal relocation: status rewrite + `type: sidekick-seed` tag + move
// to spec/ideas/archived/<slug>.md, with archive-path-collision handling and
// full rollback to the original seeds/ state on any post-rewrite failure.
//
// Feature implemented: cli/sidekick/change-status
// (terminal-relocation, rollback-includes-relocation; the exit-1 collision +
// restore precedent is idea/change-status#req:archive-relocation).
//
// This primitive owns the on-disk side effects the change-status verb (Task 6)
// composes. It does NOT run `spec lint --fix`, print the success line, or
// append the `## Resolution` note — those are the command's job. The command
// passes a RollbackHook so that any body mutation it performed BEFORE calling
// Relocate (e.g. a resolution note) is undone as part of the same atomic
// rollback, satisfying "no `## Resolution` section written" on failure
// (rollback-includes-relocation).

import (
	"os"
	"path/filepath"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

// RelocateOptions parameterizes Relocate.
type RelocateOptions struct {
	// SpecRoot is the project root containing the spec/ subtree.
	SpecRoot string
	// Slug is the seed slug (file basename without .md).
	Slug string
	// SeedPath is the resolved absolute path to spec/ideas/seeds/<slug>.md,
	// as returned by ResolveSeedPath.
	SeedPath string
	// NewStatus is the canonical title-case terminal status to write into the
	// frontmatter, as returned by ParseSeedTarget.
	NewStatus lifecycle.Status

	// RollbackHook, when non-nil, is invoked during rollback (after the seed's
	// frontmatter status and the file location have been restored) so the
	// caller can undo any body mutation it applied before calling Relocate —
	// e.g. lifecycle.RestoreBody to drop a `## Resolution` note. Its error is
	// surfaced if the primary rollback succeeded.
	RollbackHook func() error

	// AfterMoveHook, when non-nil, is invoked immediately after the file has
	// been moved to the archived path and re-written with the `type:` tag.
	// The change-status verb injects its `spec lint --fix` index sync here so
	// a lint failure triggers the same full rollback (move back to seeds/,
	// restore bytes, RollbackHook) as any other post-move failure
	// (cli/sidekick/change-status#req:rollback-includes-relocation).
	AfterMoveHook func() error

	// afterMoveHook is a test seam: when non-nil it is invoked immediately
	// after the file has been moved to the archived path and re-written with
	// the `type:` tag, simulating a post-relocation failure (e.g. a lint
	// error) so the rollback path is exercisable. Unexported — production
	// callers never set it.
	afterMoveHook func() error
}

// Relocate performs the terminal relocation for a sidekick seed:
//
//  1. If spec/ideas/archived/<slug>.md already exists, exits 1 (Conflict)
//     WITHOUT touching the source seed — it remains byte-identical at
//     spec/ideas/seeds/<slug>.md with its original frontmatter status.
//  2. Rewrites the frontmatter status: value to NewStatus (Task 3).
//  3. Adds the frontmatter key type: sidekick-seed.
//  4. mkdir-p's spec/ideas/archived/ and moves the seed there.
//
// On ANY failure after the status rewrite (step 2 onward) — including the
// afterMoveHook test seam — Relocate restores the seed to its exact
// pre-invocation seeds/ state: original status, no type, file back at
// seeds/<slug>.md, and (via RollbackHook) no body note. The returned error
// carries exit 1 for a collision and exit 10 for an I/O or post-move failure.
func Relocate(opts RelocateOptions) error {
	archivedPath := archivedSeedPath(opts.SpecRoot, opts.Slug)

	// Step 1: collision pre-check. The source seed is untouched, so on a
	// collision it stays byte-identical with its original `status:` value.
	if _, err := os.Stat(archivedPath); err == nil {
		return exitcode.ConflictErrorf(
			"archive destination %s already exists; refusing to overwrite",
			filepath.Join("spec", "ideas", "archived", opts.Slug+".md"))
	} else if !os.IsNotExist(err) {
		return exitcode.UnexpectedErrorf("stat %s: %v", archivedPath, err)
	}

	// Capture the exact pre-invocation bytes so any post-rewrite failure can
	// restore the seed verbatim at the seeds/ path.
	originalBytes, err := os.ReadFile(opts.SeedPath)
	if err != nil {
		return exitcode.UnexpectedErrorf("reading seed %s: %v", opts.SeedPath, err)
	}

	// Step 2: rewrite the frontmatter status in place.
	if _, err := RewriteFrontmatterStatus(opts.SeedPath, string(opts.NewStatus)); err != nil {
		// Pre-move failure: the only mutation attempted is the status rewrite,
		// which left the file untouched on error. Still restore the body hook
		// to honor a note the caller may have written before calling us.
		return rollback(opts, originalBytes, exitcode.UnexpectedErrorf(
			"rewriting seed status: %v", err))
	}

	// Step 3: add type: sidekick-seed to the frontmatter.
	if err := addSidekickSeedType(opts.SeedPath); err != nil {
		return rollback(opts, originalBytes, exitcode.UnexpectedErrorf(
			"adding type tag: %v", err))
	}

	// Step 4: mkdir-p archived/ and move the seed there.
	if err := os.MkdirAll(filepath.Dir(archivedPath), 0o755); err != nil {
		return rollback(opts, originalBytes, exitcode.UnexpectedErrorf(
			"creating archived dir: %v", err))
	}
	if err := os.Rename(opts.SeedPath, archivedPath); err != nil {
		return rollback(opts, originalBytes, exitcode.UnexpectedErrorf(
			"moving seed to archived: %v", err))
	}

	// Post-move hooks: the exported AfterMoveHook (the command's lint sync) and
	// the unexported afterMoveHook test seam both run here. A failure in either
	// triggers a full rollback that also moves the file back to seeds/.
	for _, hook := range []func() error{opts.AfterMoveHook, opts.afterMoveHook} {
		if hook == nil {
			continue
		}
		if err := hook(); err != nil {
			// Move back to seeds/ first, then restore bytes + hook.
			if mvErr := os.Rename(archivedPath, opts.SeedPath); mvErr != nil {
				return exitcode.UnexpectedErrorf(
					"post-move failure (%v) AND rollback move failed: %v", err, mvErr)
			}
			return rollback(opts, originalBytes, exitcode.UnexpectedErrorf(
				"post-relocation failure: %v", err))
		}
	}

	return nil
}

// rollback restores the seed file at opts.SeedPath to originalBytes (original
// status, no type), invokes the caller's RollbackHook if set, and returns
// cause. It assumes the file already lives at opts.SeedPath (any archived-path
// move has been undone by the caller). A failure to restore is surfaced as an
// exit-10 error that names the original cause.
func rollback(opts RelocateOptions, originalBytes []byte, cause error) error {
	if err := os.WriteFile(opts.SeedPath, originalBytes, 0o644); err != nil {
		return exitcode.UnexpectedErrorf(
			"rollback failed restoring %s (original error: %v): %v",
			opts.SeedPath, cause, err)
	}
	if opts.RollbackHook != nil {
		if err := opts.RollbackHook(); err != nil {
			return exitcode.UnexpectedErrorf(
				"rollback hook failed (original error: %v): %v", cause, err)
		}
	}
	return cause
}

// archivedSeedPath returns the absolute spec/ideas/archived/<slug>.md path.
func archivedSeedPath(specRoot, slug string) string {
	return filepath.Join(specRoot, "spec", "ideas", "archived", slug+".md")
}

// addSidekickSeedType inserts a `type: sidekick-seed` line into the leading
// YAML frontmatter block at seedPath, idempotently rewriting an existing
// `type:` key to the canonical value. Every other byte (line ordering,
// indentation, line endings, and all other keys and body) is preserved. The
// key is inserted immediately after the opening `---` fence when absent.
func addSidekickSeedType(seedPath string) error {
	content, err := os.ReadFile(seedPath)
	if err != nil {
		return err
	}
	lines := splitKeepTerminators(content)
	if len(lines) == 0 {
		return ErrFrontmatterStatusNotFound
	}
	if body, _ := splitTerminator(lines[0]); body != "---" {
		return ErrFrontmatterStatusNotFound
	}

	// Newline style for an inserted line: mirror the opening fence's terminator.
	_, openTerm := splitTerminator(lines[0])
	if openTerm == "" {
		openTerm = "\n"
	}

	for i := 1; i < len(lines); i++ {
		body, term := splitTerminator(lines[i])
		if body == "---" {
			break // closing fence reached, key absent — insert below.
		}
		if frontmatterKeyOf(body) == "type" {
			lines[i] = "type: sidekick-seed" + term
			return writeBack(seedPath, lines)
		}
	}

	inserted := "type: sidekick-seed" + openTerm
	out := make([]string, 0, len(lines)+1)
	out = append(out, lines[0], inserted)
	out = append(out, lines[1:]...)
	return writeBack(seedPath, out)
}

// frontmatterKeyOf returns the YAML key of a trimmed `key: value` body, or "".
func frontmatterKeyOf(body string) string {
	for i := 0; i < len(body); i++ {
		if body[i] == ':' {
			return body[:i]
		}
	}
	return ""
}

// writeBack rewrites seedPath with lines, preserving its file mode.
func writeBack(seedPath string, lines []string) error {
	stat, err := os.Stat(seedPath)
	if err != nil {
		return err
	}
	return os.WriteFile(seedPath, joinLines(lines), stat.Mode().Perm())
}
