package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/event"
	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/specscore/specscore-cli/pkg/projectdef"
)

func TestLessonImportLegacy_ConfigurationAndMappingPreflightBeforePreparedEvent(t *testing.T) {
	for name, fixture := range map[string]struct {
		config         projectdef.SpecConfig
		classification string
	}{
		"missing vocabulary":          {config: projectdef.SpecConfig{}, classification: "process"},
		"unconfigured classification": {config: lessonTestConfig(), classification: "validation"},
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			requireCLISuccess(t, projectdef.WriteSpecConfig(root, fixture.config))
			lessonsDir := filepath.Join(root, "spec", "lessons")
			requireCLISuccess(t, os.MkdirAll(lessonsDir, 0o755))
			source := filepath.Join(root, "LESSONS-LEARNED.md")
			requireCLISuccess(t, os.WriteFile(source, []byte("## L1 — use a reviewed rule\n\n**Status:** Enforced\n\nHistorical detail.\n"), 0o644))
			for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"config", "user.name", "SpecScore Test"}, {"config", "user.email", "test@example.invalid"}, {"remote", "add", "origin", "https://github.com/example/spec.git"}, {"add", "."}, {"commit", "-q", "-m", "fixture"}} {
				runGitForFlatMigration(t, root, args...)
			}
			inventory, err := lesson.InventoryLegacy(source)
			requireCLISuccess(t, err)
			mapping := lesson.LegacyMapping{Source: inventory.Source, Entries: []lesson.LegacyMappingEntry{{
				Key: "L1#1", Action: "new", Slug: "reviewed-rule", Status: "Recorded",
				Lesson: "Apply the reviewed deterministic process control.", ProcessGap: "The workflow lacked a deterministic check.",
				Classifications: []string{fixture.classification},
			}}}
			mappingBytes, err := json.Marshal(mapping)
			requireCLISuccess(t, err)
			mappingPath := filepath.Join(root, "mapping.json")
			requireCLISuccess(t, os.WriteFile(mappingPath, mappingBytes, 0o644))
			before := treeDigestForCLI(t, lessonsDir)
			_, _, err = runLesson(t, "import-legacy", "--source", source, "--apply", "--mapping", mappingPath, "--project", root)
			requireCLIError(t, err)
			if after := treeDigestForCLI(t, lessonsDir); !bytes.Equal(after, before) {
				t.Fatal("failed import preflight changed Lesson artifacts")
			}
			if _, err := os.Stat(filepath.Join(root, ".specscore", "event-outbox")); !os.IsNotExist(err) {
				t.Fatalf("failed import preflight prepared an event: %v", err)
			}
		})
	}
	requireCLIError(t, runLessonImportLegacy(lessonImportLegacyCommand(), nil))
	exerciseLegacyImportApplyAdapterEdges(t)
}

func TestLessonImportLegacy_UncertainRollbackRetainsPreparedEvent(t *testing.T) {
	root := t.TempDir()
	if err := projectdef.WriteSpecConfig(root, lessonTestConfig()); err != nil {
		t.Fatal(err)
	}
	lessonsDir := filepath.Join(root, "spec", "lessons")
	if err := os.MkdirAll(lessonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(root, "LESSONS-LEARNED.md")
	if err := os.WriteFile(source, []byte("## L1 — use a reviewed rule\n\n**Status:** Recorded\n\nHistorical detail.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.name", "SpecScore Test"},
		{"config", "user.email", "test@example.invalid"},
		{"remote", "add", "origin", "https://github.com/example/spec.git"},
		{"add", "."},
		{"commit", "-q", "-m", "fixture"},
	} {
		runGitForFlatMigration(t, root, args...)
	}
	inventory, err := lesson.InventoryLegacy(source)
	if err != nil {
		t.Fatal(err)
	}
	inventory.Source.Repository = "github.com/example/process"
	inventory.Source.Path = "LESSONS-LEARNED.md"
	inventory.Source.Revision = strings.Repeat("a", 40)
	inventory.Source.CommittedAt = "2026-08-10T12:00:00Z"
	mapping := lesson.LegacyMapping{Source: inventory.Source, Entries: []lesson.LegacyMappingEntry{{
		Key: "L1#1", Action: "new", Slug: "reviewed-rule", Status: "Recorded",
		Lesson:          "Apply the reviewed deterministic process control.",
		ProcessGap:      "The workflow lacked a deterministic check.",
		Classifications: []string{"process"},
	}}}
	mappingBytes, err := json.Marshal(mapping)
	if err != nil {
		t.Fatal(err)
	}
	mappingPath := filepath.Join(root, "mapping.json")
	if err := os.WriteFile(mappingPath, mappingBytes, 0o644); err != nil {
		t.Fatal(err)
	}
	deps := defaultLessonCLIDeps()
	deps.inventoryLegacy = func(string) (lesson.LegacyInventory, error) { return inventory, nil }
	deps.applyLegacy = func(string, []string, lesson.LegacyInventory, lesson.LegacyMapping) (lesson.LegacyApplyResult, error) {
		return lesson.LegacyApplyResult{}, &lesson.MutationError{Outcome: lesson.MutationUncertain, Err: errors.New("injected post-publication rollback uncertainty")}
	}
	cmd := lessonImportLegacyCommand()
	for name, value := range map[string]string{"source": source, "mapping": mappingPath, "project": root} {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatal(err)
		}
	}
	if err := cmd.Flags().Set("apply", "true"); err != nil {
		t.Fatal(err)
	}
	var stderr strings.Builder
	cmd.SetErr(&stderr)
	err = runLessonImportLegacyWithDeps(cmd, deps)
	if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Fatalf("exit=%d want=%d err=%v", got, exitcode.Unexpected, err)
	}
	if !strings.Contains(stderr.String()+err.Error(), "recovery required: prepared event") {
		t.Fatalf("missing recovery instruction: stderr=%q err=%v", stderr.String(), err)
	}
	prepared, readErr := event.NewOutbox(root).Prepared()
	if readErr != nil || len(prepared) != 1 || prepared[0].EventName != "lesson.legacy-import-applied" {
		t.Fatalf("serialized outbox must retain prepared legacy-import event: %#v err=%v", prepared, readErr)
	}
}
