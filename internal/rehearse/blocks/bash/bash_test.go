package bash_test

import (
	"os"
	"path/filepath"
	"reflect"
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

func TestRun_CapturesFromRehearseCapturesFile(t *testing.T) {
	res := bash.New().Run(blocks.StepCtx{WorkDir: t.TempDir(), Body: `
echo "uid=42" >> "$REHEARSE_CAPTURES"
echo "" >> "$REHEARSE_CAPTURES"
echo "url=http://x/?a=1&b=2" >> "$REHEARSE_CAPTURES"
`})
	if res.Status != blocks.StatusPass {
		t.Fatalf("status = %q, want pass (detail: %s)", res.Status, res.Detail)
	}
	want := []blocks.Capture{
		{Name: "uid", Value: "42"},
		// The value keeps everything after the first "=", verbatim.
		{Name: "url", Value: "http://x/?a=1&b=2"},
	}
	if !reflect.DeepEqual(res.Captures, want) {
		t.Errorf("captures = %v, want %v", res.Captures, want)
	}
}

func TestRun_NoCapturesWrittenMeansNoCaptures(t *testing.T) {
	res := bash.New().Run(blocks.StepCtx{WorkDir: t.TempDir(), Body: "true"})
	if res.Status != blocks.StatusPass {
		t.Fatalf("status = %q, want pass (detail: %s)", res.Status, res.Detail)
	}
	if len(res.Captures) != 0 {
		t.Errorf("captures = %v, want none", res.Captures)
	}
}

func TestRun_MalformedCaptureLineFails(t *testing.T) {
	for name, line := range map[string]string{
		"no equals sign": "just-a-word",
		"empty name":     "=value",
	} {
		t.Run(name, func(t *testing.T) {
			res := bash.New().Run(blocks.StepCtx{WorkDir: t.TempDir(),
				Body: `echo '` + line + `' >> "$REHEARSE_CAPTURES"`})
			if res.Status != blocks.StatusFail {
				t.Fatalf("status = %q, want fail", res.Status)
			}
			if !strings.Contains(res.Detail, "does not match `name=value`") || !strings.Contains(res.Detail, line) {
				t.Errorf("detail does not name the malformed line: %q", res.Detail)
			}
		})
	}
}

func TestRun_CapturesFileIsPerStepAndRemoved(t *testing.T) {
	dir := t.TempDir()
	exec := bash.New()
	if res := exec.Run(blocks.StepCtx{WorkDir: dir, Body: `echo "uid=42" >> "$REHEARSE_CAPTURES"`}); res.Status != blocks.StatusPass {
		t.Fatalf("first step: %q (detail: %s)", res.Status, res.Detail)
	}
	// The next step starts with a fresh file: no stale uid capture.
	res := exec.Run(blocks.StepCtx{WorkDir: dir, Body: `true`})
	if res.Status != blocks.StatusPass {
		t.Fatalf("second step: %q (detail: %s)", res.Status, res.Detail)
	}
	if len(res.Captures) != 0 {
		t.Errorf("stale captures leaked across steps: %v", res.Captures)
	}
	// And no captures files linger in the shared working dir after the steps.
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading workdir: %v", err)
	}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".rehearse-captures-") {
			t.Errorf("captures file left behind in workdir: %s", e.Name())
		}
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
