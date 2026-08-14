package lint

import (
	"crypto/sha256"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sort"
)

// Violation represents a single linting violation.
type Violation struct {
	File     string `json:"file" yaml:"file"`
	Line     int    `json:"line" yaml:"line"`
	Severity string `json:"severity" yaml:"severity"`
	Rule     string `json:"rule" yaml:"rule"`
	Message  string `json:"message" yaml:"message"`
	// FixTarget, when non-empty, names the `--fix=<target>` action that repairs
	// this specific violation (e.g. "no-source"). Only set on violations a fixer
	// can actually resolve; drives the "How to fix" hint in text output.
	FixTarget string `json:"fix_target,omitempty" yaml:"fix_target,omitempty"`
}

// Options holds linting options.
type Options struct {
	// SpecRoot is the spec tree to lint. ProjectRoot identifies the project
	// context used by rules that inspect specscore.yaml, .github, or project
	// identity. A staged lifecycle transaction sets both explicitly so lint
	// never reaches back into the live project through filepath derivation.
	SpecRoot    string
	ProjectRoot string
	Rules       []string // enabled rules; nil = all
	Ignore      []string // disabled rules
	Severity    string   // minimum severity: error, warning, info
	Fix         bool     // when true, auto-fixable violations are repaired on disk by checkers that support it
	// FixTargets names opt-in fixes to enable on top of the standard fix pass
	// (only consulted when Fix is true). These are fixes that are deliberately
	// off by default because they mask a likely authoring mistake — e.g.
	// "no-source" rewrites a plan with no source line to `**Source:** none`.
	FixTargets []string
	// CLIVersion is the running CLI's own version as a semver string (e.g.
	// "0.3.0", with optional leading "v"). Used by the dogfood-version-bump
	// rule to detect stale pins in .github/workflows/*.yml. Empty, "dev",
	// or any non-semver value disables the comparison silently.
	CLIVersion string
}

func (o Options) effectiveProjectRoot() string {
	return lintProjectRoot(o.ProjectRoot, o.SpecRoot)
}

// lintProjectRoot centralizes the backwards-compatible fallback for callers
// that still pass only SpecRoot. Rule implementations receive the constructed
// project root instead of deriving it from a live spec-tree path.
func lintProjectRoot(projectRoot, specRoot string) string {
	if projectRoot != "" {
		return projectRoot
	}
	return filepath.Dir(specRoot)
}

// Fix target sentinels. FixTargetAll is the implicit target of a bare `--fix`
// (run every standard fixer). FixTargetNoSource is the opt-in plan fix that
// rewrites a sourceless plan to `**Source:** none` — never enabled by the
// unscoped pass; only when explicitly named.
const (
	FixTargetAll      = "all"
	FixTargetNoSource = "no-source"
)

// fixUnscoped reports whether the fix pass runs every standard fixer — true for
// a bare `--fix` (no explicit targets) or an explicit `--fix=all`.
func (o Options) fixUnscoped() bool {
	return len(o.FixTargets) == 0 || slices.Contains(o.FixTargets, FixTargetAll)
}

// fixRequested reports whether a standard fixer whose action names are `names`
// should run: fixing must be enabled, and the pass is either unscoped or
// explicitly targets one of `names`. Opt-in-only fixes (e.g. no-source) must NOT
// be gated through this — they are never enabled by the unscoped pass; gate them
// on an explicit slices.Contains(FixTargets, FixTargetNoSource).
func (o Options) fixRequested(names ...string) bool {
	if !o.Fix {
		return false
	}
	if o.fixUnscoped() {
		return true
	}
	for _, n := range names {
		if slices.Contains(o.FixTargets, n) {
			return true
		}
	}
	return false
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
		before, err := snapshotSpecTreeFn(opts.SpecRoot)
		if err != nil {
			return Result{}, fmt.Errorf("fix snapshot error: %w", err)
		}
		if err := l.fix(); err != nil {
			return Result{}, fmt.Errorf("fix error: %w", err)
		}
		after, err := snapshotSpecTreeFn(opts.SpecRoot)
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

// snapshotSpecTreeFn is the snapshot implementation, injectable for testing the
// before/after fix-snapshot error paths in LintWithResult.
var snapshotSpecTreeFn = snapshotSpecTree

// filepathRelFn is injectable for testing the filepath.Rel error branch in
// snapshotSpecTree (otherwise unreachable, since Walk always yields paths under
// specRoot).
var filepathRelFn = filepath.Rel

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
		rel, err := filepathRelFn(specRoot, path)
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
	severity := c.Severity()
	if severity == "" {
		severity = "error"
	}
	ruleRegistry[c.Name()] = Rule{
		ID:          c.Name(),
		Description: "Custom checker: " + c.Name(),
		Family:      "custom",
		Severity:    severity,
	}
}

// ResetCustomCheckers clears all registered custom checkers (for testing).
func ResetCustomCheckers() {
	for _, c := range customCheckers {
		delete(ruleRegistry, c.Name())
	}
	customCheckers = nil
}

// AllRuleNames returns the canonical set of known rule names, derived from the
// structured rule registry (the single source of truth).
func AllRuleNames() map[string]bool {
	result := make(map[string]bool, len(ruleRegistry))
	for id := range ruleRegistry {
		result[id] = true
	}
	return result
}

// ValidateRuleNames checks that all names are known rules. The legacy
// `view-link` rule was removed by the studio-toolbar Feature; flagging
// it surfaces a migration message naming `studio-toolbar` as the
// replacement (studio-toolbar#req:studio-toolbar-lint-removes-view-link).
func ValidateRuleNames(names []string) error {
	for _, name := range names {
		if _, ok := ruleRegistry[name]; !ok {
			if name == "view-link" {
				return fmt.Errorf("unknown rule %q (removed in this release — use \"studio-toolbar\" instead)", name)
			}
			return fmt.Errorf("unknown rule %q", name)
		}
	}
	return nil
}
