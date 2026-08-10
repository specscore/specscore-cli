package cli

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/lint"
	"github.com/specscore/specscore-cli/pkg/projectdef"
)

func runGitForFlatMigration(t *testing.T, root string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
	}
}

func TestLessonMigrateFlat_EndToEndIsLintCleanAndBounded(t *testing.T) {
	root := setupLintCleanProject(t)
	physicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	root = physicalRoot
	if err := projectdef.WriteSpecConfig(root, lessonTestConfig()); err != nil {
		t.Fatal(err)
	}
	lessonsDir := filepath.Join(root, "spec", "lessons")
	if err := os.MkdirAll(lessonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	index := lessonsIndexContent(lessonTestConfig())
	if err := os.WriteFile(filepath.Join(lessonsDir, "README.md"), []byte(index), 0o644); err != nil {
		t.Fatal(err)
	}
	flat := `---
format: https://specscore.md/lesson-specification
status: Recorded
---

# Lesson: Validate before publishing

**Status:** Recorded
**Date:** 2026-08-01
**Owner:** codex
**Recurred:** 1

## Incident

An incident happened.

## Process gap

No publish-time validation ran.

## Check

Validate the artifact before publishing it.

## Enforcement

Recorded.

## Tracking

Historical tracking.

## Recurrences

- 2026-08-02 — It happened again.

---
*This document follows the https://specscore.md/lesson-specification*
`
	flatPath := filepath.Join(lessonsDir, "validate-before-publishing.md")
	if err := os.WriteFile(flatPath, []byte(flat), 0o644); err != nil {
		t.Fatal(err)
	}
	unrelatedPath := filepath.Join(root, "spec", "ideas", "README.md")
	unrelatedBefore, err := os.ReadFile(unrelatedPath)
	if err != nil {
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

	out, stderr, err := runLesson(t, "migrate-flat", "validate-before-publishing", "--classification", "process", "--format", "json", "--project", root)
	if err != nil {
		t.Fatalf("migrate-flat: %v\nstderr=%s", err, stderr)
	}
	if !strings.Contains(out, `"created_occurrences"`) {
		t.Fatalf("unexpected output: %s", out)
	}
	if _, err := os.Stat(flatPath); !os.IsNotExist(err) {
		t.Fatalf("flat source retained: %v", err)
	}
	canonical := filepath.Join(lessonsDir, "validate-before-publishing", "README.md")
	if _, err := os.Stat(canonical); err != nil {
		t.Fatal(err)
	}
	if current, _ := os.ReadFile(unrelatedPath); !bytes.Equal(current, unrelatedBefore) {
		t.Fatal("focused migration rewrote an unrelated spec file")
	}
	violations, err := lint.Lint(lint.Options{SpecRoot: filepath.Join(root, "spec")})
	if err != nil {
		t.Fatal(err)
	}
	for _, violation := range violations {
		if violation.Severity == "error" && (strings.Contains(violation.File, "validate-before-publishing") || violation.File == "lessons/README.md") {
			t.Errorf("migrated Lesson/index lint error: %#v", violation)
		}
	}

	beforeSecond := treeDigestForCLI(t, lessonsDir)
	if _, _, err := runLesson(t, "migrate-flat", "validate-before-publishing", "--classification", "process", "--project", root); err != nil {
		t.Fatal(err)
	}
	if afterSecond := treeDigestForCLI(t, lessonsDir); !bytes.Equal(beforeSecond, afterSecond) {
		t.Fatal("second migration changed Lesson artifacts")
	}
}

func TestLessonMigrateFlat_ResumesEveryDurableBoundary(t *testing.T) {
	for _, phase := range []string{"artifact-publication", "index-upsert", "event-commit"} {
		t.Run(phase, func(t *testing.T) {
			root := setupFlatMigrationCLIProject(t, "resume-boundary")
			original := lessonMigrateFlatPhaseHook
			injected := false
			lessonMigrateFlatPhaseHook = func(actual string) error {
				if actual == phase && !injected {
					injected = true
					return errors.New("simulated process stop")
				}
				return nil
			}
			t.Cleanup(func() { lessonMigrateFlatPhaseHook = original })

			if _, _, err := runLesson(t, "migrate-flat", "resume-boundary", "--classification", "process", "--project", root); err == nil {
				t.Fatal("expected injected crash")
			}
			marker := filepath.Join(root, "spec", "lessons", ".flat-migration-resume-boundary.json")
			if _, err := os.Stat(marker); err != nil {
				t.Fatalf("durable marker missing after %s: %v", phase, err)
			}
			if _, err := os.Stat(filepath.Join(root, "spec", "lessons", "resume-boundary.md")); !os.IsNotExist(err) {
				t.Fatalf("verified flat source was not removed before %s: %v", phase, err)
			}

			lessonMigrateFlatPhaseHook = nil
			if _, stderr, err := runLesson(t, "migrate-flat", "resume-boundary", "--classification", "process", "--project", root); err != nil {
				t.Fatalf("resume after %s: %v\nstderr=%s", phase, err, stderr)
			}
			if _, err := os.Stat(marker); !os.IsNotExist(err) {
				t.Fatalf("successful resume did not retire marker: %v", err)
			}
			index, err := os.ReadFile(filepath.Join(root, "spec", "lessons", "README.md"))
			if err != nil || bytes.Count(index, []byte("[resume-boundary](resume-boundary/README.md)")) != 1 {
				t.Fatalf("resume did not publish exactly one index row: err=%v\n%s", err, index)
			}
		})
	}
}

func setupFlatMigrationCLIProject(t *testing.T, slug string) string {
	t.Helper()
	root := setupLintCleanProject(t)
	physicalRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		t.Fatal(err)
	}
	root = physicalRoot
	if err := projectdef.WriteSpecConfig(root, lessonTestConfig()); err != nil {
		t.Fatal(err)
	}
	lessonsDir := filepath.Join(root, "spec", "lessons")
	if err := os.MkdirAll(lessonsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(lessonsDir, "README.md"), []byte(lessonsIndexContent(lessonTestConfig())), 0o644); err != nil {
		t.Fatal(err)
	}
	body := `---
format: https://specscore.md/lesson-specification
status: Recorded
---

# Lesson: Resume every durable boundary

**Status:** Recorded
**Date:** 2026-08-01
**Owner:** codex
**Recurred:** 0

## Incident

An interrupted migration was observed.

## Process gap

The transaction boundary ended before its index and event.

## Check

Resume one deterministic transaction through finalization.

## Enforcement

Recorded.

## Tracking

Historical tracking.

---
*This document follows the https://specscore.md/lesson-specification*
`
	if err := os.WriteFile(filepath.Join(lessonsDir, slug+".md"), []byte(body), 0o644); err != nil {
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
	return root
}

func treeDigestForCLI(t *testing.T, root string) []byte {
	t.Helper()
	var out bytes.Buffer
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, _ := filepath.Rel(root, path)
		b, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		out.WriteString(filepath.ToSlash(rel))
		out.WriteByte(0)
		out.Write(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
