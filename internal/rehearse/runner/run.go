package runner

import (
	"fmt"
	"os"
	"time"

	"github.com/specscore/specscore-cli/internal/rehearse/blocks"
	"github.com/specscore/specscore-cli/internal/rehearse/scenario"
)

// Scenario statuses (REQ: scenario-shape, REQ: run-report).
const (
	StatusPass    = "pass"
	StatusFail    = "fail"
	StatusSkipped = "skipped"
	StatusNoSteps = "no-steps"
)

// StepStatusSkipped marks steps after the first failing step
// (REQ: scenario-shape).
const StepStatusSkipped = "skipped-after-failure"

// StepReport is one executed (or skipped) step in the JSON report
// (REQ: run-report).
type StepReport struct {
	Kind   string `json:"kind"`
	Status string `json:"status"`
	Detail string `json:"detail,omitempty"`
	Output string `json:"output,omitempty"`
}

// ScenarioReport is one scenario's outcome in the JSON report
// (REQ: run-report).
type ScenarioReport struct {
	File       string            `json:"file"`
	Status     string            `json:"status"`
	Verifies   []string          `json:"verifies"`
	DurationMS int64             `json:"duration_ms"`
	Bag        map[string]string `json:"bag"`
	Steps      []StepReport      `json:"steps"`
	// Detail carries scenario-level failure detail (e.g. a parse error).
	Detail string `json:"detail,omitempty"`
	// FilterStatus is "match" or "skip" when --filter is active; omitted otherwise.
	// Feature: cli/rehearse/run-filter (REQ: filter-output-labels)
	FilterStatus string `json:"filter_status,omitempty"`
}

// Run executes the scenario files in order and returns one report per
// scenario. Failures never abort the run; they are reported per scenario.
func Run(reg blocks.Registry, files []string) []ScenarioReport {
	reports := make([]ScenarioReport, 0, len(files))
	for _, file := range files {
		reports = append(reports, runScenario(reg, file))
	}
	return reports
}

// runScenario parses and executes one scenario file: steps run in order in
// one scenario-scoped temp working dir; the first failing step fails the
// scenario and the remaining steps are skipped-after-failure; a scenario
// with zero step blocks is no-steps (REQ: scenario-shape).
func runScenario(reg blocks.Registry, file string) ScenarioReport {
	start := time.Now()
	bag := NewBag()
	rep := ScenarioReport{
		File:     file,
		Verifies: []string{},
		Steps:    []StepReport{},
	}
	finish := func(status string) ScenarioReport {
		rep.Status = status
		rep.Bag = bag.Map()
		rep.DurationMS = time.Since(start).Milliseconds()
		return rep
	}

	sc, err := scenario.Parse(file)
	if err != nil {
		// An unparsable scenario is a reported fail, not a run abort.
		rep.Detail = err.Error()
		return finish(StatusFail)
	}
	rep.Verifies = sc.Verifies

	steps := stepBlocks(reg, sc.Blocks)
	if len(steps) == 0 {
		return finish(StatusNoSteps)
	}

	// Upfront missing-binary scan: a scenario containing any binary-dependent
	// block (hurl-derived) whose binary is not on PATH is skipped before ANY
	// of its steps run, with a warning naming the binary; the exit code is
	// unaffected (REQ: hurl-block, REQ: graphql-block).
	if binary := missingBinary(reg, steps); binary != "" {
		rep.Detail = fmt.Sprintf("warning: scenario skipped — it needs the %q binary, which is not on PATH", binary)
		return finish(StatusSkipped)
	}

	workDir, err := mkdirTempFn("", "rehearse-scenario-")
	if err != nil {
		rep.Detail = fmt.Sprintf("creating scenario working dir: %v", err)
		return finish(StatusFail)
	}
	defer func() { _ = os.RemoveAll(workDir) }()

	failed := false
	for _, b := range steps {
		if failed {
			rep.Steps = append(rep.Steps, StepReport{Kind: b.Kind, Status: StepStatusSkipped})
			continue
		}
		stepCtx, err := stepContext(bag, workDir, b)
		if err != nil {
			// Interpolation failed: the step fails without dispatching.
			rep.Steps = append(rep.Steps, StepReport{Kind: b.Kind, Status: blocks.StatusFail, Detail: err.Error()})
			failed = true
			continue
		}
		res := runStep(reg[b.Kind], stepCtx)
		rep.Steps = append(rep.Steps, StepReport{
			Kind:   b.Kind,
			Status: res.Status,
			Detail: res.Detail,
			Output: res.Output,
		})
		bag.Merge(res.Captures)
		failed = failed || res.Status == blocks.StatusFail
	}
	if failed {
		return finish(StatusFail)
	}
	return finish(StatusPass)
}

// textInterpolatedKinds are the block classes whose bodies and info-string
// params get {{name}} textual interpolation from the context bag before
// dispatch (REQ: context-bag). Hurl-derived kinds (hurl, graphql) are
// deliberately absent: Hurl owns the {{name}} syntax natively, so they
// receive the bag through StepCtx.Vars and pass it on as --variable flags.
var textInterpolatedKinds = map[string]bool{"bash": true, "sql": true, "dtql": true}

// stepContext builds one step's StepCtx from the context bag: every kind
// gets the ordered bag snapshot in Vars; text-interpolated kinds additionally
// get {{name}} resolved in the body and info-string param values. An unknown
// variable is an error naming it — the step fails before dispatch.
func stepContext(bag *Bag, workDir string, b scenario.Block) (blocks.StepCtx, error) {
	ctx := blocks.StepCtx{WorkDir: workDir, Body: b.Body, Params: b.Params, Vars: bag.Snapshot()}
	if !textInterpolatedKinds[b.Kind] {
		return ctx, nil
	}
	body, err := bag.Interpolate(b.Body)
	if err != nil {
		return ctx, fmt.Errorf("%s step failed: %v", b.Kind, err)
	}
	params := make(map[string]string, len(b.Params))
	for key, value := range b.Params {
		interpolated, err := bag.Interpolate(value)
		if err != nil {
			return ctx, fmt.Errorf("%s step failed: info-string param %s: %v", b.Kind, key, err)
		}
		params[key] = interpolated
	}
	ctx.Body, ctx.Params = body, params
	return ctx, nil
}

// missingBinary scans the scenario's step kinds for executors that delegate
// to an external binary and returns the first required binary not found on
// PATH ("" when everything needed is available).
func missingBinary(reg blocks.Registry, steps []scenario.Block) string {
	checked := map[string]bool{}
	for _, b := range steps {
		dep, ok := reg[b.Kind].(blocks.BinaryDependent)
		if !ok {
			continue
		}
		binary := dep.RequiredBinary()
		if checked[binary] {
			continue
		}
		checked[binary] = true
		if _, err := lookPathFn(binary); err != nil {
			return binary
		}
	}
	return ""
}

// stepBlocks filters the scenario's fenced blocks down to executable step
// blocks: those whose kind has a registered executor. Other fenced blocks
// (yaml, json, prose samples) are documentation and are ignored.
func stepBlocks(reg blocks.Registry, all []scenario.Block) []scenario.Block {
	var steps []scenario.Block
	for _, b := range all {
		if _, ok := reg[b.Kind]; ok {
			steps = append(steps, b)
		}
	}
	return steps
}

// runStep dispatches one step to its executor, converting an executor panic
// into a step failure (the run never aborts on a broken executor).
func runStep(b blocks.Block, ctx blocks.StepCtx) (res blocks.StepResult) {
	defer func() {
		if r := recover(); r != nil {
			res = blocks.StepResult{
				Status: blocks.StatusFail,
				Detail: fmt.Sprintf("block executor panicked: %v", r),
			}
		}
	}()
	return b.Run(ctx)
}
