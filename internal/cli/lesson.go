package cli

// Features implemented: cli/lesson

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/specscore/specscore-cli/pkg/dryrun"
	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
	"github.com/specscore/specscore-cli/pkg/lint"
	"github.com/specscore/specscore-cli/pkg/projectdef"
	"github.com/spf13/cobra"
)

// lessonCommand returns the "lesson" command group. With no Run/RunE, cobra
// prints help and exits 0 for `specscore lesson`.
func lessonCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "lesson",
		Short: "Record and query process-gap lessons — what we learned but not yet enforced",
	}
	cmd.AddCommand(
		lessonListCommand(),
		lessonInfoCommand(),
		lessonNewCommand(),
		lessonChangeStatusCommand(),
		lessonRecurCommand(),
		lessonTransitionsCommand(),
	)
	return cmd
}

// lessonTransitionsCommand registers `specscore lesson transitions
// [<slug>]`, the read-only counterpart to change-status.
func lessonTransitionsCommand() *cobra.Command {
	return transitionsCommand(lifecycle.KindLesson, "slug", "Show the Lesson status matrix, or one lesson's legal next statuses",
		func(projectFlag, slug string) (string, error) {
			lessonsDir, err := resolveLessonsDir(projectFlag)
			if err != nil {
				return "", err
			}
			return resolveLessonPath(lessonsDir, slug)
		})
}

// lessonNewCommand scaffolds a lint-clean flat Lesson artifact at
// spec/lessons/<slug>.md per the cli/lesson/new Feature.
func lessonNewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new <slug>",
		Short: "Scaffold a new Lesson artifact",
		Long: `Creates a lint-clean Lesson at spec/lessons/<slug>.md, carrying the
artifact-frontmatter-convention frontmatter (format:/status:), the
body-metadata header (Status: Recorded, Date, Owner, Recurred: 0), the four
required sections with TODO prompts — Incident, Process gap, Check,
Enforcement — and the adherence footer.

No flag beyond the slug is required: recording a lesson is meant to be as
close to free as writing it in prose, so the CLI never blocks on missing
detail. Lint checks only that the four sections exist, never their content —
fill them in whenever you have time, not before the entry is saved.`,
		Args: cobra.ExactArgs(1),
		RunE: runLessonNew,
	}
	cmd.Flags().String("title", "", "lesson title (defaults to title-cased slug)")
	cmd.Flags().String("owner", "", "owner/author (defaults to $USER)")
	cmd.Flags().Bool("force", false, "overwrite an existing lesson file at that slug")
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	return cmd
}

func runLessonNew(cmd *cobra.Command, args []string) error {
	slug := args[0]
	if err := lesson.ValidateSlug(slug); err != nil {
		return exitcode.InvalidArgsErrorf("invalid slug %q: %v", slug, err)
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

	target := filepath.Join(root, "spec", "lessons", slug+".md")
	if _, statErr := os.Stat(target); statErr == nil && !force {
		return exitcode.ConflictErrorf("lesson already exists: %s (pass --force to overwrite)", target)
	}

	// Materialize ancestor indexes before the lesson file, mirroring
	// cli/plan/new#req:ancestor-indexes-materialized. Existing files are left
	// untouched.
	if err := ensureLessonAncestorIndexes(root); err != nil {
		return exitcode.UnexpectedErrorf("materializing ancestor indexes: %v", err)
	}

	repl := map[string]string{
		"<Lesson Name>": templateTitleOrSlug(title, slug),
		"YYYY-MM-DD":    templateTodayUTC(),
		"<your-handle>": templateOwnerOrUnknown(owner),
	}
	body, err := bareOrEmbedded(true, "lesson", repl, cmd.ErrOrStderr(), func() ([]byte, error) {
		return lessonScaffoldFn(lesson.ScaffoldOptions{Slug: slug, Title: title, Owner: owner})
	})
	if err != nil {
		return exitcode.UnexpectedErrorf("scaffolding lesson: %v", err)
	}
	if err := os.WriteFile(target, body, 0o644); err != nil {
		return exitcode.UnexpectedErrorf("writing %s: %v", target, err)
	}

	// Run lint in --fix mode to insert the freshly created lesson's index row,
	// then re-run without fix to surface any remaining errors touching this
	// file — mirroring `idea new` (internal/cli/idea.go:runIdeaNew), the
	// established pattern for keeping a freshly scaffolded artifact
	// immediately lint-clean including its index.
	specSub := filepath.Join(root, "spec")
	if _, err := lintLintFn(lint.Options{SpecRoot: specSub, Fix: true}); err != nil {
		_ = os.Remove(target)
		return exitcode.UnexpectedErrorf("running lint fix: %v", err)
	}
	violations, err := lintLintFn(lint.Options{SpecRoot: specSub})
	if err != nil {
		return exitcode.UnexpectedErrorf("running lint: %v", err)
	}
	relTarget, _ := filepath.Rel(specSub, target)
	var own []string
	for _, v := range violations {
		if v.Severity == "error" && (v.File == relTarget || strings.HasPrefix(v.File, "lessons/")) {
			own = append(own, fmt.Sprintf("  %s:%d [%s] %s", v.File, v.Line, v.Rule, v.Message))
		}
	}
	if len(own) > 0 {
		return exitcode.UnexpectedError("generated lesson failed lint:\n" + strings.Join(own, "\n"))
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", target)
	return nil
}

// lessonChangeStatusCommand transitions a Lesson's **Status:** field via the
// shared lifecycle state-machine contract — climbing the enforcement ladder
// (Recorded -> Stated -> Enforced) or retiring the lesson (Withdrawn,
// Superseded). See spec/features/cli/lesson/change-status/README.md.
func lessonChangeStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "change-status <slug> --to=<status>",
		Short: "Transition a Lesson's Status up the enforcement ladder (or retire it)",
		Long: `Transitions spec/lessons/<slug>.md from its current **Status:** to the
value named by --to. The transition is validated against the Lesson
legal-transition matrix below; illegal (from, to) pairs exit 4. On success,
the verb runs ` + "`specscore spec lint --fix`" + ` to keep the lessons index in
sync, prints "<slug>: <from> → <to>" to stdout, and exits 0.

Both dispositions require a reason: --to=withdrawn and --to=superseded
require --note. --to=superseded additionally requires --successor naming the
lesson that replaces this one; it is written as a **Superseded By:**
reference. If anything fails after the status rewrite (lint failure, I/O
error), the on-disk state is restored to its pre-invocation form before the
verb exits.

` + lesson.LegalTransitionMatrix() + `
Examples:

  specscore lesson change-status kinder-fake --to=stated
  specscore lesson change-status kinder-fake --to=enforced
  specscore lesson change-status stale-idea --to=withdrawn --note "turned out to be a one-off, not a pattern"
  specscore lesson change-status old-lesson --to=superseded --note "generalized" --successor newer-lesson
`,
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runLessonChangeStatus,
	}
	cmd.Flags().String("to", "", "target status (required). Legal values: "+
		strings.Join(lesson.LegalChangeStatusTargetNames(), ", ")+" (case-insensitive).")
	cmd.Flags().String("note", "", "markdown appended as a ## Resolution section; required for --to=withdrawn and --to=superseded")
	cmd.Flags().String("successor", "", "slug of the lesson that supersedes this one; required for --to=superseded, rejected otherwise")
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	cmd.Flags().Bool("dry-run", false, "report the transition and every file that would change, writing nothing")
	return cmd
}

func runLessonChangeStatus(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return exitcode.InvalidArgsError("missing required positional argument: <slug>")
	}
	if len(args) > 1 {
		return exitcode.InvalidArgsErrorf(
			"too many positional arguments: change-status accepts exactly one <slug>, got %d", len(args))
	}
	slug := args[0]
	if err := lesson.ValidateSlug(slug); err != nil {
		return exitcode.InvalidArgsErrorf("invalid slug %q: %v", slug, err)
	}

	toRaw, _ := cmd.Flags().GetString("to")
	if strings.TrimSpace(toRaw) == "" {
		return exitcode.InvalidArgsError("missing required flag: --to=<status>")
	}
	to, ok := lifecycle.ParseStatus(lifecycle.KindLesson, toRaw)
	if !ok {
		return exitcode.InvalidArgsErrorf(
			"unrecognized --to value %q for lesson; legal values: %s",
			toRaw, strings.Join(lesson.LegalChangeStatusTargetNames(), ", "))
	}

	note, _ := cmd.Flags().GetString("note")
	successor, _ := cmd.Flags().GetString("successor")

	if to == lifecycle.LessonWithdrawn && strings.TrimSpace(note) == "" {
		return exitcode.InvalidArgsError(
			"transition to Withdrawn requires a reason: pass --note explaining why the lesson was withdrawn")
	}
	if to == lifecycle.LessonSuperseded && strings.TrimSpace(note) == "" {
		return exitcode.InvalidArgsError(
			"transition to Superseded requires a reason: pass --note describing what superseded the lesson")
	}

	if to == lifecycle.LessonSuperseded {
		if strings.TrimSpace(successor) == "" {
			return exitcode.InvalidArgsError(
				"transition to Superseded requires --successor naming the lesson that replaces this one")
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

	if to == lifecycle.LessonSuperseded {
		successor = strings.TrimSpace(successor)
		if _, rerr := lesson.ResolveLessonFile(filepath.Join(specRoot, "spec", "lessons"), successor); rerr != nil {
			return exitcode.InvalidArgsErrorf(
				"successor lesson %q does not resolve to an existing lesson at spec/lessons/%s.md", successor, successor)
		}
	}

	if dryRun, _ := cmd.Flags().GetBool("dry-run"); dryRun {
		result, changes, err := dryrun.Sandbox(specRoot, func(sandboxRoot string) (lesson.ChangeStatusResult, error) {
			return lessonChangeStatusMutate(sandboxRoot, slug, to, note, successor)
		})
		if err != nil {
			return err
		}
		dryrun.PrintReport(cmd.OutOrStdout(), result.Slug, string(result.From), string(result.To), changes)
		return nil
	}

	result, err := lessonChangeStatusMutate(specRoot, slug, to, note, successor)
	if err != nil {
		return err
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s → %s\n",
		result.Slug, string(result.From), string(result.To))
	return nil
}

// lessonChangeStatusMutate performs the Lesson Status transition rooted at
// root (the project root containing spec/). It is the single mutation path
// both the real command and its --dry-run sandbox invoke.
func lessonChangeStatusMutate(root, slug string, to lifecycle.Status, note, successor string) (lesson.ChangeStatusResult, error) {
	return lesson.ChangeStatus(lesson.ChangeStatusOptions{
		SpecRoot:     root,
		Slug:         slug,
		To:           to,
		Note:         note,
		Successor:    successor,
		PostMutation: lesson.PostMutationHook(lintPostMutationHook(filepath.Join(root, "spec"))),
	})
}

// ensureLessonAncestorIndexes materializes spec/README.md and
// spec/lessons/README.md when they don't already exist, using the same
// templates as `specscore init`. Existing files are left untouched.
func ensureLessonAncestorIndexes(root string) error {
	cfg, err := projectdef.ReadSpecConfig(root)
	if err != nil {
		cfg = projectdef.SpecConfig{}
	}
	for _, w := range []struct {
		path    string
		content string
	}{
		{"spec/README.md", specReadmeContent(cfg)},
		{"spec/lessons/README.md", lessonsIndexContent(cfg)},
	} {
		if err := writeMissingIndex(root, w.path, w.content); err != nil {
			return fmt.Errorf("writing %s: %w", w.path, err)
		}
	}
	return nil
}

// resolveLessonsDir resolves the lessons directory from a --project flag or
// CWD. Unlike resolveFeaturesDir, an absent lessons directory is not an
// error: the path is returned regardless (emptiness is handled by the list
// command) — mirroring resolvePlansDir.
func resolveLessonsDir(projectFlag string) (string, error) {
	root, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return "", err
	}
	return filepath.Join(root, "spec", "lessons"), nil
}

// resolveLessonPath joins lessonsDir/<slug>.md and verifies the file exists.
// A missing file yields an exit-3 NotFound error naming the slug.
func resolveLessonPath(lessonsDir, slug string) (string, error) {
	path := filepath.Join(lessonsDir, slug+".md")
	if _, err := os.Stat(path); err != nil {
		return "", exitcode.NotFoundErrorf("lesson not found: %s", slug)
	}
	return path, nil
}
