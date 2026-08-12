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
			anchors: []string{"func ChangeStatus(", "planTransformArtifactFn(path", "lifecycle.CommittedError(path"},
		},
		"pkg/plan/reconcile.go": {
			class:   "existing-plan-reconcile-transaction",
			anchors: []string{"func Reconcile(", "planTransformArtifactFn(flatPath", "func reconcileBytes("},
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

// TestTaskPlanTransactionContractTextIsConsistent prevents the stale split
// contract that previously described the same Task/Plan command as both
// lock-free and locked, and as both rollback-on-callback and retained-commit.
func TestTaskPlanTransactionContractTextIsConsistent(t *testing.T) {
	root := taskPlanContractRoot(t)
	surfaces := map[string][]string{
		"spec/features/cli/lifecycle-transitions/README.md": {
			"shared fail-fast local artifact transaction",
			"retains the committed artifact as an explicit recovery-required state",
			"Historical compensating verbs are not silently reclassified",
		},
		"spec/features/cli/task/change-status/README.md": {
			"one fail-fast local artifact transaction",
			"Board Task mutation has no derived index callback",
			"never the local transaction conflict",
		},
		"spec/features/cli/plan/change-status/README.md": {
			"one committed artifact transaction",
			"committed Plan retained for recovery",
		},
		"spec/features/cli/plan/reconcile/README.md": {
			"committed reconciliation transaction",
			"artifact retained for explicit recovery",
		},
		"internal/cli/task.go": {
			"one fail-fast local artifact",
			"does not invoke a derived lint/index callback",
		},
		"internal/cli/task_amend.go": {
			"one fail-fast artifact transaction",
		},
		"internal/cli/plan.go": {
			"committed as one atomic durable transaction",
			"committed/recovery-required error",
		},
		"internal/cli/plan_reconcile.go": {
			"one fail-fast Plan artifact lock",
			"committed reconciliation remains visible",
		},
		"ai/skills/specscore-task/SKILL.md": {
			"one fail-fast artifact transaction",
		},
		"spec/capabilities/specscore.json": {
			"one fail-fast artifact transaction",
			"TestTransformArtifact_ForeignEditBeforeRenameIsDetected",
		},
		"pkg/lifecycle/rewrite.go": {
			"Transaction-profile writers MUST instead parse",
			"explicit legacy compensating status-line writer",
		},
	}
	for rel, required := range surfaces {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatalf("%s: %v", rel, err)
		}
		text := string(body)
		for _, phrase := range required {
			if !strings.Contains(text, phrase) {
				t.Errorf("%s lost transaction-contract phrase %q", rel, phrase)
			}
		}
		for _, stale := range []string{
			"Codes `1` (Conflict) and `5–9` are NOT used",
			"Transition succeeded and index synced",
			"with **no** claim/release, locking",
			"Invoked post-mutation for index/rollup sync",
			"post-mutation-failure-restores-bytes",
			"PostMutation rolled back",
		} {
			if strings.Contains(text, stale) {
				t.Errorf("%s retains stale transaction contract %q", rel, stale)
			}
		}
	}
	for _, rel := range []string{"pkg/plan/transitions.go", "pkg/plan/reconcile.go"} {
		body, err := os.ReadFile(filepath.Join(root, rel))
		if err != nil {
			t.Fatal(err)
		}
		for _, seam := range []string{"appendNoteFn", "setSupersededByFn"} {
			if strings.Contains(string(body), seam) {
				t.Errorf("%s retains dead split-writer seam %q", rel, seam)
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
