package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

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
			if err := projectdef.WriteSpecConfig(root, fixture.config); err != nil {
				t.Fatal(err)
			}
			lessonsDir := filepath.Join(root, "spec", "lessons")
			if err := os.MkdirAll(lessonsDir, 0o755); err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(root, "LESSONS-LEARNED.md")
			if err := os.WriteFile(source, []byte("## L1 — use a reviewed rule\n\n**Status:** Enforced\n\nHistorical detail.\n"), 0o644); err != nil {
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
			mapping := lesson.LegacyMapping{Source: inventory.Source, Entries: []lesson.LegacyMappingEntry{{
				Key: "L1#1", Action: "new", Slug: "reviewed-rule", Status: "Recorded",
				Lesson:          "Apply the reviewed deterministic process control.",
				ProcessGap:      "The workflow lacked a deterministic check.",
				Classifications: []string{fixture.classification},
			}}}
			mappingBytes, err := json.Marshal(mapping)
			if err != nil {
				t.Fatal(err)
			}
			mappingPath := filepath.Join(root, "mapping.json")
			if err := os.WriteFile(mappingPath, mappingBytes, 0o644); err != nil {
				t.Fatal(err)
			}
			before := treeDigestForCLI(t, lessonsDir)

			if _, _, err := runLesson(t, "import-legacy", "--source", source, "--apply", "--mapping", mappingPath, "--project", root); err == nil {
				t.Fatal("invalid classification preflight was accepted")
			}
			if after := treeDigestForCLI(t, lessonsDir); string(after) != string(before) {
				t.Fatal("failed import preflight changed Lesson artifacts")
			}
			if _, err := os.Stat(filepath.Join(root, ".specscore", "event-outbox")); !os.IsNotExist(err) {
				t.Fatalf("failed import preflight prepared an event: %v", err)
			}
		})
	}
}
