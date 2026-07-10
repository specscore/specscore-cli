package runner

import (
	"bytes"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/internal/rehearse/blocks"
)

// binaryBlock is a fake executor that declares an external-binary dependency
// (the shape of the hurl and graphql executors).
type binaryBlock struct {
	fakeBlock
	binary string
}

func (b binaryBlock) RequiredBinary() string { return b.binary }

// noLookPath makes every binary lookup fail, simulating a PATH without the
// binary.
func noLookPath(t *testing.T) {
	t.Helper()
	restore := lookPathFn
	lookPathFn = func(name string) (string, error) { return "", errors.New(name + " not found") }
	t.Cleanup(func() { lookPathFn = restore })
}

// TestRun_MissingBinarySkipsScenarioUpfront pins REQ hurl-block's upfront
// scan: a scenario containing a hurl-derived block is skipped before ANY of
// its steps run — including an earlier bash-like step — and the warning
// names the missing binary.
func TestRun_MissingBinarySkipsScenarioUpfront(t *testing.T) {
	noLookPath(t)
	executed := false
	reg := blocks.NewRegistry(
		fakeBlock{kind: "bash", run: func(blocks.StepCtx) blocks.StepResult {
			executed = true
			return blocks.StepResult{Status: blocks.StatusPass}
		}},
		binaryBlock{fakeBlock: fakeBlock{kind: "hurl", run: func(blocks.StepCtx) blocks.StepResult {
			executed = true
			return blocks.StepResult{Status: blocks.StatusPass}
		}}, binary: "hurl"},
	)
	file := filepath.Join(t.TempDir(), "skip.md")
	writeFile(t, file, "**Verifies:** demo/x#ac:one\n\n```bash\necho first\n```\n\n```hurl\nGET http://x\n```\n")

	r := Run(reg, []string{file})[0]
	if r.Status != StatusSkipped {
		t.Fatalf("status = %q, want skipped (detail: %s)", r.Status, r.Detail)
	}
	if executed {
		t.Error("a step executed despite the upfront skip")
	}
	if len(r.Steps) != 0 {
		t.Errorf("steps = %+v, want none reported (none ran)", r.Steps)
	}
	if !strings.Contains(r.Detail, `"hurl"`) || !strings.Contains(r.Detail, "not on PATH") {
		t.Errorf("warning does not name the missing binary: %q", r.Detail)
	}
	// Skips do not affect the exit code.
	if CountFailed([]ScenarioReport{r}) != 0 {
		t.Error("a skipped scenario must not count as failed")
	}
}

// TestRun_BinaryPresentRunsScenario: when the binary resolves, the scan is a
// no-op and the steps run (one lookup per distinct binary).
func TestRun_BinaryPresentRunsScenario(t *testing.T) {
	var lookups []string
	restore := lookPathFn
	lookPathFn = func(name string) (string, error) {
		lookups = append(lookups, name)
		return "/usr/bin/" + name, nil
	}
	t.Cleanup(func() { lookPathFn = restore })

	reg := blocks.NewRegistry(
		binaryBlock{fakeBlock: fakeBlock{kind: "hurl", run: func(blocks.StepCtx) blocks.StepResult {
			return blocks.StepResult{Status: blocks.StatusPass}
		}}, binary: "hurl"},
		binaryBlock{fakeBlock: fakeBlock{kind: "graphql", run: func(blocks.StepCtx) blocks.StepResult {
			return blocks.StepResult{Status: blocks.StatusPass}
		}}, binary: "hurl"},
	)
	file := filepath.Join(t.TempDir(), "run.md")
	writeFile(t, file, "```hurl\nGET http://x\n```\n\n```graphql url=http://x\nquery { ok }\n```\n")

	r := Run(reg, []string{file})[0]
	if r.Status != StatusPass {
		t.Fatalf("status = %q, want pass (detail: %s)", r.Status, r.Detail)
	}
	if len(lookups) != 1 || lookups[0] != "hurl" {
		t.Errorf("lookups = %v, want exactly one for the shared binary", lookups)
	}
}

// TestRun_NoBinaryDependentBlocksSkipsNothing: a bash-only scenario runs even
// on a PATH where every external-binary lookup would fail.
func TestRun_NoBinaryDependentBlocksSkipsNothing(t *testing.T) {
	noLookPath(t)
	file := filepath.Join(t.TempDir(), "plain.md")
	writeFile(t, file, "```bash\necho ok\n```\n")

	r := Run(bashRegistry(), []string{file})[0]
	if r.Status != StatusPass {
		t.Errorf("status = %q, want pass (detail: %s)", r.Status, r.Detail)
	}
}

// TestRenderHuman_SkippedWarningIsPrinted: the human report surfaces the
// skipped scenario's missing-binary warning (REQ: hurl-block).
func TestRenderHuman_SkippedWarningIsPrinted(t *testing.T) {
	var buf bytes.Buffer
	RenderHuman(&buf, []ScenarioReport{{
		File:     "s.md",
		Status:   StatusSkipped,
		Verifies: []string{"demo/x#ac:one"},
		Detail:   `warning: scenario skipped — it needs the "hurl" binary, which is not on PATH`,
	}})
	out := buf.String()
	if !strings.Contains(out, "skipped") || !strings.Contains(out, `"hurl"`) {
		t.Errorf("report does not surface the warning:\n%s", out)
	}
	if !strings.Contains(out, "1 skipped") {
		t.Errorf("totals line does not count the skip:\n%s", out)
	}
}
