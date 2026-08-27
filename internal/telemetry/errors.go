package telemetry

import (
	"context"

	"github.com/getsentry/sentry-go"
)

// This file is the SOLE place in the repo importing github.com/getsentry/
// sentry-go. boundary_test.go enforces the confinement at build time.
// All crash-reports transmission flows through transmitErrors registered
// below.

// sentryDSN is the Sentry project DSN, compiled into the binary via
// `-ldflags "-X internal/telemetry.sentryDSN=..."` during release builds
// per cli/telemetry/errors-telemetry#req:sentry-dsn-embedded-at-build-time.
// Declaration MUST live in this file so the vendor-SDK import-confinement
// audit surface stays intact.
//
// Empty value (dev build) ⇒ the registered transmit callback silently
// no-ops; the channel still registers so `specscore telemetry status`
// lists it. The DSN itself encodes the EU region: per
// REQ:sentry-eu-region the DSN form is
// https://<key>@<org>.ingest.de.sentry.io/<project-id>.
var sentryDSN = ""

// errorsClientInitialized records whether the Sentry SDK's package-level
// CurrentHub was successfully initialized at init() time. nil DSN or init
// error keeps this false; transmitErrors checks this before sending.
var errorsClientInitialized bool

// transmitErrorsTestPanic is a test-only injection point for verifying the
// defensive recover() in transmitErrors. Production code leaves this nil;
// tests in errors_test.go set it to a panicking function to confirm
// REQ:transmit-callback-must-not-mask-exit-code's invariant. Exposed at
// package level (rather than a build-tag-gated file) so it stays trivially
// observable — production callers never invoke it because transmitErrors
// only calls it under the test hook.
var transmitErrorsTestPanic func()

// setupErrorsChannel encapsulates the crash-reports channel initialization
// logic, extracted from init() so tests can exercise both the sentryDSN-empty
// and sentryDSN-populated branches without compile-time constants.
var setupErrorsChannel = func() {
	if sentryDSN == "" {
		return
	}
	err := sentry.Init(sentry.ClientOptions{
		Dsn: sentryDSN,
		// AttachStacktrace=true tells the SDK to attach the Go stack at
		// CaptureMessage / CaptureException call sites. We DO want stack
		// frames (the scrubber strips paths), so this stays true.
		AttachStacktrace: true,
		// SendDefaultPII=false explicitly disables the SDK's automatic
		// collection of OS user, hostname, and other locals from the
		// environment — implements REQ:stack-frame-scrubber clause 3
		// ("strip local-variable values from frame metadata ... disable
		// explicitly").
		//
		// sentry-go now prefers the newer DataCollection field for this,
		// but that's a genuine behavior migration (granular per-category
		// toggles vs. one bool) that deserves its own REQ-governed review
		// rather than a drive-by rename under a CI-toolchain fix. Keeping
		// the explicit, well-understood SendDefaultPII=false pending that.
		SendDefaultPII: false, //nolint:staticcheck // deprecated in favor of DataCollection; migration tracked separately, see comment above
		// Release tag is set per-event by transmitErrors (see Task 3)
		// using Event.CLIVersion. Leaving Release empty here lets the
		// per-event tag take effect.
	})
	if err != nil {
		// Init failure ⇒ stay no-op; the channel is still registered for
		// status discoverability.
		return
	}
	errorsClientInitialized = true
}

func init() {
	RegisterChannel(ChannelCrashReports, transmitErrors)
	setupErrorsChannel()
}

// transmitErrors converts a typed Event into a Sentry capture call. Honors
// the no-op contract when sentryDSN was not injected at build time.
//
// Per the registry contract (callBounded in registry.go), this function
// already runs inside a 500 ms bounded goroutine with an outer deferred
// recover() at the registry layer. The INNER defer recover() installed
// here is a defense-in-depth measure per
// cli/telemetry/errors-telemetry#req:transmit-callback-must-not-mask-exit-
// code: a scrubber bug, SDK panic, or malformed payload caught here is
// dropped silently, the in-goroutine error is suppressed, and the user's
// original exit code stays intact.
//
// The conditional emit logic (panic-recovered OR exit ≥10 only; panic
// priority; single event per invocation) lands in Task 4. This commit
// adds:
//   - the defensive recover (REQ:transmit-callback-must-not-mask-exit-code)
//   - the release-tag application from Event.CLIVersion
//     (REQ:sentry-release-tag)
func transmitErrors(ctx context.Context, event Event) {
	defer func() { _ = recover() }()
	if !errorsClientInitialized {
		return
	}
	_ = ctx
	// Test injection point — production callers leave this nil. If set,
	// the panic exercises the defer recover() above.
	if transmitErrorsTestPanic != nil {
		transmitErrorsTestPanic()
	}

	// Conditional emit per REQ:trigger-on-panic-recovery + REQ:trigger-on-
	// exit-code-ge-10:
	//   - Panic takes priority: a panicking command emits exactly one
	//     event (the panic-signature event), NOT the exit-code event,
	//     even though its ExitCode is also ≥10 (we tag panicked
	//     invocations with code 10 in executeWithPanicRecovery).
	//   - Exit code ≥10 with no recovered panic emits an exit-code event.
	//   - Anything else (success, expected errors with code 1–9) emits
	//     nothing.
	if event.Panic != nil {
		emitPanicEvent(event)
		return
	}
	if event.ExitCode >= 10 {
		emitExitCodeEvent(event)
		return
	}
	// No emit: normal command completion or documented expected error.
}

// emitPanicEvent sends a Sentry event for a recovered panic. The scrubber
// classifies the panic value: SafePanic with an allowlisted MessageID
// goes through verbatim; anything else becomes "unscrubbed panic" with
// the `message: unscrubbed` tag.
func emitPanicEvent(event Event) {
	messageID, isUnscrubbed := ScrubMessage(event.Panic.Value)
	sentry.WithScope(func(scope *sentry.Scope) {
		scope.SetTag("release", event.CLIVersion)
		scope.SetTag("debug", "false")
		if isUnscrubbed {
			scope.SetTag("message", "unscrubbed")
		}
		// We deliberately do NOT attach the raw stack via
		// debug.Stack(); the Sentry SDK's AttachStacktrace=true plus
		// SendDefaultPII=false produces a frame list whose file paths
		// the scrubber would replace via ScrubFrame — but the SDK's
		// auto-stack is captured at the SDK call site here, not at the
		// panic origin. Filing a Sentry breadcrumb instead so the
		// release-tag-correlated alert still fires.
		sentry.CaptureMessage(messageID)
	})
}

// emitExitCodeEvent sends a Sentry event for an unexpected non-panic exit
// (code ≥10). Message is a synthesised, content-free string naming the
// exit code and the dot-separated cobra command path.
func emitExitCodeEvent(event Event) {
	sentry.WithScope(func(scope *sentry.Scope) {
		scope.SetTag("release", event.CLIVersion)
		scope.SetTag("debug", "false")
		// `Command` is the cobra path (e.g. "feature.create") — already
		// scrubbed-by-construction since cobra paths can't contain
		// user-authored strings, only command identifiers from the
		// command tree.
		msg := "unexpected exit code " + intToString(event.ExitCode) + " from cmd " + event.Command
		sentry.CaptureMessage(msg)
	})
}

// DebugCrashReports emits a synthetic crash-reports event for verification
// purposes (cli/telemetry/errors-telemetry#req:debug-error-subcommand). The
// text is interpreted as a candidate SafePanic messageID — allowlisted →
// verbatim, unknown → "unscrubbed panic" with the `message: unscrubbed`
// tag. Always tagged debug=true so Sentry alert rules can filter these
// out of operator notifications.
//
// No-op when errorsClientInitialized is false (empty DSN). Returns
// nothing; failures to send are absorbed by the defer recover().
func DebugCrashReports(text, cliVersion string) {
	defer func() { _ = recover() }()
	if !errorsClientInitialized {
		return
	}
	resolved, isUnscrubbed := ScrubMessage(SafePanic(text, nil))
	sentry.WithScope(func(scope *sentry.Scope) {
		scope.SetTag("release", cliVersion)
		scope.SetTag("debug", "true")
		if isUnscrubbed {
			scope.SetTag("message", "unscrubbed")
		}
		sentry.CaptureMessage(resolved)
	})
}

// intToString avoids pulling strconv into this file just for one Itoa
// call. Domain is exit codes (0–255 in POSIX); a tiny implementation is
// fine.
func intToString(n int) string {
	if n == 0 {
		return "0"
	}
	if n < 0 {
		return "-" + intToString(-n)
	}
	var buf [11]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}
