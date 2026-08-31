// Package feature — lifecycle-transition primitives.
//
// This file hosts the kind-specific glue between the generic
// pkg/lifecycle state machine and the Feature artifact layout. It
// keeps three responsibilities:
//
//  1. Resolve a `<feature_id>` (possibly slash-bearing, e.g.
//     "cli/idea/change-status") to its canonical README path.
//  2. Validate the requested transition against the Feature kind's
//     legal-transition matrix, falling through to an exit-2 error
//     when the raw --to value isn't a recognized status name.
//  3. Rewrite the artifact's **Status:** line in place and return a
//     Restore closure the CLI layer can call to roll back on lint
//     failure.
//
// pkg/feature/transitions.go DELIBERATELY does NOT import pkg/lint —
// the lint dance (run --fix, inspect violations, roll back on error)
// belongs to the CLI handler in internal/cli/feature.go. Keeping the
// import direction strict (lint → feature, never the reverse) avoids
// the cycle that the parallel pkg/idea/transitions.go work has hit.
package feature

import (
	"errors"
	"os"
	"path/filepath"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

// ChangeStatusResult is the outcome of a successful Status rewrite.
// The Restore closure restores the artifact's original `**Status:**`
// line byte-for-byte (forwarding to lifecycle.Rollback under the
// hood). The CLI layer holds the closure across its post-rewrite
// `spec lint --fix` pass and invokes it iff a lint violation forces
// rollback (per the lifecycle-transitions Meta REQ:
// rollback-on-lint-failure).
type ChangeStatusResult struct {
	FeatureID  string
	ReadmePath string
	From       lifecycle.Status
	To         lifecycle.Status
	Restore    func() error
}

// ChangeStatus validates and applies a Feature Status transition.
//
// Flow:
//
//  1. ParseStatus(toRaw) — exit-2 if the raw flag value is not a
//     recognized Feature status (covers `--to=banana`, `--to=draft`
//     (no arc INTO Draft), `--to=archived` (Idea-only)).
//  2. resolveFeatureID(featuresDir, featureID) — exit-3 if the
//     `<feature_id>/README.md` path doesn't exist.
//  3. lifecycle.Validate(...) — reads the artifact's current Status
//     and exit-4s if (from, to) isn't in the Feature matrix.
//  4. lifecycle.Rewrite(...) — line-targeted rewrite; returns the
//     original Status line for rollback.
//
// On any pre-rewrite failure, ChangeStatus returns an *exitcode.Error
// carrying the correct exit code and an empty result. On success it
// returns the result with a Restore closure the caller MUST invoke if
// post-rewrite work fails.
func ChangeStatus(featuresDir, featureID, toRaw string) (*ChangeStatusResult, error) {
	to, ok := lifecycle.ParseStatus(lifecycle.KindFeature, toRaw)
	if !ok {
		return nil, exitcode.InvalidArgsErrorf(
			"unrecognized status %q for kind feature (legal values: %s)",
			toRaw, joinStatuses(lifecycle.LegalStatuses(lifecycle.KindFeature)),
		)
	}
	// `draft` is recognized as a kind status but is NOT a legal `--to`
	// target — no arc goes INTO Draft. Reject it at the flag layer so
	// `--to=draft` exits 2, not 4. Same shape as `--to=banana`.
	if to == lifecycle.FeatureDraft {
		return nil, exitcode.InvalidArgsErrorf(
			"unrecognized status %q for kind feature (Draft is not a legal target: no transition INTO Draft)",
			toRaw,
		)
	}
	// `planned` is recognized as a kind status (it's a legal FROM-state, see
	// FeaturePlanned's doc comment) but is NOT a legal `--to` target — no arc
	// goes INTO Planned, mirroring the Draft guard above. Reject it at the
	// flag layer so `--to=planned` exits 2, not 4.
	if to == lifecycle.FeaturePlanned {
		return nil, exitcode.InvalidArgsErrorf(
			"unrecognized status %q for kind feature (Planned is not a legal target: no transition INTO Planned)",
			toRaw,
		)
	}

	readmePath, err := resolveFeatureID(featuresDir, featureID)
	if err != nil {
		return nil, err
	}

	from, err := lifecycle.Validate(lifecycle.KindFeature, readmePath, to)
	if err != nil {
		var ite *lifecycle.InvalidTransitionError
		if errors.As(err, &ite) {
			return nil, exitcode.InvalidStateError(ite.Error())
		}
		// Any other Validate error (missing **Status:** line, I/O,
		// etc.) is a runtime failure.
		return nil, exitcode.UnexpectedErrorf("validating transition: %v", err)
	}

	originalLine, err := lifecycle.Rewrite(readmePath, to)
	if err != nil {
		return nil, exitcode.UnexpectedErrorf("rewriting status line: %v", err)
	}

	restore := func() error {
		return lifecycle.Rollback(readmePath, originalLine)
	}

	return &ChangeStatusResult{
		FeatureID:  featureID,
		ReadmePath: readmePath,
		From:       from,
		To:         to,
		Restore:    restore,
	}, nil
}

// resolveFeatureID maps a `<feature_id>` (possibly slash-bearing) to
// the absolute path of its README.md within featuresDir. Mirrors the
// existing feature.ReadmePath + feature.Exists pattern used by
// `feature info`, but emits a typed NotFound exitcode error so the CLI
// can map it to exit-3.
//
// The function does not enforce identifier syntax beyond "non-empty
// and resolves to a file" — slug character rules are enforced by
// `feature new`, not by read/mutate verbs (precedent: `feature info`).
func resolveFeatureID(featuresDir, featureID string) (string, error) {
	if featureID == "" {
		return "", exitcode.InvalidArgsError("missing feature_id")
	}
	readmePath := filepath.Join(featuresDir, filepath.FromSlash(featureID), "README.md")
	if _, err := os.Stat(readmePath); err != nil {
		return "", exitcode.NotFoundErrorf("feature not found: %s (expected README at %s)", featureID, readmePath)
	}
	return readmePath, nil
}

// joinStatuses renders a Status slice as a comma-separated lower-case
// list for stderr messages (e.g.,
// "approved, deprecated, draft, implementing, stable, under review").
// Lower-case matches the documented flag-value style (case-insensitive
// match, lower-case in --help and error messages).
func joinStatuses(ss []lifecycle.Status) string {
	if len(ss) == 0 {
		return ""
	}
	out := ""
	for i, s := range ss {
		if i > 0 {
			out += ", "
		}
		out += string(s)
	}
	return out
}
