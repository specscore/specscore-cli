package idea

// Coverage for the optional-transition-note branches in ChangeStatus: the
// note-write failure rollback, and the archived-path body restore inside the
// note-rollback wrapper when a post-move lint failure rolls back a
// note-carrying archive transition.

import (
	"errors"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

func TestChangeStatus_NoteWriteError(t *testing.T) {
	root := stageIdeaTree(t, "foo", "Approved")
	orig := appendNoteFn
	defer func() { appendNoteFn = orig }()
	appendNoteFn = func(string, string) ([]byte, bool, error) {
		return nil, false, errors.New("note write boom")
	}
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot:     root,
		Slug:         "foo",
		To:           lifecycle.IdeaArchived,
		Note:         "x",
		PostMutation: func() error { return nil },
	})
	if err == nil {
		t.Fatal("expected note-write error")
	}
	// Rolled back: file restored to the active path with its original status.
	if body := readIdea(t, root, "foo"); !strings.Contains(body, "**Status:** Approved") {
		t.Errorf("not rolled back:\n%s", body)
	}
}

func TestChangeStatus_NoteArchivedLintRollback(t *testing.T) {
	root := stageIdeaTree(t, "foo", "Approved")
	_, err := ChangeStatus(ChangeStatusOptions{
		SpecRoot:     root,
		Slug:         "foo",
		To:           lifecycle.IdeaArchived,
		Note:         "shipped via close",
		PostMutation: failingLint(errors.New("lint boom")),
	})
	if err == nil {
		t.Fatal("expected lint-failure rollback")
	}
	// File restored to active path with original status; note undone.
	body := readIdea(t, root, "foo")
	if !strings.Contains(body, "**Status:** Approved") {
		t.Errorf("status not rolled back:\n%s", body)
	}
	if strings.Contains(body, "## Resolution") {
		t.Errorf("resolution note not rolled back:\n%s", body)
	}
}
