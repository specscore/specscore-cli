package cli

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

func TestLessonAgentsReadsOfflineProjectionAndStreamsNeutralHook(t *testing.T) {
	root := canonicalLessonProject(t)
	projection := `{"version":"1","agents":[{"id":"codex-1","role":"implementer","state":"working","observed_at":"2026-08-10T12:00:00Z","url":"https://example.test/tasks/1","source_event_id":"event-1"}]}`
	path := filepath.Join(root, "spec", "lessons", "review-before-merge", "agents.json")
	if err := os.WriteFile(path, []byte(projection), 0o644); err != nil {
		t.Fatal(err)
	}
	out, _, err := runLesson(t, "agents", "review-before-merge", "--format=json")
	if err != nil || !strings.Contains(out, `"codex-1"`) {
		t.Fatalf("offline out=%q err=%v", out, err)
	}
	t.Setenv(lessonAgentsHookEnv, "/bin/cat")
	out, _, err = runLesson(t, "agents", "review-before-merge", "--message", "codex-1", "--text", "review it")
	if err != nil || !strings.Contains(out, `"action":"message"`) || !strings.Contains(out, `"agent_id":"codex-1"`) {
		t.Fatalf("hook out=%q err=%v", out, err)
	}
	// Explicit project selection must become the executable's working directory.
	t.Setenv(lessonAgentsHookEnv, "/bin/pwd")
	out, _, err = runLesson(t, "agents", "review-before-merge", "--project", root, "--refresh")
	if err != nil || strings.TrimSpace(out) != root {
		t.Fatalf("project-anchored hook out=%q err=%v want=%q", out, err, root)
	}
}

func TestLessonAgentsRefusesInvalidProjectionAndActions(t *testing.T) {
	root := canonicalLessonProject(t)
	path := filepath.Join(root, "spec", "lessons", "review-before-merge", "agents.json")
	if err := os.WriteFile(path, []byte(`{"version":"2"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, _, err := runLesson(t, "agents", "review-before-merge")
	if exitCodeOfErr(err) != exitcode.InvalidState {
		t.Fatalf("bad projection exit=%d err=%v", exitCodeOfErr(err), err)
	}
	for _, args := range [][]string{{"agents", "review-before-merge", "--message", "codex-1"}, {"agents", "review-before-merge", "--refresh", "--open", "codex-1"}, {"agents", "review-before-merge", "--open", "codex-1"}} {
		_, _, err = runLesson(t, args...)
		if exitCodeOfErr(err) != exitcode.InvalidArgs && exitCodeOfErr(err) != exitcode.InvalidState {
			t.Fatalf("args=%v exit=%d err=%v", args, exitCodeOfErr(err), err)
		}
	}
	lessons := filepath.Join(root, "spec", "lessons")
	writeLessonInDir(t, lessons, "flat", "Recorded")
	_, _, err = runLesson(t, "agents", "flat")
	if exitCodeOfErr(err) != exitcode.InvalidState {
		t.Fatalf("flat exit=%d err=%v", exitCodeOfErr(err), err)
	}
}

func TestLessonAgentsProjectionValidationAndFormats(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "agents.json")
	for _, tc := range []struct {
		name string
		body string
		want string
	}{
		{"invalid-json", `{`, "invalid JSON"},
		{"unsupported-version", `{"version":"2"}`, "unsupported projection version"},
		{"missing-agent-id", `{"version":"1","agents":[{}]}`, "agent id is required"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if err := os.WriteFile(path, []byte(tc.body), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := readLessonAgentsProjection(path)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("err = %v, want %q", err, tc.want)
			}
		})
	}
	if _, err := readLessonAgentsProjection(filepath.Join(dir, "missing.json")); !os.IsNotExist(err) {
		t.Fatalf("missing projection err = %v, want not-exist", err)
	}
	p := lessonAgentsProjection{Version: "1", Agents: []lessonAgent{{ID: "a", Role: "reviewer", State: "idle", URL: "https://example.test/a"}}}
	for _, format := range []string{"json", "yaml", "text"} {
		t.Run(format, func(t *testing.T) {
			var out strings.Builder
			if err := writeLessonAgentsProjection(&out, format, p); err != nil {
				t.Fatalf("write %s: %v", format, err)
			}
			if !strings.Contains(out.String(), "a") {
				t.Fatalf("%s output omitted agent: %q", format, out.String())
			}
		})
	}
	for _, format := range []string{"json", "yaml", "text"} {
		t.Run(format+"-writer-error", func(t *testing.T) {
			if err := writeLessonAgentsProjection(&errWriter{}, format, p); err == nil {
				t.Fatalf("expected %s writer error", format)
			}
		})
	}
}

func TestLessonAgentsActionAndHookFailures(t *testing.T) {
	root := canonicalLessonProject(t)
	// No projection is a normal offline absence, not an implicit network read.
	_, _, err := runLesson(t, "agents", "review-before-merge")
	if exitCodeOfErr(err) != exitcode.NotFound {
		t.Fatalf("missing projection exit=%d err=%v", exitCodeOfErr(err), err)
	}
	path := filepath.Join(root, "spec", "lessons", "review-before-merge", "README.md")
	cmd := lessonAgentsCommand()
	cmd.SetContext(context.Background())
	t.Setenv(lessonAgentsHookEnv, "")
	if err := invokeLessonAgentsHook(cmd, "refresh", root, path, "review-before-merge", "", ""); exitCodeOfErr(err) != exitcode.InvalidState {
		t.Fatalf("missing hook exit=%d err=%v", exitCodeOfErr(err), err)
	}
	t.Setenv(lessonAgentsHookEnv, "/usr/bin/false")
	if err := invokeLessonAgentsHook(cmd, "refresh", root, path, "review-before-merge", "", ""); err == nil {
		t.Fatal("expected failed hook")
	}
	t.Setenv(lessonAgentsHookEnv, "/bin/cat")
	out, _, err := runLesson(t, "agents", "review-before-merge", "--resume", "codex-1")
	if err != nil || !strings.Contains(out, `"action":"resume"`) {
		t.Fatalf("resume out=%q err=%v", out, err)
	}
	if _, _, err := runLesson(t, "agents", "review-before-merge", "--format=bogus"); exitCodeOfErr(err) != exitcode.InvalidArgs {
		t.Fatalf("bad format exit=%d err=%v", exitCodeOfErr(err), err)
	}
}
