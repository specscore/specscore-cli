package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
