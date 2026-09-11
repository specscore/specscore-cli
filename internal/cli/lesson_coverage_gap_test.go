package cli

// This file closes out statement-coverage gaps in lesson.go left by the
// existing lesson test suites, discovered while landing an unrelated
// feature (Plan repository routing): the repo's coverage gate
// (scripts/coverage-gate.sh) measures the whole module in one profile, so
// any package below 100% blocks every PR regardless of whether that PR
// touches it.

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/specscore/specscore-cli/pkg/event"
	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/projectdef"
)

// TestLessonNew_MissingConfigReturnsGuidance covers runLessonNewWithDeps's
// os.IsNotExist(unwrapPathError(...)) branch: a spec/features/ tree with no
// specscore.yaml at all satisfies resolveSpecRoot (it only needs
// spec/features/), but deps.readConfig then fails with a not-exist error.
func TestLessonNew_MissingConfigReturnsGuidance(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "spec", "features"), 0o755); err != nil {
		t.Fatal(err)
	}
	withCwd(t, root)

	cmd := lessonNewCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"project": root})
	err := runLessonNewWithDeps(cmd, []string{"missing-config"}, defaultLessonCLIDeps())
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := exitCodeOf(err); got != exitcode.InvalidState {
		t.Fatalf("exit code = %d, want InvalidState", got)
	}
	if !strings.Contains(err.Error(), "specscore init") {
		t.Errorf("error should point to `specscore init`; got: %v", err)
	}
}

// TestLessonNew_CommitErrorPropagates covers runLessonNewWithDeps's
// transaction.Commit error-handling branch: prepareIntentEvent is
// overridden to hand back a preparedLessonEvent whose event UUID was never
// actually durably prepared, so outbox.Commit(uuid) fails deterministically
// when the real mutation later calls transaction.Commit.
func TestLessonNew_CommitErrorPropagates(t *testing.T) {
	root := setupSpecRoot(t)
	if err := projectdef.WriteSpecConfig(root, lessonTestConfig()); err != nil {
		t.Fatal(err)
	}

	cmd := lessonNewCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"project": root, "classification": "process"})
	deps := defaultLessonCLIDeps()
	deps.prepareIntentEvent = func(root, name, slug string, payload, intentFacts map[string]any, at time.Time) (*preparedLessonEvent, error) {
		return &preparedLessonEvent{outbox: event.NewOutbox(root), event: event.Event{UUID: "never-actually-prepared"}}, nil
	}

	err := runLessonNewWithDeps(cmd, []string{"commit-edge"}, deps)
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "event publication is pending") {
		t.Errorf("err = %v, want mention of pending event publication", err)
	}
}

// TestUnwrapPathError_MultiLevelUnwrap covers unwrapPathError's loop
// continuing past a single level: existing tests only exercise zero or one
// levels of wrapping, never two.
func TestUnwrapPathError_MultiLevelUnwrap(t *testing.T) {
	inner := os.ErrNotExist
	middle := fmt.Errorf("middle: %w", inner)
	outer := fmt.Errorf("outer: %w", middle)
	if got := unwrapPathError(outer); !errors.Is(got, inner) || got != inner {
		t.Fatalf("unwrapPathError(outer) = %v, want %v", got, inner)
	}
}
