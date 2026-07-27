package event

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// execSampleEvent returns a representative event used by the Exec tests.
func execSampleEvent(t *testing.T) Event {
	t.Helper()
	return Event{
		Name:      "idea.drafted",
		Version:   1,
		UUID:      "00000000-0000-4000-8000-000000000000",
		Timestamp: time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC),
		Actor:     Actor{Kind: "skill", ID: "specstudio:ideate"},
		Artifact:  Artifact{Type: "idea", ID: "demo", Path: "spec/ideas/demo.md", Revision: "uncommitted"},
		Payload:   json.RawMessage(`{"k":"v"}`),
	}
}

// TestExecPipesEventToStdin verifies AC:exec-pipes-event-to-stdin. The Exec
// subscriber must pipe the JSON-serialized envelope to the child's stdin and
// close stdin so a child like `tee` can exit cleanly on EOF.
func TestExecPipesEventToStdin(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "recorded-event.jsonl")

	sub := NewExec([]string{"tee", out}, nil, 2*time.Second)
	if got, want := sub.Name(), "exec:tee"; got != want {
		t.Fatalf("Name() = %q, want %q", got, want)
	}

	e := execSampleEvent(t)
	if err := sub.Deliver(context.Background(), e); err != nil {
		t.Fatalf("Deliver returned error: %v", err)
	}

	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read recorded file: %v", err)
	}
	want, err := json.Marshal(e)
	if err != nil {
		t.Fatalf("marshal event: %v", err)
	}
	// Allow a single optional trailing newline.
	gotTrimmed := bytes.TrimRight(got, "\n")
	if !bytes.Equal(gotTrimmed, want) {
		t.Fatalf("recorded bytes mismatch.\n got:  %q\n want: %q", string(gotTrimmed), string(want))
	}
	// Ensure at most one trailing newline (i.e. exactly one line).
	if n := bytes.Count(got, []byte{'\n'}); n > 1 {
		t.Fatalf("recorded file contains %d newlines, want at most 1", n)
	}
}

// TestExecNonZeroExitReturnsExitError verifies that a child exiting non-zero
// surfaces a distinguishable *ExecExitError so the dispatcher can name the
// failure mode in its stderr log.
func TestExecNonZeroExitReturnsExitError(t *testing.T) {
	sub := NewExec([]string{"sh", "-c", "exit 7"}, nil, 2*time.Second)
	err := sub.Deliver(context.Background(), execSampleEvent(t))
	if err == nil {
		t.Fatalf("Deliver returned nil, want exit error")
	}
	var exitErr *ExecExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("Deliver error type = %T (%v), want *ExecExitError", err, err)
	}
	if exitErr.ExitCode != 7 {
		t.Fatalf("exitErr.ExitCode = %d, want 7", exitErr.ExitCode)
	}
}

// TestExecEmptyArgvName verifies that Name() with empty argv returns "exec:".
func TestExecEmptyArgvName(t *testing.T) {
	sub := NewExec(nil, nil, 2*time.Second)
	if got, want := sub.Name(), "exec:"; got != want {
		t.Fatalf("Name() = %q, want %q", got, want)
	}
}

// TestExecEmptyArgvDeliver verifies that Deliver with empty argv returns an
// error immediately without spawning a process.
func TestExecEmptyArgvDeliver(t *testing.T) {
	sub := NewExec(nil, nil, 2*time.Second)
	err := sub.Deliver(context.Background(), execSampleEvent(t))
	if err == nil {
		t.Fatal("Deliver with empty argv should return error")
	}
	if !strings.Contains(err.Error(), "empty argv") {
		t.Errorf("error message = %q, want to contain 'empty argv'", err.Error())
	}
}

func TestExecConfigureProcessTreeError(t *testing.T) {
	originalConfigure := configureExecProcessTreeFn
	t.Cleanup(func() { configureExecProcessTreeFn = originalConfigure })
	sentinel := errors.New("configure tree")
	configureExecProcessTreeFn = func(*exec.Cmd) (execProcessTree, error) {
		return nil, sentinel
	}

	err := NewExec([]string{"true"}, nil, time.Second).
		Deliver(context.Background(), execSampleEvent(t))
	if !errors.Is(err, sentinel) {
		t.Fatalf("Deliver error = %v, want %v", err, sentinel)
	}
}

func TestExecInitializeProcessTreeError(t *testing.T) {
	originalConfigure := configureExecProcessTreeFn
	t.Cleanup(func() { configureExecProcessTreeFn = originalConfigure })
	sentinel := errors.New("initialize tree")
	tree := &fakeExecProcessTree{afterStartErr: sentinel}
	configureExecProcessTreeFn = func(*exec.Cmd) (execProcessTree, error) {
		return tree, nil
	}

	err := NewExec([]string{"sleep", "30"}, nil, time.Second).
		Deliver(context.Background(), execSampleEvent(t))
	if !errors.Is(err, sentinel) {
		t.Fatalf("Deliver error = %v, want %v", err, sentinel)
	}
	if !tree.closed {
		t.Fatal("process tree was not closed")
	}
}

type fakeExecProcessTree struct {
	afterStartErr error
	closed        bool
}

func (t *fakeExecProcessTree) afterStart() error {
	return t.afterStartErr
}

func (t *fakeExecProcessTree) close() error {
	t.closed = true
	return nil
}

// TestExecTimeoutErrorMethods covers the Error() and Unwrap() methods of
// ExecTimeoutError.
func TestExecTimeoutErrorMethods(t *testing.T) {
	cause := errors.New("signal: killed")
	e := &ExecTimeoutError{Timeout: 200 * time.Millisecond, Cause: cause}

	wantMsg := "exec: timeout after 200ms"
	if got := e.Error(); got != wantMsg {
		t.Fatalf("Error() = %q, want %q", got, wantMsg)
	}
	if unwrapped := e.Unwrap(); unwrapped != cause {
		t.Fatalf("Unwrap() = %v, want %v", unwrapped, cause)
	}
}

// TestExecExitErrorMethods covers the Error() and Unwrap() methods of
// ExecExitError.
func TestExecExitErrorMethods(t *testing.T) {
	cause := errors.New("exit status 7")
	e := &ExecExitError{ExitCode: 7, Cause: cause}

	wantMsg := "exec: child exited with code 7"
	if got := e.Error(); got != wantMsg {
		t.Fatalf("Error() = %q, want %q", got, wantMsg)
	}
	if unwrapped := e.Unwrap(); unwrapped != cause {
		t.Fatalf("Unwrap() = %v, want %v", unwrapped, cause)
	}
}

// TestExecChildExitsBeforeReadingStdin verifies the behavior when a child
// exits non-zero without reading all of stdin. This exercises the write-error
// → Wait → classifyWaitError path.
func TestExecChildExitsBeforeReadingStdin(t *testing.T) {
	// A child that exits immediately with code 3 without reading stdin.
	sub := NewExec([]string{"sh", "-c", "exit 3"}, nil, 2*time.Second)
	err := sub.Deliver(context.Background(), execSampleEvent(t))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	// The error should be an ExecExitError with code 3.
	var exitErr *ExecExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("error type = %T (%v), want *ExecExitError", err, err)
	}
	if exitErr.ExitCode != 3 {
		t.Fatalf("ExitCode = %d, want 3", exitErr.ExitCode)
	}
}

// TestExecCancelledContext verifies that a pre-cancelled context results in a
// start failure or timeout error.
func TestExecCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	sub := NewExec([]string{"sleep", "10"}, nil, 2*time.Second)
	err := sub.Deliver(ctx, execSampleEvent(t))
	if err == nil {
		t.Fatal("expected error with cancelled context, got nil")
	}
	// The error should mention exec in some form.
	if !strings.Contains(err.Error(), "exec") {
		t.Errorf("error = %q, want to contain 'exec'", err.Error())
	}
}

// fakeWriteCloser is a WriteCloser whose Write and Close behaviors are
// controlled by the caller for seam-injection tests.
type fakeWriteCloser struct {
	writeErr error
	closeErr error
}

func (f *fakeWriteCloser) Write(p []byte) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return len(p), nil
}

func (f *fakeWriteCloser) Close() error { return f.closeErr }

// TestExecStdinPipeError exercises the StdinPipe failure path (lines 77-79)
// by injecting a fake cmdStdinPipeFn that always returns an error.
func TestExecStdinPipeError(t *testing.T) {
	orig := cmdStdinPipeFn
	t.Cleanup(func() { cmdStdinPipeFn = orig })
	cmdStdinPipeFn = func(cmd *exec.Cmd) (io.WriteCloser, error) {
		return nil, errors.New("pipe: injected failure")
	}

	sub := NewExec([]string{"cat"}, nil, 2*time.Second)
	err := sub.Deliver(context.Background(), execSampleEvent(t))
	if err == nil {
		t.Fatal("expected error from StdinPipe failure, got nil")
	}
	if !strings.Contains(err.Error(), "exec: stdin pipe") {
		t.Errorf("error = %q, want to contain 'exec: stdin pipe'", err.Error())
	}
}

// TestExecWriteErrorWithWaitError exercises the write-error → cmd.Wait()
// returns error path (lines 97-99) by injecting a WriteCloser that fails on
// Write while the child exits non-zero so Wait also returns an error.
func TestExecWriteErrorWithWaitError(t *testing.T) {
	orig := cmdStdinPipeFn
	t.Cleanup(func() { cmdStdinPipeFn = orig })
	cmdStdinPipeFn = func(cmd *exec.Cmd) (io.WriteCloser, error) {
		return &fakeWriteCloser{writeErr: errors.New("write: injected failure")}, nil
	}

	// Child exits non-zero so cmd.Wait() returns an error.
	sub := NewExec([]string{"sh", "-c", "exit 5"}, nil, 2*time.Second)
	err := sub.Deliver(context.Background(), execSampleEvent(t))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var exitErr *ExecExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("error type = %T (%v), want *ExecExitError", err, err)
	}
	if exitErr.ExitCode != 5 {
		t.Fatalf("ExitCode = %d, want 5", exitErr.ExitCode)
	}
}

// TestExecCloseErrorWithWaitError exercises the stdin.Close() error → cmd.Wait()
// returns error path (lines 103-105) by injecting a WriteCloser that succeeds
// on Write but fails on Close while the child exits non-zero.
func TestExecCloseErrorWithWaitError(t *testing.T) {
	orig := cmdStdinPipeFn
	t.Cleanup(func() { cmdStdinPipeFn = orig })
	cmdStdinPipeFn = func(cmd *exec.Cmd) (io.WriteCloser, error) {
		return &fakeWriteCloser{closeErr: errors.New("close: injected failure")}, nil
	}

	// Child exits non-zero so cmd.Wait() returns an error.
	sub := NewExec([]string{"sh", "-c", "exit 6"}, nil, 2*time.Second)
	err := sub.Deliver(context.Background(), execSampleEvent(t))
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	var exitErr *ExecExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("error type = %T (%v), want *ExecExitError", err, err)
	}
	if exitErr.ExitCode != 6 {
		t.Fatalf("ExitCode = %d, want 6", exitErr.ExitCode)
	}
}

// TestExecCloseErrorWithWaitSuccess exercises the stdin.Close() error → return
// close error path (line 106) by injecting a WriteCloser that succeeds on
// Write but fails on Close while the child exits zero so Wait returns nil.
func TestExecCloseErrorWithWaitSuccess(t *testing.T) {
	orig := cmdStdinPipeFn
	t.Cleanup(func() { cmdStdinPipeFn = orig })
	cmdStdinPipeFn = func(cmd *exec.Cmd) (io.WriteCloser, error) {
		return &fakeWriteCloser{closeErr: errors.New("close: injected failure")}, nil
	}

	// Child exits zero so cmd.Wait() returns nil; close error surfaces.
	sub := NewExec([]string{"sh", "-c", "exit 0"}, nil, 2*time.Second)
	err := sub.Deliver(context.Background(), execSampleEvent(t))
	if err == nil {
		t.Fatal("expected error from Close failure, got nil")
	}
	if !strings.Contains(err.Error(), "exec: close stdin") {
		t.Errorf("error = %q, want to contain 'exec: close stdin'", err.Error())
	}
}

// TestExecContextCancelDuringRun exercises the classifyWaitError fallback path
// where the context error is context.Canceled (not DeadlineExceeded) and the
// wait error is not *exec.ExitError.
func TestExecContextCancelDuringRun(t *testing.T) {
	// Use a parent context that we cancel after a short delay. The Exec
	// subscriber uses WithTimeout internally, so if our cancel fires before
	// the timeout, cctx.Err() might be Canceled rather than DeadlineExceeded.
	// We use a very long timeout so the cancel wins.
	ctx, cancel := context.WithCancel(context.Background())
	// Cancel after 50ms - before the 5s exec timeout fires.
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	sub := NewExec([]string{"sleep", "30"}, nil, 5*time.Second)
	err := sub.Deliver(ctx, execSampleEvent(t))
	if err == nil {
		t.Fatal("expected error with context cancel during run, got nil")
	}
	// Should get the "exec: wait:" fallback or a timeout error depending on timing.
	// Either is acceptable; the key is that we don't panic.
	if !strings.Contains(err.Error(), "exec") {
		t.Errorf("error = %q, want to contain 'exec'", err.Error())
	}
}

// TestExecNotFoundCommand verifies that a non-existent executable returns a
// start error.
func TestExecNotFoundCommand(t *testing.T) {
	sub := NewExec([]string{"/nonexistent/binary/path"}, nil, 2*time.Second)
	err := sub.Deliver(context.Background(), execSampleEvent(t))
	if err == nil {
		t.Fatal("expected error for non-existent binary, got nil")
	}
	if !strings.Contains(err.Error(), "exec: start") {
		t.Errorf("error = %q, want to contain 'exec: start'", err.Error())
	}
}

// TestExecEnvIsAdditive verifies that the configured env mapping is appended
// to os.Environ rather than replacing it: a variable from the parent
// environment must remain visible to the child, alongside the configured one.
func TestExecEnvIsAdditive(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "env.txt")

	t.Setenv("SPECSCORE_EXEC_PARENT_SENTINEL", "parent-visible")

	sub := NewExec(
		[]string{"sh", "-c", "printenv SPECSCORE_EXEC_PARENT_SENTINEL > " + out + "; printenv SPECSCORE_EXEC_CHILD_SENTINEL >> " + out},
		map[string]string{"SPECSCORE_EXEC_CHILD_SENTINEL": "child-visible"},
		2*time.Second,
	)
	if err := sub.Deliver(context.Background(), execSampleEvent(t)); err != nil {
		t.Fatalf("Deliver returned error: %v", err)
	}
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read env file: %v", err)
	}
	want := "parent-visible\nchild-visible\n"
	if string(got) != want {
		t.Fatalf("env file = %q, want %q", string(got), want)
	}
}
