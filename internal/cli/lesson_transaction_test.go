package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/specscore/specscore-cli/pkg/event"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
	"github.com/specscore/specscore-cli/pkg/lint"
	"github.com/specscore/specscore-cli/pkg/projectdef"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

func assertStructuredOutput(t *testing.T, format, output string) {
	t.Helper()
	var value any
	var err error
	switch format {
	case "json":
		err = json.Unmarshal([]byte(output), &value)
	case "yaml":
		err = yaml.Unmarshal([]byte(output), &value)
	default:
		if strings.TrimSpace(output) == "" {
			err = errors.New("empty text output")
		}
	}
	if err != nil {
		t.Fatalf("%s output is not parseable: %v\n%s", format, err, output)
	}
}

func requireCLIError(t testing.TB, err error) {
	t.Helper()
	if err == nil {
		t.Fatal("expected an error")
	}
}

func requireCLISuccess(t testing.TB, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func setLessonCommandFlags(t *testing.T, cmd *cobra.Command, flags map[string]string) (*bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	cmd.SetContext(context.Background())
	for name, value := range flags {
		if err := cmd.Flags().Set(name, value); err != nil {
			t.Fatalf("set --%s: %v", name, err)
		}
	}
	out, errOut := new(bytes.Buffer), new(bytes.Buffer)
	cmd.SetOut(out)
	cmd.SetErr(errOut)
	return out, errOut
}

func configureNoopLessonEvents(t *testing.T, root string) {
	t.Helper()
	cfg, err := projectdef.ReadSpecConfig(root)
	if errors.Is(err, os.ErrNotExist) {
		requireCLISuccess(t, projectdef.WriteSpecConfig(root, lessonTestConfig()))
		cfg, err = projectdef.ReadSpecConfig(root)
	}
	requireCLISuccess(t, err)
	if cfg.Extras == nil {
		cfg.Extras = map[string]any{}
	}
	cfg.Extras["events"] = map[string]any{"subscribers": []any{map[string]any{"type": "noop"}}}
	requireCLISuccess(t, projectdef.WriteSpecConfig(root, cfg))
}

func configureFailingLessonEvents(t *testing.T, root string) {
	t.Helper()
	cfg, err := projectdef.ReadSpecConfig(root)
	requireCLISuccess(t, err)
	if cfg.Extras == nil {
		cfg.Extras = map[string]any{}
	}
	cfg.Extras["events"] = map[string]any{"subscribers": []any{map[string]any{"type": "exec", "command": []any{"/bin/false"}}}}
	requireCLISuccess(t, projectdef.WriteSpecConfig(root, cfg))
}

func assertAbortedLessonEvent(t *testing.T, root, name string) {
	t.Helper()
	outbox := event.NewOutbox(root)
	prepared, err := outbox.Prepared()
	if err != nil || len(prepared) != 0 {
		t.Fatalf("prepared after compensation = %#v, err=%v", prepared, err)
	}
	digest := treeDigestForCLI(t, outbox.Root)
	if !bytes.Contains(digest, []byte(name)) || !bytes.Contains(digest, []byte("aborted")) {
		t.Fatalf("outbox is not an exact terminal abort for %s:\n%s", name, digest)
	}
}

func TestCanonicalOccurrenceTransactionRestoresChildIndexAndOutbox(t *testing.T) {
	for _, phase := range []string{"index", "discovery"} {
		t.Run(phase, func(t *testing.T) {
			root := canonicalLessonProject(t)
			configureNoopLessonEvents(t, root)
			lessonsDir := filepath.Join(root, "spec", "lessons")
			deps := defaultLessonCLIDeps()
			switch phase {
			case "index":
				deps.indexUpsert = func(specRoot string, l *lesson.Lesson) error {
					return errors.New("late index failure")
				}
				cmd := lessonOccurrenceAddCommand()
				setLessonCommandFlags(t, cmd, map[string]string{"summary": "must retain"})
				requireCLIError(t, runLessonOccurrenceAddWithDeps(cmd, []string{"review-before-merge"}, deps))
			case "discovery":
				deps.discoverOccurrences = func(string) ([]lesson.Occurrence, error) { return nil, errors.New("late discovery failure") }
				cmd := lessonRecurCommand()
				setLessonCommandFlags(t, cmd, map[string]string{"note": "must retain"})
				requireCLIError(t, runLessonRecurWithDeps(cmd, []string{"review-before-merge"}, deps))
			}
			items, err := lesson.DiscoverOccurrences(filepath.Join(lessonsDir, "review-before-merge", "README.md"))
			if err != nil || len(items) != 1 {
				t.Fatalf("%s failure did not retain exactly one published child: %#v, %v", phase, items, err)
			}
			index, err := os.ReadFile(filepath.Join(lessonsDir, "README.md"))
			if err != nil {
				t.Fatal(err)
			}
			wantCount := "| 0 |"
			if phase == "discovery" {
				wantCount = "| 1 |"
			}
			if !bytes.Contains(index, []byte(wantCount)) {
				t.Fatalf("%s retained index projection =\n%s", phase, index)
			}
			prepared, err := event.NewOutbox(root).Prepared()
			if err != nil || len(prepared) != 1 || prepared[0].EventName != "lesson.occurrence-recorded" {
				t.Fatalf("%s retained event = %#v, err=%v", phase, prepared, err)
			}
		})
	}
}

func TestCanonicalOccurrenceCompensationPreservesConcurrentIndexRows(t *testing.T) {
	root := canonicalLessonProject(t)
	configureNoopLessonEvents(t, root)
	indexPath := filepath.Join(root, "spec", "lessons", "README.md")
	concurrent := "| [other](other/README.md) | Recorded | process | 0 |  | — |"
	deps := defaultLessonCLIDeps()
	deps.discoverOccurrences = func(string) ([]lesson.Occurrence, error) {
		b, err := os.ReadFile(indexPath)
		requireCLISuccess(t, err)
		b = bytes.Replace(b, []byte("\n## Open Questions"), []byte("\n"+concurrent+"\n\n## Open Questions"), 1)
		requireCLISuccess(t, os.WriteFile(indexPath, b, 0o644))
		return nil, errors.New("late discovery failure")
	}
	cmd := lessonRecurCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"note": "must retain"})
	requireCLIError(t, runLessonRecurWithDeps(cmd, []string{"review-before-merge"}, deps))
	index, err := os.ReadFile(indexPath)
	requireCLISuccess(t, err)
	if !bytes.Contains(index, []byte(concurrent)) || !bytes.Contains(index, []byte("| 1 |")) {
		t.Fatalf("retained publication lost concurrent data or occurrence count:\n%s", index)
	}
	prepared, err := event.NewOutbox(root).Prepared()
	if err != nil || len(prepared) != 1 || prepared[0].EventName != "lesson.occurrence-recorded" {
		t.Fatalf("retained occurrence event = %#v, err=%v", prepared, err)
	}
}

func TestCanonicalOccurrenceReconcileFailureRetainsPreparedEvent(t *testing.T) {
	for _, phase := range []string{"reconcile", "fence"} {
		t.Run(phase, func(t *testing.T) {
			root := canonicalLessonProject(t)
			deps := defaultLessonCLIDeps()
			if phase == "reconcile" {
				deps.indexUpsert = func(string, *lesson.Lesson) error { return errors.New("index") }
			} else {
				deps.durable.open = func(string) (durableFile, error) { return nil, errors.New("fence") }
			}
			cmd := lessonOccurrenceAddCommand()
			setLessonCommandFlags(t, cmd, map[string]string{"summary": "must retain recovery"})
			requireCLIError(t, runLessonOccurrenceAddWithDeps(cmd, []string{"review-before-merge"}, deps))
			prepared, err := event.NewOutbox(root).Prepared()
			if err != nil || len(prepared) != 1 {
				t.Fatalf("prepared recovery event = %#v, err=%v", prepared, err)
			}
		})
	}
}

func TestLessonNewFailureRetainsConcurrentIndexRowAndPreparedRecovery(t *testing.T) {
	root := setupSpecRoot(t)
	requireCLISuccess(t, projectdef.WriteSpecConfig(root, lessonTestConfig()))
	requireCLISuccess(t, ensureLessonAncestorIndexes(root))
	configureNoopLessonEvents(t, root)
	concurrent := "| [foreign](foreign/README.md) | Recorded | process | 0 |  | — |"
	deps := defaultLessonCLIDeps()
	deps.lint = func(lint.Options) ([]lint.Violation, error) {
		indexPath := filepath.Join(root, "spec", "lessons", "README.md")
		body, err := os.ReadFile(indexPath)
		requireCLISuccess(t, err)
		body = bytes.Replace(body, []byte("\n## Open Questions"), []byte("\n"+concurrent+"\n\n## Open Questions"), 1)
		requireCLISuccess(t, os.WriteFile(indexPath, body, 0o644))
		return nil, errors.New("late lint failure")
	}
	cmd := lessonNewCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"project": root})
	requireCLIError(t, runLessonNewWithDeps(cmd, []string{"retained-new"}, deps))
	index, err := os.ReadFile(filepath.Join(root, "spec", "lessons", "README.md"))
	requireCLISuccess(t, err)
	if !bytes.Contains(index, []byte(concurrent)) || !bytes.Contains(index, []byte("retained-new/README.md")) {
		t.Fatalf("post-publication recovery erased a row:\n%s", index)
	}
	if _, err := os.Stat(filepath.Join(root, "spec", "lessons", "retained-new", "README.md")); err != nil {
		t.Fatalf("published Lesson was destructively rolled back: %v", err)
	}
	prepared, err := event.NewOutbox(root).Prepared()
	if err != nil || len(prepared) != 1 || prepared[0].EventName != "lesson.created" {
		t.Fatalf("prepared recovery=%#v err=%v", prepared, err)
	}
}

func TestLessonNewCoordinatorDistinguishesPrepublicationAndFenceFailure(t *testing.T) {
	root := setupSpecRoot(t)
	requireCLISuccess(t, projectdef.WriteSpecConfig(root, lessonTestConfig()))
	requireCLISuccess(t, ensureLessonAncestorIndexes(root))
	configureNoopLessonEvents(t, root)
	deps := defaultLessonCLIDeps()
	deps.fs.mkdirAll = func(string, os.FileMode) error {
		return &lesson.MutationError{Outcome: lesson.MutationPrePublication, Err: errors.New("exclusive create failed")}
	}
	cmd := lessonNewCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"project": root})
	requireCLIError(t, runLessonNewWithDeps(cmd, []string{"prepublication"}, deps))
	assertAbortedLessonEvent(t, root, "lesson.created")

	root = setupSpecRoot(t)
	requireCLISuccess(t, projectdef.WriteSpecConfig(root, lessonTestConfig()))
	configureNoopLessonEvents(t, root)
	deps = defaultLessonCLIDeps()
	deps.durable.open = func(string) (durableFile, error) { return nil, errors.New("injected scaffold fence") }
	cmd = lessonNewCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"project": root})
	requireCLIError(t, runLessonNewWithDeps(cmd, []string{"fence-recovery"}, deps))
	prepared, err := event.NewOutbox(root).Prepared()
	if err != nil || len(prepared) != 1 {
		t.Fatalf("scaffold fence prepared recovery=%#v err=%v", prepared, err)
	}
}

func TestLessonLifecycleFailureRetainsConcurrentIndexRowAndDurabilityRecovery(t *testing.T) {
	root := canonicalLessonProject(t)
	configureNoopLessonEvents(t, root)
	concurrent := "| [foreign](foreign/README.md) | Recorded | process | 0 |  | — |"
	deps := defaultLessonCLIDeps()
	deps.lint = func(lint.Options) ([]lint.Violation, error) {
		indexPath := filepath.Join(root, "spec", "lessons", "README.md")
		body, err := os.ReadFile(indexPath)
		requireCLISuccess(t, err)
		body = bytes.Replace(body, []byte("\n## Open Questions"), []byte("\n"+concurrent+"\n\n## Open Questions"), 1)
		requireCLISuccess(t, os.WriteFile(indexPath, body, 0o644))
		return nil, errors.New("late lint failure")
	}
	cmd := lessonChangeStatusCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"project": root, "to": "Stated"})
	requireCLIError(t, runLessonChangeStatusWithDeps(cmd, []string{"review-before-merge"}, deps))
	index, err := os.ReadFile(filepath.Join(root, "spec", "lessons", "README.md"))
	requireCLISuccess(t, err)
	if !bytes.Contains(index, []byte(concurrent)) || !bytes.Contains(index, []byte("| Stated |")) {
		t.Fatalf("lifecycle recovery erased concurrent/owned row:\n%s", index)
	}
	prepared, err := event.NewOutbox(root).Prepared()
	if err != nil || len(prepared) != 1 || prepared[0].EventName != "lesson.lifecycle-changed" {
		t.Fatalf("prepared recovery=%#v err=%v", prepared, err)
	}

	// A durability-fence failure likewise cannot advance the event to committed.
	root = canonicalLessonProject(t)
	configureNoopLessonEvents(t, root)
	deps = defaultLessonCLIDeps()
	deps.durable.open = func(string) (durableFile, error) { return nil, errors.New("injected directory fence") }
	cmd = lessonChangeStatusCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"project": root, "to": "Stated"})
	requireCLIError(t, runLessonChangeStatusWithDeps(cmd, []string{"review-before-merge"}, deps))
	prepared, err = event.NewOutbox(root).Prepared()
	if err != nil || len(prepared) != 1 {
		t.Fatalf("fence failure prepared recovery=%#v err=%v", prepared, err)
	}
}

func TestLessonLifecycleLockOrderDoesNotDeadlockSharedIndexAndPreservesBothRows(t *testing.T) {
	root := setupSpecRoot(t)
	requireCLISuccess(t, projectdef.WriteSpecConfig(root, lessonTestConfig()))
	requireCLISuccess(t, ensureLessonAncestorIndexes(root))
	lessonsDir := filepath.Join(root, "spec", "lessons")
	for _, slug := range []string{"lock-a", "lock-b"} {
		path := filepath.Join(lessonsDir, slug, "README.md")
		requireCLISuccess(t, os.MkdirAll(filepath.Join(filepath.Dir(path), "occurrences"), 0o755))
		body, err := lesson.ScaffoldCanonical(lesson.ScaffoldOptions{Slug: slug, Owner: "tester", Date: "2026-08-11"}, []string{"process"})
		requireCLISuccess(t, err)
		requireCLISuccess(t, os.WriteFile(path, body, 0o644))
		parsed, err := lesson.Parse(path)
		requireCLISuccess(t, err)
		requireCLISuccess(t, lint.UpsertLessonIndexRow(filepath.Join(root, "spec"), parsed))
	}
	start := make(chan struct{})
	errs := make(chan error, 2)
	for _, slug := range []string{"lock-a", "lock-b"} {
		slug := slug
		go func() {
			<-start
			deps := defaultLessonCLIDeps()
			realLint := deps.lint
			deps.lint = func(opts lint.Options) ([]lint.Violation, error) {
				violations, err := realLint(opts)
				var owned []lint.Violation
				for _, violation := range violations {
					if strings.Contains(violation.File, slug) || strings.Contains(violation.Message, slug) {
						owned = append(owned, violation)
					}
				}
				return owned, err
			}
			hook, err := prepareLessonPostMutationWithDeps(root, slug, deps)
			if err == nil {
				_, err = lesson.ChangeStatus(lesson.ChangeStatusOptions{SpecRoot: root, Slug: slug, To: lifecycle.LessonStated, PostMutation: hook})
			}
			errs <- err
		}()
	}
	close(start)
	for i := 0; i < 2; i++ {
		select {
		case err := <-errs:
			if err != nil {
				t.Fatalf("concurrent lifecycle transition: %v", err)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("per-Lesson -> shared-index lock order deadlocked")
		}
	}
	index, err := os.ReadFile(filepath.Join(lessonsDir, "README.md"))
	requireCLISuccess(t, err)
	for _, slug := range []string{"lock-a", "lock-b"} {
		if !bytes.Contains(index, []byte("["+slug+"]("+slug+"/README.md) | Stated")) {
			t.Fatalf("concurrent transition lost %s row:\n%s", slug, index)
		}
	}
}

func TestLegacyRecurRestoresBodyAndIndexBytesAndModes(t *testing.T) {
	for _, phase := range []string{"parse", "index"} {
		t.Run(phase, func(t *testing.T) {
			lessonsDir := setupLessonsSpec(t)
			root := filepath.Dir(filepath.Dir(lessonsDir))
			writeLessonInDir(t, lessonsDir, "legacy", "Recorded")
			indexPath := filepath.Join(lessonsDir, "README.md")
			requireCLISuccess(t, os.WriteFile(indexPath, []byte("# Lessons\n\nlegacy index\n"), 0o600))
			configureNoopLessonEvents(t, root)
			indexInfo, _ := os.Stat(indexPath)
			beforeIndex, _ := os.ReadFile(indexPath)
			bodyPath := filepath.Join(lessonsDir, "legacy.md")
			bodyInfo, _ := os.Stat(bodyPath)
			deps := defaultLessonCLIDeps()
			if phase == "parse" {
				realParse, calls := deps.parse, 0
				deps.parse = func(path string) (*lesson.Lesson, error) {
					calls++
					if calls == 2 {
						return nil, errors.New("post-publication parse failure")
					}
					return realParse(path)
				}
			} else {
				deps.indexUpsert = func(string, *lesson.Lesson) error {
					requireCLISuccess(t, os.WriteFile(indexPath, []byte("mutated index\n"), 0o644))
					return errors.New("post-publication index failure")
				}
			}
			cmd := lessonRecurCommand()
			setLessonCommandFlags(t, cmd, map[string]string{"note": "seen again"})
			requireCLIError(t, runLessonRecurWithDeps(cmd, []string{"legacy"}, deps))
			bodyBytes, readErr := os.ReadFile(bodyPath)
			if readErr != nil || !bytes.Contains(bodyBytes, []byte("**Recurred:** 1")) {
				t.Fatalf("%s failure did not retain the published recurrence: %q, %v", phase, bodyBytes, readErr)
			}
			indexBytes, readErr := os.ReadFile(indexPath)
			if readErr != nil {
				t.Fatal(readErr)
			}
			if phase == "parse" && !bytes.Equal(indexBytes, beforeIndex) {
				t.Fatalf("parse failure changed index before its mutation: %q", indexBytes)
			}
			if phase == "index" && string(indexBytes) != "mutated index\n" {
				t.Fatalf("foreign index mutation was overwritten: %q", indexBytes)
			}
			gotBody, _ := os.Stat(bodyPath)
			gotIndex, _ := os.Stat(indexPath)
			if gotBody.Mode().Perm() != bodyInfo.Mode().Perm() {
				t.Fatalf("body mode changed: got=%o want=%o", gotBody.Mode().Perm(), bodyInfo.Mode().Perm())
			}
			if phase == "parse" && gotIndex.Mode().Perm() != indexInfo.Mode().Perm() {
				t.Fatalf("untouched index mode changed: got=%o want=%o", gotIndex.Mode().Perm(), indexInfo.Mode().Perm())
			}
			prepared, err := event.NewOutbox(root).Prepared()
			if err != nil || len(prepared) != 1 || prepared[0].EventName != "lesson.occurrence-recorded" {
				t.Fatalf("retained recurrence recovery event = %#v, err=%v", prepared, err)
			}
		})
	}
}

func TestFlatMigrationLateFailureRestoresCompleteTreeAndAbortsOutbox(t *testing.T) {
	for _, phase := range []string{"index", "lint"} {
		t.Run(phase, func(t *testing.T) {
			root := setupFlatMigrationCLIProject(t, "late-failure")
			configureNoopLessonEvents(t, root)
			lessonsDir := filepath.Join(root, "spec", "lessons")
			requireCLISuccess(t, os.Chmod(filepath.Join(lessonsDir, "late-failure.md"), 0o600))
			requireCLISuccess(t, os.Chmod(filepath.Join(lessonsDir, "README.md"), 0o640))
			outboxRoot := event.NewOutbox(root).Root
			if _, err := os.Stat(outboxRoot); !os.IsNotExist(err) {
				t.Fatalf("unexpected preexisting outbox: %v", err)
			}
			deps := defaultLessonCLIDeps()
			if phase == "index" {
				deps.indexUpsert = func(specRoot string, l *lesson.Lesson) error {
					return errors.New("late index failure")
				}
			} else {
				deps.lint = func(lint.Options) ([]lint.Violation, error) { return nil, errors.New("late lint failure") }
			}
			if _, _, err := runFlatMigrationWithDeps(t, root, "late-failure", deps); err == nil {
				t.Fatal("late migration failure was accepted")
			} else if !strings.Contains(err.Error(), "late ") {
				t.Fatalf("migration failed before the injected late boundary: %v", err)
			}
			marker := filepath.Join(lessonsDir, ".flat-migration-late-failure.json")
			if _, err := os.Stat(marker); err != nil {
				t.Fatalf("%s failure lost durable migration marker: %v", phase, err)
			}
			if _, err := os.Stat(filepath.Join(lessonsDir, "late-failure", "README.md")); err != nil {
				t.Fatalf("%s failure lost published canonical Lesson: %v", phase, err)
			}
			prepared, err := event.NewOutbox(root).Prepared()
			if err != nil || len(prepared) != 1 || prepared[0].EventName != "lesson.flat-migrated" {
				t.Fatalf("%s retained recovery event = %#v, err=%v", phase, prepared, err)
			}
			if _, _, err := runFlatMigrationWithDeps(t, root, "late-failure", defaultLessonCLIDeps()); err != nil {
				t.Fatalf("%s clean retry did not finish retained transaction: %v", phase, err)
			}
		})
	}
}

func TestFlatMigrationRollbackConflictRetainsPreparedEvent(t *testing.T) {
	for _, timing := range []string{"before-ownership-snapshot", "before-rollback"} {
		t.Run(timing, func(t *testing.T) {
			root := setupFlatMigrationCLIProject(t, "rollback-conflict")
			configureNoopLessonEvents(t, root)
			deps := defaultLessonCLIDeps()
			mutate := func() {
				path := filepath.Join(root, "spec", "lessons", "rollback-conflict", "README.md")
				b, err := os.ReadFile(path)
				requireCLISuccess(t, err)
				requireCLISuccess(t, os.WriteFile(path, append(b, []byte("\nconcurrent status write\n")...), 0o644))
			}
			if timing == "before-ownership-snapshot" {
				deps.afterFlatPhase = func(phase string) error {
					if phase == "artifact-publication" {
						mutate()
						return errors.New("injected crash after foreign mutation")
					}
					return nil
				}
			} else {
				deps.lint = func(lint.Options) ([]lint.Violation, error) { mutate(); return nil, errors.New("late lint failure") }
			}
			if _, _, err := runFlatMigrationWithDeps(t, root, "rollback-conflict", deps); err == nil {
				t.Fatal("rollback conflict was not reported")
			}
			prepared, err := event.NewOutbox(root).Prepared()
			if err != nil || len(prepared) != 1 || prepared[0].EventName != "lesson.flat-migrated" {
				t.Fatalf("prepared recovery event = %#v, err=%v", prepared, err)
			}
			if _, err := os.Stat(filepath.Join(root, "spec", "lessons", "rollback-conflict", "README.md")); err != nil {
				t.Fatalf("conflicting canonical tree was deleted: %v", err)
			}
		})
	}
}

func TestFlatMigrationRollbackDurableBoundaryFailuresRetainPrepared(t *testing.T) {
	for _, phase := range []string{"canonical-remove", "manifest-remove", "legacy-dir-remove", "flat-restore", "index-restore", "marker-remove"} {
		t.Run(phase, func(t *testing.T) {
			root := setupFlatMigrationCLIProject(t, "durable-fault")
			configureNoopLessonEvents(t, root)
			deps := defaultLessonCLIDeps()
			deps.afterFlatPhase = func(boundary string) error {
				if boundary == "artifact-publication" {
					return errors.New(phase)
				}
				return nil
			}
			if _, _, err := runFlatMigrationWithDeps(t, root, "durable-fault", deps); err == nil {
				t.Fatal("durable rollback failure was accepted")
			}
			prepared, err := event.NewOutbox(root).Prepared()
			if err != nil || len(prepared) != 1 {
				t.Fatalf("prepared recovery event = %#v, err=%v", prepared, err)
			}
			if _, _, err := runFlatMigrationWithDeps(t, root, "durable-fault", defaultLessonCLIDeps()); err != nil {
				t.Fatalf("retry after %s did not resume: %v", phase, err)
			}
		})
	}

	t.Run("missing-index-remove", func(t *testing.T) {
		root := setupFlatMigrationCLIProject(t, "durable-fault")
		configureNoopLessonEvents(t, root)
		indexPath := filepath.Join(root, "spec", "lessons", "README.md")
		requireCLISuccess(t, os.Remove(indexPath))
		deps := defaultLessonCLIDeps()
		deps.indexUpsert = func(string, *lesson.Lesson) error { return errors.New("index") }
		if _, _, err := runFlatMigrationWithDeps(t, root, "durable-fault", deps); err == nil {
			t.Fatal("missing-index rollback failure was accepted")
		}
	})
}

func TestFlatMigrationRollbackOwnershipAdapterEdges(t *testing.T) {
	for _, phase := range []string{"legacy-dir-stat", "published-tree", "index-snapshot", "rollback-tree", "manifest-conflict", "flat-reappeared", "flat-stat"} {
		t.Run(phase, func(t *testing.T) {
			root := setupFlatMigrationCLIProject(t, "ownership-edge")
			configureNoopLessonEvents(t, root)
			deps := defaultLessonCLIDeps()
			lessonsDir := filepath.Join(root, "spec", "lessons")
			realStat, realRead, realReadDir := deps.fs.stat, deps.fs.read, deps.fs.readDir
			active, indexStats := false, 0
			switch phase {
			case "legacy-dir-stat":
				deps.fs.stat = func(path string) (os.FileInfo, error) {
					if filepath.Base(path) == ".legacy-import" {
						return nil, errors.New("legacy dir stat")
					}
					return realStat(path)
				}
			case "published-tree":
				deps.fs.readDir = func(string) ([]os.DirEntry, error) { return nil, errors.New("tree read") }
			case "index-snapshot":
				deps.fs.stat = func(path string) (os.FileInfo, error) {
					if path == filepath.Join(lessonsDir, "README.md") {
						indexStats++
						if indexStats > 1 {
							return nil, errors.New("index snapshot")
						}
					}
					return realStat(path)
				}
			case "rollback-tree":
				deps.fs.readDir = func(path string) ([]os.DirEntry, error) {
					if active {
						return nil, errors.New("rollback tree")
					}
					return realReadDir(path)
				}
			case "flat-stat":
				deps.fs.stat = func(path string) (os.FileInfo, error) {
					if active && path == filepath.Join(lessonsDir, "ownership-edge.md") {
						return nil, errors.New("flat stat")
					}
					return realStat(path)
				}
			}
			deps.lint = func(lint.Options) ([]lint.Violation, error) {
				active = true
				switch phase {
				case "manifest-conflict":
					entries, err := os.ReadDir(filepath.Join(lessonsDir, ".legacy-import"))
					requireCLISuccess(t, err)
					path := filepath.Join(lessonsDir, ".legacy-import", entries[0].Name())
					b, err := realRead(path)
					requireCLISuccess(t, err)
					requireCLISuccess(t, os.WriteFile(path, append(b, '\n'), 0o644))
				case "flat-reappeared":
					requireCLISuccess(t, os.WriteFile(filepath.Join(lessonsDir, "ownership-edge.md"), []byte("concurrent"), 0o644))
				}
				return nil, errors.New("late lint")
			}
			if _, _, err := runFlatMigrationWithDeps(t, root, "ownership-edge", deps); err == nil {
				t.Fatal("ownership failure was accepted")
			}
		})
	}
}

func TestFlatMigrationOwnershipHelpersFailClosed(t *testing.T) {
	root := setupFlatMigrationCLIProject(t, "owned")
	foreign := []byte("foreign concurrent occurrence\\n")
	deps := defaultLessonCLIDeps()
	deps.indexUpsert = func(string, *lesson.Lesson) error {
		path := filepath.Join(root, "spec", "lessons", "owned", "occurrences", "foreign.json")
		if err := os.WriteFile(path, foreign, 0o644); err != nil {
			t.Fatal(err)
		}
		return errors.New("injected index failure after foreign publication")
	}
	if _, _, err := runFlatMigrationWithDeps(t, root, "owned", deps); err == nil {
		t.Fatal("post-publication failure was accepted")
	}
	foreignPath := filepath.Join(root, "spec", "lessons", "owned", "occurrences", "foreign.json")
	if got, err := os.ReadFile(foreignPath); err != nil || !bytes.Equal(got, foreign) {
		t.Fatalf("foreign occurrence was changed: %q err=%v", got, err)
	}
	if _, err := os.Stat(filepath.Join(root, "spec", "lessons", ".flat-migration-owned.json")); err != nil {
		t.Fatalf("durable recovery marker missing: %v", err)
	}
	prepared, err := event.NewOutbox(root).Prepared()
	if err != nil || len(prepared) != 1 {
		t.Fatalf("prepared recovery event = %#v err=%v", prepared, err)
	}
}

type faultDurableFile struct{ role, fail string }

func (f *faultDurableFile) Chmod(os.FileMode) error { return f.at("chmod") }
func (f *faultDurableFile) Write([]byte) (int, error) {
	if err := f.at("write"); err != nil {
		return 0, err
	}
	return 1, nil
}
func (f *faultDurableFile) Sync() error  { return f.at(f.role + "-sync") }
func (f *faultDurableFile) Close() error { return f.at(f.role + "-close") }
func (f *faultDurableFile) at(operation string) error {
	if f.fail == operation {
		return errors.New(operation)
	}
	return nil
}

func faultDurableOps(fail string) durableFileOps {
	return durableFileOps{
		open: func(string) (durableFile, error) {
			if fail == "open-dir" {
				return nil, errors.New(fail)
			}
			return &faultDurableFile{role: "dir", fail: fail}, nil
		},
	}
}

func TestDurableFileFaultsAreReportedWithoutGlobalHooks(t *testing.T) {
	for _, fault := range []string{"open-dir", "dir-sync", "dir-close"} {
		requireCLIError(t, durableFencePathWithOps("/x/file", faultDurableOps(fault)))
	}
}

func TestCanonicalCompensationFailureRetainsPreparedEvent(t *testing.T) {
	root := canonicalLessonProject(t)
	configureNoopLessonEvents(t, root)
	deps := defaultLessonCLIDeps()
	deps.indexUpsert = func(string, *lesson.Lesson) error { return errors.New("index failure") }
	cmd := lessonOccurrenceAddCommand()
	setLessonCommandFlags(t, cmd, nil)
	err := runLessonOccurrenceAddWithDeps(cmd, []string{"review-before-merge"}, deps)
	if err == nil || !strings.Contains(err.Error(), "recovery required: prepared event") {
		t.Fatalf("uncertain retained publication = %v", err)
	}
	prepared, readErr := event.NewOutbox(root).Prepared()
	if readErr != nil || len(prepared) != 1 {
		t.Fatalf("prepared recovery record = %#v, err=%v", prepared, readErr)
	}
}
