package cli

import (
	"bytes"
	"strings"
	"testing"
)

// runConsilium invokes the consilium command tree in-process with the given args.
func runConsilium(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := consiliumCommand()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

// AC: consilium-parent-prints-help — bare `specscore consilium` with no
// subcommand prints help listing the verdict, roster, and config subcommands
// and exits 0 (cobra's default parent-with-no-Run behavior).
func TestConsiliumCommand_HelpListsVerbsAndExitsZero(t *testing.T) {
	out, _, err := runConsilium(t)
	if err != nil {
		t.Fatalf("bare `consilium` returned error: %v", err)
	}
	for _, verb := range []string{"verdict", "roster", "config"} {
		if !strings.Contains(out, verb) {
			t.Errorf("expected bare `consilium` help to mention %q subcommand; got:\n%s", verb, out)
		}
	}
}
