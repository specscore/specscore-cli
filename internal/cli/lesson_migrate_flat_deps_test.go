package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/specscore/specscore-cli/pkg/event"
	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/specscore/specscore-cli/pkg/projectdef"
)

func migrationDepsFixture(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := projectdef.WriteSpecConfig(root, lessonTestConfig()); err != nil {
		t.Fatal(err)
	}
	lessons := filepath.Join(root, "spec", "lessons")
	if err := os.MkdirAll(lessons, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lessons, "README.md"), []byte(lessonsIndexContent(lessonTestConfig())), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lessons, "rule.md"), []byte("# Lesson: rule\n\n**Status:** Recorded\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}
func runMigrationWithDeps(t *testing.T, root string, deps lessonCommandDeps) error {
	t.Helper()
	cmd := lessonMigrateFlatCommand()
	if err := cmd.Flags().Set("classification", "process"); err != nil {
		t.Fatal(err)
	}
	if err := cmd.Flags().Set("project", root); err != nil {
		t.Fatal(err)
	}
	return runLessonMigrateFlatWithDeps(cmd, []string{"rule"}, deps)
}
func migrationDepsPreflight() lesson.FlatMigrationPreflight {
	return lesson.FlatMigrationPreflight{EventUUID: uuid.NewString(), Source: lesson.LegacySourceRef{CommittedAt: "2026-08-10T12:00:00Z", Path: "spec/lessons/rule.md", Revision: strings.Repeat("a", 40), SHA256: strings.Repeat("b", 64)}}
}

func TestLessonMigrateFlatWithDepsIsolatesFaultsInParallel(t *testing.T) {
	tests := []struct {
		name string
		run  func(*testing.T, string, lessonCommandDeps) error
		want int
	}{
		{"already migrated verification fails", func(t *testing.T, root string, deps lessonCommandDeps) error {
			deps.preflightFlatMigration = func(lesson.FlatMigrationOptions) (lesson.FlatMigrationPreflight, error) {
				return lesson.FlatMigrationPreflight{AlreadyMigrated: true}, nil
			}
			deps.migrateFlat = func(lesson.FlatMigrationOptions) (lesson.FlatMigrationResult, error) {
				return lesson.FlatMigrationResult{}, errors.New("verification failed")
			}
			return runMigrationWithDeps(t, root, deps)
		}, exitcode.InvalidState},
		{"non UTC timestamp", func(t *testing.T, root string, deps lessonCommandDeps) error {
			p := migrationDepsPreflight()
			p.Source.CommittedAt = "2026-08-10T13:00:00+01:00"
			deps.preflightFlatMigration = func(lesson.FlatMigrationOptions) (lesson.FlatMigrationPreflight, error) { return p, nil }
			return runMigrationWithDeps(t, root, deps)
		}, exitcode.InvalidState},
		{"snapshot and event failures", func(t *testing.T, root string, deps lessonCommandDeps) error {
			deps.preflightFlatMigration = func(lesson.FlatMigrationOptions) (lesson.FlatMigrationPreflight, error) {
				return migrationDepsPreflight(), nil
			}
			deps.readFile = func(string) ([]byte, error) { return nil, errors.New("snapshot failed") }
			return runMigrationWithDeps(t, root, deps)
		}, exitcode.Unexpected},
		{"uncertain migration preserves recovery", func(t *testing.T, root string, deps lessonCommandDeps) error {
			deps.preflightFlatMigration = func(lesson.FlatMigrationOptions) (lesson.FlatMigrationPreflight, error) {
				return migrationDepsPreflight(), nil
			}
			deps.prepareLessonEventWithID = func(string, string, string, map[string]any, time.Time, string) (*preparedLessonEvent, error) {
				return &preparedLessonEvent{outbox: event.Outbox{Root: filepath.Join(root, "missing-outbox")}, event: event.Event{UUID: uuid.NewString()}}, nil
			}
			deps.migrateFlat = func(lesson.FlatMigrationOptions) (lesson.FlatMigrationResult, error) {
				return lesson.FlatMigrationResult{}, errors.New("uncertain")
			}
			return runMigrationWithDeps(t, root, deps)
		}, exitcode.Unexpected},
	}
	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			err := tt.run(t, migrationDepsFixture(t), defaultLessonCommandDeps())
			if got := exitCodeOfErr(err); got != tt.want {
				t.Fatalf("exit=%d want=%d err=%v", got, tt.want, err)
			}
		})
	}
}
