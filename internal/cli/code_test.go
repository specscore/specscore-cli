package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

func runCode(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := codeCommand()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func TestCodeDeps_InvalidType(t *testing.T) {
	_, _, err := runCode(t, "deps", "--type=banana")
	if err == nil {
		t.Fatal("expected error for invalid --type, got nil")
	}
	if got := exitCodeOf(err); got != exitcode.InvalidArgs {
		t.Fatalf("exit code = %d, want %d (InvalidArgs)", got, exitcode.InvalidArgs)
	}
	if !strings.Contains(err.Error(), "invalid --type") {
		t.Errorf("error should mention 'invalid --type', got: %q", err.Error())
	}
}

func TestCodeDeps_NoFiles(t *testing.T) {
	// Use a pattern that won't match anything in a temp dir.
	tmp := t.TempDir()
	withCwd(t, tmp)
	out, _, err := runCode(t, "deps", "--path=nonexistent_pattern_xyz_*.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if out != "" {
		t.Errorf("expected empty output for no matching files, got: %q", out)
	}
}

func TestCodeDeps_WithAnnotations(t *testing.T) {
	tmp := t.TempDir()
	// Create a Go file with a specscore: annotation.
	goFile := filepath.Join(tmp, "main.go")
	content := `package main

// specscore:feature/auth
func main() {}
`
	if err := os.WriteFile(goFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write go file: %v", err)
	}
	withCwd(t, tmp)
	out, _, err := runCode(t, "deps", "--path=main.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "spec/features/auth") {
		t.Errorf("expected output to contain 'spec/features/auth', got: %q", out)
	}
}

// TestCodeDeps_SpaceAfterColon reproduces the ingitdb-go annotation style
// `// specscore: feature/<slug>` (a space between the prefix colon and the
// reference body). These annotations were silently dropped before the fix.
func TestCodeDeps_SpaceAfterColon(t *testing.T) {
	tmp := t.TempDir()
	goFile := filepath.Join(tmp, "foreign_key.go")
	content := `package ingitdb

// specscore: feature/column-validation
type ForeignKey struct{}
`
	if err := os.WriteFile(goFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write go file: %v", err)
	}
	withCwd(t, tmp)
	out, _, err := runCode(t, "deps", "--path=foreign_key.go")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "spec/features/column-validation") {
		t.Errorf("expected space-form annotation to be reported, got: %q", out)
	}
}

// TestCodeDeps_RecursiveGlobMatchesDirectChildren ensures a `dir/**/*.go`
// glob reports annotations in files that are direct children of dir, not only
// in deeper subdirectories (cli/code/deps#req:path-glob).
func TestCodeDeps_RecursiveGlobMatchesDirectChildren(t *testing.T) {
	tmp := t.TempDir()
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(os.MkdirAll(filepath.Join(tmp, "ingitdb", "validator"), 0o755))
	// Direct child of ingitdb/
	must(os.WriteFile(filepath.Join(tmp, "ingitdb", "foreign_key.go"),
		[]byte("package ingitdb\n// specscore: feature/column-validation\n"), 0o644))
	// Nested under ingitdb/validator/
	must(os.WriteFile(filepath.Join(tmp, "ingitdb", "validator", "def_validator.go"),
		[]byte("package validator\n// specscore: feature/definition-inheritance\n"), 0o644))
	withCwd(t, tmp)

	out, _, err := runCode(t, "deps", "--path=ingitdb/**/*.go", "--type=feature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "spec/features/column-validation") {
		t.Errorf("expected direct-child annotation reported, got: %q", out)
	}
	if !strings.Contains(out, "spec/features/definition-inheritance") {
		t.Errorf("expected nested annotation reported, got: %q", out)
	}
}

func TestCodeDeps_TypeFilter(t *testing.T) {
	tmp := t.TempDir()
	goFile := filepath.Join(tmp, "svc.go")
	content := `package svc

// specscore:feature/payments
// specscore:plan/rollout
func process() {}
`
	if err := os.WriteFile(goFile, []byte(content), 0o644); err != nil {
		t.Fatalf("write go file: %v", err)
	}
	withCwd(t, tmp)

	// Filter to only features.
	out, _, err := runCode(t, "deps", "--path=svc.go", "--type=feature")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "spec/features/payments") {
		t.Errorf("expected feature ref in output, got: %q", out)
	}
	if strings.Contains(out, "spec/plans/rollout") {
		t.Errorf("plan ref should be filtered out, got: %q", out)
	}
}
