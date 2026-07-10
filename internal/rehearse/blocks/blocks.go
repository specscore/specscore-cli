// Package blocks defines the step-block executor contract shared by every
// rehearse block kind (bash, hurl, sql, dtql, graphql) plus the registry the
// runner dispatches through. Executors live in one sub-package per kind.
package blocks

// MaxStepOutput is the per-step captured-output budget (REQ: bash-block):
// stdout/stderr beyond 8 KiB is truncated with a note.
const MaxStepOutput = 8 * 1024

// Step statuses produced by block executors. The runner adds
// "skipped-after-failure" for steps it never dispatches.
const (
	StatusPass = "pass"
	StatusFail = "fail"
)

// StepCtx carries everything a block executor needs to run one step.
type StepCtx struct {
	// WorkDir is the scenario-scoped temp working directory shared by all
	// steps of one scenario (REQ: scenario-shape).
	WorkDir string
	// Body is the fenced block's verbatim content.
	Body string
	// Params are the info-string key=value parameters (e.g. dsn=..., url=...).
	Params map[string]string
	// Vars is the scenario's context bag as ordered name/value pairs
	// (REQ: context-bag). For bash/sql/dtql the runner has already textually
	// interpolated {{name}} into Body/Params before dispatch; hurl-derived
	// executors (hurl, graphql) instead pass Vars to Hurl as
	// `--variable name=value` flags — no textual interpolation for them.
	Vars []Capture
}

// Capture is one name=value pair captured by a step for the scenario's
// context bag (REQ: context-bag). Order is preserved because the bag is an
// ordered map.
type Capture struct {
	Name  string
	Value string
}

// StepResult is the outcome of running one step.
type StepResult struct {
	// Status is StatusPass or StatusFail.
	Status string
	// Detail is the failure diagnostic (empty on pass).
	Detail string
	// Output is the captured stdout/stderr, already truncated to
	// MaxStepOutput via Truncate.
	Output string
	// Captures are the values the step captured for the scenario's context
	// bag, in capture order (REQ: context-bag). The runner merges them into
	// the bag after the step.
	Captures []Capture
}

// Block is the executor contract implemented once per block kind.
type Block interface {
	// Kind returns the fenced-block info-string kind this executor handles.
	Kind() string
	// Run executes one step. Implementations report failures through the
	// StepResult; the runner recovers panics on their behalf.
	Run(ctx StepCtx) StepResult
}

// BinaryDependent is implemented by executors that delegate to an external
// binary (hurl and the hurl-derived graphql). The runner scans a scenario's
// step kinds upfront, before executing step 1: when a required binary is not
// on PATH the whole scenario is reported skipped — none of its steps run,
// including earlier bash steps — with a warning naming the binary; skips do
// not affect the exit code (REQ: hurl-block, REQ: graphql-block).
type BinaryDependent interface {
	// RequiredBinary returns the name of the external binary the executor
	// needs on PATH.
	RequiredBinary() string
}

// Registry maps a block kind to its executor. Fenced blocks whose kind is
// not registered are not step blocks (documentation blocks are ignored).
type Registry map[string]Block

// NewRegistry builds a Registry from the given executors, keyed by Kind().
func NewRegistry(executors ...Block) Registry {
	reg := Registry{}
	for _, b := range executors {
		reg[b.Kind()] = b
	}
	return reg
}

// Truncate caps captured step output at MaxStepOutput bytes, appending a
// note when anything was cut (REQ: bash-block).
func Truncate(output string) string {
	if len(output) <= MaxStepOutput {
		return output
	}
	return output[:MaxStepOutput] + "\n... [step output truncated at 8 KiB]"
}
