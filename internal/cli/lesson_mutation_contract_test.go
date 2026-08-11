package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"testing"
)

type lessonMutationActionContract struct {
	action           string
	class            string
	lockOrder        string
	artifactMutation bool
	rowReconcile     bool
	boundedLint      bool
	durableFence     bool
	eventResolution  bool
	anchors          map[string][]string
}

const lessonMutationLockOrder = "Lesson locks (lexical) -> relation-project lock (when used) -> shared index lock"

func TestLessonMutationActionMatrixMatchesCallGraph(t *testing.T) {
	contracts := []lessonMutationActionContract{
		coordinatedLessonAction("lesson new", map[string][]string{"internal/cli/lesson.go": {"mutationErr := deps.withMutationLock", "deps.publishExclusive", "deps.indexUpsert", "deps.lint", "transaction.Fence", "transaction.Commit"}}),
		coordinatedLessonAction("lesson new --force", map[string][]string{"internal/cli/lesson.go": {"mutationErr := deps.withMutationLock", "deps.rewriteAtomic", "deps.indexUpsert", "deps.lint", "transaction.Fence", "transaction.Commit"}}),
		coordinatedLessonAction("lesson import-legacy --apply", map[string][]string{"internal/cli/lesson_import_legacy.go": {"transactionErr := deps.withMutationLocks", "deps.applyLegacy", "reconcileLockedLessons", "prepared.Commit"}}),
		coordinatedLessonAction("lesson migrate-flat", map[string][]string{"internal/cli/lesson_migrate_flat.go": {"return deps.withMutationLock", "deps.migrateFlat", "reconcileLockedLessons", "prepared.Commit", "deps.finalizeFlat"}}),
		coordinatedLessonAction("lesson occurrence add", map[string][]string{"internal/cli/lesson_occurrence.go": {"lockErr := deps.withMutationLock", "deps.addOccurrence", "reconcileLockedLessons", "prepared.Commit"}}),
		coordinatedLessonAction("lesson recur (canonical)", map[string][]string{"internal/cli/lesson_recur.go": {"publishCanonicalOccurrence"}, "internal/cli/lesson_occurrence.go": {"lockErr := deps.withMutationLock", "deps.addOccurrence", "reconcileLockedLessons", "prepared.Commit"}}),
		coordinatedLessonAction("lesson recur (legacy)", map[string][]string{"internal/cli/lesson_recur.go": {"deps.recurWithPostMutation", "reconcileLockedLessons", "prepared.Commit"}, "pkg/lesson/recur.go": {"withLessonMutationLock"}}),
		coordinatedLessonAction("lesson relation add", map[string][]string{"internal/cli/lesson_relation.go": {"deps.addRelationWithPostMutation", "deps.indexUpsert", "deps.lint", "newLessonMutationCoordinator", "prepared.Commit"}, "pkg/lesson/relation.go": {"withLessonMutationLocks"}}),
		coordinatedLessonAction("lesson change-status / enforcement", map[string][]string{"internal/cli/lesson.go": {"prepareLessonPostMutationWithDeps", "deps.changeStatus", "deps.indexUpsert", "deps.lint", "newLessonMutationCoordinator", "transaction.Commit"}, "pkg/lesson/transitions.go": {"withLessonMutationLock"}}),
		{
			action: "lesson occurrence remove", class: "explicit-library-deletion",
			anchors: map[string][]string{"pkg/lesson/occurrence.go": {"func RemoveOccurrence("}},
		},
		{
			action: "event recovery / replay", class: "event-recovery", eventResolution: true,
			anchors: map[string][]string{"internal/cli/event.go": {"eventReplayCommand", "eventReconcileCommand", ".ReplayFrom(", ".Commit(", ".Abort("}},
		},
	}

	wantActions := []string{
		"event recovery / replay", "lesson change-status / enforcement", "lesson import-legacy --apply",
		"lesson migrate-flat", "lesson new", "lesson new --force", "lesson occurrence add",
		"lesson occurrence remove", "lesson recur (canonical)", "lesson recur (legacy)", "lesson relation add",
	}
	gotActions := make([]string, 0, len(contracts))
	for _, contract := range contracts {
		gotActions = append(gotActions, contract.action)
		switch contract.class {
		case "coordinated":
			if contract.lockOrder != lessonMutationLockOrder || !contract.artifactMutation || !contract.rowReconcile || !contract.boundedLint || !contract.durableFence || !contract.eventResolution {
				t.Errorf("%s does not declare the complete locked mutation protocol: %#v", contract.action, contract)
			}
		case "explicit-library-deletion":
			if contract.artifactMutation || contract.rowReconcile || contract.eventResolution {
				t.Errorf("%s must remain caller-owned and unavailable as an implicit CLI compensator", contract.action)
			}
		case "event-recovery":
			if contract.artifactMutation || contract.rowReconcile || contract.boundedLint || contract.durableFence || !contract.eventResolution {
				t.Errorf("%s must resolve only durable event state", contract.action)
			}
		default:
			t.Errorf("%s has unknown mutation class %q", contract.action, contract.class)
		}
		assertLessonMutationAnchors(t, contract)
	}
	sort.Strings(gotActions)
	if strings.Join(gotActions, "\n") != strings.Join(wantActions, "\n") {
		t.Fatalf("Lesson mutation action inventory drifted:\ngot:\n%s\nwant:\n%s", strings.Join(gotActions, "\n"), strings.Join(wantActions, "\n"))
	}

	for _, path := range [][]string{{"new"}, {"import-legacy"}, {"migrate-flat"}, {"occurrence", "add"}, {"recur"}, {"relation", "add"}, {"change-status"}} {
		if command, _, err := lessonCommand().Find(path); err != nil || command == nil {
			t.Errorf("Lesson mutation command %q missing from CLI inventory: %v", strings.Join(path, " "), err)
		}
	}
	if command, _, err := eventCommand().Find([]string{"replay"}); err != nil || command == nil {
		t.Errorf("event replay missing from CLI inventory: %v", err)
	}
	if command, _, err := eventCommand().Find([]string{"reconcile"}); err != nil || command == nil {
		t.Errorf("event reconcile missing from CLI inventory: %v", err)
	}
	for _, command := range lessonOccurrenceCommand().Commands() {
		if command.Name() == "remove" {
			t.Fatal("occurrence removal unexpectedly became a CLI mutation without the full coordinator contract")
		}
	}
}

func coordinatedLessonAction(action string, anchors map[string][]string) lessonMutationActionContract {
	return lessonMutationActionContract{
		action: action, class: "coordinated", lockOrder: lessonMutationLockOrder,
		artifactMutation: true, rowReconcile: true, boundedLint: true, durableFence: true, eventResolution: true,
		anchors: anchors,
	}
}

func assertLessonMutationAnchors(t *testing.T, contract lessonMutationActionContract) {
	t.Helper()
	_, current, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot resolve test source path")
	}
	repoRoot := filepath.Clean(filepath.Join(filepath.Dir(current), "..", ".."))
	for rel, anchors := range contract.anchors {
		body, err := os.ReadFile(filepath.Join(repoRoot, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatalf("%s source %s: %v", contract.action, rel, err)
		}
		for _, anchor := range anchors {
			if !strings.Contains(string(body), anchor) {
				t.Errorf("%s lost call-graph anchor %q in %s", contract.action, anchor, rel)
			}
		}
	}
}
