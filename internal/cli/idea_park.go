package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/idea"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
	"github.com/spf13/cobra"
)

var statIdeaForParking = os.Stat

// See feature_park.go: these seams preserve coverage of the CLI's defensive
// no-write guard without changing production behavior.
var (
	setIdeaParkedFn   = lifecycle.SetParked
	clearIdeaParkedFn = lifecycle.ClearParked
)

// ideaParkCommand registers `specscore idea park <slug> --reason "..."`.
//
// Parked is an axis orthogonal to **Status:** — see pkg/lifecycle/parked.go
// for the full contract this verb and `idea unpark` implement, and
// pkg/idea/archive.go for the sibling "Archived" axis this mirrors in
// shape (a structured header fact, NOT a lifecycle transition). Parking
// never relocates the file — unlike archive/unarchive, park/unpark act on
// the Idea in place at spec/ideas/<slug>.md.
func ideaParkCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "park <slug> --reason \"...\"",
		Short: "Defer an Idea without changing its Status (sets the orthogonal Parked axis)",
		Long: `Marks spec/ideas/<slug>.md with **Parked:** true, a **Parked Reason:**
(from the required --reason flag), and a **Parked Date:** (today, UTC),
WITHOUT touching **Status:**. Parking is a scheduling decision ("not this
release"), independent of maturity ("how ready is it?") — a fully-specced,
ratified Idea can be parked just as easily as a Draft. It is NOT a
lifecycle transition: parking is never gated on the Idea's current status.

--reason is REQUIRED: a bare park with no explanation is rejected before
any mutation (exit 2), so the parked axis never rots into an unaudited
graveyard. Re-running park on an already-parked Idea overwrites the
reason and resets the date to today — the way to confirm "still
deliberately deferred" is to re-park with a fresh --reason.

On success the verb runs ` + "`specscore spec lint --fix`" + `, prints
"<slug>: parked", and exits 0. If anything fails after the write (lint
failure, I/O error), the header is restored to its pre-invocation form
before the verb exits.

Examples:

  specscore idea park payment-fraud --reason "good idea, not v1"
`,
		Args: cobra.ExactArgs(1),
		RunE: runIdeaPark,
	}
	cmd.Flags().String("reason", "", "required: why this Idea is being parked")
	_ = cmd.MarkFlagRequired("reason")
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	return cmd
}

// ideaFilePath returns the resolved spec/ideas/<slug>.md path for specRoot
// (the project root, NOT the spec/ directory itself) — mirrors the inline
// path computation idea.Archive/idea.Unarchive use.
func ideaFilePath(specRoot, slug string) string {
	return filepath.Join(specRoot, "spec", "ideas", slug+".md")
}

func runIdeaPark(cmd *cobra.Command, args []string) error {
	slug := args[0]
	if err := idea.ValidateSlug(slug); err != nil {
		return exitcode.InvalidArgsErrorf("invalid slug %q: %v", slug, err)
	}

	reason, _ := cmd.Flags().GetString("reason")
	projectFlag, _ := cmd.Flags().GetString("project")
	specRoot, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}

	path := ideaFilePath(specRoot, slug)
	if _, statErr := statIdeaForParking(path); statErr != nil {
		if os.IsNotExist(statErr) {
			return exitcode.NotFoundErrorf("idea not found: %s", path)
		}
		return exitcode.UnexpectedErrorf("stat %s: %v", path, statErr)
	}

	orig, wrote, err := setIdeaParkedFn(path, reason)
	if err != nil {
		if errors.Is(err, lifecycle.ErrParkReasonRequired) {
			return exitcode.InvalidArgsError("park requires --reason (a non-empty reason for the deferral)")
		}
		if errors.Is(err, lifecycle.ErrStatusLineNotFound) {
			return exitcode.UnexpectedErrorf("%s has no **Status:** line to anchor the Parked axis", path)
		}
		return exitcode.UnexpectedErrorf("parking %s: %v", slug, err)
	}
	if !wrote {
		return exitcode.UnexpectedErrorf("parking %s: no changes written", slug)
	}

	if hookErr := lintPostMutationHook(filepath.Join(specRoot, "spec"))(); hookErr != nil {
		if rbErr := lifecycle.RestoreBody(path, orig); rbErr != nil {
			return exitcode.UnexpectedErrorf("%v; rollback also failed: %v", hookErr, rbErr)
		}
		return hookErr
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: parked\n", slug)
	return nil
}

// ideaUnparkCommand registers `specscore idea unpark <slug>`.
func ideaUnparkCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unpark <slug>",
		Short: "Return a parked Idea to active scheduling (clears the Parked axis)",
		Long: `Removes the **Parked:**, **Parked Reason:**, and **Parked Date:** header
lines from spec/ideas/<slug>.md, leaving **Status:** unchanged — nothing
is restored because nothing was taken away.

An Idea that carries no **Parked:** true axis has nothing to unpark; the
verb exits 4 (InvalidState) naming the idea rather than silently
succeeding, so a mistyped slug the caller believed was parked surfaces
immediately.

On success the verb runs ` + "`specscore spec lint --fix`" + `, prints
"<slug>: unparked", and exits 0.

Example:

  specscore idea unpark payment-fraud
`,
		Args: cobra.ExactArgs(1),
		RunE: runIdeaUnpark,
	}
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	return cmd
}

func runIdeaUnpark(cmd *cobra.Command, args []string) error {
	slug := args[0]
	if err := idea.ValidateSlug(slug); err != nil {
		return exitcode.InvalidArgsErrorf("invalid slug %q: %v", slug, err)
	}

	projectFlag, _ := cmd.Flags().GetString("project")
	specRoot, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}

	path := ideaFilePath(specRoot, slug)
	if _, statErr := statIdeaForParking(path); statErr != nil {
		if os.IsNotExist(statErr) {
			return exitcode.NotFoundErrorf("idea not found: %s", path)
		}
		return exitcode.UnexpectedErrorf("stat %s: %v", path, statErr)
	}

	orig, wrote, err := clearIdeaParkedFn(path)
	if err != nil {
		if errors.Is(err, lifecycle.ErrNotParked) {
			return exitcode.InvalidStateErrorf("%s is not parked; nothing to unpark", slug)
		}
		return exitcode.UnexpectedErrorf("unparking %s: %v", slug, err)
	}
	if !wrote {
		return exitcode.UnexpectedErrorf("unparking %s: no changes written", slug)
	}

	if hookErr := lintPostMutationHook(filepath.Join(specRoot, "spec"))(); hookErr != nil {
		if rbErr := lifecycle.RestoreBody(path, orig); rbErr != nil {
			return exitcode.UnexpectedErrorf("%v; rollback also failed: %v", hookErr, rbErr)
		}
		return hookErr
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: unparked\n", slug)
	return nil
}
