package cli

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/specscore/specscore-cli/pkg/event"
	"github.com/specscore/specscore-cli/pkg/exitcode"
	"github.com/specscore/specscore-cli/pkg/lesson"
	"github.com/spf13/cobra"
)

func TestLessonRelationAndOccurrenceReadJourneysAreParseable(t *testing.T) {
	root := canonicalLessonProject(t)
	if _, _, err := runLesson(t, "new", "second-rule", "--owner", "tester"); err != nil {
		t.Fatal(err)
	}
	token := lesson.RelationToken("review-before-merge", "related", "second-rule")
	preview, _, err := runLesson(t, "relation", "add", "review-before-merge", "second-rule", "--type", "related")
	if exitCodeOfErr(err) != exitcode.InvalidArgs || !strings.Contains(preview, token) {
		t.Fatalf("relation preview = %q, err=%v", preview, err)
	}
	if _, _, err := runLesson(t, "relation", "add", "review-before-merge", "second-rule", "--type", "related", "--confirm", token); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runLesson(t, "relation", "add", "review-before-merge", "second-rule", "--type", "invalid"); exitCodeOfErr(err) != exitcode.InvalidArgs {
		t.Fatalf("invalid relation exit=%d err=%v", exitCodeOfErr(err), err)
	}
	added, _, err := runLesson(t, "occurrence", "add", "review-before-merge", "--summary", "visible fact", "--context-json", `{"execution":{"kind":"automation"}}`)
	requireCLISuccess(t, err)
	id := strings.Fields(added)[2]
	for _, format := range []string{"json", "yaml", "text"} {
		relations, _, err := runLesson(t, "relation", "list", "review-before-merge", "--format", format)
		if err != nil {
			t.Fatalf("relation %s: %v", format, err)
		}
		assertStructuredOutput(t, format, relations)
		items, _, err := runLesson(t, "occurrence", "list", "review-before-merge", "--format", format)
		if err != nil {
			t.Fatalf("occurrence list %s: %v", format, err)
		}
		assertStructuredOutput(t, format, items)
		info, _, err := runLesson(t, "occurrence", "info", "review-before-merge", id, "--format", format)
		if err != nil {
			t.Fatalf("occurrence info %s: %v", format, err)
		}
		assertStructuredOutput(t, format, info)
	}
	for _, args := range [][]string{
		{"relation", "list", "review-before-merge", "--format", "bogus"},
		{"occurrence", "list", "review-before-merge", "--format", "bogus"},
		{"occurrence", "info", "review-before-merge", id, "--format", "bogus"},
		{"occurrence", "info", "review-before-merge", uuid.NewString()},
	} {
		if _, _, err := runLesson(t, args...); err == nil {
			t.Fatalf("invalid read accepted: %v", args)
		}
	}
	badID := uuid.NewString()
	badPath := filepath.Join(root, "spec", "lessons", "review-before-merge", "occurrences", badID+".json")
	requireCLISuccess(t, os.WriteFile(badPath, []byte("{"), 0o644))
	if _, _, err := runLesson(t, "occurrence", "info", "review-before-merge", badID); err == nil {
		t.Fatal("malformed occurrence info was accepted")
	}
	if _, _, err := runLesson(t, "occurrence", "list", "review-before-merge"); err == nil {
		t.Fatal("malformed occurrence list was accepted")
	}
}

func TestOccurrenceInputsAndTransactionFailures(t *testing.T) {
	root := canonicalLessonProject(t)
	contextPath := filepath.Join(root, "context.json")
	requireCLISuccess(t, os.WriteFile(contextPath, []byte(`{"key":"value"}`), 0o644))
	for _, tc := range []struct {
		name  string
		flags map[string]string
		stdin any
		fail  bool
	}{
		{"default", nil, nil, false},
		{"json", map[string]string{"context-json": `{"key":"value"}`}, nil, false},
		{"file", map[string]string{"context-file": contextPath}, nil, false},
		{"stdin", map[string]string{"context-stdin": "true"}, strings.NewReader(`{"key":"value"}`), false},
		{"exclusive", map[string]string{"context-json": `{}`, "context-file": contextPath}, nil, true},
		{"missing-file", map[string]string{"context-file": contextPath + ".missing"}, nil, true},
		{"bad-json", map[string]string{"context-json": `[]`}, nil, true},
		{"stdin-error", map[string]string{"context-stdin": "true"}, &testErrReader{}, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := lessonOccurrenceAddCommand()
			setLessonCommandFlags(t, cmd, tc.flags)
			if reader, ok := tc.stdin.(interface{ Read([]byte) (int, error) }); ok {
				cmd.SetIn(reader)
			}
			_, _, err := occurrenceContextInput(cmd)
			if (err != nil) != tc.fail {
				t.Fatalf("err=%v fail=%t", err, tc.fail)
			}
		})
	}
	t.Setenv("SPECSCORE_ACTOR", "agent-7")
	if execution := captureOccurrenceContext("", "")["execution"].(map[string]any); execution["id"] != "agent-7" {
		t.Fatalf("actor capture = %#v", execution)
	}
	if ctx := captureOccurrenceContext(t.TempDir(), ""); ctx["worktree"] == nil {
		t.Fatalf("non-git context lost redacted worktree: %#v", ctx)
	}

	configureNoopLessonEvents(t, root)
	for _, phase := range []string{"snapshot-stat", "snapshot-read", "prepare", "record-add", "record-recur", "commit"} {
		t.Run(phase, func(t *testing.T) {
			deps := defaultLessonCLIDeps()
			indexPath := filepath.Join(root, "spec", "lessons", "README.md")
			originalIndex, _ := os.ReadFile(indexPath)
			if phase == "snapshot-stat" {
				_ = os.Remove(indexPath)
				t.Cleanup(func() { _ = os.WriteFile(indexPath, originalIndex, 0o644) })
			}
			if phase == "snapshot-read" {
				_ = os.Remove(indexPath)
				_ = os.Mkdir(indexPath, 0o755)
				t.Cleanup(func() { _ = os.Remove(indexPath); _ = os.WriteFile(indexPath, originalIndex, 0o644) })
			}
			if phase == "prepare" {
				deps.prepareEvent = func(string, string, string, map[string]any, time.Time) (*preparedLessonEvent, error) {
					return nil, errors.New("prepare")
				}
			}
			if strings.HasPrefix(phase, "record") {
				deps.addOccurrence = func(lesson.AddOccurrenceOptions) (lesson.Occurrence, error) {
					return lesson.Occurrence{}, &lesson.MutationError{Outcome: lesson.MutationPrePublication, Err: errors.New("refused")}
				}
			}
			if phase == "commit" {
				deps.prepareEvent = func(string, string, string, map[string]any, time.Time) (*preparedLessonEvent, error) {
					return &preparedLessonEvent{outbox: event.NewOutbox(root), event: event.Event{UUID: uuid.NewString()}}, nil
				}
			}
			var err error
			if phase == "record-recur" {
				cmd := lessonRecurCommand()
				setLessonCommandFlags(t, cmd, nil)
				err = runLessonRecurWithDeps(cmd, []string{"review-before-merge"}, deps)
			} else {
				cmd := lessonOccurrenceAddCommand()
				setLessonCommandFlags(t, cmd, nil)
				err = runLessonOccurrenceAddWithDeps(cmd, []string{"review-before-merge"}, deps)
			}
			requireCLIError(t, err)
		})
	}
}

func TestOccurrenceAndRelationWriterFailures(t *testing.T) {
	root := canonicalLessonProject(t)
	added, _, err := runLesson(t, "occurrence", "add", "review-before-merge")
	requireCLISuccess(t, err)
	id := strings.Fields(added)[2]
	for _, format := range []string{"json", "yaml"} {
		for _, tc := range []struct {
			name string
			cmd  *cobra.Command
			run  func(*cobra.Command) error
		}{
			{"list", lessonOccurrenceListCommand(), func(c *cobra.Command) error { return runLessonOccurrenceList(c, []string{"review-before-merge"}) }},
			{"info", lessonOccurrenceInfoCommand(), func(c *cobra.Command) error { return runLessonOccurrenceInfo(c, []string{"review-before-merge", id}) }},
			{"relation", lessonRelationListCommand(), func(c *cobra.Command) error { return runLessonRelationList(c, []string{"review-before-merge"}) }},
		} {
			t.Run(tc.name+"-"+format, func(t *testing.T) {
				setLessonCommandFlags(t, tc.cmd, map[string]string{"format": format, "project": root})
				tc.cmd.SetOut(&errWriter{})
				requireCLIError(t, tc.run(tc.cmd))
			})
		}
	}
}
