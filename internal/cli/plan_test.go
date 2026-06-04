package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

// setupPlansSpec stages a minimal spec repo with a spec/plans/ directory
// at a fresh t.TempDir and chdirs into it so resolvePlansDir picks it up
// via the cwd-anchored FindSpecRepoRoot heuristic. Returns the plans dir.
func setupPlansSpec(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	// spec/features/ presence makes FindSpecRepoRoot recognize the root.
	if err := os.MkdirAll(filepath.Join(root, "spec", "features"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	plansDir := filepath.Join(root, "spec", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	withCwd(t, root)
	return plansDir
}

// runPlan invokes the `plan` cobra command tree in-process and captures
// stdout, stderr, and the returned error.
func runPlan(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := planCommand()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

// AC: group-exposes-subcommands
func TestPlanGroup_NoSubcommand_PrintsHelpExitsZero(t *testing.T) {
	out, _, err := runPlan(t)
	if err != nil {
		t.Fatalf("expected nil error (exit 0), got %v", err)
	}
	if !strings.Contains(out, "Query plans") {
		t.Errorf("expected help text describing the plan group, got: %q", out)
	}
}

// AC: unknown-slug-exits-3
func TestResolvePlanPath_MissingSlug_Exits3(t *testing.T) {
	plansDir := setupPlansSpec(t)
	_, err := resolvePlanPath(plansDir, "does-not-exist")
	if err == nil {
		t.Fatal("expected error for missing plan, got nil")
	}
	if got := exitCodeOfErr(err); got != exitcode.NotFound {
		t.Errorf("expected exit code %d (NotFound), got %d", exitcode.NotFound, got)
	}
	if !strings.Contains(err.Error(), "does-not-exist") {
		t.Errorf("expected error to name the slug, got: %q", err.Error())
	}
}

// resolvePlanPath returns the path for an existing plan with no error.
func TestResolvePlanPath_Existing_ReturnsPath(t *testing.T) {
	plansDir := setupPlansSpec(t)
	planPath := filepath.Join(plansDir, "my-plan.md")
	if err := os.WriteFile(planPath, []byte("# Plan\n"), 0o644); err != nil {
		t.Fatalf("write plan: %v", err)
	}
	got, err := resolvePlanPath(plansDir, "my-plan")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if got != planPath {
		t.Errorf("expected %q, got %q", planPath, got)
	}
}

// AC: invalid-format-rejected — exercises the shared validateFormat the
// plan subcommands will reuse.
func TestValidateFormat_Xml_Exits2(t *testing.T) {
	err := validateFormat("xml")
	if err == nil {
		t.Fatal("expected error for xml format, got nil")
	}
	if got := exitCodeOfErr(err); got != exitcode.InvalidArgs {
		t.Errorf("expected exit code %d (InvalidArgs), got %d", exitcode.InvalidArgs, got)
	}
	if !strings.Contains(err.Error(), "xml") {
		t.Errorf("expected error to name xml, got: %q", err.Error())
	}
}

// resolvePlansDir returns the plans path without erroring when the
// directory is absent (emptiness is the list command's concern).
func TestResolvePlansDir_AbsentDir_NoError(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "spec", "features"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	withCwd(t, root)
	got, err := resolvePlansDir("")
	if err != nil {
		t.Fatalf("expected nil error for absent plans dir, got %v", err)
	}
	// macOS resolves /tmp via a symlink, so anchor the expectation on the
	// post-chdir working directory rather than the raw t.TempDir() path.
	cwd, _ := os.Getwd()
	want := filepath.Join(cwd, "spec", "plans")
	if got != want {
		t.Errorf("expected %q, got %q", want, got)
	}
}
