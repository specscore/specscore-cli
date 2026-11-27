package cli

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// --- Fatal ---

func TestFatal_NilIsNoop(t *testing.T) {
	var called bool
	old := osExit
	osExit = func(code int) { called = true }
	t.Cleanup(func() { osExit = old })

	Fatal(nil)
	if called {
		t.Error("Fatal(nil) should not call osExit")
	}
}

func TestFatal_TypedExitCode(t *testing.T) {
	var gotCode int
	old := osExit
	osExit = func(code int) { gotCode = code }
	t.Cleanup(func() { osExit = old })

	// Redirect stderr to capture the error message.
	origStderr := os.Stderr
	r, w, _ := os.Pipe()
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = origStderr })

	Fatal(exitcode.NotFoundErrorf("missing thing"))
	_ = w.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(r)

	if gotCode != exitcode.NotFound {
		t.Errorf("exit code = %d, want %d (NotFound)", gotCode, exitcode.NotFound)
	}
	if !strings.Contains(buf.String(), "missing thing") {
		t.Errorf("stderr = %q, want to contain 'missing thing'", buf.String())
	}
}

func TestFatal_GenericError(t *testing.T) {
	var gotCode int
	old := osExit
	osExit = func(code int) { gotCode = code }
	t.Cleanup(func() { osExit = old })

	origStderr := os.Stderr
	_, w, _ := os.Pipe()
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = origStderr; _ = w.Close() })

	Fatal(exitcode.InvalidArgsErrorf("bad args"))
	if gotCode != exitcode.InvalidArgs {
		t.Errorf("exit code = %d, want %d (InvalidArgs)", gotCode, exitcode.InvalidArgs)
	}
}

func TestFatal_PlainError(t *testing.T) {
	var gotCode int
	old := osExit
	osExit = func(code int) { gotCode = code }
	t.Cleanup(func() { osExit = old })

	origStderr := os.Stderr
	_, w, _ := os.Pipe()
	os.Stderr = w
	t.Cleanup(func() { os.Stderr = origStderr; _ = w.Close() })

	Fatal(os.ErrNotExist)
	if gotCode != 1 {
		t.Errorf("exit code = %d, want 1 for plain error", gotCode)
	}
}

// --- Run ---

func TestRun_HelpOutput(t *testing.T) {
	// Run with no args should print help and return nil.
	err := Run([]string{"specscore"})
	if err != nil {
		t.Errorf("Run([specscore]) = %v, want nil", err)
	}
}

func TestRun_VersionFlag(t *testing.T) {
	err := Run([]string{"specscore", "--version"})
	if err != nil {
		t.Errorf("Run([--version]) = %v, want nil", err)
	}
}

func TestRun_VersionSubcommand(t *testing.T) {
	err := Run([]string{"specscore", "version"})
	if err != nil {
		t.Errorf("Run([version]) = %v, want nil", err)
	}
}

func TestMapUnsupportedCommand(t *testing.T) {
	tests := []struct {
		name     string
		in       error
		wantCode int // -1 means: expect no exit code attached
	}{
		{"nil", nil, -1},
		{"unknown command", errors.New(`unknown command "verdict" for "specscore consilium"`), exitcode.UnsupportedCommand},
		{"unknown flag stays generic", errors.New("unknown flag: --bogus"), -1},
		{"already coded error not clobbered", exitcode.NotFoundError(`unknown command "x" for "y"`), exitcode.NotFound},
		{"unrelated error untouched", errors.New("something else failed"), -1},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := mapUnsupportedCommand(tc.in)
			if tc.in == nil {
				if got != nil {
					t.Fatalf("mapUnsupportedCommand(nil) = %v, want nil", got)
				}
				return
			}
			type exitCoder interface{ ExitCode() int }
			var ec exitCoder
			if tc.wantCode == -1 {
				if errors.As(got, &ec) {
					t.Errorf("expected no exit code, got %d", ec.ExitCode())
				}
				return
			}
			if !errors.As(got, &ec) {
				t.Fatalf("expected exit code %d, got none (%v)", tc.wantCode, got)
			}
			if ec.ExitCode() != tc.wantCode {
				t.Errorf("exit code = %d, want %d", ec.ExitCode(), tc.wantCode)
			}
		})
	}
}

// TestMapInvalidArgs_PinsCobraMessages drives Cobra's real argument
// validators and required-flag check, so the mapping is tested against the
// exact errors Cobra produces rather than hand-copied strings: a Cobra upgrade
// that rewords one of them fails here.
func TestMapInvalidArgs_PinsCobraMessages(t *testing.T) {
	two := []string{"a", "b"}
	three := []string{"a", "b", "c"}
	required := func() error {
		c := &cobra.Command{Use: "x", RunE: func(*cobra.Command, []string) error { return nil }}
		c.Flags().String("to", "", "")
		_ = c.MarkFlagRequired("to")
		if err := c.ParseFlags(nil); err != nil {
			t.Fatalf("ParseFlags: %v", err)
		}
		return c.ValidateRequiredFlags()
	}
	cmd := &cobra.Command{Use: "x"}
	cobraErrors := map[string]error{
		"ExactArgs too few":    cobra.ExactArgs(1)(cmd, nil),
		"ExactArgs too many":   cobra.ExactArgs(1)(cmd, two),
		"MinimumNArgs":         cobra.MinimumNArgs(1)(cmd, nil),
		"MaximumNArgs":         cobra.MaximumNArgs(1)(cmd, two),
		"RangeArgs":            cobra.RangeArgs(0, 2)(cmd, three),
		"MarkFlagRequired":     required(),
		"ExactArgs(2) too few": cobra.ExactArgs(2)(cmd, []string{"a"}),
	}
	for name, in := range cobraErrors {
		t.Run(name, func(t *testing.T) {
			if in == nil {
				t.Fatal("cobra validator unexpectedly accepted the input")
			}
			got := mapInvalidArgs(in)
			var ec interface{ ExitCode() int }
			if !errors.As(got, &ec) || ec.ExitCode() != exitcode.InvalidArgs {
				t.Fatalf("mapInvalidArgs(%q) = %v, want exit %d", in, got, exitcode.InvalidArgs)
			}
			if got.Error() != in.Error() {
				t.Errorf("message changed: got %q, want %q", got.Error(), in.Error())
			}
			if !errors.Is(got, in) {
				t.Errorf("mapped error lost the Cobra cause %v", in)
			}
		})
	}
}

func TestMapInvalidArgs_LeavesOtherErrors(t *testing.T) {
	if got := mapInvalidArgs(nil); got != nil {
		t.Fatalf("mapInvalidArgs(nil) = %v, want nil", got)
	}
	coded := exitcode.NotFoundError("accepts 1 arg(s), received 0")
	if got := mapInvalidArgs(coded); got != coded {
		t.Errorf("already-coded error was replaced: %v", got)
	}
	for _, in := range []error{
		errors.New(`unknown command "x" for "specscore idea list"`), // NoArgs: stays with mapUnsupportedCommand
		errors.New("unknown flag: --bogus"),
		errors.New("lesson x accepts 1 arg(s), received 0"), // not anchored at the start
		errors.New("something else failed"),
	} {
		if got := mapInvalidArgs(in); got != in {
			t.Errorf("mapInvalidArgs(%q) = %v, want it unchanged", in, got)
		}
	}
}

// runCLIExit runs the real CLI entry point (Run, then Fatal) the way
// cmd/specscore does and returns the process exit code Fatal chose plus
// everything written to stderr.
func runCLIExit(t *testing.T, args ...string) (int, string) {
	t.Helper()
	t.Setenv("DO_NOT_TRACK", "1")
	t.Setenv("SPECSCORE_TELEMETRY", "off")
	code := 0
	old := osExit
	osExit = func(c int) { code = c }
	t.Cleanup(func() { osExit = old })
	stderr := captureStderr(t, func() {
		Fatal(Run(append([]string{"specscore"}, args...)))
	})
	return code, stderr
}

// specscore:verifies feature/cli#ac:usage-errors-exit-2 — one case per Cobra validator kind and a required flag, on verbs other than idea change-status
func TestRun_UsageErrorsExit2(t *testing.T) {
	tests := []struct {
		name       string
		args       []string
		wantStderr string
	}{
		{"ExactArgs(2) too few", []string{"lesson", "relation", "add", "a"}, "accepts 2 arg(s), received 1"},
		{"ExactArgs(1) too many", []string{"feature", "info", "a", "b"}, "accepts 1 arg(s), received 2"},
		{"MinimumNArgs", []string{"event", "merge"}, "requires at least 1 arg(s), only received 0"},
		{"MaximumNArgs", []string{"telemetry", "enable", "a", "b"}, "accepts at most 1 arg(s), received 2"},
		{"RangeArgs", []string{"studio", "resolve", "a", "b", "c"}, "accepts between 0 and 2 arg(s), received 3"},
		{"MarkFlagRequired", []string{"issue", "change-status", "foo"}, `required flag(s) "to" not set`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := setupSpecRoot(t)
			withCwd(t, root)
			before := snapshotDir(t, root)

			code, stderr := runCLIExit(t, tc.args...)

			if code != exitcode.InvalidArgs {
				t.Errorf("specscore %s exited %d, want %d (InvalidArgs)", strings.Join(tc.args, " "), code, exitcode.InvalidArgs)
			}
			if !strings.Contains(stderr, tc.wantStderr) {
				t.Errorf("stderr = %q, want it to contain %q", stderr, tc.wantStderr)
			}
			if after := snapshotDir(t, root); !reflect.DeepEqual(before, after) {
				t.Errorf("project tree changed:\nbefore=%v\nafter=%v", before, after)
			}
		})
	}
}

func TestRun_UnknownCommand(t *testing.T) {
	err := Run([]string{"specscore", "nonexistent-command"})
	if err == nil {
		t.Fatal("Run([nonexistent-command]) = nil, want error")
	}
	// An unknown/unsupported subcommand must carry the dedicated
	// UnsupportedCommand exit code (8), so callers can distinguish an
	// outdated specscore from the shell's 127 (binary absent) and from a
	// generic failure (1).
	type exitCoder interface{ ExitCode() int }
	var ec exitCoder
	if !errors.As(err, &ec) {
		t.Fatalf("unknown-command error does not carry an exit code: %v", err)
	}
	if got := ec.ExitCode(); got != exitcode.UnsupportedCommand {
		t.Errorf("exit code = %d, want %d (UnsupportedCommand)", got, exitcode.UnsupportedCommand)
	}
}

// --- version subcommand (wired by fangcmd.Wire) ---

func TestVersionCommand_Output(t *testing.T) {
	root, _ := newRootCommand()
	cmd, _, err := root.Find([]string{"version"})
	if err != nil {
		t.Fatalf("root.Find([version]) error = %v", err)
	}
	var out bytes.Buffer
	cmd.SetOut(&out)
	if cmd.RunE == nil {
		t.Fatal("version command has no RunE")
	}
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("version command RunE() error = %v", err)
	}
	if !strings.Contains(out.String(), "specscore") {
		t.Errorf("output = %q, want to contain 'specscore'", out.String())
	}
}
