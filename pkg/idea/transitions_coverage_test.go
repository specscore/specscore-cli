package idea

// Coverage for the optional-transition-note branch in ChangeStatus: the
// note-write failure rollback path.

import (
	"errors"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

func TestChangeStatus_NoteWriteError(t *testing.T) {
	root := stageIdeaTree(t, "foo", "In Review")
	orig := appendNoteFn
	defer func() { appendNoteFn = orig }()
	appendNoteFn = func(string, string) ([]byte, bool, error) {
		return nil, false, errors.New("note write boom")
	}
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot:     root,
		Slug:         "foo",
		To:           lifecycle.IdeaRejected,
		Note:         "x",
		PostMutation: func() error { return nil },
	})
	if err == nil {
		t.Fatal("expected note-write error")
	}
	// Rolled back: status restored to its original value.
	if body := readIdea(t, root, "foo"); !strings.Contains(body, "**Status:** In Review") {
		t.Errorf("not rolled back:\n%s", body)
	}
}
