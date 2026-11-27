package event

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

// recordingSubscriber captures every envelope it is asked to Deliver and
// returns a configurable error from each call.
type recordingSubscriber struct {
	name     string
	err      error
	received []Event
	calls    int
}

func (r *recordingSubscriber) Deliver(_ context.Context, e Event) error {
	r.calls++
	r.received = append(r.received, e)
	return r.err
}

func (r *recordingSubscriber) Name() string { return r.name }

// swapStderr replaces the package-level dispatchStderr sink with a bytes.Buffer
// for the duration of a test, returning the buffer and a restore function.
func swapStderr(t *testing.T) (*bytes.Buffer, func()) {
	t.Helper()
	orig := dispatchStderr
	buf := &bytes.Buffer{}
	dispatchStderr = buf
	return buf, func() { dispatchStderr = orig }
}

// TestDispatch_FanOutContinuesAfterFailure covers
// specscore:verifies feature/cli/event#ac:fan-out-continues-after-failure the first subscriber errors, the second
// still receives the envelope, and the result reports one delivery + one
// failure (verb maps Delivered>=1 to exit 0).
func TestDispatch_FanOutContinuesAfterFailure(t *testing.T) {
	_, restore := swapStderr(t)
	defer restore()

	failing := &recordingSubscriber{name: "failing", err: errors.New("boom")}
	recording := &recordingSubscriber{name: "recording", err: nil}

	e := validEvent()
	res := Dispatch(context.Background(), e, []Subscriber{failing, recording})

	if res.ValidationError != nil {
		t.Fatalf("unexpected ValidationError: %v", res.ValidationError)
	}
	if failing.calls != 1 {
		t.Fatalf("failing.Deliver called %d times, want 1", failing.calls)
	}
	if recording.calls != 1 {
		t.Fatalf("recording.Deliver called %d times, want 1", recording.calls)
	}
	if len(recording.received) != 1 || recording.received[0].Name != "idea.drafted" {
		t.Fatalf("recording did not receive envelope: %+v", recording.received)
	}
	if res.Delivered != 1 {
		t.Fatalf("Delivered = %d, want 1", res.Delivered)
	}
	if res.Failed != 1 {
		t.Fatalf("Failed = %d, want 1", res.Failed)
	}
	if len(res.Failures) != 1 || res.Failures[0].Name != "failing" {
		t.Fatalf("Failures = %+v, want one entry for 'failing'", res.Failures)
	}
}

// TestDispatch_PerSubscriberFailureStderrFormat covers
// specscore:verifies feature/cli/event#ac:per-subscriber-failure-stderr-format the exact key=value line is written
// to stderr when a subscriber returns an error, with the error message quoted.
func TestDispatch_PerSubscriberFailureStderrFormat(t *testing.T) {
	buf, restore := swapStderr(t)
	defer restore()

	sub := &recordingSubscriber{name: "exec:my-consumer", err: errors.New("exit status 1")}

	e := validEvent()
	res := Dispatch(context.Background(), e, []Subscriber{sub})

	if res.ValidationError != nil {
		t.Fatalf("unexpected ValidationError: %v", res.ValidationError)
	}

	want := `event-dispatch failure: subscriber=exec:my-consumer event=idea.drafted error="exit status 1"` + "\n"
	if got := buf.String(); got != want {
		t.Fatalf("stderr mismatch\ngot:  %q\nwant: %q", got, want)
	}
}

// TestDispatch_AllFailNonEmpty covers AC:dispatch-exit-code-when-all-fail: all
// subscribers fail, each produces a stderr line, and the result reports
// Delivered==0 / Failed==N (verb maps to exit 10).
func TestDispatch_AllFailNonEmpty(t *testing.T) {
	buf, restore := swapStderr(t)
	defer restore()

	subs := []Subscriber{
		&recordingSubscriber{name: "a", err: errors.New("e1")},
		&recordingSubscriber{name: "b", err: errors.New("e2")},
		&recordingSubscriber{name: "c", err: errors.New("e3")},
	}

	e := validEvent()
	res := Dispatch(context.Background(), e, subs)

	for i, s := range subs {
		r := s.(*recordingSubscriber)
		if r.calls != 1 {
			t.Fatalf("subscriber %d (%s) called %d times, want 1", i, r.name, r.calls)
		}
	}

	if res.Delivered != 0 {
		t.Fatalf("Delivered = %d, want 0", res.Delivered)
	}
	if res.Failed != 3 {
		t.Fatalf("Failed = %d, want 3", res.Failed)
	}

	lines := strings.Split(strings.TrimRight(buf.String(), "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 stderr lines, got %d: %q", len(lines), buf.String())
	}
	for i, name := range []string{"a", "b", "c"} {
		want := `event-dispatch failure: subscriber=` + name + ` event=idea.drafted error="e` + string(rune('1'+i)) + `"`
		if lines[i] != want {
			t.Fatalf("line %d mismatch\ngot:  %q\nwant: %q", i, lines[i], want)
		}
	}
}

// TestDispatch_EmptySubscriberList: zero subscribers, no error, no stderr,
// Delivered==0 && Failed==0 (verb maps to exit 0).
func TestDispatch_EmptySubscriberList(t *testing.T) {
	buf, restore := swapStderr(t)
	defer restore()

	res := Dispatch(context.Background(), validEvent(), nil)

	if res.ValidationError != nil {
		t.Fatalf("unexpected ValidationError: %v", res.ValidationError)
	}
	if res.Delivered != 0 || res.Failed != 0 {
		t.Fatalf("Delivered=%d Failed=%d, want 0/0", res.Delivered, res.Failed)
	}
	if buf.Len() != 0 {
		t.Fatalf("stderr wrote %d bytes: %q", buf.Len(), buf.String())
	}
}

// TestDispatch_SuccessfulDeliveryNoStderr asserts that a nil-returning
// subscriber produces zero bytes on stderr.
func TestDispatch_SuccessfulDeliveryNoStderr(t *testing.T) {
	buf, restore := swapStderr(t)
	defer restore()

	sub := &recordingSubscriber{name: "ok", err: nil}

	res := Dispatch(context.Background(), validEvent(), []Subscriber{sub})

	if res.ValidationError != nil {
		t.Fatalf("unexpected ValidationError: %v", res.ValidationError)
	}
	if res.Delivered != 1 || res.Failed != 0 {
		t.Fatalf("Delivered=%d Failed=%d, want 1/0", res.Delivered, res.Failed)
	}
	if buf.Len() != 0 {
		t.Fatalf("stderr wrote %d bytes: %q", buf.Len(), buf.String())
	}
}

// TestDispatch_InvalidEnvelope: validation fails -> ValidationError populated,
// no subscriber called, no stderr written by Dispatch.
func TestDispatch_InvalidEnvelope(t *testing.T) {
	buf, restore := swapStderr(t)
	defer restore()

	sub := &recordingSubscriber{name: "should-not-be-called", err: nil}

	e := validEvent()
	e.Name = "INVALID NAME" // fails namePattern

	res := Dispatch(context.Background(), e, []Subscriber{sub})

	if res.ValidationError == nil {
		t.Fatalf("expected ValidationError, got nil")
	}
	if sub.calls != 0 {
		t.Fatalf("subscriber was invoked %d times despite validation failure", sub.calls)
	}
	if res.Delivered != 0 || res.Failed != 0 {
		t.Fatalf("Delivered=%d Failed=%d, want 0/0", res.Delivered, res.Failed)
	}
	if buf.Len() != 0 {
		t.Fatalf("Dispatch should not write to stderr on validation failure; got %q", buf.String())
	}
}

// TestDispatch_StderrEscapesEmbeddedQuotes verifies the contract's
// double-quote-escape clause: an error message containing literal double
// quotes is rendered with escaped quotes inside the error="..." field.
func TestDispatch_StderrEscapesEmbeddedQuotes(t *testing.T) {
	buf, restore := swapStderr(t)
	defer restore()

	sub := &recordingSubscriber{
		name: "exec:weird",
		err:  errors.New(`error with "quoted" text`),
	}

	res := Dispatch(context.Background(), validEvent(), []Subscriber{sub})

	if res.Failed != 1 {
		t.Fatalf("Failed = %d, want 1", res.Failed)
	}

	want := `event-dispatch failure: subscriber=exec:weird event=idea.drafted error="error with \"quoted\" text"` + "\n"
	if got := buf.String(); got != want {
		t.Fatalf("stderr mismatch\ngot:  %q\nwant: %q", got, want)
	}
}
