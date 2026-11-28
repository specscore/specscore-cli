package cli

import (
	"bytes"
	"errors"
	"io"
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
	// Keep the real entry point away from the host's telemetry state:
	// PersistentPreRun reads ~/.specscore and creates an install id there.
	t.Setenv("DO_NOT_TRACK", "1")
	t.Setenv("SPECSCORE_TELEMETRY", "0")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	savedInvocation := invocation
	invocation = runtimeState{}
	t.Cleanup(func() { invocation = savedInvocation })
	code := 0
	old := osExit
	osExit = func(c int) { code = c }
	t.Cleanup(func() { osExit = old })
	stderr := captureStderr(t, func() {
		Fatal(Run(append([]string{"specscore"}, args...)))
	})
	return code, stderr
}

// specscore:verifies feature/cli#ac:usage-errors-exit-2 — one case per Cobra validator kind, a required flag, an extra positional on NoArgs leaves, and flag-parse errors, on verbs other than idea change-status
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
		{"NoArgs leaf given a positional", []string{"idea", "list", "extra"}, `unexpected argument "extra": specscore idea list accepts no arguments`},
		{"NoArgs top-level leaf given a positional", []string{"version", "extra"}, `unexpected argument "extra": specscore version accepts no arguments`},
		{"leaf Cobra registers during Execute", []string{"completion", "bash", "extra"}, `unexpected argument "extra": specscore completion bash accepts no arguments`},
		{"leaf fang registers during Execute", []string{"man", "extra"}, `unexpected argument "extra": specscore man accepts no arguments`},
		{"unknown flag", []string{"idea", "list", "--zz-bogus"}, "unknown flag: --zz-bogus"},
		{"flag value does not parse", []string{"lesson", "list", "--min-recurred=abc"}, `invalid argument "abc" for "--min-recurred" flag`},
		{"flag missing its value", []string{"idea", "change-status", "foo", "--to"}, "flag needs an argument: --to"},
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

// An extra positional on a command GROUP is still an unrecognized
// subcommand (cli#req:unsupported-subcommand-mapping), and --help is not a
// flag error. Guards the boundary of mapLeafExtraArgs.
func TestRun_GroupUnknownSubcommandStaysExit8(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)
	code, stderr := runCLIExit(t, "graph", "verdict")
	if code != exitcode.UnsupportedCommand {
		t.Errorf("specscore graph verdict exited %d, want %d (UnsupportedCommand)", code, exitcode.UnsupportedCommand)
	}
	if !strings.Contains(stderr, `unknown command "verdict"`) {
		t.Errorf("stderr = %q, want it to name the unknown subcommand", stderr)
	}
	if code, _ := runCLIExit(t, "idea", "list", "--help"); code != 0 {
		t.Errorf("specscore idea list --help exited %d, want 0", code)
	}
}

func TestMapLeafExtraArgs(t *testing.T) {
	leaf := &cobra.Command{Use: "leaf", Args: cobra.NoArgs, Run: func(*cobra.Command, []string) {}}
	sub := &cobra.Command{Use: "sub", Run: func(*cobra.Command, []string) {}}
	group := &cobra.Command{Use: "group", Args: cobra.NoArgs}
	group.AddCommand(sub)
	root := &cobra.Command{Use: "root"}
	root.AddCommand(leaf, group)
	// Registered after everything else, the way Cobra adds `completion` and
	// fang adds `man` during Execute: the mapping must still cover it.
	late := &cobra.Command{Use: "late", Args: cobra.NoArgs, Run: func(*cobra.Command, []string) {}}
	group.AddCommand(late)

	var ec interface{ ExitCode() int }
	for _, c := range []*cobra.Command{leaf, late} {
		in := c.Args(c, []string{"extra"})
		got := mapLeafExtraArgs(root, in)
		if !errors.As(got, &ec) || ec.ExitCode() != exitcode.InvalidArgs {
			t.Errorf("%s with a positional = %v, want exit %d", c.CommandPath(), got, exitcode.InvalidArgs)
		}
		if want := `unexpected argument "extra": ` + c.CommandPath() + " accepts no arguments"; got.Error() != want || !errors.Is(got, in) {
			t.Errorf("%s: message %q / cause lost", c.CommandPath(), got.Error())
		}
	}
	unchanged := []error{
		group.Args(group, []string{"extra"}),                                    // group: an unrecognized subcommand, exit 8 later
		errors.New(`unknown command "x" for "root nowhere"`),                    // path not in the tree
		errors.New(`unknown command "x" for "root"` + "\n\nDid you mean this?"), // root's legacyArgs wording
		exitcode.NotFoundError(`unknown command "x" for "root leaf"`),           // already coded
		errors.New("something else failed"),
	}
	for _, in := range unchanged {
		if got := mapLeafExtraArgs(root, in); got != in {
			t.Errorf("mapLeafExtraArgs(%q) = %v, want it unchanged", in, got)
		}
	}
	if got := mapLeafExtraArgs(root, nil); got != nil {
		t.Errorf("mapLeafExtraArgs(nil) = %v", got)
	}
}

// TestRejectUnknownSubcommands pins which commands the walk rewrites: a
// command with no Run function (with or without subcommands, keeping any
// annotations it already has), and a runnable group with no Args. A runnable
// group's own Args and a leaf are left alone, and so is the root.
func TestRejectUnknownSubcommands(t *testing.T) {
	run := func(*cobra.Command, []string) {}
	own := func(*cobra.Command, []string) error { return nil }
	root := &cobra.Command{Use: "root"}
	dispatcher := &cobra.Command{Use: "dispatcher", Args: cobra.ArbitraryArgs}
	dispatcher.AddCommand(&cobra.Command{Use: "sub", Run: run})
	placeholder := &cobra.Command{Use: "placeholder", Annotations: map[string]string{"kept": "yes"}}
	runnableNoArgs := &cobra.Command{Use: "runnable", Run: run}
	runnableNoArgs.AddCommand(&cobra.Command{Use: "sub", Run: run})
	runnableOwnArgs := &cobra.Command{Use: "own", Run: run, Args: own}
	runnableOwnArgs.AddCommand(&cobra.Command{Use: "sub", Run: run})
	leaf := &cobra.Command{Use: "leaf", Run: run}
	root.AddCommand(dispatcher, placeholder, runnableNoArgs, runnableOwnArgs, leaf)

	rejectUnknownSubcommands(root)

	for _, c := range []*cobra.Command{dispatcher, placeholder, runnableNoArgs} {
		if !isCommandGroup(c) {
			t.Errorf("%s is not treated as a command group", c.Name())
		}
		if err := c.Args(c, []string{"x"}); err == nil || !strings.HasPrefix(err.Error(), `unknown command "x"`) {
			t.Errorf("%s Args(x) = %v, want an unknown-command error", c.Name(), err)
		}
		if !c.Runnable() {
			t.Errorf("%s is not runnable, so Cobra would never validate its arguments", c.Name())
		}
	}
	if placeholder.Annotations["kept"] != "yes" {
		t.Errorf("placeholder annotations = %v, existing annotation lost", placeholder.Annotations)
	}
	if runnableNoArgs.RunE != nil || runnableOwnArgs.RunE != nil {
		t.Error("a runnable group's own Run function was replaced")
	}
	if runnableOwnArgs.Args(runnableOwnArgs, []string{"x"}) != nil {
		t.Error("a runnable group's own Args was replaced")
	}
	if isCommandGroup(leaf) || leaf.Args != nil || root.Args != nil || root.Runnable() {
		t.Error("a leaf or the root was rewritten")
	}
}

// executedCommandTree returns the root command as it stands after a real
// execution: it includes the commands Cobra and fang register only during
// Execute (`completion` and its shells, `man`, `help`), and every group has
// been through rejectUnknownSubcommands.
func executedCommandTree(t *testing.T) *cobra.Command {
	t.Helper()
	t.Setenv("DO_NOT_TRACK", "1")
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	savedInvocation := invocation
	invocation = runtimeState{}
	t.Cleanup(func() { invocation = savedInvocation })
	root, opts := newRootCommand()
	root.SetArgs([]string{"help"})
	root.SetOut(io.Discard)
	if err := executeWithPanicRecovery(root, opts...); err != nil {
		t.Fatalf("execute: %v", err)
	}
	return root
}

// commandArgs returns the arguments that invoke cmd, its path without the
// root's name.
func commandArgs(cmd *cobra.Command) []string {
	return strings.Fields(cmd.CommandPath())[1:]
}

// specscore:verifies feature/cli#ac:unknown-subcommand-exits-8 — every command group of the executed tree, at every depth, given an unrecognized subcommand
// specscore:verifies feature/cli/issue#ac:unknown-subcommand-rejected — `specscore issue frobnicate` is one of the enumerated groups
//
// TestRun_EveryCommandGroupRejectsUnknownSubcommand runs the real entry
// point once per command group found in the executed tree, so a group added
// later is covered without a list. Each must exit 8 with a stderr message
// naming the subcommand and nothing on stdout; invoked with no subcommand,
// each still prints its help on stdout and exits 0.
func TestRun_EveryCommandGroupRejectsUnknownSubcommand(t *testing.T) {
	var groups []*cobra.Command
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			if isCommandGroup(sub) {
				groups = append(groups, sub)
			}
			walk(sub)
		}
	}
	walk(executedCommandTree(t))

	seen := map[string]bool{}
	for _, group := range groups {
		args := commandArgs(group)
		seen[group.CommandPath()] = true
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			root := setupSpecRoot(t)
			withCwd(t, root)
			var code int
			var stderr string
			stdout := captureStdout(t, func() {
				code, stderr = runCLIExit(t, append(args, "frobnicate")...)
			})
			if code != exitcode.UnsupportedCommand {
				t.Errorf("%s frobnicate exited %d, want %d (UnsupportedCommand)", group.CommandPath(), code, exitcode.UnsupportedCommand)
			}
			if want := `unknown command "frobnicate" for "` + group.CommandPath() + `"`; !strings.Contains(stderr, want) {
				t.Errorf("stderr = %q, want it to contain %q", stderr, want)
			}
			if stdout != "" {
				t.Errorf("stdout = %q, want nothing on stdout", stdout)
			}

			stdout = captureStdout(t, func() { code, _ = runCLIExit(t, args...) })
			if code != 0 {
				t.Errorf("%s with no subcommand exited %d, want 0", group.CommandPath(), code)
			}
			if !strings.Contains(stdout, "USAGE") {
				t.Errorf("%s with no subcommand printed %q on stdout, want its help", group.CommandPath(), stdout)
			}
		})
	}
	for _, path := range []string{
		"specscore idea", "specscore lesson occurrence", "specscore skills",
		"specscore graph", "specscore event", "specscore completion", "specscore consilium",
	} {
		if !seen[path] {
			t.Errorf("enumeration did not reach command group %q", path)
		}
	}
}

// specscore:verifies feature/cli#ac:usage-errors-exit-2 — every leaf of the executed tree that takes no positionals, given one
//
// TestRun_EveryNoArgLeafRejectsExtraArgument runs the real entry point once
// per leaf command that accepts no positionals, found in the executed tree
// (including `completion bash` and fang's `man`), so a leaf added later is
// covered without a list. Each must exit 2, and stderr must say that the
// command accepts no arguments instead of calling the argument an unknown
// command.
func TestRun_EveryNoArgLeafRejectsExtraArgument(t *testing.T) {
	var leaves []*cobra.Command
	var walk func(c *cobra.Command)
	walk = func(c *cobra.Command) {
		for _, sub := range c.Commands() {
			walk(sub)
		}
		if isCommandGroup(c) || c.Args == nil {
			return
		}
		if err := c.Args(c, []string{"extra"}); err != nil && strings.HasPrefix(err.Error(), "unknown command ") {
			leaves = append(leaves, c)
		}
	}
	walk(executedCommandTree(t))

	seen := map[string]bool{}
	for _, leaf := range leaves {
		args := commandArgs(leaf)
		seen[leaf.CommandPath()] = true
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			root := setupSpecRoot(t)
			withCwd(t, root)
			before := snapshotDir(t, root)
			code, stderr := runCLIExit(t, append(args, "extra")...)
			if code != exitcode.InvalidArgs {
				t.Errorf("%s extra exited %d, want %d (InvalidArgs)", leaf.CommandPath(), code, exitcode.InvalidArgs)
			}
			if want := `unexpected argument "extra": ` + leaf.CommandPath() + " accepts no arguments"; !strings.Contains(stderr, want) {
				t.Errorf("stderr = %q, want it to contain %q", stderr, want)
			}
			if strings.Contains(strings.ToLower(stderr), "unknown command") {
				t.Errorf("stderr = %q, still calls the argument an unknown command", stderr)
			}
			if after := snapshotDir(t, root); !reflect.DeepEqual(before, after) {
				t.Errorf("project tree changed:\nbefore=%v\nafter=%v", before, after)
			}
		})
	}
	for _, path := range []string{"specscore completion bash", "specscore completion powershell", "specscore man", "specscore idea list"} {
		if !seen[path] {
			t.Errorf("enumeration did not reach NoArgs leaf %q; the executed tree is not the one being checked", path)
		}
	}
}

// captureStdout replaces os.Stdout with a pipe for the duration of fn and
// returns the bytes written.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stdout
	os.Stdout = w
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()
	fn()
	os.Stdout = orig
	_ = w.Close()
	out := <-done
	_ = r.Close()
	return out
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
