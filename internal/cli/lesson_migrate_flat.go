package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/specscore/specscore-cli/pkg/lint"
	"github.com/specscore/specscore-cli/pkg/projectdef"
	"github.com/spf13/cobra"
)

// lessonMigrateFlatPhaseHook is a test-only crash seam. Production leaves it
// nil. A non-nil error deliberately returns without rollback so tests can
// prove that the durable marker and deterministic event resume each boundary.
var lessonMigrateFlatPhaseHook func(string) error

func injectFlatMigrationCrash(phase string) error {
	if lessonMigrateFlatPhaseHook == nil {
		return nil
	}
	if err := lessonMigrateFlatPhaseHook(phase); err != nil {
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
and one deterministic prepared event; it removes only the exact verified flat
source. A source at Enforced requires explicit reviewed --control,
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
	return runLessonMigrateFlatWithDeps(cmd, args, defaultLessonCommandDeps())
}

func runLessonMigrateFlatWithDeps(cmd *cobra.Command, args []string, deps lessonCommandDeps) error {
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
	cfg, err := projectdef.ReadSpecConfig(root)
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
	preflight, err := deps.preflightFlatMigration(opts)
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
	eventAt, err := time.Parse(time.RFC3339, preflight.Source.CommittedAt)
	if err != nil || eventAt.Location() != time.UTC {
		return exitcode.InvalidStateError("flat migration source has a non-UTC committed_at timestamp")
	}
	resuming := preflight.PendingTransaction
	flatPath := filepath.Join(lessonsDir, slug+".md")
	markerPath := filepath.Join(lessonsDir, ".flat-migration-"+slug+".json")
	indexPath := filepath.Join(lessonsDir, "README.md")
	var flatBefore, indexBefore []byte
	indexExisted := false
	if !resuming {
		flatBefore, err = deps.readFile(flatPath)
		if err != nil {
			return exitcode.UnexpectedErrorf("reading flat Lesson: %v", err)
		}
		indexBefore, err = deps.readFile(indexPath)
		indexExisted = err == nil
		if err != nil && !os.IsNotExist(err) {
			return exitcode.UnexpectedErrorf("reading lessons index: %v", err)
		}
	}

	prepared, err := deps.prepareLessonEventWithID(root, "lesson.flat-migrated", slug, map[string]any{
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
	if err := injectFlatMigrationCrash("artifact-publication"); err != nil {
		return err
	}

	rollbackFresh := func() error {
		if resuming {
			return nil
		}
		var first error
		if err := deps.removeAll(filepath.Dir(result.CanonicalPath)); err != nil {
			first = err
		}
		if err := deps.writeFile(flatPath, flatBefore, 0o644); err != nil && first == nil {
			first = err
		}
		if result.ManifestPath != "" {
			if err := deps.remove(result.ManifestPath); err != nil && !os.IsNotExist(err) && first == nil {
				first = err
			}
		}
		if indexExisted {
			if err := deps.writeFile(indexPath, indexBefore, 0o644); err != nil && first == nil {
				first = err
			}
		} else if err := deps.remove(indexPath); err != nil && !os.IsNotExist(err) && first == nil {
			first = err
		}
		if err := deps.remove(markerPath); err != nil && !os.IsNotExist(err) && first == nil {
			first = err
		}
		return first
	}
	failAfterMutation := func(message string, cause error) error {
		if resuming {
			if recovery, resolved := prepared.ResolveMutationFailure(message, cause); recovery {
				return exitcode.UnexpectedErrorf("%v", resolved)
			} else {
				return exitcode.UnexpectedErrorf(message+": %v", resolved)
			}
		}
		failure := lesson.CompensatePublication(rollbackFresh, cause)
		if recovery, resolved := prepared.ResolveMutationFailure(message, failure); recovery {
			return exitcode.UnexpectedErrorf("%v", resolved)
		} else {
			return exitcode.UnexpectedErrorf(message+": %v", resolved)
		}
	}

	parsed, err := lesson.Parse(result.CanonicalPath)
	if err != nil {
		return failAfterMutation("parsing canonical migrated Lesson", err)
	}
	if err := lessonIndexUpsertFn(filepath.Join(root, "spec"), parsed); err != nil {
		return failAfterMutation("upserting migrated Lesson index row", err)
	}
	violations, err := lintLintFn(lint.Options{SpecRoot: filepath.Join(root, "spec")})
	if err != nil {
		return failAfterMutation("running read-only lint", err)
	}
	relTarget, _ := filepath.Rel(filepath.Join(root, "spec"), result.CanonicalPath)
	var own []string
	for _, violation := range violations {
		ownedIndexFinding := violation.File == "lessons/README.md" && (violation.Rule == "L-003" || violation.Rule == "L-004") && strings.Contains(violation.Message, slug)
		if violation.Severity == "error" && (violation.File == relTarget || strings.HasPrefix(violation.File, filepath.ToSlash(filepath.Join("lessons", slug, "occurrences"))) || ownedIndexFinding) {
			own = append(own, fmt.Sprintf("%s:%d [%s] %s", violation.File, violation.Line, violation.Rule, violation.Message))
		}
	}
	if len(own) != 0 {
		return failAfterMutation("migrated Lesson failed lint", fmt.Errorf("%s", strings.Join(own, "; ")))
	}
	if err := injectFlatMigrationCrash("index-upsert"); err != nil {
		return err
	}
	delivery, commitErr := prepared.Commit(cmd.Context())
	if commitErr != nil {
		return exitcode.UnexpectedErrorf("flat migration remains durably resumable; event %s commit or delivery is pending: %v", prepared.event.UUID, commitErr)
	}
	for _, failure := range delivery.Failed {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: subscriber %s remains pending: %v\n", failure.Name, failure.Err)
	}
	if err := injectFlatMigrationCrash("event-commit"); err != nil {
		return err
	}
	if err := deps.finalizeFlatMigration(opts, preflight.EventUUID); err != nil {
		return exitcode.UnexpectedErrorf("finalizing flat migration after durable event commit: %v", err)
	}
	result.PendingFinalize = false
	return writeLegacyOutput(cmd, format, result)
}
