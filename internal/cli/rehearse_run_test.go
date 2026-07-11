package cli

// Tests for rehearse run --filter flag (Task 1: v0.6 run-filter).
//
// Verifies: cli/rehearse/run-filter#ac:filter-flag-syntax
// Verifies: cli/rehearse/run-filter#ac:filter-matching-exact
// Verifies: cli/rehearse/run-filter#ac:filter-multiple-or
// Verifies: cli/rehearse/run-filter#ac:no-filter-default
// Verifies: cli/rehearse/run-filter#ac:filter-invalid-syntax
// Verifies: cli/rehearse/run-filter#ac:filter-output-labels
// Verifies: cli/rehearse/run-filter#ac:filter-no-matches

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// writeFilterScenario writes a scenario whose **Verifies:** is the given
// ac-ref and whose single bash block runs a command. Returns the file path.
func writeFilterScenario(t *testing.T, acRef, bashCmd string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "scenario.md")
	content := "# Rehearse: filter fixture\n\n**Status:** pending\n**Verifies:** " + acRef + "\n\n```bash\n" + bashCmd + "\n```\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestRehearseRun_FilterFlagSyntax verifies that the --filter flag accepts a
// valid AC reference without error.
//
// Verifies: cli/rehearse/run-filter#ac:filter-flag-syntax
func TestRehearseRun_FilterFlagSyntax(t *testing.T) {
	file := writeFilterScenario(t, "cli/studio/index#ac:index-two-repos", "echo hello")

	out, _, err := runRehearseCmd(t, "run", file, "--filter", "cli/studio/index#ac:index-two-repos")
	if err != nil {
		t.Fatalf("--filter with valid AC reference should exit 0: %v", err)
	}
	if !strings.Contains(out, "filter-match") {
		t.Errorf("output should contain [filter-match] label:\n%s", out)
	}
}

// TestRehearseRun_FilterMatching verifies that with --filter, a scenario whose
// Verifies matches the filter is included (and labeled filter-match), while a
// non-matching scenario is skipped (labeled filter-skip).
//
// Verifies: cli/rehearse/run-filter#ac:filter-matching-exact
func TestRehearseRun_FilterMatching(t *testing.T) {
	matched := writeFilterScenario(t, "feat/x#ac:one", "echo matched")
	skipped := writeFilterScenario(t, "feat/x#ac:two", "echo skipped")

	out, _, err := runRehearseCmd(t, "run", matched, skipped, "--filter", "feat/x#ac:one")
	if err != nil {
		t.Fatalf("run should exit 0 when matched scenario passes: %v", err)
	}
	if !strings.Contains(out, "[filter-match]") {
		t.Errorf("output should contain [filter-match]:\n%s", out)
	}
	if !strings.Contains(out, "[filter-skip]") {
		t.Errorf("output should contain [filter-skip]:\n%s", out)
	}
	// Matched scenario should be marked pass, not the skipped one.
	if strings.Contains(out, "No scenarios matched") {
		t.Errorf("should not emit no-match message when there is a match:\n%s", out)
	}
}

// TestRehearseRun_FilterMultipleOR verifies that multiple --filter values use
// OR semantics: a scenario matching ANY filter is included.
//
// Verifies: cli/rehearse/run-filter#ac:filter-multiple-or
func TestRehearseRun_FilterMultipleOR(t *testing.T) {
	fileA := writeFilterScenario(t, "feat/x#ac:alpha", "echo alpha")
	fileB := writeFilterScenario(t, "feat/x#ac:beta", "echo beta")
	fileC := writeFilterScenario(t, "feat/x#ac:gamma", "echo gamma")

	out, _, err := runRehearseCmd(t, "run", fileA, fileB, fileC,
		"--filter", "feat/x#ac:alpha",
		"--filter", "feat/x#ac:beta",
	)
	if err != nil {
		t.Fatalf("run should exit 0 when matched scenarios pass: %v", err)
	}
	// fileA and fileB should match; fileC should be skipped
	matchCount := strings.Count(out, "[filter-match]")
	skipCount := strings.Count(out, "[filter-skip]")
	if matchCount != 2 {
		t.Errorf("[filter-match] count = %d, want 2:\n%s", matchCount, out)
	}
	if skipCount != 1 {
		t.Errorf("[filter-skip] count = %d, want 1:\n%s", skipCount, out)
	}
}

// TestRehearseRun_FilterNoFilters verifies that without --filter all scenarios
// run normally (no filter-match or filter-skip labels).
//
// Verifies: cli/rehearse/run-filter#ac:no-filter-default
func TestRehearseRun_FilterNoFilters(t *testing.T) {
	fileA := writeFilterScenario(t, "feat/x#ac:one", "echo one")
	fileB := writeFilterScenario(t, "feat/x#ac:two", "echo two")

	out, _, err := runRehearseCmd(t, "run", fileA, fileB)
	if err != nil {
		t.Fatalf("run without --filter should exit 0: %v", err)
	}
	if strings.Contains(out, "filter-match") || strings.Contains(out, "filter-skip") {
		t.Errorf("output should not contain filter labels when no --filter flag is given:\n%s", out)
	}
	if !strings.Contains(out, "2 pass") {
		t.Errorf("both scenarios should run and pass:\n%s", out)
	}
}

// TestRehearseRun_FilterInvalidFormat verifies that an AC reference missing
// #ac: exits 2 with an informative error message.
//
// Verifies: cli/rehearse/run-filter#ac:filter-invalid-syntax
func TestRehearseRun_FilterInvalidFormat(t *testing.T) {
	file := writeFilterScenario(t, "feat/x#ac:one", "echo hello")

	tests := []struct {
		name   string
		filter string
	}{
		{"no-hash-ac", "feat/x:one"},
		{"empty-before", "#ac:one"},
		{"empty-after", "feat/x#ac:"},
		{"bare-slug", "one"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := runRehearseCmd(t, "run", file, "--filter", tc.filter)
			if code := rehearseExit(t, err); code != 2 {
				t.Errorf("filter=%q: exit code = %d, want 2", tc.filter, code)
			}
			if !strings.Contains(err.Error(), "Invalid AC reference") {
				t.Errorf("filter=%q: error %q should contain 'Invalid AC reference'", tc.filter, err.Error())
			}
		})
	}
}

// TestRehearseRun_FilterOutputLabels verifies the exact label format: matched
// scenarios show [filter-match] prefix and skipped scenarios show [filter-skip]
// prefix in human output; JSON adds filter_status field.
//
// Verifies: cli/rehearse/run-filter#ac:filter-output-labels
func TestRehearseRun_FilterOutputLabels(t *testing.T) {
	matched := writeFilterScenario(t, "feat/x#ac:one", "echo hello")
	skipped := writeFilterScenario(t, "feat/x#ac:two", "echo world")

	// Human output labels.
	out, _, err := runRehearseCmd(t, "run", matched, skipped, "--filter", "feat/x#ac:one")
	if err != nil {
		t.Fatalf("run should exit 0: %v", err)
	}
	if !strings.Contains(out, "[filter-match]") {
		t.Errorf("human output missing [filter-match]:\n%s", out)
	}
	if !strings.Contains(out, "[filter-skip]") {
		t.Errorf("human output missing [filter-skip]:\n%s", out)
	}

	// JSON output filter_status field.
	jsonOut, _, err := runRehearseCmd(t, "run", matched, skipped, "--filter", "feat/x#ac:one", "--format", "json")
	if err != nil {
		t.Fatalf("run --format json should exit 0: %v", err)
	}
	var reports []struct {
		FilterStatus string `json:"filter_status"`
	}
	if err := json.Unmarshal([]byte(jsonOut), &reports); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, jsonOut)
	}
	if len(reports) != 2 {
		t.Fatalf("reports len = %d, want 2", len(reports))
	}
	// Find the match and skip entries by filter_status.
	statuses := map[string]int{}
	for _, r := range reports {
		statuses[r.FilterStatus]++
	}
	if statuses["match"] != 1 {
		t.Errorf("filter_status=match count = %d, want 1; reports = %+v", statuses["match"], reports)
	}
	if statuses["skip"] != 1 {
		t.Errorf("filter_status=skip count = %d, want 1; reports = %+v", statuses["skip"], reports)
	}
}

// TestRehearseRun_FilterNoMatches verifies that when no scenarios match the
// filter, the command exits 0 and prints "No scenarios matched filter(s): ..."
//
// Verifies: cli/rehearse/run-filter#ac:filter-no-matches
func TestRehearseRun_FilterNoMatches(t *testing.T) {
	file := writeFilterScenario(t, "feat/x#ac:one", "echo hello")

	out, _, err := runRehearseCmd(t, "run", file, "--filter", "feat/x#ac:does-not-exist")
	if err != nil {
		t.Fatalf("zero-match filter should exit 0: %v", err)
	}
	if !strings.Contains(out, "No scenarios matched filter(s):") {
		t.Errorf("output should contain 'No scenarios matched filter(s):':\n%s", out)
	}
	if !strings.Contains(out, "feat/x#ac:does-not-exist") {
		t.Errorf("output should list the unmatched filter:\n%s", out)
	}
}

// TestRehearseRun_FilterFailingScenario verifies that a matched scenario that
// fails is reported with [filter-match] label and the failure detail is shown
// (exercising the renderHumanWithFilter fail-detail and writeFilterIndented paths).
//
// Verifies: cli/rehearse/run-filter#ac:filter-output-labels
func TestRehearseRun_FilterFailingScenario(t *testing.T) {
	matched := writeFilterScenario(t, "feat/x#ac:one", "exit 7")
	skipped := writeFilterScenario(t, "feat/x#ac:two", "echo world")

	out, _, err := runRehearseCmd(t, "run", matched, skipped, "--filter", "feat/x#ac:one")
	if code := rehearseExit(t, err); code != 1 {
		t.Fatalf("exit code = %d, want 1: err=%v\n%s", code, err, out)
	}
	if !strings.Contains(out, "[filter-match]") {
		t.Errorf("output should contain [filter-match] for the failing matched scenario:\n%s", out)
	}
	if !strings.Contains(out, "[filter-skip]") {
		t.Errorf("output should contain [filter-skip] for the non-matching scenario:\n%s", out)
	}
	// The fail detail for exit 7 should appear.
	if !strings.Contains(out, "exit status 7") {
		t.Errorf("output should contain step failure detail:\n%s", out)
	}
}

// TestRehearseRun_FilterUnparsableScenario verifies that a scenario file that
// fails to parse is treated as non-matching and becomes a filter-skip (exercising
// the scenarioMatchesFilters parse-error path).
//
// Verifies: cli/rehearse/run-filter#ac:filter-matching-exact
func TestRehearseRun_FilterUnparsableScenario(t *testing.T) {
	// A file with an unclosed fence is unparsable.
	dir := t.TempDir()
	bad := dir + "/bad.md"
	if err := os.WriteFile(bad, []byte("**Verifies:** feat/x#ac:one\n\n```bash\nunclosed\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	good := writeFilterScenario(t, "feat/x#ac:two", "echo good")

	out, _, err := runRehearseCmd(t, "run", bad, good, "--filter", "feat/x#ac:two")
	if err != nil {
		t.Fatalf("run should exit 0 when matched scenario passes: %v", err)
	}
	if !strings.Contains(out, "[filter-match]") {
		t.Errorf("output should contain [filter-match]:\n%s", out)
	}
	// The unparsable file should be filter-skipped.
	if !strings.Contains(out, "[filter-skip]") {
		t.Errorf("output should contain [filter-skip] for the unparsable scenario:\n%s", out)
	}
}

// writeMultiStepFilterScenario writes a scenario with two bash steps: first passes,
// second fails. This exercises the step-loop `continue` branch in renderHumanWithFilter
// (s.Status != StatusFail → continue).
func writeMultiStepFilterScenario(t *testing.T, acRef string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "scenario.md")
	content := "# Rehearse: multi-step fixture\n\n**Status:** pending\n**Verifies:** " + acRef + "\n\n" +
		"```bash\necho pass-step\n```\n\n```bash\nexit 3\n```\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// TestRehearseRun_FilterFailingScenarioWithPassingStep verifies that a
// filter-matched scenario with a passing step followed by a failing step
// only shows the failing step's detail (exercising s.Status != StatusFail
// → continue branch in renderHumanWithFilter).
//
// Verifies: cli/rehearse/run-filter#ac:filter-output-labels
func TestRehearseRun_FilterFailingScenarioWithPassingStep(t *testing.T) {
	file := writeMultiStepFilterScenario(t, "feat/x#ac:multi")

	out, _, err := runRehearseCmd(t, "run", file, "--filter", "feat/x#ac:multi")
	if code := rehearseExit(t, err); code != 1 {
		t.Fatalf("exit code = %d, want 1: err=%v\n%s", code, err, out)
	}
	if !strings.Contains(out, "[filter-match]") {
		t.Errorf("output should contain [filter-match]:\n%s", out)
	}
	if !strings.Contains(out, "exit status 3") {
		t.Errorf("output should contain step failure detail:\n%s", out)
	}
}

// writeFilterScenarioWithRunnerSkip writes a scenario matching a given filter that
// contains a hurl block, so with hurl absent from PATH the runner skips it with
// a detail message — exercising the r.Detail != "" branch inside
// renderHumanWithFilter for a filter-match scenario with StatusSkipped.
//
// Verifies: cli/rehearse/run-filter#ac:filter-output-labels
func TestRehearseRun_FilterMatchedRunnerSkipped(t *testing.T) {
	t.Setenv("PATH", t.TempDir()) // no hurl on PATH
	dir := t.TempDir()
	path := filepath.Join(dir, "hurl-scenario.md")
	content := "# Rehearse: hurl fixture\n\n**Status:** pending\n**Verifies:** feat/x#ac:hurlone\n\n" +
		"```hurl\nGET http://127.0.0.1:1/\nHTTP 200\n```\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	out, _, err := runRehearseCmd(t, "run", path, "--filter", "feat/x#ac:hurlone")
	if err != nil {
		t.Fatalf("a runner-skipped scenario should not fail the run: %v", err)
	}
	if !strings.Contains(out, "[filter-match]") {
		t.Errorf("output should contain [filter-match]:\n%s", out)
	}
	// The hurl-missing warning is in r.Detail — verify it appears.
	if !strings.Contains(out, "hurl") {
		t.Errorf("output should contain the hurl missing warning:\n%s", out)
	}
}
