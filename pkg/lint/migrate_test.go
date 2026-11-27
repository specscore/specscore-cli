package lint

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/plan"
)

func featureDoc(status, footerURL string) string {
	return "# Feature: Foo\n\n**Status:** " + status + "\n\n## Summary\n\nx\n\n---\n*This document follows the " + footerURL + "*\n"
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// AC: backfills-format-and-status — status-bearing artifacts gain format: +
// status: (mirrored from the body).
func TestMigrate_BackfillsFeatureAndIdea(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"features/foo/README.md": featureDoc("Approved", "https://specscore.md/feature-specification"),
		"ideas/bar.md":           "# Idea: Bar\n\n**Status:** Draft\n\n---\n*This document follows the https://specscore.md/idea-specification*\n",
	})
	changed, err := Migrate(specRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 2 {
		t.Errorf("expected 2 changed files, got %v", changed)
	}
	feat := readFile(t, filepath.Join(specRoot, "features", "foo", "README.md"))
	if !strings.HasPrefix(feat, "---\nformat: https://specscore.md/feature-specification\nstatus: Approved\n---\n\n# Feature: Foo") {
		t.Errorf("feature not migrated:\n%s", feat)
	}
	idea := readFile(t, filepath.Join(specRoot, "ideas", "bar.md"))
	if !strings.HasPrefix(idea, "---\nformat: https://specscore.md/idea-specification\nstatus: Draft\n---") {
		t.Errorf("idea not migrated:\n%s", idea)
	}
}

// AC: status-less-types-excluded — an index README gets format: but no status:.
func TestMigrate_StatusLessIndexGetsFormatOnly(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"features/README.md": "# Features\n\n## Index\n\n| Feature | Status |\n|---|---|\n\n## Open Questions\n\nNone.\n\n---\n*This document follows the https://specscore.md/features-index-specification*\n",
	})
	if _, err := Migrate(specRoot); err != nil {
		t.Fatal(err)
	}
	idx := readFile(t, filepath.Join(specRoot, "features", "README.md"))
	if !strings.HasPrefix(idx, "---\nformat: https://specscore.md/features-index-specification\n---") {
		t.Errorf("index not migrated to format-only:\n%s", idx)
	}
	if strings.Contains(idx, "status:") {
		t.Errorf("status-less index must not gain status:\n%s", idx)
	}
}

// AC: migration-idempotent — a second run changes nothing.
func TestMigrate_Idempotent(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"features/foo/README.md": featureDoc("Draft", "https://specscore.md/feature-specification"),
	})
	if _, err := Migrate(specRoot); err != nil {
		t.Fatal(err)
	}
	after := readFile(t, filepath.Join(specRoot, "features", "foo", "README.md"))
	changed, err := Migrate(specRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 0 {
		t.Errorf("second run should change nothing, got %v", changed)
	}
	if readFile(t, filepath.Join(specRoot, "features", "foo", "README.md")) != after {
		t.Error("idempotent re-run altered the file")
	}
}

// AC: footer-aligned-to-format — a wrong footer URL is realigned to format:.
func TestMigrate_AlignsFooter(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		// footer carries the WRONG (plan) URL.
		"features/foo/README.md": featureDoc("Draft", "https://specscore.md/plan-specification"),
	})
	if _, err := Migrate(specRoot); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(specRoot, "features", "foo", "README.md"))
	if !strings.Contains(got, "*This document follows the https://specscore.md/feature-specification*") {
		t.Errorf("footer not aligned to format:\n%s", got)
	}
	if strings.Contains(got, "plan-specification") {
		t.Errorf("stale footer URL remains:\n%s", got)
	}
}

// A status-bearing artifact with no body **Status:** gets format: only.
func TestMigrate_StatusBearingNoBodyStatus(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"features/foo/README.md": "# Feature: Foo\n\nNo status line.\n\n---\n*This document follows the https://specscore.md/feature-specification*\n",
	})
	if _, err := Migrate(specRoot); err != nil {
		t.Fatal(err)
	}
	got := readFile(t, filepath.Join(specRoot, "features", "foo", "README.md"))
	if !strings.HasPrefix(got, "---\nformat: https://specscore.md/feature-specification\n---") {
		t.Errorf("expected format-only frontmatter:\n%s", got)
	}
	if strings.Contains(got, "status:") {
		t.Errorf("no status should be added without a body status:\n%s", got)
	}
}

// writeTaskBoard creates a flat project-root task board (a sibling of
// spec/, matching internal/cli.resolveTasksDir's `<project-root>/tasks/`
// convention) alongside the specRoot writeSpec already built, and returns the
// project root. files are relative to the project root's tasks/ directory
// (e.g. "README.md" for the board index, "<slug>/README.md" for a task file).
func writeTaskBoard(t *testing.T, specRoot string, files map[string]string) string {
	t.Helper()
	projectRoot := filepath.Dir(specRoot)
	tasksDir := filepath.Join(projectRoot, "tasks")
	for rel, content := range files {
		path := filepath.Join(tasksDir, rel)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return projectRoot
}

// TestMigrate_BackfillsExistingBoardTaskStatus is the existing-board case the
// task-change-status-requires-a-status-line-no-scaffold-writes-one lesson
// named directly: a task README with no **Status:** line at all (exactly what
// every board written before this fix, and every hand-authored board, looks
// like), migrated, then successfully transitioned via `task change-status`.
// Only pkg/lint.Migrate is exercised here (the CLI-level transition is proven
// in internal/cli's TestTaskChangeStatus_AfterMigrateOnExistingBoard); this
// test proves the migration step in isolation: the value comes from the
// board row, never invented, and a second run is a no-op.
func TestMigrate_BackfillsExistingBoardTaskStatus(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{})
	projectRoot := writeTaskBoard(t, specRoot, map[string]string{
		"README.md": "# Tasks\n\n| Task | Status | Depends on | Branch | Agent | Requester | Time |\n" +
			"|---|---|---|---|---|---|---|\n" +
			"| [fleet-core-modules-bump](fleet-core-modules-bump/) | ⏳ `queued` | — | — | — | — | — |\n",
		// No **Status:** line anywhere in this file — the exact shape that
		// made `task change-status` exit 10 on an existing board.
		"fleet-core-modules-bump/README.md": "# Bump sneat-core-modules fleet-wide\n\n" +
			"Some description.\n\n## Dependencies\n\nNone\n\n## Summary\n\nNone\n",
	})

	changed, err := MigrateWithProjectRoot(projectRoot, specRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 || changed[0] != "tasks/fleet-core-modules-bump/README.md" {
		t.Fatalf("expected exactly the task README reported changed, got %v", changed)
	}

	readmePath := filepath.Join(projectRoot, "tasks", "fleet-core-modules-bump", "README.md")
	got := readFile(t, readmePath)
	want := "# Bump sneat-core-modules fleet-wide\n\n**Status:** queued\n\nSome description.\n\n## Dependencies\n\nNone\n\n## Summary\n\nNone\n"
	if got != want {
		t.Fatalf("task README not backfilled as expected:\ngot:\n%s\nwant:\n%s", got, want)
	}

	// Idempotent: a second run changes nothing.
	changed2, err := MigrateWithProjectRoot(projectRoot, specRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed2) != 0 {
		t.Fatalf("second migrate run must be a no-op, got %v", changed2)
	}
}

// TestMigrate_TaskBoardStatusLeavesOrphanAndExistingLineAlone covers the two
// guardrails: a task with an existing **Status:** line is never overwritten
// (even if it disagrees with the board — migrate is a bootstrap, not a
// resync), and a task with no matching board row is left unchanged rather
// than inventing a value.
func TestMigrate_TaskBoardStatusLeavesOrphanAndExistingLineAlone(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{})
	projectRoot := writeTaskBoard(t, specRoot, map[string]string{
		"README.md": "# Tasks\n\n| Task | Status | Depends on | Branch | Agent | Requester | Time |\n" +
			"|---|---|---|---|---|---|---|\n" +
			"| [has-status](has-status/) | ⏳ `queued` | — | — | — | — | — |\n",
		// Board says queued, file already says in_progress: migrate must not
		// silently overwrite an existing line from the index.
		"has-status/README.md": "# Has Status\n\n**Status:** in_progress\n\nDesc.\n\n## Dependencies\n\nNone\n\n## Summary\n\nNone\n",
		// No board row for this slug at all: nothing to source a value from.
		"orphan/README.md": "# Orphan\n\nDesc.\n\n## Dependencies\n\nNone\n\n## Summary\n\nNone\n",
	})

	changed, err := MigrateWithProjectRoot(projectRoot, specRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 0 {
		t.Fatalf("expected no changes (existing line + orphan), got %v", changed)
	}
	if got := readFile(t, filepath.Join(projectRoot, "tasks", "has-status", "README.md")); !strings.Contains(got, "**Status:** in_progress") {
		t.Fatalf("existing Status line must not be overwritten from the index:\n%s", got)
	}
	if got := readFile(t, filepath.Join(projectRoot, "tasks", "orphan", "README.md")); strings.Contains(got, "**Status:**") {
		t.Fatalf("orphan task must not gain an invented Status line:\n%s", got)
	}
}

// TestMigrate_TaskBoardParseErrorIsReturned proves a malformed task board
// (task.ParseBoard fails) surfaces as a hard Migrate error rather than being
// silently skipped, matching migrateArtifact's error-propagation convention
// for a genuinely unreadable structure.
// TestMigrate_TaskBoardNoBoardFileIsSkipped covers a tasks/ directory that
// exists but has no README.md board at all — nothing to source a value from,
// so migrate leaves it alone rather than erroring.
func TestMigrate_TaskBoardNoBoardFileIsSkipped(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{})
	projectRoot := filepath.Dir(specRoot)
	if err := os.MkdirAll(filepath.Join(projectRoot, "tasks"), 0o755); err != nil {
		t.Fatal(err)
	}
	changed, err := MigrateWithProjectRoot(projectRoot, specRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 0 {
		t.Fatalf("expected no-op with no board file, got %v", changed)
	}
}

func TestMigrate_TaskBoardParseErrorIsReturned(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{})
	projectRoot := writeTaskBoard(t, specRoot, map[string]string{
		"README.md": "# Tasks\n\nnot a table at all\n",
	})
	if _, err := MigrateWithProjectRoot(projectRoot, specRoot); err == nil {
		t.Fatal("expected a task board parse error from migration")
	}
}

// TestMigrate_TaskBoardNoReadmeUnderEntryIsSkipped covers a directory entry
// under tasks/ that isn't a task at all (no README.md) — left alone rather
// than erroring the whole migration.
func TestMigrate_TaskBoardNoReadmeUnderEntryIsSkipped(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{})
	projectRoot := writeTaskBoard(t, specRoot, map[string]string{
		"README.md":      taskBoard([2]string{"auth", "queued"}),
		"auth/README.md": taskReadmeWithStatus("Auth", ""),
	})
	if err := os.MkdirAll(filepath.Join(projectRoot, "tasks", "not-a-task"), 0o755); err != nil {
		t.Fatal(err)
	}
	changed, err := MigrateWithProjectRoot(projectRoot, specRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 1 || changed[0] != "tasks/auth/README.md" {
		t.Fatalf("expected only the real task backfilled, got %v", changed)
	}
}

// TestMigrate_TaskBoardTitleNotRecognizedIsSkipped covers a task README that
// doesn't start with "# Title" at all — insertTaskBodyStatus refuses to guess
// at its structure, and migrate leaves it for a human.
func TestMigrate_TaskBoardTitleNotRecognizedIsSkipped(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{})
	projectRoot := writeTaskBoard(t, specRoot, map[string]string{
		"README.md":      taskBoard([2]string{"auth", "queued"}),
		"auth/README.md": "Not a title line at all.\n",
	})
	changed, err := MigrateWithProjectRoot(projectRoot, specRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 0 {
		t.Fatalf("expected the unrecognizable task file to be left alone, got %v", changed)
	}
	if got := readFile(t, filepath.Join(projectRoot, "tasks", "auth", "README.md")); got != "Not a title line at all.\n" {
		t.Fatalf("file must be byte-unchanged, got:\n%s", got)
	}
}

// TestInsertTaskBodyStatus_NoTrailingNewlineIsRejected covers a title line
// with no trailing newline at all (a single-line file) — insertTaskBodyStatus
// refuses to guess where the title ends rather than corrupting the file.
func TestInsertTaskBodyStatus_NoTrailingNewlineIsRejected(t *testing.T) {
	if _, ok := insertTaskBodyStatus([]byte("# Title, no newline at all"), "queued"); ok {
		t.Fatal("expected insertTaskBodyStatus to reject a title line with no trailing newline")
	}
}

// TestMigrate_TaskBoardReadDirErrorIsReturned and
// TestMigrate_TaskBoardWriteErrorIsReturned exercise the two OS-failure
// branches via permission changes, matching TestMigrate_WalkError/
// TestMigrate_WriteError's existing convention in this file.
func TestMigrate_TaskBoardReadDirErrorIsReturned(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{})
	projectRoot := writeTaskBoard(t, specRoot, map[string]string{
		"README.md": taskBoard([2]string{"auth", "queued"}),
	})
	tasksDir := filepath.Join(projectRoot, "tasks")
	// 0o111 (execute-only, no read): os.ReadFile(tasksDir/README.md) can still
	// traverse INTO tasksDir (search only needs execute), but os.ReadDir(tasksDir)
	// needs read permission on the directory itself and fails — isolating the
	// ReadDir branch specifically from the earlier board-read branch.
	if err := os.Chmod(tasksDir, 0o111); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(tasksDir, 0o755) }()
	if _, err := MigrateWithProjectRoot(projectRoot, specRoot); err == nil {
		t.Error("expected a ReadDir error")
	}
}

func TestMigrate_TaskBoardWriteErrorIsReturned(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{})
	projectRoot := writeTaskBoard(t, specRoot, map[string]string{
		"README.md":      taskBoard([2]string{"auth", "queued"}),
		"auth/README.md": taskReadmeWithStatus("Auth", ""),
	})
	readmePath := filepath.Join(projectRoot, "tasks", "auth", "README.md")
	if err := os.Chmod(readmePath, 0o444); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(readmePath, 0o644) }()
	if _, err := MigrateWithProjectRoot(projectRoot, specRoot); err == nil {
		t.Error("expected a write error")
	}
}

func TestMigrate_FlatNonPlanIsIgnored(t *testing.T) {
	original := "# Random notes\n\n**Status:** Draft\n"
	specRoot := writeSpec(t, map[string]string{"plans/notes.md": original})
	changed, err := Migrate(specRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 0 {
		t.Fatalf("non-Plan flat markdown must not gain Plan frontmatter: %v", changed)
	}
	if got := readFile(t, filepath.Join(specRoot, "plans", "notes.md")); got != original {
		t.Fatalf("non-Plan flat markdown was mutated:\nwant:\n%s\ngot:\n%s", original, got)
	}
}

func TestMigrate_PlanIgnoresFauxBodyStatus(t *testing.T) {
	original := "---\nformat: https://specscore.md/plan-specification\n---\n\n" +
		"**Status:** Pre-title example\n\n" +
		"```markdown\n**Status:** Fenced example\n```\n\n" +
		"# Plan: Genuine\n\n## Summary\n\nNo actual status.\n"
	specRoot := writeSpec(t, map[string]string{"plans/genuine.md": original})
	changed, err := Migrate(specRoot)
	if err != nil {
		t.Fatal(err)
	}
	if len(changed) != 0 {
		t.Fatalf("faux Plan body statuses must not create a mirror: %v", changed)
	}
	if got := readFile(t, filepath.Join(specRoot, "plans", "genuine.md")); got != original {
		t.Fatalf("migration rewrote a Plan from faux metadata:\nwant:\n%s\ngot:\n%s", original, got)
	}
}

func TestMigrate_PlanStatusParserErrorIsReturned(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"plans/genuine.md": "# Plan: Genuine\n\n**Status:** Draft\n",
	})
	original := parsePlanForArtifactStatus
	parsePlanForArtifactStatus = func(string) (*plan.Plan, error) {
		return nil, errors.New("forced Plan parser failure")
	}
	t.Cleanup(func() { parsePlanForArtifactStatus = original })
	if _, err := Migrate(specRoot); err == nil {
		t.Fatal("expected Plan parser error from migration")
	}
}

func TestMigrate_WalkError(t *testing.T) {
	specRoot := writeSpec(t, map[string]string{
		"ideas/ok.md": "# Idea: OK\n",
	})
	badDir := filepath.Join(specRoot, "ideas", "bad-subdir")
	if err := os.MkdirAll(badDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(badDir, 0o000); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(badDir, 0o755) }()
	if _, err := Migrate(specRoot); err == nil {
		t.Error("expected walk error")
	}
}

func TestMigrate_WriteError(t *testing.T) {
	// Two read-only features needing migration under one walker: the first
	// write fails (setting writeErr); the second exercises the early return.
	specRoot := writeSpec(t, map[string]string{
		"features/a/README.md": featureDoc("Draft", "https://specscore.md/feature-specification"),
		"features/b/README.md": featureDoc("Draft", "https://specscore.md/feature-specification"),
	})
	for _, s := range []string{"a", "b"} {
		p := filepath.Join(specRoot, "features", s, "README.md")
		if err := os.Chmod(p, 0o444); err != nil {
			t.Skip("cannot change permissions")
		}
		defer func() { _ = os.Chmod(p, 0o644) }()
	}
	if _, err := Migrate(specRoot); err == nil {
		t.Error("expected write error on read-only artifacts needing migration")
	}
}

func TestEnsureFrontmatter(t *testing.T) {
	fields := [][2]string{{"format", "u"}, {"status", "Draft"}}
	t.Run("no block prepends", func(t *testing.T) {
		got := string(ensureFrontmatter([]byte("# Title\n"), fields))
		if got != "---\nformat: u\nstatus: Draft\n---\n\n# Title\n" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("existing block upserts", func(t *testing.T) {
		got := string(ensureFrontmatter([]byte("---\nformat: old\n---\n\n# T\n"), fields))
		if got != "---\nformat: u\nstatus: Draft\n---\n\n# T\n" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("dotted closer is preserved", func(t *testing.T) {
		got := string(ensureFrontmatter([]byte("---\nformat: old\n...\n\n# T\n"), fields))
		if got != "---\nformat: u\nstatus: Draft\n...\n\n# T\n" {
			t.Errorf("got %q", got)
		}
	})
	t.Run("opening fence without closing prepends", func(t *testing.T) {
		got := string(ensureFrontmatter([]byte("---\nformat: x\nno close\n"), fields))
		if !strings.HasPrefix(got, "---\nformat: u\nstatus: Draft\n---\n\n---\n") {
			t.Errorf("got %q", got)
		}
	})
}

func TestHasClosingFence(t *testing.T) {
	if !hasClosingFence([]string{"---", "k: v", "---"}) {
		t.Error("expected true with closing fence")
	}
	if !hasClosingFence([]string{"---", "k: v", "..."}) {
		t.Error("expected dotted closer to close frontmatter")
	}
	if hasClosingFence([]string{"---", "k: v"}) {
		t.Error("expected false without closing fence")
	}
}
