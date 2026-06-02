package lint

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Violation represents a single linting violation.
type Violation struct {
	File     string `json:"file" yaml:"file"`
	Line     int    `json:"line" yaml:"line"`
	Severity string `json:"severity" yaml:"severity"`
	Rule     string `json:"rule" yaml:"rule"`
	Message  string `json:"message" yaml:"message"`
}

// Options holds linting options.
type Options struct {
	SpecRoot string
	Rules    []string // enabled rules; nil = all
	Ignore   []string // disabled rules
	Severity string   // minimum severity: error, warning, info
	Fix      bool     // when true, auto-fixable violations are repaired on disk by checkers that support it
	// CLIVersion is the running CLI's own version as a semver string (e.g.
	// "0.3.0", with optional leading "v"). Used by the dogfood-version-bump
	// rule to detect stale pins in .github/workflows/*.yml. Empty, "dev",
	// or any non-semver value disables the comparison silently.
	CLIVersion string
}

// Result is the full outcome of a lint pass. Fixed is populated only when
// opts.Fix is true and lists the specRoot-relative paths of files whose bytes
// changed during the fix pass (de-duplicated, sorted). It matches how
// Violation.File paths are rendered (relative to opts.SpecRoot).
type Result struct {
	Violations []Violation
	Fixed      []string
}

// Lint runs all enabled lint rules against the spec tree.
func Lint(opts Options) ([]Violation, error) {
	res, err := LintWithResult(opts)
	if err != nil {
		return nil, err
	}
	return res.Violations, nil
}

// LintWithResult runs all enabled lint rules against the spec tree and returns
// both the violations and, when opts.Fix is true, the set of files the fix pass
// modified.
func LintWithResult(opts Options) (Result, error) {
	// Check spec root exists.
	info, err := os.Stat(opts.SpecRoot)
	if err != nil {
		return Result{}, fmt.Errorf("spec root not found: %s", opts.SpecRoot)
	}
	if !info.IsDir() {
		return Result{}, fmt.Errorf("spec root is not a directory: %s", opts.SpecRoot)
	}

	l := newLinter(opts)

	var fixed []string
	if opts.Fix {
		before, err := snapshotSpecTree(opts.SpecRoot)
		if err != nil {
			return Result{}, fmt.Errorf("fix snapshot error: %w", err)
		}
		if err := l.fix(); err != nil {
			return Result{}, fmt.Errorf("fix error: %w", err)
		}
		after, err := snapshotSpecTree(opts.SpecRoot)
		if err != nil {
			return Result{}, fmt.Errorf("fix snapshot error: %w", err)
		}
		fixed = diffSnapshots(before, after)
	}

	violations, err := l.lint()
	if err != nil {
		return Result{}, fmt.Errorf("linting error: %w", err)
	}

	// Filter by severity if specified.
	if opts.Severity != "" {
		violations = FilterBySeverity(violations, opts.Severity)
	}

	return Result{Violations: violations, Fixed: fixed}, nil
}

// snapshotSpecTree walks every regular file under specRoot and records a
// content hash keyed by the specRoot-relative path.
func snapshotSpecTree(specRoot string) (map[string][32]byte, error) {
	snap := make(map[string][32]byte)
	err := filepath.Walk(specRoot, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		content, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(specRoot, path)
		if err != nil {
			return err
		}
		snap[rel] = sha256.Sum256(content)
		return nil
	})
	if err != nil {
		return nil, err
	}
	return snap, nil
}

// diffSnapshots returns the sorted, de-duplicated set of specRoot-relative
// paths whose content hash changed (or which appeared) between before and after.
// A file with identical bytes in both snapshots is never reported.
func diffSnapshots(before, after map[string][32]byte) []string {
	var changed []string
	for rel, h := range after {
		prev, existed := before[rel]
		if !existed || prev != h {
			changed = append(changed, rel)
		}
	}
	sort.Strings(changed)
	return changed
}

// FilterBySeverity filters violations to those at or above the minimum severity.
func FilterBySeverity(violations []Violation, minSeverity string) []Violation {
	severityOrder := map[string]int{"error": 0, "warning": 1, "info": 2}
	minLevel, ok := severityOrder[minSeverity]
	if !ok {
		return violations
	}

	var filtered []Violation
	for _, v := range violations {
		if severityOrder[v.Severity] <= minLevel {
			filtered = append(filtered, v)
		}
	}
	return filtered
}

// Checker is the public interface for custom rule implementations.
// External tools can implement this to add custom rules to the linter.
type Checker interface {
	Name() string
	Severity() string
	Check(specRoot string) ([]Violation, error)
}

var customCheckers []Checker

// RegisterChecker registers a custom checker that will run alongside
// built-in checkers during Lint(). Must be called before any concurrent
// use of Lint (typically during init or program startup).
func RegisterChecker(c Checker) {
	if c == nil {
		return
	}
	customCheckers = append(customCheckers, c)
	allRuleNames[c.Name()] = true
}

// ResetCustomCheckers clears all registered custom checkers (for testing).
func ResetCustomCheckers() {
	for _, c := range customCheckers {
		delete(allRuleNames, c.Name())
	}
	customCheckers = nil
}

// allRuleNames is the canonical list of known rule names.
var allRuleNames = map[string]bool{
	"readme-exists":        true,
	"oq-section":           true,
	"oq-not-empty":         true,
	"index-entries":        true,
	"heading-levels":       true,
	"feature-ref-syntax":   true,
	"internal-links":       true,
	"forward-refs":         true,
	"code-annotations":     true,
	"plan-hierarchy":       true,
	"plan-roi-metadata":    true,
	"adherence-footer":     true,
	"studio-toolbar":       true,
	"dogfood-version-bump": true,
	// Idea lint rules.
	"idea-location":                     true,
	"idea-slug-format":                  true,
	"idea-single-file":                  true,
	"idea-title-format":                 true,
	"idea-header-fields":                true,
	"idea-id-is-slug":                   true,
	"idea-required-sections":            true,
	"idea-not-doing-non-empty":          true,
	"idea-must-be-true-present":         true,
	"idea-hmw-framing":                  true,
	"idea-status-values":                true,
	"idea-specified-requires-promotion": true,
	"idea-archived-location":            true,
	"idea-archive-reason":               true,
	"idea-supersedes-target-archived":   true,
	"idea-related-ideas-format":         true,
	"idea-related-ideas-target-exists":  true,
	"idea-sync-lint-strict":             true,
	"idea-feature-cross-reference":      true,
	"idea-index-completeness":           true,
	"idea-archived-index-chronological": true,
	// Feature index lint rules.
	"feature-index-row-sync": true,
	// Sidekick-seed lint rule.
	"sidekick-seed": true,
	// Grade body-metadata-field lint rules (canonical-grade-metadata-field
	// Feature). Implemented by gradeChecker.
	"grade-values-shape": true,
	"grade-single-value": true,
	"grade-placement":    true,
	"grade-value":        true,
	// Plan lint rules (single-file Plans at spec/plans/<slug>.md, per the
	// SpecStudio plan-Feature contract). Implemented by planRulesChecker.
	"P-001": true,
	"P-002": true,
	"P-003": true,
	"P-004": true,
	// Decision lint rules (decision Feature). Implemented by decisionRulesChecker.
	"D-title-format":                      true,
	"D-header-fields":                     true,
	"D-status-values":                     true,
	"D-filename-format":                   true,
	"D-number-assignment":                 true,
	"D-single-file":                       true,
	"D-required-sections":                 true,
	"D-declined-alternatives-non-empty":   true,
	"D-observed-consequences-placeholder": true,
	"D-source-idea-optional":              true,
	"D-supersedes-target-exists":          true,
	"D-supersedes-bidirectional":          true,
	"D-archived-location":                 true,
	"D-superseded-requires-successor":     true,
	"D-affected-features-target-exists":   true,
	// Decision immutability rules.
	"D-immutability-once-accepted":        true,
	"D-observed-consequences-append-only": true,
	// Decisions-index lint rules (decisions-index Feature). Implemented by decisionsIndexChecker.
	"DI-list-section-heading":         true,
	"DI-index-columns":                true,
	"DI-status-excludes-archived":     true,
	"DI-numeric-ordering":             true,
	"DI-archived-index-chronological": true,
	"DI-completeness":                 true,
	// Issue lint rules (cli/spec/lint/issue-rules Feature). All 15 IDs are
	// registered up-front so `--rules I-NNN` and the default suite both
	// resolve every ID even while individual rule bodies land
	// incrementally per the implementation Plan. Implemented by
	// issueRulesChecker; only I-009 has logic in this initial scaffold.
	"I-001": true,
	"I-002": true,
	"I-003": true,
	"I-004": true,
	"I-005": true,
	"I-006": true,
	"I-007": true,
	"I-008": true,
	"I-009": true,
	"I-010": true,
	"I-011": true,
	"I-012": true,
	"I-013": true,
	"I-014": true,
	"I-015": true,
	// Property lint rules.
	"property-location":                    true,
	"property-slug-format":                 true,
	"property-single-file":                 true,
	"property-frontmatter-required":        true,
	"property-frontmatter-required-fields": true,
	"property-id-equals-slug":              true,
	"property-data-type-values":            true,
	"property-checks-shape":                true,
	"property-title-format":                true,
	"property-required-sections":           true,
	"property-referenced-by-managed":       true,
	// Entity lint rules.
	"entity-location":                    true,
	"entity-slug-format":                 true,
	"entity-single-file":                 true,
	"entity-frontmatter-required":        true,
	"entity-frontmatter-required-fields": true,
	"entity-id-equals-slug":              true,
	"entity-properties-list-shape":       true,
	"entity-ref-target-exists":           true,
	"entity-inherits-additive-only":      true,
	"entity-inherits-target-exists":      true,
	"entity-inherits-acyclic":            true,
	"entity-title-format":                true,
	"entity-required-sections":           true,
	"entity-properties-table-managed":    true,
	"entity-referenced-by-managed":       true,
}

// AllRuleNames returns the canonical set of known rule names.
func AllRuleNames() map[string]bool {
	// Return a copy to prevent mutation.
	result := make(map[string]bool, len(allRuleNames))
	for k, v := range allRuleNames {
		result[k] = v
	}
	return result
}

// ValidateRuleNames checks that all names are known rules. The legacy
// `view-link` rule was removed by the studio-toolbar Feature; flagging
// it surfaces a migration message naming `studio-toolbar` as the
// replacement (studio-toolbar#req:studio-toolbar-lint-removes-view-link).
func ValidateRuleNames(names []string) error {
	for _, name := range names {
		if !allRuleNames[name] {
			if name == "view-link" {
				return fmt.Errorf("unknown rule %q (removed in this release — use \"studio-toolbar\" instead)", name)
			}
			return fmt.Errorf("unknown rule %q", name)
		}
	}
	return nil
}
