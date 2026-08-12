package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/specscore/specscore-cli/pkg/event"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/specscore/specscore-cli/pkg/lint"
	"github.com/specscore/specscore-cli/pkg/projectdef"
)

func TestLessonNewForceRejectsConcurrentLifecycleMutation(t *testing.T) {
	root := setupLintCleanProject(t)
	requireCLISuccess(t, projectdef.WriteSpecConfig(root, lessonTestConfig()))
	configureNoopLessonEvents(t, root)
	create := lessonNewCommand()
	setLessonCommandFlags(t, create, map[string]string{"project": root, "title": "Original"})
	requireCLISuccess(t, runLessonNewWithDeps(create, []string{"force-race"}, defaultLessonCLIDeps()))

	entered := make(chan struct{})
	release := make(chan struct{})
	deps := defaultLessonCLIDeps()
	deps.withMutationLock = func(root, slug string, mutate func() error) error {
		close(entered)
		<-release
		return lesson.WithMutationLock(root, slug, mutate)
	}
	force := lessonNewCommand()
	setLessonCommandFlags(t, force, map[string]string{"project": root, "title": "Clobber", "force": "true"})
	forceResult := make(chan error, 1)
	go func() { forceResult <- runLessonNewWithDeps(force, []string{"force-race"}, deps) }()
	<-entered

	change := lessonChangeStatusCommand()
	setLessonCommandFlags(t, change, map[string]string{"project": root, "to": "stated"})
	requireCLISuccess(t, runLessonChangeStatusWithDeps(change, []string{"force-race"}, defaultLessonCLIDeps()))
	close(release)
	if err := <-forceResult; err == nil || !strings.Contains(err.Error(), "changed after --force preflight") {
		t.Fatalf("force overwrite did not fail its ownership check: %v", err)
	}

	path := filepath.Join(root, "spec", "lessons", "force-race", "README.md")
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(body), "**Status:** Stated") || strings.Contains(string(body), "# Lesson: Clobber") {
		t.Fatalf("concurrent lifecycle state was lost:\n%s", body)
	}
	index, err := os.ReadFile(filepath.Join(root, "spec", "lessons", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), "[force-race](force-race/README.md) | Stated |") {
		t.Fatalf("index lost concurrent lifecycle row:\n%s", index)
	}
	prepared, err := event.NewOutbox(root).Prepared()
	if err != nil || len(prepared) != 0 {
		t.Fatalf("prepublication force conflict retained event: %#v, %v", prepared, err)
	}
}

func TestLessonRelationUpdatesBothRowsBeforeEventCommit(t *testing.T) {
	root := setupSpecRoot(t)
	requireCLISuccess(t, projectdef.WriteSpecConfig(root, lessonTestConfig()))
	configureNoopLessonEvents(t, root)
	for _, slug := range []string{"retained", "canonical"} {
		cmd := lessonNewCommand()
		setLessonCommandFlags(t, cmd, map[string]string{"project": root})
		requireCLISuccess(t, runLessonNewWithDeps(cmd, []string{slug}, defaultLessonCLIDeps()))
	}

	cmd := lessonRelationAddCommand()
	setLessonCommandFlags(t, cmd, map[string]string{
		"project": root,
		"type":    "duplicates",
		"confirm": lesson.RelationToken("retained", "duplicates", "canonical"),
	})
	requireCLISuccess(t, runLessonRelationAddWithDeps(cmd, []string{"retained", "canonical"}, defaultLessonCLIDeps()))
	index, err := os.ReadFile(filepath.Join(root, "spec", "lessons", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(index), "[retained](retained/README.md) | Superseded |") {
		t.Fatalf("relation left the retained row stale:\n%s", index)
	}
	violations, err := lint.Lint(lint.Options{SpecRoot: filepath.Join(root, "spec")})
	if err != nil {
		t.Fatal(err)
	}
	for _, violation := range violations {
		if violation.Severity == "error" && violation.Rule == "L-004" && strings.Contains(violation.Message, "retained") {
			t.Fatalf("relation left an index drift violation: %+v", violation)
		}
	}
}

func TestLessonNewForceRereadAndScaffoldPreflightFailures(t *testing.T) {
	root := setupLintCleanProject(t)
	requireCLISuccess(t, projectdef.WriteSpecConfig(root, lessonTestConfig()))
	configureNoopLessonEvents(t, root)
	create := lessonNewCommand()
	setLessonCommandFlags(t, create, map[string]string{"project": root})
	requireCLISuccess(t, runLessonNewWithDeps(create, []string{"force-read"}, defaultLessonCLIDeps()))

	deps := defaultLessonCLIDeps()
	realRead := deps.fs.read
	target := filepath.Join(root, "spec", "lessons", "force-read", "README.md")
	targetReads := 0
	deps.fs.read = func(path string) ([]byte, error) {
		if path == target {
			targetReads++
			if targetReads == 3 {
				return nil, errors.New("force reread")
			}
		}
		return realRead(path)
	}
	force := lessonNewCommand()
	setLessonCommandFlags(t, force, map[string]string{"project": root, "force": "true"})
	if err := runLessonNewWithDeps(force, []string{"force-read"}, deps); err == nil || !strings.Contains(err.Error(), "re-reading --force target") {
		t.Fatalf("force reread failure = %v", err)
	}

	preflightRoot := t.TempDir()
	preflightTarget := filepath.Join(preflightRoot, "spec", "lessons", "x", "README.md")
	requireCLISuccess(t, os.MkdirAll(filepath.Join(preflightRoot, "spec"), 0o755))
	requireCLISuccess(t, os.WriteFile(filepath.Join(preflightRoot, "spec", "README.md"), []byte("index\n"), 0o644))
	fs := defaultLessonCLIDeps().fs
	fs.read = func(string) ([]byte, error) { return nil, errors.New("preflight read") }
	if err := preflightLessonScaffoldWriteSetWithOps(preflightRoot, preflightTarget, fs); err == nil {
		t.Fatal("scaffold preflight read failure was accepted")
	}
}

func TestPrepareRelationPostMutationFaultMatrix(t *testing.T) {
	root := setupSpecRoot(t)
	requireCLISuccess(t, projectdef.WriteSpecConfig(root, lessonTestConfig()))
	requireCLISuccess(t, ensureLessonAncestorIndexes(root))
	lessonsDir := filepath.Join(root, "spec", "lessons")
	for _, slug := range []string{"from", "to"} {
		body, err := lesson.ScaffoldCanonical(lesson.ScaffoldOptions{Slug: slug}, []string{"process"})
		requireCLISuccess(t, err)
		path := filepath.Join(lessonsDir, slug, "README.md")
		requireCLISuccess(t, os.MkdirAll(filepath.Dir(path), 0o755))
		requireCLISuccess(t, os.WriteFile(path, body, 0o644))
	}

	tests := []struct {
		name   string
		typ    string
		to     string
		mutate func(*lessonCLIDeps)
	}{
		{name: "resolve", typ: "duplicates", to: "missing"},
		{name: "parse", typ: "duplicates", mutate: func(d *lessonCLIDeps) {
			d.parse = func(string) (*lesson.Lesson, error) { return nil, errors.New("parse") }
		}},
		{name: "index", typ: "duplicates", mutate: func(d *lessonCLIDeps) {
			d.indexUpsert = func(string, *lesson.Lesson) error { return errors.New("index") }
		}},
		{name: "lint", typ: "duplicates", mutate: func(d *lessonCLIDeps) {
			d.indexUpsert = func(string, *lesson.Lesson) error { return nil }
			d.lint = func(lint.Options) ([]lint.Violation, error) { return nil, errors.New("lint") }
		}},
		{name: "owned violation", typ: "duplicates", mutate: func(d *lessonCLIDeps) {
			d.indexUpsert = func(string, *lesson.Lesson) error { return nil }
			d.lint = func(lint.Options) ([]lint.Violation, error) {
				return []lint.Violation{{File: "lessons/from/README.md", Line: 1, Rule: "L-001", Severity: "error", Message: "owned"}}, nil
			}
		}},
		{name: "fence related sidecar", typ: "related", mutate: func(d *lessonCLIDeps) {
			d.indexUpsert = func(string, *lesson.Lesson) error { return nil }
			d.lint = func(lint.Options) ([]lint.Violation, error) { return nil, nil }
			d.durable.open = func(string) (durableFile, error) { return nil, errors.New("fence") }
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			deps := defaultLessonCLIDeps()
			if tc.mutate != nil {
				tc.mutate(&deps)
			}
			to := tc.to
			if to == "" {
				to = "to"
			}
			hook, err := prepareRelationPostMutationWithDeps(root, "from", tc.typ, to, deps)
			requireCLISuccess(t, err)
			if err := hook(); err == nil {
				t.Fatalf("%s failure was accepted", tc.name)
			}
		})
	}
}

func TestCanonicalOccurrenceSerializesWithLifecycleAndKeepsIndexCurrent(t *testing.T) {
	root := setupLintCleanProject(t)
	requireCLISuccess(t, projectdef.WriteSpecConfig(root, lessonTestConfig()))
	configureNoopLessonEvents(t, root)
	create := lessonNewCommand()
	setLessonCommandFlags(t, create, map[string]string{"project": root})
	requireCLISuccess(t, runLessonNewWithDeps(create, []string{"review-before-merge"}, defaultLessonCLIDeps()))
	started := make(chan struct{})
	changed := make(chan error, 1)
	lifecycleCmd := lessonChangeStatusCommand()
	setLessonCommandFlags(t, lifecycleCmd, map[string]string{"project": root, "to": "stated"})
	deps := defaultLessonCLIDeps()
	realAdd := deps.addOccurrence
	deps.addOccurrence = func(opts lesson.AddOccurrenceOptions) (lesson.Occurrence, error) {
		go func() {
			close(started)
			changed <- runLessonChangeStatusWithDeps(lifecycleCmd, []string{"review-before-merge"}, defaultLessonCLIDeps())
		}()
		<-started
		select {
		case err := <-changed:
			return lesson.Occurrence{}, errors.New("lifecycle mutation escaped occurrence lock before publication: " + errorText(err))
		case <-time.After(100 * time.Millisecond):
		}
		return realAdd(opts)
	}
	cmd := lessonOccurrenceAddCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"project": root, "summary": "serialized", "context-json": `{}`})
	requireCLISuccess(t, runLessonOccurrenceAddWithDeps(cmd, []string{"review-before-merge"}, deps))
	select {
	case err := <-changed:
		requireCLISuccess(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("lifecycle mutation deadlocked after occurrence released its lock")
	}
	body, err := os.ReadFile(filepath.Join(root, "spec", "lessons", "review-before-merge", "README.md"))
	requireCLISuccess(t, err)
	if !strings.Contains(string(body), "**Status:** Stated") {
		t.Fatalf("serialized lifecycle status missing:\n%s", body)
	}
	index, err := os.ReadFile(filepath.Join(root, "spec", "lessons", "README.md"))
	requireCLISuccess(t, err)
	if !strings.Contains(string(index), "[review-before-merge](review-before-merge/README.md) | Stated |") {
		t.Fatalf("occurrence overwrote the serialized lifecycle row:\n%s", index)
	}
}

func TestFlatMigrationSerializesWithLifecycleThroughFinalization(t *testing.T) {
	root := setupFlatMigrationCLIProject(t, "serialized-flat")
	configureNoopLessonEvents(t, root)
	started := make(chan struct{})
	changed := make(chan error, 1)
	lifecycleCmd := lessonChangeStatusCommand()
	setLessonCommandFlags(t, lifecycleCmd, map[string]string{"project": root, "to": "stated"})
	deps := defaultLessonCLIDeps()
	realMigrate := deps.migrateFlat
	deps.migrateFlat = func(opts lesson.FlatMigrationOptions) (lesson.FlatMigrationResult, error) {
		go func() {
			close(started)
			changed <- runLessonChangeStatusWithDeps(lifecycleCmd, []string{"serialized-flat"}, defaultLessonCLIDeps())
		}()
		<-started
		select {
		case err := <-changed:
			return lesson.FlatMigrationResult{}, errors.New("lifecycle mutation escaped migration lock before publication: " + errorText(err))
		case <-time.After(100 * time.Millisecond):
		}
		return realMigrate(opts)
	}
	_, _, err := runFlatMigrationWithDeps(t, root, "serialized-flat", deps)
	requireCLISuccess(t, err)
	select {
	case err := <-changed:
		requireCLISuccess(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("lifecycle mutation deadlocked after flat migration released its lock")
	}
	body, err := os.ReadFile(filepath.Join(root, "spec", "lessons", "serialized-flat", "README.md"))
	requireCLISuccess(t, err)
	if !strings.Contains(string(body), "**Status:** Stated") {
		t.Fatalf("post-migration lifecycle status missing:\n%s", body)
	}
	if _, err := os.Stat(filepath.Join(root, "spec", "lessons", "serialized-flat.md")); !os.IsNotExist(err) {
		t.Fatalf("flat source was not finalized after serial transaction: %v", err)
	}
}

func errorText(err error) string {
	if err == nil {
		return "success"
	}
	return err.Error()
}
