// Package bash implements the ```bash rehearse step block: the block body
// runs via `bash -euo pipefail` in the scenario's working directory; a
// non-zero exit fails the step (REQ: bash-block).
package bash

import (
	"bytes"
	"fmt"
	"os/exec"

	"github.com/specscore/specscore-cli/internal/rehearse/blocks"
)

// Executor runs bash step blocks.
type Executor struct{}

// New returns the bash block executor.
func New() *Executor { return &Executor{} }

// Kind returns "bash".
func (*Executor) Kind() string { return "bash" }

// Run executes the block body via `bash -euo pipefail -c`, capturing
// combined stdout/stderr (truncated per blocks.Truncate).
func (*Executor) Run(ctx blocks.StepCtx) blocks.StepResult {
	cmd := exec.Command("bash", "-euo", "pipefail", "-c", ctx.Body)
	cmd.Dir = ctx.WorkDir
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out

	err := cmd.Run()
	res := blocks.StepResult{Status: blocks.StatusPass, Output: blocks.Truncate(out.String())}
	if err != nil {
		res.Status = blocks.StatusFail
		res.Detail = fmt.Sprintf("bash step failed: %v", err)
	}
	return res
}
