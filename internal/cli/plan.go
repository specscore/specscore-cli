package cli

// Features implemented: cli/plan

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/feature"
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
	)
	return cmd
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
	// Collision check BEFORE any write (cli/plan/new#req:no-clobber-default).
	if _, statErr := os.Stat(target); statErr == nil && !force {
		return exitcode.ConflictErrorf("plan already exists: %s (pass --force to overwrite)", target)
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
	if err := os.WriteFile(target, body, 0o644); err != nil {
		return exitcode.UnexpectedErrorf("writing %s: %v", target, err)
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

	return bareOrEmbedded(true, "plan", repl, cmd.ErrOrStderr(), func() ([]byte, error) {
		return planScaffoldFn(plan.ScaffoldOptions{
			Slug:          slug,
			Title:         title,
			Owner:         owner,
			SourceFeature: featureSrc,
			SourceIdea:    ideaSrc,
			Parent:        parent,
		})
	})
}

// ensurePlanAncestorIndexes materializes spec/README.md and spec/plans/README.md
// when they don't already exist, using the same templates as `specscore init`.
// Existing files are left untouched (cli/plan/new#req:ancestor-indexes-materialized).
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
