package cli

import (
	"os"
	"path/filepath"

	"github.com/specscore/specscore-cli/pkg/decision"
	"github.com/specscore/specscore-cli/pkg/entity"
	"github.com/specscore/specscore-cli/pkg/feature"
	"github.com/specscore/specscore-cli/pkg/idea"
	"github.com/specscore/specscore-cli/pkg/ideapromote"
	"github.com/specscore/specscore-cli/pkg/idearelocate"
	"github.com/specscore/specscore-cli/pkg/issue"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/specscore/specscore-cli/pkg/lint"
	"github.com/specscore/specscore-cli/pkg/plan"
	"github.com/specscore/specscore-cli/pkg/rule"
)

// Test seams — package-level vars wrapping external functions.
// Production code calls these vars; tests replace them via t.Cleanup.
var (
	decisionScaffoldFn     = decision.Scaffold
	decisionNextNumberFn   = decision.NextNumber
	ideaScaffoldFn         = idea.Scaffold
	planScaffoldFn         = plan.Scaffold
	planSyncIndexFn        = plan.SyncIndex
	planPreviewReconcileFn = plan.PreviewReconcile
	planReconcileFn        = plan.Reconcile
	issueScaffoldFn        = issue.Scaffold
	issueParseFn           = issue.Parse
	featureFindRefsFn      = feature.FindFeatureRefs
	filepathRelFn          = filepath.Rel

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

	// rule verb seams. Every `rule` verb validates its inputs before touching
	// the tree, so the I/O and re-parse failures that follow a successful
	// preflight cannot be reached from filesystem state alone. These wrap the
	// pkg/rule surface (and the two os calls the verbs make directly) so those
	// defensive branches stay testable — the same reason the ideapromote seams
	// above exist.
	ruleScaffoldDetailFn      = rule.ScaffoldDetail
	ruleParseDetailFn         = rule.ParseDetail
	ruleWriteFileAtomicFn     = rule.WriteFileAtomic
	ruleUpsertIndexRowFn      = rule.UpsertRow
	ruleRemoveIndexRowFn      = rule.RemoveRow
	ruleEnsureIndexFn         = rule.EnsureIndex
	ruleApplyFieldEditsFn     = rule.ApplyFieldEdits
	ruleSetLessonPromotesToFn = rule.SetLessonPromotesTo
	ruleReadIndexFn           = rule.ReadIndex
	lessonResolveLessonFileFn = lesson.ResolveLessonFile
	lessonDiscoverFn          = lesson.Discover
	featureDiscoverFn         = feature.Discover
	lintRunFn                 = lint.Lint

	osMkdirAllCLI  = os.MkdirAll
	osRemoveAllCLI = os.RemoveAll
	osReadFileCLI  = os.ReadFile
	osReadDirCLI   = os.ReadDir
	osStatCLI      = os.Stat
	osGetenvCLI    = os.Getenv
)
