package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

func TestLessonAgentsReadsOfflineProjectionAndStreamsGenericExternalResourceHook(t *testing.T) {
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
	var request map[string]json.RawMessage
	if err := json.Unmarshal([]byte(out), &request); err != nil {
		t.Fatalf("decode hook request: %v", err)
	}
	forbiddenFields := map[string]bool{
		"lesson": true, "lesson_slug": true, "lesson_path": true,
		"slug": true, "status": true, "occurrence": true,
		"occurrences": true, "relation": true, "relations": true,
	}
	var decoded any
	if err := json.Unmarshal([]byte(out), &decoded); err != nil {
		t.Fatalf("decode generic hook request: %v", err)
	}
	var rejectLessonFields func(any)
	rejectLessonFields = func(value any) {
		switch typed := value.(type) {
		case map[string]any:
			for key, child := range typed {
				if forbiddenFields[key] {
					t.Errorf("hook request exposes SpecScore-only %q field: %s", key, out)
				}
				rejectLessonFields(child)
			}
		case []any:
			for _, child := range typed {
				rejectLessonFields(child)
			}
		}
	}
	rejectLessonFields(decoded)
	var project struct {
		Root string `json:"root"`
	}
	canonicalRoot, canonicalErr := filepath.EvalSymlinks(root)
	if err := json.Unmarshal(request["project"], &project); err != nil || canonicalErr != nil || project.Root != canonicalRoot {
		t.Fatalf("project context = %+v, err=%v, canonicalErr=%v, want root %q", project, err, canonicalErr, canonicalRoot)
	}
	var resource struct {
		Ref      string `json:"ref"`
		Revision string `json:"revision"`
	}
	if err := json.Unmarshal(request["external_resource"], &resource); err != nil {
		t.Fatalf("decode external resource: %v", err)
	}
	if resource.Ref != "spec/lessons/review-before-merge/README.md" || !strings.HasPrefix(resource.Revision, "sha256:") {
		t.Fatalf("external resource = %+v", resource)
	}
	if got := strings.TrimSpace(string(request["version"])); got != `"2"` {
		t.Fatalf("request version = %s, want 2", got)
	}
	// Explicit project selection must become the executable's working directory.
	t.Setenv(lessonAgentsHookEnv, "/bin/pwd")
	out, _, err = runLesson(t, "agents", "review-before-merge", "--project", root, "--refresh")
	if err != nil || strings.TrimSpace(out) != canonicalRoot {
		t.Fatalf("project-anchored hook out=%q err=%v want=%q", out, err, canonicalRoot)
	}
}

func TestLessonAgentsCanonicalizesSymlinkedProjectContext(t *testing.T) {
	physicalRoot := canonicalLessonProject(t)
	physicalRoot, err := filepath.EvalSymlinks(physicalRoot)
	if err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(t.TempDir(), "project-alias")
	if err := os.Symlink(physicalRoot, alias); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	t.Setenv(lessonAgentsHookEnv, "/bin/cat")
	out, _, err := runLesson(t, "agents", "review-before-merge", "--project", alias, "--refresh")
	if err != nil {
		t.Fatalf("refresh through symlink alias: %v", err)
	}
	var request lessonAgentsRequest
	if err := json.Unmarshal([]byte(out), &request); err != nil {
		t.Fatalf("decode hook request: %v", err)
	}
	if request.Project.Root != physicalRoot {
		t.Fatalf("project.root = %q, want canonical %q", request.Project.Root, physicalRoot)
	}
	if request.ExternalResource.Ref != "spec/lessons/review-before-merge/README.md" {
		t.Fatalf("external_resource.ref = %q", request.ExternalResource.Ref)
	}
	if _, err := canonicalLessonAgentsProjectRoot(filepath.Join(t.TempDir(), "missing")); exitCodeOfErr(err) != exitcode.Unexpected {
		t.Fatalf("missing canonical root exit=%d err=%v", exitCodeOfErr(err), err)
	}
}

func TestLessonAgentsBoundaryHasNoSynchestraBackendAccess(t *testing.T) {
	source, err := os.ReadFile("lesson_agents.go")
	if err != nil {
		t.Fatal(err)
	}
	lower := strings.ToLower(string(source))
	for _, forbidden := range []string{
		`"database/sql"`, `"net/http"`, "github.com/dal-go", "ingitdb",
		"modernc.org/sqlite", "/v1/lesson-agents", `exec.commandcontext(ctx, "git"`,
	} {
		if strings.Contains(lower, forbidden) {
			t.Errorf("Lesson adapter boundary contains forbidden backend access %q", forbidden)
		}
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
	if err := invokeLessonAgentsHook(cmd, "refresh", root, path, "", ""); exitCodeOfErr(err) != exitcode.InvalidState {
		t.Fatalf("missing hook exit=%d err=%v", exitCodeOfErr(err), err)
	}
	t.Setenv(lessonAgentsHookEnv, "/usr/bin/false")
	if err := invokeLessonAgentsHook(cmd, "refresh", root, path, "", ""); err == nil {
		t.Fatal("expected failed hook")
	}
	t.Setenv(lessonAgentsHookEnv, "/bin/cat")
	outside := filepath.Join(t.TempDir(), "README.md")
	if err := os.WriteFile(outside, []byte("external"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := invokeLessonAgentsHook(cmd, "refresh", root, outside, "", ""); exitCodeOfErr(err) != exitcode.Unexpected {
		t.Fatalf("outside resource exit=%d err=%v", exitCodeOfErr(err), err)
	}
	out, _, err := runLesson(t, "agents", "review-before-merge", "--resume", "codex-1")
	if err != nil || !strings.Contains(out, `"action":"resume"`) {
		t.Fatalf("resume out=%q err=%v", out, err)
	}
	if _, _, err := runLesson(t, "agents", "review-before-merge", "--format=bogus"); exitCodeOfErr(err) != exitcode.InvalidArgs {
		t.Fatalf("bad format exit=%d err=%v", exitCodeOfErr(err), err)
	}
}
