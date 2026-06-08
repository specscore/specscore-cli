package cli

// Features implemented: cli/spec

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lint"
	"github.com/specscore/specscore-cli/pkg/projectdef"
	"github.com/spf13/cobra"
)

// specCommand returns the "spec" command group.
func specCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "spec",
		Short: "Validate and search specification repositories",
	}
	cmd.AddCommand(
		specLintCommand(),
		specMigrateCommand(),
	)
	return cmd
}

// specMigrateCommand performs the one-shot artifact-frontmatter-convention
// backfill across the spec tree (cli/spec/migrate).
func specMigrateCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate",
		Short: "Backfill artifact-frontmatter-convention frontmatter across the spec tree",
		Long: `One-shot, deterministic, offline migration: writes the format:/status:
frontmatter into every artifact that lacks it (status: only for status-bearing
types) and aligns each adherence-footer URL to format:. Idempotent — re-running
on an already-migrated tree changes nothing and exits 0.`,
		Args: cobra.NoArgs,
		RunE: runSpecMigrate,
	}
	cmd.Flags().StringP("project", "p", "", "path to spec repository root (default: auto-discover from CWD)")
	return cmd
}

func runSpecMigrate(cmd *cobra.Command, _ []string) error {
	projectFlag, _ := cmd.Flags().GetString("project")

	specRoot, err := resolveConfigSpecRoot(projectFlag)
	if err != nil {
		return err
	}

	changed, err := lintMigrateFn(specRoot)
	if err != nil {
		return exitcode.UnexpectedErrorf("migrating frontmatter: %v", err)
	}

	w := cmd.OutOrStdout()
	if len(changed) == 0 {
		_, _ = fmt.Fprintln(w, "Already migrated: no files changed.")
		return nil
	}
	_, _ = fmt.Fprintf(w, "Migrated %d file(s):\n", len(changed))
	for _, f := range changed {
		_, _ = fmt.Fprintf(w, "  %s\n", f)
	}
	return nil
}

func specLintCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lint",
		Short: "Validate spec tree for structural convention violations",
		Long: `Scans the specification tree and reports violations of structural
conventions (README.md files, Open Questions sections, heading
levels, feature references, internal links, index entries).

Violations are categorized by severity: error (must fix), warning (should
fix), info (advisory). Exit code 0 = valid, 1 = violations found, 2 =
invalid arguments, 10+ = unexpected error.`,
		Args: cobra.NoArgs,
		RunE: runSpecLint,
	}
	cmd.Flags().StringP("project", "p", "", "path to spec repository root (default: auto-discover from CWD)")
	cmd.Flags().String("rules", "", "enable only specified rules (comma-separated)")
	cmd.Flags().String("ignore", "", "disable specified rules (comma-separated)")
	cmd.Flags().String("severity", "error", "minimum severity: error, warning, info")
	cmd.Flags().String("format", "text", "output format: text, json, yaml")
	cmd.Flags().String("fix", "", "apply autofixes: bare `--fix` runs the standard fixers (e.g. adherence-footer, studio-toolbar, idea sync/index/archived-order); `--fix=<targets>` (comma-separated) also enables opt-in fixes — currently `no-source` (rewrite a plan with no source line to `**Source:** none`)")
	// NoOptDefVal lets bare `--fix` (no `=value`) mean "standard fixers only".
	cmd.Flags().Lookup("fix").NoOptDefVal = fixStandardOnly
	return cmd
}

// fixStandardOnly is the value bare `--fix` resolves to (via NoOptDefVal): run
// the standard fix pass with no opt-in fixes enabled.
const fixStandardOnly = lint.FixTargetAll

// parseFixTargets splits the raw `--fix` value into target tokens, trimming
// whitespace and dropping empties. The bare sentinel (`all`) is preserved so the
// lint layer can treat it as the unscoped pass. Returns nil when no fix was
// requested.
func parseFixTargets(raw string) []string {
	if raw == "" {
		return nil
	}
	var targets []string
	for _, tok := range strings.Split(raw, ",") {
		if tok = strings.TrimSpace(tok); tok != "" {
			targets = append(targets, tok)
		}
	}
	return targets
}

// resolveConfigSpecRoot resolves the `spec/` directory of the project that
// anchors --project (or the CWD). Unlike feature/idea/task commands, the spec
// verbs REQUIRE an anchoring specscore.yaml (cli/spec/lint#req:specscore-yaml-required)
// — no fallback to a bare spec/features/ directory.
func resolveConfigSpecRoot(projectFlag string) (string, error) {
	var startDir string
	if projectFlag != "" {
		abs, err := filepathAbsFn(projectFlag)
		if err != nil {
			return "", exitcode.InvalidArgsErrorf("resolving --project path: %v", err)
		}
		startDir = abs
	} else {
		cwd, err := osGetwdFn()
		if err != nil {
			return "", exitcode.UnexpectedErrorf("cannot determine working directory: %v", err)
		}
		startDir = cwd
	}
	root, err := findRepoConfigRoot(startDir)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "spec"), nil
}

func runSpecLint(cmd *cobra.Command, args []string) error {
	projectFlag, _ := cmd.Flags().GetString("project")

	specRoot, err := resolveConfigSpecRoot(projectFlag)
	if err != nil {
		return err
	}

	rulesStr, _ := cmd.Flags().GetString("rules")
	ignoreStr, _ := cmd.Flags().GetString("ignore")
	severity, _ := cmd.Flags().GetString("severity")
	format, _ := cmd.Flags().GetString("format")

	// Validate mutual exclusion.
	if rulesStr != "" && ignoreStr != "" {
		return exitcode.InvalidArgsError("--rules and --ignore are mutually exclusive")
	}

	// Parse rules.
	var rules []string
	if rulesStr != "" {
		for _, r := range strings.Split(rulesStr, ",") {
			r = strings.TrimSpace(r)
			if r != "" {
				rules = append(rules, r)
			}
		}
	}

	// Parse ignore.
	var ignore []string
	if ignoreStr != "" {
		for _, r := range strings.Split(ignoreStr, ",") {
			r = strings.TrimSpace(r)
			if r != "" {
				ignore = append(ignore, r)
			}
		}
	}

	// Validate severity.
	if severity != "error" && severity != "warning" && severity != "info" {
		return exitcode.InvalidArgsErrorf("invalid severity level: %s", severity)
	}

	// Validate format.
	if format != "text" && format != "json" && format != "yaml" {
		return exitcode.InvalidArgsErrorf("invalid format: %s", format)
	}

	// Validate rule names.
	if err := lint.ValidateRuleNames(rules); err != nil {
		return exitcode.InvalidArgsError(err.Error())
	}
	if err := lint.ValidateRuleNames(ignore); err != nil {
		return exitcode.InvalidArgsError(err.Error())
	}

	fixRaw, _ := cmd.Flags().GetString("fix")
	fix := fixRaw != ""
	fixTargets := parseFixTargets(fixRaw)
	if err := lint.ValidateFixTargets(fixTargets); err != nil {
		return exitcode.InvalidArgsError(err.Error())
	}

	opts := lint.Options{
		SpecRoot:   specRoot,
		Rules:      rules,
		Ignore:     ignore,
		Severity:   severity,
		Fix:        fix,
		FixTargets: fixTargets,
		CLIVersion: version,
	}

	res, err := lint.LintWithResult(opts)
	if err != nil {
		return exitcode.UnexpectedErrorf("linting error: %v", err)
	}
	violations := res.Violations

	// lint.Result.Fixed is relative to the spec tree; render it relative to
	// the project root (where specscore.yaml lives) so consumers can `git add`
	// the reported paths directly. cli/spec/lint#req:fix-reports-modified-files.
	fixedRel := res.Fixed
	if prefix, perr := filepath.Rel(filepath.Dir(specRoot), specRoot); perr == nil && prefix != "." {
		fixedRel = make([]string, len(res.Fixed))
		for i, f := range res.Fixed {
			fixedRel[i] = filepath.ToSlash(filepath.Join(prefix, f))
		}
	}

	w := cmd.OutOrStdout()
	// Under --fix with a structured format, emit a single envelope carrying
	// both the fixed-files report and the violations. Without --fix the
	// pre-existing bare-array contract is preserved.
	if fix && (format == "json" || format == "yaml") {
		if err := outputLintFixEnvelope(w, fixedRel, violations, format); err != nil {
			return exitcode.UnexpectedErrorf("output error: %v", err)
		}
	} else {
		// In default text format under --fix, write the "Fixed N file(s):"
		// summary naming each modified path to stderr (only when files
		// changed), leaving stdout as the remaining-violations report.
		if fix && format == "text" && len(fixedRel) > 0 {
			ew := cmd.ErrOrStderr()
			_, _ = fmt.Fprintf(ew, "Fixed %d file(s):\n", len(fixedRel))
			for _, f := range fixedRel {
				_, _ = fmt.Fprintf(ew, "  %s\n", f)
			}
		}
		if err := outputLintViolations(w, violations, format); err != nil {
			return exitcode.UnexpectedErrorf("output error: %v", err)
		}
		// Advertise opt-in fixes for any remaining fixable violations, once per
		// distinct fix target. Text format only; structured output already
		// carries each violation's fix_target field.
		if format == "text" {
			writeHowToFix(cmd.ErrOrStderr(), violations)
		}
	}

	if len(violations) > 0 {
		return exitcode.ConflictErrorf("%d violation(s) found", len(violations))
	}
	return nil
}

// writeHowToFix prints a "How to fix" section to w listing, once per distinct
// fix target, how many remaining violations it would repair and the command to
// run. Nothing is written when no violation carries a FixTarget.
func writeHowToFix(w io.Writer, violations []lint.Violation) {
	counts := map[string]int{}
	var order []string
	for _, v := range violations {
		if v.FixTarget == "" {
			continue
		}
		if counts[v.FixTarget] == 0 {
			order = append(order, v.FixTarget)
		}
		counts[v.FixTarget]++
	}
	if len(order) == 0 {
		return
	}
	_, _ = fmt.Fprintln(w, "\nHow to fix:")
	for _, target := range order {
		_, _ = fmt.Fprintf(w, "  %d fixable finding(s) — run: specscore spec lint --fix=%s\n",
			counts[target], target)
	}
}

// lintFixEnvelope is the stdout shape under `--fix --format json|yaml`: a
// single object carrying both the fixed-files report and the violations.
type lintFixEnvelope struct {
	Fixed      []string         `json:"fixed" yaml:"fixed"`
	Violations []lint.Violation `json:"violations" yaml:"violations"`
}

// outputLintFixEnvelope marshals the {fixed, violations} envelope. Both arrays
// serialize as `[]` (never `null`) when empty, matching the non-fix contract.
func outputLintFixEnvelope(w io.Writer, fixed []string, violations []lint.Violation, format string) error {
	if fixed == nil {
		fixed = []string{}
	}
	if violations == nil {
		violations = []lint.Violation{}
	}
	env := lintFixEnvelope{Fixed: fixed, Violations: violations}
	switch format {
	case "yaml":
		enc := newYAMLEnc(w)
		if err := enc.Encode(env); err != nil {
			return err
		}
		return enc.Close()
	default:
		return newJSONEnc(w).Encode(env)
	}
}

func outputLintViolations(w io.Writer, violations []lint.Violation, format string) error {
	switch format {
	case "json":
		return outputLintJSON(w, violations)
	case "yaml":
		return outputLintYAML(w, violations)
	default:
		return outputLintText(w, violations)
	}
}

func outputLintJSON(w io.Writer, violations []lint.Violation) error {
	if violations == nil {
		violations = []lint.Violation{}
	}
	return newJSONEnc(w).Encode(violations)
}

func outputLintYAML(w io.Writer, violations []lint.Violation) error {
	if len(violations) == 0 {
		_, _ = fmt.Fprintln(w, "[]")
		return nil
	}
	enc := newYAMLEnc(w)
	if err := enc.Encode(violations); err != nil {
		return err
	}
	return enc.Close()
}

func outputLintText(w io.Writer, violations []lint.Violation) error {
	// Sort by file then line.
	sort.Slice(violations, func(i, j int) bool {
		if violations[i].File != violations[j].File {
			return violations[i].File < violations[j].File
		}
		return violations[i].Line < violations[j].Line
	})

	for _, v := range violations {
		_, _ = fmt.Fprintf(w, "%s:%d [%s] %s: %s\n", v.File, v.Line, v.Severity, v.Rule, v.Message)
	}

	if len(violations) > 0 {
		errorCount := 0
		warningCount := 0
		infoCount := 0
		for _, v := range violations {
			switch v.Severity {
			case "error":
				errorCount++
			case "warning":
				warningCount++
			case "info":
				infoCount++
			}
		}

		_, _ = fmt.Fprintf(w, "\n%d violations found", len(violations))
		var parts []string
		if errorCount > 0 {
			parts = append(parts, fmt.Sprintf("%d error%s", errorCount, lintPlural(errorCount)))
		}
		if warningCount > 0 {
			parts = append(parts, fmt.Sprintf("%d warning%s", warningCount, lintPlural(warningCount)))
		}
		if infoCount > 0 {
			parts = append(parts, fmt.Sprintf("%d info", infoCount))
		}
		if len(parts) > 0 {
			_, _ = fmt.Fprintf(w, " (%s)", strings.Join(parts, ", "))
		}
		_, _ = fmt.Fprintln(w)
	} else {
		_, _ = fmt.Fprintln(w, "0 violations found")
	}
	return nil
}

func lintPlural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// findRepoConfigRoot walks up from startDir looking for specscore.yaml
// and returns the directory that contains it. Unlike
// feature.FindSpecRepoRoot, no fallback to a bare spec/features/ is
// allowed — `spec lint` REQUIRES a config file
// (cli/spec/lint#req:specscore-yaml-required). On failure, the returned
// error carries exit code 3 (NotFound) and tells the caller to run
// `specscore init`.
func findRepoConfigRoot(startDir string) (string, error) {
	current, err := filepathAbsFn(startDir)
	if err != nil {
		return "", exitcode.UnexpectedErrorf("resolving path: %v", err)
	}
	for {
		if _, statErr := os.Stat(filepath.Join(current, projectdef.SpecConfigFile)); statErr == nil {
			return current, nil
		}
		parent := filepath.Dir(current)
		if parent == current {
			return "", exitcode.NotFoundErrorf(
				"%s is required but was not found in %s or any parent directory; run `specscore init` to create it",
				projectdef.SpecConfigFile, startDir,
			)
		}
		current = parent
	}
}
