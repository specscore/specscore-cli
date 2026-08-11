package cli

import (
	"path/filepath"

	"github.com/specscore/specscore-cli/pkg/decision"
	"github.com/specscore/specscore-cli/pkg/entity"
	"github.com/specscore/specscore-cli/pkg/feature"
	"github.com/specscore/specscore-cli/pkg/idea"
	"github.com/specscore/specscore-cli/pkg/ideapromote"
	"github.com/specscore/specscore-cli/pkg/idearelocate"
	"github.com/specscore/specscore-cli/pkg/issue"
	"github.com/specscore/specscore-cli/pkg/plan"
)

// Test seams — package-level vars wrapping external functions.
// Production code calls these vars; tests replace them via t.Cleanup.
var (
	decisionScaffoldFn   = decision.Scaffold
	decisionNextNumberFn = decision.NextNumber
	ideaScaffoldFn       = idea.Scaffold
	planScaffoldFn       = plan.Scaffold
	issueScaffoldFn      = issue.Scaffold
	issueParseFn         = issue.Parse
	featureFindRefsFn    = feature.FindFeatureRefs
	filepathRelFn        = filepath.Rel

	idearelocateDiscoverSiblingsFn   = idearelocate.DiscoverSiblings
	idearelocateExecuteCommitPhaseFn = idearelocate.ExecuteCommitPhase
	idearelocatePreflightSubjectsFn  = idearelocate.PreflightSubjectsForRelocate
	idearelocateCheckPreflightFn     = idearelocate.CheckPreflight

	// idea promote seams — the verb's pure/mutating helpers wrapped so the
	// CLI's defensive error-return branches (which a real fixture cannot
	// reach once the pre-mutation guards have passed) are testable.
	ideapromoteDiscoverBackLinksFn = ideapromote.DiscoverBackLinks
	ideapromoteTransformFn         = ideapromote.Transform
	ideapromoteSameRepoPromoteFn   = ideapromote.SameRepoPromote
	ideapromoteCrossRepoPromoteFn  = ideapromote.CrossRepoPromote
	ideapromoteReconcileFn         = ideapromote.ReconcileSameRepoBackLinks

	// filepathAbsCLI wraps filepath.Abs for the entity/property verbs'
	// defensive fallbacks. Tests inject failures via cleanup-restored swap.
	filepathAbsCLI = filepath.Abs

	// entityDiscoverCLI wraps entity.Discover for the property-refs verb's
	// defensive error-return path (entity.Discover fails after property
	// discovery succeeds — unreachable through filesystem state alone).
	entityDiscoverCLI = entity.Discover

	// entityResolveInheritsCLI wraps entity.ResolveInherits for the
	// runEntityRefs verb's resolveErr branch (URL paths short-circuit
	// before this; only seam injection triggers the error).
	entityResolveInheritsCLI = entity.ResolveInherits
)
