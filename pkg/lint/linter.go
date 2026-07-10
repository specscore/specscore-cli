package lint

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
)

// linter orchestrates rule checking across the spec tree.
type linter struct {
	opts    Options
	ruleSet map[string]checker
}

// checker is the interface for individual rule implementations.
type checker interface {
	check(specRoot string) ([]Violation, error)
	name() string
	severity() string
}

// fixer is an optional interface that checkers may implement to support
// `specscore spec lint --fix`. A fixer mutates the spec tree to resolve
// the violations it would otherwise report. Fixes must be idempotent.
type fixer interface {
	fix(specRoot string) error
}

// fixTargeter is an optional interface a fixer may implement to declare the
// `--fix=<target>` action names it answers to, when those differ from the rule
// names it is registered under (e.g. the plan checker answers to "no-source",
// not "P-001".."P-005"). Without it, a fixer's targets are its rule names.
type fixTargeter interface {
	fixTargets() []string
}

func newLinter(opts Options) *linter {
	l := &linter{
		opts:    opts,
		ruleSet: make(map[string]checker),
	}

	l.registerChecker(newReadmeExistsChecker())
	oqChecker := newOQSectionChecker()
	l.registerChecker(oqChecker)
	l.ruleSet["oq-not-empty"] = oqChecker
	l.registerChecker(newIndexEntriesChecker())
	l.registerChecker(newConfigScopeChecker())
	l.registerChecker(newPlanHierarchyChecker())
	l.registerChecker(newPlanROIChecker())
	l.registerChecker(newAdherenceFooterChecker())
	l.registerChecker(newFormatFieldChecker())
	l.registerChecker(newStatusMirrorChecker())
	l.registerChecker(newFooterFormatMirrorChecker())
	l.registerChecker(newStudioToolbarChecker())
	l.registerChecker(newDogfoodVersionChecker(opts.CLIVersion))
	l.registerChecker(newImplementsReferenceChecker())
	l.registerChecker(newImplementationMatrixChecker())
	l.registerChecker(newOtherPlatformsChecker())

	// Register idea checker under every idea-* rule name.
	ic := newIdeaChecker()
	ic.fix = opts.fixRequested(ideaRuleNames...)
	for _, n := range ideaRuleNames {
		l.ruleSet[n] = ic
	}

	// Register feature-index checker (feature-index-row-sync).
	l.registerChecker(newFeatureIndexChecker())

	// Register sidekick-seed checker.
	l.registerChecker(newSidekickSeedChecker())

	// Register grade checker under all four grade-* rule IDs (one checker,
	// many rule names; per-rule filtering via Violation.Rule in lint()).
	grc := newGradeChecker()
	for _, n := range gradeRuleNames {
		l.ruleSet[n] = grc
	}

	// Register plan-rules checker under all rule IDs (P-001..P-007).
	// The single checker emits violations for all rules; deduping by
	// pointer identity in lint() ensures it runs once per pass.
	pc := newPlanRulesChecker()
	pc.fixNoSource = slices.Contains(opts.FixTargets, FixTargetNoSource)
	pc.fixP007 = opts.fixRequested("P-007")
	pc.fixP006Legacy = opts.fixRequested("P-006")
	pc.fixP004Legacy = opts.fixRequested("P-004")
	for _, n := range []string{"P-001", "P-002", "P-003", "P-004", "P-005", "P-006", "P-007", "P-008"} {
		l.ruleSet[n] = pc
	}

	// Register the feature-rules checker (feature-source-ideas-required).
	frc := newFeatureRulesChecker()
	frc.autofix = opts.fixRequested("feature-source-ideas-required")
	l.ruleSet["feature-source-ideas-required"] = frc

	// Register decision-rules checker under all D-* rule IDs.
	dc := newDecisionRulesChecker()
	dc.fixLegacy = opts.fixRequested("D-status-values")
	for _, n := range decisionRuleIDs {
		l.ruleSet[n] = dc
	}

	// Register decision immutability checker.
	dimm := newDecisionImmutabilityChecker()
	for _, n := range decisionImmutabilityRuleIDs {
		l.ruleSet[n] = dimm
	}

	// Register decisions-index checker under all DI-* rule IDs.
	dic := newDecisionsIndexChecker()
	dic.autofix = opts.fixRequested(decisionsIndexRuleIDs...)
	for _, n := range decisionsIndexRuleIDs {
		l.ruleSet[n] = dic
	}

	// Register issue-rules checker under all 15 rule IDs (I-001..I-015).
	// Same pattern as plan-rules: one checker, many rule IDs; per-rule
	// filtering happens via the Violation.Rule field in lint().
	ic2 := newIssueRulesChecker()
	for _, n := range issueRuleIDs {
		l.ruleSet[n] = ic2
	}

	// Register property checker under every property-* rule name.
	prc := newPropertyChecker()
	prc.autofix = opts.fixRequested(propertyRuleNames...)
	for _, n := range propertyRuleNames {
		l.ruleSet[n] = prc
	}

	// Register entity checker under every entity-* rule name.
	ec := newEntityChecker()
	ec.autofix = opts.fixRequested(entityRuleNames...)
	for _, n := range entityRuleNames {
		l.ruleSet[n] = ec
	}

	// Register custom checkers
	for _, c := range customCheckers {
		l.ruleSet[c.Name()] = &customCheckerAdapter{c}
	}

	return l
}

func (l *linter) registerChecker(c checker) {
	l.ruleSet[c.name()] = c
}

func (l *linter) isRuleEnabled(ruleName string) bool {
	if len(l.opts.Rules) > 0 {
		return slices.Contains(l.opts.Rules, ruleName)
	}

	if len(l.opts.Ignore) > 0 {
		return !slices.Contains(l.opts.Ignore, ruleName)
	}

	return true
}

// fix invokes every enabled checker that implements the fixer interface,
// mutating the spec tree to resolve the violations those checkers report.
// Fix failures are aggregated but do not stop subsequent fixers from running.
func (l *linter) fix() error {
	// checker -> the rule names it is registered under.
	ruleNamesByChecker := map[checker][]string{}
	for ruleName, c := range l.ruleSet {
		ruleNamesByChecker[c] = append(ruleNamesByChecker[c], ruleName)
	}
	unscoped := l.opts.fixUnscoped()

	seen := make(map[checker]bool)
	var firstErr error
	for _, c := range l.ruleSet {
		if seen[c] {
			continue
		}
		seen[c] = true
		f, ok := c.(fixer)
		if !ok {
			continue
		}
		// Respect --rules / --ignore: at least one rule name must be enabled.
		enabled := false
		for _, ruleName := range ruleNamesByChecker[c] {
			if l.isRuleEnabled(ruleName) {
				enabled = true
				break
			}
		}
		if !enabled {
			continue
		}
		// Respect --fix=<targets>: when the pass is scoped, run only fixers an
		// explicit target names. A fixer's targets default to its rule names,
		// overridden via fixTargeter (e.g. the plan checker -> "no-source").
		if !unscoped {
			targets := ruleNamesByChecker[c]
			if ft, ok := c.(fixTargeter); ok {
				targets = ft.fixTargets()
			}
			inScope := slices.ContainsFunc(targets, func(t string) bool {
				return slices.Contains(l.opts.FixTargets, t)
			})
			if !inScope {
				continue
			}
		}
		if err := f.fix(l.opts.SpecRoot); err != nil && firstErr == nil {
			firstErr = fmt.Errorf("fixer %s: %v", c.name(), err)
		}
	}
	return firstErr
}

// lint runs all enabled checkers and returns violations.
// A checker runs if any of the rule names it is registered under are
// enabled.  Individual violations are then filtered so only violations
// whose Rule field matches an enabled rule are returned.
func (l *linter) lint() ([]Violation, error) {
	var violations []Violation

	// Deduplicate checkers (the same checker may be registered under
	// multiple rule names).
	seen := make(map[checker]bool)
	for _, c := range l.ruleSet {
		if seen[c] {
			continue
		}
		seen[c] = true

		// Run checker if any of its registered rule names are enabled.
		enabled := false
		for ruleName, rc := range l.ruleSet {
			if rc == c && l.isRuleEnabled(ruleName) {
				enabled = true
				break
			}
		}
		if !enabled {
			continue
		}

		v, err := c.check(l.opts.SpecRoot)
		if err != nil {
			return nil, fmt.Errorf("checker %s: %v", c.name(), err)
		}

		// Keep only violations whose rule is enabled and which do not target a
		// reserved benchmark-fixture subtree.
		for _, vi := range v {
			if l.isRuleEnabled(vi.Rule) && !isReservedFixturePath(vi.File) {
				violations = append(violations, vi)
			}
		}
	}

	return violations, nil
}

// reservedFeatureSubtree is the name of the per-feature subtree that holds
// benchmark tooling and its committed fixture workspace (questions.jsonl,
// run.sh, fixture/). Like `_tests`, it lives under a feature directory but is
// not itself a spec artifact, so the structural rules do not apply to it and a
// parent index need not list it (cli/studio/answers#req:benchmark-file,
// #req:exit-gate-fixture-and-sneat).
const reservedFeatureSubtree = "benchmark"

// isReservedFixturePath reports whether a spec-root-relative path lives inside a
// per-feature `benchmark/` subtree — the committed benchmark tooling and fixture
// workspace (hand-authored miniature repo checkouts a benchmark runner indexes
// offline). These are not spec artifacts of THIS repo, so the structural rules
// (readme-exists, oq-section, studio-toolbar, status-mirror, …) do not apply.
// The exclusion mirrors the `ideas/seeds` precedent in readme-exists: a
// documented, path-scoped skip for a known non-spec subtree.
func isReservedFixturePath(relPath string) bool {
	for _, seg := range strings.Split(filepath.ToSlash(relPath), "/") {
		if seg == reservedFeatureSubtree {
			return true
		}
	}
	return false
}

// FixTargetNames returns the sorted set of valid `--fix=<target>` action names:
// every fixer-backed rule name (both fixer-interface and check-time fixers) plus
// the opt-in "no-source" action. It excludes the "all" sentinel, which is always
// valid. Used by the CLI to validate `--fix=<targets>` and to render help.
func FixTargetNames() []string {
	set := map[string]bool{FixTargetNoSource: true}

	l := newLinter(Options{})
	ruleNamesByChecker := map[checker][]string{}
	for ruleName, c := range l.ruleSet {
		ruleNamesByChecker[c] = append(ruleNamesByChecker[c], ruleName)
	}
	seen := map[checker]bool{}
	for _, c := range l.ruleSet {
		if seen[c] {
			continue
		}
		seen[c] = true
		if _, ok := c.(fixer); !ok {
			continue
		}
		if ft, ok := c.(fixTargeter); ok {
			for _, n := range ft.fixTargets() {
				set[n] = true
			}
			continue
		}
		for _, n := range ruleNamesByChecker[c] {
			set[n] = true
		}
	}

	// Check-time fixers apply fixes during check() rather than via the fixer
	// interface, so they aren't discoverable by the type assertion above.
	for _, n := range ideaRuleNames {
		set[n] = true
	}
	for _, n := range entityRuleNames {
		set[n] = true
	}

	out := make([]string, 0, len(set))
	for n := range set {
		out = append(out, n)
	}
	sort.Strings(out)
	return out
}

// ValidateFixTargets rejects any target that is neither the "all" sentinel nor a
// known fix-target name. Empty input is valid (no targets).
func ValidateFixTargets(targets []string) error {
	valid := map[string]bool{FixTargetAll: true}
	for _, n := range FixTargetNames() {
		valid[n] = true
	}
	for _, t := range targets {
		if !valid[t] {
			return fmt.Errorf("unknown --fix target %q (known targets: %s)",
				t, strings.Join(FixTargetNames(), ", "))
		}
	}
	return nil
}

// customCheckerAdapter adapts the public Checker interface to the internal checker interface.
type customCheckerAdapter struct {
	c Checker
}

func (a *customCheckerAdapter) name() string     { return a.c.Name() }
func (a *customCheckerAdapter) severity() string { return a.c.Severity() }
func (a *customCheckerAdapter) check(specRoot string) ([]Violation, error) {
	return a.c.Check(specRoot)
}

// walkSpecDirs returns subdirectory paths under specRoot, skipping hidden dirs
// except .github (whose children are traversed but .github itself is skipped).
func walkSpecDirs(specRoot string, fn func(dirPath, relPath string) error) error {
	return filepath.Walk(specRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if !info.IsDir() {
			return nil
		}
		if strings.HasPrefix(info.Name(), ".") {
			if info.Name() == ".github" {
				return nil // traverse children but skip .github itself
			}
			return filepath.SkipDir
		}
		relPath, _ := filepath.Rel(specRoot, path)
		if relPath == "." {
			relPath = filepath.Base(specRoot)
		}
		return fn(path, relPath)
	})
}
