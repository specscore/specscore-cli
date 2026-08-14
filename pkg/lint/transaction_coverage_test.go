package lint

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

func TestWriteLintLessonMissingSnapshotFailsBeforeWrite(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "lessons", "README.md")
	if err := writeLintFile(root, path, []byte("before"), []byte("after"), 0o644); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("err=%v, want not exist", err)
	}
}

func TestPlanFixesRejectMalformedLockedSnapshot(t *testing.T) {
	root := t.TempDir()
	plansDir := filepath.Join(root, "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(plansDir, "oversized.md")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), (1<<20)+1), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		fix  func(string) error
	}{
		{"source", newPlanRulesChecker().fixNoSourceLines},
		{"task-status", fixLegacyTaskStatuses},
		{"plan-status", fixP007},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.fix(root); err == nil {
				t.Fatal("malformed locked snapshot should be rejected")
			}
		})
	}
}

func TestPlanStatusTransformsFenceStaleCoordinates(t *testing.T) {
	if _, err := rewriteTaskStatusLinesBytes([]byte("not status\n"), map[int]string{1: "complete"}); !errors.Is(err, lifecycle.ErrConcurrentMutation) {
		t.Fatalf("task status err=%v", err)
	}
	if _, err := rewritePlanStatusLineBytes([]byte("# Plan: P\n"), 1, "Implemented"); !errors.Is(err, lifecycle.ErrConcurrentMutation) {
		t.Fatalf("plan status err=%v", err)
	}
}
