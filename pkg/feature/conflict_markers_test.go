package feature

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/specscore/specscore-cli/pkg/conflictmarker"
	"github.com/specscore/specscore-cli/pkg/exitcode"
)

const conflictedFeatureIndex = "# Features\n\n| Feature | Status | Kind | Description |\n|---|---|---|---|\n<<<<<<< HEAD\n| [a](a/README.md) | Draft | — | A. |\n=======\n>>>>>>> topic\n"

// AC: writers-refuse-conflicted-index — the feature index writers refuse a
// conflicted index rather than rewrite around its markers, and feature.New
// refuses before creating the feature directory.
func TestFeatureIndexWriters_RefuseConflictedIndex(t *testing.T) {
	dir := t.TempDir()
	index := filepath.Join(dir, "README.md")
	if err := os.WriteFile(index, []byte(conflictedFeatureIndex), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := UpdateFeatureIndex(index, "b", "Draft", ""); !errors.Is(err, conflictmarker.ErrConflicted) {
		t.Fatalf("UpdateFeatureIndex err = %v", err)
	}
	if _, err := UpdateParentContents(index, "b", ""); !errors.Is(err, conflictmarker.ErrConflicted) {
		t.Fatalf("UpdateParentContents err = %v", err)
	}
	if b, _ := os.ReadFile(index); string(b) != conflictedFeatureIndex {
		t.Fatalf("index rewritten:\n%s", b)
	}

	_, err := New(dir, NewOptions{Title: "Fresh Feature", Status: "draft"})
	var coded *exitcode.Error
	if !errors.As(err, &coded) || coded.ExitCode() != exitcode.Conflict || !errors.Is(err, conflictmarker.ErrConflicted) {
		t.Fatalf("New err = %v, want an exit-1 conflict refusal", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "fresh-feature")); !os.IsNotExist(statErr) {
		t.Fatalf("feature directory created despite the refusal: %v", statErr)
	}
}

func TestNew_SubFeatureRefusesConflictedParent(t *testing.T) {
	featDir := setupTestFeatures(t, map[string]string{"parent": "# Parent\n\n## Contents\n\n<<<<<<< HEAD\n| [x](x/README.md) | X. |\n=======\n>>>>>>> t\n"})
	if _, err := New(featDir, NewOptions{Title: "Child", Parent: "parent", Status: "draft"}); !errors.Is(err, conflictmarker.ErrConflicted) {
		t.Fatalf("err = %v, want a conflict refusal naming the parent README", err)
	}
}
