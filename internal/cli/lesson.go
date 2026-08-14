package cli

// Features implemented: cli/lesson

import (
	"bytes"
	"errors"
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
		lessonAgentsCommand(),
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
	return runLessonNewWithDeps(cmd, args, defaultLessonCLIDeps())
}

func runLessonNewWithDeps(cmd *cobra.Command, args []string, deps lessonCLIDeps) error {
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
	cfg, configErr := deps.readConfig(root)
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
	occurrenceKeepFile := filepath.Join(filepath.Dir(target), "occurrences", lesson.OccurrenceStoreKeepFile)
	legacy := filepath.Join(lessonsDir, slug+".md")
	if _, err := deps.fs.stat(legacy); err == nil {
		return exitcode.ConflictErrorf("legacy lesson already exists: %s; migrate it before creating canonical %q", legacy, slug)
	} else if !os.IsNotExist(err) {
		return exitcode.UnexpectedErrorf("preflighting %s: %v", legacy, err)
	}
	targetExisted := false
	var targetBefore []byte
	if _, statErr := deps.fs.stat(target); statErr == nil && !force {
		return exitcode.ConflictErrorf("lesson already exists: %s (pass --force to overwrite)", target)
	} else if statErr == nil {
		targetExisted = true
		targetBefore, err = deps.fs.read(target)
		if err != nil {
			return exitcode.UnexpectedErrorf("reading existing Lesson before --force: %v", err)
		}
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return exitcode.UnexpectedErrorf("preflighting %s: %v", target, statErr)
	}

	body, err := deps.scaffoldCanonical(lesson.ScaffoldOptions{Slug: slug, Title: title, Owner: owner}, classifications)
	if err != nil {
		return exitcode.UnexpectedErrorf("scaffolding Lesson: %v", err)
	}
	if err := lesson.ValidateSafeContent("Lesson scaffold", string(body)); err != nil {
		return exitcode.InvalidArgsErrorf("unsafe Lesson content: %v", err)
	}

	if err := preflightLessonScaffoldWriteSetWithOps(root, target, deps.fs); err != nil {
		return exitcode.UnexpectedErrorf("preflighting declared write set: %v", err)
	}
	eventPayload := map[string]any{"classifications": classifications}
	intentFacts := map[string]any{"content_sha256": lessonIntentDigest(body)}
	prepared, err := deps.resumeEvent(root, "lesson.created", slug, eventPayload, intentFacts)
	if err != nil {
		return exitcode.UnexpectedErrorf("inspecting prepared lesson event: %v", err)
	}
	transaction := newLessonMutationCoordinator(prepared, deps.durable)
	var prepareErr error
	noOp := false
	fail := func(message string, cause error) error {
		// Every failure after this point may follow a visible file or directory.
		// Retain it with the prepared event; a whole-tree restore cannot prove
		// ownership and could erase a concurrent Lesson/index mutation.
		if recovery, resolved := transaction.ResolveFailure(message, cause); recovery {
			return exitcode.UnexpectedErrorf("%v", resolved)
		} else {
			return exitcode.UnexpectedErrorf(message+": %v", resolved)
		}
	}

	specSub := filepath.Join(root, "spec")
	mutationErr := deps.withMutationLock(root, slug, func() error {
		if targetExisted {
			current, err := deps.fs.read(target)
			if err != nil {
				return &lesson.MutationError{Outcome: lesson.MutationPrePublication, Err: fmt.Errorf("re-reading --force target: %w", err)}
			}
			if !bytes.Equal(current, targetBefore) {
				return &lesson.MutationError{Outcome: lesson.MutationPrePublication, Err: fmt.Errorf("lesson changed after --force preflight; retry against the current artifact")}
			}
			if _, markerErr := deps.fs.stat(occurrenceKeepFile); markerErr == nil && bytes.Equal(current, body) && prepared == nil {
				noOp = true
				return nil
			} else if markerErr != nil && !os.IsNotExist(markerErr) {
				return &lesson.MutationError{Outcome: lesson.MutationPrePublication, Err: fmt.Errorf("preflighting occurrence-store marker: %w", markerErr)}
			}
		}
		if prepared == nil {
			prepared, prepareErr = deps.prepareIntentEvent(root, "lesson.created", slug, eventPayload, intentFacts, time.Time{})
			if prepareErr != nil {
				return &lesson.MutationError{Outcome: lesson.MutationPrePublication, Err: prepareErr}
			}
			transaction = newLessonMutationCoordinator(prepared, deps.durable)
		}
		// Materialize only the two declared ancestor indexes. Existing files are
		// byte-preserved; the narrow row upsert below owns this Lesson's row.
		if err := ensureLessonAncestorIndexes(root); err != nil {
			return fmt.Errorf("materializing ancestor indexes: %w", err)
		}
		if err := deps.fs.mkdirAll(filepath.Dir(target), 0o755); err != nil {
			return fmt.Errorf("creating lesson directory: %w", err)
		}
		if err := deps.fs.mkdirAll(filepath.Join(filepath.Dir(target), "occurrences"), 0o755); err != nil {
			return fmt.Errorf("creating occurrences directory: %w", err)
		}
		if !targetExisted || !bytes.Equal(targetBefore, body) {
			var publishErr error
			if targetExisted {
				publishErr = deps.rewriteAtomic(target, body)
			} else {
				publishErr = deps.publishExclusive(target, body, 0o644)
			}
			if publishErr != nil {
				return fmt.Errorf("writing Lesson: %w", publishErr)
			}
		}
		if _, err := deps.fs.stat(occurrenceKeepFile); os.IsNotExist(err) {
			if err := deps.publishExclusive(occurrenceKeepFile, nil, 0o644); err != nil {
				return fmt.Errorf("writing occurrence-store marker: %w", err)
			}
		} else if err != nil {
			return fmt.Errorf("preflighting occurrence-store marker: %w", err)
		}
		parsed, err := deps.parse(target)
		if err != nil {
			return fmt.Errorf("parsing generated Lesson: %w", err)
		}
		if err := deps.indexUpsert(specSub, parsed); err != nil {
			return fmt.Errorf("upserting declared Lesson index row: %w", err)
		}
		violations, err := deps.lint(lint.Options{SpecRoot: specSub})
		if err != nil {
			return fmt.Errorf("running read-only lint: %w", err)
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
			return fmt.Errorf("generated Lesson failed lint: %s", strings.Join(own, "\n"))
		}
		return transaction.Fence(
			filepath.Join(root, "spec", "README.md"),
			filepath.Join(root, "spec", "lessons", "README.md"),
			target,
			filepath.Join(filepath.Dir(target), "occurrences"),
		)
	})
	if mutationErr != nil {
		if prepareErr != nil {
			return exitcode.UnexpectedErrorf("preparing lesson event: %v", prepareErr)
		}
		return fail("creating Lesson transaction", mutationErr)
	}
	if noOp {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", target)
		return nil
	}
	result, commitErr := transaction.Commit(cmd.Context())
	if commitErr != nil {
		return exitcode.UnexpectedErrorf("Lesson created but event publication is pending for event %s: %v", prepared.event.UUID, commitErr)
	}
	for _, delivery := range result.Failed {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: subscriber %s remains pending: %v\n", delivery.Name, delivery.Err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n", target)
	return nil
}

func preflightLessonScaffoldWriteSetWithOps(root, target string, fs lessonFileOps) error {
	for _, p := range []string{filepath.Join(root, "spec", "README.md"), filepath.Join(root, "spec", "lessons", "README.md"), target} {
		_, err := fs.stat(p)
		if err == nil {
			if _, readErr := fs.read(p); readErr != nil {
				return readErr
			}
		} else if os.IsNotExist(err) {
			continue
		} else {
			return err
		}
	}
	return nil
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
reference. If anything fails after the status rewrite (lint, I/O, or durability
fence), the command exits recovery-required with its prepared event and visible
state retained; it never restores a whole snapshot over concurrent work.

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
	return runLessonChangeStatusWithDeps(cmd, args, defaultLessonCLIDeps())
}

func runLessonChangeStatusWithDeps(cmd *cobra.Command, args []string, deps lessonCLIDeps) error {
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
	// Establish every non-mutating lifecycle precondition before preparing an
	// outbox record. This keeps ordinary missing/illegal-transition failures
	// write-free rather than manufacturing a recovery item for a mutation that
	// could not have started.
	path, err := lesson.ResolveLessonFile(filepath.Join(specRoot, "spec", "lessons"), slug)
	if err != nil {
		return err
	}
	if _, err := lifecycle.Validate(lifecycle.KindLesson, path, to); err != nil {
		var transition *lifecycle.InvalidTransitionError
		if errors.As(err, &transition) {
			return exitcode.InvalidStateErrorf("invalid transition: lesson %q is in status %q", slug, string(transition.From))
		}
		return exitcode.UnexpectedErrorf("reading lesson status: %v", err)
	}
	postMutation, err := prepareLessonPostMutationWithDeps(specRoot, slug, deps)
	if err != nil {
		return err
	}
	prepared, err := deps.prepareEvent(specRoot, "lesson.lifecycle-changed", slug, map[string]any{"to": string(to)}, time.Time{})
	if err != nil {
		return exitcode.UnexpectedErrorf("preparing lifecycle event: %v", err)
	}
	transaction := newLessonMutationCoordinator(prepared, deps.durable)

	result, err := deps.changeStatus(lesson.ChangeStatusOptions{
		SpecRoot:     specRoot,
		Slug:         slug,
		To:           to,
		Note:         note,
		Successor:    successor,
		PostMutation: postMutation,
	})
	if err != nil {
		if recovery, resolved := transaction.ResolveFailure("changing lesson status", err); recovery {
			return exitcode.UnexpectedErrorf("%v", resolved)
		} else {
			return resolved
		}
	}
	delivery, commitErr := transaction.Commit(cmd.Context())
	if commitErr != nil {
		return exitcode.UnexpectedErrorf("status changed but event publication is pending for event %s: %v", prepared.event.UUID, commitErr)
	}
	for _, failure := range delivery.Failed {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: subscriber %s remains pending: %v\n", failure.Name, failure.Err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: %s → %s\n",
		result.Slug, string(result.From), string(result.To))
	return nil
}

// prepareLessonPostMutation returns the shared lifecycle completion hook. It
// upserts only this Lesson's row, validates read-only, and durably fences both
// artifact and index before the event can commit. It never restores a whole
// index snapshot: after publication, retaining an uncertain prepared event is
// safer than erasing a concurrent row.
func prepareLessonPostMutationWithDeps(root, slug string, deps lessonCLIDeps) (lesson.PostMutationHook, error) {
	specSub := filepath.Join(root, "spec")
	indexPath := filepath.Join(specSub, "lessons", "README.md")
	if _, err := deps.fs.stat(indexPath); err != nil {
		return nil, exitcode.UnexpectedErrorf("preflighting lessons index: %v", err)
	}

	return func() error {
		path, err := lesson.ResolveLessonFile(filepath.Join(specSub, "lessons"), slug)
		if err != nil {
			return exitcode.UnexpectedErrorf("resolving rewritten Lesson: %v", err)
		}
		updated, err := deps.parse(path)
		if err != nil {
			return exitcode.UnexpectedErrorf("parsing rewritten Lesson: %v", err)
		}
		if err := deps.indexUpsert(specSub, updated); err != nil {
			return exitcode.UnexpectedErrorf("upserting Lesson index row: %v", err)
		}
		violations, err := deps.lint(lint.Options{SpecRoot: specSub})
		if err != nil {
			return exitcode.UnexpectedErrorf("running read-only lint: %v", err)
		}
		var errs []lint.Violation
		for _, violation := range violations {
			if violation.Severity == "error" {
				errs = append(errs, violation)
			}
		}
		if len(errs) > 0 {
			var message strings.Builder
			message.WriteString("lint failed after status rewrite")
			for _, violation := range errs {
				fmt.Fprintf(&message, "\n  %s:%d [%s] %s", violation.File, violation.Line, violation.Rule, violation.Message)
			}
			return exitcode.UnexpectedErrorf("%s: %d error-severity violation(s)", message.String(), len(errs))
		}
		if err := newLessonMutationCoordinator(nil, deps.durable).Fence(path, indexPath); err != nil {
			return exitcode.UnexpectedErrorf("%v", err)
		}
		return nil
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
