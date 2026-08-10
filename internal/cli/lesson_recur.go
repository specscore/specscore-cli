package cli

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
	"github.com/spf13/cobra"
)

// lessonRecurCommand records that a lesson's gap manifested again — the
// strongest possible signal a lesson needs to graduate up the enforcement
// ladder. It does NOT change **Status:**; promoting the lesson stays a
// deliberate `change-status` call.
func lessonRecurCommand() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "recur <slug>",
		Short: "Record that a lesson's gap manifested again",
		Long: `For a canonical Lesson, appends exactly one immutable typed JSON
child under occurrences/, leaves the README byte-identical, and refreshes only its derived index row;
recurrence metadata is derived from valid children. The compatibility path for
a legacy flat Lesson increments **Recurred:** and appends its old prose entry.
Neither path changes **Status:**. Run
"specscore lesson change-status <slug> --to=<status>" separately to act on the
signal. A missing lesson exits 3.

A recurrence against a lesson already retired (Withdrawn or Superseded) is
evidence the retirement itself was wrong — it still exits 0 and records the
occurrence (the evidence is worth keeping), but prints a warning to stderr
rather than succeeding silently.

Docs: docs/agent-lessons.md#create-and-record`,
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE:          runLessonRecur,
	}
	cmd.Flags().String("note", "", "free-form note describing this occurrence")
	cmd.Flags().String("project", "", "project root (autodetected from current directory if omitted)")
	return cmd
}

// warnIfLessonRetired prints a stderr warning when l's status is terminal
// (Withdrawn or Superseded — recognized statuses with no legal outgoing
// arc). An unrecognized or missing status is not this verb's concern (L-002
// already governs status validity), so it is left silent.
func warnIfLessonRetired(w io.Writer, slug string, status string) {
	canonical, ok := lifecycle.ParseStatus(lifecycle.KindLesson, strings.TrimSpace(status))
	if !ok || len(lifecycle.LegalTargets(lifecycle.KindLesson, canonical)) > 0 {
		return
	}
	_, _ = fmt.Fprintf(w,
		"warning: %s is %s — recording a recurrence against a retired lesson suggests the retirement should be revisited (consider recording a fresh lesson referencing this one) rather than continuing to log against it\n",
		slug, string(canonical))
}

func runLessonRecur(cmd *cobra.Command, args []string) error {
	if len(args) == 0 {
		return exitcode.InvalidArgsError("missing required positional argument: <slug>")
	}
	if len(args) > 1 {
		return exitcode.InvalidArgsErrorf(
			"too many positional arguments: recur accepts exactly one <slug>, got %d", len(args))
	}
	slug := args[0]
	if err := lesson.ValidateSlug(slug); err != nil {
		return exitcode.InvalidArgsErrorf("invalid slug %q: %v", slug, err)
	}

	note, _ := cmd.Flags().GetString("note")
	projectFlag, _ := cmd.Flags().GetString("project")

	root, err := resolveSpecRoot(projectFlag)
	if err != nil {
		return err
	}

	path, err := lesson.ResolveLessonFile(filepath.Join(root, "spec", "lessons"), slug)
	if err != nil {
		return err
	}

	before, err := lessonParseFn(path)
	if err != nil {
		return exitcode.UnexpectedErrorf("parsing lesson %s: %v", slug, err)
	}
	warnIfLessonRetired(cmd.ErrOrStderr(), slug, before.Status)
	if before.Canonical {
		root := projectRootForOccurrence(projectFlag)
		indexSnapshot, indexErr := snapshotOccurrenceIndex(root)
		if indexErr != nil {
			return exitcode.UnexpectedErrorf("snapshotting lessons index: %v", indexErr)
		}
		id := uuid.NewString()
		now := time.Now().UTC()
		prepared, err := prepareLessonEvent(root, "lesson.occurrence-recorded", slug, map[string]any{"occurrence_id": id}, now)
		if err != nil {
			return exitcode.UnexpectedErrorf("preparing occurrence event: %v", err)
		}
		o, err := lessonAddOccurrenceFn(lesson.AddOccurrenceOptions{LessonPath: path, ID: id, Summary: note, Context: captureOccurrenceContext(root, path), Evidence: lesson.Evidence{Kind: "none"}, Now: now})
		if err != nil {
			if recovery, resolved := prepared.ResolveMutationFailure("recording occurrence", err); recovery {
				return exitcode.UnexpectedErrorf("%v", resolved)
			} else {
				return exitcode.UnexpectedErrorf("recording occurrence: %v", resolved)
			}
		}
		if err := lessonIndexUpsertFn(filepath.Join(root, "spec"), before); err != nil {
			failure := lesson.CompensatePublication(func() error {
				if removeErr := lesson.RemoveOccurrence(o.Path); removeErr != nil {
					return removeErr
				}
				return indexSnapshot.restore()
			}, err)
			if recovery, resolved := prepared.ResolveMutationFailure("upserting occurrence index row", failure); recovery {
				return exitcode.UnexpectedErrorf("%v", resolved)
			} else {
				return exitcode.UnexpectedErrorf("upserting occurrence index row: %v", resolved)
			}
		}
		items, err := lesson.DiscoverOccurrences(path)
		if err != nil {
			failure := lesson.CompensatePublication(func() error {
				if removeErr := lesson.RemoveOccurrence(o.Path); removeErr != nil {
					return removeErr
				}
				return indexSnapshot.restore()
			}, err)
			if recovery, resolved := prepared.ResolveMutationFailure("reading occurrences after publication", failure); recovery {
				return exitcode.UnexpectedErrorf("%v", resolved)
			} else {
				return exitcode.UnexpectedErrorf("reading occurrences: %v", resolved)
			}
		}
		result, commitErr := prepared.Commit(cmd.Context())
		if commitErr != nil {
			return exitcode.UnexpectedErrorf("occurrence recorded but event publication is pending for event %s: %v", prepared.event.UUID, commitErr)
		}
		for _, failure := range result.Failed {
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: subscriber %s remains pending: %v\n", failure.Name, failure.Err)
		}
		_ = items // occurrence identifier stays internal for historical output compatibility.
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: recurred %d\n", slug, len(items))
		return nil
	}

	if err := lesson.ValidateSafeContent("legacy recurrence note", note); err != nil {
		return exitcode.InvalidArgsErrorf("unsafe recurrence note: %v", err)
	}
	indexPath := filepath.Join(root, "spec", "lessons", "README.md")
	beforeBody, err := os.ReadFile(path)
	if err != nil {
		return exitcode.UnexpectedErrorf("snapshotting legacy Lesson: %v", err)
	}
	bodyInfo, err := os.Stat(path)
	if err != nil {
		return exitcode.UnexpectedErrorf("snapshotting legacy Lesson mode: %v", err)
	}
	beforeIndex, indexErr := os.ReadFile(indexPath)
	if indexErr != nil && !os.IsNotExist(indexErr) {
		return exitcode.UnexpectedErrorf("snapshotting lessons index: %v", indexErr)
	}
	var indexMode os.FileMode
	if indexErr == nil {
		indexInfo, statErr := os.Stat(indexPath)
		if statErr != nil {
			return exitcode.UnexpectedErrorf("snapshotting lessons index mode: %v", statErr)
		}
		indexMode = indexInfo.Mode().Perm()
	}
	prepared, err := prepareLessonEvent(root, "lesson.occurrence-recorded", slug, map[string]any{"kind": "legacy-recurrence"}, time.Now().UTC())
	if err != nil {
		return exitcode.UnexpectedErrorf("preparing recurrence event: %v", err)
	}
	count, err := lessonRecurFn(path, note)
	if err != nil {
		if recovery, resolved := prepared.ResolveMutationFailure("recording legacy recurrence", err); recovery {
			return exitcode.UnexpectedErrorf("%v", resolved)
		} else {
			return exitcode.UnexpectedErrorf("recording recurrence: %v", resolved)
		}
	}

	specSub := filepath.Join(root, "spec")
	updated, parseErr := lesson.Parse(path)
	if parseErr != nil {
		failure := lesson.CompensatePublication(func() error {
			return restoreLegacyRecurFiles(path, beforeBody, bodyInfo.Mode().Perm(), indexPath, beforeIndex, indexMode, indexErr == nil)
		}, parseErr)
		if recovery, resolved := prepared.ResolveMutationFailure("parsing updated legacy Lesson", failure); recovery {
			return exitcode.UnexpectedErrorf("%v", resolved)
		} else {
			return exitcode.UnexpectedErrorf("parsing updated legacy Lesson: %v", resolved)
		}
	}
	if err := lessonIndexUpsertFn(specSub, updated); err != nil {
		failure := lesson.CompensatePublication(func() error {
			return restoreLegacyRecurFiles(path, beforeBody, bodyInfo.Mode().Perm(), indexPath, beforeIndex, indexMode, indexErr == nil)
		}, err)
		if recovery, resolved := prepared.ResolveMutationFailure("upserting legacy Lesson index row", failure); recovery {
			return exitcode.UnexpectedErrorf("%v", resolved)
		} else {
			return exitcode.UnexpectedErrorf("upserting legacy Lesson index row: %v", resolved)
		}
	}
	result, commitErr := prepared.Commit(cmd.Context())
	if commitErr != nil {
		return exitcode.UnexpectedErrorf("recurrence recorded but event publication is pending for event %s: %v", prepared.event.UUID, commitErr)
	}
	for _, failure := range result.Failed {
		_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "warning: subscriber %s remains pending: %v\n", failure.Name, failure.Err)
	}

	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s: recurred %d\n", slug, count)
	return nil
}

func restoreLegacyRecurFiles(path string, body []byte, bodyMode os.FileMode, indexPath string, index []byte, indexMode os.FileMode, restoreIndex bool) error {
	if err := durableRestoreFile(path, body, bodyMode); err != nil {
		return err
	}
	if restoreIndex {
		return durableRestoreFile(indexPath, index, indexMode)
	}
	return nil
}

func durableRestoreFile(path string, data []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	if err := f.Chmod(mode); err != nil {
		_ = f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()
	return dir.Sync()
}

func durableRemovePath(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	defer func() { _ = dir.Close() }()
	return dir.Sync()
}
