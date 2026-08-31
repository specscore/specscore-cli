package cli

import (
	"fmt"
	"io"

	"github.com/specscore/specscore-cli/internal/telemetry"
	"github.com/spf13/cobra"
)

// debugCommand returns the `specscore debug` command group. Currently houses
// just one subcommand (`debug error`) for verifying the crash-reports
// channel; future verification utilities may attach here. The group is
// hidden from `specscore --help` per cli/telemetry/errors-telemetry#req:
// debug-error-subcommand — diagnostic surface, not user-facing.
func debugCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "debug",
		Short:  "Diagnostic utilities for verifying telemetry pipelines",
		Hidden: true,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(debugErrorCommand())
	return cmd
}

// debugErrorCommand implements `specscore debug error --text "<msg>" [--force]`.
// Synthesises a crash-reports event through the live Sentry pipeline so an
// operator can confirm wiring without crashing the binary. Honors the
// crash-reports opt-out by default; --force bypasses for a single
// invocation without modifying persistent state.
func debugErrorCommand() *cobra.Command {
	var (
		text  string
		force bool
	)
	cmd := &cobra.Command{
		Use:   "error",
		Short: "Send a synthesised crash-reports event for pipeline verification",
		Long: "Send a synthesised crash-reports event with the given --text " +
			"as a candidate SafePanic messageID. If the text is in the allowlist " +
			"the event ships verbatim; otherwise it coerces to 'unscrubbed panic'.\n\n" +
			"Without --force: honors the crash-reports opt-out. " +
			"With --force: bypasses opt-out for THIS invocation only (does not " +
			"modify persistent state).",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runDebugError(cmd.OutOrStdout(), text, force)
		},
	}
	cmd.Flags().StringVar(&text, "text", "", "candidate messageID (required)")
	cmd.Flags().BoolVar(&force, "force", false, "bypass crash-reports opt-out for this invocation (does not modify persistent state)")
	_ = cmd.MarkFlagRequired("text")
	return cmd
}

// runDebugError is the body of `specscore debug error`. Factored out for
// testability (caller injects the stdout writer and the two flag values).
//
// Behavior per cli/telemetry/errors-telemetry#req:debug-error-subcommand:
//   - --text is interpreted as a candidate SafePanic messageID, NOT as
//     free-form prose. Coercion to "unscrubbed panic" happens via the
//     scrubber for anything not in the allowlist.
//   - Without --force: consult the crash-reports opt-out. If disabled,
//     no-op with stdout pointing at `specscore telemetry enable
//     crash-reports`. Exit 0.
//   - With --force: bypass opt-out, print exactly what's about to be
//     sent (so the operator can audit), invoke the transmit-fn directly.
//     ~/.specscore/telemetry.yaml MUST be byte-identical before/after.
//   - Tagged debug=true so Sentry alert rules can filter these out.
func runDebugError(w io.Writer, text string, force bool) error {
	// Determine the effective opt-out for crash-reports.
	specZero, doNotTrack, ciDetected := telemetry.CollectOSEnvSignals(nil)
	stateResult, _ := telemetry.ReadState()
	sigs := telemetry.OptOutSignals{
		NoTelemetryFlag:        noTelemetryFlag,
		SpecScoreTelemetryZero: specZero,
		DoNotTrack:             doNotTrack,
		CIDetected:             ciDetected,
		PersistentState:        stateResult.State,
		PersistentStateInvalid: stateResult.InvalidReason != "",
	}
	known := telemetry.RegisteredChannels()
	decisions := telemetry.ResolveOptOut(sigs, known)
	d, ok := decisions[telemetry.ChannelCrashReports]
	disabled := ok && !d.Enabled

	if disabled && !force {
		_, _ = fmt.Fprintln(w,
			"crash-reports opt-out is in effect; no event sent. "+
				"Run `specscore telemetry enable crash-reports` to re-enable, "+
				"or use `--force` for a single bypass.")
		return nil
	}

	// Run through the scrubber so the operator sees exactly what would
	// ship — verbatim allowlisted message OR the "unscrubbed panic"
	// sentinel.
	resolved, isUnscrubbed := telemetry.ScrubMessage(telemetry.SafePanic(text, nil))
	scope := "would-be-sent"
	if !disabled || force {
		scope = "sent"
	}
	_, _ = fmt.Fprintf(w,
		"%s: message=%q debug=true unscrubbed=%v release=%s\n",
		scope, resolved, isUnscrubbed, buildInfo.Version)

	// Build a synthetic Event tagged as debug. The debug=true tag at the
	// emit-event level is what the operator-runbook alert filter excludes.
	telemetry.DebugCrashReports(text, buildInfo.Version)
	return nil
}
