// Package bash implements the ```bash rehearse step block: the block body
// runs via `bash -euo pipefail` in the scenario's working directory; a
// non-zero exit fails the step (REQ: bash-block). The step captures context
// bag variables by appending `name=value` lines to the file at
// `$REHEARSE_CAPTURES` (REQ: context-bag).
package bash

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"strings"

	"github.com/specscore/specscore-cli/internal/rehearse/blocks"
)

// Test seams — package-level vars wrapping external functions.
var (
	createTempFn = os.CreateTemp
	readFileFn   = os.ReadFile
)

// Executor runs bash step blocks.
type Executor struct{}

// New returns the bash block executor.
func New() *Executor { return &Executor{} }

// Kind returns "bash".
func (*Executor) Kind() string { return "bash" }

// Run executes the block body via `bash -euo pipefail -c`, capturing
// combined stdout/stderr (truncated per blocks.Truncate). The step gets a
// fresh empty file at $REHEARSE_CAPTURES; the `name=value` lines it appends
// there become the step's captures (REQ: context-bag).
func (*Executor) Run(ctx blocks.StepCtx) blocks.StepResult {
	capturesFile, err := createTempFn(ctx.WorkDir, ".rehearse-captures-*")
	if err != nil {
		return fail("creating $REHEARSE_CAPTURES file: %v", err)
	}
	capturesPath := capturesFile.Name()
	_ = capturesFile.Close()
	defer func() { _ = os.Remove(capturesPath) }()

	cmd := exec.Command("bash", "-euo", "pipefail", "-c", ctx.Body)
	cmd.Dir = ctx.WorkDir
	cmd.Env = append(os.Environ(), "REHEARSE_CAPTURES="+capturesPath)
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	runErr := cmd.Run()
	res := blocks.StepResult{Status: blocks.StatusPass, Output: blocks.Truncate(out.String())}
	if runErr != nil {
		res.Status = blocks.StatusFail
		res.Detail = fmt.Sprintf("bash step failed: %v", runErr)
		return res
	}
	captures, err := parseCaptures(capturesPath)
	if err != nil {
		res.Status = blocks.StatusFail
		res.Detail = fmt.Sprintf("bash step failed: %v", err)
		return res
	}
	res.Captures = captures
	return res
}

// parseCaptures reads the step's $REHEARSE_CAPTURES file: one `name=value`
// line per capture, blank lines ignored. Any other line is an error so a
// malformed capture fails loudly instead of silently dropping a variable.
func parseCaptures(path string) ([]blocks.Capture, error) {
	data, err := readFileFn(path)
	if err != nil {
		return nil, fmt.Errorf("reading $REHEARSE_CAPTURES file: %v", err)
	}
	var captures []blocks.Capture
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		name, value, ok := strings.Cut(line, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" {
			return nil, fmt.Errorf("$REHEARSE_CAPTURES line %q does not match `name=value`", line)
		}
		captures = append(captures, blocks.Capture{Name: name, Value: value})
	}
	return captures, nil
}

// fail builds a failing StepResult with a formatted diagnostic.
func fail(format string, args ...any) blocks.StepResult {
	return blocks.StepResult{Status: blocks.StatusFail, Detail: fmt.Sprintf("bash step failed: "+format, args...)}
}
