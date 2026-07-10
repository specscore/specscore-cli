// Package hurl implements the ```hurl rehearse step block: the block body is
// verbatim Hurl syntax delegated to the `hurl` binary in test mode — the
// runner does NOT implement an HTTP client (REQ: hurl-block). The scenario's
// context bag arrives as StepCtx.Vars and is passed on as `--variable
// name=value` flags (no textual interpolation — Hurl owns the {{name}}
// syntax natively); the block's native [Captures] are read back from hurl's
// JSON report and merged into the bag (REQ: context-bag).
package hurl

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/internal/rehearse/blocks"
)

// Binary is the external binary this block delegates to. The runner's
// upfront scan skips a whole scenario when it is not on PATH
// (REQ: hurl-block).
const Binary = "hurl"

// Test seams — package-level vars wrapping external functions.
var (
	createTempFn = os.CreateTemp
	writeFileFn  = os.WriteFile
	mkdirTempFn  = os.MkdirTemp
	readFileFn   = os.ReadFile
	runCmdFn     = func(cmd *exec.Cmd) error { return cmd.Run() }
	helpOutputFn = func() (string, error) {
		out, err := exec.Command(Binary, "--help").CombinedOutput()
		return string(out), err
	}
)

// Executor runs hurl step blocks.
type Executor struct{}

// New returns the hurl block executor.
func New() *Executor { return &Executor{} }

// Kind returns "hurl".
func (*Executor) Kind() string { return "hurl" }

// RequiredBinary names the external binary the executor delegates to,
// feeding the runner's upfront missing-binary scan (REQ: hurl-block).
func (*Executor) RequiredBinary() string { return Binary }

// Run delegates the verbatim block body to the hurl binary.
func (*Executor) Run(ctx blocks.StepCtx) blocks.StepResult {
	output, captures, err := Execute(ctx.WorkDir, ctx.Body, ctx.Vars)
	if err != nil {
		return blocks.StepResult{
			Status: blocks.StatusFail,
			Detail: fmt.Sprintf("hurl step failed: %v", err),
			Output: blocks.Truncate(output),
		}
	}
	return blocks.StepResult{
		Status:   blocks.StatusPass,
		Output:   blocks.Truncate(output),
		Captures: captures,
	}
}

// Execute writes one Hurl document to a file in workDir and delegates it to
// the hurl binary, passing vars as `--variable name=value` flags. It returns
// the run's human output and the [Captures] extracted from hurl's JSON
// report. The graphql block composes onto this same engine (REQ:
// graphql-block).
func Execute(workDir, content string, vars []blocks.Capture) (string, []blocks.Capture, error) {
	file, err := createTempFn(workDir, ".rehearse-step-*.hurl")
	if err != nil {
		return "", nil, fmt.Errorf("creating hurl file: %v", err)
	}
	path := file.Name()
	_ = file.Close()
	defer func() { _ = os.Remove(path) }()
	if err := writeFileFn(path, []byte(content), 0o600); err != nil {
		return "", nil, fmt.Errorf("writing hurl file: %v", err)
	}

	// Capability detection: hurl >= 4.3 has `--report-json <dir>` (the test
	// report carrying captures); older versions expose the same JSON shape on
	// stdout via `--json`.
	help, err := helpOutputFn()
	if err != nil {
		return "", nil, fmt.Errorf("probing hurl capabilities (hurl --help): %v", err)
	}
	varArgs := make([]string, 0, 2*len(vars))
	for _, v := range vars {
		varArgs = append(varArgs, "--variable", v.Name+"="+v.Value)
	}
	if strings.Contains(help, "--report-json") {
		return runWithReportJSON(workDir, path, varArgs)
	}
	return runWithJSONOutput(workDir, path, varArgs)
}

// runWithReportJSON executes `hurl --test --report-json <dir>` and extracts
// captures from <dir>/report.json (REQ: hurl-block, REQ: context-bag).
func runWithReportJSON(workDir, hurlFile string, varArgs []string) (string, []blocks.Capture, error) {
	reportDir, err := mkdirTempFn(workDir, ".hurl-report-*")
	if err != nil {
		return "", nil, fmt.Errorf("creating hurl report dir: %v", err)
	}
	defer func() { _ = os.RemoveAll(reportDir) }()

	args := append([]string{"--test", "--report-json", reportDir}, varArgs...)
	cmd := command(workDir, append(args, hurlFile))
	var out bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &out
	if err := runCmdFn(cmd); err != nil {
		return out.String(), nil, fmt.Errorf("hurl --test: %v", err)
	}
	data, err := readFileFn(filepath.Join(reportDir, "report.json"))
	if err != nil {
		return out.String(), nil, fmt.Errorf("reading hurl JSON report: %v", err)
	}
	captures, err := parseCaptures(data)
	return out.String(), captures, err
}

// runWithJSONOutput executes `hurl --json` (pre-4.3 fallback) and extracts
// captures from the JSON result on stdout; stderr is the human output.
func runWithJSONOutput(workDir, hurlFile string, varArgs []string) (string, []blocks.Capture, error) {
	cmd := command(workDir, append(append([]string{"--json"}, varArgs...), hurlFile))
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := runCmdFn(cmd); err != nil {
		return stderr.String(), nil, fmt.Errorf("hurl --json: %v", err)
	}
	captures, err := parseCaptures(stdout.Bytes())
	return stderr.String(), captures, err
}

// command builds the hurl invocation, rooted in the scenario working dir.
func command(workDir string, args []string) *exec.Cmd {
	cmd := exec.Command(Binary, args...)
	cmd.Dir = workDir
	return cmd
}

// jsonTestcase is the per-file result in hurl's JSON output — the shape is
// shared by the `--report-json` report (an array of these) and the legacy
// `--json` stdout (a single one).
type jsonTestcase struct {
	Entries []struct {
		Captures []struct {
			Name  string `json:"name"`
			Value any    `json:"value"`
		} `json:"captures"`
	} `json:"entries"`
}

// parseCaptures extracts the [Captures] name/value pairs, in document order,
// from hurl's JSON report (REQ: context-bag).
func parseCaptures(data []byte) ([]blocks.Capture, error) {
	trimmed := bytes.TrimSpace(data)
	var cases []jsonTestcase
	if len(trimmed) > 0 && trimmed[0] == '[' {
		if err := decodeJSON(trimmed, &cases); err != nil {
			return nil, fmt.Errorf("parsing hurl JSON report: %v", err)
		}
	} else {
		var one jsonTestcase
		if err := decodeJSON(trimmed, &one); err != nil {
			return nil, fmt.Errorf("parsing hurl JSON result: %v", err)
		}
		cases = []jsonTestcase{one}
	}
	var captures []blocks.Capture
	for _, c := range cases {
		for _, e := range c.Entries {
			for _, capt := range e.Captures {
				captures = append(captures, blocks.Capture{Name: capt.Name, Value: captureValue(capt.Value)})
			}
		}
	}
	return captures, nil
}

// decodeJSON unmarshals with number fidelity: JSON numbers stay json.Number
// so an integer capture round-trips as "42", not "4.2e+01".
func decodeJSON(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	return dec.Decode(v)
}

// captureValue renders one captured value as the bag's string form: strings
// verbatim, numbers with fidelity, anything else (bool, null, structured) as
// its JSON text.
func captureValue(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	default:
		// The value was decoded from JSON, so re-marshalling cannot fail.
		data, _ := json.Marshal(t)
		return string(data)
	}
}
