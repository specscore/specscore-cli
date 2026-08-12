package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
	"github.com/specscore/specscore-cli/pkg/task"
)

func executeTaskNew(root, slug, title string) error {
	cmd := taskCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"new", "--project", root, "--task", slug, "--title", title})
	return cmd.Execute()
}

func taskNewPaths(root, slug string) (board, readme, marker string) {
	tasksDir := filepath.Join(root, "tasks")
	return filepath.Join(tasksDir, "README.md"), filepath.Join(tasksDir, slug, "README.md"), taskNewMarkerPath(tasksDir, slug)
}

func boardRowCount(t *testing.T, board, slug string) int {
	t.Helper()
	b, err := os.ReadFile(board)
	if err != nil {
		t.Fatal(err)
	}
	v, err := task.ParseBoard(b)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, row := range v.Rows {
		if row.Task == slug {
			n++
		}
	}
	return n
}

func TestTaskNew_PreparedBoundaryRecoveryMatrix(t *testing.T) {
	t.Run("marker-publication-fails-before-visible-state", func(t *testing.T) {
		root := setupTaskProjectForNew(t)
		board, readme, marker := taskNewPaths(root, "bounded")
		boom := errors.New("marker publication failed")
		orig := taskNewPublishExclusiveFn
		taskNewPublishExclusiveFn = func(string, []byte, os.FileMode) error { return boom }
		t.Cleanup(func() { taskNewPublishExclusiveFn = orig })
		err := executeTaskNew(root, "bounded", "Bounded")
		if !errors.Is(err, boom) || exitCodeOfErr(err) != exitcode.Unexpected {
			t.Fatalf("err=%v", err)
		}
		if boardRowCount(t, board, "bounded") != 0 {
			t.Fatal("board row became visible before prepared marker")
		}
		for _, p := range []string{readme, marker} {
			if _, statErr := os.Stat(p); !os.IsNotExist(statErr) {
				t.Fatalf("unexpected visible path %s: %v", p, statErr)
			}
		}
	})

	t.Run("board-commit-failure-retries-exact-intent", func(t *testing.T) {
		root := setupTaskProjectForNew(t)
		board, readme, marker := taskNewPaths(root, "retry")
		boom := errors.New("board fence failed")
		orig := taskNewCommitBoardFn
		taskNewCommitBoardFn = func(*lifecycle.ArtifactTransaction, []byte) error { return boom }
		err := executeTaskNew(root, "retry", "Retry")
		taskNewCommitBoardFn = orig
		t.Cleanup(func() { taskNewCommitBoardFn = orig })
		if !errors.Is(err, boom) || boardRowCount(t, board, "retry") != 0 {
			t.Fatalf("first err=%v row=%d", err, boardRowCount(t, board, "retry"))
		}
		for _, p := range []string{readme, marker} {
			if _, statErr := os.Stat(p); statErr != nil {
				t.Fatalf("recovery path missing %s: %v", p, statErr)
			}
		}
		if err := executeTaskNew(root, "retry", "Retry"); err != nil {
			t.Fatalf("exact retry: %v", err)
		}
		if boardRowCount(t, board, "retry") != 1 {
			t.Fatal("exact retry did not commit exactly one row")
		}
		if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
			t.Fatalf("marker not finalized: %v", statErr)
		}
	})

	t.Run("different-intent-cannot-adopt", func(t *testing.T) {
		root := setupTaskProjectForNew(t)
		board, _, marker := taskNewPaths(root, "owned")
		boom := errors.New("board commit failed")
		orig := taskNewCommitBoardFn
		taskNewCommitBoardFn = func(*lifecycle.ArtifactTransaction, []byte) error { return boom }
		_ = executeTaskNew(root, "owned", "Original")
		taskNewCommitBoardFn = orig
		t.Cleanup(func() { taskNewCommitBoardFn = orig })
		beforeMarker, _ := os.ReadFile(marker)
		err := executeTaskNew(root, "owned", "Different")
		if exitCodeOfErr(err) != exitcode.Conflict || boardRowCount(t, board, "owned") != 0 {
			t.Fatalf("foreign adoption err=%v row=%d", err, boardRowCount(t, board, "owned"))
		}
		if got, _ := os.ReadFile(marker); !bytes.Equal(got, beforeMarker) {
			t.Fatal("conflicting retry replaced owned marker")
		}
	})

	t.Run("committed-row-finalizes-marker-idempotently", func(t *testing.T) {
		root := setupTaskProjectForNew(t)
		board, _, marker := taskNewPaths(root, "finalize")
		boom := errors.New("marker cleanup failed")
		orig := taskNewRemoveMarkerFn
		taskNewRemoveMarkerFn = func(string, []byte) error { return boom }
		err := executeTaskNew(root, "finalize", "Finalize")
		taskNewRemoveMarkerFn = orig
		t.Cleanup(func() { taskNewRemoveMarkerFn = orig })
		var committed *lifecycle.CommittedMutationError
		if !errors.As(err, &committed) || !errors.Is(err, boom) || boardRowCount(t, board, "finalize") != 1 {
			t.Fatalf("cleanup err=%v row=%d", err, boardRowCount(t, board, "finalize"))
		}
		if _, statErr := os.Stat(marker); statErr != nil {
			t.Fatalf("marker not retained: %v", statErr)
		}
		if err := executeTaskNew(root, "finalize", "Finalize"); err != nil {
			t.Fatalf("finalization retry: %v", err)
		}
		if boardRowCount(t, board, "finalize") != 1 {
			t.Fatal("finalization retry duplicated row")
		}
		if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
			t.Fatalf("marker still present: %v", statErr)
		}
	})

	t.Run("foreign-marker-replacement-is-never-deleted", func(t *testing.T) {
		root := setupTaskProjectForNew(t)
		board, _, marker := taskNewPaths(root, "foreign")
		foreign := []byte("foreign replacement\n")
		orig := taskNewRemoveMarkerFn
		taskNewRemoveMarkerFn = func(path string, expected []byte) error {
			if err := os.WriteFile(path, foreign, 0o600); err != nil {
				return err
			}
			return removeOwnedFileDurable(path, expected)
		}
		err := executeTaskNew(root, "foreign", "Foreign")
		taskNewRemoveMarkerFn = orig
		t.Cleanup(func() { taskNewRemoveMarkerFn = orig })
		var committed *lifecycle.CommittedMutationError
		if !errors.As(err, &committed) || boardRowCount(t, board, "foreign") != 1 {
			t.Fatalf("err=%v row=%d", err, boardRowCount(t, board, "foreign"))
		}
		if got, _ := os.ReadFile(marker); !bytes.Equal(got, foreign) {
			t.Fatalf("foreign marker was removed or changed: %q", got)
		}
		if err := executeTaskNew(root, "foreign", "Foreign"); exitCodeOfErr(err) != exitcode.Conflict {
			t.Fatalf("foreign replacement retry err=%v", err)
		}
	})
}

func TestTaskNew_FreshConflictsLeaveNoMarker(t *testing.T) {
	for _, tc := range []struct {
		name    string
		prepare func(root, slug string)
	}{
		{"readme", func(root, slug string) {
			_, readme, _ := taskNewPaths(root, slug)
			if err := os.MkdirAll(filepath.Dir(readme), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(readme, []byte("foreign"), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
		{"directory", func(root, slug string) {
			_, readme, _ := taskNewPaths(root, slug)
			if err := os.MkdirAll(filepath.Dir(readme), 0o755); err != nil {
				t.Fatal(err)
			}
		}},
		{"board-row", func(root, slug string) {
			board, _, _ := taskNewPaths(root, slug)
			b, err := os.ReadFile(board)
			if err != nil {
				t.Fatal(err)
			}
			v, err := task.ParseBoard(b)
			if err != nil {
				t.Fatal(err)
			}
			v.Rows = append(v.Rows, task.BoardRow{Task: slug, Status: task.StatusPlanning})
			if err := os.WriteFile(board, task.RenderBoard(v), 0o644); err != nil {
				t.Fatal(err)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := setupTaskProjectForNew(t)
			tc.prepare(root, "conflict")
			board, _, marker := taskNewPaths(root, "conflict")
			err := executeTaskNew(root, "conflict", "Conflict")
			if exitCodeOfErr(err) != exitcode.Conflict {
				t.Fatalf("err=%v", err)
			}
			if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
				t.Fatalf("conflict created marker: %v", statErr)
			}
			if tc.name == "readme" && boardRowCount(t, board, "conflict") != 0 {
				t.Fatal("README conflict appended row")
			}
		})
	}
}

func TestTaskNew_ConcurrentCreatorsPublishExactlyOnce(t *testing.T) {
	root := setupTaskProjectForNew(t)
	board, readme, marker := taskNewPaths(root, "same")
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs <- executeTaskNew(root, "same", "Same")
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	success, conflict := 0, 0
	for err := range errs {
		switch exitCodeOfErr(err) {
		case -1:
			if err != nil {
				t.Fatalf("uncoded error: %v", err)
			}
			success++
		case exitcode.Conflict:
			conflict++
		default:
			t.Fatalf("unexpected creator result: %v", err)
		}
	}
	if success != 1 || conflict != 1 || boardRowCount(t, board, "same") != 1 {
		t.Fatalf("success=%d conflict=%d rows=%d", success, conflict, boardRowCount(t, board, "same"))
	}
	if body, err := os.ReadFile(readme); err != nil || !strings.Contains(string(body), "# Same") {
		t.Fatalf("README=%q err=%v", body, err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Fatalf("prepared marker left after successful concurrent creation: %v", err)
	}
}

func TestTaskNew_InvalidSlugAndMissingDependencyWriteNothing(t *testing.T) {
	for _, tc := range []struct {
		name string
		slug string
		deps string
		code int
	}{
		{name: "invalid-slug", slug: "../escape", code: exitcode.InvalidArgs},
		{name: "missing-dependency", slug: "fresh", deps: "does-not-exist", code: exitcode.NotFound},
		{name: "self-dependency", slug: "self", deps: "self", code: exitcode.NotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := setupTaskProjectForNew(t)
			board, readme, marker := taskNewPaths(root, tc.slug)
			before, err := os.ReadFile(board)
			if err != nil {
				t.Fatal(err)
			}
			cmd := taskCommand()
			cmd.SetOut(&bytes.Buffer{})
			cmd.SetErr(&bytes.Buffer{})
			args := []string{"new", "--project", root, "--task", tc.slug, "--title", "Fresh"}
			if tc.deps != "" {
				args = append(args, "--depends-on", tc.deps)
			}
			cmd.SetArgs(args)
			err = cmd.Execute()
			if got := exitCodeOfErr(err); got != tc.code {
				t.Fatalf("exit=%d want=%d err=%v", got, tc.code, err)
			}
			after, readErr := os.ReadFile(board)
			if readErr != nil || !bytes.Equal(after, before) {
				t.Fatalf("board changed: err=%v before=%q after=%q", readErr, before, after)
			}
			for _, path := range []string{readme, marker} {
				if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
					t.Fatalf("invalid request published %s: %v", path, statErr)
				}
			}
		})
	}
}

func TestTaskNew_DirectoryDependencyIsRevalidatedBeforeBoardCommit(t *testing.T) {
	root := setupTaskProjectForNew(t)
	depDir := filepath.Join(root, "tasks", "dep")
	if err := os.MkdirAll(depDir, 0o755); err != nil {
		t.Fatal(err)
	}
	board, readme, marker := taskNewPaths(root, "fresh")
	orig := taskNewStatFn
	calls := 0
	taskNewStatFn = func(path string) (os.FileInfo, error) {
		if path == depDir {
			calls++
			if calls == 2 {
				return nil, os.ErrNotExist
			}
		}
		return orig(path)
	}
	err := executeTaskNewWithDeps(root, "fresh", "dep")
	taskNewStatFn = orig
	t.Cleanup(func() { taskNewStatFn = orig })
	if got := exitCodeOfErr(err); got != exitcode.Unexpected {
		t.Fatalf("exit=%d want=%d err=%v", got, exitcode.Unexpected, err)
	}
	var committed *lifecycle.CommittedMutationError
	if !errors.As(err, &committed) {
		t.Fatalf("err=%T %v, want retained prepared-state recovery error", err, err)
	}
	if calls != 2 || boardRowCount(t, board, "fresh") != 0 {
		t.Fatalf("dependency checks=%d rows=%d", calls, boardRowCount(t, board, "fresh"))
	}
	for _, path := range []string{readme, marker} {
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("recoverable prepared state missing %s: %v", path, statErr)
		}
	}
	if err := executeTaskNewWithDeps(root, "fresh", "dep"); err != nil {
		t.Fatalf("exact retry after dependency recovery: %v", err)
	}
	if boardRowCount(t, board, "fresh") != 1 {
		t.Fatal("recovered retry did not commit one row")
	}
}

func TestTaskNew_OutputFailureRetainsPreparedRecovery(t *testing.T) {
	root := setupTaskProjectForNew(t)
	board, readme, marker := taskNewPaths(root, "output-retry")
	boom := errors.New("output encoder failed")
	orig := newYAMLEnc
	newYAMLEnc = func(io.Writer) yamlEnc { return &failYAMLEnc{err: boom} }
	err := executeTaskNew(root, "output-retry", "Output Retry")
	newYAMLEnc = orig
	t.Cleanup(func() { newYAMLEnc = orig })
	var committed *lifecycle.CommittedMutationError
	if !errors.As(err, &committed) || !errors.Is(err, boom) {
		t.Fatalf("err=%T %v, want committed encoder failure", err, err)
	}
	if boardRowCount(t, board, "output-retry") != 1 {
		t.Fatal("output failure lost committed row")
	}
	for _, path := range []string{readme, marker} {
		if _, statErr := os.Stat(path); statErr != nil {
			t.Fatalf("output recovery state missing %s: %v", path, statErr)
		}
	}
	if err := executeTaskNew(root, "output-retry", "Output Retry"); err != nil {
		t.Fatalf("exact output retry: %v", err)
	}
	if boardRowCount(t, board, "output-retry") != 1 {
		t.Fatal("output retry duplicated committed row")
	}
	if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
		t.Fatalf("marker not finalized after output retry: %v", statErr)
	}
}

func TestOwnedMarkerFinalizationFaultMatrixRetainsRetryReceipt(t *testing.T) {
	boom := errors.New("injected finalization fault")
	for _, tc := range []struct {
		name   string
		inject func(prepared, committed string)
	}{
		{name: "receipt-link", inject: func(_, _ string) {
			ownedMarkerLinkFn = func(string, string) error { return boom }
		}},
		{name: "receipt-parent-sync", inject: func(_, _ string) {
			calls := 0
			ownedMarkerSyncDirFn = func(string) error {
				calls++
				if calls == 1 {
					return boom
				}
				return nil
			}
		}},
		{name: "prepared-unlink", inject: func(prepared, _ string) {
			ownedMarkerRemoveFn = func(path string) error {
				if path == prepared {
					return boom
				}
				return os.Remove(path)
			}
		}},
		{name: "prepared-unlink-parent-sync", inject: func(_, _ string) {
			calls := 0
			ownedMarkerSyncDirFn = func(string) error {
				calls++
				if calls == 2 {
					return boom
				}
				return nil
			}
		}},
		{name: "receipt-unlink", inject: func(_, committed string) {
			ownedMarkerRemoveFn = func(path string) error {
				if path == committed {
					return boom
				}
				return os.Remove(path)
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			prepared := filepath.Join(dir, "task.prepared")
			committed := committedMarkerPath(prepared)
			expected := []byte(`{"schema_version":2,"task_id":"owned","board_after_sha256":"post"}` + "\n")
			if err := os.WriteFile(prepared, expected, 0o600); err != nil {
				t.Fatal(err)
			}
			origLink, origRemove, origSync := ownedMarkerLinkFn, ownedMarkerRemoveFn, ownedMarkerSyncDirFn
			t.Cleanup(func() { ownedMarkerLinkFn, ownedMarkerRemoveFn, ownedMarkerSyncDirFn = origLink, origRemove, origSync })
			tc.inject(prepared, committed)
			err := removeOwnedFileDurable(prepared, expected)
			if !errors.Is(err, boom) {
				t.Fatalf("err=%v, want injected fault", err)
			}
			preparedBytes, preparedErr := os.ReadFile(prepared)
			committedBytes, committedErr := os.ReadFile(committed)
			if (preparedErr != nil || !bytes.Equal(preparedBytes, expected)) && (committedErr != nil || !bytes.Equal(committedBytes, expected)) {
				t.Fatalf("no exact retry receipt remains: prepared=%v committed=%v", preparedErr, committedErr)
			}
			ownedMarkerLinkFn, ownedMarkerRemoveFn, ownedMarkerSyncDirFn = os.Link, os.Remove, syncOwnedMarkerDir
			if err := removeOwnedFileDurable(prepared, expected); err != nil {
				t.Fatalf("idempotent retry: %v", err)
			}
			for _, path := range []string{prepared, committed} {
				if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
					t.Fatalf("receipt not finalized %s: %v", path, statErr)
				}
			}
		})
	}
}

func TestOwnedMarkerFinalizationFinalCleanupFenceIsSafe(t *testing.T) {
	dir := t.TempDir()
	prepared := filepath.Join(dir, "task.prepared")
	committed := committedMarkerPath(prepared)
	expected := []byte("owned\n")
	if err := os.WriteFile(prepared, expected, 0o600); err != nil {
		t.Fatal(err)
	}
	orig := ownedMarkerSyncDirFn
	calls := 0
	ownedMarkerSyncDirFn = func(string) error {
		calls++
		if calls == 3 {
			return errors.New("final receipt cleanup fence failed")
		}
		return nil
	}
	t.Cleanup(func() { ownedMarkerSyncDirFn = orig })
	if err := removeOwnedFileDurable(prepared, expected); err != nil {
		t.Fatalf("final receipt fence cannot manufacture an unretryable error: %v", err)
	}
	if calls != 3 {
		t.Fatalf("sync calls=%d want=3", calls)
	}
	for _, path := range []string{prepared, committed} {
		if _, statErr := os.Stat(path); !os.IsNotExist(statErr) {
			t.Fatalf("cleanup path remains %s: %v", path, statErr)
		}
	}
}

func TestTaskNew_CommittedReceiptOnlyRetryAndForeignReceiptRefusal(t *testing.T) {
	t.Run("receipt-only-exact-retry", func(t *testing.T) {
		root := setupTaskProjectForNew(t)
		board, _, marker := taskNewPaths(root, "receipt-retry")
		committed := committedMarkerPath(marker)
		orig := ownedMarkerSyncDirFn
		calls := 0
		ownedMarkerSyncDirFn = func(string) error {
			calls++
			if calls == 2 {
				return errors.New("prepared unlink fence failed")
			}
			return nil
		}
		err := executeTaskNew(root, "receipt-retry", "Receipt Retry")
		ownedMarkerSyncDirFn = orig
		t.Cleanup(func() { ownedMarkerSyncDirFn = orig })
		var committedErr *lifecycle.CommittedMutationError
		if !errors.As(err, &committedErr) || boardRowCount(t, board, "receipt-retry") != 1 {
			t.Fatalf("err=%v rows=%d", err, boardRowCount(t, board, "receipt-retry"))
		}
		if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
			t.Fatalf("prepared marker survived injected boundary: %v", statErr)
		}
		if _, statErr := os.Stat(committed); statErr != nil {
			t.Fatalf("committed receipt missing: %v", statErr)
		}
		if err := executeTaskNew(root, "receipt-retry", "Receipt Retry"); err != nil {
			t.Fatalf("receipt-only exact retry: %v", err)
		}
		if boardRowCount(t, board, "receipt-retry") != 1 {
			t.Fatal("receipt retry duplicated board row")
		}
		if _, statErr := os.Stat(committed); !os.IsNotExist(statErr) {
			t.Fatalf("committed receipt not finalized: %v", statErr)
		}
	})

	t.Run("foreign-receipt-mismatch", func(t *testing.T) {
		root := setupTaskProjectForNew(t)
		board, _, marker := taskNewPaths(root, "foreign-receipt")
		orig := taskNewRemoveMarkerFn
		taskNewRemoveMarkerFn = func(string, []byte) error { return errors.New("retain prepared") }
		_ = executeTaskNew(root, "foreign-receipt", "Foreign Receipt")
		taskNewRemoveMarkerFn = orig
		t.Cleanup(func() { taskNewRemoveMarkerFn = orig })
		foreign := []byte("foreign receipt\n")
		if err := os.WriteFile(committedMarkerPath(marker), foreign, 0o600); err != nil {
			t.Fatal(err)
		}
		err := executeTaskNew(root, "foreign-receipt", "Foreign Receipt")
		if exitCodeOfErr(err) != exitcode.Conflict || boardRowCount(t, board, "foreign-receipt") != 1 {
			t.Fatalf("err=%v rows=%d", err, boardRowCount(t, board, "foreign-receipt"))
		}
		if got, _ := os.ReadFile(committedMarkerPath(marker)); !bytes.Equal(got, foreign) {
			t.Fatal("foreign receipt was changed or removed")
		}
	})
}

func TestTaskNew_CommittedRetryRequiresExactBoardRowAndBoundPostimage(t *testing.T) {
	root := setupTaskProjectForNew(t)
	board, _, marker := taskNewPaths(root, "row-bound")
	orig := taskNewRemoveMarkerFn
	taskNewRemoveMarkerFn = func(string, []byte) error { return errors.New("retain marker") }
	_ = executeTaskNewWithDeps(root, "row-bound", "")
	taskNewRemoveMarkerFn = orig
	t.Cleanup(func() { taskNewRemoveMarkerFn = orig })
	markerBytes, err := os.ReadFile(marker)
	if err != nil {
		t.Fatal(err)
	}
	var receipt taskNewPrepared
	if err := json.Unmarshal(markerBytes, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.SchemaVersion != 2 || receipt.BoardAfterSHA256 == "" || receipt.BoardBeforeSHA256 == receipt.BoardAfterSHA256 {
		t.Fatalf("receipt does not bind pre/post board identity: %+v", receipt)
	}
	boardBytes, _ := os.ReadFile(board)
	v, err := task.ParseBoard(boardBytes)
	if err != nil {
		t.Fatal(err)
	}
	for i := range v.Rows {
		if v.Rows[i].Task == "row-bound" {
			v.Rows[i].Status = task.StatusQueued
		}
	}
	if err := os.WriteFile(board, task.RenderBoard(v), 0o644); err != nil {
		t.Fatal(err)
	}
	err = executeTaskNewWithDeps(root, "row-bound", "")
	if exitCodeOfErr(err) != exitcode.Conflict {
		t.Fatalf("mutated committed row retry err=%v", err)
	}
	if got, _ := os.ReadFile(marker); !bytes.Equal(got, markerBytes) {
		t.Fatal("row mismatch changed owned marker")
	}
}

func TestTaskNew_CommittedReceiptOnlyRetryRefusesChangedVisibleState(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(t *testing.T, board, readme string)
	}{
		{
			name: "row-absent",
			mutate: func(t *testing.T, board, _ string) {
				b, err := os.ReadFile(board)
				if err != nil {
					t.Fatal(err)
				}
				v, err := task.ParseBoard(b)
				if err != nil {
					t.Fatal(err)
				}
				rows := v.Rows[:0]
				for _, row := range v.Rows {
					if row.Task != "receipt-bound" {
						rows = append(rows, row)
					}
				}
				v.Rows = rows
				if err := os.WriteFile(board, task.RenderBoard(v), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "unrelated-row-added",
			mutate: func(t *testing.T, board, _ string) {
				b, err := os.ReadFile(board)
				if err != nil {
					t.Fatal(err)
				}
				v, err := task.ParseBoard(b)
				if err != nil {
					t.Fatal(err)
				}
				v.Rows = append(v.Rows, task.BoardRow{Task: "unrelated", Status: task.StatusPlanning})
				if err := os.WriteFile(board, task.RenderBoard(v), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "row-mutated",
			mutate: func(t *testing.T, board, _ string) {
				b, err := os.ReadFile(board)
				if err != nil {
					t.Fatal(err)
				}
				v, err := task.ParseBoard(b)
				if err != nil {
					t.Fatal(err)
				}
				for i := range v.Rows {
					if v.Rows[i].Task == "receipt-bound" {
						v.Rows[i].Status = task.StatusQueued
					}
				}
				if err := os.WriteFile(board, task.RenderBoard(v), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
		{
			name: "readme-mutated",
			mutate: func(t *testing.T, _, readme string) {
				b, err := os.ReadFile(readme)
				if err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(readme, append(b, []byte("foreign edit\n")...), 0o644); err != nil {
					t.Fatal(err)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := setupTaskProjectForNew(t)
			board, readme, marker := taskNewPaths(root, "receipt-bound")
			receipt := committedMarkerPath(marker)
			orig := ownedMarkerSyncDirFn
			calls := 0
			ownedMarkerSyncDirFn = func(string) error {
				calls++
				if calls == 2 {
					return errors.New("prepared unlink fence failed")
				}
				return nil
			}
			err := executeTaskNew(root, "receipt-bound", "Receipt Bound")
			ownedMarkerSyncDirFn = orig
			t.Cleanup(func() { ownedMarkerSyncDirFn = orig })
			var committedErr *lifecycle.CommittedMutationError
			if !errors.As(err, &committedErr) {
				t.Fatalf("initial finalization err=%v", err)
			}
			if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
				t.Fatalf("prepared marker survived: %v", statErr)
			}
			receiptBefore, err := os.ReadFile(receipt)
			if err != nil {
				t.Fatal(err)
			}

			tc.mutate(t, board, readme)
			boardBefore, _ := os.ReadFile(board)
			readmeBefore, _ := os.ReadFile(readme)
			err = executeTaskNew(root, "receipt-bound", "Receipt Bound")
			if exitCodeOfErr(err) != exitcode.Conflict {
				t.Fatalf("changed committed retry err=%v", err)
			}
			boardAfter, _ := os.ReadFile(board)
			readmeAfter, _ := os.ReadFile(readme)
			receiptAfter, receiptErr := os.ReadFile(receipt)
			if !bytes.Equal(boardAfter, boardBefore) || !bytes.Equal(readmeAfter, readmeBefore) {
				t.Fatal("failed retry rewrote changed committed state")
			}
			if receiptErr != nil || !bytes.Equal(receiptAfter, receiptBefore) {
				t.Fatalf("failed retry changed committed receipt: %v", receiptErr)
			}
			if _, statErr := os.Stat(marker); !os.IsNotExist(statErr) {
				t.Fatalf("failed retry recreated prepared marker: %v", statErr)
			}
		})
	}
}

func executeTaskNewWithDeps(root, slug, deps string) error {
	cmd := taskCommand()
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetErr(&bytes.Buffer{})
	cmd.SetArgs([]string{"new", "--project", root, "--task", slug, "--title", "Fresh", "--depends-on", deps})
	return cmd.Execute()
}
