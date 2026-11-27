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
	"github.com/specscore/specscore-cli/pkg/rule"
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

// AC: writers-refuse-conflicted-files — `lesson change-status` rewrites the
// Lessons index, so over a conflicted index it exits 1 naming the file before
// any write or event and leaves the whole tree byte-identical. Its --dry-run
// runs the same transaction in a sandbox and so reports the same refusal.
func TestLessonChangeStatus_RefusesConflictedIndexBeforeAnyWrite(t *testing.T) {
	for _, args := range [][]string{
		{"lesson", "change-status", "kinder-fake", "--to=stated"},
		{"lesson", "change-status", "kinder-fake", "--to=stated", "--dry-run"},
	} {
		t.Run(strings.Join(args[2:], " "), func(t *testing.T) {
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

// conflictedRulesProject creates rule fresh-rule on a clean project, then
// injects a one-sided conflict block after its row in the Rules index.
func conflictedRulesProject(t *testing.T) string {
	t.Helper()
	root := stageLesson(t, "kinder-fake", "Recorded")
	if _, _, err := runRoot(t, "rule", "new", "fresh-rule", "--project", root); err != nil {
		t.Fatalf("rule new on a clean tree: %v", err)
	}
	path := rule.IndexPath(rule.RulesDir(root))
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.SplitAfter(string(raw), "\n")
	var b strings.Builder
	injected := false
	for _, line := range lines {
		b.WriteString(line)
		if !injected && strings.Contains(line, "fresh-rule") {
			b.WriteString("<<<<<<< HEAD\n| [other-rule](other-rule/README.md) | Stated | x | — | — | — | — |\n=======\n>>>>>>> topic\n")
			injected = true
		}
	}
	if !injected {
		t.Fatalf("rules index has no fresh-rule row:\n%s", raw)
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

// AC: writers-refuse-conflicted-files — the Rules index is the only copy of
// an inline rule, so every rule writer verb refuses a conflicted index before
// any write, byte-identically.
func TestRuleWriterVerbs_RefuseConflictedIndexBeforeAnyWrite(t *testing.T) {
	for _, args := range [][]string{
		{"rule", "new", "second-rule"},
		{"rule", "update", "fresh-rule", "--title", "Fresh"},
		{"rule", "delete", "fresh-rule"},
		{"rule", "expand", "fresh-rule"},
		{"rule", "promote", "promoted-rule", "--from-lesson", "kinder-fake"},
	} {
		t.Run(args[1], func(t *testing.T) {
			root := conflictedRulesProject(t)
			before := treeDigest(t, root)
			_, _, err := runRoot(t, append(args, "--project", root)...)
			if got := exitCodeOf(err); got != exitcode.Conflict {
				t.Fatalf("exit = %d, want %d; err=%v", got, exitcode.Conflict, err)
			}
			if !strings.Contains(err.Error(), "spec/rules/README.md") {
				t.Fatalf("err = %v, want the refusal naming spec/rules/README.md", err)
			}
			if !reflect.DeepEqual(treeDigest(t, root), before) {
				t.Fatal("the tree changed before the refusal")
			}
		})
	}
}

// AC: writers-refuse-conflicted-files — reading stays allowed: `rule list`
// and `rule show` work over a conflicted Rules index exactly as before.
func TestRuleReadVerbs_WorkOverConflictedIndex(t *testing.T) {
	root := conflictedRulesProject(t)
	before := treeDigest(t, root)
	for _, args := range [][]string{{"rule", "list"}, {"rule", "show", "fresh-rule"}} {
		out, _, err := runRoot(t, append(args, "--project", root)...)
		if err != nil || !strings.Contains(out, "fresh-rule") {
			t.Fatalf("%v: out=%q err=%v, want a normal read", args, out, err)
		}
	}
	if !reflect.DeepEqual(treeDigest(t, root), before) {
		t.Fatal("a read-only verb changed the tree")
	}
}

func TestRefuseConflictedTables_ReadErrorIsUnexpected(t *testing.T) {
	root := t.TempDir()
	err := refuseConflictedTables(root, root) // a directory cannot be read as a file
	if got := exitCodeOf(err); got != exitcode.Unexpected || !strings.Contains(err.Error(), "merge-conflict markers") {
		t.Fatalf("exit = %d err = %v, want Unexpected read failure", got, err)
	}
	if fixRefusal("spec", errors.New("other")) != nil || fixRefusal("spec", nil) != nil {
		t.Fatal("fixRefusal must ignore errors that are not *lint.FixRefusedError")
	}
}

// AC: merge-conflict-markers-fix-refused-byte-identical — `rule lint --fix`
// maps a refused pass onto the shared exit-1 refusal.
func TestRuleLintFix_RefusesConflictedTree(t *testing.T) {
	root := stageLesson(t, "kinder-fake", "Recorded")
	oneSidedLessonConflict(t, root)
	before := treeDigest(t, root)
	_, _, err := runRoot(t, "rule", "lint", "--fix", "--project", root)
	if got := exitCodeOf(err); got != exitcode.Conflict || !strings.Contains(err.Error(), "spec/lessons/README.md") {
		t.Fatalf("exit = %d err = %v, want the exit-1 refusal naming the index", got, err)
	}
	if !reflect.DeepEqual(treeDigest(t, root), before) {
		t.Fatal("a refused fix changed the tree")
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
