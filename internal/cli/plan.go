package cli

// Features implemented: cli/plan

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/pkg/dryrun"
	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/feature"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
	"github.com/specscore/specscore-cli/pkg/lint"
	"github.com/specscore/specscore-cli/pkg/plan"
	"github.com/specscore/specscore-cli/pkg/projectdef"
	"github.com/spf13/cobra"
)

// planCommand returns the "plan" command group. With no Run/RunE, cobra
// prints help and exits 0 for `specscore plan`.
func planCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Query plans — listing and inspecting plan metadata",
	}
	cmd.AddCommand(
		planListCommand(),
		planInfoCommand(),
		planNewCommand(),
		planChangeStatusCommand(),
		planReconcileCommand(),
		planTransitionsCommand(),
		planReadinessCommand(),
	)
	return cmd
}

// planTransitionsCommand registers `specscore plan transitions [<slug>]`,
// the read-only counterpart to change-status.
func planTransitionsCommand() *cobra.Command {
	return transitionsCommand(lifecycle.KindPlan, "slug", "Show the Plan status matrix, or one plan's legal next statuses",
		func(projectFlag, slug string) (string, error) {
			specRoot, err := resolveSpecRoot(projectFlag)
			if err != nil {
				return "", err
			}
			return resolvePlanFile(filepath.Join(specRoot, "spec", "plans"), slug)
		})
}

// planChangeStatusCommand transitions a Plan's **Status:** field via the
// shared lifecycle state-machine contract. It owns ONLY the human-authored
// arcs (the prep band plus the dispositions); the execution band
// (Executing/Blocked/Implemented/Failed) is derived by `spec lint --fix` from
// the task-status rollup and is NOT settable here.
// See spec/features/cli/plan/change-status/README.md.
func planChangeStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "change-status <slug> --to=<status>",
		Short: "Transition a Plan's Status (human-authored arcs only)",
		Long: `Transitions spec/plans/<slug>.md (or the directory-form
spec/plans/<slug>/README.md) from its current **Status:** to the value named
by --to. The transition is validated against the Plan legal-transition matrix
below; illegal (from, to) pairs exit 4. On success, the verb runs
` + "`specscore spec lint --fix`" + ` to keep the plans-index in sync, prints
"<slug>: <from> → <to>" to stdout, and exits 0.

This verb owns only the human-authored transitions. The execution-band
statuses (Executing, Blocked, Implemented, Failed) are derived by
` + "`specscore spec lint --fix`" + ` from the task-status rollup and cannot be
set here — passing one as --to exits 2. If the plan was actually implemented
outside the tracked flow (so the task rollup is already there but the record
never caught up), use ` + "`specscore plan reconcile`" + ` instead — it corrects
the record directly and marks the jump as a reconciliation rather than a
normal transition.

Both dispositions require a reason: --to=withdrawn and --to=superseded
require --note. --to=superseded additionally requires --successor naming the
plan that replaces this one; it is written as a **Superseded By:** reference.
The Plan artifact is committed as one atomic durable transaction before lint
and index synchronization begins. If that derived work fails, the command
exits with a committed/recovery-required error and retains the visible Plan;
it never attempts a stale rollback.

When the plan declares **Coordination:** <owner>/<repo>@<branch>, this verb
refuses (exit 1) unless the current git repo/branch matches — mutating a
coordinated plan from anywhere else is a process violation, not a merge
chore. --force-coordination bypasses the check for this invocation only,
printing a warning naming what was bypassed.

` + plan.LegalTransitionMatrix() + `
Examples:

  specscore plan change-status auth --to="In Review"
  specscore plan change-status auth --to=approved
  specscore plan change-status auth --to=withdrawn --note "abandoned after the v2 pivot"
  specscore plan change-status auth --to=superseded --note "replaced" --successor auth-v2
`,
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runPlanChangeStatus,
	}
	cmd.Flags().String("to", "", "target status (required). Legal values: "+
		strings.Join(plan.LegalChangeStatusTargetNames(), ", ")+" (case-insensitive).")
	cmd.Flags().String("note", "", "markdown appended as a ## Resolution section; required for --to=withdrawn and --to=superseded")
	cmd.Flags().String("successor", "", "slug of the plan that supersedes this one; required for --to=superseded, rejected otherwise")
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().Bool(coordinationForceFlagName, false, coordinationForceFlagUsage)
	cmd.Flags().Bool("dry-run", false, "report the transition and every file that would change, writing nothing")
	return cmd
}

func runPlanChangeStatus(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return exitcode.InvalidArgsError("missing required positional argument: <slug>")
	}
	if len(args) > 1 {
		return exitcode.InvalidArgsErrorf(
			"too many positional arguments: change-status accepts exactly one <slug>, got %d", len(args))
	}
	slug := args[0]
	if err := plan.ValidateSlug(slug); err != nil {
		return exitcode.InvalidArgsErrorf("invalid slug %q: %v", slug, err)
	}

	toRaw, _ := cmd.Flags().GetString("to")
	if strings.TrimSpace(toRaw) == "" {
		return exitcode.InvalidArgsError("missing required flag: --to=<status>")
	}
	to, ok := lifecycle.ParseStatus(lifecycle.KindPlan, toRaw)
	if !ok {
		return exitcode.InvalidArgsErrorf(
			"unrecognized --to value %q for plan; legal values: %s",
			toRaw, strings.Join(plan.LegalChangeStatusTargetNames(), ", "))
	}
	// The execution-band statuses are recognized Plan statuses but are
	// lint-derived, not human-settable. Reject them with a pointed message
	// (exit 2) BEFORE the state-machine check. Every other recognized Plan
	// status is a human-settable target (each appears as a To-column in the
	// KindPlan matrix), so no further target filtering is needed here.
	if plan.IsExecutionBandStatus(to) {
		return exitcode.InvalidArgsErrorf(
			"%s is a lint-derived execution-band status; it is set by `specscore spec lint --fix` from task rollup, not via change-status. "+
				"If the work was actually delivered outside the tracked flow, use `specscore plan reconcile` to record it.",
			string(to))
	}

	note, _ := cmd.Flags().GetString("note")
	successor, _ := cmd.Flags().GetString("successor")

	// Reason-required dispositions: Withdrawn and Superseded require --note.
	if to == lifecycle.PlanWithdrawn && strings.TrimSpace(note) == "" {
		return exitcode.InvalidArgsError(
			"transition to Withdrawn requires a reason: pass --note explaining why the plan was abandoned")
	}
	if to == lifecycle.PlanSuperseded && strings.TrimSpace(note) == "" {
		return exitcode.InvalidArgsError(
			"transition to Superseded requires a reason: pass --note describing what superseded the plan")
	}

	// Successor handling: required for Superseded (and must resolve to an
	// existing plan), rejected for every other transition.
	if to == lifecycle.PlanSuperseded {
		if strings.TrimSpace(successor) == "" {
			return exitcode.InvalidArgsError(
				"transition to Superseded requires --successor naming the plan that replaces this one")
		}
		if err := plan.ValidateSlug(strings.TrimSpace(successor)); err != nil {
			return exitcode.InvalidArgsErrorf("invalid successor plan slug: %v", err)
		}
	} else if strings.TrimSpace(successor) != "" {
		return exitcode.InvalidArgsErrorf(
			"--successor is only valid with --to=superseded (got --to=%s)", string(to))
	}

	projectFlag, _ := cmd.Flags().GetString("project")
	specRoot, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}

	forceCoordination, _ := cmd.Flags().GetBool(coordinationForceFlagName)
	successor = strings.TrimSpace(successor)
	var coordinationWarning bytes.Buffer

	if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
		result, changes, err := dryrun.Sandbox(specRoot, func(sandboxRoot string) (plan.ChangeStatusResult, error) {
			return planChangeStatusMutate(sandboxRoot, specRoot, slug, to, note, successor, forceCoordination, &coordinationWarning)
		})
		if err != nil {
			return handlePlanChangeStatusError(err, &coordinationWarning, cmd.ErrOrStderr())
		}
		_, _ = cmd.ErrOrStderr().Write(coordinationWarning.Bytes())
		dryrun.PrintReport(cmd.OutOrStdout(), result.Slug, string(result.From), string(result.To), changes)
		return nil
	}

	result, err := planChangeStatusMutate(specRoot, specRoot, slug, to, note, successor, forceCoordination, &coordinationWarning)
	if err != nil {
		return handlePlanChangeStatusError(err, &coordinationWarning, cmd.ErrOrStderr())
	}
	_, _ = cmd.ErrOrStderr().Write(coordinationWarning.Bytes())

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s → %s\n",
		result.Slug, string(result.From), string(result.To))
	return nil
}

// planChangeStatusMutate is the full Plan status transaction used by both the
// real command and --dry-run. ArtifactRoot is the tree that is transformed;
// coordinationRoot stays on the real project during a preview because the
// sandbox intentionally contains only spec/ and is not a Git checkout.
func planChangeStatusMutate(artifactRoot, coordinationRoot, slug string, to lifecycle.Status, note, successor string, forceCoordination bool, coordinationWarning *bytes.Buffer) (plan.ChangeStatusResult, error) {
	if err := preflightPlanChangeStatus(artifactRoot); err != nil {
		return plan.ChangeStatusResult{}, err
	}
	return plan.ChangeStatus(plan.ChangeStatusOptions{
		SpecRoot:  artifactRoot,
		Slug:      slug,
		To:        to,
		Note:      note,
		Successor: successor,
		ValidateSnapshot: func(path string, before []byte) error {
			parsedPlan, err := plan.ParseBytes(path, before)
			if err != nil {
				return exitcode.UnexpectedErrorCause(fmt.Sprintf("parsing plan %s: %v", slug, err), err)
			}
			if err := enforceCoordinationBranch(parsedPlan, coordinationRoot, forceCoordination, coordinationWarning); err != nil {
				return err
			}
			if to == lifecycle.PlanSuperseded {
				if _, err := resolvePlanFile(filepath.Join(artifactRoot, "spec", "plans"), successor); err != nil {
					return exitcode.InvalidArgsErrorf("successor plan %q does not resolve to an existing plan at spec/plans/%s.md", successor, successor)
				}
			}
			return nil
		},
		PostMutation: plan.PostMutationHook(lintPostMutationHook(filepath.Join(artifactRoot, "spec"))),
	})
}

// preflightPlanChangeStatus validates the complete tree before ChangeStatus
// acquires and commits the Plan artifact. The post-mutation lint pass remains
// necessary for derived index synchronization, but it is too late to protect
// an invalid source/AC reference: by then the Plan status is durable. Keeping
// this pass read-only makes an invalid lifecycle request write-free.
func preflightPlanChangeStatus(projectRoot string) error {
	violations, err := lint.Lint(lint.Options{
		SpecRoot: filepath.Join(projectRoot, "spec"),
		Severity: "error",
	})
	if err != nil {
		return exitcode.UnexpectedErrorf("preflight lint before plan status change: %v", err)
	}
	if len(violations) == 0 {
		return nil
	}
	var details strings.Builder
	for i, v := range violations {
		if i > 0 {
			details.WriteString("; ")
		}
		fmt.Fprintf(&details, "%s:%d [%s] %s", v.File, v.Line, v.Rule, v.Message)
	}
	return exitcode.ConflictErrorf("preflight lint failed; no Plan or index changes were made: %s", details.String())
}

func handlePlanChangeStatusError(err error, coordinationWarning *bytes.Buffer, stderr io.Writer) error {
	var committed *lifecycle.CommittedMutationError
	if errors.As(err, &committed) {
		_, _ = stderr.Write(coordinationWarning.Bytes())
		return exitcode.UnexpectedErrorCause(err.Error(), err)
	}
	return err
}

// resolvePlanFile mirrors plan.resolvePlanFile for the cobra layer's
// --successor existence check: a successor resolves if either the flat file
// spec/plans/<slug>.md or the directory-form spec/plans/<slug>/README.md
// exists.
func resolvePlanFile(plansDir, slug string) (string, error) {
	flat := filepath.Join(plansDir, slug+".md")
	if _, err := os.Stat(flat); err == nil {
		return flat, nil
	}
	dir := filepath.Join(plansDir, slug, "README.md")
	if _, err := os.Stat(dir); err == nil {
		return dir, nil
	}
	return "", exitcode.NotFoundErrorf("plan not found: %s", slug)
}

// planNewCommand scaffolds a lint-clean flat Plan artifact at
// spec/plans/<slug>.md per the cli/plan/new Feature.
func planNewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new <slug>",
		Short: "Scaffold a new Plan artifact",
		Long: `Creates a lint-clean Plan at spec/plans/<slug>.md, carrying the
artifact-frontmatter-convention frontmatter (format:/status:), the
body-metadata header, the required sections with TODO prompts, and the
adherence footer.

A plan decomposes at most one source: pass --feature <feature-slug> (a
Feature-sourced plan), --idea <idea-slug> (an Idea-sourced plan), or
neither (a source-less plan, recorded as **Source:** none). --feature and
--idea are mutually exclusive. A bare scaffold pulls the published template
from the gallery; on any fetch failure the embedded template is used and a
warning is printed to stderr.`,
		Args: cobra.ExactArgs(1),
		RunE: runPlanNew,
	}
	cmd.Flags().String("feature", "", "source Feature slug (mutually exclusive with --idea)")
	cmd.Flags().String("idea", "", "source Idea slug (mutually exclusive with --feature)")
	cmd.Flags().String("parent", "", "parent (master) plan reference — same-repo slug or <repo-slug>:<slug> cross-repo soft ref")
	cmd.Flags().String("title", "", "plan title (defaults to title-cased slug)")
	cmd.Flags().String("owner", "", "owner/author (defaults to $USER)")
	cmd.Flags().Bool("force", false, "overwrite an existing plan file at that slug")
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	return cmd
}

func runPlanNew(cmd *cobra.Command, args []string) error {
	slug := args[0]
	if err := plan.ValidateSlug(slug); err != nil {
		return exitcode.InvalidArgsErrorf("invalid slug %q: %v", slug, err)
	}

	featureSrc, _ := cmd.Flags().GetString("feature")
	ideaSrc, _ := cmd.Flags().GetString("idea")
	// At most one of --feature / --idea (cli/plan/new#req:source-optional).
	// Passing neither produces a source-less plan (`**Source:** none`).
	if featureSrc != "" && ideaSrc != "" {
		return exitcode.InvalidArgsError(
			"--feature <feature-slug> and --idea <idea-slug> are mutually exclusive")
	}

	// --parent is optional; when supplied it MUST be non-empty
	// (cli/plan/new#req:parent-ref-optional). An explicit empty value exits 2.
	if cmd.Flags().Changed("parent") {
		if p, _ := cmd.Flags().GetString("parent"); strings.TrimSpace(p) == "" {
			return exitcode.InvalidArgsError("--parent value must not be empty")
		}
	}

	title, _ := cmd.Flags().GetString("title")
	owner, _ := cmd.Flags().GetString("owner")
	force, _ := cmd.Flags().GetBool("force")
	projectFlag, _ := cmd.Flags().GetString("project")
	if owner == "" {
		owner = os.Getenv("USER")
	}

	root, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}

	target := filepath.Join(root, "spec", "plans", slug+".md")
	// Preserve the write-free fast conflict while the exclusive publication
	// below remains the authoritative no-clobber decision under races.
	if !force {
		if _, statErr := os.Stat(target); statErr == nil {
			return exitcode.ConflictErrorf("plan already exists: %s (pass --force to overwrite)", target)
		} else if !os.IsNotExist(statErr) {
			return exitcode.UnexpectedErrorCause(fmt.Sprintf("checking %s: %v", target, statErr), statErr)
		}
	}
	// Materialize ancestor indexes before the plan file
	// (cli/plan/new#req:ancestor-indexes-materialized). This also creates the
	// spec/plans/ directory (via spec/plans/README.md), so the plan file's
	// parent is guaranteed to exist before the write below.
	if err := ensurePlanAncestorIndexes(root); err != nil {
		return exitcode.UnexpectedErrorf("materializing ancestor indexes: %v", err)
	}

	body, err := buildPlanBody(cmd, slug, title, owner, featureSrc, ideaSrc)
	if err != nil {
		return exitcode.UnexpectedErrorf("scaffolding plan: %v", err)
	}
	if force {
		if _, statErr := os.Stat(target); statErr == nil {
			if err := lifecycle.TransformArtifact(target, func([]byte) ([]byte, error) { return body, nil }); err != nil {
				return exitcode.UnexpectedErrorCause(fmt.Sprintf("replacing %s: %v", target, err), err)
			}
		} else if os.IsNotExist(statErr) {
			if err := publishFileExclusive(target, body, 0o644); err != nil {
				return exitcode.UnexpectedErrorCause(fmt.Sprintf("publishing %s: %v", target, err), err)
			}
		} else {
			return exitcode.UnexpectedErrorCause(fmt.Sprintf("checking %s: %v", target, statErr), statErr)
		}
	} else if err := publishFileExclusive(target, body, 0o644); err != nil {
		if errors.Is(err, os.ErrExist) {
			return exitcode.ConflictErrorf("plan already exists: %s (pass --force to overwrite)", target)
		}
		return exitcode.UnexpectedErrorCause(fmt.Sprintf("publishing %s: %v", target, err), err)
	}
	if _, err := planSyncIndexFn(filepath.Dir(target)); err != nil {
		committed := lifecycle.CommittedError(target, "syncing plans index", err)
		return exitcode.UnexpectedErrorCause(fmt.Sprintf("syncing plans index after committed Plan publication: %v", err), committed)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", target)
	return nil
}

// buildPlanBody resolves the Plan body: a bare scaffold fetches the published
// gallery template (<base>/new/plan.md) and substitutes the known fields; on
// any fetch failure it falls back to the embedded scaffolder (which emits the
// same frontmatter, header, sections, and footer). The source line differs by
// mode: --feature yields `**Source Feature:** <slug>`, --idea yields
// `**Source:** idea:<slug>`, and neither yields `**Source:** none`.
func buildPlanBody(cmd *cobra.Command, slug, title, owner, featureSrc, ideaSrc string) ([]byte, error) {
	repl := map[string]string{
		"<Plan Name>":   templateTitleOrSlug(title, slug),
		"YYYY-MM-DD":    templateTodayUTC(),
		"<your-handle>": templateOwnerOrUnknown(owner),
	}
	switch {
	case featureSrc != "":
		repl["<feature-slug>"] = featureSrc
	case ideaSrc != "":
		// Rewrite the whole default (feature-sourced) line into the idea form.
		repl["**Source Feature:** <feature-slug>"] = "**Source:** idea:" + ideaSrc
	default:
		// Source-less plan: rewrite the default line into the `none` form.
		repl["**Source Feature:** <feature-slug>"] = "**Source:** none"
	}

	// When --parent is supplied, inject a `**Parent:** <value>` line after the
	// Supersedes line of the gallery template (cli/plan/new#req:parent-ref-optional).
	// The embedded scaffolder honors ScaffoldOptions.Parent directly.
	parent, _ := cmd.Flags().GetString("parent")
	parent = strings.TrimSpace(parent)
	if parent != "" {
		repl["**Supersedes:** —"] = "**Supersedes:** —\n**Parent:** " + parent
	}

	body, err := bareOrEmbedded(true, "plan", repl, cmd.ErrOrStderr(), func() ([]byte, error) {
		return planScaffoldFn(plan.ScaffoldOptions{
			Slug:          slug,
			Title:         title,
			Owner:         owner,
			SourceFeature: featureSrc,
			SourceIdea:    ideaSrc,
			Parent:        parent,
		})
	})
	if err != nil {
		return nil, err
	}
	body = normalizePlanTaskStatusVocabulary(body)
	return normalizePlanTaskACPlaceholders(body), nil
}

// normalizePlanTaskStatusVocabulary protects the generated Plan contract when
// a cached or self-hosted gallery still contains the retired `pending` task
// status. The gallery remains the source of the template, while plan new
// guarantees its output uses the linter's canonical `planning` vocabulary.
func normalizePlanTaskStatusVocabulary(body []byte) []byte {
	lines := strings.Split(string(body), "\n")
	for i, line := range lines {
		if line == "**Status:** pending" {
			lines[i] = "**Status:** planning"
		}
	}
	return []byte(strings.Join(lines, "\n"))
}

// normalizePlanTaskACPlaceholders keeps gallery task examples out of the
// Plan's live AC-reference fields. A placeholder such as
// `<feature-slug>#ac:<ac-slug>` is useful authoring guidance, but after the
// source Feature substitution it would otherwise become a syntactically live
// reference and P-002 would reject a brand-new Feature that has no ACs yet.
// Keep the guidance in an HTML comment so the author still sees it while the
// generated Plan remains valid until they add a real `**Verifies:**` field.
func normalizePlanTaskACPlaceholders(body []byte) []byte {
	lines := strings.Split(string(body), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !strings.HasPrefix(trimmed, "**Verifies:**") || !strings.Contains(trimmed, "<ac-slug>") {
			continue
		}
		indent := line[:len(line)-len(strings.TrimLeft(line, " \t"))]
		lines[i] = indent + "<!-- TODO: add " + trimmed + " -->"
	}
	return []byte(strings.Join(lines, "\n"))
}

// ensurePlanAncestorIndexes materializes spec/README.md and spec/plans/README.md
// when they don't already exist, using the same templates as `specscore init`.
// Existing files are left untouched except that the plans index receives the
// derived row for each newly scaffolded Plan.
func ensurePlanAncestorIndexes(root string) error {
	cfg, err := projectdef.ReadSpecConfig(root)
	if err != nil {
		cfg = projectdef.SpecConfig{}
	}
	for _, w := range []struct {
		path    string
		content string
	}{
		{"spec/README.md", specReadmeContent(cfg)},
		{"spec/plans/README.md", plansIndexContent(cfg)},
	} {
		if err := writeMissingIndex(root, w.path, w.content); err != nil {
			return fmt.Errorf("writing %s: %w", w.path, err)
		}
	}
	return nil
}

// resolvePlansDir resolves the plans directory from a --project flag or CWD.
// Unlike resolveFeaturesDir, an absent plans directory is not an error: the
// path is returned regardless (emptiness is handled by the list command).
// Only a missing project root produces an error.
func resolvePlansDir(projectFlag string) (string, error) {
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

	root, err := feature.FindSpecRepoRoot(startDir)
	if err != nil {
		return "", err
	}

	return filepath.Join(root, "spec", "plans"), nil
}

// resolvePlanPath joins plansDir/<slug>.md and verifies the file exists.
// A missing file yields an exit-3 NotFound error naming the slug.
func resolvePlanPath(plansDir, slug string) (string, error) {
	path := filepath.Join(plansDir, slug+".md")
	if _, err := os.Stat(path); err != nil {
		return "", exitcode.NotFoundErrorf("plan not found: %s", slug)
	}
	return path, nil
}
