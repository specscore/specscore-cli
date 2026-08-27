package cli

import (
	"bytes"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
	"github.com/specscore/specscore-cli/pkg/plan"
	"github.com/specscore/specscore-cli/pkg/task"
)

func TestTaskChangeStatusInstanceFaultMapping(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		code int
	}{
		{"missing-status", lifecycle.ErrStatusLineNotFound, exitcode.Unexpected},
		{"missing-file", os.ErrNotExist, exitcode.NotFound},
		{"runtime", errors.New("write failed"), exitcode.Unexpected},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, path := stageTaskWithStatus(t, "auth", "planning")
			before, _ := os.ReadFile(path)
			deps := taskMutationDeps{rewriteBoardTask: func(string, lifecycle.Status, []string) (lifecycle.Status, error) { return "", tc.err }}
			_, _, err := runTaskWithMutationDeps(t, deps, "change-status", "auth", "--to=queued")
			if exitCodeOfErr(err) != tc.code {
				t.Fatalf("err=%v", err)
			}
			if after, _ := os.ReadFile(path); !bytes.Equal(after, before) {
				t.Fatal("fault mapping mutated task")
			}
		})
	}
}

func TestPlanInlineTaskPureTransformFaultMapping(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		code int
	}{
		{"concurrent", lifecycle.ErrConcurrentMutation, exitcode.Conflict},
		{"runtime", errors.New("pure transform failed"), exitcode.Unexpected},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, path := stagePlanWithTasks(t, "auth", twoTaskPlanBody)
			before, _ := os.ReadFile(path)
			deps := taskMutationDeps{rewritePlanTaskStatus: func([]byte, int, lifecycle.Status, []string) ([]byte, error) { return nil, tc.err }}
			_, _, err := runTaskWithMutationDeps(t, deps, "change-status", "setup", "--plan=auth", "--to=complete")
			if exitCodeOfErr(err) != tc.code {
				t.Fatalf("err=%v", err)
			}
			if after, _ := os.ReadFile(path); !bytes.Equal(after, before) {
				t.Fatal("pure transform fault mutated Plan")
			}
		})
	}
}

func TestPlanInlineProvenancePureTransformFaultMapping(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		code int
	}{
		{"not-found", os.ErrNotExist, exitcode.NotFound},
		{"runtime", errors.New("provenance transform failed"), exitcode.Unexpected},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, path := stagePlanWithTasks(t, "auth", completeTaskPlanBody)
			before, _ := os.ReadFile(path)
			deps := taskMutationDeps{amendPlanProvenance: func([]byte, int, string) ([]byte, error) { return nil, tc.err }}
			_, _, err := runTaskWithMutationDeps(t, deps, "change-status", "setup", "--plan=auth", "--amend-provenance", "--commit=abc1234")
			if exitCodeOfErr(err) != tc.code {
				t.Fatalf("err=%v", err)
			}
			if after, _ := os.ReadFile(path); !bytes.Equal(after, before) {
				t.Fatal("provenance transform fault mutated Plan")
			}
		})
	}
}

func TestTaskFieldTransformsNoOpAndStaleCoordinates(t *testing.T) {
	lines := []string{"**Status:** planning", "body"}
	if got := withExtraFieldLines(lines, 0, nil); len(got) != 2 || got[1] != "body" {
		t.Fatalf("nil fields changed lines: %v", got)
	}
	if _, err := rewritePlanTaskStatusLineBytes([]byte("body\n"), 1, lifecycle.TaskComplete, nil); !errors.Is(err, lifecycle.ErrConcurrentMutation) {
		t.Fatalf("status coordinate err=%v", err)
	}
	if _, err := amendPlanImplementedByBytes([]byte("body\n"), 1, "repo@abc"); !errors.Is(err, lifecycle.ErrConcurrentMutation) {
		t.Fatalf("provenance coordinate err=%v", err)
	}
}

func TestTaskAmendResolutionAndArtifactFaultsAreWriteFree(t *testing.T) {
	t.Run("board-stat-error", func(t *testing.T) {
		root := setupTaskProjectForNew(t)
		loop := filepath.Join(root, "tasks", "loop")
		if err := os.Symlink(loop, loop); err != nil {
			t.Fatal(err)
		}
		_, _, err := runTask(t, "amend", "loop", "--project="+root, "--note=x", "--actor=a", "--reason=r")
		if exitCodeOfErr(err) != exitcode.Unexpected {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("plan-stat-error", func(t *testing.T) {
		root, path := stagePlanWithTasks(t, "auth", twoTaskPlanBody)
		if err := os.Remove(path); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(path, path); err != nil {
			t.Fatal(err)
		}
		_, _, err := runTask(t, "amend", "setup", "--plan=auth", "--project="+root, "--note=x", "--actor=a", "--reason=r")
		if exitCodeOfErr(err) != exitcode.Unexpected {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("oversized-plan-snapshot", func(t *testing.T) {
		_, path := stagePlanWithTasks(t, "auth", twoTaskPlanBody)
		before := bytes.Repeat([]byte("x"), (1<<20)+1)
		if err := os.WriteFile(path, before, 0o644); err != nil {
			t.Fatal(err)
		}
		_, _, err := runTask(t, "amend", "setup", "--plan=auth", "--note=x", "--actor=a", "--reason=r")
		if exitCodeOfErr(err) != exitcode.Unexpected || !strings.Contains(err.Error(), "token too long") {
			t.Fatalf("err=%v", err)
		}
		if after, _ := os.ReadFile(path); !bytes.Equal(after, before) {
			t.Fatal("malformed Plan was mutated")
		}
	})
	for _, tc := range []struct {
		name string
		err  error
		code int
	}{
		{"conflict", lifecycle.ErrConcurrentMutation, exitcode.Conflict},
		{"runtime", errors.New("transaction failed"), exitcode.Unexpected},
	} {
		t.Run("board-"+tc.name, func(t *testing.T) {
			_, path := stageTaskWithStatus(t, "auth", "blocked")
			before, _ := os.ReadFile(path)
			deps := taskMutationDeps{transformArtifact: func(string, func([]byte) ([]byte, error)) error { return tc.err }}
			_, _, err := runTaskWithMutationDeps(t, deps, "amend", "auth", "--note=x", "--actor=a", "--reason=r")
			if exitCodeOfErr(err) != tc.code {
				t.Fatalf("err=%v", err)
			}
			if after, _ := os.ReadFile(path); !bytes.Equal(after, before) {
				t.Fatal("fault mutated board task")
			}
		})
		t.Run("plan-"+tc.name, func(t *testing.T) {
			_, path := stagePlanWithTasks(t, "auth", twoTaskPlanBody)
			before, _ := os.ReadFile(path)
			deps := taskMutationDeps{transformArtifact: func(string, func([]byte) ([]byte, error)) error { return tc.err }}
			_, _, err := runTaskWithMutationDeps(t, deps, "amend", "setup", "--plan=auth", "--note=x", "--actor=a", "--reason=r")
			if exitCodeOfErr(err) != tc.code {
				t.Fatalf("err=%v", err)
			}
			if after, _ := os.ReadFile(path); !bytes.Equal(after, before) {
				t.Fatal("fault mutated Plan")
			}
		})
	}
}

func TestTaskAmendMalformedAnnotationAndAdjacentInsertion(t *testing.T) {
	_, path := stageTaskWithStatus(t, "auth", "blocked")
	bad := "# Auth\n\n**Status:** blocked\n**Note:**   \n"
	if err := os.WriteFile(path, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := runTask(t, "amend", "auth", "--note=x", "--actor=a", "--reason=r")
	if exitCodeOfErr(err) != exitcode.InvalidArgs || string(mustRead(path)) != bad {
		t.Fatalf("err=%v body=%q", err, mustRead(path))
	}
	seed := "# Auth\n\n**Status:** blocked\n**Implemented-by:** repo@abc\n**Evidence:** proof\n\nbody\n"
	if err := os.WriteFile(path, []byte(seed), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runTask(t, "amend", "auth", "--note=fixed", "--actor=a", "--reason=r"); err != nil {
		t.Fatal(err)
	}
	got := string(mustRead(path))
	if !strings.Contains(got, "**Implemented-by:** repo@abc\n**Evidence:** proof\n**Note:** fixed") {
		t.Fatalf("adjacent insertion wrong:\n%s", got)
	}
}

type closeFailYAML struct{ err error }

func (e closeFailYAML) Encode(any) error { return nil }
func (e closeFailYAML) Close() error     { return e.err }

type fakeSyncClose struct {
	syncErr  error
	closeErr error
}

func (f fakeSyncClose) Sync() error  { return f.syncErr }
func (f fakeSyncClose) Close() error { return f.closeErr }

func TestOwnedMarkerDirectoryFenceAndReadFailures(t *testing.T) {
	boom := errors.New("dir fence failed")
	if err := syncOwnedMarkerDir(filepath.Join(t.TempDir(), "missing")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("open err=%v", err)
	}
	if err := syncOwnedMarkerDirWithOpen("x", func(string) (syncCloseFile, error) { return fakeSyncClose{syncErr: boom}, nil }); !errors.Is(err, boom) {
		t.Fatalf("sync err=%v", err)
	}
	if err := syncOwnedMarkerDirWithOpen("x", func(string) (syncCloseFile, error) { return fakeSyncClose{closeErr: boom}, nil }); !errors.Is(err, boom) {
		t.Fatalf("close err=%v", err)
	}
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing.prepared")
	if err := removeOwnedFileDurable(missing, []byte("owned")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing receipts err=%v", err)
	}
	preparedDir := filepath.Join(dir, "prepared-dir")
	if err := os.Mkdir(preparedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := removeOwnedFileDurable(preparedDir, []byte("owned")); err == nil {
		t.Fatal("prepared receipt read should fail")
	}
	if err := os.Remove(preparedDir); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(committedMarkerPath(preparedDir), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := removeOwnedFileDurable(preparedDir, []byte("owned")); err == nil {
		t.Fatal("committed receipt read should fail")
	}
}

func TestTaskNewOutputCloseFailureRetainsRecovery(t *testing.T) {
	root := setupTaskProjectForNew(t)
	board, _, marker := taskNewPaths(root, "close-fail")
	boom := errors.New("yaml close failed")
	deps := taskMutationDeps{newYAMLEncoder: func(io.Writer) yamlEnc { return closeFailYAML{err: boom} }}
	err := executeTaskNewWithMutationDeps(root, "close-fail", "Close Fail", deps)
	if !errors.Is(err, boom) || boardRowCount(t, board, "close-fail") != 1 {
		t.Fatalf("err=%v", err)
	}
	if _, statErr := os.Stat(marker); statErr != nil {
		t.Fatalf("recovery marker missing: %v", statErr)
	}
}

func TestTaskNewDependencyAndMarkerIdentityFailures(t *testing.T) {
	rows := []task.BoardRow{{Task: "on-board"}}
	deps := defaultTaskMutationDeps()
	if err := validateTaskNewDependencies(t.TempDir(), "self", []string{"self"}, rows, deps); exitCodeOfErr(err) != exitcode.NotFound {
		t.Fatalf("self dependency err=%v", err)
	}
	if err := validateTaskNewDependencies(t.TempDir(), "new", []string{"on-board"}, rows, deps); err != nil {
		t.Fatalf("board dependency err=%v", err)
	}
	file := filepath.Join(t.TempDir(), "dep")
	if err := os.WriteFile(file, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validateTaskNewDependencies(filepath.Dir(file), "new", []string{"dep"}, nil, deps); exitCodeOfErr(err) != exitcode.NotFound {
		t.Fatalf("file dependency err=%v", err)
	}
	boom := errors.New("stat failed")
	if err := validateTaskNewDependencies(t.TempDir(), "new", []string{"dep"}, nil, taskMutationDeps{stat: func(string) (os.FileInfo, error) { return nil, boom }}.withDefaults()); !errors.Is(err, boom) || exitCodeOfErr(err) != exitcode.Unexpected {
		t.Fatalf("runtime dependency err=%v", err)
	}

	if taskNewRowMatches(nil, task.BoardRow{Task: "x"}) {
		t.Fatal("missing row matched")
	}
	base := task.BoardRow{Task: "x", Status: task.StatusPlanning, DependsOn: []string{"a"}}
	for _, row := range []task.BoardRow{
		{Task: "x", Status: task.StatusQueued, DependsOn: []string{"a"}},
		{Task: "x", Status: task.StatusPlanning, DependsOn: []string{"b"}},
		{Task: "x", Status: task.StatusPlanning, DependsOn: []string{"a"}, Branch: "foreign"},
	} {
		if taskNewRowMatches([]task.BoardRow{row}, base) {
			t.Fatalf("foreign row matched: %+v", row)
		}
	}
}

func TestReadTaskNewRecoveryMarkerReadFailures(t *testing.T) {
	dir := t.TempDir()
	prepared := filepath.Join(dir, "task.prepared")
	if err := os.Mkdir(prepared, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := readTaskNewRecoveryMarker(prepared); err == nil {
		t.Fatal("prepared directory read should fail")
	}
	if err := os.Remove(prepared); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(committedMarkerPath(prepared), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, _, err := readTaskNewRecoveryMarker(prepared); err == nil {
		t.Fatal("committed directory read should fail")
	}
}

func TestPlanCallbacksRejectOversizedLockedSnapshot(t *testing.T) {
	root := stagePlan(t, "auth", "Draft")
	path := filepath.Join(root, "spec", "plans", "auth.md")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), (1<<20)+1), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := runPlan(t, "change-status", "auth", "--to=in review")
	if exitCodeOfErr(err) != exitcode.Unexpected || !strings.Contains(err.Error(), "token too long") {
		t.Fatalf("change status err=%v", err)
	}

	root = stageReconcilablePlan(t, "reconcile", "Draft", "planning")
	path = filepath.Join(root, "spec", "plans", "reconcile.md")
	if err := os.WriteFile(path, bytes.Repeat([]byte("x"), (1<<20)+1), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err = runPlan(t, "reconcile", "reconcile", "--tasks=complete", "--note=x")
	if exitCodeOfErr(err) != exitcode.Unexpected || !strings.Contains(err.Error(), "token too long") {
		t.Fatalf("reconcile err=%v", err)
	}
}

func TestPlanNewFilesystemRaceAndForceFaults(t *testing.T) {
	t.Run("non-force-stat-error", func(t *testing.T) {
		root := setupSpecRoot(t)
		path := filepath.Join(root, "spec", "plans", "loop.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(path, path); err != nil {
			t.Fatal(err)
		}
		_, _, err := runPlan(t, "new", "loop", "--owner=tester", "--project="+root)
		if exitCodeOfErr(err) != exitcode.Unexpected {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("force-existing-transaction-read-error", func(t *testing.T) {
		root := setupSpecRoot(t)
		path := filepath.Join(root, "spec", "plans", "directory.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Mkdir(path, 0o755); err != nil {
			t.Fatal(err)
		}
		_, _, err := runPlan(t, "new", "directory", "--owner=tester", "--force", "--project="+root)
		if exitCodeOfErr(err) != exitcode.Unexpected {
			t.Fatalf("err=%v", err)
		}
	})
	for _, force := range []bool{false, true} {
		name := "non-force-publication-race"
		if force {
			name = "force-missing-publication-collision"
		}
		t.Run(name, func(t *testing.T) {
			root := setupSpecRoot(t)
			path := filepath.Join(root, "spec", "plans", "broken.md")
			if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.Symlink(filepath.Join(root, "absent"), path); err != nil {
				t.Fatal(err)
			}
			args := []string{"new", "broken", "--owner=tester", "--project=" + root}
			if force {
				args = append(args, "--force")
			}
			_, _, err := runPlan(t, args...)
			want := exitcode.Conflict
			if force {
				want = exitcode.Unexpected
			}
			if exitCodeOfErr(err) != want {
				t.Fatalf("err=%v", err)
			}
		})
	}
	t.Run("force-stat-error", func(t *testing.T) {
		root := setupSpecRoot(t)
		path := filepath.Join(root, "spec", "plans", "loop.md")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(path, path); err != nil {
			t.Fatal(err)
		}
		_, _, err := runPlan(t, "new", "loop", "--owner=tester", "--force", "--project="+root)
		if exitCodeOfErr(err) != exitcode.Unexpected {
			t.Fatalf("err=%v", err)
		}
	})
}

func TestBoardRewritePureFaultsLeaveOriginal(t *testing.T) {
	boom := errors.New("pure transform fault")
	for _, tc := range []struct {
		name    string
		fields  []string
		rewrite func([]byte, lifecycle.Status) ([]byte, string, error)
		add     func([]byte, []string) ([]byte, error)
	}{
		{"status", nil, func([]byte, lifecycle.Status) ([]byte, string, error) { return nil, "", boom }, boardExtraFieldsBytes},
		{"fields", []string{"**Note:** x"}, lifecycle.RewriteBytes, func([]byte, []string) ([]byte, error) { return nil, boom }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, path := stageTaskWithStatus(t, "auth", "planning")
			before, _ := os.ReadFile(path)
			_, err := rewriteBoardTaskWithTransforms(path, lifecycle.TaskQueued, tc.fields, tc.rewrite, tc.add)
			if !errors.Is(err, boom) {
				t.Fatalf("err=%v", err)
			}
			if after, _ := os.ReadFile(path); !bytes.Equal(after, before) {
				t.Fatal("pure fault mutated task")
			}
		})
	}
}

func TestPlanInlineMalformedSnapshotAndWrapperSuccess(t *testing.T) {
	_, path := stagePlanWithTasks(t, "auth", twoTaskPlanBody)
	before := bytes.Repeat([]byte("x"), (1<<20)+1)
	if err := os.WriteFile(path, before, 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := runTask(t, "change-status", "setup", "--plan=auth", "--to=complete")
	if exitCodeOfErr(err) != exitcode.Unexpected || !strings.Contains(err.Error(), "token too long") {
		t.Fatalf("status err=%v", err)
	}
	_, _, err = runTask(t, "change-status", "setup", "--plan=auth", "--amend-provenance", "--commit=abc1234")
	if exitCodeOfErr(err) != exitcode.Unexpected || !strings.Contains(err.Error(), "token too long") {
		t.Fatalf("provenance err=%v", err)
	}

	p := filepath.Join(t.TempDir(), "plan.md")
	if err := os.WriteFile(p, []byte("**Status:** planning\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := rewritePlanTaskStatusLine(p, 1, lifecycle.TaskQueued, nil); err != nil {
		t.Fatal(err)
	}
	if err := amendPlanImplementedBy(p, 1, "repo@abc"); err != nil {
		t.Fatal(err)
	}
	if got := string(mustRead(p)); !strings.Contains(got, "**Status:** queued\n**Implemented-by:** repo@abc") {
		t.Fatalf("wrapper output=%q", got)
	}
}

func TestTaskNewResidualIdentityAndFilesystemFaults(t *testing.T) {
	t.Run("invalid-dependency-slug", func(t *testing.T) {
		root := setupTaskProjectForNew(t)
		err := executeTaskNewWithDependenciesAndMutationDeps(root, "fresh", "../escape", taskMutationDeps{})
		if exitCodeOfErr(err) != exitcode.InvalidArgs {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("duplicate-board-rows", func(t *testing.T) {
		root := setupTaskProjectForNew(t)
		board := filepath.Join(root, "tasks", "README.md")
		b, _ := os.ReadFile(board)
		v, err := task.ParseBoard(b)
		if err != nil {
			t.Fatal(err)
		}
		v.Rows = append(v.Rows, task.BoardRow{Task: "dup", Status: task.StatusPlanning}, task.BoardRow{Task: "dup", Status: task.StatusPlanning})
		if err := os.WriteFile(board, task.RenderBoard(v), 0o644); err != nil {
			t.Fatal(err)
		}
		err = executeTaskNew(root, "dup", "Dup")
		if exitCodeOfErr(err) != exitcode.Conflict {
			t.Fatalf("err=%v", err)
		}
	})
	for _, tc := range []struct {
		name       string
		withMarker bool
		readme     bool
	}{
		{"fresh-task-dir-stat", false, false},
		{"prepared-task-dir-stat", true, false},
		{"fresh-readme-read", false, true},
		{"prepared-readme-read", true, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := setupTaskProjectForNew(t)
			_, readme, marker := taskNewPaths(root, "faulty")
			taskDir := filepath.Dir(readme)
			if tc.withMarker {
				markerData := taskNewPrepared{SchemaVersion: 2, TaskID: "faulty", TaskPath: "faulty/README.md", ContentSHA256: taskNewDigest(task.RenderTaskFile(task.TaskFileData{Title: "Faulty"}))}
				if err := os.WriteFile(marker, markerData.bytes(), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			if tc.readme {
				if err := os.Mkdir(taskDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.Symlink(readme, readme); err != nil {
					t.Fatal(err)
				}
			} else if err := os.Symlink(taskDir, taskDir); err != nil {
				t.Fatal(err)
			}
			err := executeTaskNew(root, "faulty", "Faulty")
			if exitCodeOfErr(err) != exitcode.Unexpected {
				t.Fatalf("err=%v", err)
			}
			var committed *lifecycle.CommittedMutationError
			if tc.withMarker && !errors.As(err, &committed) {
				t.Fatalf("prepared fault lost recovery type: %v", err)
			}
		})
	}
	t.Run("changed-board-preimage", func(t *testing.T) {
		root := setupTaskProjectForNew(t)
		boom := errors.New("commit failed")
		_ = executeTaskNewWithMutationDeps(root, "drift", "Drift", taskMutationDeps{commitBoard: func(*lifecycle.ArtifactTransaction, []byte) error { return boom }})
		board := filepath.Join(root, "tasks", "README.md")
		b, _ := os.ReadFile(board)
		if err := os.WriteFile(board, append(b, []byte("\n<!-- foreign -->\n")...), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := executeTaskNew(root, "drift", "Drift"); exitCodeOfErr(err) != exitcode.Conflict {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("changed-readme", func(t *testing.T) {
		root := setupTaskProjectForNew(t)
		boom := errors.New("commit failed")
		_ = executeTaskNewWithMutationDeps(root, "readme-drift", "Readme Drift", taskMutationDeps{commitBoard: func(*lifecycle.ArtifactTransaction, []byte) error { return boom }})
		_, readme, _ := taskNewPaths(root, "readme-drift")
		if err := os.WriteFile(readme, []byte("foreign"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := executeTaskNew(root, "readme-drift", "Readme Drift"); exitCodeOfErr(err) != exitcode.Conflict {
			t.Fatalf("err=%v", err)
		}
	})
	t.Run("foreign-marker-postimage", func(t *testing.T) {
		root := setupTaskProjectForNew(t)
		board := filepath.Join(root, "tasks", "README.md")
		boardBytes, _ := os.ReadFile(board)
		taskBytes := task.RenderTaskFile(task.TaskFileData{Title: "Foreign Post", Status: string(task.StatusPlanning)})
		_, _, marker := taskNewPaths(root, "foreign-post")
		m := taskNewPrepared{SchemaVersion: 2, TaskID: "foreign-post", TaskPath: "foreign-post/README.md", ContentSHA256: taskNewDigest(taskBytes), BoardBeforeSHA256: taskNewDigest(boardBytes), BoardAfterSHA256: "foreign"}
		if err := os.WriteFile(marker, m.bytes(), 0o600); err != nil {
			t.Fatal(err)
		}
		err := executeTaskNew(root, "foreign-post", "Foreign Post")
		var committed *lifecycle.CommittedMutationError
		if !errors.As(err, &committed) || !errors.Is(err, lifecycle.ErrConcurrentMutation) {
			t.Fatalf("err=%v", err)
		}
	})
	if taskNewRowMatches([]task.BoardRow{{Task: "other"}, {Task: "x", Status: task.StatusPlanning}}, task.BoardRow{Task: "x", Status: task.StatusPlanning, DependsOn: []string{"a"}}) {
		t.Fatal("dependency length mismatch matched")
	}
}

func TestTaskAmendResidualResolutionErrors(t *testing.T) {
	bare := t.TempDir()
	for _, args := range [][]string{
		{"amend", "auth", "--project=" + bare, "--note=x", "--actor=a", "--reason=r"},
		{"amend", "setup", "--plan=auth", "--project=" + bare, "--note=x", "--actor=a", "--reason=r"},
	} {
		_, _, err := runTask(t, args...)
		if err == nil {
			t.Fatalf("args=%v", args)
		}
	}
	root := setupSpecRoot(t)
	_, _, err := runTask(t, "amend", "setup", "--plan=ghost", "--project="+root, "--note=x", "--actor=a", "--reason=r")
	if exitCodeOfErr(err) != exitcode.NotFound {
		t.Fatalf("missing plan err=%v", err)
	}

	root, path := stagePlanWithTasks(t, "auth", twoTaskPlanBody)
	coordinated := strings.Replace(string(mustRead(path)), "**Source Feature:** auth", "**Source Feature:** auth\n**Coordination:** specscore/specscore-cli@main", 1)
	if err := os.WriteFile(path, []byte(coordinated), 0o644); err != nil {
		t.Fatal(err)
	}
	gitInitWithRemoteAndBranch(t, root, "https://github.com/specscore/specscore-cli.git", "other")
	_, _, err = runTask(t, "amend", "setup", "--plan=auth", "--note=x", "--actor=a", "--reason=r")
	if exitCodeOfErr(err) != exitcode.Conflict {
		t.Fatalf("coordination err=%v", err)
	}

	p, err := plan.ParseBytes("auth.md", []byte(strings.Replace(twoTaskPlanBody, "**Id:** setup", "**Id:** setup\n**Id:** setup", 1)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := uniquePlanTaskByID(p, "setup"); exitCodeOfErr(err) != exitcode.InvalidArgs {
		t.Fatalf("duplicate singleton err=%v", err)
	}
	p, err = plan.ParseBytes("auth.md", []byte(strings.Replace(twoTaskPlanBody, "**Id:** deploy", "**Id:** setup", 1)))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := uniquePlanTaskByID(p, "setup"); exitCodeOfErr(err) != exitcode.InvalidArgs {
		t.Fatalf("duplicate identity err=%v", err)
	}
}
