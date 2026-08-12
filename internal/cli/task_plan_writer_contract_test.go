package cli

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

type artifactWriterContract struct {
	class   string
	anchors []string
}

// TestTaskPlanBodyWriterInventoryHasNoUnclassifiedPath is the executable
// writer inventory for human-authored Task/Plan bodies. Existing bodies have
// one transaction primitive; new bodies are exclusive publishers. The Plan
// index is deliberately absent: it is a derived projection written only after
// the body lock is released.
func TestTaskPlanBodyWriterInventoryHasNoUnclassifiedPath(t *testing.T) {
	inventory := map[string]artifactWriterContract{
		"internal/cli/task.go": {
			class: "existing-task-and-plan-body-transactions-plus-prepared-task-publisher",
			anchors: []string{
				"func rewriteBoardTask(", "taskTransformArtifactFn(path",
				"func runTaskChangeStatusPlanInline(", "taskTransformArtifactFn(planPath",
				"func runTaskAmendProvenanceBoard(", "func runTaskAmendProvenancePlanInline(",
				"func runTaskNew(", "taskWithArtifactTxFn(boardPath", "taskNewPublishExclusiveFn(markerPath",
				"taskNewPublishExclusiveFn(taskFilePath", "taskNewCommitBoardFn(tx", "taskNewRemoveMarkerFn(markerPath",
			},
		},
		"internal/cli/task_amend.go": {
			class:   "existing-task-and-inline-plan-annotation-transactions",
			anchors: []string{"func amendPlanTask(", "func amendTaskArtifact(", "taskTransformArtifactFn(path"},
		},
		"internal/cli/plan.go": {
			class:   "exclusive-new-plan-publisher-or-force-transaction",
			anchors: []string{"func runPlanNew(", "publishFileExclusive(target", "lifecycle.TransformArtifact(target"},
		},
		"pkg/plan/transitions.go": {
			class:   "existing-plan-change-status-transaction",
			anchors: []string{"func ChangeStatus(", "lifecycle.TransformArtifact(path", "lifecycle.CommittedError(path"},
		},
		"pkg/plan/reconcile.go": {
			class:   "existing-plan-reconcile-transaction",
			anchors: []string{"func Reconcile(", "lifecycle.TransformArtifact(flatPath", "func reconcileBytes("},
		},
		"pkg/lint/legacy_status_fix.go": {
			class:   "legacy-plan-body-status-transaction",
			anchors: []string{"lifecycle.TransformArtifact(path"},
		},
		"pkg/lint/plan_rules.go": {
			class: "plan-body-fixer-transactions",
			anchors: []string{
				"lifecycle.TransformArtifact(planPath", "lifecycle.TransformArtifact(path",
				"func insertSourceNoneBytes(", "func rewriteTaskStatusLinesBytes(", "func rewritePlanStatusLineBytes(",
			},
		},
	}

	root := taskPlanContractRoot(t)
	for rel, contract := range inventory {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		text := string(body)
		for _, anchor := range contract.anchors {
			if !strings.Contains(text, anchor) {
				t.Errorf("%s (%s) lost anchor %q", rel, contract.class, anchor)
			}
		}
		for _, forbidden := range []string{
			"os.WriteFile(", "osWriteFileFn(", "WithArtifactMutationLock(",
			"TransformArtifactUnderLock(", "CompareAndSwap(", "RewriteUnderLock(",
			"AppendResolutionNoteUnderLock(", "SetSupersededByUnderLock(",
		} {
			if strings.Contains(text, forbidden) {
				t.Errorf("%s (%s) has unclassified body writer %q", rel, contract.class, forbidden)
			}
		}
	}
}

func taskPlanContractRoot(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Clean(filepath.Join(filepath.Dir(file), "..", ".."))
}
