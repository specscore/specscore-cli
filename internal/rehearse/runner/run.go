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
	rep := ScenarioReport{
		File:     file,
		Verifies: []string{},
		Bag:      map[string]string{},
		Steps:    []StepReport{},
	}
	finish := func(status string) ScenarioReport {
		rep.Status = status
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
		res := runStep(reg[b.Kind], blocks.StepCtx{WorkDir: workDir, Body: b.Body, Params: b.Params})
		rep.Steps = append(rep.Steps, StepReport{
			Kind:   b.Kind,
			Status: res.Status,
			Detail: res.Detail,
			Output: res.Output,
		})
		failed = failed || res.Status == blocks.StatusFail
	}
	if failed {
		return finish(StatusFail)
	}
	return finish(StatusPass)
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
