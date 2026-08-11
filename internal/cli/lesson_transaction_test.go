package cli

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/event"
	"github.com/specscore/specscore-cli/pkg/lesson"
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
	path := filepath.Join(root, "specscore.yaml")
	body, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		requireCLISuccess(t, projectdef.WriteSpecConfig(root, lessonTestConfig()))
		body, err = os.ReadFile(path)
	}
	requireCLISuccess(t, err)
	body = append(body, []byte("\nevents:\n  subscribers:\n    - type: noop\n")...)
	requireCLISuccess(t, os.WriteFile(path, body, 0o644))
}

func configureFailingLessonEvents(t *testing.T, root string) {
	t.Helper()
	path := filepath.Join(root, "specscore.yaml")
	body, err := os.ReadFile(path)
	requireCLISuccess(t, err)
	body = append(body, []byte("\nevents:\n  subscribers:\n    - type: exec\n      command: [/bin/false]\n")...)
	requireCLISuccess(t, os.WriteFile(path, body, 0o644))
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
			before := treeDigestForCLI(t, lessonsDir)
			deps := defaultLessonCLIDeps()
			switch phase {
			case "index":
				deps.indexUpsert = func(specRoot string, l *lesson.Lesson) error {
					return errors.New("late index failure")
				}
				cmd := lessonOccurrenceAddCommand()
				setLessonCommandFlags(t, cmd, map[string]string{"summary": "must roll back"})
				requireCLIError(t, runLessonOccurrenceAddWithDeps(cmd, []string{"review-before-merge"}, deps))
			case "discovery":
				deps.discoverOccurrences = func(string) ([]lesson.Occurrence, error) { return nil, errors.New("late discovery failure") }
				cmd := lessonRecurCommand()
				setLessonCommandFlags(t, cmd, map[string]string{"note": "must roll back"})
				requireCLIError(t, runLessonRecurWithDeps(cmd, []string{"review-before-merge"}, deps))
			}
			if after := treeDigestForCLI(t, lessonsDir); !bytes.Equal(after, before) {
				t.Fatalf("%s failure changed the complete Lesson tree\nbefore=%q\nafter=%q", phase, before, after)
			}
			assertAbortedLessonEvent(t, root, "lesson.occurrence-recorded")
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
	setLessonCommandFlags(t, cmd, map[string]string{"note": "must roll back"})
	requireCLIError(t, runLessonRecurWithDeps(cmd, []string{"review-before-merge"}, deps))
	index, err := os.ReadFile(indexPath)
	requireCLISuccess(t, err)
	if !bytes.Contains(index, []byte(concurrent)) || !bytes.Contains(index, []byte("| 0 |")) {
		t.Fatalf("owned-row inverse lost concurrent data or retained occurrence count:\n%s", index)
	}
	assertAbortedLessonEvent(t, root, "lesson.occurrence-recorded")
}

func TestCanonicalOccurrenceReconcileFailureRetainsPreparedEvent(t *testing.T) {
	for _, phase := range []string{"reconcile", "fence"} {
		t.Run(phase, func(t *testing.T) {
			root := canonicalLessonProject(t)
			deps := defaultLessonCLIDeps()
			deps.indexUpsert = func(string, *lesson.Lesson) error { return errors.New("index") }
			if phase == "reconcile" {
				deps.reconcileIndex = func(string, *lesson.Lesson) error { return errors.New("reconcile") }
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

func TestLegacyRecurRestoresBodyAndIndexBytesAndModes(t *testing.T) {
	for _, phase := range []string{"parse", "index"} {
		t.Run(phase, func(t *testing.T) {
			lessonsDir := setupLessonsSpec(t)
			root := filepath.Dir(filepath.Dir(lessonsDir))
			writeLessonInDir(t, lessonsDir, "legacy", "Recorded")
			indexPath := filepath.Join(lessonsDir, "README.md")
			requireCLISuccess(t, os.WriteFile(indexPath, []byte("# Lessons\n\nlegacy index\n"), 0o600))
			configureNoopLessonEvents(t, root)
			before := treeDigestForCLI(t, lessonsDir)
			indexInfo, _ := os.Stat(indexPath)
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
			if after := treeDigestForCLI(t, lessonsDir); !bytes.Equal(after, before) {
				t.Fatalf("%s failure changed body/index bytes\nbefore=%q\nafter=%q", phase, before, after)
			}
			gotBody, _ := os.Stat(bodyPath)
			gotIndex, _ := os.Stat(indexPath)
			if gotBody.Mode().Perm() != bodyInfo.Mode().Perm() || gotIndex.Mode().Perm() != indexInfo.Mode().Perm() {
				t.Fatalf("modes changed: body=%o index=%o", gotBody.Mode().Perm(), gotIndex.Mode().Perm())
			}
			assertAbortedLessonEvent(t, root, "lesson.occurrence-recorded")
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
			beforeLessons := treeDigestForCLI(t, lessonsDir)
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
			if after := treeDigestForCLI(t, lessonsDir); !bytes.Equal(after, beforeLessons) {
				t.Fatalf("%s failure changed flat/canonical/index/marker/manifest tree\nbefore=%q\nafter=%q", phase, beforeLessons, after)
			}
			assertAbortedLessonEvent(t, root, "lesson.flat-migrated")
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
			deps.parse = func(string) (*lesson.Lesson, error) { return nil, errors.New("late parse failure") }
			base := deps.durable
			switch phase {
			case "canonical-remove", "manifest-remove", "legacy-dir-remove", "marker-remove":
				realRemove := base.remove
				base.remove = func(path string) error {
					canonical := strings.Contains(path, string(filepath.Separator)+"durable-fault"+string(filepath.Separator))
					manifest := strings.Contains(path, string(filepath.Separator)+".legacy-import"+string(filepath.Separator)+"flat-")
					legacyDir := filepath.Base(path) == ".legacy-import"
					marker := strings.HasPrefix(filepath.Base(path), ".flat-migration-")
					if phase == "canonical-remove" && canonical || phase == "manifest-remove" && manifest || phase == "legacy-dir-remove" && legacyDir || phase == "marker-remove" && marker {
						return errors.New(phase)
					}
					return realRemove(path)
				}
			case "flat-restore", "index-restore":
				realOpen := base.openFile
				base.openFile = func(path string, flags int, mode os.FileMode) (durableFile, error) {
					flat := filepath.Base(path) == "durable-fault.md"
					index := filepath.Base(path) == "README.md" && filepath.Base(filepath.Dir(path)) == "lessons"
					if phase == "flat-restore" && flat || phase == "index-restore" && index {
						return nil, errors.New(phase)
					}
					return realOpen(path, flags, mode)
				}
			}
			deps.durable = base
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
		base, realRemove := deps.durable, deps.durable.remove
		base.remove = func(path string) error {
			if path == indexPath {
				return errors.New("index remove")
			}
			return realRemove(path)
		}
		deps.durable = base
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
	boom := errors.New("adapter")
	if err := removePathIfExistsWith("owned", func(string) error { return boom }); !errors.Is(err, boom) {
		t.Fatalf("remove error = %v", err)
	}
	dir, file := t.TempDir(), filepath.Join(t.TempDir(), "file")
	requireCLISuccess(t, os.WriteFile(file, []byte("x"), 0o600))
	if exists, err := pathExistsAsDirectory(dir, os.Stat); err != nil || !exists {
		t.Fatalf("directory existence = %v, %v", exists, err)
	}
	if _, err := pathExistsAsDirectory(file, os.Stat); err == nil {
		t.Fatal("regular file accepted as directory")
	}

	base := defaultLessonCLIDeps().fs
	for _, phase := range []string{"root-stat", "file-read", "child-stat", "verify-stat"} {
		t.Run(phase, func(t *testing.T) {
			fs := base
			switch phase {
			case "root-stat":
				fs.stat = func(string) (os.FileInfo, error) { return nil, boom }
				if _, err := snapshotRollbackTree(dir, fs); !errors.Is(err, boom) {
					t.Fatalf("tree stat error = %v", err)
				}
			case "file-read":
				fs.read = func(string) ([]byte, error) { return nil, boom }
				if _, err := snapshotRollbackTree(file, fs); !errors.Is(err, boom) {
					t.Fatalf("tree read error = %v", err)
				}
			case "child-stat":
				realStat := fs.stat
				fs.stat = func(path string) (os.FileInfo, error) {
					if path != dir {
						return nil, boom
					}
					return realStat(path)
				}
				child := filepath.Join(dir, "child")
				requireCLISuccess(t, os.WriteFile(child, []byte("x"), 0o600))
				if _, err := snapshotRollbackTree(dir, fs); !errors.Is(err, boom) {
					t.Fatalf("child stat error = %v", err)
				}
			case "verify-stat":
				fs.stat = func(string) (os.FileInfo, error) { return nil, boom }
				if err := verifyRollbackFile(rollbackFile{path: file, existed: true}, fs); !errors.Is(err, boom) {
					t.Fatalf("verify stat error = %v", err)
				}
			}
		})
	}

	lessonsDir := filepath.Join(t.TempDir(), "lessons")
	canonicalRoot := filepath.Join(lessonsDir, "owned")
	readme, occurrence, manifestData := []byte("readme"), []byte("occurrence"), []byte("manifest")
	hash := func(data []byte) string { return fmt.Sprintf("%x", sha256.Sum256(data)) }
	markerFor := func(extra ...map[string]string) []byte {
		files := []map[string]string{{"path": "owned/README.md", "sha256": hash(readme)}, {"path": "owned/occurrences/id.json", "sha256": hash(occurrence)}, {"path": ".legacy-import/owned.json", "sha256": hash(manifestData)}}
		b, _ := json.Marshal(map[string]any{"files": append(files, extra...)})
		return b
	}
	baseTree := []rollbackTreeEntry{{path: canonicalRoot, dir: true}, {path: filepath.Join(canonicalRoot, "README.md"), data: readme}, {path: filepath.Join(canonicalRoot, "occurrences"), dir: true}, {path: filepath.Join(canonicalRoot, "occurrences", "id.json"), data: occurrence}}
	result := lesson.FlatMigrationResult{CanonicalPath: filepath.Join(canonicalRoot, "README.md")}
	manifest := rollbackFile{path: filepath.Join(lessonsDir, ".legacy-import", "owned.json"), data: manifestData, existed: true}
	for _, tc := range []struct {
		name     string
		marker   []byte
		tree     []rollbackTreeEntry
		manifest rollbackFile
	}{
		{name: "malformed-marker", marker: []byte("{")},
		{name: "unsafe-marker-path", marker: markerFor(map[string]string{"path": "../outside", "sha256": "x"})},
		{name: "unexpected-directory", marker: markerFor(), tree: append(append([]rollbackTreeEntry{}, baseTree...), rollbackTreeEntry{path: filepath.Join(canonicalRoot, "other"), dir: true})},
		{name: "changed-manifest", marker: markerFor(), tree: baseTree, manifest: rollbackFile{path: manifest.path, data: []byte("changed"), existed: true}},
		{name: "unpublished-marker-path", marker: markerFor(map[string]string{"path": "owned/missing", "sha256": "x"}), tree: baseTree, manifest: manifest},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tree, ownedManifest := tc.tree, tc.manifest
			if tree == nil {
				tree = baseTree
			}
			if ownedManifest.path == "" {
				ownedManifest = manifest
			}
			if err := validateFlatMigrationOwnership(lessonsDir, result, tc.marker, tree, ownedManifest); err == nil {
				t.Fatal("invalid migration ownership was accepted")
			}
		})
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
		openFile: func(string, int, os.FileMode) (durableFile, error) {
			if fail == "open-file" {
				return nil, errors.New(fail)
			}
			return &faultDurableFile{role: "file", fail: fail}, nil
		},
		open: func(string) (durableFile, error) {
			if fail == "open-dir" {
				return nil, errors.New(fail)
			}
			return &faultDurableFile{role: "dir", fail: fail}, nil
		},
		remove: func(string) error {
			switch fail {
			case "remove":
				return errors.New(fail)
			case "remove-missing":
				return fs.ErrNotExist
			default:
				return nil
			}
		},
	}
}

func TestDurableFileFaultsAreReportedWithoutGlobalHooks(t *testing.T) {
	for _, fault := range []string{"open-file", "chmod", "write", "file-sync", "file-close", "open-dir", "dir-sync"} {
		t.Run("restore-"+fault, func(t *testing.T) {
			requireCLIError(t, durableRestoreFileWithOps("/x/file", []byte("x"), 0o600, faultDurableOps(fault)))
		})
	}
	for _, fault := range []string{"remove", "open-dir", "dir-sync"} {
		t.Run("remove-"+fault, func(t *testing.T) {
			requireCLIError(t, durableRemovePathWithOps("/x/file", faultDurableOps(fault)))
		})
	}
	if err := durableRemovePathWithOps("/x/missing", faultDurableOps("remove-missing")); err != nil {
		t.Fatalf("missing remove should still fence its directory: %v", err)
	}
	removed := filepath.Join(t.TempDir(), "remove-me")
	requireCLISuccess(t, os.WriteFile(removed, []byte("x"), 0o600))
	requireCLISuccess(t, durableRemovePath(removed))
	for _, fault := range []string{"dir-sync", "dir-close"} {
		requireCLIError(t, durableFencePathWithOps("/x/file", faultDurableOps(fault)))
	}
}

func TestRestoreLegacyRecurWithoutIndex(t *testing.T) {
	dir := t.TempDir()
	body := filepath.Join(dir, "lesson.md")
	requireCLISuccess(t, restoreLegacyRecurFiles(body, []byte("restored"), 0o600, filepath.Join(dir, "index.md"), nil, 0, false))
	if got, _ := os.ReadFile(body); string(got) != "restored" {
		t.Fatalf("restored body = %q", got)
	}
}

func TestCanonicalCompensationFailureRetainsPreparedEvent(t *testing.T) {
	root := canonicalLessonProject(t)
	configureNoopLessonEvents(t, root)
	deps := defaultLessonCLIDeps()
	deps.indexUpsert = func(string, *lesson.Lesson) error { return errors.New("index failure") }
	deps.removeOccurrence = func(string) error { return errors.New("remove failure") }
	cmd := lessonOccurrenceAddCommand()
	setLessonCommandFlags(t, cmd, nil)
	err := runLessonOccurrenceAddWithDeps(cmd, []string{"review-before-merge"}, deps)
	if err == nil || !strings.Contains(err.Error(), "recovery required: prepared event") {
		t.Fatalf("uncertain compensation = %v", err)
	}
	prepared, readErr := event.NewOutbox(root).Prepared()
	if readErr != nil || len(prepared) != 1 {
		t.Fatalf("prepared recovery record = %#v, err=%v", prepared, readErr)
	}
}
