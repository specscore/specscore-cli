package cli

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/specscore/specscore-cli/pkg/lint"
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
	resuming := preflight.PendingTransaction
	flatPath := filepath.Join(lessonsDir, slug+".md")
	markerPath := filepath.Join(lessonsDir, ".flat-migration-"+slug+".json")
	indexPath := filepath.Join(lessonsDir, "README.md")
	legacyImportDir := filepath.Join(lessonsDir, ".legacy-import")
	var before flatMigrationRollback
	if !resuming {
		before.flat, err = snapshotRollbackFile(flatPath, deps.fs)
		if err != nil {
			return exitcode.UnexpectedErrorf("reading flat Lesson: %v", err)
		}
		before.index, err = snapshotOptionalRollbackFile(indexPath, deps.fs)
		if err != nil {
			return exitcode.UnexpectedErrorf("reading lessons index: %v", err)
		}
		before.legacyImportDirExisted, err = pathExistsAsDirectory(legacyImportDir, deps.fs.stat)
		if err != nil {
			return exitcode.UnexpectedErrorf("inspecting legacy manifest directory: %v", err)
		}
		before.markerPath = markerPath
		before.legacyImportDir = legacyImportDir
	}

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
	if err := afterFlatMigrationPhase(deps, "artifact-publication"); err != nil {
		return err
	}
	if !resuming {
		before.marker, err = snapshotRollbackFile(markerPath, deps.fs)
		if err == nil {
			before.canonical, err = snapshotRollbackTree(filepath.Dir(result.CanonicalPath), deps.fs)
		}
		if err == nil {
			before.manifest, err = snapshotRollbackFile(result.ManifestPath, deps.fs)
		}
		if err == nil {
			err = validateFlatMigrationOwnership(lessonsDir, result, before.marker.data, before.canonical, before.manifest)
		}
		if err != nil {
			_, resolved := prepared.ResolveMutationFailure("snapshotting published flat migration", &lesson.MutationError{Outcome: lesson.MutationUncertain, Err: err})
			return exitcode.UnexpectedErrorf("%v", resolved)
		}
		before.expectedIndex = before.index
	}

	rollbackFresh := func() error {
		return before.restore(flatPath, indexPath, deps)
	}
	failAfterMutation := func(message string, cause error) error {
		if resuming {
			// A resumed transaction already crossed artifact publication; no
			// downstream adapter error can prove it safe to abort the prepared event.
			_, resolved := prepared.ResolveMutationFailure(message, &lesson.MutationError{Outcome: lesson.MutationUncertain, Err: cause})
			return exitcode.UnexpectedErrorf("%v", resolved)
		}
		failure := lesson.CompensatePublication(rollbackFresh, cause)
		if recovery, resolved := prepared.ResolveMutationFailure(message, failure); recovery {
			return exitcode.UnexpectedErrorf("%v", resolved)
		} else {
			return exitcode.UnexpectedErrorf(message+": %v", resolved)
		}
	}

	parsed, err := deps.parse(result.CanonicalPath)
	if err != nil {
		return failAfterMutation("parsing canonical migrated Lesson", err)
	}
	if err := deps.indexUpsert(filepath.Join(root, "spec"), parsed); err != nil {
		return failAfterMutation("upserting migrated Lesson index row", err)
	}
	if !resuming {
		before.expectedIndex, err = snapshotOptionalRollbackFile(indexPath, deps.fs)
		if err != nil {
			return failAfterMutation("snapshotting migrated Lesson index row", err)
		}
	}
	violations, err := deps.lint(lint.Options{SpecRoot: filepath.Join(root, "spec")})
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

func removePathIfExists(path string) error {
	return removePathIfExistsWith(path, os.Remove)
}

func removePathIfExistsWith(path string, remove func(string) error) error {
	err := remove(path)
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

type rollbackFile struct {
	path    string
	data    []byte
	mode    os.FileMode
	existed bool
}

type rollbackTreeEntry struct {
	path string
	mode os.FileMode
	dir  bool
	data []byte
}

type flatMigrationRollback struct {
	flat                   rollbackFile
	index                  rollbackFile
	expectedIndex          rollbackFile
	manifest               rollbackFile
	marker                 rollbackFile
	canonical              []rollbackTreeEntry
	markerPath             string
	legacyImportDir        string
	legacyImportDirExisted bool
}

func snapshotRollbackFile(path string, fs lessonFileOps) (rollbackFile, error) {
	info, err := fs.stat(path)
	if err != nil {
		return rollbackFile{}, err
	}
	b, err := fs.read(path)
	if err != nil {
		return rollbackFile{}, err
	}
	return rollbackFile{path: path, data: b, mode: info.Mode().Perm(), existed: true}, nil
}

func snapshotOptionalRollbackFile(path string, fs lessonFileOps) (rollbackFile, error) {
	snapshot, err := snapshotRollbackFile(path, fs)
	if os.IsNotExist(err) {
		return rollbackFile{path: path}, nil
	}
	return snapshot, err
}

func pathExistsAsDirectory(path string, stat func(string) (os.FileInfo, error)) (bool, error) {
	info, err := stat(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if !info.IsDir() {
		return false, fmt.Errorf("%s is not a directory", path)
	}
	return true, nil
}

func snapshotRollbackTree(root string, fs lessonFileOps) ([]rollbackTreeEntry, error) {
	var entries []rollbackTreeEntry
	var visit func(string) error
	visit = func(path string) error {
		info, err := fs.stat(path)
		if err != nil {
			return err
		}
		entry := rollbackTreeEntry{path: path, mode: info.Mode().Perm(), dir: info.IsDir()}
		if !entry.dir {
			entry.data, err = fs.read(path)
			if err != nil {
				return err
			}
		}
		entries = append(entries, entry)
		if !entry.dir {
			return nil
		}
		children, err := fs.readDir(path)
		if err != nil {
			return err
		}
		for _, child := range children {
			if err := visit(filepath.Join(path, child.Name())); err != nil {
				return err
			}
		}
		return nil
	}
	if err := visit(root); err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].path < entries[j].path })
	return entries, nil
}

func verifyRollbackFile(expected rollbackFile, fs lessonFileOps) error {
	actual, err := snapshotOptionalRollbackFile(expected.path, fs)
	if err != nil {
		return err
	}
	if actual.existed != expected.existed || actual.mode != expected.mode || !bytes.Equal(actual.data, expected.data) {
		return fmt.Errorf("rollback ownership conflict at %s", expected.path)
	}
	return nil
}

func validateFlatMigrationOwnership(lessonsDir string, result lesson.FlatMigrationResult, markerBytes []byte, canonical []rollbackTreeEntry, manifest rollbackFile) error {
	var marker struct {
		Files []struct {
			Path   string `json:"path"`
			SHA256 string `json:"sha256"`
		} `json:"files"`
	}
	if err := json.Unmarshal(markerBytes, &marker); err != nil {
		return fmt.Errorf("decoding migration ownership marker: %w", err)
	}
	expected := make(map[string]string, len(marker.Files))
	for _, file := range marker.Files {
		clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(file.Path)))
		_, duplicate := expected[clean]
		if clean != file.Path || filepath.IsAbs(filepath.FromSlash(clean)) || strings.HasPrefix(clean, "../") || duplicate {
			return fmt.Errorf("migration ownership marker has an unsafe or duplicate path")
		}
		expected[clean] = file.SHA256
	}
	checkFile := func(path string, data []byte) error {
		// Published paths were produced beneath lessonsDir by MigrateFlat, so
		// filepath.Rel cannot cross a volume boundary here.
		rel, _ := filepath.Rel(lessonsDir, path)
		rel = filepath.ToSlash(rel)
		want, ok := expected[rel]
		actual := fmt.Sprintf("%x", sha256.Sum256(data))
		if !ok || want != actual {
			return fmt.Errorf("published migration path is outside the marker or changed: %s", rel)
		}
		delete(expected, rel)
		return nil
	}
	canonicalRoot := filepath.Dir(result.CanonicalPath)
	for _, entry := range canonical {
		if entry.dir {
			if entry.path != canonicalRoot && entry.path != filepath.Join(canonicalRoot, "occurrences") {
				return fmt.Errorf("published migration has an unexpected directory: %s", entry.path)
			}
			continue
		}
		if err := checkFile(entry.path, entry.data); err != nil {
			return err
		}
	}
	if err := checkFile(manifest.path, manifest.data); err != nil {
		return err
	}
	if len(expected) != 0 {
		return fmt.Errorf("migration marker contains unpublished paths")
	}
	return nil
}

func (s flatMigrationRollback) restore(flatPath, indexPath string, deps lessonCLIDeps) error {
	canonical, err := snapshotRollbackTree(s.canonical[0].path, deps.fs)
	if err != nil {
		return fmt.Errorf("reading canonical migration tree before rollback: %w", err)
	}
	if !reflect.DeepEqual(canonical, s.canonical) {
		return fmt.Errorf("canonical migration tree changed before rollback")
	}
	for _, expected := range []rollbackFile{s.manifest, s.marker, s.expectedIndex} {
		if err := verifyRollbackFile(expected, deps.fs); err != nil {
			return err
		}
	}
	if _, err := deps.fs.stat(flatPath); !os.IsNotExist(err) {
		if err == nil {
			return fmt.Errorf("flat Lesson reappeared before rollback")
		}
		return err
	}
	// Restore the durable retry source and index before deleting any published
	// artifact. If a later exact removal fails, the marker can replay missing
	// paths from the original committed source instead of stranding a partial
	// canonical-only tree.
	if err := durableRestoreFileWithOps(flatPath, s.flat.data, s.flat.mode, deps.durable); err != nil {
		return err
	}
	if s.index.existed {
		if err := durableRestoreFileWithOps(indexPath, s.index.data, s.index.mode, deps.durable); err != nil {
			return err
		}
	} else if err := durableRemovePathWithOps(indexPath, deps.durable); err != nil {
		return err
	}
	for i := len(s.canonical) - 1; i >= 0; i-- {
		if err := durableRemovePathWithOps(s.canonical[i].path, deps.durable); err != nil {
			return err
		}
	}
	if err := durableRemovePathWithOps(s.manifest.path, deps.durable); err != nil {
		return err
	}
	if !s.legacyImportDirExisted {
		if err := durableRemovePathWithOps(s.legacyImportDir, deps.durable); err != nil {
			return err
		}
	}
	return durableRemovePathWithOps(s.markerPath, deps.durable)
}
