package runner

// Feature: cli/rehearse/evidence (REQ: report-out, REQ: report-provenance)

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// RunReport is the persisted report envelope written by --report-out.
// It carries run provenance alongside the scenario results.
// Feature: cli/rehearse/evidence (REQ: report-provenance)
type RunReport struct {
	RunnerVersion string           `json:"runner_version"`
	GitSHA        string           `json:"git_sha"`
	GitDirty      bool             `json:"git_dirty"`
	StartedAt     string           `json:"started_at"`
	Scenarios     []ScenarioReport `json:"scenarios"`
}

// GitProvenance collects HEAD SHA and dirty flag from the working tree at dir.
// Outside a git work tree (or when git is unavailable) it returns empty SHA and
// dirty=false — provenance is honest, never invented.
// Feature: cli/rehearse/evidence (REQ: report-provenance)
func GitProvenance(dir string) (sha string, dirty bool) {
	// HEAD SHA
	shaOut, err := ExecCommandFn(dir, "git", "rev-parse", "HEAD")
	if err != nil {
		return "", false
	}
	sha = strings.TrimSpace(string(shaOut))
	if sha == "" {
		return "", false
	}

	// Dirty flag: "git status --porcelain" is empty when clean.
	statusOut, err := ExecCommandFn(dir, "git", "status", "--porcelain")
	if err != nil {
		// git is available (rev-parse succeeded) but status failed — treat as clean.
		return sha, false
	}
	dirty = len(strings.TrimSpace(string(statusOut))) > 0
	return sha, dirty
}

// repoRootForDir returns the git work tree root for dir, or "" if dir is not
// inside a git work tree.
func repoRootForDir(dir string) string {
	out, err := ExecCommandFn(dir, "git", "rev-parse", "--show-toplevel")
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

// relativeScenarioPaths returns reports with file paths made relative to
// repoRoot. Paths that cannot be made relative are kept as-is.
func relativeScenarioPaths(reports []ScenarioReport, repoRoot string) []ScenarioReport {
	if repoRoot == "" {
		return reports
	}
	out := make([]ScenarioReport, len(reports))
	copy(out, reports)
	for i, r := range out {
		rel, err := filepath.Rel(repoRoot, r.File)
		if err == nil {
			out[i].File = rel
		}
	}
	return out
}

// WriteReport persists the run report envelope to path, creating parent
// directories as needed. The scenarios' file paths are made repo-root-relative
// when dir is inside a git work tree (the stdout report keeps its paths as-is).
//
// Feature: cli/rehearse/evidence (REQ: report-out, REQ: report-provenance)
func WriteReport(path string, runnerVersion string, startedAt time.Time, reports []ScenarioReport, dir string) error {
	sha, dirty := GitProvenance(dir)
	repoRoot := repoRootForDir(dir)
	scenarios := relativeScenarioPaths(reports, repoRoot)

	envelope := RunReport{
		RunnerVersion: runnerVersion,
		GitSHA:        sha,
		GitDirty:      dirty,
		StartedAt:     startedAt.UTC().Format(time.RFC3339),
		Scenarios:     scenarios,
	}

	data, err := jsonMarshalIndentFn(envelope, "", "  ")
	if err != nil {
		return err
	}
	// Append a trailing newline so the file is terminated like standard JSON output.
	data = append(data, '\n')

	if err := osMkdirAllFn(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return osWriteFileFn(path, data, 0o644)
}

// execCommandOutput runs a command in dir and returns its combined stdout.
// This is the production implementation behind ExecCommandFn.
func execCommandOutput(dir string, name string, args ...string) ([]byte, error) {
	cmd := execNewCommandFn(name, args...)
	cmd.Dir = dir
	return cmd.Output()
}

// NowFn is the test seam for time.Now. The CLI calls this to capture the
// run start time before dispatching to Run, so tests can control started_at.
// Feature: cli/rehearse/evidence (REQ: report-provenance)
var NowFn = func() time.Time { return time.Now() }

// jsonMarshalIndentFn is the test seam for json.MarshalIndent, used to cover
// the encoding-error path in WriteReport.
var jsonMarshalIndentFn = func(v any, prefix, indent string) ([]byte, error) {
	return json.MarshalIndent(v, prefix, indent)
}

// osMkdirAllFn is the test seam for os.MkdirAll.
var osMkdirAllFn = func(path string, perm os.FileMode) error {
	return os.MkdirAll(path, perm)
}

// osWriteFileFn is the test seam for os.WriteFile.
var osWriteFileFn = func(path string, data []byte, perm os.FileMode) error {
	return os.WriteFile(path, data, perm)
}
