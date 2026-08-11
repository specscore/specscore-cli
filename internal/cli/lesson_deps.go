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
	mkdirAll func(string, os.FileMode) error
}

// lessonCLIDeps is an immutable, per-invocation dependency value. It keeps
// fault injection local to one test and command instead of changing package
// state that concurrent commands can observe.
type lessonCLIDeps struct {
	fs                          lessonFileOps
	readConfig                  func(string) (projectdef.SpecConfig, error)
	scaffoldCanonical           func(lesson.ScaffoldOptions, []string) ([]byte, error)
	parse                       func(string) (*lesson.Lesson, error)
	addOccurrence               func(lesson.AddOccurrenceOptions) (lesson.Occurrence, error)
	discoverOccurrences         func(string) ([]lesson.Occurrence, error)
	addRelationWithPostMutation func(string, string, string, string, lesson.RelationPostMutationHook) error
	listRelations               func(string, string) ([]lesson.Relation, error)
	recurWithPostMutation       func(string, string, func(int) error) (int, error)
	inventoryLegacy             func(string) (lesson.LegacyInventory, error)
	applyLegacy                 func(string, []string, lesson.LegacyInventory, lesson.LegacyMapping) (lesson.LegacyApplyResult, error)
	indexUpsert                 func(string, *lesson.Lesson) error
	lint                        func(lint.Options) ([]lint.Violation, error)
	prepareEvent                func(string, string, string, map[string]any, time.Time) (*preparedLessonEvent, error)
	prepareEventWithID          func(string, string, string, map[string]any, time.Time, string) (*preparedLessonEvent, error)
	finalizeFlat                func(lesson.FlatMigrationOptions, string) error
	preflightFlat               func(lesson.FlatMigrationOptions) (lesson.FlatMigrationPreflight, error)
	migrateFlat                 func(lesson.FlatMigrationOptions) (lesson.FlatMigrationResult, error)
	preflightLegacy             func(string, []string, lesson.LegacyInventory, lesson.LegacyMapping) error
	changeStatus                func(lesson.ChangeStatusOptions) (lesson.ChangeStatusResult, error)
	afterFlatPhase              func(string) error
	publishExclusive            func(string, []byte, os.FileMode) error
	withMutationLock            func(string, string, func() error) error
	rewriteAtomic               func(string, []byte) error
	durable                     durableFileOps
}

func defaultLessonCLIDeps() lessonCLIDeps {
	return lessonCLIDeps{
		fs:                          lessonFileOps{stat: os.Stat, read: os.ReadFile, readDir: os.ReadDir, mkdirAll: os.MkdirAll},
		readConfig:                  projectdef.ReadSpecConfig,
		scaffoldCanonical:           lesson.ScaffoldCanonical,
		parse:                       lesson.Parse,
		addOccurrence:               lesson.AddOccurrence,
		discoverOccurrences:         lesson.DiscoverOccurrences,
		addRelationWithPostMutation: lesson.AddRelationWithPostMutation,
		listRelations:               lesson.ListRelations,
		recurWithPostMutation:       lesson.RecurWithPostMutation,
		inventoryLegacy:             lesson.InventoryLegacy,
		applyLegacy:                 lesson.ApplyLegacy,
		indexUpsert:                 lint.UpsertLessonIndexRow,
		lint:                        lint.Lint,
		prepareEvent:                prepareLessonEvent,
		prepareEventWithID:          prepareLessonEventWithID,
		finalizeFlat:                lesson.FinalizeFlatMigration,
		preflightFlat:               lesson.PreflightFlatMigration,
		migrateFlat:                 lesson.MigrateFlat,
		preflightLegacy:             lesson.PreflightLegacyApply,
		changeStatus:                lesson.ChangeStatus,
		afterFlatPhase:              func(string) error { return nil },
		publishExclusive:            publishFileExclusive,
		withMutationLock:            lesson.WithMutationLock,
		rewriteAtomic:               lesson.RewriteFileAtomic,
		durable:                     defaultDurableFileOps(),
	}
}
