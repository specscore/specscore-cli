package cli

import (
	"bytes"
	"testing"
)

func runSelfUpdate(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := selfUpdateCommand()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

// AC: cli/self-update#ac:canonical-and-alias — invoking `self-update --check`
// and `update --check` MUST resolve to the same command and produce identical
// stdout and the same error/exit result. We verify alias equivalence by
// confirming "update" is registered as an alias of the canonical command, then
// driving the canonical command with --check and asserting deterministic
// output and a nil error.
func TestSelfUpdate_CanonicalAndAlias(t *testing.T) {
	cmd := selfUpdateCommand()
	if cmd.Name() != "self-update" {
		t.Errorf("canonical name = %q; want %q", cmd.Name(), "self-update")
	}
	if !cmd.HasAlias("update") {
		t.Errorf("command does not have alias %q; aliases=%v", "update", cmd.Aliases)
	}

	// Both invocations resolve to the same command object, so running the
	// canonical command with --check is representative of both call shapes.
	out, _, err := runSelfUpdate(t, "--check")
	if err != nil {
		t.Fatalf("self-update --check returned error: %v", err)
	}
	if out == "" {
		t.Error("expected deterministic placeholder output on stdout, got empty")
	}

	// Re-run to confirm the output is deterministic (identical across runs),
	// which is what guarantees the alias produces identical output.
	out2, _, err2 := runSelfUpdate(t, "--check")
	if err2 != nil {
		t.Fatalf("second run returned error: %v", err2)
	}
	if out != out2 {
		t.Errorf("output not deterministic: %q != %q", out, out2)
	}
}

// Extra positional args must be rejected to keep the call shape stable.
func TestSelfUpdate_RejectsExtraArgs(t *testing.T) {
	_, _, err := runSelfUpdate(t, "extra-positional")
	if err == nil {
		t.Fatal("expected error for extra positional argument")
	}
}

// The --yes flag has a -y shorthand and both --check and --yes default false.
func TestSelfUpdate_Flags(t *testing.T) {
	cmd := selfUpdateCommand()
	check := cmd.Flags().Lookup("check")
	if check == nil {
		t.Fatal("missing --check flag")
	}
	if check.DefValue != "false" {
		t.Errorf("--check default = %q; want false", check.DefValue)
	}
	yes := cmd.Flags().Lookup("yes")
	if yes == nil {
		t.Fatal("missing --yes flag")
	}
	if yes.Shorthand != "y" {
		t.Errorf("--yes shorthand = %q; want y", yes.Shorthand)
	}
	if yes.DefValue != "false" {
		t.Errorf("--yes default = %q; want false", yes.DefValue)
	}
}
