package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/specscore/specscore-cli/pkg/lint"
	"github.com/specscore/specscore-cli/pkg/projectdef"
)

func readLesson(t *testing.T, root, slug string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, "spec", "lessons", slug, "README.md"))
	if err != nil {
		t.Fatalf("reading lesson %s: %v", slug, err)
	}
	return string(b)
}

func TestLessonScaffoldSnapshotRestorePreservesCapturedMode(t *testing.T) {
	root := t.TempDir()
	target := filepath.Join(root, "spec", "lessons", "existing", "README.md")
	if err := os.MkdirAll(filepath.Join(filepath.Dir(target), "occurrences"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("original\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	snapshot, err := snapshotLessonScaffold(root, target)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(target, []byte("mutated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(target, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := snapshot.restore(); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(target)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "original\n" {
		t.Fatalf("restored bytes = %q", got)
	}
	info, err := os.Stat(target)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Fatalf("restored mode = %o, want 600", info.Mode().Perm())
	}
}

// AC: scaffold-emits-frontmatter-and-sections — the embedded (offline)
// scaffold carries format:/status: frontmatter mirroring the body, all four
// required sections, and the footer, AND `lesson new` leaves its bounded
// Lesson/index write set lint-clean without invoking a repository-wide fixer.
func TestLessonNew_EmbeddedEmitsFrontmatterSectionsAndIndex(t *testing.T) {
	root := setupSpecRoot(t)
	if err := projectdef.WriteSpecConfig(root, lessonTestConfig()); err != nil {
		t.Fatal(err)
	}
	withCwd(t, root)

	_, stderr, err := runLesson(t, "new", "kinder-fake", "--owner", "alex")
	if err != nil {
		t.Fatalf("lesson new: %v (stderr=%s)", err, stderr)
	}
	if strings.Contains(stderr, "no specscore.yaml") {
		t.Errorf("configured project must not take legacy fallback: %q", stderr)
	}
	b, err := os.ReadFile(filepath.Join(root, "spec", "lessons", "kinder-fake", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		"---\nformat: https://specscore.md/lesson-specification\nstatus: Recorded\n---",
		"# Lesson: Kinder Fake",
		"**Status:** Recorded",
		"**Owner:** alex",
		"## Lesson",
		"## Process Gap",
		"## Tracking",
		"## Enforcement",
		"*This document follows the https://specscore.md/lesson-specification*",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("scaffold missing %q:\n%s", want, s)
		}
	}

	if _, err := os.Stat(filepath.Join(root, "spec", "lessons", "kinder-fake", "occurrences")); err != nil {
		t.Fatalf("canonical occurrence directory missing: %v", err)
	}
}

// A repository can adopt directory-form Lessons incrementally. Creating its
// first canonical Lesson must upgrade only the declared lessons index, keeping
// every pre-existing flat Lesson visible in the six-column projection.
func TestLessonNew_UpgradesLegacyIndexForFirstCanonicalLesson(t *testing.T) {
	root := setupSpecRoot(t)
	if err := projectdef.WriteSpecConfig(root, lessonTestConfig()); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "spec", "lessons"), 0o755); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(root, "spec", "lessons", "existing.md")
	writeFileT(t, legacy, "---\nformat: https://specscore.md/lesson-specification\nstatus: Recorded\n---\n\n"+
		"# Lesson: Existing\n\n**Status:** Recorded\n**Date:** 2026-08-10\n**Owner:** codex\n**Recurred:** 0\n\n"+
		"## Incident\n\nExisting legacy Lesson.\n\n## Process gap\n\nLegacy projection.\n\n## Check\n\nMigrate the index when needed.\n\n## Enforcement\n\nRecorded.\n")
	index := "# Lessons\n\n## Index\n\n" +
		"| Lesson | Status | Recurred | Date | Owner |\n|---|---|---|---|---|\n" +
		"| [existing](existing.md) | Recorded | 0 | 2026-08-10 | codex |\n\n" +
		"## Open Questions\n\nNone at this time.\n"
	writeFileT(t, filepath.Join(root, "spec", "lessons", "README.md"), index)
	withCwd(t, root)

	if _, stderr, err := runLesson(t, "new", "canonical", "--owner", "codex"); err != nil {
		t.Fatalf("lesson new: %v (stderr=%s)", err, stderr)
	}
	got, err := os.ReadFile(filepath.Join(root, "spec", "lessons", "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{
		"| Lesson | Status | Classifications | Occurrences | Last Occurred | Enforcement |",
		"| [existing](existing.md) | Recorded | Legacy | 0 |  | — |",
		"| [canonical](canonical/README.md) | Recorded |",
	} {
		if !strings.Contains(string(got), want) {
			t.Errorf("upgraded index missing %q:\n%s", want, got)
		}
	}
	violations, err := lint.Lint(lint.Options{SpecRoot: filepath.Join(root, "spec")})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range violations {
		if v.Severity == "error" && (v.Rule == "L-003" || v.Rule == "L-004") {
			t.Errorf("unexpected index violation after canonical creation: %+v", v)
		}
	}
}

func TestLessonNew_InvalidSlug(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)
	_, _, err := runLesson(t, "new", "Bad_Slug")
	if got := exitCodeOf(err); got != exitcode.InvalidArgs {
		t.Errorf("exit = %d, want %d", got, exitcode.InvalidArgs)
	}
	if _, statErr := os.Stat(filepath.Join(root, "spec", "lessons", "Bad_Slug.md")); statErr == nil {
		t.Error("no file should be created for an invalid slug")
	}
}

func TestLessonNew_Collision(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)
	if _, _, err := runLesson(t, "new", "dup"); err != nil {
		t.Fatalf("first lesson new: %v", err)
	}
	orig := readLesson(t, root, "dup")

	_, _, err := runLesson(t, "new", "dup")
	if got := exitCodeOf(err); got != exitcode.Conflict {
		t.Errorf("exit = %d, want %d (Conflict)", got, exitcode.Conflict)
	}
	if cur := readLesson(t, root, "dup"); cur != orig {
		t.Error("conflicting run must leave the existing file untouched")
	}

	if _, _, err := runLesson(t, "new", "dup", "--title", "Duplicate Again", "--force"); err != nil {
		t.Fatalf("--force run failed: %v", err)
	}
	if !strings.Contains(readLesson(t, root, "dup"), "# Lesson: Duplicate Again") {
		t.Error("--force should overwrite with the new title")
	}
}

// AC: ancestor-indexes-materialized
func TestLessonNew_AncestorIndexesMaterialized(t *testing.T) {
	root := setupSpecRoot(t) // has spec/features + spec/ideas, but no spec/README.md or spec/lessons
	withCwd(t, root)

	if _, _, err := runLesson(t, "new", "l1"); err != nil {
		t.Fatalf("lesson new: %v", err)
	}
	for _, rel := range []string{"spec/README.md", "spec/lessons/README.md", "spec/lessons/l1/README.md", "spec/lessons/l1/occurrences"} {
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(rel))); err != nil {
			t.Errorf("expected %s to exist: %v", rel, err)
		}
	}
}

func TestLessonNew_TitleOwnerDefaults(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)
	t.Setenv("USER", "envuser")
	if _, _, err := runLesson(t, "new", "defaults-lesson"); err != nil {
		t.Fatalf("lesson new: %v", err)
	}
	s := readLesson(t, root, "defaults-lesson")
	if !strings.Contains(s, "# Lesson: Defaults Lesson") {
		t.Errorf("title not defaulted from slug:\n%s", s)
	}
	if !strings.Contains(s, "**Owner:** envuser") {
		t.Errorf("owner not defaulted from $USER:\n%s", s)
	}
}

func TestLessonNew_ResolveSpecRootError(t *testing.T) {
	dir := t.TempDir() // no spec/ tree
	withCwd(t, dir)
	_, _, err := runLesson(t, "new", "p")
	if err == nil {
		t.Fatal("expected error when no spec root can be resolved")
	}
}

func TestLessonNew_ConfigPreflightWritesNothing(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)
	cmd := lessonCommand()
	cmd.SetArgs([]string{"new", "must-not-write"})
	if err := cmd.Execute(); exitCodeOf(err) != exitcode.InvalidState {
		t.Fatalf("exit = %d, want InvalidState; err=%v", exitCodeOf(err), err)
	}
	for _, rel := range []string{"specscore.yaml", "spec/README.md", "spec/lessons/must-not-write/README.md"} {
		if _, err := os.Stat(filepath.Join(root, rel)); err == nil {
			t.Errorf("preflight unexpectedly wrote %s", rel)
		}
	}
}

func TestLessonNew_InvalidClassificationVocabularyPreflightWritesNothing(t *testing.T) {
	tests := []struct {
		name            string
		classifications []string
	}{
		{name: "classification is not a slug", classifications: []string{"Bad Value"}},
		{name: "classification is duplicated", classifications: []string{"process", "process"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			root := setupSpecRoot(t)
			cfg := projectdef.SpecConfig{Extras: map[string]any{
				"lessons": map[string]any{"classifications": tt.classifications},
			}}
			if err := projectdef.WriteSpecConfig(root, cfg); err != nil {
				t.Fatal(err)
			}
			before := treeDigestForCLI(t, root)

			_, _, err := runLesson(t, "new", "must-not-write", "--project", root)
			if got := exitCodeOf(err); got != exitcode.InvalidState {
				t.Fatalf("exit = %d, want InvalidState; err=%v", got, err)
			}
			if after := treeDigestForCLI(t, root); !bytes.Equal(before, after) {
				t.Fatal("classification preflight changed the project tree")
			}
		})
	}
}

func TestLessonNew_AncestorIndexError(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)
	specDir := filepath.Join(root, "spec")
	if err := os.Chmod(specDir, 0o555); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(specDir, 0o755) }()

	_, _, err := runLesson(t, "new", "p")
	if got := exitCodeOf(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d (Unexpected)", got, exitcode.Unexpected)
	}
}

func TestLessonNew_WriteError(t *testing.T) {
	root := setupSpecRoot(t)
	withCwd(t, root)
	lessonsDir := filepath.Join(root, "spec", "lessons")
	if err := os.MkdirAll(lessonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	writeFileT(t, filepath.Join(root, "spec", "README.md"), "# spec\n")
	writeFileT(t, filepath.Join(lessonsDir, "README.md"), "# lessons\n")
	if err := os.Chmod(lessonsDir, 0o555); err != nil {
		t.Skip("cannot change permissions")
	}
	defer func() { _ = os.Chmod(lessonsDir, 0o755) }()

	_, _, err := runLesson(t, "new", "p")
	if got := exitCodeOf(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d (Unexpected)", got, exitcode.Unexpected)
	}
}

// AC: narrow-index-upsert-failure — the lint-owned bounded row writer fails
// transactionally without invoking a whole-tree lint fixer.
func TestLessonNew_IndexUpsertFails(t *testing.T) {
	root := setupSpecRoot(t)
	if err := projectdef.WriteSpecConfig(root, lessonTestConfig()); err != nil {
		t.Fatal(err)
	}
	withCwd(t, root)
	orig := lessonIndexUpsertFn
	lessonIndexUpsertFn = func(string, *lesson.Lesson) error { return errors.New("index boom") }
	t.Cleanup(func() { lessonIndexUpsertFn = orig })

	_, _, err := runLesson(t, "new", "boom")
	if got := exitCodeOf(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d (Unexpected)", got, exitcode.Unexpected)
	}
	if _, statErr := os.Stat(filepath.Join(root, "spec", "lessons", "boom", "README.md")); statErr == nil {
		t.Error("partial file should be removed after an index-upsert failure")
	}
}

// AC: internal-verify-pass-failure — the focused read-only lint call erroring exits 10.
func TestLessonNew_LintVerifyFails(t *testing.T) {
	root := setupSpecRoot(t)
	if err := projectdef.WriteSpecConfig(root, lessonTestConfig()); err != nil {
		t.Fatal(err)
	}
	withCwd(t, root)
	orig := lintLintFn
	lintLintFn = func(opts lint.Options) ([]lint.Violation, error) {
		return nil, errors.New("verify boom")
	}
	t.Cleanup(func() { lintLintFn = orig })

	_, _, err := runLesson(t, "new", "boom")
	if got := exitCodeOf(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d (Unexpected)", got, exitcode.Unexpected)
	}
}

// AC: generated-lesson-lint-error-surfaced — a remaining error-severity
// violation touching the new file (or the lessons/ family) after the read-only
// pass surfaces as a failure rather than a silent success.
func TestLessonNew_GeneratedLessonFailsLint(t *testing.T) {
	root := setupSpecRoot(t)
	if err := projectdef.WriteSpecConfig(root, lessonTestConfig()); err != nil {
		t.Fatal(err)
	}
	withCwd(t, root)
	orig := lintLintFn
	lintLintFn = func(opts lint.Options) ([]lint.Violation, error) {
		return []lint.Violation{
			{File: "lessons/boom/README.md", Line: 1, Rule: "L-001", Severity: "error", Message: "boom"},
		}, nil
	}
	t.Cleanup(func() { lintLintFn = orig })

	_, _, err := runLesson(t, "new", "boom")
	if got := exitCodeOf(err); got != exitcode.Unexpected {
		t.Errorf("exit = %d, want %d (Unexpected)", got, exitcode.Unexpected)
	}
	if !strings.Contains(err.Error(), "generated Lesson failed lint") {
		t.Errorf("expected generated-lesson-failed-lint message, got: %v", err)
	}
}

func TestLessonNew_DoesNotMigrateUnrelatedOutstandingQuestions(t *testing.T) {
	root := setupLintCleanProject(t)
	if err := projectdef.WriteSpecConfig(root, lessonTestConfig()); err != nil {
		t.Fatal(err)
	}
	unrelatedDir := filepath.Join(root, "spec", "research")
	if err := os.MkdirAll(unrelatedDir, 0o755); err != nil {
		t.Fatal(err)
	}
	unrelatedPath := filepath.Join(unrelatedDir, "legacy.md")
	unrelated := []byte("# Legacy research\n\n## Outstanding Questions\n\n- Keep this byte-identical until explicit lint --fix.\n")
	if err := os.WriteFile(unrelatedPath, unrelated, 0o644); err != nil {
		t.Fatal(err)
	}

	if _, _, err := runLesson(t, "new", "bounded-write", "--project", root); err != nil {
		t.Fatalf("lesson new: %v", err)
	}
	if got, err := os.ReadFile(unrelatedPath); err != nil || !bytes.Equal(got, unrelated) {
		t.Fatalf("lesson new ran a hidden whole-tree fixer: err=%v\n%s", err, got)
	}
}

func TestLessonNew_CommitsDurableCreatedEvent(t *testing.T) {
	root := setupLintCleanProject(t)
	if err := projectdef.WriteSpecConfig(root, lessonTestConfig()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runLesson(t, "new", "durable-created-event", "--project", root); err != nil {
		t.Fatal(err)
	}
	outboxRoot := filepath.Join(root, ".specscore", "event-outbox")
	ledgerEntries, err := os.ReadDir(filepath.Join(outboxRoot, "ledger"))
	if err != nil || len(ledgerEntries) != 1 {
		t.Fatalf("lesson new ledger entries = %d err=%v", len(ledgerEntries), err)
	}
	ledgerBytes, err := os.ReadFile(filepath.Join(outboxRoot, "ledger", ledgerEntries[0].Name()))
	if err != nil {
		t.Fatal(err)
	}
	var record struct {
		Event struct {
			Name     string `json:"name"`
			UUID     string `json:"uuid"`
			Artifact struct {
				ID string `json:"id"`
			} `json:"artifact"`
		} `json:"event"`
		Subscribers []string `json:"subscribers"`
		State       string   `json:"state"`
	}
	if err := json.Unmarshal(ledgerBytes, &record); err != nil {
		t.Fatal(err)
	}
	if record.Event.Name != "lesson.created" || record.Event.Artifact.ID != "durable-created-event" || record.Event.UUID == "" || len(record.Subscribers) != 1 || record.State != "prepared" {
		t.Fatalf("unexpected immutable created-event ledger: %#v", record)
	}
	state, err := os.ReadFile(filepath.Join(outboxRoot, "state", record.Event.UUID))
	if err != nil || strings.TrimSpace(string(state)) != "committed" {
		t.Fatalf("created event was not committed: %q err=%v", state, err)
	}
}

// AC: scaffolded-lesson-is-lint-clean — on a lint-clean project, lesson new
// introduces no error-severity violations anywhere in the tree.
func TestLessonNew_LintCleanOutsideFile(t *testing.T) {
	root := setupLintCleanProject(t)
	if err := projectdef.WriteSpecConfig(root, lessonTestConfig()); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runLesson(t, "new", "clean-lesson", "--project", root); err != nil {
		t.Fatalf("lesson new: %v", err)
	}
	specSub := filepath.Join(root, "spec")
	violations, err := lint.Lint(lint.Options{SpecRoot: specSub})
	if err != nil {
		t.Fatal(err)
	}
	for _, v := range violations {
		if v.Severity == "error" {
			t.Errorf("unexpected error-severity violation: %s:%d [%s] %s",
				v.File, v.Line, v.Rule, v.Message)
		}
	}
}
