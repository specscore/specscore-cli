package runner

// Feature: cli/rehearse/evidence (REQ: report-out, REQ: report-provenance)

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// stubExecCommand returns a fake ExecCommandFn that responds to git subcommands
// with canned outputs. Set sha="" to simulate "not a git repo".
func stubExecCommand(t *testing.T, sha string, dirty bool) {
	t.Helper()
	restore := ExecCommandFn
	ExecCommandFn = func(dir string, name string, args ...string) ([]byte, error) {
		if name != "git" {
			return nil, fmt.Errorf("unexpected command: %s", name)
		}
		switch args[0] {
		case "rev-parse":
			if args[1] == "HEAD" {
				if sha == "" {
					return nil, errors.New("not a git repository")
				}
				return []byte(sha + "\n"), nil
			}
			if args[1] == "--show-toplevel" {
				if sha == "" {
					return nil, errors.New("not a git repository")
				}
				return []byte(dir + "\n"), nil
			}
		case "status":
			if sha == "" {
				return nil, errors.New("not a git repository")
			}
			if dirty {
				return []byte(" M some/file.go\n"), nil
			}
			return []byte(""), nil
		}
		return nil, fmt.Errorf("unexpected git args: %v", args)
	}
	t.Cleanup(func() { ExecCommandFn = restore })
}

// TestGitProvenance_InsideCleanGitRepo checks SHA and dirty=false.
func TestGitProvenance_InsideCleanGitRepo(t *testing.T) {
	stubExecCommand(t, "abc1234", false)
	sha, dirty := GitProvenance(t.TempDir())
	if sha != "abc1234" {
		t.Errorf("sha = %q, want abc1234", sha)
	}
	if dirty {
		t.Error("dirty = true, want false")
	}
}

// TestGitProvenance_DirtyRepo checks dirty=true when status output is non-empty.
func TestGitProvenance_DirtyRepo(t *testing.T) {
	stubExecCommand(t, "abc1234", true)
	_, dirty := GitProvenance(t.TempDir())
	if !dirty {
		t.Error("dirty = false, want true")
	}
}

// TestGitProvenance_OutsideGitRepo: SHA empty, dirty false.
func TestGitProvenance_OutsideGitRepo(t *testing.T) {
	stubExecCommand(t, "", false)
	sha, dirty := GitProvenance(t.TempDir())
	if sha != "" {
		t.Errorf("sha = %q, want empty", sha)
	}
	if dirty {
		t.Error("dirty = true, want false")
	}
}

// TestGitProvenance_StatusErrorAfterSHASucceeds: status fails but SHA was
// obtained — treat as clean (not dirty).
func TestGitProvenance_StatusErrorAfterSHASucceeds(t *testing.T) {
	restore := ExecCommandFn
	ExecCommandFn = func(dir string, name string, args ...string) ([]byte, error) {
		if args[0] == "rev-parse" && args[1] == "HEAD" {
			return []byte("deadbeef\n"), nil
		}
		if args[0] == "rev-parse" && args[1] == "--show-toplevel" {
			return []byte(dir + "\n"), nil
		}
		// status fails
		return nil, errors.New("status error")
	}
	t.Cleanup(func() { ExecCommandFn = restore })

	sha, dirty := GitProvenance(t.TempDir())
	if sha != "deadbeef" {
		t.Errorf("sha = %q, want deadbeef", sha)
	}
	if dirty {
		t.Error("dirty = true after status error, want false")
	}
}

// TestGitProvenance_EmptySHAOutput: rev-parse returns empty string → treat as outside git.
func TestGitProvenance_EmptySHAOutput(t *testing.T) {
	restore := ExecCommandFn
	ExecCommandFn = func(dir string, name string, args ...string) ([]byte, error) {
		if args[0] == "rev-parse" && args[1] == "HEAD" {
			return []byte("   \n"), nil // whitespace only
		}
		return nil, errors.New("unexpected")
	}
	t.Cleanup(func() { ExecCommandFn = restore })

	sha, dirty := GitProvenance(t.TempDir())
	if sha != "" {
		t.Errorf("sha = %q, want empty", sha)
	}
	if dirty {
		t.Error("dirty = true, want false")
	}
}

func TestNowFnReturnsCurrentTime(t *testing.T) {
	before := time.Now()
	got := NowFn()
	after := time.Now()
	if got.Before(before) || got.After(after) {
		t.Fatalf("NowFn() = %s, want a time between %s and %s", got, before, after)
	}
}

// TestWriteReport_CreatesFileWithEnvelope verifies the JSON envelope fields.
func TestWriteReport_CreatesFileWithEnvelope(t *testing.T) {
	stubExecCommand(t, "abc1234", false)
	dir := t.TempDir()
	outPath := filepath.Join(dir, "out", "report.json")
	started := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)

	reports := []ScenarioReport{
		{
			File:       "/some/repo/spec/features/x/_tests/s.md",
			Status:     StatusPass,
			Verifies:   []string{"x#ac:one"},
			DurationMS: 42,
			Bag:        map[string]string{},
			Steps:      []StepReport{{Kind: "bash", Status: StatusPass}},
		},
	}

	if err := WriteReport(outPath, "v1.2.3", started, reports, dir); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var env RunReport
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.RunnerVersion != "v1.2.3" {
		t.Errorf("runner_version = %q, want v1.2.3", env.RunnerVersion)
	}
	if env.GitSHA != "abc1234" {
		t.Errorf("git_sha = %q, want abc1234", env.GitSHA)
	}
	if env.GitDirty {
		t.Error("git_dirty = true, want false")
	}
	if env.StartedAt != "2026-01-02T03:04:05Z" {
		t.Errorf("started_at = %q, want 2026-01-02T03:04:05Z", env.StartedAt)
	}
	if len(env.Scenarios) != 1 {
		t.Fatalf("scenarios len = %d, want 1", len(env.Scenarios))
	}
}

// TestWriteReport_OutsideGitSHAEmpty verifies empty sha / dirty false outside
// a git work tree.
func TestWriteReport_OutsideGitSHAEmpty(t *testing.T) {
	stubExecCommand(t, "", false)
	dir := t.TempDir()
	outPath := filepath.Join(dir, "report.json")
	started := time.Now()

	if err := WriteReport(outPath, "dev", started, []ScenarioReport{}, dir); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}

	data, err := os.ReadFile(outPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	var env RunReport
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if env.GitSHA != "" {
		t.Errorf("git_sha = %q, want empty", env.GitSHA)
	}
	if env.GitDirty {
		t.Error("git_dirty = true, want false")
	}
}

// TestWriteReport_ScenariosAreRepoRelative: inside a git work tree, scenario
// file paths are made relative to the repo root.
func TestWriteReport_ScenariosAreRepoRelative(t *testing.T) {
	dir := t.TempDir()
	repoRoot := dir

	restore := ExecCommandFn
	ExecCommandFn = func(d string, name string, args ...string) ([]byte, error) {
		switch {
		case args[0] == "rev-parse" && args[1] == "HEAD":
			return []byte("abc1234\n"), nil
		case args[0] == "rev-parse" && args[1] == "--show-toplevel":
			return []byte(repoRoot + "\n"), nil
		case args[0] == "status":
			return []byte(""), nil
		}
		return nil, fmt.Errorf("unexpected: %v", args)
	}
	t.Cleanup(func() { ExecCommandFn = restore })

	absFile := filepath.Join(repoRoot, "spec", "features", "x", "_tests", "s.md")
	reports := []ScenarioReport{
		{File: absFile, Status: StatusPass, Verifies: []string{}, Bag: map[string]string{}, Steps: []StepReport{}},
	}

	outPath := filepath.Join(dir, "report.json")
	if err := WriteReport(outPath, "dev", time.Now(), reports, dir); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}

	data, _ := os.ReadFile(outPath)
	var env RunReport
	_ = json.Unmarshal(data, &env)

	wantFile := filepath.Join("spec", "features", "x", "_tests", "s.md")
	if env.Scenarios[0].File != wantFile {
		t.Errorf("scenario file = %q, want %q", env.Scenarios[0].File, wantFile)
	}
}

// TestWriteReport_JSONMarshalError: jsonMarshalIndentFn failure is propagated.
func TestWriteReport_JSONMarshalError(t *testing.T) {
	stubExecCommand(t, "abc", false)
	swap(t, &jsonMarshalIndentFn, func(v any, prefix, indent string) ([]byte, error) {
		return nil, errors.New("marshal failed")
	})

	err := WriteReport("/some/path/report.json", "dev", time.Now(), nil, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "marshal failed") {
		t.Errorf("expected marshal error, got %v", err)
	}
}

// TestWriteReport_MkdirAllError: osMkdirAllFn failure is propagated.
func TestWriteReport_MkdirAllError(t *testing.T) {
	stubExecCommand(t, "abc", false)
	swap(t, &osMkdirAllFn, func(path string, perm os.FileMode) error {
		return errors.New("mkdir failed")
	})

	err := WriteReport("/some/path/report.json", "dev", time.Now(), nil, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "mkdir failed") {
		t.Errorf("expected mkdir error, got %v", err)
	}
}

// TestWriteReport_WriteFileError: osWriteFileFn failure is propagated.
func TestWriteReport_WriteFileError(t *testing.T) {
	stubExecCommand(t, "abc", false)
	swap(t, &osMkdirAllFn, func(path string, perm os.FileMode) error { return nil })
	swap(t, &osWriteFileFn, func(path string, data []byte, perm os.FileMode) error {
		return errors.New("disk full")
	})

	err := WriteReport("/some/report.json", "dev", time.Now(), nil, t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "disk full") {
		t.Errorf("expected write error, got %v", err)
	}
}

// TestRelativeScenarioPaths_NoRoot: when repoRoot is empty, paths are unchanged.
func TestRelativeScenarioPaths_NoRoot(t *testing.T) {
	reports := []ScenarioReport{{File: "/abs/path/s.md"}}
	out := relativeScenarioPaths(reports, "")
	if out[0].File != "/abs/path/s.md" {
		t.Errorf("file = %q, want /abs/path/s.md", out[0].File)
	}
}

// TestRepoRootForDir_OutsideGit: returns empty string outside a git repo.
func TestRepoRootForDir_OutsideGit(t *testing.T) {
	stubExecCommand(t, "", false)
	root := repoRootForDir(t.TempDir())
	if root != "" {
		t.Errorf("root = %q, want empty", root)
	}
}

// TestExecCommandOutput_RunsRealCommand exercises the production exec wrapper
// (the function behind ExecCommandFn) by running "echo" through the real
// exec.Command path.
func TestExecCommandOutput_RunsRealCommand(t *testing.T) {
	out, err := execCommandOutput(t.TempDir(), "echo", "hello-from-exec")
	if err != nil {
		t.Fatalf("execCommandOutput: %v", err)
	}
	if !strings.Contains(string(out), "hello-from-exec") {
		t.Errorf("output = %q, want to contain hello-from-exec", out)
	}
}

// TestExecCommandOutput_FailingCommand exercises the error path of the
// production exec wrapper.
func TestExecCommandOutput_FailingCommand(t *testing.T) {
	_, err := execCommandOutput(t.TempDir(), "false")
	if err == nil {
		t.Fatal("expected error from false command")
	}
}
