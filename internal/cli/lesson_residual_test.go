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
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/specscore/specscore-cli/pkg/lint"
	"github.com/specscore/specscore-cli/pkg/projectdef"
	"github.com/spf13/cobra"
)

func TestLessonClassificationShapesAndNewFailures(t *testing.T) {
	for _, cfg := range []projectdef.SpecConfig{
		{},
		{Extras: map[string]any{"lessons": "bad"}},
		{Extras: map[string]any{"lessons": map[string]any{"classifications": "bad"}}},
		{Extras: map[string]any{"lessons": map[string]any{"classifications": []any{"", 7}}}},
	} {
		if got := lessonClassificationsFromConfig(cfg); got != nil {
			t.Fatalf("invalid classifications = %#v", got)
		}
	}
	if got := lessonClassificationsFromConfig(projectdef.SpecConfig{Extras: map[string]any{"lessons": map[string]any{"classifications": []any{" process ", 7}}}}); len(got) != 1 || got[0] != "process" {
		t.Fatalf("normalized classifications = %#v", got)
	}

	for _, tc := range []struct {
		name  string
		setup func(*testing.T, string)
		flags map[string]string
	}{
		{"malformed-config", func(t *testing.T, root string) {
			_ = os.WriteFile(filepath.Join(root, "specscore.yaml"), []byte("bad: ["), 0o644)
		}, nil},
		{"empty-vocabulary", func(t *testing.T, root string) { _ = projectdef.WriteSpecConfig(root, projectdef.SpecConfig{}) }, nil},
		{"duplicate-vocabulary", func(t *testing.T, root string) {
			_ = projectdef.WriteSpecConfig(root, projectdef.SpecConfig{Extras: map[string]any{"lessons": map[string]any{"classifications": []string{"process", "process"}}}})
		}, nil},
		{"legacy-collision", func(t *testing.T, root string) {
			_ = projectdef.WriteSpecConfig(root, lessonTestConfig())
			_ = os.MkdirAll(filepath.Join(root, "spec", "lessons"), 0o755)
			_ = os.WriteFile(filepath.Join(root, "spec", "lessons", "rule.md"), []byte("legacy"), 0o644)
		}, nil},
		{"unsafe-title", func(t *testing.T, root string) { _ = projectdef.WriteSpecConfig(root, lessonTestConfig()) }, map[string]string{"title": "owner@example.com"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := setupSpecRoot(t)
			withCwd(t, root)
			tc.setup(t, root)
			cmd := lessonNewCommand()
			setLessonCommandFlags(t, cmd, tc.flags)
			requireCLIError(t, runLessonNewWithDeps(cmd, []string{"rule"}, defaultLessonCLIDeps()))
		})
	}

	root := setupSpecRoot(t)
	withCwd(t, root)
	requireCLISuccess(t, projectdef.WriteSpecConfig(root, lessonTestConfig()))
	for _, phase := range []string{"prepare", "parse", "index", "lint", "commit"} {
		t.Run(phase, func(t *testing.T) {
			deps := defaultLessonCLIDeps()
			slug := "new-" + phase
			switch phase {
			case "prepare":
				deps.prepareEvent = func(string, string, string, map[string]any, time.Time) (*preparedLessonEvent, error) {
					return nil, errors.New("prepare")
				}
			case "parse":
				deps.parse = func(string) (*lesson.Lesson, error) { return nil, errors.New("parse") }
			case "index":
				deps.indexUpsert = func(string, *lesson.Lesson) error { return errors.New("index") }
			case "lint":
				deps.lint = func(lint.Options) ([]lint.Violation, error) { return nil, errors.New("lint") }
			case "commit":
				deps.prepareEvent = func(string, string, string, map[string]any, time.Time) (*preparedLessonEvent, error) {
					return &preparedLessonEvent{outbox: event.NewOutbox(root), event: event.Event{UUID: uuid.NewString()}}, nil
				}
			}
			cmd := lessonNewCommand()
			setLessonCommandFlags(t, cmd, nil)
			requireCLIError(t, runLessonNewWithDeps(cmd, []string{slug}, deps))
		})
	}
	for _, tc := range []struct{ slug, project string }{{"Bad_Slug", root}, {"valid", filepath.Join(t.TempDir(), "missing")}} {
		cmd := lessonNewCommand()
		setLessonCommandFlags(t, cmd, map[string]string{"project": tc.project})
		requireCLIError(t, runLessonNewWithDeps(cmd, []string{tc.slug}, defaultLessonCLIDeps()))
	}
}

func TestMigrationInputAndLateBoundaryFailures(t *testing.T) {
	for _, tc := range []struct {
		name, slug string
		flags      map[string]string
	}{
		{"invalid-slug", "Bad_Slug", map[string]string{"classification": "process"}},
		{"invalid-format", "rule", map[string]string{"classification": "process", "format": "bad"}},
		{"missing-classification", "rule", nil},
		{"missing-project", "rule", map[string]string{"classification": "process", "project": filepath.Join(t.TempDir(), "missing")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := lessonMigrateFlatCommand()
			setLessonCommandFlags(t, cmd, tc.flags)
			requireCLIError(t, runLessonMigrateFlatWithDeps(cmd, []string{tc.slug}, defaultLessonCLIDeps()))
		})
	}
	for _, selected := range []string{"other", "process,process"} {
		root := setupFlatMigrationCLIProject(t, "classification")
		cmd := lessonMigrateFlatCommand()
		setLessonCommandFlags(t, cmd, map[string]string{"classification": selected, "project": root})
		requireCLIError(t, runLessonMigrateFlatWithDeps(cmd, []string{"classification"}, defaultLessonCLIDeps()))
	}
	for _, phase := range []string{"prepare", "migrate", "parse", "owned-lint", "finalize"} {
		t.Run(phase, func(t *testing.T) {
			root := setupFlatMigrationCLIProject(t, "boundary")
			deps := defaultLessonCLIDeps()
			switch phase {
			case "prepare":
				deps.prepareEventWithID = func(string, string, string, map[string]any, time.Time, string) (*preparedLessonEvent, error) {
					return nil, errors.New("prepare")
				}
			case "migrate":
				deps.prepareEventWithID = func(_ string, _, _ string, _ map[string]any, _ time.Time, id string) (*preparedLessonEvent, error) {
					_ = os.Remove(filepath.Join(root, "spec", "lessons", "boundary.md"))
					return &preparedLessonEvent{disabled: true, event: event.Event{UUID: id}}, nil
				}
			case "parse":
				deps.afterFlatPhase = func(actual string) error {
					if actual == "artifact-publication" {
						return os.WriteFile(filepath.Join(root, "spec", "lessons", "boundary", "README.md"), []byte("bad"), 0o644)
					}
					return nil
				}
			case "owned-lint":
				deps.lint = func(lint.Options) ([]lint.Violation, error) {
					return []lint.Violation{{File: "lessons/boundary/README.md", Rule: "L-001", Severity: "error", Message: "bad"}}, nil
				}
			case "finalize":
				deps.finalizeFlat = func(lesson.FlatMigrationOptions, string) error { return errors.New("finalize") }
			}
			if _, _, err := runFlatMigrationWithDeps(t, root, "boundary", deps); err == nil {
				t.Fatal("injected migration boundary was accepted")
			}
		})
	}
}

func TestLegacyImportValidationAndOutput(t *testing.T) {
	inv := lesson.LegacyInventory{}
	for _, format := range []string{"json", "yaml", "text"} {
		cmd := lessonImportLegacyCommand()
		out, _ := setLessonCommandFlags(t, cmd, map[string]string{"source": "ignored", "dry-run": "true", "format": format, "project": setupSpecRoot(t)})
		deps := defaultLessonCLIDeps()
		deps.inventoryLegacy = func(string) (lesson.LegacyInventory, error) { return inv, nil }
		requireCLISuccess(t, runLessonImportLegacyWithDeps(cmd, deps))
		assertStructuredOutput(t, format, out.String())
	}
	for _, flags := range []map[string]string{
		nil,
		{"source": "x"},
		{"source": "x", "dry-run": "true", "apply": "true"},
		{"source": "x", "dry-run": "true", "format": "bad"},
	} {
		cmd := lessonImportLegacyCommand()
		setLessonCommandFlags(t, cmd, flags)
		requireCLIError(t, runLessonImportLegacyWithDeps(cmd, defaultLessonCLIDeps()))
	}
	for _, format := range []string{"json", "yaml", "text"} {
		cmd := lessonImportLegacyCommand()
		cmd.SetOut(&errWriter{})
		requireCLIError(t, writeLegacyOutput(cmd, format, map[string]string{"x": "y"}))
	}
	cmd := lessonImportLegacyCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"source": "missing", "dry-run": "true", "project": setupSpecRoot(t)})
	deps := defaultLessonCLIDeps()
	deps.inventoryLegacy = func(string) (lesson.LegacyInventory, error) { return inv, errors.New("inventory") }
	requireCLIError(t, runLessonImportLegacyWithDeps(cmd, deps))
}

func TestLessonCheckResidualPredicatesAndErrors(t *testing.T) {
	if _, err := countCheckedLessons(nil, "Recorded", true, 0); err == nil {
		t.Fatal("conflicting filters were accepted")
	}
	if _, err := countCheckedLessons(nil, "bogus", false, 0); err == nil {
		t.Fatal("invalid status was accepted")
	}
	items := []*lesson.Lesson{{Status: "Enforced", Recurred: 9}, {Status: "Recorded", Recurred: 2}}
	if n, err := countCheckedLessons(items, "Recorded", false, 2); err != nil || n != 1 {
		t.Fatalf("filtered count = %d, %v", n, err)
	}
	if got := listArgsForCheck("root", "Recorded", true, 2, "json"); len(got) != 9 {
		t.Fatalf("complete list args = %#v", got)
	}

	cmd := lessonCheckCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"max": "-1"})
	requireCLIError(t, runLessonCheck(cmd, nil))
	cmd = lessonCheckCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"project": filepath.Join(t.TempDir(), "missing")})
	requireCLIError(t, runLessonCheck(cmd, nil))
	root := setupSpecRoot(t)
	requireCLISuccess(t, os.WriteFile(filepath.Join(root, "spec", "lessons"), []byte("not a directory"), 0o644))
	cmd = lessonCheckCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"project": root})
	requireCLIError(t, runLessonCheck(cmd, nil))

	root = canonicalLessonProject(t)
	canonical := filepath.Join(root, "spec", "lessons", "review-before-merge", "README.md")
	occDir := filepath.Join(filepath.Dir(canonical), "occurrences")
	requireCLISuccess(t, os.WriteFile(filepath.Join(occDir, "bad.json"), []byte("{"), 0o644))
	if _, err := countCheckedLessons([]*lesson.Lesson{{Canonical: true, Path: canonical}}, "", false, 0); err == nil {
		t.Fatal("malformed canonical occurrence was accepted")
	}
	requireCLISuccess(t, os.RemoveAll(occDir))
	for _, flags := range []map[string]string{{"project": root, "status": "bogus"}, {"project": root, "format": "bogus"}} {
		cmd = lessonCheckCommand()
		setLessonCommandFlags(t, cmd, flags)
		requireCLIError(t, runLessonCheck(cmd, nil))
	}
}

func TestLessonRelationAdapterFailures(t *testing.T) {
	root := setupSpecRoot(t)
	requireCLISuccess(t, projectdef.WriteSpecConfig(root, lessonTestConfig()))
	from, typ, to := "one", "related", "two"
	for _, phase := range []string{"root", "prepare", "add-compensated", "add-uncertain", "commit", "delivery"} {
		cmd := lessonRelationAddCommand()
		flags := map[string]string{"project": root, "type": typ, "confirm": lesson.RelationToken(from, typ, to)}
		deps := defaultLessonCLIDeps()
		switch phase {
		case "root":
			flags["project"] = filepath.Join(t.TempDir(), "missing")
		case "prepare":
			deps.prepareEvent = func(string, string, string, map[string]any, time.Time) (*preparedLessonEvent, error) {
				return nil, errors.New("prepare")
			}
		case "add-compensated":
			deps.addRelation = func(string, string, string, string) error {
				return &lesson.MutationError{Outcome: lesson.MutationCompensated, Err: errors.New("add")}
			}
		case "add-uncertain":
			deps.addRelation = func(string, string, string, string) error { return errors.New("add") }
		case "commit":
			deps.addRelation = func(string, string, string, string) error { return nil }
			deps.prepareEvent = func(string, string, string, map[string]any, time.Time) (*preparedLessonEvent, error) {
				return &preparedLessonEvent{outbox: event.NewOutbox(root), event: event.Event{UUID: "missing"}}, nil
			}
		case "delivery":
			deps.addRelation = func(string, string, string, string) error { return nil }
			configureFailingLessonEvents(t, root)
		}
		setLessonCommandFlags(t, cmd, flags)
		err := runLessonRelationAddWithDeps(cmd, []string{from, to}, deps)
		if phase == "delivery" {
			requireCLISuccess(t, err)
		} else if err == nil {
			t.Fatalf("%s failure was accepted", phase)
		}
	}
	cmd := lessonRelationListCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"project": filepath.Join(t.TempDir(), "missing")})
	requireCLIError(t, runLessonRelationListWithDeps(cmd, []string{from}, defaultLessonCLIDeps()))
}

func TestLessonRelationCycleValidationLeavesNoPreparedEvent(t *testing.T) {
	root := setupSpecRoot(t)
	requireCLISuccess(t, projectdef.WriteSpecConfig(root, lessonTestConfig()))
	configureNoopLessonEvents(t, root)
	for _, slug := range []string{"first", "second"} {
		if _, stderr, err := runLesson(t, "new", slug, "--project", root); err != nil {
			t.Fatalf("lesson new %s: %v stderr=%s", slug, err, stderr)
		}
	}
	add := func(from, to string) error {
		_, _, err := runLesson(t, "relation", "add", from, to, "--type", "supersedes", "--confirm", lesson.RelationToken(from, "supersedes", to), "--project", root)
		return err
	}
	if err := add("first", "second"); err != nil {
		t.Fatal(err)
	}
	if err := add("second", "first"); err == nil {
		t.Fatal("supersession cycle was accepted")
	}
	prepared, err := event.NewOutbox(root).Prepared()
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared) != 0 {
		t.Fatalf("cycle validation left a misleading prepared event: %#v", prepared)
	}
}

func TestLessonOccurrenceAdapterAndValidationEdges(t *testing.T) {
	for _, tc := range []struct{ slug, project string }{{"Bad_Slug", ""}, {"missing", filepath.Join(t.TempDir(), "missing")}} {
		cmd := lessonOccurrenceAddCommand()
		setLessonCommandFlags(t, cmd, map[string]string{"project": tc.project})
		requireCLIError(t, runLessonOccurrenceAddWithDeps(cmd, []string{tc.slug}, defaultLessonCLIDeps()))
	}
	root := setupSpecRoot(t)
	_ = projectdef.WriteSpecConfig(root, lessonTestConfig())
	for _, body := range []string{"", "bad"} {
		slug := "missing"
		if body != "" {
			slug = "malformed"
			_ = os.MkdirAll(filepath.Join(root, "spec", "lessons"), 0o755)
			_ = os.Mkdir(filepath.Join(root, "spec", "lessons", slug+".md"), 0o755)
		}
		cmd := lessonOccurrenceAddCommand()
		setLessonCommandFlags(t, cmd, map[string]string{"project": root})
		requireCLIError(t, runLessonOccurrenceAddWithDeps(cmd, []string{slug}, defaultLessonCLIDeps()))
	}
	writeLessonInDir(t, filepath.Join(root, "spec", "lessons"), "legacy", "Recorded")
	cmd := lessonOccurrenceAddCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"project": root})
	requireCLIError(t, runLessonOccurrenceAddWithDeps(cmd, []string{"legacy"}, defaultLessonCLIDeps()))

	root = canonicalLessonProject(t)
	configureNoopLessonEvents(t, root)
	cmd = lessonOccurrenceAddCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"project": root, "summary": "edge", "context-json": `{}`, "evidence-kind": "url", "evidence-ref": "https://example.invalid/evidence"})
	requireCLISuccess(t, runLessonOccurrenceAddWithDeps(cmd, []string{"review-before-merge"}, defaultLessonCLIDeps()))
	if _, err := occurrencePath(func() *cobra.Command {
		c := lessonOccurrenceListCommand()
		setLessonCommandFlags(t, c, map[string]string{"project": filepath.Join(t.TempDir(), "missing")})
		return c
	}(), "x"); err == nil {
		t.Fatal("missing occurrence project was accepted")
	}

	for _, args := range [][]string{{"init", "-q", "-b", "main"}, {"config", "user.name", "SpecScore Test"}, {"config", "user.email", "test@example.invalid"}, {"add", "."}, {"commit", "-q", "-m", "fixture"}} {
		runGitForFlatMigration(t, root, args...)
	}
	if git, ok := captureOccurrenceContext(root, "")["git"].(map[string]any); !ok || git["branch"] != "main" {
		t.Fatalf("active branch context = %#v", git)
	}

	requireCLISuccess(t, os.WriteFile(filepath.Join(root, "specscore.yaml"), []byte("events:\n  subscribers:\n    - type: exec\n      command: [/bin/false]\n"), 0o644))
	cmd = lessonOccurrenceAddCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"project": root, "summary": "pending", "context-json": `{}`})
	requireCLISuccess(t, runLessonOccurrenceAddWithDeps(cmd, []string{"review-before-merge"}, defaultLessonCLIDeps()))
}

func TestLessonReadAdaptersRejectBrokenInputs(t *testing.T) {
	for _, project := range []string{filepath.Join(t.TempDir(), "missing"), setupSpecRoot(t)} {
		cmd := lessonAgentsCommand()
		setLessonCommandFlags(t, cmd, map[string]string{"project": project})
		requireCLIError(t, runLessonAgents(cmd, []string{"missing"}))
	}
	root := setupSpecRoot(t)
	_ = projectdef.WriteSpecConfig(root, lessonTestConfig())
	_ = os.MkdirAll(filepath.Join(root, "spec", "lessons"), 0o755)
	_ = os.Mkdir(filepath.Join(root, "spec", "lessons", "bad.md"), 0o755)
	cmd := lessonAgentsCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"project": root})
	requireCLIError(t, runLessonAgents(cmd, []string{"bad"}))
	t.Setenv(lessonAgentsHookEnv, "/bin/true")
	requireCLIError(t, invokeLessonAgentsHookWithRunner(cmd, "refresh", root, filepath.Join(root, "missing"), "bad", "", "", runLessonAgentsHook))

	root = canonicalLessonProject(t)
	occDir := filepath.Join(root, "spec", "lessons", "review-before-merge", "occurrences")
	_ = os.WriteFile(filepath.Join(occDir, "bad.json"), []byte("{"), 0o644)
	cmd = lessonInfoCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"project": root})
	requireCLIError(t, runLessonInfo(cmd, []string{"review-before-merge"}))
	for _, fields := range []string{"", "status"} {
		list := lessonListCommand()
		setLessonCommandFlags(t, list, map[string]string{"project": root, "fields": fields})
		requireCLIError(t, runLessonList(list, nil))
	}
}

func TestLessonListInfoCompatibilityEdges(t *testing.T) {
	if fields, err := parseLessonFields("status, ,status,recurred"); err != nil || len(fields) != 2 {
		t.Fatalf("fields=%v err=%v", fields, err)
	}
	if lessonFieldValue(&lesson.Lesson{Recurred: 3}, "recurred") != "3" || lessonFieldValue(&lesson.Lesson{}, "bogus") != "" {
		t.Fatal("field projection mismatch")
	}
	for _, args := range map[string][]string{"info": {"info", "any"}, "list": {"list"}} {
		_, _, err := runLesson(t, append(args, "--project", filepath.Join(t.TempDir(), "missing"))...)
		requireCLIError(t, err)
	}
	root := setupSpecRoot(t)
	lessonsDir := filepath.Join(root, "spec", "lessons")
	_ = os.WriteFile(lessonsDir, []byte("file"), 0o644)
	if _, _, err := runLesson(t, "list", "--project", root); err == nil {
		t.Fatal("list accepted unreadable directory")
	}

	lessonsDir = setupLessonsSpec(t)
	root = filepath.Dir(filepath.Dir(lessonsDir))
	_ = os.Mkdir(filepath.Join(lessonsDir, "broken.md"), 0o755)
	if _, _, err := runLesson(t, "info", "broken", "--project", root); err == nil {
		t.Fatal("info accepted unreadable Lesson")
	}
	writeLessonRaw(t, lessonsDir, "old", "# Lesson: Old\n\n**Status:** Superseded\n**Superseded By:** new\n\n## Incident\n\nx\n\n## Process gap\n\nx\n\n## Check\n\nx\n\n## Enforcement\n\nx\n")
	if out, _, err := runLesson(t, "info", "old", "--project", root, "--format", "text"); err != nil || !strings.Contains(out, "Superseded By") {
		t.Fatalf("text info=%q err=%v", out, err)
	}
	for _, args := range [][]string{{"info", "old"}, {"list", "--fields", "status", "--status", "Recorded"}, {"list", "--format", "yaml"}} {
		cmd := lessonCommand()
		cmd.SetOut(&errWriter{})
		cmd.SetErr(&errWriter{})
		cmd.SetArgs(append(args, "--project", root))
		requireCLIError(t, cmd.Execute())
	}
}

func TestNonLessonResidualFilesystemEdges(t *testing.T) {
	if failurePath(errors.New("plain")) != "" {
		t.Fatal("plain error exposed a failure path")
	}

	root := setupSpecRoot(t)
	readme := filepath.Join(root, "spec", "plans", "README.md")
	requireCLISuccess(t, os.MkdirAll(readme, 0o755))
	withCwd(t, root)
	if _, _, err := runPlan(t, "new", "sync-fails", "--title", "Sync Fails"); err == nil {
		t.Fatal("plan index directory was accepted")
	}

	root = setupFeatureSpec(t, "Draft")
	config := filepath.Join(root, projectdef.SpecConfigFile)
	_ = os.Remove(config)
	requireCLISuccess(t, os.Symlink(projectdef.SpecConfigFile, config))
	if _, _, err := runFeature(t, "new", "--title", "Stat Fails", "--description", "fixture", "--project", root); err == nil {
		t.Fatal("feature config stat failure was accepted")
	}
}

func TestLessonNewFilesystemAndDeliveryEdges(t *testing.T) {
	for _, phase := range []string{"legacy-stat", "target-stat", "snapshot-read", "snapshot-stat", "lesson-mkdir", "occurrences-mkdir", "write"} {
		root := setupSpecRoot(t)
		_ = projectdef.WriteSpecConfig(root, lessonTestConfig())
		cmd := lessonNewCommand()
		flags := map[string]string{"project": root}
		deps := defaultLessonCLIDeps()
		slug := "edge-" + phase
		target := filepath.Join(root, "spec", "lessons", slug, "README.md")
		switch phase {
		case "legacy-stat":
			real := deps.fs.stat
			deps.fs.stat = func(p string) (os.FileInfo, error) {
				if strings.HasSuffix(p, slug+".md") {
					return nil, errors.New("stat")
				}
				return real(p)
			}
		case "target-stat":
			real := deps.fs.stat
			deps.fs.stat = func(p string) (os.FileInfo, error) {
				if p == target {
					return nil, errors.New("stat")
				}
				return real(p)
			}
		case "snapshot-read":
			_ = os.MkdirAll(filepath.Dir(target), 0o755)
			_ = os.WriteFile(target, []byte("old"), 0o644)
			flags["force"] = "true"
			real := deps.fs.read
			deps.fs.read = func(p string) ([]byte, error) {
				if p == target {
					return nil, errors.New("read")
				}
				return real(p)
			}
		case "snapshot-stat":
			real := deps.fs.stat
			deps.fs.stat = func(p string) (os.FileInfo, error) {
				if strings.HasSuffix(p, "spec/README.md") {
					return nil, errors.New("stat")
				}
				return real(p)
			}
		case "occurrences-mkdir":
			real := deps.fs.mkdirAll
			deps.fs.mkdirAll = func(p string, m os.FileMode) error {
				if filepath.Base(p) == "occurrences" {
					return errors.New("mkdir")
				}
				return real(p, m)
			}
		case "lesson-mkdir":
			deps.fs.mkdirAll = func(string, os.FileMode) error { return errors.New("mkdir") }
		case "write":
			real := deps.fs.write
			deps.fs.write = func(p string, b []byte, m os.FileMode) error {
				if p == target {
					return errors.New("write")
				}
				return real(p, b, m)
			}
		}
		setLessonCommandFlags(t, cmd, flags)
		requireCLIError(t, runLessonNewWithDeps(cmd, []string{slug}, deps))
	}
	root := setupSpecRoot(t)
	_ = projectdef.WriteSpecConfig(root, lessonTestConfig())
	configureFailingLessonEvents(t, root)
	cmd := lessonNewCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"project": root})
	requireCLISuccess(t, runLessonNewWithDeps(cmd, []string{"delivery-edge"}, defaultLessonCLIDeps()))
	requireCLISuccess(t, ensureLessonAncestorIndexes(t.TempDir()))
}

func TestLessonLifecycleFilesystemAndPublicationEdges(t *testing.T) {
	for _, phase := range []string{"index-stat", "index-read", "prepare", "post-parse", "commit", "delivery"} {
		root := canonicalLessonProject(t)
		cmd := lessonChangeStatusCommand()
		setLessonCommandFlags(t, cmd, map[string]string{"project": root, "to": "Stated"})
		deps := defaultLessonCLIDeps()
		deps.lint = func(lint.Options) ([]lint.Violation, error) { return nil, nil }
		switch phase {
		case "index-stat":
			deps.fs.stat = func(string) (os.FileInfo, error) { return nil, errors.New("stat") }
		case "index-read":
			deps.durable.open = func(string) (durableFile, error) { return nil, errors.New("fence") }
		case "prepare":
			deps.prepareEvent = func(string, string, string, map[string]any, time.Time) (*preparedLessonEvent, error) {
				return nil, errors.New("prepare")
			}
		case "post-parse":
			deps.parse = func(string) (*lesson.Lesson, error) { return nil, errors.New("parse") }
		case "commit":
			deps.prepareEvent = func(string, string, string, map[string]any, time.Time) (*preparedLessonEvent, error) {
				return &preparedLessonEvent{outbox: event.NewOutbox(root), event: event.Event{UUID: "missing"}}, nil
			}
		case "delivery":
			configureFailingLessonEvents(t, root)
		}
		err := runLessonChangeStatusWithDeps(cmd, []string{"review-before-merge"}, deps)
		if phase == "delivery" {
			requireCLISuccess(t, err)
		} else if err == nil {
			t.Fatalf("%s failure was accepted", phase)
		}
	}

	for _, phase := range []string{"resolve", "parse", "index", "lint", "restore"} {
		root := canonicalLessonProject(t)
		deps := defaultLessonCLIDeps()
		hook, err := prepareLessonPostMutationWithDeps(root, "review-before-merge", deps)
		requireCLISuccess(t, err)
		switch phase {
		case "resolve":
			_ = os.RemoveAll(filepath.Join(root, "spec", "lessons", "review-before-merge"))
		case "parse":
			deps.parse = func(string) (*lesson.Lesson, error) { return nil, errors.New("parse") }
			hook, _ = prepareLessonPostMutationWithDeps(root, "review-before-merge", deps)
		case "index":
			deps.indexUpsert = func(string, *lesson.Lesson) error { return errors.New("index") }
			hook, _ = prepareLessonPostMutationWithDeps(root, "review-before-merge", deps)
		case "lint":
			deps.lint = func(lint.Options) ([]lint.Violation, error) { return nil, errors.New("lint") }
			hook, _ = prepareLessonPostMutationWithDeps(root, "review-before-merge", deps)
		case "restore":
			deps.durable.open = func(string) (durableFile, error) { return nil, errors.New("fence") }
			hook, _ = prepareLessonPostMutationWithDeps(root, "review-before-merge", deps)
		}
		requireCLIError(t, hook())
	}
}

func TestLessonLifecycleUnexpectedAndCompensatedFailures(t *testing.T) {
	root := canonicalLessonProject(t)
	path := filepath.Join(root, "spec", "lessons", "review-before-merge", "README.md")
	_ = os.Remove(path)
	_ = os.Mkdir(path, 0o755)
	cmd := lessonChangeStatusCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"project": root, "to": "Stated"})
	requireCLIError(t, runLessonChangeStatusWithDeps(cmd, []string{"review-before-merge"}, defaultLessonCLIDeps()))

	root = canonicalLessonProject(t)
	cmd = lessonChangeStatusCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"project": root, "to": "Stated"})
	deps := defaultLessonCLIDeps()
	deps.changeStatus = func(lesson.ChangeStatusOptions) (lesson.ChangeStatusResult, error) {
		return lesson.ChangeStatusResult{}, &lesson.MutationError{Outcome: lesson.MutationCompensated, Err: errors.New("change")}
	}
	requireCLIError(t, runLessonChangeStatusWithDeps(cmd, []string{"review-before-merge"}, deps))
}

func exerciseLegacyImportApplyAdapterEdges(t *testing.T) {
	base := func(t *testing.T) (*cobra.Command, lessonCLIDeps, string) {
		root := setupSpecRoot(t)
		_ = projectdef.WriteSpecConfig(root, lessonTestConfig())
		mapping := filepath.Join(root, "mapping.json")
		_ = os.WriteFile(mapping, []byte(`{}`), 0o644)
		cmd := lessonImportLegacyCommand()
		setLessonCommandFlags(t, cmd, map[string]string{"source": "source", "apply": "true", "mapping": mapping, "project": root})
		deps := defaultLessonCLIDeps()
		deps.inventoryLegacy = func(string) (lesson.LegacyInventory, error) { return lesson.LegacyInventory{}, nil }
		deps.preflightLegacy = func(string, []string, lesson.LegacyInventory, lesson.LegacyMapping) error { return nil }
		deps.applyLegacy = func(string, []string, lesson.LegacyInventory, lesson.LegacyMapping) (lesson.LegacyApplyResult, error) {
			return lesson.LegacyApplyResult{}, nil
		}
		return cmd, deps, root
	}
	for _, phase := range []string{"root", "mapping-required", "mapping-read", "mapping-parse", "mapping-trailing", "config", "vocabulary", "preflight", "prepare", "apply-compensated", "commit", "delivery", "output"} {
		cmd, deps, root := base(t)
		switch phase {
		case "root":
			_ = cmd.Flags().Set("project", filepath.Join(t.TempDir(), "missing"))
		case "mapping-required":
			_ = cmd.Flags().Set("mapping", "")
		case "mapping-read":
			_ = cmd.Flags().Set("mapping", filepath.Join(root, "missing"))
		case "mapping-parse":
			_ = os.WriteFile(filepath.Join(root, "mapping.json"), []byte("{"), 0o644)
		case "mapping-trailing":
			_ = os.WriteFile(filepath.Join(root, "mapping.json"), []byte("{}{}"), 0o644)
		case "config":
			deps.readConfig = func(string) (projectdef.SpecConfig, error) { return projectdef.SpecConfig{}, errors.New("config") }
		case "vocabulary":
			deps.readConfig = func(string) (projectdef.SpecConfig, error) { return projectdef.SpecConfig{}, nil }
		case "preflight":
			deps.preflightLegacy = func(string, []string, lesson.LegacyInventory, lesson.LegacyMapping) error {
				return errors.New("preflight")
			}
		case "prepare":
			deps.prepareEvent = func(string, string, string, map[string]any, time.Time) (*preparedLessonEvent, error) {
				return nil, errors.New("prepare")
			}
		case "apply-compensated":
			deps.applyLegacy = func(string, []string, lesson.LegacyInventory, lesson.LegacyMapping) (lesson.LegacyApplyResult, error) {
				return lesson.LegacyApplyResult{}, &lesson.MutationError{Outcome: lesson.MutationCompensated, Err: errors.New("apply")}
			}
		case "commit":
			deps.prepareEvent = func(string, string, string, map[string]any, time.Time) (*preparedLessonEvent, error) {
				return &preparedLessonEvent{outbox: event.NewOutbox(root), event: event.Event{UUID: "missing"}}, nil
			}
		case "delivery":
			configureFailingLessonEvents(t, root)
		case "output":
			cmd.SetOut(&errWriter{})
		}
		err := runLessonImportLegacyWithDeps(cmd, deps)
		if phase == "delivery" {
			requireCLISuccess(t, err)
		} else if err == nil {
			t.Fatalf("%s import failure was accepted", phase)
		}
	}
}

func TestFlatMigrationAdapterAndResumeEdges(t *testing.T) {
	for _, phase := range []string{"config", "vocabulary", "preflight", "already-migrate", "flat-read", "index-read", "migrate-compensated", "parse", "commit", "delivery", "rollback-missing-index", "rollback-failure"} {
		root := setupFlatMigrationCLIProject(t, "adapter")
		deps := defaultLessonCLIDeps()
		switch phase {
		case "config":
			deps.readConfig = func(string) (projectdef.SpecConfig, error) { return projectdef.SpecConfig{}, errors.New("config") }
		case "vocabulary":
			deps.readConfig = func(string) (projectdef.SpecConfig, error) { return projectdef.SpecConfig{}, nil }
		case "preflight":
			deps.preflightFlat = func(lesson.FlatMigrationOptions) (lesson.FlatMigrationPreflight, error) {
				return lesson.FlatMigrationPreflight{}, errors.New("preflight")
			}
		case "already-migrate":
			deps.preflightFlat = func(lesson.FlatMigrationOptions) (lesson.FlatMigrationPreflight, error) {
				return lesson.FlatMigrationPreflight{AlreadyMigrated: true}, nil
			}
			deps.migrateFlat = func(lesson.FlatMigrationOptions) (lesson.FlatMigrationResult, error) {
				return lesson.FlatMigrationResult{}, errors.New("migrate")
			}
		case "flat-read":
			deps.fs.read = func(string) ([]byte, error) { return nil, errors.New("read") }
		case "index-read":
			real := deps.fs.read
			deps.fs.read = func(p string) ([]byte, error) {
				if filepath.Base(p) == "README.md" {
					return nil, errors.New("read")
				}
				return real(p)
			}
		case "migrate-compensated":
			deps.migrateFlat = func(lesson.FlatMigrationOptions) (lesson.FlatMigrationResult, error) {
				return lesson.FlatMigrationResult{}, &lesson.MutationError{Outcome: lesson.MutationCompensated, Err: errors.New("migrate")}
			}
		case "parse":
			deps.parse = func(string) (*lesson.Lesson, error) { return nil, errors.New("parse") }
		case "commit":
			deps.prepareEventWithID = func(string, string, string, map[string]any, time.Time, string) (*preparedLessonEvent, error) {
				return &preparedLessonEvent{outbox: event.NewOutbox(root), event: event.Event{UUID: "missing"}}, nil
			}
		case "delivery":
			configureFailingLessonEvents(t, root)
		case "rollback-missing-index":
			_ = os.Remove(filepath.Join(root, "spec", "lessons", "README.md"))
			deps.parse = func(string) (*lesson.Lesson, error) { return nil, errors.New("parse") }
		case "rollback-failure":
			deps.parse = func(string) (*lesson.Lesson, error) { return nil, errors.New("parse") }
			deps.durable = faultDurableOps("remove")
		}
		_, _, err := runFlatMigrationWithDeps(t, root, "adapter", deps)
		if phase == "delivery" {
			requireCLISuccess(t, err)
		} else if err == nil {
			t.Fatalf("%s migration failure was accepted", phase)
		}
	}
	root := setupFlatMigrationCLIProject(t, "resume-edge")
	deps := defaultLessonCLIDeps()
	deps.afterFlatPhase = func(phase string) error {
		if phase == "artifact-publication" {
			return errors.New("crash")
		}
		return nil
	}
	if _, _, err := runFlatMigrationWithDeps(t, root, "resume-edge", deps); err == nil {
		t.Fatal("crash boundary was accepted")
	}
	deps = defaultLessonCLIDeps()
	deps.indexUpsert = func(string, *lesson.Lesson) error { return errors.New("index") }
	if _, _, err := runFlatMigrationWithDeps(t, root, "resume-edge", deps); err == nil {
		t.Fatal("resumed index failure was accepted")
	}
	requireCLISuccess(t, removePathIfExists(filepath.Join(t.TempDir(), "missing")))
}

func TestLegacyRecurFilesystemAndPublicationEdges(t *testing.T) {
	for _, phase := range []string{"unsafe", "body-stat", "body-read", "index-stat", "index-read", "prepare", "recur-compensated", "recur-uncertain", "parse", "parse-rollback", "index", "index-rollback", "commit", "delivery"} {
		lessonsDir := setupLessonsSpec(t)
		root := filepath.Dir(filepath.Dir(lessonsDir))
		writeLessonInDir(t, lessonsDir, "legacy", "Recorded")
		cmd := lessonRecurCommand()
		flags := map[string]string{"project": root, "note": "seen again"}
		deps := defaultLessonCLIDeps()
		body, index := filepath.Join(lessonsDir, "legacy.md"), filepath.Join(lessonsDir, "README.md")
		_ = os.WriteFile(index, []byte("# Lessons\n\n| Lesson | Status | Recurred | Date | Owner |\n|---|---|---|---|---|\n"), 0o644)
		switch phase {
		case "unsafe":
			flags["note"] = "owner@example.com"
		case "body-stat":
			deps.fs.stat = func(string) (os.FileInfo, error) { return nil, errors.New("stat") }
		case "body-read":
			real := deps.fs.read
			deps.fs.read = func(p string) ([]byte, error) {
				if p == body {
					return nil, errors.New("read")
				}
				return real(p)
			}
		case "index-stat":
			real := deps.fs.stat
			deps.fs.stat = func(p string) (os.FileInfo, error) {
				if p == index {
					return nil, errors.New("stat")
				}
				return real(p)
			}
		case "index-read":
			_ = os.WriteFile(index, []byte("index"), 0o644)
			real := deps.fs.read
			deps.fs.read = func(p string) ([]byte, error) {
				if p == index {
					return nil, errors.New("read")
				}
				return real(p)
			}
		case "prepare":
			deps.prepareEvent = func(string, string, string, map[string]any, time.Time) (*preparedLessonEvent, error) {
				return nil, errors.New("prepare")
			}
		case "recur-compensated":
			deps.recur = func(string, string) (int, error) {
				return 0, &lesson.MutationError{Outcome: lesson.MutationCompensated, Err: errors.New("recur")}
			}
		case "recur-uncertain":
			deps.recur = func(string, string) (int, error) { return 0, errors.New("recur") }
		case "parse":
			real, calls := deps.parse, 0
			deps.parse = func(p string) (*lesson.Lesson, error) {
				calls++
				if calls == 2 {
					return nil, errors.New("parse")
				}
				return real(p)
			}
		case "parse-rollback":
			real, calls := deps.parse, 0
			deps.parse = func(p string) (*lesson.Lesson, error) {
				calls++
				if calls == 2 {
					_ = os.Remove(body)
					_ = os.Mkdir(body, 0o755)
					return nil, errors.New("parse")
				}
				return real(p)
			}
		case "index":
			deps.indexUpsert = func(string, *lesson.Lesson) error { return errors.New("index") }
		case "index-rollback":
			deps.indexUpsert = func(string, *lesson.Lesson) error {
				_ = os.Remove(body)
				_ = os.Mkdir(body, 0o755)
				return errors.New("index")
			}
		case "commit":
			deps.prepareEvent = func(string, string, string, map[string]any, time.Time) (*preparedLessonEvent, error) {
				return &preparedLessonEvent{outbox: event.NewOutbox(root), event: event.Event{UUID: "missing"}}, nil
			}
		case "delivery":
			_ = projectdef.WriteSpecConfig(root, lessonTestConfig())
			configureFailingLessonEvents(t, root)
		}
		setLessonCommandFlags(t, cmd, flags)
		err := runLessonRecurWithDeps(cmd, []string{"legacy"}, deps)
		if phase == "delivery" {
			requireCLISuccess(t, err)
		} else if err == nil {
			t.Fatalf("%s recurrence failure was accepted", phase)
		}
	}
	dir := t.TempDir()
	requireCLIError(t, restoreLegacyRecurFiles(dir, []byte("x"), 0o644, "", nil, 0, false))
	root := canonicalLessonProject(t)
	configureFailingLessonEvents(t, root)
	cmd := lessonRecurCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"project": root})
	requireCLISuccess(t, runLessonRecurWithDeps(cmd, []string{"review-before-merge"}, defaultLessonCLIDeps()))
}

func TestLessonRecurCommandValidation(t *testing.T) {
	var warning strings.Builder
	warnIfLessonRetired(&warning, "old", "Withdrawn")
	if warning.Len() == 0 {
		t.Fatal("retired Lesson omitted warning")
	}
	for _, args := range [][]string{nil, {"one", "two"}, {"Bad_Slug"}} {
		requireCLIError(t, runLessonRecurWithDeps(lessonRecurCommand(), args, defaultLessonCLIDeps()))
	}
	for _, project := range []string{filepath.Join(t.TempDir(), "missing"), setupSpecRoot(t)} {
		cmd := lessonRecurCommand()
		setLessonCommandFlags(t, cmd, map[string]string{"project": project})
		requireCLIError(t, runLessonRecurWithDeps(cmd, []string{"missing"}, defaultLessonCLIDeps()))
	}
	root := canonicalLessonProject(t)
	cmd := lessonRecurCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"project": root})
	deps := defaultLessonCLIDeps()
	deps.parse = func(string) (*lesson.Lesson, error) { return nil, errors.New("parse") }
	requireCLIError(t, runLessonRecurWithDeps(cmd, []string{"review-before-merge"}, deps))
}
