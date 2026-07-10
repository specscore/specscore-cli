package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/specscore/specscore-cli/internal/rehearse/runner"
)

// runRehearseCmd executes the rehearse command group with args against a
// fresh command tree, capturing stdout/stderr.
func runRehearseCmd(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := rehearseCommand()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func rehearseExit(t *testing.T, err error) int {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error")
	}
	var ec interface{ ExitCode() int }
	if !errors.As(err, &ec) {
		t.Fatalf("error carries no exit code: %v", err)
	}
	return ec.ExitCode()
}

// writeScenario writes a scenario file with one bash block and returns its
// path. The enclosing dir carries no specscore.yaml (standalone mode).
func writeScenario(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "scenario.md")
	content := "# Rehearse: fixture\n\n**Status:** pending\n**Verifies:** demo/x#ac:one (REQ: y)\n\n```bash\n" +
		body + "\n```\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestRehearse_HelpExitsZero(t *testing.T) {
	out, _, err := runRehearseCmd(t)
	if err != nil {
		t.Fatalf("rehearse with no subcommand: %v", err)
	}
	if !strings.Contains(out, "run") {
		t.Errorf("help output does not mention the run verb: %q", out)
	}
}

// AC: standalone-run — a passing scenario in a directory with no
// specscore.yaml exits 0 and reports pass.
func TestRehearseRun_StandalonePassExitsZero(t *testing.T) {
	file := writeScenario(t, "echo hello")

	out, _, err := runRehearseCmd(t, "run", file)
	if err != nil {
		t.Fatalf("rehearse run: %v", err)
	}
	if !strings.Contains(out, "pass") || !strings.Contains(out, file) {
		t.Errorf("report does not mark the scenario pass:\n%s", out)
	}
	if !strings.Contains(out, "Total: 1 scenario(s) — 1 pass, 0 fail, 0 skipped, 0 no-steps") {
		t.Errorf("totals line missing:\n%s", out)
	}
}

// AC: failing-scenario-fails-run — a failing bash step exits 1 and the
// report marks the scenario fail.
func TestRehearseRun_FailingScenarioExitsOne(t *testing.T) {
	file := writeScenario(t, "exit 5")

	out, _, err := runRehearseCmd(t, "run", file)
	if code := rehearseExit(t, err); code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
	if !strings.Contains(err.Error(), "1 of 1 scenario(s) failed") {
		t.Errorf("error = %v", err)
	}
	if !strings.Contains(out, "fail") || !strings.Contains(out, file) {
		t.Errorf("report does not mark the scenario fail:\n%s", out)
	}
}

func TestRehearseRun_NoStepsScenarioPassesRun(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "prose.md")
	if err := os.WriteFile(file, []byte("# Prose only\n\n**Verifies:** demo/x#ac:one\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, _, err := runRehearseCmd(t, "run", file)
	if err != nil {
		t.Fatalf("a no-steps scenario must not fail the run: %v", err)
	}
	if !strings.Contains(out, "no-steps") {
		t.Errorf("report does not mark the scenario no-steps:\n%s", out)
	}
	if !strings.Contains(out, "0 pass, 0 fail, 0 skipped, 1 no-steps") {
		t.Errorf("totals line missing the no-steps count:\n%s", out)
	}
}

func TestRehearseRun_JSONReportShape(t *testing.T) {
	file := writeScenario(t, "echo hello")

	out, _, err := runRehearseCmd(t, "run", file, "--format", "json")
	if err != nil {
		t.Fatalf("rehearse run --format json: %v", err)
	}
	var reports []struct {
		File       string            `json:"file"`
		Status     string            `json:"status"`
		Verifies   []string          `json:"verifies"`
		DurationMS *int64            `json:"duration_ms"`
		Bag        map[string]string `json:"bag"`
		Steps      []struct {
			Kind   string `json:"kind"`
			Status string `json:"status"`
		} `json:"steps"`
	}
	if err := json.Unmarshal([]byte(out), &reports); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if len(reports) != 1 {
		t.Fatalf("reports = %d, want 1", len(reports))
	}
	r := reports[0]
	if r.File != file || r.Status != "pass" || r.DurationMS == nil || r.Bag == nil {
		t.Errorf("report fields wrong: %+v", r)
	}
	if len(r.Verifies) != 1 || r.Verifies[0] != "demo/x#ac:one" {
		t.Errorf("verifies = %v", r.Verifies)
	}
	if len(r.Steps) != 1 || r.Steps[0].Kind != "bash" || r.Steps[0].Status != "pass" {
		t.Errorf("steps = %+v", r.Steps)
	}
}

// AC: json-report-shape — the raw JSON carries exactly the REQ run-report
// contract fields: the first element has `file`, `status`, `verifies`,
// `duration_ms`, `bag` (present even when empty) and a non-empty `steps`
// array whose entries carry `kind` and `status`.
func TestRehearseRun_JSONReportRawContractKeys(t *testing.T) {
	file := writeScenario(t, "echo hello")

	out, _, err := runRehearseCmd(t, "run", file, "--format", "json")
	if err != nil {
		t.Fatalf("rehearse run --format json: %v", err)
	}
	var raw []map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &raw); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if len(raw) == 0 {
		t.Fatal("JSON report is empty")
	}
	first := raw[0]
	for _, key := range []string{"file", "status", "verifies", "duration_ms", "bag", "steps"} {
		if _, ok := first[key]; !ok {
			t.Errorf("first element lacks the %q key:\n%s", key, out)
		}
	}
	if string(first["bag"]) != "{}" {
		t.Errorf("empty bag must serialize as {}, got %s", first["bag"])
	}
	var steps []map[string]json.RawMessage
	if err := json.Unmarshal(first["steps"], &steps); err != nil || len(steps) == 0 {
		t.Fatalf("steps is not a non-empty array: %v\n%s", err, first["steps"])
	}
	for _, key := range []string{"kind", "status"} {
		if _, ok := steps[0][key]; !ok {
			t.Errorf("step entry lacks the %q key:\n%s", key, first["steps"])
		}
	}
}

// REQ: run-report — a failing step's report entry carries its `detail`.
func TestRehearseRun_JSONReportFailingStepCarriesDetail(t *testing.T) {
	file := writeScenario(t, "exit 7")

	out, _, err := runRehearseCmd(t, "run", file, "--format", "json")
	if code := rehearseExit(t, err); code != 1 {
		t.Fatalf("exit code = %d, want 1", code)
	}
	var reports []struct {
		Status string `json:"status"`
		Steps  []struct {
			Kind   string `json:"kind"`
			Status string `json:"status"`
			Detail string `json:"detail"`
		} `json:"steps"`
	}
	if err := json.Unmarshal([]byte(out), &reports); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if len(reports) != 1 || reports[0].Status != "fail" {
		t.Fatalf("reports = %+v", reports)
	}
	step := reports[0].Steps[0]
	if step.Kind != "bash" || step.Status != "fail" || !strings.Contains(step.Detail, "exit status 7") {
		t.Errorf("failing step = %+v", step)
	}
}

// writeScenarioBlocks writes a scenario file with the given pre-fenced block
// sections and returns its path.
func writeScenarioBlocks(t *testing.T, fencedBlocks ...string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "scenario.md")
	content := "# Rehearse: fixture\n\n**Status:** pending\n**Verifies:** demo/x#ac:one (REQ: y)\n\n" +
		strings.Join(fencedBlocks, "\n\n") + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// AC: sql-assert-rows — a scenario whose sql block queries a 3-row table
// with `-- assert-rows: 3` exits 0 and is reported pass.
func TestRehearseRun_SQLBlockAssertRows(t *testing.T) {
	file := writeScenarioBlocks(t,
		"```sql dsn=sqlite:fixture.db\nCREATE TABLE t (id INTEGER);\nINSERT INTO t VALUES (1), (2), (3);\n```",
		"```sql dsn=sqlite:fixture.db\nSELECT * FROM t;\n-- assert-rows: 3\n```")

	out, _, err := runRehearseCmd(t, "run", file)
	if err != nil {
		t.Fatalf("rehearse run: %v\n%s", err, out)
	}
	if !strings.Contains(out, "pass") || !strings.Contains(out, "1 pass") {
		t.Errorf("report does not mark the sql scenario pass:\n%s", out)
	}
}

// AC: dtql-counts-facts — a scenario whose dtql block selects facts with
// `-- assert-rows:` equal to the store's fact count exits 0 and is pass.
// The store is shaped like a Studio fact store (table `facts`), built by a
// preceding sql step in the same scenario working dir.
func TestRehearseRun_DTQLBlockCountsFacts(t *testing.T) {
	file := writeScenarioBlocks(t,
		"```sql dsn=sqlite:facts.db\nCREATE TABLE facts (subject TEXT, predicate TEXT, object TEXT);\n"+
			"INSERT INTO facts VALUES ('a', 'imports', 'b'), ('b', 'imports', 'c');\n```",
		"```dtql db=facts.db\nfrom:\n  name: facts\n-- assert-rows: 2\n```")

	out, _, err := runRehearseCmd(t, "run", file)
	if err != nil {
		t.Fatalf("rehearse run: %v\n%s", err, out)
	}
	if !strings.Contains(out, "pass") || !strings.Contains(out, "1 pass") {
		t.Errorf("report does not mark the dtql scenario pass:\n%s", out)
	}
}

func TestRehearseRun_UnknownFormatExits2(t *testing.T) {
	file := writeScenario(t, "echo hello")
	_, _, err := runRehearseCmd(t, "run", file, "--format", "xml")
	if code := rehearseExit(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

func TestRehearseRun_MissingPathExits2(t *testing.T) {
	_, _, err := runRehearseCmd(t, "run", filepath.Join(t.TempDir(), "absent.md"))
	if code := rehearseExit(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
}

func TestRehearseRun_DefaultDiscoveryInsideRepo(t *testing.T) {
	root := t.TempDir()
	scenarioPath := filepath.Join(root, "spec", "features", "x", "_tests", "one.md")
	if err := os.MkdirAll(filepath.Dir(scenarioPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "specscore.yaml"), []byte("project:\n  title: Fixture\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(scenarioPath, []byte("**Verifies:** demo/x#ac:one\n\n```bash\ntrue\n```\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	restore := osGetwdFn
	osGetwdFn = func() (string, error) { return root, nil }
	t.Cleanup(func() { osGetwdFn = restore })

	out, _, err := runRehearseCmd(t, "run")
	if err != nil {
		t.Fatalf("rehearse run (default discovery): %v", err)
	}
	if !strings.Contains(out, scenarioPath) || !strings.Contains(out, "1 pass") {
		t.Errorf("default discovery did not run the _tests scenario:\n%s", out)
	}
}

func TestRehearseRun_GetwdErrorExits10(t *testing.T) {
	restore := osGetwdFn
	osGetwdFn = func() (string, error) { return "", errors.New("cwd gone") }
	t.Cleanup(func() { osGetwdFn = restore })

	_, _, err := runRehearseCmd(t, "run")
	if code := rehearseExit(t, err); code != 10 {
		t.Errorf("exit code = %d, want 10", code)
	}
}

// failWriter fails every write, driving the JSON-encoder error branch.
type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, errors.New("pipe closed") }

func TestRehearseRun_JSONEncodeErrorExits10(t *testing.T) {
	file := writeScenario(t, "echo hello")
	cmd := rehearseCommand()
	cmd.SetOut(failWriter{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"run", file, "--format", "json"})

	err := cmd.Execute()
	if code := rehearseExit(t, err); code != 10 {
		t.Errorf("exit code = %d, want 10", code)
	}
}

// TestRehearseRegistry_AllV03Kinds pins the registered block kinds: the five
// v0.3 executors, hurl-derived ones declaring their binary dependency.
func TestRehearseRegistry_AllV03Kinds(t *testing.T) {
	reg := rehearseRegistry()
	for _, kind := range []string{"bash", "hurl", "sql", "dtql", "graphql"} {
		if _, ok := reg[kind]; !ok {
			t.Errorf("registry lacks the %q executor", kind)
		}
	}
	for _, kind := range []string{"hurl", "graphql"} {
		dep, ok := reg[kind].(interface{ RequiredBinary() string })
		if !ok || dep.RequiredBinary() != "hurl" {
			t.Errorf("the %q executor does not declare its hurl dependency", kind)
		}
	}
}

// fakeHurlOnPath installs a fake `hurl` script as the only binary on PATH:
// it answers the --help capability probe with --report-json support and
// writes $FAKE_REPORT_JSON as the report.
func fakeHurlOnPath(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	script := `#!/bin/bash
if [ "$1" = "--help" ]; then echo "--report-json <DIR>"; exit 0; fi
prev=""
for a in "$@"; do
  if [ "$prev" = "--report-json" ]; then printf '%s' "$FAKE_REPORT_JSON" > "$a/report.json"; fi
  prev="$a"
done
exit 0
`
	if err := os.WriteFile(filepath.Join(dir, "hurl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("FAKE_REPORT_JSON", `[{"entries":[{"captures":[]}]}]`)
}

// AC: hurl-missing-skips (unit mirror) — with a PATH lacking hurl, a
// scenario containing a hurl block exits 0, is reported skipped, and the
// warning names the hurl binary.
func TestRehearseRun_MissingHurlSkipsScenario(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	file := writeScenarioBlocks(t, "```hurl\nGET http://127.0.0.1:1/\nHTTP 200\n```")

	out, _, err := runRehearseCmd(t, "run", file)
	if err != nil {
		t.Fatalf("a skipped scenario must not fail the run: %v\n%s", err, out)
	}
	if !strings.Contains(out, "skipped") || !strings.Contains(out, `"hurl"`) {
		t.Errorf("report does not skip with a warning naming hurl:\n%s", out)
	}
	if !strings.Contains(out, "1 skipped") {
		t.Errorf("totals line does not count the skip:\n%s", out)
	}
}

// REQ: hurl-block + context-bag wiring through the real command: a hurl
// scenario passes and its [Captures] land in the report's bag.
func TestRehearseRun_HurlBlockPassesAndCaptures(t *testing.T) {
	fakeHurlOnPath(t)
	t.Setenv("FAKE_REPORT_JSON", `[{"entries":[{"captures":[{"name":"token","value":"t-9"}]}]}]`)
	file := writeScenarioBlocks(t, "```hurl\nGET http://127.0.0.1:1/\nHTTP 200\n```")

	out, _, err := runRehearseCmd(t, "run", file, "--format", "json")
	if err != nil {
		t.Fatalf("rehearse run: %v\n%s", err, out)
	}
	var reports []struct {
		Status string            `json:"status"`
		Bag    map[string]string `json:"bag"`
	}
	if err := json.Unmarshal([]byte(out), &reports); err != nil {
		t.Fatalf("output is not valid JSON: %v\n%s", err, out)
	}
	if reports[0].Status != "pass" || reports[0].Bag["token"] != "t-9" {
		t.Errorf("report = %+v, want pass with the hurl capture in the bag", reports[0])
	}
}

// REQ: graphql-block wiring through the real command: the graphql executor
// compiles onto the hurl delegation and passes.
func TestRehearseRun_GraphQLBlockPasses(t *testing.T) {
	fakeHurlOnPath(t)
	file := writeScenarioBlocks(t,
		"```graphql url=http://127.0.0.1:1/graphql\nquery { ok }\n-- assert-jsonpath: $.data.ok == true\n```")

	out, _, err := runRehearseCmd(t, "run", file)
	if err != nil {
		t.Fatalf("rehearse run: %v\n%s", err, out)
	}
	if !strings.Contains(out, "1 pass") {
		t.Errorf("report does not mark the graphql scenario pass:\n%s", out)
	}
}

// --- Feature: cli/rehearse/evidence (REQ: report-out, REQ: report-provenance) ---

// swapWriteReport replaces writeReportFn for the duration of a test and
// restores it via t.Cleanup.
func swapWriteReport(t *testing.T, fn func(string, string, time.Time, []runner.ScenarioReport, string) error) {
	t.Helper()
	orig := writeReportFn
	writeReportFn = fn
	t.Cleanup(func() { writeReportFn = orig })
}

// AC: report-out-writes-envelope — --report-out writes a valid JSON envelope
// with non-empty runner_version, non-empty git_sha, git_dirty false,
// non-empty RFC 3339 started_at, and a scenarios array with pass status.
func TestRehearseRun_ReportOutWritesEnvelope(t *testing.T) {
	file := writeScenario(t, "echo hello")
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "out", "report.json")

	// Stub osGetwdFn so the runner has a consistent cwd (not a real git repo).
	restoreWd := osGetwdFn
	osGetwdFn = func() (string, error) { return dir, nil }
	t.Cleanup(func() { osGetwdFn = restoreWd })

	// Capture what WriteReport receives and write a canned envelope.
	var capturedPath, capturedVersion string
	var capturedStartedAt time.Time
	var capturedReports []runner.ScenarioReport
	var capturedDir string
	swapWriteReport(t, func(path, rv string, started time.Time, rpts []runner.ScenarioReport, d string) error {
		capturedPath = path
		capturedVersion = rv
		capturedStartedAt = started
		capturedReports = rpts
		capturedDir = d
		// Write a real envelope so we can verify the JSON shape.
		return runner.WriteReport(path, rv, started, rpts, d)
	})
	// Stub git: clean HEAD so the envelope has a non-empty sha.
	restoreExec := runner.ExecCommandFn
	runner.ExecCommandFn = func(d, name string, args ...string) ([]byte, error) {
		switch {
		case args[0] == "rev-parse" && args[1] == "HEAD":
			return []byte("deadbeef\n"), nil
		case args[0] == "rev-parse" && args[1] == "--show-toplevel":
			return []byte(d + "\n"), nil
		case args[0] == "status":
			return []byte(""), nil
		}
		return nil, errors.New("unexpected git call")
	}
	t.Cleanup(func() { runner.ExecCommandFn = restoreExec })

	// Fix NowFn so started_at is deterministic.
	fixedTime := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	restoreNow := runner.NowFn
	runner.NowFn = func() time.Time { return fixedTime }
	t.Cleanup(func() { runner.NowFn = restoreNow })

	out, _, err := runRehearseCmd(t, "run", file, "--report-out", reportPath)
	if err != nil {
		t.Fatalf("rehearse run: %v\n%s", err, out)
	}

	// Verify the seam received the right arguments.
	if capturedPath != reportPath {
		t.Errorf("WriteReport path = %q, want %q", capturedPath, reportPath)
	}
	if capturedVersion == "" {
		t.Errorf("WriteReport runnerVersion is empty")
	}
	if !capturedStartedAt.Equal(fixedTime) {
		t.Errorf("WriteReport startedAt = %v, want %v", capturedStartedAt, fixedTime)
	}
	if len(capturedReports) != 1 {
		t.Fatalf("WriteReport reports len = %d, want 1", len(capturedReports))
	}
	if capturedDir == "" {
		t.Errorf("WriteReport dir is empty")
	}

	// Verify the actual JSON file.
	data, err := os.ReadFile(reportPath)
	if err != nil {
		t.Fatalf("ReadFile(%q): %v", reportPath, err)
	}
	var env runner.RunReport
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("unmarshal envelope: %v", err)
	}
	if env.RunnerVersion == "" {
		t.Error("runner_version is empty")
	}
	if env.GitSHA != "deadbeef" {
		t.Errorf("git_sha = %q, want deadbeef", env.GitSHA)
	}
	if env.GitDirty {
		t.Error("git_dirty = true, want false")
	}
	if env.StartedAt != "2026-01-02T03:04:05Z" {
		t.Errorf("started_at = %q, want RFC 3339", env.StartedAt)
	}
	if len(env.Scenarios) != 1 {
		t.Fatalf("scenarios len = %d, want 1", len(env.Scenarios))
	}
	sc := env.Scenarios[0]
	if sc.Status != "pass" {
		t.Errorf("scenarios[0].status = %q, want pass", sc.Status)
	}
	if len(sc.Verifies) == 0 {
		t.Error("scenarios[0].verifies is empty")
	}
	if sc.DurationMS < 0 {
		t.Errorf("scenarios[0].duration_ms = %d", sc.DurationMS)
	}
	if len(sc.Steps) == 0 {
		t.Error("scenarios[0].steps is empty")
	}

	// stdout report is unchanged.
	if !strings.Contains(out, "pass") {
		t.Errorf("stdout does not contain pass:\n%s", out)
	}
}

// AC: report-out-on-failure — a failing run still writes the report and exits 1.
func TestRehearseRun_ReportOutOnFailure(t *testing.T) {
	file := writeScenario(t, "exit 7")
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "report.json")

	restoreWd := osGetwdFn
	osGetwdFn = func() (string, error) { return dir, nil }
	t.Cleanup(func() { osGetwdFn = restoreWd })

	// Stub WriteReport to delegate to the real impl (no git needed here).
	swapWriteReport(t, func(path, rv string, started time.Time, rpts []runner.ScenarioReport, d string) error {
		return runner.WriteReport(path, rv, started, rpts, d)
	})
	// Stub git to return empty (outside git).
	restoreExec := runner.ExecCommandFn
	runner.ExecCommandFn = func(d, name string, args ...string) ([]byte, error) {
		return nil, errors.New("not a git repo")
	}
	t.Cleanup(func() { runner.ExecCommandFn = restoreExec })

	_, _, err := runRehearseCmd(t, "run", file, "--report-out", reportPath)
	if code := rehearseExit(t, err); code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}

	data, readErr := os.ReadFile(reportPath)
	if readErr != nil {
		t.Fatalf("report not written: %v", readErr)
	}
	var env runner.RunReport
	if err := json.Unmarshal(data, &env); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(env.Scenarios) != 1 || env.Scenarios[0].Status != "fail" {
		t.Errorf("scenarios[0].status = %q, want fail", env.Scenarios[0].Status)
	}
}

// AC: report-out-outside-git — git_sha is empty and git_dirty is false.
func TestRehearseRun_ReportOutOutsideGit(t *testing.T) {
	file := writeScenario(t, "echo ok")
	dir := t.TempDir()
	reportPath := filepath.Join(dir, "report.json")

	restoreWd := osGetwdFn
	osGetwdFn = func() (string, error) { return dir, nil }
	t.Cleanup(func() { osGetwdFn = restoreWd })

	// Stub git to simulate "not a git repo".
	restoreExec := runner.ExecCommandFn
	runner.ExecCommandFn = func(d, name string, args ...string) ([]byte, error) {
		return nil, errors.New("not a git repository")
	}
	t.Cleanup(func() { runner.ExecCommandFn = restoreExec })

	swapWriteReport(t, func(path, rv string, started time.Time, rpts []runner.ScenarioReport, d string) error {
		return runner.WriteReport(path, rv, started, rpts, d)
	})

	_, _, err := runRehearseCmd(t, "run", file, "--report-out", reportPath)
	if err != nil {
		t.Fatalf("rehearse run: %v", err)
	}

	data, _ := os.ReadFile(reportPath)
	var env runner.RunReport
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

// report-out unwritable path → exits 2 after stdout report is printed.
func TestRehearseRun_ReportOutUnwritableExits2(t *testing.T) {
	file := writeScenario(t, "echo hi")
	dir := t.TempDir()

	restoreWd := osGetwdFn
	osGetwdFn = func() (string, error) { return dir, nil }
	t.Cleanup(func() { osGetwdFn = restoreWd })

	// Stub WriteReport to fail.
	swapWriteReport(t, func(path, rv string, started time.Time, rpts []runner.ScenarioReport, d string) error {
		return errors.New("permission denied")
	})

	out, _, err := runRehearseCmd(t, "run", file, "--report-out", "/no/such/path/report.json")
	if code := rehearseExit(t, err); code != 2 {
		t.Errorf("exit code = %d, want 2", code)
	}
	// stdout report must have been printed before the error.
	if !strings.Contains(out, "pass") {
		t.Errorf("stdout report not printed before exit 2:\n%s", out)
	}
}

// --report-out with explicit paths triggers osGetwdFn (for git provenance);
// a getwd failure exits 10.
func TestRehearseRun_ReportOutGetwdErrorExits10(t *testing.T) {
	file := writeScenario(t, "echo hi")
	restoreWd := osGetwdFn
	osGetwdFn = func() (string, error) { return "", errors.New("getwd gone") }
	t.Cleanup(func() { osGetwdFn = restoreWd })

	_, _, err := runRehearseCmd(t, "run", file, "--report-out", "out/report.json")
	if code := rehearseExit(t, err); code != 10 {
		t.Errorf("exit code = %d, want 10", code)
	}
}
