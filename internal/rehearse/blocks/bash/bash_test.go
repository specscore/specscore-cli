package bash_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/internal/rehearse/blocks"
	"github.com/specscore/specscore-cli/internal/rehearse/blocks/bash"
)

func TestKind(t *testing.T) {
	if got := bash.New().Kind(); got != "bash" {
		t.Fatalf("Kind() = %q, want bash", got)
	}
}

func TestRun_PassCapturesOutput(t *testing.T) {
	res := bash.New().Run(blocks.StepCtx{WorkDir: t.TempDir(), Body: "echo hello; echo oops >&2"})
	if res.Status != blocks.StatusPass {
		t.Fatalf("status = %q, want pass (detail: %s)", res.Status, res.Detail)
	}
	if res.Detail != "" {
		t.Errorf("pass carries detail %q", res.Detail)
	}
	if !strings.Contains(res.Output, "hello") || !strings.Contains(res.Output, "oops") {
		t.Errorf("combined stdout/stderr not captured: %q", res.Output)
	}
}

func TestRun_NonZeroExitFails(t *testing.T) {
	res := bash.New().Run(blocks.StepCtx{WorkDir: t.TempDir(), Body: "echo before; exit 3"})
	if res.Status != blocks.StatusFail {
		t.Fatalf("status = %q, want fail", res.Status)
	}
	if !strings.Contains(res.Detail, "exit status 3") {
		t.Errorf("detail lacks the exit status: %q", res.Detail)
	}
	if !strings.Contains(res.Output, "before") {
		t.Errorf("output before the failure not captured: %q", res.Output)
	}
}

func TestRun_PipefailAndErrexitApply(t *testing.T) {
	// Without pipefail this pipeline exits 0; with -o pipefail it must fail.
	res := bash.New().Run(blocks.StepCtx{WorkDir: t.TempDir(), Body: "false | true"})
	if res.Status != blocks.StatusFail {
		t.Fatalf("pipefail not in effect: status = %q", res.Status)
	}
}

func TestRun_RunsInWorkDir(t *testing.T) {
	dir := t.TempDir()
	res := bash.New().Run(blocks.StepCtx{WorkDir: dir, Body: "touch marker"})
	if res.Status != blocks.StatusPass {
		t.Fatalf("status = %q, want pass (detail: %s)", res.Status, res.Detail)
	}
	if _, err := os.Stat(filepath.Join(dir, "marker")); err != nil {
		t.Errorf("step did not run in WorkDir: %v", err)
	}
}

func TestRun_OutputTruncatedAt8KiB(t *testing.T) {
	res := bash.New().Run(blocks.StepCtx{WorkDir: t.TempDir(), Body: `head -c 10000 /dev/zero | tr '\0' 'x'`})
	if res.Status != blocks.StatusPass {
		t.Fatalf("status = %q, want pass (detail: %s)", res.Status, res.Detail)
	}
	if !strings.Contains(res.Output, "[step output truncated at 8 KiB]") {
		t.Error("oversized output lacks the truncation note")
	}
	if strings.Count(res.Output, "x") != blocks.MaxStepOutput {
		t.Errorf("kept %d bytes of payload, want %d", strings.Count(res.Output, "x"), blocks.MaxStepOutput)
	}
}
