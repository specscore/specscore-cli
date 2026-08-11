package cli

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/specscore/specscore-cli/pkg/lesson"
)

func TestPreparedLessonEvent_DisabledUncertaintyStillRequiresArtifactRecovery(t *testing.T) {
	p := &preparedLessonEvent{disabled: true}
	recovery, err := p.ResolveMutationFailure("writing Lesson", &lesson.MutationError{
		Outcome: lesson.MutationUncertain,
		Err:     errors.New("injected post-publication boundary failure"),
	})
	if !recovery {
		t.Fatal("disabled event delivery must not hide uncertain artifact recovery")
	}
	if !strings.Contains(err.Error(), "recovery required: artifact state is uncertain") || !strings.Contains(err.Error(), "event delivery was disabled") {
		t.Fatalf("missing no-ledger recovery instruction: %v", err)
	}
	if strings.Contains(err.Error(), "prepared event ") {
		t.Fatalf("disabled delivery must not invent an event UUID: %v", err)
	}
}

func TestPreparedLessonEventBoundaries(t *testing.T) {
	root := setupSpecRoot(t)
	_, err := prepareLessonEventWithID(root, "lesson.test", "x", map[string]any{"bad": make(chan int)}, time.Time{}, "id")
	requireCLIError(t, err)
	requireCLISuccess(t, os.WriteFile(filepath.Join(root, "specscore.yaml"), []byte("events: ["), 0o644))
	_, err = prepareLessonEventWithID(root, "lesson.test", "x", nil, time.Time{}, "id")
	requireCLIError(t, err)
	requireCLISuccess(t, os.WriteFile(filepath.Join(root, "specscore.yaml"), []byte("events: {}\n"), 0o644))
	disabled, err := prepareLessonEventWithID(root, "lesson.test", "x", nil, time.Time{}, "id")
	if err != nil || !disabled.disabled || disabled.Abort() != nil {
		t.Fatalf("disabled event = %#v, err=%v", disabled, err)
	}
	_, err = disabled.Commit(context.Background())
	requireCLISuccess(t, err)
	var none *preparedLessonEvent
	if none.Abort() != nil {
		t.Fatal("nil abort failed")
	}
	_, err = none.Commit(context.Background())
	requireCLISuccess(t, err)
	if recovery, got := none.ResolveMutationFailure("test", &lesson.MutationError{Outcome: lesson.MutationCompensated, Err: errors.New("rolled back")}); recovery || got == nil {
		t.Fatalf("nil compensated resolution = (%t, %v)", recovery, got)
	}

	requireCLISuccess(t, os.WriteFile(filepath.Join(root, "specscore.yaml"), []byte("events:\n  subscribers:\n    - type: noop\n"), 0o644))
	_, err = prepareLessonEventWithID(root, "lesson.test", "x", nil, time.Time{}, "")
	requireCLIError(t, err)
	p, err := prepareLessonEvent(root, "lesson.test", "x", map[string]any{}, time.Time{})
	requireCLISuccess(t, err)
	_, err = p.Commit(context.Background())
	requireCLISuccess(t, err)
	if recovery, err := p.ResolveMutationFailure("test", &lesson.MutationError{Outcome: lesson.MutationCompensated, Err: errors.New("rolled back")}); !recovery || !strings.Contains(err.Error(), "aborting fully-compensated event failed") {
		t.Fatalf("committed resolution = (%t, %v)", recovery, err)
	}
}
