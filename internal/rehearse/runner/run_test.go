package runner

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/internal/rehearse/blocks"
	"github.com/specscore/specscore-cli/internal/rehearse/blocks/bash"
)

func bashRegistry() blocks.Registry { return blocks.NewRegistry(bash.New()) }

// panicBlock is a broken executor used to prove panic recovery.
type panicBlock struct{}

func (panicBlock) Kind() string                         { return "boom" }
func (panicBlock) Run(blocks.StepCtx) blocks.StepResult { panic("kaboom") }

func TestRun_PassingScenario(t *testing.T) {
	file := filepath.Join(t.TempDir(), "pass.md")
	writeFile(t, file, "**Verifies:** demo/x#ac:one (REQ: y)\n\n```bash\necho hello\n```\n")

	reports := Run(bashRegistry(), []string{file})
	if len(reports) != 1 {
		t.Fatalf("reports = %d, want 1", len(reports))
	}
	r := reports[0]
	if r.Status != StatusPass {
		t.Fatalf("status = %q, want pass (detail: %s)", r.Status, r.Detail)
	}
	if r.File != file {
		t.Errorf("file = %q", r.File)
	}
	if !reflect.DeepEqual(r.Verifies, []string{"demo/x#ac:one"}) {
		t.Errorf("verifies = %v", r.Verifies)
	}
	if r.Bag == nil || len(r.Bag) != 0 {
		t.Errorf("bag = %v, want empty non-nil map", r.Bag)
	}
	if len(r.Steps) != 1 || r.Steps[0].Kind != "bash" || r.Steps[0].Status != StatusPass {
		t.Errorf("steps = %+v", r.Steps)
	}
	if !strings.Contains(r.Steps[0].Output, "hello") {
		t.Errorf("step output = %q", r.Steps[0].Output)
	}
	if r.DurationMS < 0 {
		t.Errorf("duration_ms = %d", r.DurationMS)
	}
}

func TestRun_FirstFailSkipsRemainingSteps(t *testing.T) {
	file := filepath.Join(t.TempDir(), "fail.md")
	writeFile(t, file, "```bash\necho ok\n```\n\n```bash\nexit 7\n```\n\n```bash\necho never\n```\n")

	r := Run(bashRegistry(), []string{file})[0]
	if r.Status != StatusFail {
		t.Fatalf("status = %q, want fail", r.Status)
	}
	if len(r.Steps) != 3 {
		t.Fatalf("steps = %d, want 3", len(r.Steps))
	}
	wantStatuses := []string{StatusPass, StatusFail, StepStatusSkipped}
	for i, want := range wantStatuses {
		if r.Steps[i].Status != want {
			t.Errorf("step %d status = %q, want %q", i, r.Steps[i].Status, want)
		}
	}
	if !strings.Contains(r.Steps[1].Detail, "exit status 7") {
		t.Errorf("failing step detail = %q", r.Steps[1].Detail)
	}
}

func TestRun_NoStepBlocksIsNoSteps(t *testing.T) {
	for name, content := range map[string]string{
		"no fenced blocks":         "# Prose only\n\n**Verifies:** demo/x#ac:one\n",
		"only documentation block": "```yaml\nkey: value\n```\n",
	} {
		t.Run(name, func(t *testing.T) {
			file := filepath.Join(t.TempDir(), "steps.md")
			writeFile(t, file, content)

			r := Run(bashRegistry(), []string{file})[0]
			if r.Status != StatusNoSteps {
				t.Fatalf("status = %q, want no-steps", r.Status)
			}
			if len(r.Steps) != 0 {
				t.Errorf("steps = %+v, want none", r.Steps)
			}
		})
	}
}

func TestRun_UnparsableScenarioIsReportedFail(t *testing.T) {
	file := filepath.Join(t.TempDir(), "broken.md")
	writeFile(t, file, "```bash\necho unclosed\n")

	r := Run(bashRegistry(), []string{file})[0]
	if r.Status != StatusFail {
		t.Fatalf("status = %q, want fail", r.Status)
	}
	if !strings.Contains(r.Detail, "unclosed fenced block") {
		t.Errorf("detail = %q", r.Detail)
	}
}

func TestRun_ExecutorPanicIsStepFail(t *testing.T) {
	file := filepath.Join(t.TempDir(), "panic.md")
	writeFile(t, file, "```boom\nanything\n```\n")

	r := Run(blocks.NewRegistry(panicBlock{}), []string{file})[0]
	if r.Status != StatusFail {
		t.Fatalf("status = %q, want fail", r.Status)
	}
	if !strings.Contains(r.Steps[0].Detail, "block executor panicked: kaboom") {
		t.Errorf("step detail = %q", r.Steps[0].Detail)
	}
}

func TestRun_WorkDirIsScenarioScopedAndCleanedUp(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "wd.md")
	// Step 1 writes a file into the scenario workdir; step 2 sees it (shared
	// dir) and prints the dir so the test can check post-run cleanup.
	writeFile(t, file, "```bash\necho seed > state.txt\n```\n\n```bash\ncat state.txt\npwd\n```\n")

	r := Run(bashRegistry(), []string{file})[0]
	if r.Status != StatusPass {
		t.Fatalf("status = %q, want pass (steps: %+v)", r.Status, r.Steps)
	}
	if !strings.Contains(r.Steps[1].Output, "seed") {
		t.Errorf("steps do not share one working dir: %q", r.Steps[1].Output)
	}
	lines := strings.Fields(strings.TrimSpace(r.Steps[1].Output))
	workDir := lines[len(lines)-1]
	if _, err := os.Stat(workDir); !os.IsNotExist(err) {
		t.Errorf("scenario working dir %s not cleaned up (stat err: %v)", workDir, err)
	}
}

// TestRun_FileAssertionPasses covers the runScenario file-assertion loop when
// a bash step creates a file that a `### Assert: file … exists` heading checks.
func TestRun_FileAssertionPasses(t *testing.T) {
	file := filepath.Join(t.TempDir(), "assert-pass.md")
	writeFile(t, file, "```bash\necho hi > out.txt\n```\n\n### Assert: file `out.txt` exists\n")

	r := Run(bashRegistry(), []string{file})[0]
	if r.Status != StatusPass {
		t.Fatalf("status = %q, want pass (steps: %+v)", r.Status, r.Steps)
	}
	// A passing assertion is silent: only the bash step is reported.
	if len(r.Steps) != 1 || r.Steps[0].Kind != "bash" {
		t.Errorf("steps = %+v, want a single bash step", r.Steps)
	}
}

// TestRun_FileAssertionFails covers the failure branch: a `### Assert: file …
// exists` heading for a file no step created fails the scenario and appends a
// synthetic "file" step carrying the assertion message.
func TestRun_FileAssertionFails(t *testing.T) {
	file := filepath.Join(t.TempDir(), "assert-fail.md")
	writeFile(t, file, "```bash\necho hi\n```\n\n### Assert: file `absent.txt` exists\n")

	r := Run(bashRegistry(), []string{file})[0]
	if r.Status != StatusFail {
		t.Fatalf("status = %q, want fail (steps: %+v)", r.Status, r.Steps)
	}
	last := r.Steps[len(r.Steps)-1]
	if last.Kind != "file" || last.Status != blocks.StatusFail {
		t.Errorf("last step = %+v, want a failed file step", last)
	}
	if !strings.Contains(last.Detail, "absent.txt") {
		t.Errorf("file step detail = %q, want it to name the missing file", last.Detail)
	}
}

// TestRun_ExpectFail_FailingScenarioReportsPass — a scenario with
// `**Expect:** fail` whose step fails is inverted to pass; the report carries
// Expect=="fail" and retains the failing step for transparency.
func TestRun_ExpectFail_FailingScenarioReportsPass(t *testing.T) {
	file := filepath.Join(t.TempDir(), "neg.md")
	writeFile(t, file, "**Expect:** fail\n\n```bash\nexit 3\n```\n")

	r := Run(bashRegistry(), []string{file})[0]
	if r.Status != StatusPass {
		t.Fatalf("status = %q, want pass (expected-fail inversion)", r.Status)
	}
	if r.Expect != "fail" {
		t.Errorf("Expect = %q, want \"fail\"", r.Expect)
	}
	if len(r.Steps) != 1 || r.Steps[0].Status != blocks.StatusFail {
		t.Errorf("steps = %+v, want the failing step retained", r.Steps)
	}
}

// TestRun_ExpectFail_PassingScenarioReportsFail — a scenario with
// `**Expect:** fail` whose steps all pass is inverted to fail with an
// explanatory detail.
func TestRun_ExpectFail_PassingScenarioReportsFail(t *testing.T) {
	file := filepath.Join(t.TempDir(), "pos.md")
	writeFile(t, file, "**Expect:** fail\n\n```bash\necho ok\n```\n")

	r := Run(bashRegistry(), []string{file})[0]
	if r.Status != StatusFail {
		t.Fatalf("status = %q, want fail (expected fail but passed)", r.Status)
	}
	if !strings.Contains(r.Detail, "expected to fail") {
		t.Errorf("detail = %q, want it to note the scenario was expected to fail", r.Detail)
	}
}

func TestRun_MkdirTempFailureIsScenarioFail(t *testing.T) {
	swap(t, &mkdirTempFn, func(string, string) (string, error) {
		return "", errors.New("tmp full")
	})
	file := filepath.Join(t.TempDir(), "tmp.md")
	writeFile(t, file, "```bash\necho hi\n```\n")

	r := Run(bashRegistry(), []string{file})[0]
	if r.Status != StatusFail {
		t.Fatalf("status = %q, want fail", r.Status)
	}
	if !strings.Contains(r.Detail, "tmp full") {
		t.Errorf("detail = %q", r.Detail)
	}
}

func TestRenderHuman_LinesAndTotals(t *testing.T) {
	var buf bytes.Buffer
	RenderHuman(&buf, []ScenarioReport{
		{File: "a.md", Status: StatusPass, Verifies: []string{"x#ac:1", "x#ac:2"}, DurationMS: 12},
		{File: "b.md", Status: StatusFail, Verifies: []string{}, Detail: "parse broke",
			Steps: []StepReport{{Kind: "bash", Status: StatusFail, Detail: "exit status 1", Output: "line1\nline2\n"}}},
		{File: "c.md", Status: StatusNoSteps, Verifies: []string{}},
		// No scenario-level detail, a passing step before the failure, and a
		// failing step with no captured output.
		{File: "d.md", Status: StatusFail, Verifies: []string{},
			Steps: []StepReport{
				{Kind: "bash", Status: StatusPass, Output: "fine"},
				{Kind: "bash", Status: StatusFail, Detail: "exit status 2"},
			}},
		// A nested-suite case is labelled `file › When …`.
		{File: "e.md", Case: "When it branches", Status: StatusPass, Verifies: []string{}, DurationMS: 5},
	})
	out := buf.String()
	for _, want := range []string{
		"pass", "a.md", "[x#ac:1, x#ac:2]", "12ms",
		"fail", "b.md",
		"    parse broke",
		"    bash step: exit status 1",
		"    line1",
		"    line2",
		"no-steps", "c.md",
		"    bash step: exit status 2",
		"e.md › When it branches",
		"Total: 5 scenario(s) — 2 pass, 2 fail, 0 skipped, 1 no-steps",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("human report missing %q; got:\n%s", want, out)
		}
	}
}

func TestRenderHuman_PassingStepOutputNotDumped(t *testing.T) {
	var buf bytes.Buffer
	RenderHuman(&buf, []ScenarioReport{
		{File: "a.md", Status: StatusPass, Verifies: []string{},
			Steps: []StepReport{{Kind: "bash", Status: StatusPass, Output: "chatty output"}}},
	})
	if strings.Contains(buf.String(), "chatty output") {
		t.Errorf("passing step output leaked into the human report:\n%s", buf.String())
	}
}

func TestCountFailed(t *testing.T) {
	reports := []ScenarioReport{
		{Status: StatusPass}, {Status: StatusFail}, {Status: StatusNoSteps}, {Status: StatusFail},
	}
	if got := CountFailed(reports); got != 2 {
		t.Errorf("CountFailed = %d, want 2", got)
	}
}

// --- reusable checks (**Use:**) ---

func TestRun_Check_Passes(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "ok.check.md"), "# Check: ok\n\n```bash\nexit 0\n```\n")
	file := filepath.Join(dir, "s.md")
	writeFile(t, file, "```bash\necho hi\n```\n\n**Use:** [ok](./ok.check.md)\n")

	r := Run(bashRegistry(), []string{file})[0]
	if r.Status != StatusPass {
		t.Fatalf("status = %q, want pass (steps %+v)", r.Status, r.Steps)
	}
	// A passing check is silent: only the bash step is reported.
	if len(r.Steps) != 1 {
		t.Errorf("steps = %+v, want just the bash step", r.Steps)
	}
}

func TestRun_Check_Fails(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "bad.check.md"), "# Check: bad\n\n```bash\necho boom; exit 1\n```\n")
	file := filepath.Join(dir, "s.md")
	writeFile(t, file, "**Use:** [bad](./bad.check.md)\n")

	r := Run(bashRegistry(), []string{file})[0]
	if r.Status != StatusFail {
		t.Fatalf("status = %q, want fail", r.Status)
	}
	last := r.Steps[len(r.Steps)-1]
	if last.Kind != "use" || !strings.Contains(last.Detail, "bad.check.md") {
		t.Errorf("last step = %+v, want a failed use naming the check", last)
	}
}

func TestRun_Check_ParamBinding(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "contains.check.md"),
		"# Check: contains\n**Params:** expected\n\n```bash\ngrep -q \"{{expected}}\" out.txt || exit 1\n```\n")
	file := filepath.Join(dir, "s.md")
	writeFile(t, file, "```bash\necho hello > out.txt\n```\n\n**Use:** [contains](./contains.check.md) with expected=hello\n")

	r := Run(bashRegistry(), []string{file})[0]
	if r.Status != StatusPass {
		t.Fatalf("status = %q, want pass (steps %+v)", r.Status, r.Steps)
	}
}

func TestRun_Check_ContextParam(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "tok.check.md"),
		"# Check: tok\n**Params:** value\n\n```bash\n[ \"{{value}}\" = \"secret42\" ] || exit 1\n```\n")
	file := filepath.Join(dir, "s.md")
	writeFile(t, file, "```bash\necho \"token=secret42\" >> \"$REHEARSE_CAPTURES\"\n```\n\n**Use:** [tok](./tok.check.md) with value={{token}}\n")

	r := Run(bashRegistry(), []string{file})[0]
	if r.Status != StatusPass {
		t.Fatalf("status = %q, want pass (steps %+v)", r.Status, r.Steps)
	}
}

func TestRun_Check_MissingCheck(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "s.md")
	writeFile(t, file, "**Use:** [nope](./nope.check.md)\n")

	r := Run(bashRegistry(), []string{file})[0]
	if r.Status != StatusFail {
		t.Fatalf("status = %q, want fail", r.Status)
	}
	if !strings.Contains(r.Steps[len(r.Steps)-1].Detail, "cannot load check") {
		t.Errorf("detail = %q, want a load error", r.Steps[len(r.Steps)-1].Detail)
	}
}

func TestRun_Check_MissingParam(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "needs.check.md"), "# Check: needs\n**Params:** expected\n\n```bash\ntrue\n```\n")
	file := filepath.Join(dir, "s.md")
	writeFile(t, file, "**Use:** [needs](./needs.check.md)\n")

	r := Run(bashRegistry(), []string{file})[0]
	if r.Status != StatusFail {
		t.Fatalf("status = %q, want fail", r.Status)
	}
	if !strings.Contains(r.Steps[len(r.Steps)-1].Detail, "missing required param") {
		t.Errorf("detail = %q, want a missing-param error", r.Steps[len(r.Steps)-1].Detail)
	}
}

func TestRun_Check_BadParamRef(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "c.check.md"), "# Check: c\n**Params:** value\n\n```bash\ntrue\n```\n")
	file := filepath.Join(dir, "s.md")
	writeFile(t, file, "**Use:** [c](./c.check.md) with value={{unknown}}\n")

	r := Run(bashRegistry(), []string{file})[0]
	if r.Status != StatusFail {
		t.Fatalf("status = %q, want fail", r.Status)
	}
	if !strings.Contains(r.Steps[len(r.Steps)-1].Detail, "unknown variable") {
		t.Errorf("detail = %q, want an interpolation error", r.Steps[len(r.Steps)-1].Detail)
	}
}

func TestRun_Check_BadBody(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "c.check.md"), "# Check: c\n\n```bash\necho {{unbound}}\n```\n")
	file := filepath.Join(dir, "s.md")
	writeFile(t, file, "**Use:** [c](./c.check.md)\n")

	r := Run(bashRegistry(), []string{file})[0]
	if r.Status != StatusFail {
		t.Fatalf("status = %q, want fail", r.Status)
	}
	if !strings.Contains(r.Steps[len(r.Steps)-1].Detail, "unknown variable") {
		t.Errorf("detail = %q, want a body-interpolation error", r.Steps[len(r.Steps)-1].Detail)
	}
}

// --- nested scenario suites (describe/context/it) ---

func TestRun_Suite_TwoBranchesEachACase(t *testing.T) {
	file := filepath.Join(t.TempDir(), "suite.md")
	// Shared Given writes a seed file; each When (branch) reads it — proving the
	// Given setup runs per branch, in that branch's own workdir.
	writeFile(t, file, "**Verifies:** demo#ac:x\n\n"+
		"## Given a seed file\n\n```bash\necho seed > s.txt\n```\n\n"+
		"### When the file is present\n\n```bash\ntest -f s.txt\n```\n\n"+
		"### When the seed is read\n\n```bash\ngrep -q seed s.txt\n```\n")

	reports := Run(bashRegistry(), []string{file})
	if len(reports) != 2 {
		t.Fatalf("reports = %d, want 2 (one per When branch)", len(reports))
	}
	for _, r := range reports {
		if r.Status != StatusPass {
			t.Errorf("case %q status = %q, want pass (steps %+v)", r.Case, r.Status, r.Steps)
		}
		if r.Case == "" || r.File != file {
			t.Errorf("case report = %+v, want File+Case set", r)
		}
		if len(r.Verifies) != 1 || r.Verifies[0] != "demo#ac:x" {
			t.Errorf("case %q Verifies = %v, want the file-level AC", r.Case, r.Verifies)
		}
	}
	if reports[0].Case == reports[1].Case {
		t.Errorf("both cases share a label: %q", reports[0].Case)
	}
}

func TestRun_Suite_BranchesRunIsolated(t *testing.T) {
	file := filepath.Join(t.TempDir(), "suite.md")
	// One branch passes, one fails: they must be reported independently, and the
	// failing branch's fresh workdir must not affect the passing one.
	writeFile(t, file, "## Given nothing special\n\n```bash\necho ok\n```\n\n"+
		"### When it passes\n\n```bash\ntrue\n```\n\n"+
		"### When it fails\n\n```bash\nexit 1\n```\n")

	reports := Run(bashRegistry(), []string{file})
	if len(reports) != 2 {
		t.Fatalf("reports = %d, want 2", len(reports))
	}
	var pass, fail int
	for _, r := range reports {
		switch r.Status {
		case StatusPass:
			pass++
		case StatusFail:
			fail++
		}
	}
	if pass != 1 || fail != 1 {
		t.Errorf("want exactly 1 pass + 1 fail, got pass=%d fail=%d: %+v", pass, fail, reports)
	}
}

func TestRun_MissingFileIsReportedFail(t *testing.T) {
	r := Run(bashRegistry(), []string{filepath.Join(t.TempDir(), "nope.md")})[0]
	if r.Status != StatusFail {
		t.Fatalf("status = %q, want fail", r.Status)
	}
	if !strings.Contains(r.Detail, "reading scenario") {
		t.Errorf("detail = %q, want a read error", r.Detail)
	}
}
