package cli

import (
	"os"
	"time"

	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/specscore/specscore-cli/pkg/lint"
	"github.com/specscore/specscore-cli/pkg/projectdef"
)

type lessonFileOps struct {
	stat     func(string) (os.FileInfo, error)
	read     func(string) ([]byte, error)
	readDir  func(string) ([]os.DirEntry, error)
	write    func(string, []byte, os.FileMode) error
	mkdirAll func(string, os.FileMode) error
}

// lessonCLIDeps is an immutable, per-invocation dependency value. It keeps
// fault injection local to one test and command instead of changing package
// state that concurrent commands can observe.
type lessonCLIDeps struct {
	fs                  lessonFileOps
	readConfig          func(string) (projectdef.SpecConfig, error)
	parse               func(string) (*lesson.Lesson, error)
	addOccurrence       func(lesson.AddOccurrenceOptions) (lesson.Occurrence, error)
	discoverOccurrences func(string) ([]lesson.Occurrence, error)
	removeOccurrence    func(string) error
	addRelation         func(string, string, string, string) error
	listRelations       func(string, string) ([]lesson.Relation, error)
	recur               func(string, string) (int, error)
	inventoryLegacy     func(string) (lesson.LegacyInventory, error)
	applyLegacy         func(string, []string, lesson.LegacyInventory, lesson.LegacyMapping) (lesson.LegacyApplyResult, error)
	indexUpsert         func(string, *lesson.Lesson) error
	reconcileIndex      func(string, *lesson.Lesson) error
	lint                func(lint.Options) ([]lint.Violation, error)
	prepareEvent        func(string, string, string, map[string]any, time.Time) (*preparedLessonEvent, error)
	prepareEventWithID  func(string, string, string, map[string]any, time.Time, string) (*preparedLessonEvent, error)
	finalizeFlat        func(lesson.FlatMigrationOptions, string) error
	preflightFlat       func(lesson.FlatMigrationOptions) (lesson.FlatMigrationPreflight, error)
	migrateFlat         func(lesson.FlatMigrationOptions) (lesson.FlatMigrationResult, error)
	preflightLegacy     func(string, []string, lesson.LegacyInventory, lesson.LegacyMapping) error
	changeStatus        func(lesson.ChangeStatusOptions) (lesson.ChangeStatusResult, error)
	afterFlatPhase      func(string) error
	durable             durableFileOps
}

func defaultLessonCLIDeps() lessonCLIDeps {
	return lessonCLIDeps{
		fs:                  lessonFileOps{os.Stat, os.ReadFile, os.ReadDir, os.WriteFile, os.MkdirAll},
		readConfig:          projectdef.ReadSpecConfig,
		parse:               lesson.Parse,
		addOccurrence:       lesson.AddOccurrence,
		discoverOccurrences: lesson.DiscoverOccurrences,
		removeOccurrence:    lesson.RemoveOccurrence,
		addRelation:         lesson.AddRelation,
		listRelations:       lesson.ListRelations,
		recur:               lesson.Recur,
		inventoryLegacy:     lesson.InventoryLegacy,
		applyLegacy:         lesson.ApplyLegacy,
		indexUpsert:         lint.UpsertLessonIndexRow,
		reconcileIndex:      lint.UpsertLessonIndexRow,
		lint:                lint.Lint,
		prepareEvent:        prepareLessonEvent,
		prepareEventWithID:  prepareLessonEventWithID,
		finalizeFlat:        lesson.FinalizeFlatMigration,
		preflightFlat:       lesson.PreflightFlatMigration,
		migrateFlat:         lesson.MigrateFlat,
		preflightLegacy:     lesson.PreflightLegacyApply,
		changeStatus:        lesson.ChangeStatus,
		afterFlatPhase:      func(string) error { return nil },
		durable:             defaultDurableFileOps(),
	}
}
