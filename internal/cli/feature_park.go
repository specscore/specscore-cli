package cli

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/feature"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
	"github.com/spf13/cobra"
)

// featureParkCommand registers `specscore feature park <feature_id> --reason`.
//
// Parked is an axis orthogonal to **Status:** — see pkg/lifecycle/parked.go
// for the full contract this verb and `feature unpark` implement. It is NOT
// a lifecycle transition: parking never touches **Status:** and is not
// gated on the feature's current status (a Draft can be parked, so can an
// Approved).
func featureParkCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "park <feature_id> --reason \"...\"",
		Short: "Defer a Feature without changing its Status (sets the orthogonal Parked axis)",
		Long: `Marks spec/features/<feature_id>/README.md with **Parked:** true, a
**Parked Reason:** (from the required --reason flag), and a **Parked
Date:** (today, UTC), WITHOUT touching **Status:**. Parking is a
scheduling decision ("not this release"), independent of maturity
("how ready is it?") — a fully-specced, ratified Feature can be parked
just as easily as a Draft.

--reason is REQUIRED: a bare park with no explanation is rejected before
any mutation (exit 2), so the parked axis never rots into an unaudited
graveyard. Re-running park on an already-parked Feature overwrites the
reason and resets the date to today — the way to confirm "still
deliberately deferred" is to re-park with a fresh --reason.

On success the verb runs ` + "`specscore spec lint --fix`" + `, prints
"<feature_id>: parked", and exits 0. If anything fails after the write
(lint failure, I/O error), the header is restored to its pre-invocation
form before the verb exits.

Examples:

  specscore feature park cli/some-subfeature --reason "good idea, not v1"
`,
		Args: cobra.ExactArgs(1),
		RunE: runFeaturePark,
	}
	cmd.Flags().String("reason", "", "required: why this Feature is being parked")
	_ = cmd.MarkFlagRequired("reason")
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	return cmd
}

func runFeaturePark(cmd *cobra.Command, args []string) error {
	featureID := args[0]
	reason, _ := cmd.Flags().GetString("reason")
	projectFlag, _ := cmd.Flags().GetString("project")

	featuresDir, err := resolveFeaturesDir(projectFlag)
	if err != nil {
		return err
	}
	if !feature.Exists(featuresDir, featureID) {
		return exitcode.NotFoundErrorf("feature not found: %s (expected README at %s)",
			featureID, feature.ReadmePath(featuresDir, featureID))
	}
	readmePath := feature.ReadmePath(featuresDir, featureID)

	orig, wrote, err := lifecycle.SetParked(readmePath, reason)
	if err != nil {
		if errors.Is(err, lifecycle.ErrParkReasonRequired) {
			return exitcode.InvalidArgsError("park requires --reason (a non-empty reason for the deferral)")
		}
		if errors.Is(err, lifecycle.ErrStatusLineNotFound) {
			return exitcode.UnexpectedErrorf("%s has no **Status:** line to anchor the Parked axis", readmePath)
		}
		return exitcode.UnexpectedErrorf("parking %s: %v", featureID, err)
	}
	if !wrote {
		return exitcode.UnexpectedErrorf("parking %s: no changes written", featureID)
	}

	specRoot := filepath.Dir(featuresDir) // spec/features -> spec
	if hookErr := lintPostMutationHook(specRoot)(); hookErr != nil {
		if rbErr := lifecycle.RestoreBody(readmePath, orig); rbErr != nil {
			return exitcode.UnexpectedErrorf("%v; rollback also failed: %v", hookErr, rbErr)
		}
		return hookErr
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: parked\n", featureID)
	return nil
}

// featureUnparkCommand registers `specscore feature unpark <feature_id>`.
func featureUnparkCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "unpark <feature_id>",
		Short: "Return a parked Feature to active scheduling (clears the Parked axis)",
		Long: `Removes the **Parked:**, **Parked Reason:**, and **Parked Date:** header
lines from spec/features/<feature_id>/README.md, leaving **Status:**
unchanged — nothing is restored because nothing was taken away.

A Feature that carries no **Parked:** true axis has nothing to unpark;
the verb exits 4 (InvalidState) naming the feature rather than silently
succeeding, so a mistyped feature_id the caller believed was parked
surfaces immediately.

On success the verb runs ` + "`specscore spec lint --fix`" + `, prints
"<feature_id>: unparked", and exits 0.

Example:

  specscore feature unpark cli/some-subfeature
`,
		Args: cobra.ExactArgs(1),
		RunE: runFeatureUnpark,
	}
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	return cmd
}

func runFeatureUnpark(cmd *cobra.Command, args []string) error {
	featureID := args[0]
	projectFlag, _ := cmd.Flags().GetString("project")

	featuresDir, err := resolveFeaturesDir(projectFlag)
	if err != nil {
		return err
	}
	if !feature.Exists(featuresDir, featureID) {
		return exitcode.NotFoundErrorf("feature not found: %s (expected README at %s)",
			featureID, feature.ReadmePath(featuresDir, featureID))
	}
	readmePath := feature.ReadmePath(featuresDir, featureID)

	orig, wrote, err := lifecycle.ClearParked(readmePath)
	if err != nil {
		if errors.Is(err, lifecycle.ErrNotParked) {
			return exitcode.InvalidStateErrorf("%s is not parked; nothing to unpark", featureID)
		}
		return exitcode.UnexpectedErrorf("unparking %s: %v", featureID, err)
	}
	if !wrote {
		return exitcode.UnexpectedErrorf("unparking %s: no changes written", featureID)
	}

	specRoot := filepath.Dir(featuresDir)
	if hookErr := lintPostMutationHook(specRoot)(); hookErr != nil {
		if rbErr := lifecycle.RestoreBody(readmePath, orig); rbErr != nil {
			return exitcode.UnexpectedErrorf("%v; rollback also failed: %v", hookErr, rbErr)
		}
		return hookErr
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: unparked\n", featureID)
	return nil
}
