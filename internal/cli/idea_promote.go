package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/idea"
	"github.com/specscore/specscore-cli/pkg/ideapromote"
	"github.com/specscore/specscore-cli/pkg/lint"
	"github.com/spf13/cobra"
)

// ideaPromoteCommand returns the "idea promote" subcommand.
// See spec/features/cli/idea/promote/README.md.
//
// Task 1 of the implementation plan ships the command scaffold + the
// three pre-mutation guards (seed resolution, destination collision,
// clean-tree pre-flight) plus --verdict enum validation. The move/
// transform, back-link reconcile, cross-repo archive, verdict carry-
// forward, and stdout summary land in later tasks.
func ideaPromoteCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "promote <slug>",
		Short: "Promote a sidekick seed into a lint-clean Idea",
		Long: `Turns spec/ideas/seeds/<slug>.md into a lint-clean Idea at
spec/ideas/<slug>.md. A deterministic, non-interactive relocate-and-
transform in the family of ` + "`idea new`" + ` and ` + "`idea relocate`" + `: it
folds the seed body into the Idea skeleton, it does not author prose.

When every back-link to the seed is same-repo (or the seed has none), the
verb git-moves the seed to the Idea path, transforms it, and reconciles
same-repo back-links. When any back-link is cross-repo (a <repo-slug>:
qualified target), the verb instead copies+transforms the seed into a new
Idea and archives the seed to spec/ideas/archived/<slug>.md with
status: promoted.

Examples:

  specscore idea promote foo
  specscore idea promote foo --force
  specscore idea promote foo --verdict=full
`,
		Args: cobra.ExactArgs(1),
		RunE: runIdeaPromote,
	}
	cmd.Flags().Bool("force", false, "overwrite an existing spec/ideas/<slug>.md (otherwise the collision exits 1)")
	cmd.Flags().String("verdict", "", "verdict carry-forward mode: pointer (default), full, or drop. "+
		"Overrides the promote.verdict_carry_forward project default.")
	cmd.Flags().String("format", "", "output format: text (default), json, or yaml")
	cmd.Flags().String("project", "", "source project root (autodetected from cwd if omitted)")
	return cmd
}

func runIdeaPromote(cmd *cobra.Command, args []string) error {
	slug := args[0]
	if err := idea.ValidateSlug(slug); err != nil {
		return exitcode.InvalidArgsErrorf("invalid slug %q: %v", slug, err)
	}

	verdictRaw, _ := cmd.Flags().GetString("verdict")
	verdictFlag, err := ideapromote.ValidateVerdict(verdictRaw)
	if err != nil {
		return err
	}

	force, _ := cmd.Flags().GetBool("force")

	format, _ := cmd.Flags().GetString("format")
	if format != "" && format != "text" && format != "yaml" && format != "json" {
		return exitcode.InvalidArgsErrorf("invalid --format: %s (valid: text, yaml, json)", format)
	}

	projectFlag, _ := cmd.Flags().GetString("project")
	specRoot, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}

	// Guard 1: seed must exist (exit 3, no mutation).
	seedPath, err := ideapromote.ResolveSeed(specRoot, slug)
	if err != nil {
		return err
	}

	// Guard 2: destination collision (exit 1 unless --force).
	if err := ideapromote.CheckCollision(specRoot, slug, force); err != nil {
		return err
	}

	// Discover and classify back-links to the seed across the source repo
	// and sibling SpecScore repos (the same discovery `idea relocate`
	// performs). The presence of any cross-repo (<repo-slug>:) reference
	// selects the cross-repo path; otherwise the same-repo path applies.
	scanRepos, err := promoteScanRepos(specRoot)
	if err != nil {
		return err
	}
	backLinks, err := ideapromote.DiscoverBackLinks(scanRepos, slug)
	if err != nil {
		return err
	}

	// Guard 3: clean-tree pre-flight over the paths the verb would touch
	// — the seed, the destination Idea path, and every same-repo file
	// carrying a reconcilable back-link.
	paths := ideapromote.PathsFor(slug)
	preflightPaths := []string{paths.SeedRel, paths.IdeaRel, paths.ArchivedSeedRel}
	for _, bl := range backLinks {
		if !bl.CrossRepo && bl.RepoRoot == specRoot {
			preflightPaths = append(preflightPaths, bl.RelPath)
		}
	}
	if err := ideapromote.Preflight(specRoot, preflightPaths); err != nil {
		return err
	}

	// Resolve the effective verdict carry-forward mode: flag wins,
	// else specscore.yaml promote.verdict_carry_forward, else default.
	verdictMode := ideapromote.ResolveVerdictMode(specRoot, verdictFlag)

	// Read + parse the seed, then build the transformed Idea body.
	seedBytes, err := os.ReadFile(seedPath)
	if err != nil {
		return exitcode.UnexpectedErrorf("reading seed %s: %v", seedPath, err)
	}
	seed := ideapromote.ParseSeed(string(seedBytes))
	transformed, err := ideapromote.Transform(seed, ideapromote.TransformOptions{
		Slug:              slug,
		Owner:             seed.Frontmatter["captured_by"],
		VerdictMode:       verdictMode,
		SeedRefForPointer: paths.SeedRel,
	})
	if err != nil {
		return exitcode.UnexpectedErrorf("transforming seed: %v", err)
	}

	var ideaAbs, seedAbs string
	var reconciled []ideapromote.ReconcileResult
	crossRepo := ideapromote.HasCrossRepo(backLinks)
	seedFate := ideapromote.FateMoved

	if crossRepo {
		// Cross-repo path: create the Idea by copy+transform, git-mv the
		// seed to spec/ideas/archived/<slug>.md with frontmatter
		// status: promoted and promoted_to: <slug>. Sibling repos are
		// left untouched (delegated to lint/UI cross-repo resolution).
		ideaAbs, seedAbs, err = ideapromote.CrossRepoPromote(specRoot, slug, transformed, string(seedBytes))
		if err != nil {
			return err
		}
		seedFate = ideapromote.FateArchived
	} else {
		// Same-repo path: git-mv the seed to the Idea path, overwrite
		// with the transformed body, and reconcile same-repo back-links.
		ideaAbs, err = ideapromote.SameRepoPromote(specRoot, slug, transformed)
		if err != nil {
			return err
		}
		seedAbs = ideaAbs // the seed became the Idea
		reconciled, err = ideapromote.ReconcileSameRepoBackLinks(backLinks, slug)
		if err != nil {
			return err
		}
	}

	// Run lint-fix so the created Idea is lint-clean by construction.
	if err := promoteLintFix(specRoot, ideaAbs, crossRepo); err != nil {
		return err
	}

	backLinkPaths := make([]string, 0, len(reconciled))
	for _, r := range reconciled {
		backLinkPaths = append(backLinkPaths, r.RelPath)
	}
	result := ideapromote.Result{
		Slug:                slug,
		IdeaPath:            ideaAbs,
		SeedFate:            seedFate,
		SeedPath:            seedAbs,
		ReconciledBackLinks: backLinkPaths,
	}

	w := cmd.OutOrStdout()
	switch format {
	case "yaml":
		enc := newYAMLEnc(w)
		if err := enc.Encode(result); err != nil {
			return exitcode.UnexpectedErrorf("encoding yaml: %v", err)
		}
		return enc.Close()
	case "json":
		return newJSONEnc(w).Encode(result)
	default:
		_, _ = fmt.Fprint(w, result.FormatText())
		return nil
	}
}

// promoteScanRepos returns the set of repos to scan for back-links: the
// source repo plus every sibling SpecScore repo discovered in the
// workspace, deduplicated by canonical path so the source (which
// DiscoverSiblings also returns) is scanned exactly once.
func promoteScanRepos(specRoot string) ([]ideapromote.RepoRef, error) {
	siblings, err := idearelocateDiscoverSiblingsFn(specRoot)
	if err != nil {
		return nil, exitcode.UnexpectedErrorf("discovering sibling repos: %v", err)
	}
	canon := func(p string) string {
		if abs, err := filepath.Abs(p); err == nil {
			if r, err := filepath.EvalSymlinks(abs); err == nil {
				return filepath.Clean(r)
			}
			return filepath.Clean(abs)
		}
		return filepath.Clean(p)
	}
	seen := map[string]struct{}{}
	repos := []ideapromote.RepoRef{{Path: specRoot}}
	seen[canon(specRoot)] = struct{}{}
	for _, s := range siblings {
		c := canon(s.Path)
		if _, ok := seen[c]; ok {
			continue
		}
		seen[c] = struct{}{}
		repos = append(repos, ideapromote.RepoRef{Path: s.Path})
	}
	return repos, nil
}

// promoteLintFix runs `lint --fix` to sync indexes touched by the
// promotion, then validates the created Idea is lint-clean.
//
// Same-repo: the seed leaves the seeds/ dir entirely, so the whole ideas/
// tree should be clean — any error-severity violation under ideas/ fails.
//
// Cross-repo: the retained seed lands in spec/ideas/archived/<slug>.md
// where it shares the slug with the new active Idea. The idea-lint layer
// keys parsed ideas by slug, so an active+archived slug collision makes a
// whole-tree check unreliable for the new Idea, and the archived seed —
// a sidekick-seed, not an Idea — necessarily fails Idea-schema rules. Per
// cli/idea/promote#req:cross-repo-archive the requirement is that the new
// IDEA is lint-clean, not the archived seed; so we validate the new Idea
// structurally in isolation instead of failing on the archived seed's
// expected violations.
func promoteLintFix(specRoot, ideaAbs string, crossRepo bool) error {
	specSub := filepath.Join(specRoot, "spec")
	if _, err := lintLintFn(lint.Options{SpecRoot: specSub, Fix: true}); err != nil {
		return exitcode.UnexpectedErrorf("running lint --fix: %v", err)
	}

	if crossRepo {
		return validateIdeaSkeleton(ideaAbs)
	}

	violations, err := lintLintFn(lint.Options{SpecRoot: specSub})
	if err != nil {
		return exitcode.UnexpectedErrorf("running lint: %v", err)
	}
	relIdea, _ := filepath.Rel(specSub, ideaAbs)
	var own []lint.Violation
	for _, v := range violations {
		if v.Severity == "error" && (v.File == relIdea || strings.HasPrefix(v.File, "ideas/")) {
			own = append(own, v)
		}
	}
	if len(own) > 0 {
		var sb strings.Builder
		sb.WriteString("promoted Idea failed lint:\n")
		for _, v := range own {
			fmt.Fprintf(&sb, "  %s:%d [%s] %s\n", v.File, v.Line, v.Rule, v.Message)
		}
		return exitcode.UnexpectedError(sb.String())
	}
	return nil
}

// validateIdeaSkeleton confirms the file at ideaAbs is a structurally
// valid Idea skeleton: a `# Idea: <Name>` title and every required header
// field and section present. Used for the cross-repo path where a
// whole-tree lint pass is confounded by the archived-seed slug collision.
func validateIdeaSkeleton(ideaAbs string) error {
	p, err := idea.Parse(ideaAbs)
	if err != nil {
		return exitcode.UnexpectedErrorf("parsing promoted Idea %s: %v", ideaAbs, err)
	}
	var missing []string
	if !p.TitleOK || p.TitlePrefix != "Idea" {
		missing = append(missing, "title `# Idea: <Name>`")
	}
	for _, f := range idea.RequiredHeaderFields {
		if _, ok := p.FieldByName[f]; !ok {
			missing = append(missing, "**"+f+":**")
		}
	}
	for _, s := range idea.RequiredSections {
		if _, ok := p.SectionByTitle[s]; !ok {
			missing = append(missing, "section "+s)
		}
	}
	if len(missing) > 0 {
		return exitcode.UnexpectedErrorf(
			"promoted Idea %s is not a lint-clean skeleton; missing: %s",
			ideaAbs, strings.Join(missing, ", "))
	}
	return nil
}
