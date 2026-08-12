package cli

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lifecycle"
)

func TestTaskAmend_BoardReplaceAndClearPreservesLifecycleAndAudit(t *testing.T) {
	_, path := stageTaskWithStatus(t, "auth", "complete")
	seed := "**Implemented-by:** repo@abc1234\n**Note:** awaiting merge\n**Evidence:** old-pr\n"
	b, _ := os.ReadFile(path)
	if err := os.WriteFile(path, []byte(strings.Replace(string(b), "\nTask body.", "\n"+seed+"\nTask body.", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(path)
	taskAmendNow = func() time.Time { return time.Date(2026, 8, 12, 9, 30, 0, 0, time.UTC) }
	t.Cleanup(func() { taskAmendNow = time.Now })
	out, _, err := runTask(t, "amend", "auth", "--note", "merged", "--evidence", "main-sha, pr-140", "--actor", "codex", "--reason", "landed")
	if err != nil {
		t.Fatal(err)
	}
	if out != "auth: annotations amended\n" {
		t.Fatalf("stdout=%q", out)
	}
	after, _ := os.ReadFile(path)
	text := string(after)
	for _, want := range []string{"**Status:** complete", "**Implemented-by:** repo@abc1234", "**Note:** merged", "**Evidence:** main-sha, pr-140", "**Annotation Amendment:** actor=codex; at=2026-08-12T09:30:00Z; reason=landed; before_sha256=" + fmtDigest(before)} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q:\n%s", want, text)
		}
	}
	if strings.Count(text, "**Note:**") != 1 || strings.Count(text, "**Evidence:**") != 1 {
		t.Fatalf("annotations not singleton:\n%s", text)
	}
	if _, _, err := runTask(t, "amend", "auth", "--clear-note", "--clear-evidence", "--actor", "codex", "--reason", "obsolete"); err != nil {
		t.Fatal(err)
	}
	text = string(mustRead(path))
	if strings.Contains(text, "**Note:**") || strings.Contains(text, "**Evidence:**") || strings.Count(text, "**Annotation Amendment:**") != 2 {
		t.Fatalf("clear/audit failed:\n%s", text)
	}
}

func fmtDigest(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }

func TestTaskAmend_StatusAgnosticAndPlanDirectory(t *testing.T) {
	root, path := stagePlanWithTasks(t, "auth", twoTaskPlanBody)
	dir := filepath.Join(root, "spec", "plans", "auth")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	dirPath := filepath.Join(dir, "README.md")
	b, _ := os.ReadFile(path)
	if err := os.WriteFile(dirPath, b, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runTask(t, "amend", "setup", "--plan", "auth", "--note", "waiting", "--actor", "agent", "--reason", "clarify"); err != nil {
		t.Fatal(err)
	}
	got := string(mustRead(dirPath))
	if !strings.Contains(got, "**Status:** in_progress") || !strings.Contains(got, "**Note:** waiting") {
		t.Fatalf("directory plan amend failed:\n%s", got)
	}
}

func TestTaskAmend_RejectsDuplicateMalformedAndInvalidArgsWithoutWrites(t *testing.T) {
	_, path := stageTaskWithStatus(t, "auth", "blocked")
	b, _ := os.ReadFile(path)
	dirty := strings.Replace(string(b), "Task body.", "**Note:** one\n**Note:** two\nTask body.", 1)
	if err := os.WriteFile(path, []byte(dirty), 0o644); err != nil {
		t.Fatal(err)
	}
	before := string(mustRead(path))
	_, _, err := runTask(t, "amend", "auth", "--note", "truth", "--actor", "a", "--reason", "r")
	if exitCodeOfErr(err) != exitcode.InvalidArgs {
		t.Fatalf("duplicate exit=%v", err)
	}
	if string(mustRead(path)) != before {
		t.Fatal("duplicate mutated")
	}
	_, _, err = runTask(t, "amend", "auth", "--note", "", "--actor", "a", "--reason", "r")
	if exitCodeOfErr(err) != exitcode.InvalidArgs {
		t.Fatalf("blank exit=%v", err)
	}
	_, _, err = runTask(t, "amend", "auth", "--clear-note", "--actor", "", "--reason", "r")
	if exitCodeOfErr(err) != exitcode.InvalidArgs {
		t.Fatalf("actor exit=%v", err)
	}
}

func TestTaskAmend_FlagAndResolutionFailuresAreWriteFree(t *testing.T) {
	root, path := stageTaskWithStatus(t, "auth", "blocked")
	before := string(mustRead(path))
	cases := [][]string{
		{"--actor", "a", "--reason", "r"},
		{"--note", "x", "--clear-note", "--actor", "a", "--reason", "r"},
		{"--evidence", "x", "--clear-evidence", "--actor", "a", "--reason", "r"},
		{"--note", "x", "--actor", "a", "--reason", "bad\nreason"},
	}
	for _, args := range cases {
		_, _, err := runTask(t, append([]string{"amend", "auth"}, args...)...)
		if exitCodeOfErr(err) != exitcode.InvalidArgs {
			t.Fatalf("args %v: %v", args, err)
		}
	}
	if string(mustRead(path)) != before {
		t.Fatal("invalid args wrote task")
	}
	_, _, err := runTask(t, "amend", "ghost", "--note", "x", "--actor", "a", "--reason", "r")
	if exitCodeOfErr(err) != exitcode.NotFound {
		t.Fatalf("missing=%v", err)
	}
	flat := filepath.Join(root, "tasks", "flat.md")
	if err := os.WriteFile(flat, []byte("# Flat\n\n**Status:** blocked\n\nBody\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, _, err = runTask(t, "amend", "flat", "--note", "fixed", "--actor", "a", "--reason", "r"); err != nil {
		t.Fatal(err)
	}
}

func TestTaskAmend_MalformedStatusAndCASFailuresAreWriteFree(t *testing.T) {
	_, path := stageTaskWithStatus(t, "auth", "blocked")
	for _, body := range []string{"# Auth\n\nNo status\n", "# Auth\n\n**Status:** blocked\n**Status:** blocked\n"} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		before := string(mustRead(path))
		_, _, err := runTask(t, "amend", "auth", "--note", "fixed", "--actor", "a", "--reason", "r")
		if exitCodeOfErr(err) != exitcode.Unexpected {
			t.Fatalf("status body=%q err=%v", body, err)
		}
		if string(mustRead(path)) != before {
			t.Fatal("malformed status wrote")
		}
	}
	_, path = stageTaskWithStatus(t, "auth", "blocked")
	orig := taskAmendCAS
	taskAmendCAS = func(string, []byte, []byte) error { return lifecycle.ErrConcurrentMutation }
	t.Cleanup(func() { taskAmendCAS = orig })
	before := string(mustRead(path))
	_, _, err := runTask(t, "amend", "auth", "--note", "fixed", "--actor", "a", "--reason", "r")
	if exitCodeOfErr(err) != exitcode.Conflict {
		t.Fatalf("CAS conflict=%v", err)
	}
	if string(mustRead(path)) != before {
		t.Fatal("CAS conflict wrote")
	}
	taskAmendCAS = func(string, []byte, []byte) error { return errors.New("write fence") }
	_, _, err = runTask(t, "amend", "auth", "--note", "fixed", "--actor", "a", "--reason", "r")
	if exitCodeOfErr(err) != exitcode.Unexpected {
		t.Fatalf("write fence=%v", err)
	}
}
