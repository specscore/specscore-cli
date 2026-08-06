package cli

// End-to-end integration smoke-test for the two new lifecycle verbs:
// `specscore idea change-status` and `specscore feature change-status`.
//
// This test is the final acceptance gate of the lifecycle-verbs-
// implementation plan. It exercises BOTH verbs across the full happy
// path against a single tmp spec tree, asserting:
//
//   * each transition exits 0 and rewrites the **Status:** line,
//   * the appropriate index README is synced after every step,
//   * `lint.Lint` reports 0 error-severity violations at every
//     documented checkpoint (steps 3-10 in the plan),
//   * a sample of illegal transitions return exit 4 with no on-disk
//     drift, and
//   * `--help` lists both new subcommands under their parent groups.
//
// In-process cobra invocation (no shelling out) keeps the test under
// the <10s budget.

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lint"
	"github.com/spf13/cobra"
)

// assertLintClean fails the test if the spec tree under <root>/spec
// contains any error-severity lint violations. Used between every
// happy-path transition to enforce the plan's "lint is clean at every
// checkpoint" acceptance criterion.
//
// Runs with Fix=true so that auto-fixable rules (adherence-footer,
// studio-toolbar, etc.) are materialized first — mirroring the real
// workflow where `specscore spec lint --fix` is run before the
// "clean" check.
func assertLintClean(t *testing.T, root, label string) {
	t.Helper()
	violations, err := lint.Lint(lint.Options{SpecRoot: filepath.Join(root, "spec"), Fix: true})
	if err != nil {
		t.Fatalf("[%s] lint invocation failed: %v", label, err)
	}
	for _, v := range violations {
		if v.Severity == "error" {
			t.Errorf("[%s] unexpected error-severity violation: %s:%d [%s] %s",
				label, v.File, v.Line, v.Rule, v.Message)
		}
	}
}

// readFileStatus returns the trimmed value of the first **Status:** line
// found in the file at path. Empty string if no status line is found.
func readFileStatus(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	for _, line := range strings.Split(string(data), "\n") {
		l := strings.TrimSpace(line)
		if strings.HasPrefix(l, "**Status:**") {
			return strings.TrimSpace(strings.TrimPrefix(l, "**Status:**"))
		}
	}
	return ""
}

// fileContains returns true when path contains the literal substring.
func fileContains(t *testing.T, path, substring string) bool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return strings.Contains(string(data), substring)
}

// snapshotFile returns the raw bytes of path; nil if missing.
func snapshotFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		t.Fatalf("snapshot %s: %v", path, err)
	}
	return data
}

// assertExitCode pulls the exit code from err and fails the test if it
// does not match want. Used by the illegal-transition assertions.
func assertExitCode(t *testing.T, err error, want int, label string) {
	t.Helper()
	if err == nil {
		t.Fatalf("[%s] expected error with exit code %d, got nil", label, want)
	}
	var ec *exitcode.Error
	if !errors.As(err, &ec) {
		t.Fatalf("[%s] error does not carry ExitCode: %v", label, err)
	}
	if got := ec.ExitCode(); got != want {
		t.Errorf("[%s] exit code = %d, want %d (err=%v)", label, got, want, err)
	}
}

// TestLifecycleIntegration drives both lifecycle verbs end-to-end
// against a single tmp spec tree. Steps map 1:1 to the plan's task-5
// flow.
func TestLifecycleIntegration(t *testing.T) {
	root := t.TempDir()
	withCwd(t, root)

	// Step 1: specscore init.
	// Pass explicit project identity via flags so that downstream
	// feature READMEs get a resolvable studio-toolbar URL grammar even
	// without a real git remote in the tmpdir.
	if _, errOut, err := runInitCmd(t, nil,
		"--project", root,
		"--host", "github.com",
		"--org", "test",
		"--repo", "lifecycle",
	); err != nil {
		t.Fatalf("step 1: init failed: %v\nstderr=%s", err, errOut)
	}
	if _, err := os.Stat(filepath.Join(root, "specscore.yaml")); err != nil {
		t.Fatalf("step 1: specscore.yaml missing after init: %v", err)
	}
	assertLintClean(t, root, "after init")

	// Step 2: idea new foo — creates spec/ideas/foo.md at Status: Draft.
	if _, _, err := runIdea(t, "new", "foo", "--owner", "tester"); err != nil {
		t.Fatalf("step 2: idea new foo failed: %v", err)
	}
	ideaActive := filepath.Join(root, "spec", "ideas", "foo.md")
	if got := readFileStatus(t, ideaActive); got != "Draft" {
		t.Fatalf("step 2: idea status = %q, want Draft", got)
	}
	assertLintClean(t, root, "after idea new")

	// Step 3: idea change-status foo --to=approved.
	out, _, err := runIdea(t, "change-status", "foo", "--to=approved")
	if err != nil {
		t.Fatalf("step 3: idea change-status approved failed: %v", err)
	}
	if want := "foo: Draft → Approved\n"; out != want {
		t.Errorf("step 3: stdout = %q, want %q", out, want)
	}
	if got := readFileStatus(t, ideaActive); got != "Approved" {
		t.Errorf("step 3: idea status = %q, want Approved", got)
	}
	// Index row reflects the new status.
	if !fileContains(t, filepath.Join(root, "spec", "ideas", "README.md"), "Approved") {
		t.Errorf("step 3: ideas index not synced to Approved")
	}
	assertLintClean(t, root, "after idea→approved")

	// Illegal transition 1: re-run approved on a now-Approved idea.
	// Per the Idea matrix, Approved → Approved is rejected (exit 4)
	// because there is no idempotent shortcut.
	preBytes := snapshotFile(t, ideaActive)
	_, _, err = runIdea(t, "change-status", "foo", "--to=approved")
	assertExitCode(t, err, exitcode.InvalidState, "idea Approved→Approved re-run")
	if got := snapshotFile(t, ideaActive); !bytes.Equal(preBytes, got) {
		t.Errorf("idea file mutated by rejected re-run")
	}

	// Step 4a: idea change-status foo --to=stale — a terminal disposition.
	// Archival is orthogonal to status and is the next step.
	out, _, err = runIdea(t, "change-status", "foo", "--to=stale")
	if err != nil {
		t.Fatalf("step 4a: idea change-status stale failed: %v", err)
	}
	if want := "foo: Approved → Stale\n"; out != want {
		t.Errorf("step 4a: stdout = %q, want %q", out, want)
	}
	assertLintClean(t, root, "after idea→stale")

	// Step 4b: idea archive foo — keeps the terminal **Status:** (Stale),
	// adds **Archived:** true, relocates to archived/, and syncs both
	// indexes.
	if _, _, err = runIdea(t, "archive", "foo"); err != nil {
		t.Fatalf("step 4b: idea archive failed: %v", err)
	}
	if _, err := os.Stat(ideaActive); !os.IsNotExist(err) {
		t.Errorf("step 4b: active idea file should be gone, err=%v", err)
	}
	ideaArchived := filepath.Join(root, "spec", "ideas", "archived", "foo.md")
	if got := readFileStatus(t, ideaArchived); got != "Stale" {
		t.Errorf("step 4b: archived idea status = %q, want Stale (terminal status preserved)", got)
	}
	if !fileContains(t, ideaArchived, "**Archived:** true") {
		t.Errorf("step 4b: archived idea missing **Archived:** true axis")
	}
	if !fileContains(t, filepath.Join(root, "spec", "ideas", "archived", "README.md"), "foo") {
		t.Errorf("step 4b: archived index not synced to list foo")
	}
	assertLintClean(t, root, "after idea→archived")

	// Step 5: feature new bar --status Draft. The flag spelling here
	// matches the existing featureNewCommand contract (--title is
	// required; --slug and --status are explicit).
	if _, errOut, err := runFeature(t, "new",
		"--title", "Bar",
		"--slug", "bar",
		"--status", "Draft",
		"--description", "integration test feature",
	); err != nil {
		t.Fatalf("step 5: feature new bar failed: %v\nstderr=%s", err, errOut)
	}
	featPath := filepath.Join(root, "spec", "features", "bar", "README.md")
	featuresIdx := filepath.Join(root, "spec", "features", "README.md")
	if got := readFileStatus(t, featPath); got != "Draft" {
		t.Fatalf("step 5: feature status = %q, want Draft", got)
	}
	if !fileContains(t, featuresIdx, "| [Bar](bar/README.md) | Draft | integration test feature |") {
		t.Errorf("step 5: features index missing row for bar at Draft")
	}
	assertLintClean(t, root, "after feature new")

	// Illegal transition 2: Draft → Stable skip-step (exit 4).
	preBytes = snapshotFile(t, featPath)
	_, _, err = runFeature(t, "change-status", "bar", "--to=stable")
	assertExitCode(t, err, exitcode.InvalidState, "feature Draft→Stable skip-step")
	if got := snapshotFile(t, featPath); !bytes.Equal(preBytes, got) {
		t.Errorf("feature file mutated by rejected Draft→Stable")
	}

	// Step 6: feature change-status bar --to="in review".
	out, _, err = runFeature(t, "change-status", "bar", "--to=in review")
	if err != nil {
		t.Fatalf("step 6: feature change-status in review failed: %v", err)
	}
	if want := "bar: Draft → In Review\n"; out != want {
		t.Errorf("step 6: stdout = %q, want %q", out, want)
	}
	if got := readFileStatus(t, featPath); got != "In Review" {
		t.Errorf("step 6: feature status = %q, want In Review", got)
	}
	if !fileContains(t, featuresIdx, "| [Bar](bar/README.md) | In Review | integration test feature |") {
		t.Errorf("step 6: features index Status cell not synced to In Review")
	}
	assertLintClean(t, root, "after feature→in review")

	// Illegal transition 3: same-target re-run (exit 4).
	preBytes = snapshotFile(t, featPath)
	_, _, err = runFeature(t, "change-status", "bar", "--to=in review")
	assertExitCode(t, err, exitcode.InvalidState, "feature In Review→In Review re-run")
	if got := snapshotFile(t, featPath); !bytes.Equal(preBytes, got) {
		t.Errorf("feature file mutated by rejected same-target re-run")
	}

	// Step 7: feature change-status bar --to=approved.
	out, _, err = runFeature(t, "change-status", "bar", "--to=approved")
	if err != nil {
		t.Fatalf("step 7: feature change-status approved failed: %v", err)
	}
	if want := "bar: In Review → Approved\n"; out != want {
		t.Errorf("step 7: stdout = %q, want %q", out, want)
	}
	if got := readFileStatus(t, featPath); got != "Approved" {
		t.Errorf("step 7: feature status = %q, want Approved", got)
	}
	if !fileContains(t, featuresIdx, "| [Bar](bar/README.md) | Approved | integration test feature |") {
		t.Errorf("step 7: features index Status cell not synced to Approved")
	}
	assertLintClean(t, root, "after feature→approved")

	// Step 8: feature change-status bar --to=implementing.
	out, _, err = runFeature(t, "change-status", "bar", "--to=implementing")
	if err != nil {
		t.Fatalf("step 8: feature change-status implementing failed: %v", err)
	}
	if want := "bar: Approved → Implementing\n"; out != want {
		t.Errorf("step 8: stdout = %q, want %q", out, want)
	}
	if got := readFileStatus(t, featPath); got != "Implementing" {
		t.Errorf("step 8: feature status = %q, want Implementing", got)
	}
	if !fileContains(t, featuresIdx, "| [Bar](bar/README.md) | Implementing | integration test feature |") {
		t.Errorf("step 8: features index Status cell not synced to Implementing")
	}
	assertLintClean(t, root, "after feature→implementing")

	// Step 9: feature change-status bar --to=stable.
	out, _, err = runFeature(t, "change-status", "bar", "--to=stable")
	if err != nil {
		t.Fatalf("step 9: feature change-status stable failed: %v", err)
	}
	if want := "bar: Implementing → Stable\n"; out != want {
		t.Errorf("step 9: stdout = %q, want %q", out, want)
	}
	if got := readFileStatus(t, featPath); got != "Stable" {
		t.Errorf("step 9: feature status = %q, want Stable", got)
	}
	if !fileContains(t, featuresIdx, "| [Bar](bar/README.md) | Stable | integration test feature |") {
		t.Errorf("step 9: features index Status cell not synced to Stable")
	}
	assertLintClean(t, root, "after feature→stable")

	// Illegal transition 4: reverse (Stable → Approved) → exit 4.
	preBytes = snapshotFile(t, featPath)
	_, _, err = runFeature(t, "change-status", "bar", "--to=approved")
	assertExitCode(t, err, exitcode.InvalidState, "feature Stable→Approved reverse")
	if got := snapshotFile(t, featPath); !bytes.Equal(preBytes, got) {
		t.Errorf("feature file mutated by rejected reverse transition")
	}

	// Step 10: feature change-status bar --to=deprecated.
	out, _, err = runFeature(t, "change-status", "bar", "--to=deprecated")
	if err != nil {
		t.Fatalf("step 10: feature change-status deprecated failed: %v", err)
	}
	if want := "bar: Stable → Deprecated\n"; out != want {
		t.Errorf("step 10: stdout = %q, want %q", out, want)
	}
	if got := readFileStatus(t, featPath); got != "Deprecated" {
		t.Errorf("step 10: feature status = %q, want Deprecated", got)
	}
	if !fileContains(t, featuresIdx, "| [Bar](bar/README.md) | Deprecated | integration test feature |") {
		t.Errorf("step 10: features index Status cell not synced to Deprecated")
	}
	assertLintClean(t, root, "after feature→deprecated")

	// Step 11: final lint pass — explicit confirmation that the
	// tmp tree ends the sequence with 0 error-severity violations.
	assertLintClean(t, root, "final")
}

// TestLifecycleIntegration_HelpListsChangeStatus is the discoverability
// sub-test required by the plan's last acceptance criterion. Walks the
// cobra command tree (no help-text string parsing) and asserts both
// parent command groups expose a `change-status` subcommand.
func TestLifecycleIntegration_HelpListsChangeStatus(t *testing.T) {
	cases := []struct {
		name   string
		parent *cobra.Command
	}{
		{"idea", ideaCommand()},
		{"feature", featureCommand()},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			if !hasChangeStatusSubcommand(tc.parent) {
				var names []string
				for _, c := range tc.parent.Commands() {
					names = append(names, c.Name())
				}
				t.Errorf("%s command missing `change-status` subcommand; got %v",
					tc.name, names)
			}
		})
	}
}

// hasChangeStatusSubcommand returns true when parent has a direct
// subcommand whose Name() is "change-status". cobra splits the Use
// string on whitespace so c.Name() yields the bare verb regardless of
// trailing positional/flag hints.
func hasChangeStatusSubcommand(parent *cobra.Command) bool {
	for _, sub := range parent.Commands() {
		if sub.Name() == "change-status" {
			return true
		}
	}
	return false
}
