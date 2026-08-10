package cli

import (
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
