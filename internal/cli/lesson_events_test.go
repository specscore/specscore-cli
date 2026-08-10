package cli

import (
	"errors"
	"strings"
	"testing"

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
