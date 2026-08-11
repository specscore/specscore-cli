package cli

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/specscore/specscore-cli/pkg/event"
)

type fakeEventOutbox struct {
	prepared []event.PreparedRecord
	result   event.ReplayResult
	err      error
	action   error
}

func (f *fakeEventOutbox) Replay(context.Context, []event.Subscriber, string, int) (event.ReplayResult, error) {
	return f.result, f.err
}
func (f *fakeEventOutbox) Prepared() ([]event.PreparedRecord, error)     { return f.prepared, f.err }
func (f *fakeEventOutbox) Commit(string) error                           { return f.action }
func (f *fakeEventOutbox) Abort(string) error                            { return f.action }
func (f *fakeEventOutbox) Enqueue(event.Event, []event.Subscriber) error { return f.action }

func eventTestDeps(outbox eventCLIOutbox) eventCLIDeps {
	return eventCLIDeps{func() (string, error) { return "root", nil }, func(s string) (string, error) { return s, nil }, func(string) ([]event.Subscriber, error) { return []event.Subscriber{event.NoOp{}}, nil }, func(string) eventCLIOutbox { return outbox }}
}

// runEvent invokes the event command tree in-process with the given args.
func runEvent(t *testing.T, args ...string) (string, string, error) {
	t.Helper()
	cmd := eventCommand()
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs(args)
	err := cmd.Execute()
	return out.String(), errOut.String(), err
}

func TestEventAdapterFailures(t *testing.T) {
	cmd := eventReplayCommand()
	requireCLIError(t, runEventReplayWithDeps(cmd, defaultEventCLIDeps()))
	cmd = eventReplayCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"subscriber": "sink", "limit": "-1"})
	requireCLIError(t, runEventReplayWithDeps(cmd, defaultEventCLIDeps()))
	for _, phase := range []string{"getwd", "root", "load", "replay", "failed"} {
		cmd = eventReplayCommand()
		setLessonCommandFlags(t, cmd, map[string]string{"subscriber": "sink"})
		fake := &fakeEventOutbox{}
		deps := eventTestDeps(fake)
		switch phase {
		case "getwd":
			deps.getwd = func() (string, error) { return "", errors.New("getwd") }
		case "root":
			deps.findRoot = func(string) (string, error) { return "", errors.New("root") }
		case "load":
			deps.load = func(string) ([]event.Subscriber, error) { return nil, errors.New("load") }
		case "replay":
			fake.err = errors.New("replay")
		case "failed":
			fake.result.Failed = []event.SubscriberFailure{{Name: "sink", Err: errors.New("delivery")}}
		}
		requireCLIError(t, runEventReplayWithDeps(cmd, deps))
	}

	for _, phase := range []string{"decision", "getwd", "root", "prepared", "missing", "action"} {
		cmd = eventReconcileCommand()
		fake := &fakeEventOutbox{}
		deps := eventTestDeps(fake)
		record := event.PreparedRecord{EventUUID: "id", EventName: "lesson.test", ArtifactPath: "spec/x"}
		fake.prepared = []event.PreparedRecord{record}
		decision := "abort"
		switch phase {
		case "decision":
			decision = "bad"
		case "getwd":
			deps.getwd = func() (string, error) { return "", errors.New("getwd") }
		case "root":
			deps.findRoot = func(string) (string, error) { return "", errors.New("root") }
		case "prepared":
			fake.err = errors.New("prepared")
		case "missing":
			fake.prepared = nil
		case "action":
			fake.action = errors.New("action")
		}
		setLessonCommandFlags(t, cmd, map[string]string{"decision": decision, "confirm": event.ReconciliationToken(record, decision)})
		requireCLIError(t, runEventReconcileWithDeps(cmd, []string{"id"}, deps))
	}
	cmd = eventReconcileCommand()
	record := event.PreparedRecord{EventUUID: "id"}
	setLessonCommandFlags(t, cmd, map[string]string{"decision": "abort", "confirm": event.ReconciliationToken(record, "abort")})
	requireCLISuccess(t, runEventReconcileWithDeps(cmd, []string{"id"}, eventTestDeps(&fakeEventOutbox{prepared: []event.PreparedRecord{record}})))

	cmd = eventEmitCommand()
	setLessonCommandFlags(t, cmd, map[string]string{"name": "lesson.test", "actor-kind": "external", "actor-id": "specscore", "artifact-type": "lesson", "artifact-id": "x", "artifact-path": "spec/lessons/x", "payload-json": "{}"})
	requireCLIError(t, runEventEmitWithDeps(cmd, eventTestDeps(&fakeEventOutbox{err: errors.New("replay")})))
	deps := eventTestDeps(&fakeEventOutbox{})
	deps.getwd = func() (string, error) { return "", errors.New("getwd") }
	requireCLIError(t, runEventEmitWithDeps(cmd, deps))
}

// AC: verb-registers-and-helps — bare `event` exits 0 and lists `emit`.
func TestEventCommand_HelpExitsZero(t *testing.T) {
	out, _, err := runEvent(t)
	if err != nil {
		t.Fatalf("bare `event` returned error: %v", err)
	}
	if !strings.Contains(out, "emit") {
		t.Errorf("expected bare `event` help to list `emit` subcommand; got:\n%s", out)
	}
}

// AC: verb-registers-and-helps — `event --help` exits 0.
func TestEventCommand_ExplicitHelpExitsZero(t *testing.T) {
	out, _, err := runEvent(t, "--help")
	if err != nil {
		t.Fatalf("`event --help` returned error: %v", err)
	}
	if !strings.Contains(out, "emit") {
		t.Errorf("expected `event --help` to list `emit`; got:\n%s", out)
	}
}

// AC: verb-registers-and-helps — `event emit --help` includes the canonical docs link.
func TestEventCommand_HelpHasDocsLink(t *testing.T) {
	out, _, err := runEvent(t, "emit", "--help")
	if err != nil {
		t.Fatalf("`event emit --help` returned error: %v", err)
	}
	const want = "https://specscore.md/event-emit"
	if !strings.Contains(out, want) {
		t.Errorf("expected `event emit --help` output to contain %q; got:\n%s", want, out)
	}
}

// AC: verb-registers-and-helps — `event emit --help` enumerates envelope flags.
// NOTE: Full nine-flag completeness (including --payload-json / --payload-file
// from REQ:payload-input-modes) lands in Task 4. This batch only covers the
// seven REQ:envelope-flags.
func TestEventCommand_HelpShowsEnvelopeFlags(t *testing.T) {
	out, _, err := runEvent(t, "emit", "--help")
	if err != nil {
		t.Fatalf("`event emit --help` returned error: %v", err)
	}
	for _, flag := range []string{
		"--name",
		"--actor-kind",
		"--actor-id",
		"--artifact-type",
		"--artifact-id",
		"--artifact-path",
		"--artifact-revision",
	} {
		if !strings.Contains(out, flag) {
			t.Errorf("expected `event emit --help` to mention %s; got:\n%s", flag, out)
		}
	}
}

// AC: required-flag-missing-fails-2.
//
// NOTE: the AC's example invocation includes `--payload-json '{}'`, but
// payload flags are introduced in Task 4. We omit `--payload-json` here —
// the AC's intent is "missing required flag → exit 2 naming the flag and
// envelope field". The full nine-flag invocation lands in a Task-4
// integration test.
func TestEventEmit_MissingNameFlagFails2(t *testing.T) {
	tmp := t.TempDir()
	withCwd(t, tmp)

	_, stderr, err := runEvent(t,
		"emit",
		"--actor-kind", "skill",
		"--actor-id", "skill:t",
		"--artifact-type", "idea",
		"--artifact-id", "x",
		"--artifact-path", "spec/ideas/x.md",
	)
	if err == nil {
		t.Fatal("expected error for missing --name; got nil")
	}

	type exitCoder interface{ ExitCode() int }
	var ec exitCoder
	if !errors.As(err, &ec) {
		t.Fatalf("error does not carry ExitCode: %v", err)
	}
	if got := ec.ExitCode(); got != 2 {
		t.Errorf("exit code = %d; want 2", got)
	}

	combined := err.Error() + stderr
	if !strings.Contains(combined, "--name") {
		t.Errorf("expected error/stderr to name `--name`; got err=%v stderr=%q", err, stderr)
	}
	// AC requires stderr to also name the envelope field the flag supplies (`name`).
	if !strings.Contains(combined, "envelope field") && !strings.Contains(combined, "envelope `name`") &&
		!strings.Contains(combined, "field `name`") && !strings.Contains(combined, "name`") {
		t.Errorf("expected error/stderr to name envelope field `name`; got err=%v stderr=%q", err, stderr)
	}

	// No event MUST be written to .specscore/events.jsonl.
	jsonl := filepath.Join(tmp, ".specscore", "events.jsonl")
	if _, statErr := os.Stat(jsonl); !os.IsNotExist(statErr) {
		t.Errorf("expected no events.jsonl after failure; stat err: %v", statErr)
	}
}

// Reject extra positional args (REQ:envelope-flags: "MUST NOT accept extra
// positional arguments — only flag-form input").
func TestEventEmit_RejectsPositionalArgs(t *testing.T) {
	tmp := t.TempDir()
	withCwd(t, tmp)

	_, _, err := runEvent(t,
		"emit",
		"unexpected-positional",
		"--name", "idea.drafted",
		"--actor-kind", "skill",
		"--actor-id", "skill:t",
		"--artifact-type", "idea",
		"--artifact-id", "x",
		"--artifact-path", "spec/ideas/x.md",
	)
	if err == nil {
		t.Fatal("expected error for unexpected positional arg; got nil")
	}
}

// uuidV4RE matches the AC regex for a lowercase-hyphenated v4 UUID.
var uuidV4RE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// gitInitWithCommit initialises a real git repo in dir with one empty
// commit so HEAD resolves. Skips the calling test if `git` is missing.
func gitInitWithCommit(t *testing.T, dir string) {
	t.Helper()
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available on PATH")
	}
	for _, args := range [][]string{
		{"init", "-q", "-b", "main"},
		{"config", "user.email", "t@example.com"},
		{"config", "user.name", "T"},
		{"commit", "--allow-empty", "-q", "-m", "initial"},
	} {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, out)
		}
	}
}

// TestAutofillEnvelope_GitRepo (covers AC: envelope-auto-fill-fields).
// Calls autofillEnvelope against a real git repo with no override and
// asserts every auto-filled field matches the AC's contract.
func TestAutofillEnvelope_GitRepo(t *testing.T) {
	dir := t.TempDir()
	gitInitWithCommit(t, dir)

	// Capture the expected SHA via the same command the AC names.
	expectedSHA := func() string {
		cmd := exec.Command("git", "-C", dir, "rev-parse", "HEAD")
		out, err := cmd.Output()
		if err != nil {
			t.Fatalf("git rev-parse HEAD: %v", err)
		}
		return strings.TrimSpace(string(out))
	}()

	before := time.Now().UTC()
	var e event.Event
	autofillEnvelope(&e, dir, "")
	after := time.Now().UTC()

	if e.Version != 1 {
		t.Errorf("Version = %d; want 1", e.Version)
	}
	if !uuidV4RE.MatchString(e.UUID) {
		t.Errorf("UUID = %q; does not match v4 regex %q", e.UUID, uuidV4RE.String())
	}
	// Timestamp must be UTC (Go formats UTC times with `Z` suffix when
	// using time.RFC3339) and within ±5s of the call window.
	if e.Timestamp.Location() != time.UTC {
		t.Errorf("Timestamp location = %v; want UTC", e.Timestamp.Location())
	}
	formatted := e.Timestamp.Format(time.RFC3339)
	if !strings.HasSuffix(formatted, "Z") {
		t.Errorf("Timestamp RFC3339 = %q; want `Z` suffix", formatted)
	}
	if e.Timestamp.Before(before.Add(-5*time.Second)) || e.Timestamp.After(after.Add(5*time.Second)) {
		t.Errorf("Timestamp %v outside ±5s window [%v, %v]", e.Timestamp, before, after)
	}
	if e.Artifact.Revision != expectedSHA {
		t.Errorf("Artifact.Revision = %q; want %q (from `git rev-parse HEAD`)",
			e.Artifact.Revision, expectedSHA)
	}
}

// TestAutofillEnvelope_NoGitRepo (covers AC: envelope-auto-fill-revision-no-git).
// Calls autofillEnvelope against a directory that is NOT a git repo and
// asserts Artifact.Revision is the literal string "uncommitted".
func TestAutofillEnvelope_NoGitRepo(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available on PATH")
	}
	dir := t.TempDir() // no `git init`

	var e event.Event
	autofillEnvelope(&e, dir, "")

	if e.Artifact.Revision != "uncommitted" {
		t.Errorf("Artifact.Revision = %q; want %q", e.Artifact.Revision, "uncommitted")
	}
	// The other auto-fill fields should still be populated correctly even
	// when the git path fails.
	if e.Version != 1 {
		t.Errorf("Version = %d; want 1", e.Version)
	}
	if !uuidV4RE.MatchString(e.UUID) {
		t.Errorf("UUID = %q; does not match v4 regex", e.UUID)
	}
}

// TestResolvePayload_JsonFlag covers AC: payload-json-flag-shape (happy path).
// When --payload-json is set, its bytes ARE the payload.
func TestResolvePayload_JsonFlag(t *testing.T) {
	want := `{"slug":"x","approved":false}`
	got, err := resolvePayload(want, "", strings.NewReader("ignored stdin"), t.TempDir())
	if err != nil {
		t.Fatalf("resolvePayload: %v", err)
	}
	if string(got) != want {
		t.Errorf("payload = %q; want %q", string(got), want)
	}
}

// TestResolvePayload_FileFlag covers AC: payload-file-flag-shape (happy path).
// When --payload-file is set (and --payload-json is empty), read the file's bytes.
func TestResolvePayload_FileFlag(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "p.json")
	want := `{"slug":"x","approved":false}`
	if err := os.WriteFile(path, []byte(want), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	got, err := resolvePayload("", path, strings.NewReader("ignored stdin"), dir)
	if err != nil {
		t.Fatalf("resolvePayload: %v", err)
	}
	if string(got) != want {
		t.Errorf("payload = %q; want %q", string(got), want)
	}
}

// TestResolvePayload_Stdin covers AC: payload-stdin-shape (happy path).
// When neither flag is set, read stdin to EOF.
func TestResolvePayload_Stdin(t *testing.T) {
	want := `{"slug":"x","approved":false}`
	got, err := resolvePayload("", "", strings.NewReader(want), t.TempDir())
	if err != nil {
		t.Fatalf("resolvePayload: %v", err)
	}
	if string(got) != want {
		t.Errorf("payload = %q; want %q", string(got), want)
	}
}

// TestResolvePayload_FileRelativeToProjectRoot: relative --payload-file
// resolves against the project root.
func TestResolvePayload_FileRelativeToProjectRoot(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.MkdirAll(sub, 0o755); err != nil {
		t.Fatalf("mkdir sub: %v", err)
	}
	want := `{"slug":"x","approved":false}`
	if err := os.WriteFile(filepath.Join(sub, "p.json"), []byte(want), 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	got, err := resolvePayload("", "sub/p.json", strings.NewReader("ignored stdin"), root)
	if err != nil {
		t.Fatalf("resolvePayload: %v", err)
	}
	if string(got) != want {
		t.Errorf("payload = %q; want %q", string(got), want)
	}
}

// TestArbitratePayloadMode_BothFlags covers AC: payload-mode-conflict-fails-2.
// When both --payload-json and --payload-file are set, the arbitration
// MUST fail with an error that names both flags AND enumerates the three
// accepted input modes.
func TestArbitratePayloadMode_BothFlags(t *testing.T) {
	err := arbitratePayloadMode(`{}`, "/tmp/p.json", false, false)
	if err == nil {
		t.Fatal("expected error when both --payload-json and --payload-file set; got nil")
	}

	type exitCoder interface{ ExitCode() int }
	var ec exitCoder
	if !errors.As(err, &ec) {
		t.Fatalf("error does not carry ExitCode: %v", err)
	}
	if got := ec.ExitCode(); got != 2 {
		t.Errorf("exit code = %d; want 2", got)
	}

	msg := err.Error()
	for _, want := range []string{
		"--payload-json",
		"--payload-file",
		"stdin",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected error to contain %q; got: %s", want, msg)
		}
	}
}

// TestArbitratePayloadMode_TtyStdinNoFlag covers AC: payload-tty-stdin-fails-2.
// When neither flag is set AND stdin is a TTY, arbitration MUST fail with
// an error naming the three accepted modes.
func TestArbitratePayloadMode_TtyStdinNoFlag(t *testing.T) {
	err := arbitratePayloadMode("", "", true, false)
	if err == nil {
		t.Fatal("expected error when no payload flag set and stdin is TTY; got nil")
	}

	type exitCoder interface{ ExitCode() int }
	var ec exitCoder
	if !errors.As(err, &ec) {
		t.Fatalf("error does not carry ExitCode: %v", err)
	}
	if got := ec.ExitCode(); got != 2 {
		t.Errorf("exit code = %d; want 2", got)
	}

	msg := err.Error()
	for _, want := range []string{
		"--payload-json",
		"--payload-file",
		"stdin",
	} {
		if !strings.Contains(msg, want) {
			t.Errorf("expected error to contain %q; got: %s", want, msg)
		}
	}
}

// TestArbitratePayloadMode_PipedStdinNoFlag — happy path: no flag set, stdin
// is piped (non-TTY); arbitration MUST succeed.
func TestArbitratePayloadMode_PipedStdinNoFlag(t *testing.T) {
	if err := arbitratePayloadMode("", "", false, true); err != nil {
		t.Errorf("expected nil error for piped stdin with no flag; got: %v", err)
	}
}

// TestArbitratePayloadMode_OneFlag — happy path: exactly one flag set;
// arbitration MUST succeed regardless of stdin state.
func TestArbitratePayloadMode_OneFlag(t *testing.T) {
	if err := arbitratePayloadMode(`{}`, "", false, false); err != nil {
		t.Errorf("expected nil error for --payload-json only; got: %v", err)
	}
	if err := arbitratePayloadMode(`{}`, "", true, false); err != nil {
		t.Errorf("expected nil error for --payload-json only with TTY stdin; got: %v", err)
	}
	if err := arbitratePayloadMode("", "/tmp/p.json", false, false); err != nil {
		t.Errorf("expected nil error for --payload-file only; got: %v", err)
	}
	if err := arbitratePayloadMode("", "/tmp/p.json", true, false); err != nil {
		t.Errorf("expected nil error for --payload-file only with TTY stdin; got: %v", err)
	}
}

// TestValidatePayloadJSON_Valid — well-formed JSON object passes.
func TestValidatePayloadJSON_Valid(t *testing.T) {
	if err := validatePayloadJSON([]byte(`{"slug":"x"}`), "--payload-json"); err != nil {
		t.Errorf("expected nil error for valid JSON object; got: %v", err)
	}
}

// TestValidatePayloadJSON_BadSyntax covers AC: payload-bad-json-fails-2.
// Bytes that fail to parse MUST produce an exit-2 error naming the input
// mode AND a JSON parse error description.
func TestValidatePayloadJSON_BadSyntax(t *testing.T) {
	err := validatePayloadJSON([]byte(`not json`), "--payload-json")
	if err == nil {
		t.Fatal("expected error for non-JSON bytes; got nil")
	}

	type exitCoder interface{ ExitCode() int }
	var ec exitCoder
	if !errors.As(err, &ec) {
		t.Fatalf("error does not carry ExitCode: %v", err)
	}
	if got := ec.ExitCode(); got != 2 {
		t.Errorf("exit code = %d; want 2", got)
	}

	msg := err.Error()
	if !strings.Contains(msg, "--payload-json") {
		t.Errorf("expected error to name --payload-json; got: %s", msg)
	}
	// The error MUST describe the parse failure. Go's json package errors
	// contain words like "invalid character" / "unexpected" / "json".
	lower := strings.ToLower(msg)
	if !strings.Contains(lower, "json") && !strings.Contains(lower, "parse") &&
		!strings.Contains(lower, "invalid character") && !strings.Contains(lower, "unexpected") {
		t.Errorf("expected error to describe a JSON parse failure; got: %s", msg)
	}
}

// TestValidatePayloadJSON_NotAnObject — scalars / arrays / null are not
// valid envelope payloads; arbitration MUST reject them.
func TestValidatePayloadJSON_NotAnObject(t *testing.T) {
	for _, bytes := range []string{`42`, `"string"`, `null`, `true`, `[1,2,3]`} {
		err := validatePayloadJSON([]byte(bytes), "stdin")
		if err == nil {
			t.Errorf("expected error for non-object JSON %q; got nil", bytes)
			continue
		}
		type exitCoder interface{ ExitCode() int }
		var ec exitCoder
		if !errors.As(err, &ec) {
			t.Errorf("error for %q does not carry ExitCode: %v", bytes, err)
			continue
		}
		if got := ec.ExitCode(); got != 2 {
			t.Errorf("exit code for %q = %d; want 2", bytes, got)
		}
		if !strings.Contains(err.Error(), "stdin") {
			t.Errorf("expected error for %q to name input mode `stdin`; got: %s", bytes, err.Error())
		}
	}
}

// TestStdinIsTTY — when os.Stdin is redirected to /dev/null (a character
// device on macOS but NOT the controlling TTY), the result depends on the
// platform. The reliable check: replace os.Stdin with a *pipe* (definitely
// not a TTY) and assert false.
func TestStdinIsTTY(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer func() { _ = r.Close() }()
	defer func() { _ = w.Close() }()

	orig := os.Stdin
	os.Stdin = r
	defer func() { os.Stdin = orig }()

	if stdinIsTTY() {
		t.Error("stdinIsTTY() with pipe stdin = true; want false")
	}
}

// captureStderr replaces os.Stderr with a pipe for the duration of fn and
// returns the bytes written. The dispatcher emits its per-subscriber failure
// lines to a package-private `dispatchStderr` sink that defaults to os.Stderr;
// the CLI verb's end-to-end ACs assert on those lines, so the test must read
// the real fd.
func captureStderr(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	orig := os.Stderr
	os.Stderr = w
	done := make(chan string, 1)
	go func() {
		var buf bytes.Buffer
		_, _ = io.Copy(&buf, r)
		done <- buf.String()
	}()

	fn()

	os.Stderr = orig
	_ = w.Close()
	out := <-done
	_ = r.Close()
	return out
}

// writeSpecscoreYAML writes a minimal specscore.yaml at root with the given
// `events:` block body. Schema-header validation isn't enforced by
// event.LoadSubscribers (it does not call projectdef.ReadSpecConfig), so a
// bare YAML body is sufficient.
func writeSpecscoreYAML(t *testing.T, root, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "specscore.yaml"), []byte(body), 0o644); err != nil {
		t.Fatalf("write specscore.yaml: %v", err)
	}
}

// TestEventEmit_AllSubscribersFail_ExitCode10 covers AC:
// dispatch-exit-code-handoff. Two `/bin/false` exec subscribers both exit 1;
// the verb MUST return exit code 10.
//
// Note: the per-subscriber failure-line format (key=value stderr lines) is
// tested at the pkg/event level in dispatcher_test.go. We don't assert it
// here because pkg/event.dispatchStderr captures os.Stderr at init time, so
// replacing os.Stderr in tests does not redirect the failure lines.
func TestEventEmit_AllSubscribersFail_ExitCode10(t *testing.T) {
	falseBin := "/bin/false"
	if _, statErr := os.Stat(falseBin); os.IsNotExist(statErr) {
		// Fallback to /usr/bin/false (common on Ubuntu where /bin -> /usr/bin).
		falseBin = "/usr/bin/false"
		if _, statErr2 := os.Stat(falseBin); os.IsNotExist(statErr2) {
			t.Skip("neither /bin/false nor /usr/bin/false available on this OS")
		}
	}

	tmp := t.TempDir()
	withCwd(t, tmp)
	writeSpecscoreYAML(t, tmp, "events:\n  subscribers:\n    - type: exec\n      command: ["+falseBin+"]\n    - type: exec\n      command: ["+falseBin+"]\n")

	_, _, runErr := runEvent(t,
		"emit",
		"--name", "idea.drafted",
		"--actor-kind", "skill",
		"--actor-id", "skill:t",
		"--artifact-type", "idea",
		"--artifact-id", "x",
		"--artifact-path", "spec/ideas/x.md",
		"--payload-json", "{}",
	)
	if runErr == nil {
		t.Fatal("expected error from all-subscribers-fail path; got nil")
	}

	type exitCoder interface{ ExitCode() int }
	var ec exitCoder
	if !errors.As(runErr, &ec) {
		t.Fatalf("error does not carry ExitCode: %v", runErr)
	}
	if got := ec.ExitCode(); got != 10 {
		t.Errorf("exit code = %d; want 10", got)
	}
}

// TestEventEmit_NoopSubscriber_ExitCode0 — happy-path companion to the
// all-fail test: a single noop subscriber returns nil from Deliver, so
// Delivered=1, Failed=0, and the verb MUST return exit 0.
func TestEventEmit_NoopSubscriber_ExitCode0(t *testing.T) {
	tmp := t.TempDir()
	withCwd(t, tmp)
	writeSpecscoreYAML(t, tmp, `events:
  subscribers:
    - type: noop
`)

	var runErr error
	stderr := captureStderr(t, func() {
		_, _, runErr = runEvent(t,
			"emit",
			"--name", "idea.drafted",
			"--actor-kind", "skill",
			"--actor-id", "skill:t",
			"--artifact-type", "idea",
			"--artifact-id", "x",
			"--artifact-path", "spec/ideas/x.md",
			"--payload-json", "{}",
		)
	})
	if runErr != nil {
		t.Fatalf("expected nil error on noop success path; got: %v\nstderr=%s", runErr, stderr)
	}
	if strings.Contains(stderr, "event-dispatch failure") {
		t.Errorf("expected no failure lines on success path; got stderr:\n%s", stderr)
	}
}

func TestEventEmit_ExplicitEmptySubscribersCreatesNoRecipientlessLedger(t *testing.T) {
	root := t.TempDir()
	withCwd(t, root)
	writeSpecscoreYAML(t, root, "events:\n  subscribers: []\n")
	if _, _, err := runEvent(t,
		"emit",
		"--name", "idea.drafted",
		"--actor-kind", "skill",
		"--actor-id", "skill:t",
		"--artifact-type", "idea",
		"--artifact-id", "x",
		"--artifact-path", "spec/ideas/x.md",
		"--payload-json", "{}",
	); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, ".specscore", "event-outbox")); !os.IsNotExist(err) {
		t.Fatalf("explicit event opt-out created a recipientless outbox: %v", err)
	}
}

func TestEventReplaySurfacesPreparedArtifactEvidenceAndReconcileRequiresToken(t *testing.T) {
	root := t.TempDir()
	withCwd(t, root)
	writeSpecscoreYAML(t, root, "events:\n  subscribers:\n    - type: noop\n")
	artifact := filepath.Join(root, "spec", "ideas", "demo.md")
	if err := os.MkdirAll(filepath.Dir(artifact), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifact, []byte("# demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	e := event.Event{
		Name:      "idea.drafted",
		Version:   1,
		UUID:      "11111111-1111-4111-8111-111111111111",
		Timestamp: time.Date(2026, 8, 10, 12, 0, 0, 0, time.UTC),
		Actor:     event.Actor{Kind: "external", ID: "specscore"},
		Artifact:  event.Artifact{Type: "idea", ID: "demo", Path: "spec/ideas/demo.md", Revision: "uncommitted"},
		Payload:   []byte(`{}`),
	}
	outbox := event.NewOutbox(root)
	if err := outbox.Prepare(e, []event.Subscriber{event.NoOp{}}); err != nil {
		t.Fatal(err)
	}
	out, stderr, err := runEvent(t, "replay", "--subscriber", "noop")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "prepared=1") || !strings.Contains(stderr, e.UUID) || !strings.Contains(stderr, "artifact_exists=true") {
		t.Fatalf("prepared replay output=%q stderr=%q", out, stderr)
	}
	preview, _, err := runEvent(t, "reconcile", e.UUID, "--decision", "commit")
	if exitCodeOf(err) != 2 || !strings.Contains(preview, "confirmation-token:") {
		t.Fatalf("reconcile preview=%q err=%v", preview, err)
	}
	prepared, err := outbox.Prepared()
	if err != nil || len(prepared) != 1 {
		t.Fatalf("prepared=%#v err=%v", prepared, err)
	}
	token := event.ReconciliationToken(prepared[0], "commit")
	out, _, err = runEvent(t, "reconcile", e.UUID, "--decision", "commit", "--confirm", token)
	if err != nil || !strings.Contains(out, ": commit") {
		t.Fatalf("reconcile=%q err=%v", out, err)
	}
	prepared, err = outbox.Prepared()
	if err != nil || len(prepared) != 0 {
		t.Fatalf("reconciled event remains prepared: %#v err=%v", prepared, err)
	}
}

// TestAutofillEnvelope_RevisionOverride (covers AC:
// envelope-artifact-revision-override). When the override is non-empty
// it MUST be used verbatim — the git path MUST NOT be invoked and MUST
// NOT replace the override.
func TestAutofillEnvelope_RevisionOverride(t *testing.T) {
	dir := t.TempDir()
	gitInitWithCommit(t, dir)

	const override = "deadbeef00000000000000000000000000000000"

	var e event.Event
	autofillEnvelope(&e, dir, override)

	if e.Artifact.Revision != override {
		t.Errorf("Artifact.Revision = %q; want override %q", e.Artifact.Revision, override)
	}
}
