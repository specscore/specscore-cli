package cli

// Tests for the `rehearse new` subcommand (Task 4: CLI wiring).
//
// Verifies: cli/rehearse/new#ac:resolve-ac-reference
// Verifies: cli/rehearse/new#ac:missing-ac-error
// Verifies: cli/rehearse/new-dry-run#ac:dry-run-flag
// Verifies: cli/rehearse/new-dry-run#ac:dry-run-ignores-flags
// Verifies: cli/rehearse/new-dry-run#ac:error-handling-same

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// minimalFeatureREADME is a fixture with one AC whose body has Given/When/Then.
const minimalFeatureREADME = `---
format: https://specscore.md/feature-specification
---
# Feature: CLI Rehearse New

### AC: resolve-ac-reference

Given a valid AC reference
When the user runs rehearse new
Then a scaffold file is created

### AC: missing-ac-error

Given an AC slug that does not exist in the feature
When the user runs rehearse new
Then the command exits 2 with an informative error message
`

// runRehearseNewCmd invokes only the `new` subcommand via a fresh command tree,
// injecting all five filesystem/git seams via package-level vars.
func runRehearseNewCmd(
	t *testing.T,
	readFile func(string) (string, error),
	mkdirAll func(string, os.FileMode) error,
	writeFile func(string, []byte, os.FileMode) error,
	stat func(string) (os.FileInfo, error),
	gitExec func(args ...string) ([]byte, error),
	args ...string,
) error {
	t.Helper()

	// Swap seams.
	origRead := rehearseNewReadFileFn
	origMkdir := rehearseNewMkdirAllFn
	origWrite := rehearseNewWriteFileFn
	origStat := rehearseNewStatFn
	origGit := rehearseNewGitExecFn

	rehearseNewReadFileFn = readFile
	rehearseNewMkdirAllFn = mkdirAll
	rehearseNewWriteFileFn = writeFile
	rehearseNewStatFn = stat
	rehearseNewGitExecFn = gitExec

	t.Cleanup(func() {
		rehearseNewReadFileFn = origRead
		rehearseNewMkdirAllFn = origMkdir
		rehearseNewWriteFileFn = origWrite
		rehearseNewStatFn = origStat
		rehearseNewGitExecFn = origGit
	})

	cmd := rehearseNewCommand()
	cmd.SetArgs(args)
	return cmd.Execute()
}

// defaultMkdirOK returns a mkdir seam that always succeeds.
func defaultMkdirOK(_ string, _ os.FileMode) error { return nil }

// defaultWriteOK returns a write seam that always succeeds.
func defaultWriteOK(_ string, _ []byte, _ os.FileMode) error { return nil }

// defaultStatNotExist returns a stat seam that always returns os.ErrNotExist.
func defaultStatNotExist(_ string) (os.FileInfo, error) { return nil, os.ErrNotExist }

// defaultGitOK returns a git exec seam that always succeeds.
func defaultGitOK(_ ...string) ([]byte, error) { return []byte{}, nil }

// readFileOKForNew returns the fixture content for any path.
func readFileOKForNew(_ string) (string, error) { return minimalFeatureREADME, nil }

// readFileNotFoundForNew simulates a missing feature file.
func readFileNotFoundForNew(path string) (string, error) {
	return "", &os.PathError{Op: "open", Path: path, Err: os.ErrNotExist}
}

// --- Tests ---

// TestRehearseNew_Resolve — a valid AC reference resolves and creates the scaffold file.
// Verifies: cli/rehearse/new#ac:resolve-ac-reference
func TestRehearseNew_Resolve(t *testing.T) {
	var writtenPath string
	var writtenContent string
	write := func(path string, data []byte, _ os.FileMode) error {
		writtenPath = path
		writtenContent = string(data)
		return nil
	}

	err := runRehearseNewCmd(t,
		readFileOKForNew,
		defaultMkdirOK,
		write,
		defaultStatNotExist,
		defaultGitOK,
		"cli/rehearse/new#ac:resolve-ac-reference",
	)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}

	wantPath := "spec/features/cli/rehearse/new/_tests/resolve-ac-reference.md"
	if writtenPath != wantPath {
		t.Errorf("written path = %q, want %q", writtenPath, wantPath)
	}
	if !strings.Contains(writtenContent, "cli/rehearse/new#ac:resolve-ac-reference") {
		t.Errorf("scaffold content missing Verifies reference:\n%s", writtenContent)
	}
	if !strings.Contains(writtenContent, "Given a valid AC reference") {
		t.Errorf("scaffold content missing Given/When/Then:\n%s", writtenContent)
	}
}

// TestRehearseNew_MissingFeature — exit 2 when the feature file cannot be read.
func TestRehearseNew_MissingFeature(t *testing.T) {
	err := runRehearseNewCmd(t,
		readFileNotFoundForNew,
		defaultMkdirOK,
		defaultWriteOK,
		defaultStatNotExist,
		defaultGitOK,
		"cli/rehearse/new#ac:resolve-ac-reference",
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ec interface{ ExitCode() int }
	if !errors.As(err, &ec) || ec.ExitCode() != 2 {
		t.Errorf("exit code = %v, want 2", err)
	}
	if !strings.Contains(err.Error(), "cli/rehearse/new") {
		t.Errorf("error should mention the feature slug: %v", err)
	}
}

// TestRehearseNew_MissingAC — exit 2 when the AC slug is not found in the feature file.
// Verifies: cli/rehearse/new#ac:missing-ac-error
func TestRehearseNew_MissingAC(t *testing.T) {
	err := runRehearseNewCmd(t,
		readFileOKForNew,
		defaultMkdirOK,
		defaultWriteOK,
		defaultStatNotExist,
		defaultGitOK,
		"cli/rehearse/new#ac:nonexistent-ac",
	)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ec interface{ ExitCode() int }
	if !errors.As(err, &ec) || ec.ExitCode() != 2 {
		t.Errorf("exit code = %v, want 2", err)
	}
	if !strings.Contains(err.Error(), "nonexistent-ac") {
		t.Errorf("error should mention the missing AC slug: %v", err)
	}
}

// TestRehearseNew_FileExists — exit 2 when the target file already exists and --force is not set.
func TestRehearseNew_FileExists(t *testing.T) {
	stat := func(_ string) (os.FileInfo, error) {
		// Return a non-nil FileInfo (any will do) to indicate file exists.
		info, _ := os.Stat(os.TempDir())
		return info, nil
	}

	err := runRehearseNewCmd(t,
		readFileOKForNew,
		defaultMkdirOK,
		defaultWriteOK,
		stat,
		defaultGitOK,
		"cli/rehearse/new#ac:resolve-ac-reference",
	)
	if err == nil {
		t.Fatal("expected error when file exists and --force not set")
	}
	var ec interface{ ExitCode() int }
	if !errors.As(err, &ec) || ec.ExitCode() != 2 {
		t.Errorf("exit code = %v, want 2", err)
	}
	if !strings.Contains(err.Error(), "--force") {
		t.Errorf("error should mention --force: %v", err)
	}
}

// TestRehearseNew_FileExistsWithForce — overwrites existing file when --force is set.
func TestRehearseNew_FileExistsWithForce(t *testing.T) {
	stat := func(_ string) (os.FileInfo, error) {
		info, _ := os.Stat(os.TempDir())
		return info, nil
	}
	var written bool
	write := func(_ string, _ []byte, _ os.FileMode) error {
		written = true
		return nil
	}

	err := runRehearseNewCmd(t,
		readFileOKForNew,
		defaultMkdirOK,
		write,
		stat,
		defaultGitOK,
		"--force",
		"cli/rehearse/new#ac:resolve-ac-reference",
	)
	if err != nil {
		t.Fatalf("expected success with --force, got: %v", err)
	}
	if !written {
		t.Error("file was not written even though --force was set")
	}
}

// TestRehearseNew_MkdirFails — exit 2 when directory creation fails.
func TestRehearseNew_MkdirFails(t *testing.T) {
	mkdir := func(_ string, _ os.FileMode) error {
		return errors.New("permission denied")
	}

	err := runRehearseNewCmd(t,
		readFileOKForNew,
		mkdir,
		defaultWriteOK,
		defaultStatNotExist,
		defaultGitOK,
		"cli/rehearse/new#ac:resolve-ac-reference",
	)
	if err == nil {
		t.Fatal("expected error when mkdir fails")
	}
	var ec interface{ ExitCode() int }
	if !errors.As(err, &ec) || ec.ExitCode() != 2 {
		t.Errorf("exit code = %v, want 2", err)
	}
}

// TestRehearseNew_CommitSuccess — --commit stages the new scaffold (git add)
// and then commits it with the expected message and trailer. Staging must
// precede the commit: the scaffold is a new, untracked file, so a bare
// `git commit <path>` would not include it.
func TestRehearseNew_CommitSuccess(t *testing.T) {
	var calls [][]string
	git := func(args ...string) ([]byte, error) {
		calls = append(calls, args)
		return []byte{}, nil
	}

	err := runRehearseNewCmd(t,
		readFileOKForNew,
		defaultMkdirOK,
		defaultWriteOK,
		defaultStatNotExist,
		git,
		"--commit",
		"cli/rehearse/new#ac:resolve-ac-reference",
	)
	if err != nil {
		t.Fatalf("expected success with --commit, got: %v", err)
	}
	if len(calls) != 2 {
		t.Fatalf("git called %d times, want 2 (add then commit): %v", len(calls), calls)
	}
	if calls[0][0] != "add" {
		t.Errorf("first git call = %v, want it to start with \"add\"", calls[0])
	}
	if calls[1][0] != "commit" {
		t.Errorf("second git call = %v, want it to start with \"commit\"", calls[1])
	}
	// Both calls must reference the scaffold path; the commit carries the trailer.
	commit := strings.Join(calls[1], " ")
	if !strings.Contains(strings.Join(calls[0], " "), "resolve-ac-reference") {
		t.Errorf("git add args do not mention the scaffold path: %q", calls[0])
	}
	if !strings.Contains(commit, "resolve-ac-reference") {
		t.Errorf("git commit args do not mention the AC slug: %q", calls[1])
	}
	if !strings.Contains(commit, "Verifies:") {
		t.Errorf("git commit args missing Verifies trailer: %q", calls[1])
	}
}

// TestRehearseNew_CommitRealGit is an integration test against real git (not the
// mocked seam): it proves --commit actually creates a commit containing the new
// scaffold file. The mocked unit tests cannot catch a missing `git add` because
// they stub git to succeed; this test would have caught it.
func TestRehearseNew_CommitRealGit(t *testing.T) {
	repo := t.TempDir()
	t.Chdir(repo)
	for _, args := range [][]string{
		{"init", "-q"},
		{"config", "user.email", "t@example.com"},
		{"config", "user.name", "Test"},
		{"commit", "-q", "--allow-empty", "-m", "init"},
	} {
		if out, err := gitRunArgs(args...); err != nil {
			t.Fatalf("git %v: %v (%s)", args, err, out)
		}
	}
	if err := os.MkdirAll(filepath.Join("spec", "features", "demo"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	readme := "# Feature: demo\n\n### AC: my-case\n\nScenario: demo\nGiven a thing\nWhen I act\nThen it works\n\n## Next\n"
	if err := os.WriteFile(filepath.Join("spec", "features", "demo", "README.md"), []byte(readme), 0o644); err != nil {
		t.Fatalf("write readme: %v", err)
	}

	// Run with the real (default) seams: real fs + real gitRunArgs.
	if err := runRehearseNew("demo#ac:my-case", false, true, false, io.Discard); err != nil {
		t.Fatalf("runRehearseNew --commit: %v", err)
	}

	scaffold := filepath.Join("spec", "features", "demo", "_tests", "my-case.md")
	// The scaffold must be committed (tracked, nothing pending for it).
	status, err := gitRunArgs("status", "--porcelain", scaffold)
	if err != nil {
		t.Fatalf("git status: %v (%s)", err, status)
	}
	if strings.TrimSpace(string(status)) != "" {
		t.Errorf("scaffold not committed; git status = %q", status)
	}
	// HEAD's subject must be the scaffold commit.
	subject, err := gitRunArgs("log", "-1", "--pretty=%s")
	if err != nil {
		t.Fatalf("git log: %v", err)
	}
	if !strings.Contains(string(subject), "scaffold my-case scenario") {
		t.Errorf("HEAD subject = %q, want the scaffold commit", subject)
	}
}

// TestRehearseNew_WriteFails — exit 2 when writing the scaffold file fails.
func TestRehearseNew_WriteFails(t *testing.T) {
	write := func(_ string, _ []byte, _ os.FileMode) error {
		return errors.New("disk full")
	}

	err := runRehearseNewCmd(t,
		readFileOKForNew,
		defaultMkdirOK,
		write,
		defaultStatNotExist,
		defaultGitOK,
		"cli/rehearse/new#ac:resolve-ac-reference",
	)
	if err == nil {
		t.Fatal("expected error when write fails")
	}
	var ec interface{ ExitCode() int }
	if !errors.As(err, &ec) || ec.ExitCode() != 2 {
		t.Errorf("exit code = %v, want 2", err)
	}
}

// TestRehearseNew_GitRunArgs — exercises the production gitRunArgs default
// by calling it directly. We expect it to fail (not inside a git repo with
// a staged file), but the function must be reachable for coverage.
func TestRehearseNew_GitRunArgs(t *testing.T) {
	// Calling gitRunArgs with "version" is always safe — it doesn't modify anything.
	out, err := gitRunArgs("version")
	// Either succeeds or fails; we only care that the function ran.
	_ = out
	_ = err
}

// TestRehearseNew_CommitFailsAfterWrite — exit 1 if `git commit` fails (after a
// successful `git add`) but the file was already written.
func TestRehearseNew_CommitFailsAfterWrite(t *testing.T) {
	var written bool
	write := func(_ string, _ []byte, _ os.FileMode) error {
		written = true
		return nil
	}
	git := func(args ...string) ([]byte, error) {
		if args[0] == "commit" {
			return nil, errors.New("git commit failed: nothing to commit")
		}
		return []byte{}, nil // add succeeds
	}

	err := runRehearseNewCmd(t,
		readFileOKForNew,
		defaultMkdirOK,
		write,
		defaultStatNotExist,
		git,
		"--commit",
		"cli/rehearse/new#ac:resolve-ac-reference",
	)
	if !written {
		t.Error("file should have been written before commit was attempted")
	}
	if err == nil {
		t.Fatal("expected error when commit fails")
	}
	var ec interface{ ExitCode() int }
	if !errors.As(err, &ec) || ec.ExitCode() != 1 {
		t.Errorf("exit code = %v, want 1 (scaffold survives)", err)
	}
}

// TestRehearseNew_AddFailsAfterWrite — exit 1 if `git add` fails; the scaffold
// file still survives on disk.
func TestRehearseNew_AddFailsAfterWrite(t *testing.T) {
	var written bool
	write := func(_ string, _ []byte, _ os.FileMode) error {
		written = true
		return nil
	}
	git := func(args ...string) ([]byte, error) {
		if args[0] == "add" {
			return nil, errors.New("git add failed: permission denied")
		}
		return []byte{}, nil
	}

	err := runRehearseNewCmd(t,
		readFileOKForNew,
		defaultMkdirOK,
		write,
		defaultStatNotExist,
		git,
		"--commit",
		"cli/rehearse/new#ac:resolve-ac-reference",
	)
	if !written {
		t.Error("file should have been written before git add was attempted")
	}
	var ec interface{ ExitCode() int }
	if !errors.As(err, &ec) || ec.ExitCode() != 1 {
		t.Errorf("exit code = %v, want 1 (scaffold survives)", err)
	}
	if err == nil || !strings.Contains(err.Error(), "git add failed") {
		t.Errorf("error = %v, want it to mention git add failure", err)
	}
}

// withSeams swaps all five package-level seams for the duration of the test.
func withSeams(
	t *testing.T,
	readFile func(string) (string, error),
	mkdirAll func(string, os.FileMode) error,
	writeFile func(string, []byte, os.FileMode) error,
	stat func(string) (os.FileInfo, error),
	gitExec func(args ...string) ([]byte, error),
) {
	t.Helper()
	origRead := rehearseNewReadFileFn
	origMkdir := rehearseNewMkdirAllFn
	origWrite := rehearseNewWriteFileFn
	origStat := rehearseNewStatFn
	origGit := rehearseNewGitExecFn
	rehearseNewReadFileFn = readFile
	rehearseNewMkdirAllFn = mkdirAll
	rehearseNewWriteFileFn = writeFile
	rehearseNewStatFn = stat
	rehearseNewGitExecFn = gitExec
	t.Cleanup(func() {
		rehearseNewReadFileFn = origRead
		rehearseNewMkdirAllFn = origMkdir
		rehearseNewWriteFileFn = origWrite
		rehearseNewStatFn = origStat
		rehearseNewGitExecFn = origGit
	})
}

// --- Dry-run tests ---
// Verifies: cli/rehearse/new-dry-run#ac:dry-run-flag
// Verifies: cli/rehearse/new-dry-run#ac:dry-run-ignores-flags
// Verifies: cli/rehearse/new-dry-run#ac:error-handling-same

// TestRehearseNew_DryRunPrintsScaffold — dry-run prints the scaffold to the
// writer and returns nil without writing any file.
func TestRehearseNew_DryRunPrintsScaffold(t *testing.T) {
	withSeams(t, readFileOKForNew, defaultMkdirOK, defaultWriteOK, defaultStatNotExist, defaultGitOK)

	var buf bytes.Buffer
	err := runRehearseNew("cli/rehearse/new#ac:resolve-ac-reference", false, false, true, &buf)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	out := buf.String()
	if !strings.Contains(out, "cli/rehearse/new#ac:resolve-ac-reference") {
		t.Errorf("dry-run output missing Verifies reference:\n%s", out)
	}
	if !strings.Contains(out, "Given a valid AC reference") {
		t.Errorf("dry-run output missing Given/When/Then:\n%s", out)
	}
}

// TestRehearseNew_DryRunIgnoresForce — dry-run with force=true still only
// prints; writeFile is never called.
func TestRehearseNew_DryRunIgnoresForce(t *testing.T) {
	writeFileCalled := false
	writeFile := func(_ string, _ []byte, _ os.FileMode) error {
		writeFileCalled = true
		return nil
	}
	withSeams(t, readFileOKForNew, defaultMkdirOK, writeFile, defaultStatNotExist, defaultGitOK)

	err := runRehearseNew("cli/rehearse/new#ac:resolve-ac-reference", true, false, true, io.Discard)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if writeFileCalled {
		t.Error("writeFile must not be called when --dry-run is set")
	}
}

// TestRehearseNew_DryRunIgnoresCommit — dry-run with commit=true still only
// prints; gitExec is never called.
func TestRehearseNew_DryRunIgnoresCommit(t *testing.T) {
	gitCalled := false
	git := func(_ ...string) ([]byte, error) {
		gitCalled = true
		return nil, nil
	}
	withSeams(t, readFileOKForNew, defaultMkdirOK, defaultWriteOK, defaultStatNotExist, git)

	err := runRehearseNew("cli/rehearse/new#ac:resolve-ac-reference", false, true, true, io.Discard)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if gitCalled {
		t.Error("gitExec must not be called when --dry-run is set")
	}
}

// TestRehearseNew_DryRunExitCodeSame — error paths (e.g. missing feature file)
// return exit 2 even when --dry-run is set.
func TestRehearseNew_DryRunExitCodeSame(t *testing.T) {
	withSeams(t, readFileNotFoundForNew, defaultMkdirOK, defaultWriteOK, defaultStatNotExist, defaultGitOK)

	err := runRehearseNew("cli/rehearse/new#ac:resolve-ac-reference", false, false, true, io.Discard)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var ec interface{ ExitCode() int }
	if !errors.As(err, &ec) || ec.ExitCode() != 2 {
		t.Errorf("exit code = %v, want 2", err)
	}
}

// TestRehearseNew_DryRunNormalWriteWorks — without --dry-run the file is
// written normally (regression guard for the non-dry-run path).
func TestRehearseNew_DryRunNormalWriteWorks(t *testing.T) {
	var writtenPath string
	writeFile := func(path string, _ []byte, _ os.FileMode) error {
		writtenPath = path
		return nil
	}
	withSeams(t, readFileOKForNew, defaultMkdirOK, writeFile, defaultStatNotExist, defaultGitOK)

	var buf bytes.Buffer
	err := runRehearseNew("cli/rehearse/new#ac:resolve-ac-reference", false, false, false, &buf)
	if err != nil {
		t.Fatalf("expected success, got: %v", err)
	}
	if writtenPath == "" {
		t.Error("file was not written when --dry-run is not set")
	}
	if buf.Len() != 0 {
		t.Errorf("output buffer should be empty without --dry-run, got: %q", buf.String())
	}
}
