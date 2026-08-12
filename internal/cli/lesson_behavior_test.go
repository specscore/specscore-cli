package cli

import (
	"bytes"
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

func TestLessonRelationCompletedSecondRunIsByteAndEventNoop(t *testing.T) {
	for _, typ := range []string{"related", "duplicates", "supersedes"} {
		t.Run(typ, func(t *testing.T) {
			root := canonicalLessonProject(t)
			configureNoopLessonEvents(t, root)
			if _, stderr, err := runLesson(t, "new", "second-rule", "--owner", "tester"); err != nil {
				t.Fatalf("second Lesson: %v\nstderr=%s", err, stderr)
			}
			from, to := "review-before-merge", "second-rule"
			token := lesson.RelationToken(from, typ, to)
			if _, stderr, err := runLesson(t, "relation", "add", from, to, "--type", typ, "--confirm", token); err != nil {
				t.Fatalf("first relation: %v\nstderr=%s", err, stderr)
			}
			lessonsBefore := treeDigestForCLI(t, filepath.Join(root, "spec", "lessons"))
			outboxRoot := filepath.Join(root, ".specscore", "event-outbox")
			outboxBefore := treeDigestForCLI(t, outboxRoot)
			ledgerBefore := treeDigestForCLI(t, filepath.Join(outboxRoot, "ledger"))
			if _, stderr, err := runLesson(t, "relation", "add", from, to, "--type", typ, "--confirm", token); err != nil {
				t.Fatalf("completed second relation: %v\nstderr=%s", err, stderr)
			}
			if got := treeDigestForCLI(t, filepath.Join(root, "spec", "lessons")); !bytes.Equal(got, lessonsBefore) {
				t.Fatal("completed second relation changed Lesson artifacts or index bytes")
			}
			if got := treeDigestForCLI(t, outboxRoot); !bytes.Equal(got, outboxBefore) {
				t.Fatal("completed second relation changed outbox bytes")
			}
			if got := treeDigestForCLI(t, filepath.Join(outboxRoot, "ledger")); !bytes.Equal(got, ledgerBefore) {
				t.Fatal("completed second relation created or changed an event ledger")
			}
			if typ == "related" {
				reversed := lesson.RelationToken(to, typ, from)
				if _, stderr, err := runLesson(t, "relation", "add", to, from, "--type", typ, "--confirm", reversed); err != nil {
					t.Fatalf("reversed completed relation: %v\nstderr=%s", err, stderr)
				}
				if got := treeDigestForCLI(t, outboxRoot); !bytes.Equal(got, outboxBefore) {
					t.Fatal("reversed completed relation changed outbox bytes")
				}
			}
		})
	}
}

func TestLessonRelationCompletedArtifactRetryFinishesOriginalPreparedEvent(t *testing.T) {
	root := canonicalLessonProject(t)
	configureNoopLessonEvents(t, root)
	if _, stderr, err := runLesson(t, "new", "second-rule", "--owner", "tester"); err != nil {
		t.Fatalf("second Lesson: %v\nstderr=%s", err, stderr)
	}
	from, to, typ := "review-before-merge", "second-rule", "related"
	cmd := lessonRelationAddCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"project": root, "type": typ, "confirm": lesson.RelationToken(from, typ, to)})
	deps := defaultLessonCLIDeps()
	realTransaction := deps.addRelationTransaction
	deps.addRelationTransaction = func(dir, gotFrom, gotType, gotTo string, hooks lesson.RelationTransactionHooks) (bool, error) {
		hooks.PostMutation = func() error { return errors.New("injected reconciliation interruption after relation publication") }
		return realTransaction(dir, gotFrom, gotType, gotTo, hooks)
	}
	if err := runLessonRelationAddWithDeps(cmd, []string{from, to}, deps); err == nil || !strings.Contains(err.Error(), "prepared event") {
		t.Fatalf("first interrupted relation = %v", err)
	}
	outbox := event.NewOutbox(root)
	prepared, err := outbox.Prepared()
	requireCLISuccess(t, err)
	if len(prepared) != 1 || prepared[0].EventName != "lesson.relation-recorded" {
		t.Fatalf("interrupted relation prepared events = %#v", prepared)
	}
	ledgerBeforeRetry := treeDigestForCLI(t, filepath.Join(outbox.Root, "ledger"))
	if _, stderr, err := runLesson(t, "relation", "add", from, to, "--type", typ, "--confirm", lesson.RelationToken(from, typ, to)); err != nil {
		t.Fatalf("relation recovery retry: %v\nstderr=%s", err, stderr)
	}
	if got := treeDigestForCLI(t, filepath.Join(outbox.Root, "ledger")); !bytes.Equal(got, ledgerBeforeRetry) {
		t.Fatal("relation recovery retry created or changed a second ledger")
	}
	prepared, err = outbox.Prepared()
	requireCLISuccess(t, err)
	if len(prepared) != 0 {
		t.Fatalf("relation recovery retry did not finish original event: %#v", prepared)
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
		{"trailing-json", map[string]string{"context-json": `{} {}`}, nil, true},
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
