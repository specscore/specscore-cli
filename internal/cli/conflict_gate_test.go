package cli

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/specscore/specscore-cli/pkg/lint"
)

// oneSidedLessonConflict injects a one-sided conflict block (one branch added
// a row, the other added nothing) into the Lessons index of a staged tree.
func oneSidedLessonConflict(t *testing.T, root string) {
	t.Helper()
	indexPath := filepath.Join(root, "spec", "lessons", "README.md")
	raw, err := os.ReadFile(indexPath)
	if err != nil {
		t.Fatal(err)
	}
	const sep = "|---|---|---|---:|---|\n"
	if !strings.Contains(string(raw), sep) {
		t.Fatalf("staged index lacks the canonical separator:\n%s", raw)
	}
	conflicted := strings.Replace(string(raw), sep, sep+"<<<<<<< HEAD\n| [other](other/README.md) | Recorded | — | 0 | — |\n=======\n>>>>>>> topic\n", 1)
	if err := os.WriteFile(indexPath, []byte(conflicted), 0o644); err != nil {
		t.Fatal(err)
	}
}

// AC: writers-refuse-conflicted-files — every producer verb and index writer
// runs the shared conflict-marker precondition before it mutates any file or
// prepares any event: over a conflicted tree it exits 1 naming the file and
// leaves the whole tree (Lesson file, index, events, everything) byte-identical.
func TestProducerVerbs_RefuseConflictedTreeBeforeAnyWrite(t *testing.T) {
	verbs := [][]string{
		{"lesson", "new", "fresh-lesson", "--owner", "tester"},
		{"lesson", "change-status", "kinder-fake", "--to=stated"},
		{"lesson", "change-status", "kinder-fake", "--to=stated", "--dry-run"},
		{"lesson", "relation", "add", "kinder-fake", "other", "--type", "related", "--confirm", lesson.RelationToken("kinder-fake", "related", "other")},
		{"lesson", "occurrence", "add", "kinder-fake"},
		{"lesson", "recur", "kinder-fake"},
		{"lesson", "migrate-flat", "kinder-fake", "--classification", "process"},
		{"lesson", "import-legacy", "--source", "LESSONS-LEARNED.md", "--dry-run"},
		{"feature", "new", "--title=Fresh Feature"},
		{"idea", "new", "fresh-idea"},
		{"idea", "promote", "fresh-idea"},
		{"decision", "new", "fresh-decision"},
		{"issue", "new", "fresh-issue"},
		{"issue", "change-status", "fresh-issue", "--to=resolved"},
		{"rule", "new", "fresh-rule"},
		{"rule", "update", "fresh-rule", "--title", "Fresh"},
		{"rule", "delete", "fresh-rule"},
		{"rule", "promote", "fresh-rule", "--from-lesson", "kinder-fake"},
	}
	for _, args := range verbs {
		t.Run(strings.Join(args[:2], "-"), func(t *testing.T) {
			root := stageLesson(t, "kinder-fake", "Recorded")
			oneSidedLessonConflict(t, root)
			before := treeDigest(t, root)
			_, _, err := runRoot(t, append(args, "--project", root)...)
			if got := exitCodeOf(err); got != exitcode.Conflict {
				t.Fatalf("exit = %d, want %d; err=%v", got, exitcode.Conflict, err)
			}
			if !strings.Contains(err.Error(), "refusing to modify the spec tree") || !strings.Contains(err.Error(), "spec/lessons/README.md") {
				t.Fatalf("err = %v, want the shared refusal naming spec/lessons/README.md", err)
			}
			if !reflect.DeepEqual(treeDigest(t, root), before) {
				t.Fatal("the tree changed before the refusal")
			}
		})
	}
}

func TestRefuseConflictedProject_ScanErrorIsUnexpected(t *testing.T) {
	prev := conflictMarkersFn
	t.Cleanup(func() { conflictMarkersFn = prev })
	conflictMarkersFn = func(lint.Options) ([]lint.Violation, error) { return nil, errors.New("scan failed") }
	err := refuseConflictedProject(t.TempDir())
	if got := exitCodeOf(err); got != exitcode.Unexpected || !strings.Contains(err.Error(), "scan failed") {
		t.Fatalf("exit = %d err = %v, want Unexpected scan failure", got, err)
	}
	if fixRefusal("spec", errors.New("other")) != nil || fixRefusal("spec", nil) != nil {
		t.Fatal("fixRefusal must ignore errors that are not *lint.FixRefusedError")
	}
}

// AC: conflicted-tree-refused-unchanged (cli/spec/migrate) and
// merge-conflict-markers-fix-refused-byte-identical — `spec migrate` and
// `rule lint --fix` map a refused pass onto the shared exit-1 refusal.
func TestSpecMigrateAndRuleLintFix_RefuseConflictedTree(t *testing.T) {
	root := stageLesson(t, "kinder-fake", "Recorded")
	oneSidedLessonConflict(t, root)
	before := treeDigest(t, root)
	for _, args := range [][]string{
		{"spec", "migrate", "--project", root},
		{"rule", "lint", "--fix", "--project", root},
	} {
		_, _, err := runRoot(t, args...)
		if got := exitCodeOf(err); got != exitcode.Conflict || !strings.Contains(err.Error(), "spec/lessons/README.md") {
			t.Fatalf("%v: exit = %d err = %v, want the exit-1 refusal naming the index", args, got, err)
		}
	}
	if !reflect.DeepEqual(treeDigest(t, root), before) {
		t.Fatal("a refused migration or fix changed the tree")
	}
}

// Defense in depth: a marker finding surfacing in a producer's post-write
// lint (a conflict written concurrently, after the precondition) is always
// fatal, even though it names no file the per-Lesson filter owns.
func TestOwnedLessonMutationViolation_ConflictMarkerAlwaysFatal(t *testing.T) {
	if !ownedLessonMutationViolation(lint.Violation{File: "features/README.md", Rule: lint.MergeConflictMarkersRule}, []string{"kinder-fake"}) {
		t.Fatal("a merge-conflict-markers finding must be treated as owned")
	}
	if ownedLessonMutationViolation(lint.Violation{File: "features/README.md", Rule: "index-entries"}, []string{"kinder-fake"}) {
		t.Fatal("an unrelated finding elsewhere must not be owned")
	}
}

// AC: enforced-to-recorded-requires-mechanism-control — demoting an Enforced
// Lesson whose free-text Control names no mechanism is refused before any
// write (exit 4) instead of failing the post-write L-010 lint with exit 10.
func TestLessonChangeStatus_EnforcedToRecordedFreeTextControlRefusedBeforeWrite_CLI(t *testing.T) {
	root := stageLesson(t, "kinder-fake", "Enforced")
	path := filepath.Join(root, "spec", "lessons", "kinder-fake", "README.md")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, line := range strings.SplitAfter(string(raw), "\n") {
		if strings.HasPrefix(line, "**Control:**") {
			line = "**Control:** reviewers watch for it\n"
		}
		b.WriteString(line)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	before := treeDigest(t, root)
	_, _, err = runLesson(t, "change-status", "kinder-fake", "--to=recorded", "--note", "claimed control does not bind")
	if got := exitCodeOfErr(err); got != exitcode.InvalidState {
		t.Fatalf("exit = %d, want %d; err=%v", got, exitcode.InvalidState, err)
	}
	if !strings.Contains(err.Error(), "**Control:**") || !strings.Contains(err.Error(), "none-yet") {
		t.Fatalf("error must explain the Control requirement: %v", err)
	}
	if !reflect.DeepEqual(treeDigest(t, root), before) {
		t.Fatal("the refused demotion changed the tree")
	}

	deps := defaultLessonCLIDeps()
	deps.parse = func(string) (*lesson.Lesson, error) { return nil, errors.New("parse failed") }
	cmd := lessonChangeStatusCommand()
	for flag, value := range map[string]string{"to": "recorded", "note": "x"} {
		if err := cmd.Flags().Set(flag, value); err != nil {
			t.Fatal(err)
		}
	}
	err = runLessonChangeStatusWithDeps(cmd, []string{"kinder-fake"}, deps)
	if got := exitCodeOfErr(err); got != exitcode.Unexpected || !strings.Contains(err.Error(), "parse failed") {
		t.Fatalf("exit = %d err = %v, want Unexpected parse failure", got, err)
	}
}

// bypassConflictGate disables the producer precondition for a test that
// drives a later unreadable-file branch the gate's scan would report first.
func bypassConflictGate(t *testing.T) {
	t.Helper()
	prev := conflictMarkersFn
	conflictMarkersFn = func(lint.Options) ([]lint.Violation, error) { return nil, nil }
	t.Cleanup(func() { conflictMarkersFn = prev })
}
