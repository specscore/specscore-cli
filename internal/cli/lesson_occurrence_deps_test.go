package cli

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/specscore/specscore-cli/pkg/projectdef"
)

func occurrenceDepsFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := projectdef.WriteSpecConfig(root, lessonTestConfig()); err != nil {
		t.Fatal(err)
	}
	lessonsDir := filepath.Join(root, "spec", "lessons")
	path := filepath.Join(lessonsDir, "rule", "README.md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	body, err := lesson.ScaffoldCanonical(lesson.ScaffoldOptions{Slug: "rule", Owner: "tester"}, []string{"process"})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, body, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(filepath.Dir(path), "occurrences"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lessonsDir, "README.md"), []byte(lessonsIndexContent(lessonTestConfig())), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func runOccurrenceWithDeps(t *testing.T, root string, deps lessonOccurrenceDeps) error {
	t.Helper()
	cmd := lessonOccurrenceAddCommand()
	if err := cmd.Flags().Set("project", root); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("capture-context", "false"); err != nil {
		t.Fatal(err)
	}
	return runLessonOccurrenceAddWithDeps(cmd, []string{"rule"}, deps)
}

func TestLessonOccurrenceWithDepsIsolatesFaultsInParallel(t *testing.T) {
	tests := []struct {
		name string
		run  func(*testing.T, string, lessonOccurrenceDeps) error
		want int
	}{
		{"index snapshot read fails", func(_ *testing.T, root string, deps lessonOccurrenceDeps) error {
			deps.readFile = func(string) ([]byte, error) { return nil, errors.New("index read failed") }
			return runOccurrenceWithDeps(t, root, deps)
		}, exitcode.Unexpected},
		{"index stat fails", func(_ *testing.T, root string, deps lessonOccurrenceDeps) error {
			deps.stat = func(string) (os.FileInfo, error) { return nil, errors.New("index stat failed") }
			return runOccurrenceWithDeps(t, root, deps)
		}, exitcode.Unexpected},
		{"event preparation fails", func(_ *testing.T, root string, deps lessonOccurrenceDeps) error {
			deps.prepareEvent = func(string, string, string, map[string]any, time.Time) (*preparedLessonEvent, error) {
				return nil, errors.New("event prepare failed")
			}
			return runOccurrenceWithDeps(t, root, deps)
		}, exitcode.Unexpected},
		{"prepublication add refusal", func(_ *testing.T, root string, deps lessonOccurrenceDeps) error {
			deps.prepareEvent = func(string, string, string, map[string]any, time.Time) (*preparedLessonEvent, error) {
				return &preparedLessonEvent{disabled: true}, nil
			}
			deps.addOccurrence = func(lesson.AddOccurrenceOptions) (lesson.Occurrence, error) {
				return lesson.Occurrence{}, &lesson.MutationError{Outcome: lesson.MutationPrePublication, Err: errors.New("occurrence rejected")}
			}
			return runOccurrenceWithDeps(t, root, deps)
		}, exitcode.InvalidArgs},
		{"index failure compensates published child", func(t *testing.T, root string, deps lessonOccurrenceDeps) error {
			deps.prepareEvent = func(string, string, string, map[string]any, time.Time) (*preparedLessonEvent, error) {
				return &preparedLessonEvent{disabled: true}, nil
			}
			deps.indexUpsert = func(string, *lesson.Lesson) error { return errors.New("index unavailable") }
			err := runOccurrenceWithDeps(t, root, deps)
			entries, readErr := os.ReadDir(filepath.Join(root, "spec", "lessons", "rule", "occurrences"))
			if readErr != nil || len(entries) != 0 {
				t.Fatalf("compensation entries=%v err=%v", entries, readErr)
			}
			return err
		}, exitcode.Unexpected},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.run(t, occurrenceDepsFixture(t), defaultLessonOccurrenceDeps())
			if got := exitCodeOfErr(err); got != tt.want {
				t.Fatalf("exit=%d want=%d err=%v", got, tt.want, err)
			}
		})
	}
}
