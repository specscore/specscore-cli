package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/projectdef"
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

func TestCodeDeps_CheckRequirementCitation(t *testing.T) {
	root := t.TempDir()
	if err := projectdef.WriteSpecConfig(root, projectdef.SpecConfig{}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "spec", "features", "auth"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "spec", "features", "auth", "README.md"), []byte("# Feature: Auth\n\n#### REQ: login\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "source.go"), []byte("// specscore:feature/auth#REQ:login\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	withCwd(t, root)
	if _, _, err := runCode(t, "deps", "--path=source.go", "--check"); err != nil {
		t.Fatalf("valid local REQ citation: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "source.go"), []byte("// specscore:feature/auth#REQ:renamed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := runCode(t, "deps", "--path=source.go", "--check")
	if err == nil || !strings.Contains(err.Error(), "source.go") || !strings.Contains(err.Error(), "REQ:renamed") {
		t.Fatalf("deleted requirement must name source file and citation: %v", err)
	}
}

func TestCodeDeps_CheckCrossRepoRequirementCitation(t *testing.T) {
	root, mirror := t.TempDir(), t.TempDir()
	config := projectdef.SchemaHeader + "\n\nproject:\n  host: github.com\n  org: acme\n  repo: consumer\nprojects:\n  - " + mirror + "\n"
	if err := os.WriteFile(filepath.Join(root, "specscore.yaml"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	mirrorConfig := projectdef.SchemaHeader + "\n\nproject:\n  host: github.com\n  org: acme\n  repo: provider\n"
	if err := os.WriteFile(filepath.Join(mirror, "specscore.yaml"), []byte(mirrorConfig), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(mirror, "spec", "features", "contract"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mirror, "spec", "features", "contract", "README.md"), []byte("# Feature: Contract\n\n#### REQ: retained\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"init", "-b", "main"}, {"config", "user.email", "cli@example.test"}, {"config", "user.name", "cli test"}, {"add", "."}, {"commit", "-m", "fixture"}} {
		if out, err := exec.Command("git", append([]string{"-C", mirror}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "source.go"), []byte("// specscore://github.com/acme/provider/feature/contract#REQ:retained\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	withCwd(t, root)
	if _, _, err := runCode(t, "deps", "--path=source.go", "--check"); err != nil {
		t.Fatalf("valid cross-repo citation: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mirror, "spec", "features", "contract", "README.md"), []byte("# Feature: Contract\n\n#### REQ: renamed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"add", "."}, {"commit", "-m", "rename requirement"}} {
		if out, err := exec.Command("git", append([]string{"-C", mirror}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
	_, _, err := runCode(t, "deps", "--path=source.go", "--check")
	if err == nil || exitCodeOf(err) != exitcode.InvalidState || !strings.Contains(err.Error(), "REQ:retained") {
		t.Fatalf("deleted cross-repo REQ must fail validation: %v", err)
	}
}

func TestCodeDeps_CheckCoverageBranches(t *testing.T) {
	missing := t.TempDir()
	if err := os.WriteFile(filepath.Join(missing, "source.go"), []byte("// specscore:feature/x#REQ:y\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	withCwd(t, missing)
	if _, _, err := runCode(t, "deps", "--path=source.go", "--check"); err == nil || !strings.Contains(err.Error(), "resolving project") {
		t.Fatalf("missing project: %v", err)
	}

	root := t.TempDir()
	if err := projectdef.WriteSpecConfig(root, projectdef.SpecConfig{}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "spec", "features", "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "spec", "features", "x", "README.md"), []byte("# Feature\n\n#### REQ: y\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "source.go"), []byte("// specscore:feature/x#REQ:y\n// specscore:plan/ignored\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	withCwd(t, root)
	if _, _, err := runCode(t, "deps", "--path=source.go", "--check", "--type=feature"); err != nil {
		t.Fatalf("filtered check: %v", err)
	}
	if _, _, err := runCode(t, "deps", "--path=source.go", "--check"); err != nil {
		t.Fatalf("plan citation remains listing-only: %v", err)
	}
}

func TestCodeDeps_CheckReportsMalformedCitationsInFileOrder(t *testing.T) {
	root := t.TempDir()
	if err := projectdef.WriteSpecConfig(root, projectdef.SpecConfig{}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.go"), []byte("// specscore:feature/missing#REQ:nope\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.go"), []byte("// specscore:feature/x@github.com/acme/provider\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	withCwd(t, root)
	if _, _, err := runCode(t, "deps", "--path=*.go"); err != nil {
		t.Fatalf("listing must keep malformed citation compatibility: %v", err)
	}
	_, _, err := runCode(t, "deps", "--path=*.go", "--check")
	if err == nil || exitCodeOf(err) != exitcode.InvalidState {
		t.Fatalf("malformed citation must fail validation: %v", err)
	}
	message := err.Error()
	if !strings.Contains(message, "b.go:1") {
		t.Fatalf("missing file/line diagnostic: %s", message)
	}
	if strings.Index(message, "a.go") > strings.Index(message, "b.go") {
		t.Fatalf("diagnostics are not file sorted: %s", message)
	}
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
