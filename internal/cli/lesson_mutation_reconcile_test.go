package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/specscore/specscore-cli/pkg/lint"
)

func TestReconcileLockedLessonsResidualBoundaries(t *testing.T) {
	root := t.TempDir()
	requireCLISuccess(t, os.MkdirAll(filepath.Join(root, "spec", "lessons"), 0o755))
	if err := reconcileLockedLessons(root, []string{"missing"}, nil, defaultLessonCLIDeps()); err == nil || !strings.Contains(err.Error(), "resolving Lesson") {
		t.Fatalf("missing locked Lesson = %v", err)
	}

	path := filepath.Join(root, "spec", "lessons", "canonical", "README.md")
	requireCLISuccess(t, os.MkdirAll(filepath.Dir(path), 0o755))
	requireCLISuccess(t, os.WriteFile(path, []byte("fixture\n"), 0o644))
	deps := defaultLessonCLIDeps()
	deps.parse = func(string) (*lesson.Lesson, error) {
		return &lesson.Lesson{Slug: "canonical", Path: path, Canonical: true}, nil
	}
	deps.indexUpsert = func(string, *lesson.Lesson) error { return nil }
	deps.fs.stat = func(candidate string) (os.FileInfo, error) {
		if strings.HasSuffix(candidate, "occurrences") {
			return nil, errors.New("occurrences stat")
		}
		return os.Stat(candidate)
	}
	if err := reconcileLockedLessons(root, []string{"canonical"}, nil, deps); err == nil || !strings.Contains(err.Error(), "occurrences stat") {
		t.Fatalf("occurrences stat boundary = %v", err)
	}

	if !ownedLessonMutationViolation(lint.Violation{File: "lessons/README.md", Rule: "L-003", Message: "missing canonical"}, []string{"canonical"}) {
		t.Fatal("owned index violation was not attributed to the locked Lesson")
	}
}
