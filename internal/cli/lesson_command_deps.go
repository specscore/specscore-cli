package cli

import (
	"io/fs"
	"os"
	"time"

	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/specscore/specscore-cli/pkg/lint"
)

// Private, per-call dependency values keep CLI fault tests deterministic
// without shared mutable package hooks. Public command handlers use defaults.
type lessonCommandDeps struct {
	readFile                 func(string) ([]byte, error)
	writeFile                func(string, []byte, os.FileMode) error
	remove                   func(string) error
	removeAll                func(string) error
	preflightFlatMigration   func(lesson.FlatMigrationOptions) (lesson.FlatMigrationPreflight, error)
	migrateFlat              func(lesson.FlatMigrationOptions) (lesson.FlatMigrationResult, error)
	prepareLessonEventWithID func(string, string, string, map[string]any, time.Time, string) (*preparedLessonEvent, error)
	finalizeFlatMigration    func(lesson.FlatMigrationOptions, string) error
}

func defaultLessonCommandDeps() lessonCommandDeps {
	return lessonCommandDeps{os.ReadFile, os.WriteFile, os.Remove, os.RemoveAll, lesson.PreflightFlatMigration, lesson.MigrateFlat, prepareLessonEventWithID, lesson.FinalizeFlatMigration}
}

type lessonOccurrenceDeps struct {
	addOccurrence func(lesson.AddOccurrenceOptions) (lesson.Occurrence, error)
	indexUpsert   func(string, *lesson.Lesson) error
	prepareEvent  func(string, string, string, map[string]any, time.Time) (*preparedLessonEvent, error)
	readFile      func(string) ([]byte, error)
	stat          func(string) (fs.FileInfo, error)
}

func defaultLessonOccurrenceDeps() lessonOccurrenceDeps {
	return lessonOccurrenceDeps{lesson.AddOccurrence, lint.UpsertLessonIndexRow, prepareLessonEvent, os.ReadFile, os.Stat}
}
