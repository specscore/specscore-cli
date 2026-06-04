package cli

import (
	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/idea"
	"github.com/specscore/specscore-cli/pkg/ideapromote"
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
	if _, err := ideapromote.ValidateVerdict(verdictRaw); err != nil {
		return err
	}

	force, _ := cmd.Flags().GetBool("force")

	projectFlag, _ := cmd.Flags().GetString("project")
	specRoot, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}

	// Guard 1: seed must exist (exit 3, no mutation).
	if _, err := ideapromote.ResolveSeed(specRoot, slug); err != nil {
		return err
	}

	// Guard 2: destination collision (exit 1 unless --force).
	if err := ideapromote.CheckCollision(specRoot, slug, force); err != nil {
		return err
	}

	// Guard 3: clean-tree pre-flight over the paths the verb would touch.
	paths := ideapromote.PathsFor(slug)
	if err := ideapromote.Preflight(specRoot, []string{
		paths.SeedRel, paths.IdeaRel, paths.ArchivedSeedRel,
	}); err != nil {
		return err
	}

	return nil
}
