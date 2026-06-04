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

	// Same-repo path: git-mv the seed to the Idea path, overwrite with
	// the transformed body. (Cross-repo archive is Task 4.)
	ideaAbs, err := ideapromote.SameRepoPromote(specRoot, slug, transformed)
	if err != nil {
		return err
	}

	// Reconcile same-repo back-links from the old seeds path to the new
	// Idea path. Cross-repo entries are left untouched.
	reconciled, err := ideapromote.ReconcileSameRepoBackLinks(backLinks, slug)
	if err != nil {
		return err
	}

	// Run lint-fix so the created Idea is lint-clean by construction.
	if err := promoteLintFix(specRoot, ideaAbs); err != nil {
		return err
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", ideaAbs)
	for _, r := range reconciled {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "reconciled back-link: %s\n", r.RelPath)
	}
	return nil
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

// promoteLintFix runs `lint --fix` to sync indexes touched by the move,
// then re-runs lint to surface any error-severity violation on the
// created Idea or in the ideas tree. Mirrors `idea new`'s lint discipline.
func promoteLintFix(specRoot, ideaAbs string) error {
	specSub := filepath.Join(specRoot, "spec")
	if _, err := lintLintFn(lint.Options{SpecRoot: specSub, Fix: true}); err != nil {
		return exitcode.UnexpectedErrorf("running lint --fix: %v", err)
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
