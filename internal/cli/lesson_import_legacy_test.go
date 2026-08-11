package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/specscore/specscore-cli/pkg/event"
	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/specscore/specscore-cli/pkg/lint"
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
	requireCLISuccess(t, ensureLessonAncestorIndexes(root))
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

func TestLessonImportLegacy_ApplyReconcilesIndexAndRetriesCleanly(t *testing.T) {
	root := setupSpecRoot(t)
	requireCLISuccess(t, projectdef.WriteSpecConfig(root, lessonTestConfig()))
	requireCLISuccess(t, ensureLessonAncestorIndexes(root))
	configureNoopLessonEvents(t, root)
	source := filepath.Join(root, "LESSONS-LEARNED.md")
	requireCLISuccess(t, os.WriteFile(source, []byte("## L1 — use a reviewed rule\n\n**Status:** Recorded\n\nHistorical detail.\n"), 0o644))
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"config", "user.name", "SpecScore Test"}, {"config", "user.email", "test@example.invalid"}, {"remote", "add", "origin", "https://github.com/example/spec.git"}, {"add", "."}, {"commit", "-q", "-m", "fixture"}} {
		runGitForFlatMigration(t, root, args...)
	}
	inv, err := lesson.InventoryLegacy(source)
	requireCLISuccess(t, err)
	mapping := lesson.LegacyMapping{Source: inv.Source, Entries: []lesson.LegacyMappingEntry{{
		Key: "L1#1", Action: "new", Slug: "reviewed-rule", Status: "Recorded",
		Lesson: "Apply the reviewed deterministic process control.", ProcessGap: "The workflow lacked a deterministic check.",
		Classifications: []string{"process"},
	}}}
	mappingBody, err := json.Marshal(mapping)
	requireCLISuccess(t, err)
	mappingPath := filepath.Join(root, "mapping.json")
	requireCLISuccess(t, os.WriteFile(mappingPath, mappingBody, 0o644))

	for attempt := 0; attempt < 2; attempt++ {
		_, stderr, runErr := runLesson(t, "import-legacy", "--source", source, "--apply", "--mapping", mappingPath, "--project", root, "--format", "json")
		if runErr != nil {
			t.Fatalf("apply attempt %d: %v\nstderr=%s", attempt+1, runErr, stderr)
		}
	}
	indexBody, err := os.ReadFile(filepath.Join(root, "spec", "lessons", "README.md"))
	requireCLISuccess(t, err)
	if got := bytes.Count(indexBody, []byte("[reviewed-rule](reviewed-rule/README.md)")); got != 1 {
		t.Fatalf("legacy import index row count=%d\n%s", got, indexBody)
	}
	violations, err := lint.Lint(lint.Options{SpecRoot: filepath.Join(root, "spec")})
	requireCLISuccess(t, err)
	for _, violation := range violations {
		if violation.Severity == "error" && ownedLessonMutationViolation(violation, []string{"reviewed-rule"}) {
			t.Fatalf("legacy import left owned lint drift: %#v", violation)
		}
	}
}

func TestLessonImportLegacy_SerializesPublishedLessonThroughReconciliation(t *testing.T) {
	root := setupLintCleanProject(t)
	requireCLISuccess(t, projectdef.WriteSpecConfig(root, lessonTestConfig()))
	requireCLISuccess(t, ensureLessonAncestorIndexes(root))
	configureNoopLessonEvents(t, root)
	source := filepath.Join(root, "LESSONS-LEARNED.md")
	requireCLISuccess(t, os.WriteFile(source, []byte("## L1 — serialize import\n\n**Status:** Recorded\n\nHistorical detail.\n"), 0o644))
	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"config", "user.name", "SpecScore Test"}, {"config", "user.email", "test@example.invalid"}, {"remote", "add", "origin", "https://github.com/example/spec.git"}, {"add", "."}, {"commit", "-q", "-m", "fixture"}} {
		runGitForFlatMigration(t, root, args...)
	}
	inv, err := lesson.InventoryLegacy(source)
	requireCLISuccess(t, err)
	mapping := lesson.LegacyMapping{Source: inv.Source, Entries: []lesson.LegacyMappingEntry{{
		Key: "L1#1", Action: "new", Slug: "serialized-import", Status: "Recorded",
		Lesson: "Serialize the reviewed import transaction.", ProcessGap: "The import path lacked a shared mutation lock.",
		Classifications: []string{"process"},
	}}}
	mappingBody, err := json.Marshal(mapping)
	requireCLISuccess(t, err)
	mappingPath := filepath.Join(root, "mapping.json")
	requireCLISuccess(t, os.WriteFile(mappingPath, mappingBody, 0o644))

	lifecycleCmd := lessonChangeStatusCommand()
	setLessonCommandFlags(t, lifecycleCmd, map[string]string{"project": root, "to": "stated"})
	started := make(chan struct{})
	changed := make(chan error, 1)
	deps := defaultLessonCLIDeps()
	realApply := deps.applyLegacy
	deps.applyLegacy = func(dir string, allowed []string, inventory lesson.LegacyInventory, reviewed lesson.LegacyMapping) (lesson.LegacyApplyResult, error) {
		result, applyErr := realApply(dir, allowed, inventory, reviewed)
		if applyErr != nil {
			return result, applyErr
		}
		go func() {
			close(started)
			changed <- runLessonChangeStatusWithDeps(lifecycleCmd, []string{"serialized-import"}, defaultLessonCLIDeps())
		}()
		<-started
		select {
		case changeErr := <-changed:
			return result, errors.New("lifecycle mutation escaped import lock before reconciliation: " + errorText(changeErr))
		case <-time.After(100 * time.Millisecond):
		}
		return result, nil
	}
	cmd := lessonImportLegacyCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"source": source, "mapping": mappingPath, "project": root, "apply": "true"})
	requireCLISuccess(t, runLessonImportLegacyWithDeps(cmd, deps))
	select {
	case changeErr := <-changed:
		requireCLISuccess(t, changeErr)
	case <-time.After(2 * time.Second):
		t.Fatal("lifecycle mutation deadlocked after legacy import released its locks")
	}
	indexBody, err := os.ReadFile(filepath.Join(root, "spec", "lessons", "README.md"))
	requireCLISuccess(t, err)
	if !strings.Contains(string(indexBody), "[serialized-import](serialized-import/README.md) | Stated |") {
		t.Fatalf("serialized import/lifecycle row is stale:\n%s", indexBody)
	}
}

func TestLessonImportLegacy_OwnerMarkerStatUncertaintyRetainsPreparedEvent(t *testing.T) {
	root := setupSpecRoot(t)
	requireCLISuccess(t, projectdef.WriteSpecConfig(root, lessonTestConfig()))
	requireCLISuccess(t, ensureLessonAncestorIndexes(root))
	mapping := lesson.LegacyMapping{Entries: []lesson.LegacyMappingEntry{{Action: "new", Slug: "owner-stat"}}}
	mappingBody, err := json.Marshal(mapping)
	requireCLISuccess(t, err)
	mappingPath := filepath.Join(root, "mapping.json")
	requireCLISuccess(t, os.WriteFile(mappingPath, mappingBody, 0o644))
	deps := defaultLessonCLIDeps()
	deps.inventoryLegacy = func(string) (lesson.LegacyInventory, error) { return lesson.LegacyInventory{}, nil }
	deps.preflightLegacy = func(string, []string, lesson.LegacyInventory, lesson.LegacyMapping) error { return nil }
	deps.applyLegacy = func(string, []string, lesson.LegacyInventory, lesson.LegacyMapping) (lesson.LegacyApplyResult, error) {
		return lesson.LegacyApplyResult{}, nil
	}
	realStat := deps.fs.stat
	deps.fs.stat = func(path string) (os.FileInfo, error) {
		if strings.HasSuffix(path, ".legacy-import-owner") {
			return nil, errors.New("owner stat")
		}
		return realStat(path)
	}
	cmd := lessonImportLegacyCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"source": "source", "mapping": mappingPath, "project": root, "apply": "true"})
	err = runLessonImportLegacyWithDeps(cmd, deps)
	if err == nil || !strings.Contains(err.Error(), "owner stat") || !strings.Contains(err.Error(), "recovery required") {
		t.Fatalf("owner-marker stat uncertainty = %v", err)
	}
}
