package telemetry

import (
	"context"
	"slices"
	"testing"
)

// TestUsageStatsChannel_Registered verifies that the usage-stats channel's
// init() ran and registered with the channel registry. Implements (part of)
// cli/telemetry/usage-telemetry#ac:usage-stats-channel-registered.
func TestUsageStatsChannel_Registered(t *testing.T) {
	registered := RegisteredChannels()
	if !slices.Contains(registered, ChannelUsageStats) {
		t.Errorf("expected %q in RegisteredChannels(), got %v", ChannelUsageStats, registered)
	}
}

// TestCrashReportsChannel_Registered verifies the symmetric registration
// for the errors channel. Implements (part of)
// cli/telemetry/errors-telemetry#ac:crash-reports-channel-registered.
func TestCrashReportsChannel_Registered(t *testing.T) {
	registered := RegisteredChannels()
	if !slices.Contains(registered, ChannelCrashReports) {
		t.Errorf("expected %q in RegisteredChannels(), got %v", ChannelCrashReports, registered)
	}
}

// TestTransmitErrors_NoDSNIsNoOp covers cli/telemetry/errors-telemetry#ac:
// sentry-dsn-empty-no-op. With an empty sentryDSN, the Sentry SDK isn't
// initialized and transmitErrors MUST return without panicking.
func TestTransmitErrors_NoDSNIsNoOp(t *testing.T) {
	if sentryDSN != "" {
		t.Skipf("test is meaningful only when sentryDSN is empty (dev build); got %q", sentryDSN)
	}
	if errorsClientInitialized {
		t.Fatalf("expected errorsClientInitialized=false with empty DSN")
	}
	// Should not panic.
	transmitErrors(context.Background(), Event{
		Command:    "feature.list",
		InstallID:  "test-install-id",
		CLIVersion: "0.0.0-test",
	})
}

// TestTransmitUsage_NoKeyIsNoOp covers cli/telemetry/usage-telemetry#ac:
// posthog-write-key-empty-no-op. With an empty posthogWriteKey, the
// usageClient is nil and transmitUsage MUST return without panicking and
// without doing any work.
//
// In dev builds, posthogWriteKey is naturally empty so this exercises the
// real code path. A direct integration test against a live PostHog endpoint
// would require an injected proxy and a real write key — that level of
// verification is deferred to the operational AC:posthog-funnel-defined
// release-checklist step.
func TestTransmitUsage_NoKeyIsNoOp(t *testing.T) {
	if posthogWriteKey != "" {
		t.Skipf("test is meaningful only when posthogWriteKey is empty (dev build); got %q", posthogWriteKey)
	}
	if usageClient != nil {
		t.Fatalf("expected usageClient to be nil with empty write key, got %v", usageClient)
	}
	// Should not panic and should return immediately.
	transmitUsage(context.Background(), Event{
		Command:    "feature.list",
		InstallID:  "test-install-id",
		CLIVersion: "0.0.0-test",
	})
}

// TestResolveCaller covers cli/telemetry/usage-telemetry#ac:
// caller-resolution-precedence + caller-enum-coercion.
func TestResolveCaller(t *testing.T) {
	cases := []struct {
		name string
		flag string
		env  string
		want string
	}{
		{"default-when-neither-set", "", "", CallerCLI},
		{"flag-only", "claude", "", "claude"},
		{"env-only", "", "codex", "codex"},
		{"flag-beats-env", "claude", "codex", "claude"},
		{"empty-flag-falls-through-to-env", "", "aider", "aider"},
		{"unknown-coerces-to-other-via-flag", "my-custom-tag", "", CallerOther},
		{"unknown-coerces-to-other-via-env", "", "rando-agent", CallerOther},
		{"pi-dev-recognized", "pi.dev", "", "pi.dev"},
		{"antigravity-google-recognized", "antigravity.google", "", "antigravity.google"},
		{"amazon-q-recognized", "amazon-q", "", "amazon-q"},
		{"deepseek-recognized", "deepseek", "", "deepseek"},
		{"deepseek-recognized-via-env", "", "deepseek", "deepseek"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ResolveCaller(tc.flag, tc.env)
			if got != tc.want {
				t.Errorf("ResolveCaller(%q, %q) = %q, want %q", tc.flag, tc.env, got, tc.want)
			}
		})
	}
}

// TestCallerEnumKnown_HasExpectedSize is a regression guard: the closed enum
// is documented in docs/telemetry.md as 21 values. If someone adds or removes
// a value without updating the doc table, this test fires.
func TestCallerEnumKnown_HasExpectedSize(t *testing.T) {
	const want = 21
	if got := len(CallerEnumKnown); got != want {
		t.Errorf("CallerEnumKnown has %d entries, expected %d; "+
			"if the change is intentional, update docs/telemetry.md table AND this test",
			got, want)
	}
}

// TestBuildPostHogCapture_EventNameAndAllProperties verifies the event name
// and the complete 10-key property set per
// cli/telemetry/usage-telemetry#ac:event-name-and-properties. Inspects the
// PostHog Capture message that would be sent — no live network, no mock
// proxy needed because the builder is a pure function.
func TestBuildPostHogCapture_EventNameAndAllProperties(t *testing.T) {
	event := Event{
		Command:    "feature.create",
		Success:    true,
		DurationMs: 123,
		ExitCode:   0,
		CLIVersion: "0.2.0",
		OS:         "darwin",
		Arch:       "arm64",
		Caller:     CallerClaude,
		InstallID:  "00000000-0000-4000-8000-000000000001",
		IsFirstRun: false,
	}
	capture := buildPostHogCapture(event)

	if capture.Event != usageStatsEventName {
		t.Errorf("event name = %q, want %q", capture.Event, usageStatsEventName)
	}
	if capture.DistinctId != event.InstallID {
		t.Errorf("DistinctId = %q, want %q (install_id)", capture.DistinctId, event.InstallID)
	}

	expectKeys := []string{
		"command", "success", "duration_ms", "exit_code",
		"cli_version", "os", "arch", "caller", "install_id", "is_first_run",
	}
	if len(capture.Properties) != len(expectKeys) {
		t.Errorf("properties has %d keys, want %d (the closed enum)",
			len(capture.Properties), len(expectKeys))
	}
	for _, k := range expectKeys {
		if _, ok := capture.Properties[k]; !ok {
			t.Errorf("property %q missing from Capture", k)
		}
	}
	// Spot-check key values.
	if capture.Properties["command"] != "feature.create" {
		t.Errorf("command = %v, want feature.create", capture.Properties["command"])
	}
	if capture.Properties["caller"] != CallerClaude {
		t.Errorf("caller = %v, want %s", capture.Properties["caller"], CallerClaude)
	}
	if capture.Properties["install_id"] != event.InstallID {
		t.Errorf("install_id property = %v, want %s", capture.Properties["install_id"], event.InstallID)
	}
}

// TestBuildPostHogCapture_CallerCoercion ensures the defensive double-coerce
// in the builder works even if Event.Caller was constructed with an
// out-of-enum value (bypassing ResolveCaller).
func TestBuildPostHogCapture_CallerCoercion(t *testing.T) {
	event := Event{Caller: "freshly-invented-agent-2027"}
	capture := buildPostHogCapture(event)
	if capture.Properties["caller"] != CallerOther {
		t.Errorf("out-of-enum caller should coerce to %q, got %v",
			CallerOther, capture.Properties["caller"])
	}
}
