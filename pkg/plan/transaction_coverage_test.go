package plan

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

func TestParseBytesRejectsOversizedLine(t *testing.T) {
	_, err := ParseBytes("auth.md", bytes.Repeat([]byte("x"), (1<<20)+1))
	if err == nil || !strings.Contains(err.Error(), "token too long") {
		t.Fatalf("err=%v", err)
	}
}

func TestChangeStatusMalformedSnapshotAndTransactionContention(t *testing.T) {
	t.Run("malformed-oversized-snapshot", func(t *testing.T) {
		root, path := stageFlatPlan(t, "auth", "Draft")
		before := bytes.Repeat([]byte("x"), (1<<20)+1)
		if err := os.WriteFile(path, before, 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := ChangeStatus(ChangeStatusOptions{SpecRoot: root, Slug: "auth", To: lifecycle.PlanInReview, PostMutation: okHook})
		if codeOf(t, err) != exitcode.Unexpected || !strings.Contains(err.Error(), "token too long") {
			t.Fatalf("err=%v", err)
		}
		if after, _ := os.ReadFile(path); !bytes.Equal(after, before) {
			t.Fatal("malformed Plan was mutated")
		}
	})
	t.Run("contention", func(t *testing.T) {
		root, _ := stageFlatPlan(t, "auth", "Draft")
		_, err := ChangeStatus(ChangeStatusOptions{
			SpecRoot: root, Slug: "auth", To: lifecycle.PlanInReview, PostMutation: okHook,
			transformArtifact: func(string, func([]byte) ([]byte, error)) error { return lifecycle.ErrConcurrentMutation },
		})
		if codeOf(t, err) != exitcode.Conflict || !errors.Is(err, lifecycle.ErrConcurrentMutation) {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestReconcileSnapshotValidationAndMalformedSnapshotAreWriteFree(t *testing.T) {
	t.Run("snapshot-validator", func(t *testing.T) {
		root, path := stageReconcilePlan(t, "auth", "Draft", "planning")
		before, _ := os.ReadFile(path)
		boom := errors.New("coordination changed")
		called := false
		_, err := Reconcile(ReconcileOptions{
			SpecRoot: root, Slug: "auth", Note: "x", PostMutation: okHook,
			ValidateSnapshot: func(gotPath string, got []byte) error {
				called = true
				if gotPath != path || !bytes.Equal(got, before) {
					t.Fatalf("validator did not see exact locked snapshot")
				}
				return boom
			},
		})
		if !called || !errors.Is(err, boom) || codeOf(t, err) != exitcode.Unexpected {
			t.Fatalf("called=%v err=%v", called, err)
		}
		if after, _ := os.ReadFile(path); !bytes.Equal(after, before) {
			t.Fatal("rejected coordination snapshot was mutated")
		}
	})
	t.Run("malformed-oversized-snapshot", func(t *testing.T) {
		root, path := stageReconcilePlan(t, "auth", "Draft", "planning")
		before := bytes.Repeat([]byte("x"), (1<<20)+1)
		if err := os.WriteFile(path, before, 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := Reconcile(ReconcileOptions{SpecRoot: root, Slug: "auth", Note: "x", PostMutation: okHook})
		if codeOf(t, err) != exitcode.Unexpected || !strings.Contains(err.Error(), "token too long") {
			t.Fatalf("err=%v", err)
		}
		if after, _ := os.ReadFile(path); !bytes.Equal(after, before) {
			t.Fatal("malformed reconciliation Plan was mutated")
		}
	})
}
