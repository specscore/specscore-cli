package cli

// Features implemented: cli/sidekick, cli/sidekick/new

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/idea"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
	"github.com/specscore/specscore-cli/pkg/projectdef"
	"github.com/specscore/specscore-cli/pkg/sidekick"
	"github.com/spf13/cobra"
)

// sidekickCommand returns the "sidekick" command group. With no Run/RunE,
// cobra prints help and exits 0 for `specscore sidekick`
// (cli/sidekick#req:subcommands).
func sidekickCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "sidekick",
		Short: "Sidekick-seed management — scaffold scaled-down Idea seeds",
	}
	cmd.AddCommand(sidekickNewCommand(), sidekickChangeStatusCommand())
	return cmd
}

// sidekickChangeStatusCommand transitions a sidekick-seed from Queued to a
// terminal status (Implemented, Rejected, or Archived), relocating it to
// spec/ideas/archived/<slug>.md and tagging it `type: sidekick-seed`. Mirrors
// `idea change-status`; see spec/features/cli/sidekick/change-status/README.md.
func sidekickChangeStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "change-status <slug> --to=<status>",
		Short: "Transition a sidekick-seed to a terminal status and archive it",
		Long: `Transitions spec/ideas/seeds/<slug>.md from its Queued frontmatter status
to the terminal status named by --to (Implemented, Rejected, or Archived).
On success the seed is moved to spec/ideas/archived/<slug>.md, tagged with
` + "`type: sidekick-seed`" + `, the verb runs ` + "`specscore spec lint --fix`" + ` to keep
the ideas index in sync, prints "<slug>: Queued → <status>" to stdout, and
exits 0.

The Queued → Rejected transition is reason-required: pass --note with the
reasoning. When supplied on any target, the note is written as a
` + "`## Resolution`" + ` section in the relocated seed.

If anything fails after the status rewrite (archive collision, file-move
failure, lint failure, I/O error, or a --note write failure), the on-disk
state is restored to its exact pre-invocation form before the verb exits.

Examples:

  specscore sidekick change-status foo --to=implemented
  specscore sidekick change-status bar --to=rejected --note "Superseded by X"
`,
		Args: cobra.ExactArgs(1),
		RunE: runSidekickChangeStatus,
	}
	cmd.Flags().String("to", "", "target status (required). Legal values: "+
		strings.Join(sidekick.SeedTargetNames(), ", ")+" (case-insensitive).")
	_ = cmd.MarkFlagRequired("to")
	cmd.Flags().String("note", "", "transition reasoning (required for --to=rejected; "+
		"written as a ## Resolution section when supplied)")
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	return cmd
}

func runSidekickChangeStatus(cmd *cobra.Command, args []string) error {
	slug := args[0]

	// 2. Parse --to into a canonical terminal status (exit 2 on unrecognized).
	toRaw, _ := cmd.Flags().GetString("to")
	to, err := sidekick.ParseSeedTarget(toRaw)
	if err != nil {
		return err
	}
	note, _ := cmd.Flags().GetString("note")

	projectFlag, _ := cmd.Flags().GetString("project")
	specRoot, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}

	// 3. Resolve the seed path (exit 3 on NotFound).
	seedPath, err := sidekick.ResolveSeedPath(specRoot, slug)
	if err != nil {
		return err
	}

	// 4. Strict source-state check (exit 4 if the current status is not Queued).
	current, err := sidekick.ReadFrontmatterStatus(seedPath)
	if err != nil {
		return exitcode.UnexpectedErrorf("reading seed frontmatter status: %v", err)
	}
	if err := sidekick.CheckSeedSource(current); err != nil {
		return err
	}

	// 5. Reason-required guard BEFORE any mutation (exit 2 if Rejected w/o note).
	if err := lifecycle.GuardReason(
		sidekick.SeedReasonRequiredSet(), sidekick.SeedQueued, to, note,
	); err != nil {
		var rre *lifecycle.ReasonRequiredError
		if errors.As(err, &rre) {
			return exitcode.InvalidArgsError(err.Error())
		}
		return err
	}

	// 6. Mutate. Append the note first (if any), then relocate. The note is
	// appended to the seeds/ file; Relocate's RollbackHook restores it on any
	// post-rewrite failure. Relocate's collision pre-check returns BEFORE the
	// RollbackHook window, so the note is undone explicitly on a collision.
	var noteOriginal []byte
	noteWritten := false
	if strings.TrimSpace(note) != "" {
		orig, wrote, err := lifecycle.AppendResolutionNote(seedPath, note)
		if err != nil {
			return exitcode.UnexpectedErrorf("writing resolution note: %v", err)
		}
		noteOriginal, noteWritten = orig, wrote
	}

	specSub := filepath.Join(specRoot, "spec")
	relErr := sidekick.Relocate(sidekick.RelocateOptions{
		SpecRoot:  specRoot,
		Slug:      slug,
		SeedPath:  seedPath,
		NewStatus: to,
		// 7. Index sync runs after the move; a lint failure triggers Relocate's
		// full rollback (move back to seeds/, restore bytes, RollbackHook).
		AfterMoveHook: func() error {
			return lintPostMutationHook(specSub)()
		},
		// Undo the resolution note as part of the atomic rollback.
		RollbackHook: func() error {
			if noteWritten {
				return lifecycle.RestoreBody(seedPath, noteOriginal)
			}
			return nil
		},
	})
	if relErr != nil {
		// Relocate's collision pre-check returns without invoking RollbackHook,
		// leaving the seed (with any appended note) untouched at seeds/. Undo the
		// note here so the seed is restored byte-identical per
		// rollback-includes-relocation.
		if noteWritten {
			if _, statErr := os.Stat(seedPath); statErr == nil {
				_ = lifecycle.RestoreBody(seedPath, noteOriginal)
			}
		}
		return relErr
	}

	// 8. Success line.
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s → %s\n",
		slug, string(sidekick.SeedQueued), string(to))
	return nil
}

// sidekickNewCommand scaffolds a lint-clean sidekick-seed at
// spec/ideas/seeds/<slug>.md, deriving the slug from the one-liner
// (cli/sidekick/new).
func sidekickNewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new <one-liner>",
		Short: "Scaffold a lint-clean sidekick-seed from a one-liner",
		Long: `Creates a lint-clean sidekick-seed at spec/ideas/seeds/<slug>.md, carrying
the minimal sidekick-seed frontmatter (captured_by, status) and an H1 whose
heading is the one-liner verbatim.

The slug is DERIVED from the one-liner (a seed is a scaled-down Idea, so
capture is quick and slug-free at the call site). The same algorithm the
specstudio:sidekick skill uses is applied, so the CLI and the skill produce
identical slugs for identical input. Pass --slug to override the derived
slug verbatim — used by callers that own their own slug + collision policy.`,
		Args: cobra.ExactArgs(1),
		RunE: runSidekickNew,
	}
	cmd.Flags().String("slug", "", "override the derived slug (must be lowercase, hyphen-separated, URL-safe)")
	cmd.Flags().String("captured-by", "", `capturer identity (defaults to "user")`)
	cmd.Flags().String("body", "", "additional markdown appended after the H1")
	cmd.Flags().Bool("force", false, "overwrite an existing seed file at that slug")
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	return cmd
}

func runSidekickNew(cmd *cobra.Command, args []string) error {
	oneLiner := strings.TrimSpace(args[0])
	if oneLiner == "" {
		return exitcode.InvalidArgsError("a non-empty one-liner is required")
	}
	if len(oneLiner) > sidekick.MaxOneLinerChars {
		return exitcode.InvalidArgsErrorf(
			"one-liner too long (%d chars); max is %d", len(oneLiner), sidekick.MaxOneLinerChars)
	}

	capturedBy, _ := cmd.Flags().GetString("captured-by")
	body, _ := cmd.Flags().GetString("body")
	force, _ := cmd.Flags().GetBool("force")
	projectFlag, _ := cmd.Flags().GetString("project")
	slugFlag, _ := cmd.Flags().GetString("slug")

	// A --slug override is used verbatim (validated); otherwise the slug is
	// derived from the one-liner (cli/sidekick/new#req:slug-override).
	var slug string
	if slugFlag != "" {
		if err := sidekick.ValidateSlug(slugFlag); err != nil {
			return exitcode.InvalidArgsErrorf("invalid --slug %q: %v", slugFlag, err)
		}
		slug = slugFlag
	} else {
		derived, err := sidekick.DeriveSlug(oneLiner)
		if err != nil {
			return exitcode.InvalidArgsErrorf("cannot derive a slug from %q: %v", oneLiner, err)
		}
		slug = derived
	}

	// Build (and validate) the body before touching the filesystem, so invalid
	// args (over-cap --body) never materialize indexes.
	content, err := sidekick.Scaffold(sidekick.ScaffoldOptions{
		OneLiner:   oneLiner,
		CapturedBy: capturedBy,
		Body:       body,
	})
	if err != nil {
		return exitcode.InvalidArgsErrorf("scaffolding seed: %v", err)
	}

	root, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}

	target := filepath.Join(idea.ResolveSeedsDir(filepath.Join(root, "spec")), slug+".md")
	// Collision check BEFORE any write (cli/sidekick/new#req:no-clobber-default).
	if _, statErr := os.Stat(target); statErr == nil && !force {
		return exitcode.ConflictErrorf("seed already exists: %s (pass --force to overwrite)", target)
	}

	// Materialize ancestor indexes and the seeds directory before the seed file
	// (cli/sidekick/new#req:ancestor-indexes-materialized). Existing files are
	// left untouched.
	if err := ensureSidekickAncestorIndexes(root); err != nil {
		return exitcode.UnexpectedErrorf("materializing ancestor indexes: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return exitcode.UnexpectedErrorf("creating %s: %v", filepath.Dir(target), err)
	}

	if err := os.WriteFile(target, content, 0o644); err != nil {
		return exitcode.UnexpectedErrorf("writing %s: %v", target, err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", target)
	return nil
}

// ensureSidekickAncestorIndexes materializes spec/README.md and
// spec/ideas/README.md when they don't already exist, using the same
// templates as `specscore init`. Existing files are left untouched
// (cli/sidekick/new#req:ancestor-indexes-materialized).
func ensureSidekickAncestorIndexes(root string) error {
	cfg, err := projectdef.ReadSpecConfig(root)
	if err != nil {
		cfg = projectdef.SpecConfig{}
	}
	for _, w := range []struct {
		path    string
		content string
	}{
		{"spec/README.md", specReadmeContent(cfg)},
		{"spec/ideas/README.md", ideasIndexContent(cfg)},
	} {
		if err := writeMissingIndex(root, w.path, w.content); err != nil {
			return fmt.Errorf("writing %s: %w", w.path, err)
		}
	}
	return nil
}
