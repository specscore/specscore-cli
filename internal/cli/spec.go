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
	)
	return cmd
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
	cmd.Flags().Bool("fix", false, "apply fixes from checkers that support autofix (e.g. adherence-footer, studio-toolbar, idea sync/index/archived-order)")
	return cmd
}

func runSpecLint(cmd *cobra.Command, args []string) error {
	projectFlag, _ := cmd.Flags().GetString("project")

	var startDir string
	if projectFlag != "" {
		abs, err := filepathAbsFn(projectFlag)
		if err != nil {
			return exitcode.InvalidArgsErrorf("resolving --project path: %v", err)
		}
		startDir = abs
	} else {
		cwd, err := osGetwdFn()
		if err != nil {
			return exitcode.UnexpectedErrorf("cannot determine working directory: %v", err)
		}
		startDir = cwd
	}

	// cli/spec/lint#req:specscore-yaml-required — lint REQUIRES an
	// anchoring specscore.yaml. Unlike feature/idea/task commands, no
	// fallback to a bare spec/features/ directory.
	root, err := findRepoConfigRoot(startDir)
	if err != nil {
		return err
	}
	specRoot := filepath.Join(root, "spec")

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

	fix, _ := cmd.Flags().GetBool("fix")

	opts := lint.Options{
		SpecRoot:   specRoot,
		Rules:      rules,
		Ignore:     ignore,
		Severity:   severity,
		Fix:        fix,
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
	if prefix, perr := filepath.Rel(root, specRoot); perr == nil && prefix != "." {
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
	}

	if len(violations) > 0 {
		return exitcode.ConflictErrorf("%d violation(s) found", len(violations))
	}
	return nil
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
