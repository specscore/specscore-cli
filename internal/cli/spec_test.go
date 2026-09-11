package cli

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/projectdef"
)

// runSpecLint runs `specscore spec lint` against the given working
// directory and returns stdout, stderr, and the exit error.
func runSpecLintCmd(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := specCommand()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(append([]string{"lint"}, args...))
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

// writeValidSpecscoreYAML drops a minimal lint-valid specscore.yaml at
// dir so that findRepoConfigRoot finds it. The schema-header comment on
// line 1 is mandatory per repo-config#req:schema-header-comment.
func writeValidSpecscoreYAML(t *testing.T, dir string) {
	t.Helper()
	if err := projectdef.WriteSpecConfig(dir, projectdef.SpecConfig{}); err != nil {
		t.Fatalf("write specscore.yaml: %v", err)
	}
}

// AC: missing-specscore-yaml-exits-3 — bare spec/features/ tree without
// specscore.yaml MUST exit 3 with init-pointing message.
func TestSpecLint_MissingSpecscoreYAML_ExitsNotFound(t *testing.T) {
	root := t.TempDir()
	// Create the legacy spec/features/ fallback to prove it does NOT
	// satisfy the lint gate.
	if err := os.MkdirAll(filepath.Join(root, "spec", "features"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	_, _, err := runSpecLintCmd(t, "--project", root)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if got := exitCodeOf(err); got != exitcode.NotFound {
		t.Fatalf("exit code = %d, want %d (NotFound); err = %v", got, exitcode.NotFound, err)
	}
	msg := err.Error()
	if !strings.Contains(msg, "specscore.yaml") {
		t.Errorf("error message should name specscore.yaml; got: %q", msg)
	}
	if !strings.Contains(msg, "specscore init") {
		t.Errorf("error message should instruct caller to run `specscore init`; got: %q", msg)
	}
}

// Walking-up behavior — a specscore.yaml in an ancestor of the start
// directory MUST satisfy the gate.
func TestSpecLint_FindsSpecscoreYAMLInAncestor(t *testing.T) {
	root := t.TempDir()
	writeValidSpecscoreYAML(t, root)
	// Build an empty (but valid) spec tree so lint has zero violations.
	if err := os.MkdirAll(filepath.Join(root, "spec", "features"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	specReadme := "# Specifications\n\nTest tree.\n\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(filepath.Join(root, "spec", "README.md"), []byte(specReadme), 0o644); err != nil {
		t.Fatalf("write spec/README.md: %v", err)
	}
	featReadme := "# Features\n\n## Index\n\n| Feature | Status | Description |\n|---------|--------|-------------|\n\n## Open Questions\n\nNone at this time.\n"
	if err := os.WriteFile(filepath.Join(root, "spec", "features", "README.md"), []byte(featReadme), 0o644); err != nil {
		t.Fatalf("write features/README.md: %v", err)
	}

	// Run from a nested subdirectory — the walk-up must locate
	// specscore.yaml at root.
	nested := filepath.Join(root, "spec", "features")
	_, _, err := runSpecLintCmd(t, "--project", nested)
	// We don't assert success of every rule (empty tree may still have
	// rule complaints); we assert we do NOT get the NotFound gate error.
	if err != nil {
		if got := exitCodeOf(err); got == exitcode.NotFound {
			t.Fatalf("unexpected NotFound from lint when specscore.yaml exists at %s: %v", root, err)
		}
	}
}

// TestSpecLint_FixTextFormatPrintsReconciledRows covers runSpecLint's text
// -format reconciliation summary: --fix repairing a feature-index-row-sync
// drift (the index Status cell for "auth" left stale after the Feature
// file's own **Status:** line was edited directly, bypassing `feature
// change-status`) must be echoed to stderr in text format.
func TestSpecLint_FixTextFormatPrintsReconciledRows(t *testing.T) {
	root := setupFeatureSpec(t, "Draft")
	// Host/Org/Repo satisfy the studio-toolbar rule so the only violation
	// exercised by this test is the feature-index drift being fixed below.
	cfg := projectdef.SpecConfig{Project: &projectdef.ProjectConfig{Host: "github.com", Org: "acme", Repo: "widgets"}}
	if err := projectdef.WriteSpecConfig(root, cfg); err != nil {
		t.Fatalf("write specscore.yaml: %v", err)
	}

	// Drift the Feature file's Status directly (not via `feature
	// change-status`, which would already repair the index itself) so the
	// features/README.md index row is left stale at "Draft".
	authPath := filepath.Join(root, "spec", "features", "auth", "README.md")
	before, err := os.ReadFile(authPath)
	if err != nil {
		t.Fatalf("read auth/README.md: %v", err)
	}
	drifted := strings.Replace(string(before), "**Status:** Draft", "**Status:** In Review", 1)
	if drifted == string(before) {
		t.Fatalf("drift replacement did not match; fixture changed?")
	}
	if err := os.WriteFile(authPath, []byte(drifted), 0o644); err != nil {
		t.Fatalf("write drifted auth/README.md: %v", err)
	}

	out, errOut, err := runSpecLintCmd(t, "--fix", "--project", root)
	if err != nil {
		t.Fatalf("unexpected err: %v\nstdout=%s\nstderr=%s", err, out, errOut)
	}
	if !strings.Contains(errOut, "Reconciled 1 index row(s) from their artifact file(s):") {
		t.Fatalf("stderr missing reconciliation summary header; got: %q", errOut)
	}
	if !strings.Contains(errOut, "auth") {
		t.Fatalf("stderr missing reconciled row detail for auth; got: %q", errOut)
	}
	if got := readIndexStatus(t, root); got != "In Review" {
		t.Errorf("index Status after --fix = %q, want In Review", got)
	}
}
