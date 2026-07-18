package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"runtime"
	"runtime/debug"
	"strings"
	"time"

	"charm.land/fang/v2"
	"github.com/specscore/specscore-cli/internal/telemetry"
	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/spf13/cobra"
)

// runtimeState carries per-invocation telemetry state from PreRun to the
// post-Execute emission point in Run(). It is a module-level singleton
// because the CLI is a single-process invocation; concurrent goroutines
// don't read or write it.
type runtimeState struct {
	StartTime  time.Time
	InstallID  string
	IsFirstRun bool
	Decisions  map[telemetry.ChannelName]telemetry.ChannelDecision
	Signals    telemetry.OptOutSignals
	// CommandPath is the dot-separated cobra command path captured at
	// PreRun time. PostRun computes the same string but we capture it
	// in PreRun in case the command short-circuits.
	CommandPath string
	// Panic carries the recovered panic value + stack when fang.Execute
	// panicked. nil on normal command completion. Populated by
	// executeWithPanicRecovery and read by emitInvocationEvent so the
	// crash-reports channel's transmit-fn sees the panic context.
	Panic *telemetry.PanicInfo
}

// invocation is the singleton runtime state for the current process.
var invocation runtimeState

// Testable indirections for telemetry functions. Tests inject failures via these.
var (
	installIDFn = telemetry.InstallID
	statePathFn = telemetry.StatePath
)

// noTelemetryFlag is the persistent --no-telemetry boolean flag bound at the
// root command. cobra populates it before PersistentPreRun fires.
var noTelemetryFlag bool

// callerFlag is the persistent --caller string flag bound at the root
// command, per cli/telemetry/usage-telemetry#req:caller-flag. cobra
// populates it before PersistentPreRun fires. Empty means "flag not
// supplied"; precedence falls through to SPECSCORE_CALLER then to the
// default "cli" via telemetry.ResolveCaller.
var callerFlag string

// firstRunNoticeWriter is the destination for the first-run notice. Defaults
// to os.Stderr; tests can substitute.
var firstRunNoticeWriter io.Writer = os.Stderr

// firstRunNoticeFunc renders the three-line first-run notice text. Exposed as
// a package variable so a future copy-review can update the prose without
// touching the trigger logic. The shape, "SpecScore" capitalization, channel
// names, and literal command/env-var strings are normative per
// cli/telemetry#req:first-run-notice-content.
var firstRunNoticeFunc = defaultFirstRunNotice

func defaultFirstRunNotice() string {
	return "SpecScore collects 2 anonymous telemetry streams (EU-hosted):\n" +
		"  - usage-stats (PostHog product analytics)\n" +
		"  - crash-reports (Sentry)\n" +
		"Disable anytime: `specscore telemetry disable [channel-id]` " +
		"(omit channel or use `all` to disable all telemetry)\n"
}

// attachTelemetry installs the persistent --no-telemetry + --caller flags
// and the PersistentPreRun hook on the root cobra command. The post-Execute
// emission step is wired in Run() after fang.Execute returns, so the
// actual exit code is known.
func attachTelemetry(rootCmd *cobra.Command) {
	rootCmd.PersistentFlags().BoolVar(&noTelemetryFlag, "no-telemetry", false,
		"Disable telemetry for this invocation. See docs/telemetry.md.")
	rootCmd.PersistentFlags().StringVar(&callerFlag, "caller", "",
		"AI coding agent driving the CLI (e.g. claude, codex, aider). "+
			"See docs/telemetry.md for the full list. Default: cli (human at terminal).")

	rootCmd.PersistentPreRun = func(cmd *cobra.Command, _ []string) {
		preRun(cmd)
	}
}

// preRun is the hook body factored out so tests can drive it directly with a
// constructed cobra.Command rather than going through cobra.Execute.
func preRun(cmd *cobra.Command) {
	invocation.StartTime = time.Now()
	invocation.CommandPath = commandDotPath(cmd)

	// Collect opt-out signals.
	specZero, doNotTrack, ciDetected := telemetry.CollectOSEnvSignals(nil)
	stateResult, _ := telemetry.ReadState()
	invocation.Signals = telemetry.OptOutSignals{
		NoTelemetryFlag:        noTelemetryFlag,
		SpecScoreTelemetryZero: specZero,
		DoNotTrack:             doNotTrack,
		CIDetected:             ciDetected,
		PersistentState:        stateResult.State,
		PersistentStateInvalid: stateResult.InvalidReason != "",
	}
	// Stderr breadcrumb if state file was malformed.
	if stateResult.InvalidReason != "" {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(),
			"specscore: ignoring %s (%s); telemetry disabled for this invocation. "+
				"Run `specscore telemetry status` to inspect.\n",
			stateFilePathForMessage(), stateResult.InvalidReason)
	}

	// Resolve per-channel decisions. RegisteredChannels() is the registry
	// snapshot — at this point channels may or may not have registered
	// depending on what the binary imports.
	known := telemetry.RegisteredChannels()
	invocation.Decisions = telemetry.ResolveOptOut(invocation.Signals, known)

	// Install-id handling and first-run notice.
	id, justCreated, err := installIDFn()
	if err != nil {
		// Best-effort; telemetry already gated by opt-out. Leave InstallID
		// empty so emission paths can skip.
		invocation.InstallID = ""
		invocation.IsFirstRun = false
		return
	}
	invocation.InstallID = id
	invocation.IsFirstRun = justCreated

	if justCreated && !suppressFirstRunNotice(invocation.Signals) {
		_, _ = io.WriteString(firstRunNoticeWriter, firstRunNoticeFunc())
	}
}

// suppressFirstRunNotice returns true when any auto-disable signal (step 1-3
// of opt-out-signal-precedence) is present. CI suppression MUST still create
// the install_id (handled outside this function) so a later interactive run
// on the same machine does not re-trigger the notice.
func suppressFirstRunNotice(sigs telemetry.OptOutSignals) bool {
	return sigs.NoTelemetryFlag || sigs.SpecScoreTelemetryZero ||
		sigs.DoNotTrack || sigs.CIDetected
}

// stateFilePathForMessage returns a user-friendly path to telemetry.yaml for
// error messages; on any error resolving the path, returns the literal
// "~/.specscore/telemetry.yaml" as a fallback.
func stateFilePathForMessage() string {
	if p, err := statePathFn(); err == nil {
		return p
	}
	return "~/.specscore/telemetry.yaml"
}

// emitInvocationEvent builds the Event from the runtime state captured in
// PreRun plus the final result of fang.Execute, and dispatches via
// telemetry.Emit. Called from Run() after Execute returns.
//
// On any opt-out path (every channel disabled), the registry traversal in
// Emit still happens — channels are expected to honor their own decision via
// the OptOutSignals carried implicitly through the registry state. But we
// also short-circuit here: if NO channel is enabled, don't even build the
// Event.
func emitInvocationEvent(runErr error) {
	if invocation.StartTime.IsZero() {
		return // PreRun never fired (e.g., flag parsing failed before lifecycle).
	}
	if !anyChannelEnabled() {
		return
	}
	if invocation.InstallID == "" {
		return // InstallID failed; nothing safe to tag events with.
	}
	exit := exitCodeFromError(runErr)
	event := telemetry.Event{
		Command:    invocation.CommandPath,
		Success:    runErr == nil,
		DurationMs: time.Since(invocation.StartTime).Milliseconds(),
		ExitCode:   exit,
		CLIVersion: version,
		OS:         runtime.GOOS,
		Arch:       runtime.GOARCH,
		Caller:     telemetry.ResolveCaller(callerFlag, os.Getenv("SPECSCORE_CALLER")),
		InstallID:  invocation.InstallID,
		IsFirstRun: invocation.IsFirstRun,
		Panic:      invocation.Panic,
	}
	telemetry.Emit(rootContext(), event)
}

// silentSignalErrorHandler renders command errors like fang's default, EXCEPT
// for a "pure exit-code signal" — an exitcode.Error whose message is empty. Some
// commands (e.g. `self-update --check`, which exits 10 to report "update
// available" per cli/self-update#req:check-exit-codes) print their human-readable
// status to stdout themselves and return an empty-message error only to carry the
// exit code. fang's default handler would render that empty message as a lone
// styled "." box; suppress it so the exit code is delivered without spurious
// output. Everything with a real message renders normally.
func silentSignalErrorHandler(w io.Writer, styles fang.Styles, err error) {
	var ec *exitcode.Error
	if errors.As(err, &ec) && strings.TrimSpace(ec.Error()) == "" {
		return // exit code only — nothing to render
	}
	fang.DefaultErrorHandler(w, styles, err)
}

// executeWithPanicRecovery wraps fang.Execute with a panic-recovery
// boundary. Captures any panic into invocation.Panic for the crash-reports
// channel's transmitErrors to consume, emits the telemetry event, prints
// the panic + stack to stderr so the user sees their crash, and returns
// an exitcode.Unexpected (code 10) error so the regular Fatal path exits
// with the documented unexpected-error code rather than Go's default
// panic-exit code 2.
//
// Implements cli/telemetry/errors-telemetry#req:trigger-on-panic-recovery.
func executeWithPanicRecovery(rootCmd *cobra.Command) (returnErr error) {
	var (
		panicVal   any
		panicStack []byte
	)
	func() {
		defer func() {
			if r := recover(); r != nil {
				panicVal = r
				panicStack = debug.Stack()
			}
		}()
		returnErr = fang.Execute(context.Background(), rootCmd,
			fang.WithoutVersion(),
			fang.WithErrorHandler(silentSignalErrorHandler),
		)
	}()

	if panicVal != nil {
		// User-visible: show the crash to stderr so debugging is possible.
		// The panic value MAY include user-authored content (e.g. file
		// paths from fmt.Errorf) — that content goes to the user's
		// terminal, not to Sentry. The scrubber in transmitErrors decides
		// what (if anything) goes over the network.
		_, _ = fmt.Fprintf(os.Stderr, "panic: %v\n\n%s\n", panicVal, panicStack)
		// Telemetry: capture panic context for the crash-reports channel.
		invocation.Panic = &telemetry.PanicInfo{Value: panicVal, Stack: panicStack}
		emitInvocationEvent(exitcode.UnexpectedErrorf("panic: %v", panicVal))
		return exitcode.UnexpectedErrorf("panic recovered: %v", panicVal)
	}

	returnErr = mapUnsupportedCommand(returnErr)
	emitInvocationEvent(returnErr)
	return returnErr
}

// rootContext returns a fresh background context used by telemetry emission
// after fang.Execute has returned. The cobra command's context is no longer
// available at this point, so we create a new short-lived one bounded by the
// 500 ms per-channel timeout inside Emit.
func rootContext() context.Context {
	return context.Background()
}

// anyChannelEnabled returns true when at least one channel's decision was
// Enabled=true. Used to skip Event construction entirely when everything is
// opted out.
func anyChannelEnabled() bool {
	for _, d := range invocation.Decisions {
		if d.Enabled {
			return true
		}
	}
	return false
}

// commandDotPath returns the cobra command path in dot-separated form per
// cli/telemetry/usage-telemetry#req:usage-stats-event-properties.
// e.g. for `specscore feature create` returns "feature.create"; for the bare
// `specscore` returns the empty string.
func commandDotPath(cmd *cobra.Command) string {
	if cmd == nil {
		return ""
	}
	parts := []string{}
	for c := cmd; c != nil && c.HasParent(); c = c.Parent() {
		parts = append([]string{c.Name()}, parts...)
	}
	return strings.Join(parts, ".")
}

// exitCodeFromError extracts the exit code an error implies. mirrors the
// logic in Fatal() so the Event's ExitCode matches what os.Exit will use.
func exitCodeFromError(err error) int {
	if err == nil {
		return 0
	}
	type exitCoder interface{ ExitCode() int }
	var ec exitCoder
	if errors.As(err, &ec) {
		return ec.ExitCode()
	}
	return 1
}
