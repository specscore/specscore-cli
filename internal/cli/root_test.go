package cli

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

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
