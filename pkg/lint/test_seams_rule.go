package lint

import (
	"os"

	"github.com/specscore/specscore-cli/pkg/rule"
)

// Test seams for the R-family fixer. The fix pass publishes the index and only
// then re-reads it to repair each detail document's mirror, so the failures
// between those two steps cannot be produced by filesystem state alone once the
// preflight has passed. These wrap the pkg/rule calls so those defensive
// branches stay testable — the same pattern test_seams_decision.go uses.
var (
	ruleWriteIndexRowsFn  = rule.WriteIndexRows
	ruleReadIndexFn       = rule.ReadIndex
	ruleApplyFieldEditsFn = rule.ApplyFieldEdits
	ruleWriteFileAtomicFn = rule.WriteFileAtomic
	ruleReadFileFn        = os.ReadFile
)
