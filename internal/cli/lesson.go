package cli

// Features implemented: cli/lesson

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

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
		lessonOccurrenceCommand(),
		lessonRelationCommand(),
		lessonImportLegacyCommand(),
		lessonMigrateFlatCommand(),
		lessonCheckCommand(),
	)
	return cmd
}

// lessonNewCommand scaffolds a lint-clean canonical Lesson directory at
// spec/lessons/<slug>/ per the cli/lesson/new Feature.
func lessonNewCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "new <slug>",
		Short: "Scaffold a new Lesson artifact",
		Long: `Creates the canonical compact Lesson at
spec/lessons/<slug>/README.md plus an empty occurrences/ directory. The README
contains controlled classifications, relation/provenance fields, the durable
Lesson and Process Gap, exact occurrence-schema Tracking line, deterministic
Enforcement fields, Open Questions, and the adherence footer.

The command preflights specscore.yaml and non-empty lessons.classifications
before creating any path. Its bounded write set is the requested Lesson, the
declared ancestor/index rows, and the durable prepared event/outbox; it never
runs a repository-wide fixer. A sibling
legacy spec/lessons/<slug>.md is a collision and must be migrated explicitly.

Docs: docs/agent-lessons.md#create-and-record`,
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

	// Creation is deliberately config-gated.  A mutating scaffolder must not
	// turn a legacy tree into a half-migrated tree merely because it happened
	// to find spec/.  In particular, do this before materializing indexes or
	// directories so a failed preflight has a declared empty write set.
	cfg, configErr := projectdef.ReadSpecConfig(root)
	if configErr != nil {
		if os.IsNotExist(unwrapPathError(configErr)) {
			return exitcode.InvalidStateError("lesson new requires specscore.yaml; run `specscore init`, configure non-empty lessons.classifications, then retry")
		}
		return exitcode.InvalidStateErrorf("lesson new requires a valid specscore.yaml: %v", configErr)
	}
	classifications := lessonClassificationsFromConfig(cfg)
	if len(classifications) == 0 {
		return exitcode.InvalidStateError("lesson new requires non-empty lessons.classifications in specscore.yaml; configure the repository vocabulary, then retry")
	}
	seenClassifications := map[string]bool{}
	for _, classification := range classifications {
		if err := lesson.ValidateSlug(classification); err != nil || seenClassifications[classification] {
			return exitcode.InvalidStateError("lesson new requires unique slug-form values in lessons.classifications; fix specscore.yaml before retrying")
		}
		seenClassifications[classification] = true
	}

	lessonsDir := filepath.Join(root, "spec", "lessons")
	target := filepath.Join(lessonsDir, slug, "README.md")
	legacy := filepath.Join(lessonsDir, slug+".md")
	if _, err := os.Stat(legacy); err == nil {
		return exitcode.ConflictErrorf("legacy lesson already exists: %s; migrate it before creating canonical %q", legacy, slug)
	} else if !os.IsNotExist(err) {
		return exitcode.UnexpectedErrorf("preflighting %s: %v", legacy, err)
	}
	if _, statErr := os.Stat(target); statErr == nil && !force {
		return exitcode.ConflictErrorf("lesson already exists: %s (pass --force to overwrite)", target)
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return exitcode.UnexpectedErrorf("preflighting %s: %v", target, statErr)
	}

	body, err := lesson.ScaffoldCanonical(lesson.ScaffoldOptions{Slug: slug, Title: title, Owner: owner}, classifications)
	if err != nil {
		return exitcode.UnexpectedErrorf("scaffolding lesson: %v", err)
	}
	if err := lesson.ValidateSafeContent("Lesson scaffold", string(body)); err != nil {
		return exitcode.InvalidArgsErrorf("unsafe Lesson content: %v", err)
	}

	snapshot, err := snapshotLessonScaffold(root, target)
	if err != nil {
		return exitcode.UnexpectedErrorf("snapshotting declared write set: %v", err)
	}
	prepared, err := prepareLessonEvent(root, "lesson.created", slug, map[string]any{"classifications": classifications}, time.Time{})
	if err != nil {
		return exitcode.UnexpectedErrorf("preparing lesson event: %v", err)
	}
	fail := func(message string, cause error) error {
		if rollbackErr := snapshot.restore(); rollbackErr != nil {
			message += fmt.Sprintf("; rollback failed: %v", rollbackErr)
		}
		_ = prepared.Abort()
		return exitcode.UnexpectedErrorf(message+": %v", cause)
	}

	// Materialize only the two declared ancestor indexes. Existing files are
	// byte-preserved; the narrow row upsert below owns the new Lesson's row.
	if err := ensureLessonAncestorIndexes(root); err != nil {
		return fail("materializing ancestor indexes", err)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return fail("creating lesson directory", err)
	}
	if err := os.MkdirAll(filepath.Join(filepath.Dir(target), "occurrences"), 0o755); err != nil {
		return fail("creating occurrences directory", err)
	}
	if err := os.WriteFile(target, body, 0o644); err != nil {
		return fail("writing Lesson", err)
	}

	specSub := filepath.Join(root, "spec")
	parsed, err := lesson.Parse(target)
	if err != nil {
		return fail("parsing generated Lesson", err)
	}
	if err := lessonIndexUpsertFn(specSub, parsed); err != nil {
		return fail("upserting declared Lesson index row", err)
	}
	violations, err := lintLintFn(lint.Options{SpecRoot: specSub})
	if err != nil {
		return fail("running read-only lint", err)
	}
	relTarget, _ := filepath.Rel(specSub, target)
	var own []string
	for _, v := range violations {
		ownedIndexFinding := v.File == "lessons/README.md" && (v.Rule == "L-003" || v.Rule == "L-004") && strings.Contains(v.Message, slug)
		if v.Severity == "error" && (v.File == relTarget || strings.HasPrefix(v.File, filepath.ToSlash(filepath.Join("lessons", slug, "occurrences"))) || ownedIndexFinding) {
			own = append(own, fmt.Sprintf("  %s:%d [%s] %s", v.File, v.Line, v.Rule, v.Message))
		}
	}
	if len(own) > 0 {
		return fail("generated Lesson failed lint", fmt.Errorf("%s", strings.Join(own, "\n")))
	}
	result, commitErr := prepared.Commit(cmd.Context())
	if commitErr != nil {
		// If Abort succeeds, the event never committed and the artifact can be
		// safely rolled back. If it cannot abort, the ledger is already durable;
		// keep the successful mutation and report delivery as pending.
		if abortErr := prepared.Abort(); abortErr == nil {
			_ = snapshot.restore()
			return exitcode.UnexpectedErrorf("committing lesson event: %v", commitErr)
		}
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: Lesson created; durable event delivery is pending: %v\n", commitErr)
	}
	for _, delivery := range result.Failed {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: subscriber %s remains pending: %v\n", delivery.Name, delivery.Err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", target)
	return nil
}

type fileSnapshot struct {
	path   string
	data   []byte
	mode   os.FileMode
	exists bool
}

type lessonScaffoldSnapshot struct {
	files         []fileSnapshot
	lessonsDir    string
	lessonDir     string
	occurrences   string
	hadLessonsDir bool
	hadLessonDir  bool
	hadOccurrence bool
}

func snapshotLessonScaffold(root, target string) (lessonScaffoldSnapshot, error) {
	s := lessonScaffoldSnapshot{
		lessonsDir:  filepath.Join(root, "spec", "lessons"),
		lessonDir:   filepath.Dir(target),
		occurrences: filepath.Join(filepath.Dir(target), "occurrences"),
	}
	for _, p := range []string{filepath.Join(root, "spec", "README.md"), filepath.Join(root, "spec", "lessons", "README.md"), target} {
		b, err := os.ReadFile(p)
		if err == nil {
			info, statErr := os.Stat(p)
			if statErr != nil {
				return s, statErr
			}
			s.files = append(s.files, fileSnapshot{path: p, data: b, mode: info.Mode().Perm(), exists: true})
		} else if os.IsNotExist(err) {
			s.files = append(s.files, fileSnapshot{path: p})
		} else {
			return s, err
		}
	}
	s.hadLessonsDir = pathIsDir(s.lessonsDir)
	s.hadLessonDir = pathIsDir(s.lessonDir)
	s.hadOccurrence = pathIsDir(s.occurrences)
	return s, nil
}

func pathIsDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func (s lessonScaffoldSnapshot) restore() error {
	var first error
	for _, f := range s.files {
		if f.exists {
			if err := os.WriteFile(f.path, f.data, f.mode); err != nil && first == nil {
				first = err
			}
		} else if err := os.Remove(f.path); err != nil && !os.IsNotExist(err) && first == nil {
			first = err
		}
	}
	if !s.hadOccurrence {
		if err := os.Remove(s.occurrences); err != nil && !os.IsNotExist(err) && first == nil {
			first = err
		}
	}
	if !s.hadLessonDir {
		if err := os.Remove(s.lessonDir); err != nil && !os.IsNotExist(err) && first == nil {
			first = err
		}
	}
	if !s.hadLessonsDir {
		if err := os.Remove(s.lessonsDir); err != nil && !os.IsNotExist(err) && first == nil {
			first = err
		}
	}
	return first
}

func lessonClassifications(root string) []string {
	cfg, err := projectdef.ReadSpecConfig(root)
	if err != nil {
		return nil
	}
	return lessonClassificationsFromConfig(cfg)
}

func lessonClassificationsFromConfig(cfg projectdef.SpecConfig) []string {
	if cfg.Extras == nil {
		return nil
	}
	raw, ok := cfg.Extras["lessons"].(map[string]any)
	if !ok {
		return nil
	}
	values, ok := raw["classifications"].([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, v := range values {
		if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
			out = append(out, strings.TrimSpace(s))
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

// unwrapPathError makes missing-config detection work through the contextual
// error added by projectdef.ReadSpecConfig without treating malformed YAML as
// a missing configuration.
func unwrapPathError(err error) error {
	for {
		if u, ok := err.(interface{ Unwrap() error }); ok && u.Unwrap() != nil {
			err = u.Unwrap()
			continue
		}
		return err
	}
}

// lessonChangeStatusCommand transitions a Lesson's **Status:** field via the
// shared lifecycle state-machine contract — climbing the enforcement ladder
// (Recorded -> Stated -> Enforced) or retiring the lesson (Withdrawn,
// Superseded). See spec/features/cli/lesson/change-status/README.md.
func lessonChangeStatusCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "change-status <slug> --to=<status>",
		Short: "Transition a Lesson's Status up the enforcement ladder (or retire it)",
		Long: `Transitions the canonical spec/lessons/<slug>/README.md or a
compatibility spec/lessons/<slug>.md from its current **Status:** to the value
named by --to. The transition is validated against the Lesson
legal-transition matrix below; illegal (from, to) pairs exit 4. On success,
the verb upserts only that Lesson's index row, runs lint read-only, prints
"<slug>: <from> → <to>" to stdout, and exits 0. Repository-wide fixes remain
exclusive to an explicit ` + "`specscore spec lint --fix`" + ` command.

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
				"successor lesson %q does not resolve to an existing canonical or compatibility Lesson", successor)
		}
	}
	postMutation, err := prepareLessonPostMutation(specRoot, slug)
	if err != nil {
		return err
	}
	prepared, err := prepareLessonEvent(specRoot, "lesson.lifecycle-changed", slug, map[string]any{"to": string(to)}, time.Time{})
	if err != nil {
		return exitcode.UnexpectedErrorf("preparing lifecycle event: %v", err)
	}

	result, err := lesson.ChangeStatus(lesson.ChangeStatusOptions{
		SpecRoot:     specRoot,
		Slug:         slug,
		To:           to,
		Note:         note,
		Successor:    successor,
		PostMutation: postMutation,
	})
	if err != nil {
		_ = prepared.Abort()
		return err
	}
	delivery, commitErr := prepared.Commit(cmd.Context())
	if commitErr != nil {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: status changed; durable event delivery is pending: %v\n", commitErr)
	}
	for _, failure := range delivery.Failed {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: subscriber %s remains pending: %v\n", failure.Name, failure.Err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s → %s\n",
		result.Slug, string(result.From), string(result.To))
	return nil
}

// prepareLessonPostMutation snapshots the only index file owned by a Lesson
// lifecycle change. The returned hook upserts that Lesson's row, then runs
// lint read-only. It deliberately does not invoke the repository-wide fixer:
// only an explicit `specscore spec lint --fix` may migrate unrelated content
// such as an Outstanding Questions heading.
func prepareLessonPostMutation(root, slug string) (lesson.PostMutationHook, error) {
	specSub := filepath.Join(root, "spec")
	indexPath := filepath.Join(specSub, "lessons", "README.md")
	indexBefore, err := os.ReadFile(indexPath)
	if err != nil {
		return nil, exitcode.UnexpectedErrorf("snapshotting lessons index: %v", err)
	}
	info, err := os.Stat(indexPath)
	if err != nil {
		return nil, exitcode.UnexpectedErrorf("snapshotting lessons index mode: %v", err)
	}
	restore := func() error { return os.WriteFile(indexPath, indexBefore, info.Mode().Perm()) }
	fail := func(message string, cause error) error {
		if rollbackErr := restore(); rollbackErr != nil {
			message += fmt.Sprintf("; index rollback failed: %v", rollbackErr)
		}
		return exitcode.UnexpectedErrorf(message+": %v", cause)
	}

	return func() error {
		path, err := lesson.ResolveLessonFile(filepath.Join(specSub, "lessons"), slug)
		if err != nil {
			return fail("resolving rewritten Lesson", err)
		}
		updated, err := lesson.Parse(path)
		if err != nil {
			return fail("parsing rewritten Lesson", err)
		}
		if err := lessonIndexUpsertFn(specSub, updated); err != nil {
			return fail("upserting Lesson index row", err)
		}
		violations, err := lintLintFn(lint.Options{SpecRoot: specSub})
		if err != nil {
			return fail("running read-only lint", err)
		}
		var errs []lint.Violation
		for _, violation := range violations {
			if violation.Severity == "error" {
				errs = append(errs, violation)
			}
		}
		if len(errs) == 0 {
			return nil
		}
		var message strings.Builder
		message.WriteString("lint failed after status rewrite")
		for _, violation := range errs {
			fmt.Fprintf(&message, "\n  %s:%d [%s] %s", violation.File, violation.Line, violation.Rule, violation.Message)
		}
		return fail(message.String(), fmt.Errorf("%d error-severity violation(s)", len(errs)))
	}, nil
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
	return lesson.ResolveLessonFile(lessonsDir, slug)
}
