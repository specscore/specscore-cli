package cli

import (
	"fmt"
	"path/filepath"
	"time"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/spf13/cobra"
)

func afterFlatMigrationPhase(deps lessonCLIDeps, phase string) error {
	if err := deps.afterFlatPhase(phase); err != nil {
		return exitcode.UnexpectedErrorf("injected flat-migration crash after %s: %v", phase, err)
	}
	return nil
}

func lessonMigrateFlatCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "migrate-flat <slug>",
		Short: "Migrate one committed flat Lesson to the canonical directory form",
		Long: `Migrates exactly spec/lessons/<slug>.md to
spec/lessons/<slug>/README.md plus append-only occurrences. The current flat
bytes must match the repository's committed HEAD. The canonical README and a
private-data-free manifest retain repository, full revision, path, byte range,
and hashes; raw legacy prose is never copied into migration metadata.

The source Lesson itself becomes one provider occurrence. Each structured
Recurrences bullet becomes one further occurrence; the command refuses a
Recurred/bullet mismatch rather than inventing history. Publication is
exclusive and crash-resumable across canonical artifacts, the exact index row,
and one deterministic prepared event. It atomically moves the flat source and
completed marker without replacement into the private gitignored
.specscore/recovery/flat-migration/<event-uuid>/ namespace; it never unlinks a
post-publication path. A source at Enforced requires explicit reviewed --control,
--verification, and --evidence values. Git provenance alone never certifies
enforcement. The command never runs a repository-wide fixer.

Docs: docs/agent-lessons.md#migrate-a-structured-flat-lesson`,
		Args:          cobra.ExactArgs(1),
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runLessonMigrateFlat,
	}
	cmd.Flags().StringSlice("classification", nil, "reviewed classification (repeatable; each value must exist in specscore.yaml)")
	cmd.Flags().String("control", "", "reviewed deterministic control (required when the source status is Enforced)")
	cmd.Flags().String("verification", "", "reviewed reproducible verification (required when the source status is Enforced)")
	cmd.Flags().String("evidence", "", "reviewed stable evidence reference (required when the source status is Enforced)")
	cmd.Flags().String("format", "text", "output format: text, yaml, json")
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	return cmd
}

func runLessonMigrateFlat(cmd *cobra.Command, args []string) error {
	return runLessonMigrateFlatWithDeps(cmd, args, defaultLessonCLIDeps())
}

func runLessonMigrateFlatWithDeps(cmd *cobra.Command, args []string, deps lessonCLIDeps) error {
	slug := args[0]
	if err := lesson.ValidateSlug(slug); err != nil {
		return exitcode.InvalidArgsErrorf("invalid slug %q: %v", slug, err)
	}
	format, _ := cmd.Flags().GetString("format")
	if err := validateFormat(format); err != nil {
		return err
	}
	selected, _ := cmd.Flags().GetStringSlice("classification")
	if len(selected) == 0 {
		return exitcode.InvalidArgsError("migrate-flat requires at least one reviewed --classification")
	}
	projectFlag, _ := cmd.Flags().GetString("project")
	root, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}
	cfg, err := deps.readConfig(root)
	if err != nil {
		return exitcode.InvalidStateErrorf("migrate-flat requires a valid specscore.yaml: %v", err)
	}
	configured := lessonClassificationsFromConfig(cfg)
	if len(configured) == 0 {
		return exitcode.InvalidStateError("migrate-flat requires non-empty lessons.classifications in specscore.yaml")
	}
	allowed := map[string]bool{}
	for _, value := range configured {
		allowed[value] = true
	}
	seen := map[string]bool{}
	for _, value := range selected {
		if !allowed[value] || seen[value] {
			return exitcode.InvalidArgsErrorf("classification %q is duplicated or outside lessons.classifications", value)
		}
		seen[value] = true
	}

	lessonsDir := filepath.Join(root, "spec", "lessons")
	control, _ := cmd.Flags().GetString("control")
	verification, _ := cmd.Flags().GetString("verification")
	evidence, _ := cmd.Flags().GetString("evidence")
	opts := lesson.FlatMigrationOptions{
		LessonsDir: lessonsDir, Classifications: selected, Slug: slug,
		Control: control, Verification: verification, Evidence: evidence,
	}
	return deps.withMutationLock(root, slug, func() error {
		return runLessonMigrateFlatLocked(cmd, format, root, slug, selected, opts, deps)
	})
}

func runLessonMigrateFlatLocked(cmd *cobra.Command, format, root, slug string, selected []string, opts lesson.FlatMigrationOptions, deps lessonCLIDeps) error {
	lessonsDir := opts.LessonsDir
	preflight, err := deps.preflightFlat(opts)
	if err != nil {
		return exitcode.InvalidStateErrorf("flat migration preflight refused: %v", err)
	}
	if preflight.AlreadyMigrated {
		result, migrateErr := deps.migrateFlat(opts)
		if migrateErr != nil {
			return exitcode.InvalidStateErrorf("verifying completed flat migration: %v", migrateErr)
		}
		return writeLegacyOutput(cmd, format, result)
	}
	opts.EventUUID = preflight.EventUUID
	// PreflightFlatMigration already validates canonical UTC RFC3339 source
	// identity; parsing cannot fail after that contract succeeds.
	eventAt, _ := time.Parse(time.RFC3339, preflight.Source.CommittedAt)
	markerPath := filepath.Join(lessonsDir, ".flat-migration-"+slug+".json")

	prepared, err := deps.prepareEventWithID(root, "lesson.flat-migrated", slug, map[string]any{
		"classifications": selected,
		"source_path":     preflight.Source.Path,
		"source_revision": preflight.Source.Revision,
		"source_sha256":   preflight.Source.SHA256,
	}, eventAt, preflight.EventUUID)
	if err != nil {
		return exitcode.UnexpectedErrorf("preparing flat-migration event: %v", err)
	}
	result, err := deps.migrateFlat(opts)
	if err != nil {
		if recovery, resolved := prepared.ResolveMutationFailure("migrating flat Lesson", err); recovery {
			return exitcode.UnexpectedErrorf("%v", resolved)
		} else {
			return exitcode.InvalidStateErrorf("flat migration refused: %v", resolved)
		}
	}
	failAfterMutation := func(message string, cause error) error {
		// MigrateFlat returning success means its durable marker and at least one
		// artifact path are visible. No later CLI adapter may restore snapshots or
		// delete paths: a concurrent occurrence/index row may already exist.
		_, resolved := prepared.ResolveMutationFailure(message, &lesson.MutationError{Outcome: lesson.MutationUncertain, Err: cause})
		return exitcode.UnexpectedErrorf("%v", resolved)
	}
	if err := afterFlatMigrationPhase(deps, "artifact-publication"); err != nil {
		return failAfterMutation("continuing after flat artifact publication", err)
	}

	if err := reconcileLockedLessons(root, []string{slug}, []string{result.ManifestPath, markerPath}, deps); err != nil {
		return failAfterMutation("reconciling flat migration", err)
	}
	if err := afterFlatMigrationPhase(deps, "index-upsert"); err != nil {
		return err
	}
	delivery, commitErr := prepared.Commit(cmd.Context())
	if commitErr != nil {
		return exitcode.UnexpectedErrorf("flat migration remains durably resumable; event %s commit or delivery is pending: %v", prepared.event.UUID, commitErr)
	}
	for _, failure := range delivery.Failed {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: subscriber %s remains pending: %v\n", failure.Name, failure.Err)
	}
	if err := afterFlatMigrationPhase(deps, "event-commit"); err != nil {
		return err
	}
	if err := deps.finalizeFlat(opts, preflight.EventUUID); err != nil {
		return exitcode.UnexpectedErrorf("finalizing flat migration after durable event commit: %v", err)
	}
	result.PendingFinalize = false
	return writeLegacyOutput(cmd, format, result)
}
