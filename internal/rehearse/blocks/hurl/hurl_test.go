package hurl

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/internal/rehearse/blocks"
)

// fakeHurl installs an executable fake `hurl` script as the only binary on
// PATH and returns the path of the file where the script records its argv
// (one arg per line). The script's behavior is driven by env vars:
//
//	FAKE_HELP           — text printed for `--help` (capability probe)
//	FAKE_REPORT_JSON    — JSON written to <report-dir>/report.json (modern mode)
//	FAKE_STDOUT         — text printed to stdout for a run (legacy --json mode)
//	FAKE_EXIT           — run exit code (default 0)
//	FAKE_SKIP_REPORT    — when "1", no report.json is written
func fakeHurl(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	argsFile := filepath.Join(dir, "argv.txt")
	script := `#!/bin/bash
if [ "$1" = "--help" ]; then printf '%s\n' "$FAKE_HELP"; exit 0; fi
printf '%s\n' "$@" > "` + argsFile + `"
prev=""
for a in "$@"; do
  if [ "$prev" = "--report-json" ] && [ "$FAKE_SKIP_REPORT" != "1" ]; then
    printf '%s' "$FAKE_REPORT_JSON" > "$a/report.json"
  fi
  prev="$a"
done
if [ -n "$FAKE_STDOUT" ]; then printf '%s' "$FAKE_STDOUT"; fi
echo "fake hurl ran" >&2
exit "${FAKE_EXIT:-0}"
`
	if err := os.WriteFile(filepath.Join(dir, "hurl"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", dir)
	t.Setenv("FAKE_HELP", "Usage: hurl [OPTIONS]\n      --report-json <DIR>    Generate JSON report to DIR")
	t.Setenv("FAKE_REPORT_JSON", `[{"entries":[{"captures":[]}]}]`)
	t.Setenv("FAKE_STDOUT", "")
	t.Setenv("FAKE_EXIT", "0")
	t.Setenv("FAKE_SKIP_REPORT", "0")
	return argsFile
}

// recordedArgs reads the argv the fake hurl script recorded.
func recordedArgs(t *testing.T, argsFile string) []string {
	t.Helper()
	data, err := os.ReadFile(argsFile)
	if err != nil {
		t.Fatalf("the fake hurl was never run: %v", err)
	}
	return strings.Split(strings.TrimRight(string(data), "\n"), "\n")
}

func TestExecutor_KindAndRequiredBinary(t *testing.T) {
	e := New()
	if e.Kind() != "hurl" {
		t.Errorf("Kind() = %q, want hurl", e.Kind())
	}
	if e.RequiredBinary() != "hurl" {
		t.Errorf("RequiredBinary() = %q, want hurl", e.RequiredBinary())
	}
}

// TestRun_ModernPassWithVarsAndCaptures covers the --report-json path: vars
// become --variable flags (no textual interpolation), captures come back
// from report.json in order and with number/bool fidelity (REQ: hurl-block,
// REQ: context-bag).
func TestRun_ModernPassWithVarsAndCaptures(t *testing.T) {
	argsFile := fakeHurl(t)
	t.Setenv("FAKE_REPORT_JSON",
		`[{"entries":[{"captures":[{"name":"token","value":"t-1"},{"name":"n","value":42},{"name":"ok","value":true}]}]}]`)

	res := New().Run(blocks.StepCtx{
		WorkDir: t.TempDir(),
		Body:    "GET http://x/{{uid}}\nHTTP 200\n",
		Vars:    []blocks.Capture{{Name: "uid", Value: "42"}, {Name: "name", Value: "alice"}},
	})
	if res.Status != blocks.StatusPass {
		t.Fatalf("status = %q, want pass (detail: %s)", res.Status, res.Detail)
	}
	want := []blocks.Capture{{Name: "token", Value: "t-1"}, {Name: "n", Value: "42"}, {Name: "ok", Value: "true"}}
	if !reflect.DeepEqual(res.Captures, want) {
		t.Errorf("captures = %v, want %v", res.Captures, want)
	}
	if !strings.Contains(res.Output, "fake hurl ran") {
		t.Errorf("output = %q, want the run output captured", res.Output)
	}

	args := recordedArgs(t, argsFile)
	joined := strings.Join(args, " ")
	if args[0] != "--test" {
		t.Errorf("args = %v, want --test first", args)
	}
	if !strings.Contains(joined, "--variable uid=42") || !strings.Contains(joined, "--variable name=alice") {
		t.Errorf("args lack the --variable flags: %v", args)
	}
	hurlFile := args[len(args)-1]
	body, err := os.ReadFile(hurlFile)
	if err == nil {
		// The temp file may already be cleaned up; when readable it must be
		// the verbatim body (no interpolation of {{uid}}).
		if !strings.Contains(string(body), "{{uid}}") {
			t.Errorf("hurl file body was interpolated: %q", body)
		}
	}
	if !strings.HasSuffix(hurlFile, ".hurl") {
		t.Errorf("last arg %q is not the .hurl file", hurlFile)
	}
}

// TestRun_LegacyJSONFallback covers the pre-4.3 path: no --report-json in
// help, so the run uses `--json` and parses stdout.
func TestRun_LegacyJSONFallback(t *testing.T) {
	argsFile := fakeHurl(t)
	t.Setenv("FAKE_HELP", "Usage: hurl [OPTIONS]\n      --json    Output result to JSON")
	t.Setenv("FAKE_STDOUT", `{"entries":[{"captures":[{"name":"token","value":"t-2"}]}]}`)

	res := New().Run(blocks.StepCtx{WorkDir: t.TempDir(), Body: "GET http://x\nHTTP 200\n"})
	if res.Status != blocks.StatusPass {
		t.Fatalf("status = %q, want pass (detail: %s)", res.Status, res.Detail)
	}
	if want := []blocks.Capture{{Name: "token", Value: "t-2"}}; !reflect.DeepEqual(res.Captures, want) {
		t.Errorf("captures = %v, want %v", res.Captures, want)
	}
	args := recordedArgs(t, argsFile)
	if args[0] != "--json" {
		t.Errorf("args = %v, want --json first", args)
	}
	for _, a := range args {
		if a == "--test" || a == "--report-json" {
			t.Errorf("legacy mode must not pass %s: %v", a, args)
		}
	}
	if !strings.Contains(res.Output, "fake hurl ran") {
		t.Errorf("output = %q, want stderr captured as the run output", res.Output)
	}
}

func TestRun_HurlFailureIsStepFail(t *testing.T) {
	fakeHurl(t)
	t.Setenv("FAKE_EXIT", "3")

	res := New().Run(blocks.StepCtx{WorkDir: t.TempDir(), Body: "GET http://x\nHTTP 200\n"})
	if res.Status != blocks.StatusFail {
		t.Fatalf("status = %q, want fail", res.Status)
	}
	if !strings.Contains(res.Detail, "hurl step failed") || !strings.Contains(res.Detail, "hurl --test") {
		t.Errorf("detail = %q", res.Detail)
	}
	if !strings.Contains(res.Output, "fake hurl ran") {
		t.Errorf("output = %q, want the failing run's output kept", res.Output)
	}
}

func TestRun_LegacyHurlFailureIsStepFail(t *testing.T) {
	fakeHurl(t)
	t.Setenv("FAKE_HELP", "no json report here")
	t.Setenv("FAKE_EXIT", "4")

	res := New().Run(blocks.StepCtx{WorkDir: t.TempDir(), Body: "GET http://x\nHTTP 200\n"})
	if res.Status != blocks.StatusFail || !strings.Contains(res.Detail, "hurl --json") {
		t.Errorf("res = %+v, want a hurl --json failure", res)
	}
}

func TestRun_HelpProbeFailure(t *testing.T) {
	// PATH without any hurl: the capability probe itself fails.
	t.Setenv("PATH", t.TempDir())

	res := New().Run(blocks.StepCtx{WorkDir: t.TempDir(), Body: "GET http://x\n"})
	if res.Status != blocks.StatusFail || !strings.Contains(res.Detail, "probing hurl capabilities") {
		t.Errorf("res = %+v, want a capability-probe failure", res)
	}
}

func TestRun_MissingReportJSONFileFails(t *testing.T) {
	fakeHurl(t)
	t.Setenv("FAKE_SKIP_REPORT", "1")

	res := New().Run(blocks.StepCtx{WorkDir: t.TempDir(), Body: "GET http://x\n"})
	if res.Status != blocks.StatusFail || !strings.Contains(res.Detail, "reading hurl JSON report") {
		t.Errorf("res = %+v, want a report-read failure", res)
	}
}

func TestRun_MalformedReportJSONFails(t *testing.T) {
	fakeHurl(t)
	t.Setenv("FAKE_REPORT_JSON", `[not json`)

	res := New().Run(blocks.StepCtx{WorkDir: t.TempDir(), Body: "GET http://x\n"})
	if res.Status != blocks.StatusFail || !strings.Contains(res.Detail, "parsing hurl JSON report") {
		t.Errorf("res = %+v, want a report-parse failure", res)
	}
}

func TestRun_MalformedLegacyJSONFails(t *testing.T) {
	fakeHurl(t)
	t.Setenv("FAKE_HELP", "no json report here")
	t.Setenv("FAKE_STDOUT", "{broken")

	res := New().Run(blocks.StepCtx{WorkDir: t.TempDir(), Body: "GET http://x\n"})
	if res.Status != blocks.StatusFail || !strings.Contains(res.Detail, "parsing hurl JSON result") {
		t.Errorf("res = %+v, want a result-parse failure", res)
	}
}

func TestExecute_CreateTempError(t *testing.T) {
	restore := createTempFn
	createTempFn = func(string, string) (*os.File, error) { return nil, errors.New("disk full") }
	t.Cleanup(func() { createTempFn = restore })

	if _, _, err := Execute(t.TempDir(), "GET http://x\n", nil); err == nil || !strings.Contains(err.Error(), "creating hurl file") {
		t.Errorf("err = %v, want a create-temp failure", err)
	}
}

func TestExecute_WriteFileError(t *testing.T) {
	fakeHurl(t)
	restore := writeFileFn
	writeFileFn = func(string, []byte, os.FileMode) error { return errors.New("disk full") }
	t.Cleanup(func() { writeFileFn = restore })

	if _, _, err := Execute(t.TempDir(), "GET http://x\n", nil); err == nil || !strings.Contains(err.Error(), "writing hurl file") {
		t.Errorf("err = %v, want a write failure", err)
	}
}

func TestExecute_MkdirTempError(t *testing.T) {
	fakeHurl(t)
	restore := mkdirTempFn
	mkdirTempFn = func(string, string) (string, error) { return "", errors.New("disk full") }
	t.Cleanup(func() { mkdirTempFn = restore })

	if _, _, err := Execute(t.TempDir(), "GET http://x\n", nil); err == nil || !strings.Contains(err.Error(), "creating hurl report dir") {
		t.Errorf("err = %v, want a report-dir failure", err)
	}
}

func TestParseCaptures_ValueForms(t *testing.T) {
	captures, err := parseCaptures([]byte(
		`[{"entries":[{"captures":[` +
			`{"name":"s","value":"text"},` +
			`{"name":"i","value":42},` +
			`{"name":"f","value":4.5},` +
			`{"name":"b","value":false},` +
			`{"name":"nil","value":null},` +
			`{"name":"list","value":[1,"two"]}]}]}]`))
	if err != nil {
		t.Fatal(err)
	}
	want := []blocks.Capture{
		{Name: "s", Value: "text"},
		{Name: "i", Value: "42"},
		{Name: "f", Value: "4.5"},
		{Name: "b", Value: "false"},
		{Name: "nil", Value: "null"},
		{Name: "list", Value: `[1,"two"]`},
	}
	if !reflect.DeepEqual(captures, want) {
		t.Errorf("captures = %v, want %v", captures, want)
	}
}

func TestParseCaptures_EmptyAndNoCaptures(t *testing.T) {
	for _, data := range []string{"", `[]`, `[{"entries":[]}]`, `{"entries":[{"captures":[]}]}`} {
		captures, err := parseCaptures([]byte(data))
		if err != nil {
			// The empty string is not valid JSON — an object-form parse error.
			if data == "" && strings.Contains(err.Error(), "parsing hurl JSON result") {
				continue
			}
			t.Errorf("parseCaptures(%q): %v", data, err)
			continue
		}
		if len(captures) != 0 {
			t.Errorf("parseCaptures(%q) = %v, want none", data, captures)
		}
	}
}

// TestExecute_RunCmdSeam pins that the executor runs commands through the
// runCmdFn seam (the door unit tests use instead of a real binary).
func TestExecute_RunCmdSeam(t *testing.T) {
	fakeHurl(t)
	var ran *exec.Cmd
	restore := runCmdFn
	runCmdFn = func(cmd *exec.Cmd) error { ran = cmd; return errors.New("intercepted") }
	t.Cleanup(func() { runCmdFn = restore })

	workDir := t.TempDir()
	_, _, err := Execute(workDir, "GET http://x\n", nil)
	if err == nil || !strings.Contains(err.Error(), "intercepted") {
		t.Fatalf("err = %v, want the seam's error", err)
	}
	if ran == nil || ran.Dir != workDir {
		t.Errorf("command not rooted in the scenario working dir: %+v", ran)
	}
}
