package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/pkg/decision"
	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
	"github.com/specscore/specscore-cli/pkg/lint"
	"github.com/specscore/specscore-cli/pkg/projectdef"
	"github.com/spf13/cobra"
)

// ensureDecisionAncestorIndexes materializes spec/README.md and
// spec/decisions/README.md when they don't already exist, using the same
// templates as `specscore init`. Existing files are left untouched. Mirrors
// ensurePlanAncestorIndexes for the decisions tree.
func ensureDecisionAncestorIndexes(root string) error {
	cfg, err := projectdef.ReadSpecConfig(root)
	if err != nil {
		cfg = projectdef.SpecConfig{}
	}
	for _, w := range []struct {
		path    string
		content string
	}{
		{"spec/README.md", specReadmeContent(cfg)},
		{"spec/decisions/README.md", decisionsIndexContent(cfg)},
	} {
		if err := writeMissingIndex(root, w.path, w.content); err != nil {
			return fmt.Errorf("writing %s: %w", w.path, err)
		}
	}
	return nil
}

func decisionCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "decision",
		Short: "Decision management — scaffold and transition Decision artifacts",
	}
	cmd.AddCommand(decisionNewCommand(), decisionChangeStatusCommand())
	return cmd
}

// decisionChangeStatusCommand transitions a Decision's **Status:** field via
// the shared lifecycle state-machine contract. Unlike Idea/Plan/Lesson,
// archival is NOT an optional axis for a Decision: pkg/lint's
// D-archived-location rule requires every Rejected/Superseded/Deprecated
// Decision to live under spec/decisions/archived/, so this verb relocates the
// file (and syncs both index surfaces) as part of the same atomic transition
// whenever --to names one of those three dispositions. There is no separate
// `decision archive` verb.
//
// Unlike idea/change-status, plan/change-status, and lesson/change-status,
// there is no spec/features/cli/decision/ Feature spec yet — `decision new`
// itself predates this repo's self-hosting discipline for the Decision kind.
// This verb's behavioral contract is documented here and in
// pkg/decision/transitions.go rather than in a spec/features artifact.
func decisionChangeStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "change-status <slug> --to=<status>",
		Short: "Transition a Decision's Status, archiving it atomically when terminal",
		Long: `Transitions spec/decisions/<slug>.md from its current **Status:** to the
value named by --to. <slug> is the FULL on-disk identifier, i.e. the filename
minus ".md" (e.g. 0009-go-wasm-single-engine) — not the bare slug ` + "`decision new`" + `
takes, since the sequence number is part of the file's real identity.

The transition is validated against the Decision legal-transition matrix
below; illegal (from, to) pairs exit 4.

Rejected, Superseded, and Deprecated are terminal dispositions: on success,
the decision is relocated to spec/decisions/archived/<slug>.md, both the
active and archived decisions-index entries are updated, the verb runs
` + "`specscore spec lint --fix`" + ` to catch anything else, prints
"<slug>: <from> → <to>" to stdout, and exits 0.

Every disposition target requires --note (it becomes the archived-index
entry's reason column AND a ` + "`## Resolution`" + ` section in the archived file).
--to=superseded additionally requires --successor naming the decision that
replaces this one; it is written as a bidirectional link — **Superseded By:**
on this decision and **Supersedes:** on the successor.

If anything fails after the status rewrite (archive collision, index-sync
failure, lint failure, I/O error), the on-disk state — including both index
files and the successor's **Supersedes:** field — is restored to its
pre-invocation form before the verb exits.

` + decision.LegalTransitionMatrix() + `
Examples:

  specscore decision change-status 0001-use-postgres --to="In Review"
  specscore decision change-status 0001-use-postgres --to=approved
  specscore decision change-status 0001-use-postgres --to=deprecated --note "no longer applies"
  specscore decision change-status 0007-bot-tiers-starlark --to=superseded --note "engine unified" --successor 0009-go-wasm-single-engine
`,
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runDecisionChangeStatus,
	}
	cmd.Flags().String("to", "", "target status (required). Legal values: "+
		strings.Join(decision.LegalChangeStatusTargetNames(), ", ")+" (case-insensitive).")
	cmd.Flags().String("note", "", "markdown appended as a ## Resolution section; required for --to=rejected, --to=superseded, and --to=deprecated")
	cmd.Flags().String("successor", "", "full identifier (NNNN-slug) of the decision that supersedes this one; required for --to=superseded, rejected otherwise")
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	return cmd
}

func runDecisionChangeStatus(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return exitcode.InvalidArgsError("missing required positional argument: <slug>")
	}
	if len(args) > 1 {
		return exitcode.InvalidArgsErrorf(
			"too many positional arguments: change-status accepts exactly one <slug>, got %d", len(args))
	}
	slug := args[0]
	if err := decision.ValidateFullSlug(slug); err != nil {
		return exitcode.InvalidArgsErrorf("invalid slug %q: %v", slug, err)
	}

	toRaw, _ := cmd.Flags().GetString("to")
	if strings.TrimSpace(toRaw) == "" {
		return exitcode.InvalidArgsError("missing required flag: --to=<status>")
	}
	to, ok := lifecycle.ParseStatus(lifecycle.KindDecision, toRaw)
	if !ok {
		return exitcode.InvalidArgsErrorf(
			"unrecognized --to value %q for decision; legal values: %s",
			toRaw, strings.Join(decision.LegalChangeStatusTargetNames(), ", "))
	}

	note, _ := cmd.Flags().GetString("note")
	successor, _ := cmd.Flags().GetString("successor")

	// Every disposition requires a reason: it becomes the archived-index
	// entry's reason column, so it can never be empty.
	dispositionNames := map[lifecycle.Status]bool{
		lifecycle.DecisionRejected:   true,
		lifecycle.DecisionSuperseded: true,
		lifecycle.DecisionDeprecated: true,
	}
	if dispositionNames[to] && strings.TrimSpace(note) == "" {
		return exitcode.InvalidArgsErrorf(
			"transition to %s requires a reason: pass --note explaining the disposition", string(to))
	}

	// Successor handling: required for Superseded (and must be a well-formed
	// full identifier), rejected for every other transition.
	if to == lifecycle.DecisionSuperseded {
		if strings.TrimSpace(successor) == "" {
			return exitcode.InvalidArgsError(
				"transition to Superseded requires --successor naming the decision that replaces this one")
		}
		if err := decision.ValidateFullSlug(successor); err != nil {
			return exitcode.InvalidArgsErrorf("invalid --successor %q: %v", successor, err)
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

	result, err := decision.ChangeStatus(decision.ChangeStatusOptions{
		SpecRoot:     specRoot,
		Slug:         slug,
		To:           to,
		Note:         note,
		Successor:    successor,
		PostMutation: decision.PostMutationHook(lintPostMutationHook(filepath.Join(specRoot, "spec"))),
	})
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s → %s\n",
		result.Slug, string(result.From), string(result.To))
	return nil
}

func decisionNewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new <slug>",
		Short: "Scaffold a new Decision artifact",
		Long: `Creates a lint-clean Decision skeleton at spec/decisions/<next-number>-<slug>.md.

The sequence number is auto-assigned (highest existing + 1, including archived/).
Each required section is emitted with an HTML-comment prompt. Supply content
via flags (--title, --owner, --source-idea, --supersedes, --tags). The generated
file is always lint-clean — running ` + "`specscore spec lint`" + ` immediately
afterwards passes.`,
		Args: cobra.ExactArgs(1),
		RunE: runDecisionNew,
	}
	cmd.Flags().String("title", "", "Decision title (defaults to title-cased slug)")
	cmd.Flags().String("owner", "", "author identifier (defaults to $USER)")
	cmd.Flags().String("source-idea", "", "Source Idea slug")
	cmd.Flags().String("supersedes", "", "Decision ID this one replaces")
	cmd.Flags().String("tags", "", "comma-separated tags")
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().Bool("force", false, "overwrite an existing decision file at that slug")
	return cmd
}

func runDecisionNew(cmd *cobra.Command, args []string) error {
	slug := args[0]
	if err := decision.ValidateSlug(slug); err != nil {
		return exitcode.InvalidArgsErrorf("invalid slug %q: %v", slug, err)
	}

	title, _ := cmd.Flags().GetString("title")
	owner, _ := cmd.Flags().GetString("owner")
	sourceIdea, _ := cmd.Flags().GetString("source-idea")
	supersedes, _ := cmd.Flags().GetString("supersedes")
	tags, _ := cmd.Flags().GetString("tags")
	projectFlag, _ := cmd.Flags().GetString("project")
	force, _ := cmd.Flags().GetBool("force")

	if owner == "" {
		if u := os.Getenv("USER"); u != "" {
			owner = u
		}
	}

	specRoot, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}

	specSub := filepath.Join(specRoot, "spec")

	nextNum, err := decisionNextNumberFn(specSub)
	if err != nil {
		return exitcode.UnexpectedErrorf("determining next number: %v", err)
	}

	filename := fmt.Sprintf("%04d-%s.md", nextNum, slug)
	decisionsDir := filepath.Join(specSub, "decisions")
	if err := os.MkdirAll(decisionsDir, 0o755); err != nil {
		return exitcode.UnexpectedErrorf("creating %s: %v", decisionsDir, err)
	}

	// Materialize the spec/ and spec/decisions/ index READMEs when absent so
	// the readme-exists rule is satisfied (it has no fixer) and the
	// post-scaffold `lint --fix` can backfill the new decision's index row.
	if err := ensureDecisionAncestorIndexes(specRoot); err != nil {
		return exitcode.UnexpectedErrorf("materializing decision indexes: %v", err)
	}

	target := filepath.Join(decisionsDir, filename)

	if _, err := os.Stat(target); err == nil && !force {
		return exitcode.ConflictErrorf("decision already exists: %s (pass --force to overwrite)", target)
	}

	opts := decision.ScaffoldOptions{
		Slug:       slug,
		Title:      title,
		Owner:      owner,
		Tags:       tags,
		SourceIdea: sourceIdea,
		Supersedes: supersedes,
	}

	// cli-template-runtime-fetch: a bare decision (no tags / source-idea /
	// supersedes) pulls the published template; otherwise the embedded
	// scaffolder fills those fields.
	bare := tags == "" && sourceIdea == "" && supersedes == ""
	body, err := bareOrEmbedded(bare, "decision", map[string]string{
		"<Decision Name>": templateTitleOrSlug(title, slug),
		"YYYY-MM-DD":      templateTodayUTC(),
		"<your-handle>":   templateOwnerOrUnknown(owner),
	}, cmd.ErrOrStderr(), func() ([]byte, error) {
		return decisionScaffoldFn(opts)
	})
	if err != nil {
		return exitcode.UnexpectedErrorf("scaffolding decision: %v", err)
	}
	if err := os.WriteFile(target, body, 0o644); err != nil {
		return exitcode.UnexpectedErrorf("writing %s: %v", target, err)
	}

	// Run lint in --fix mode to update index, then re-run to verify.
	if _, err := lintLintFn(lint.Options{SpecRoot: specSub, Fix: true}); err != nil {
		_ = os.Remove(target)
		return exitcode.UnexpectedErrorf("running lint fix: %v", err)
	}
	violations, err := lintLintFn(lint.Options{SpecRoot: specSub})
	if err != nil {
		return exitcode.UnexpectedErrorf("running lint: %v", err)
	}
	relTarget, _ := filepath.Rel(specSub, target)
	var own []lint.Violation
	for _, v := range violations {
		if v.Severity == "error" && (v.File == relTarget || strings.HasPrefix(v.File, "decisions/")) {
			own = append(own, v)
		}
	}
	if len(own) > 0 {
		var sb strings.Builder
		sb.WriteString("generated decision failed lint:\n")
		for _, v := range own {
			fmt.Fprintf(&sb, "  %s:%d [%s] %s\n", v.File, v.Line, v.Rule, v.Message)
		}
		return exitcode.UnexpectedError(sb.String())
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", target)
	return nil
}
