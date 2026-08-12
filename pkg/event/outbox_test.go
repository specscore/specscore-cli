package event

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"
)

type outboxSub struct {
	name string
	fail bool
	seen []string
}

func TestOutbox_PrepareDoesNotDeliverUntilCommit(t *testing.T) {
	a := &outboxSub{name: "a"}
	o := NewOutbox(t.TempDir())
	e := validEvent()
	if err := o.Prepare(e, []Subscriber{a}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(o.ledgerPath(e.UUID)); err != nil {
		t.Fatalf("prepared ledger missing: %v", err)
	}
	if _, err := os.Stat(o.pendingPath("a", e.UUID)); !os.IsNotExist(err) {
		t.Fatalf("prepared event unexpectedly deliverable: %v", err)
	}
	result, err := o.Replay(context.Background(), []Subscriber{a}, "", 0)
	if err != nil || result.Delivered != 0 || len(a.seen) != 0 {
		t.Fatalf("prepared replay = %#v err=%v seen=%v", result, err, a.seen)
	}
	if err := o.Commit(e.UUID); err != nil {
		t.Fatal(err)
	}
	result, err = o.Replay(context.Background(), []Subscriber{a}, "", 0)
	if err != nil || result.Delivered != 1 || len(a.seen) != 1 {
		t.Fatalf("committed replay = %#v err=%v seen=%v", result, err, a.seen)
	}
}

func TestOutbox_RecoverReconstructsRecipientOmittedByInterruption(t *testing.T) {
	a := &outboxSub{name: "a"}
	b := &outboxSub{name: "b"}
	o := NewOutbox(t.TempDir())
	e := validEvent()
	if err := o.Enqueue(e, []Subscriber{a, b}); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(o.pendingPath("b", e.UUID)); err != nil {
		t.Fatal(err)
	}
	if err := o.Recover(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(o.pendingPath("b", e.UUID)); err != nil {
		t.Fatalf("recipient marker not reconstructed from ledger: %v", err)
	}
	result, err := o.Replay(context.Background(), []Subscriber{a, b}, "", 0)
	if err != nil || result.Delivered != 2 {
		t.Fatalf("replay = %#v err=%v", result, err)
	}
}

func TestOutbox_AbortPreparedRetainsAuditableUndeliverableLedger(t *testing.T) {
	o := NewOutbox(t.TempDir())
	e := validEvent()
	if err := o.Prepare(e, []Subscriber{&outboxSub{name: "audit"}}); err != nil {
		t.Fatal(err)
	}
	if err := o.Abort(e.UUID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(o.ledgerPath(e.UUID)); err != nil {
		t.Fatalf("aborted immutable ledger missing: %v", err)
	}
	state, err := o.readState(e.UUID)
	if err != nil || state != abortedState {
		t.Fatalf("abort state = %q err=%v", state, err)
	}
}

func (s *outboxSub) Name() string { return s.name }
func (s *outboxSub) Deliver(_ context.Context, e Event) error {
	s.seen = append(s.seen, e.UUID)
	if s.fail {
		return errors.New("down")
	}
	return nil
}
func TestOutbox_ReplaysFailedSubscriberIndependently(t *testing.T) {
	a := &outboxSub{name: "a", fail: true}
	b := &outboxSub{name: "b"}
	o := NewOutbox(t.TempDir())
	e := validEvent()
	if err := o.Enqueue(e, []Subscriber{a, b}); err != nil {
		t.Fatal(err)
	}
	r, err := o.Replay(context.Background(), []Subscriber{a, b}, "", 0)
	if err != nil {
		t.Fatal(err)
	}
	if r.Delivered != 1 || len(r.Failed) != 1 || len(b.seen) != 1 {
		t.Fatalf("first replay %#v a=%v b=%v", r, a.seen, b.seen)
	}
	a.fail = false
	r, err = o.Replay(context.Background(), []Subscriber{a, b}, "a", 0)
	if err != nil {
		t.Fatal(err)
	}
	if r.Delivered != 1 || len(a.seen) != 2 || len(b.seen) != 1 {
		t.Fatalf("independent replay %#v a=%v b=%v", r, a.seen, b.seen)
	}
}

func TestOutbox_ReplayFromUsesLedgerOrderAndDurableInclusiveCursor(t *testing.T) {
	sink := &outboxSub{name: "sink"}
	o := NewOutbox(t.TempDir())
	base := validEvent()
	base.Timestamp = time.Date(2026, 5, 22, 9, 0, 0, 0, time.UTC)
	base.UUID = "ffffffff-ffff-4fff-8fff-fffffffffff1"
	middle := validEvent()
	middle.Timestamp = time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC)
	middle.UUID = "ffffffff-ffff-4fff-8fff-fffffffffff2"
	late := validEvent()
	late.Timestamp = time.Date(2026, 5, 22, 11, 0, 0, 0, time.UTC)
	late.UUID = "00000000-0000-4000-8000-000000000003"
	for _, e := range []Event{late, base, middle} {
		if err := o.Enqueue(e, []Subscriber{sink}); err != nil {
			t.Fatal(err)
		}
	}

	result, err := o.ReplayFrom(context.Background(), []Subscriber{sink}, "sink", middle.UUID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if result.Delivered != 2 || result.Pending != 2 || !slices.Equal(sink.seen, []string{middle.UUID, late.UUID}) {
		t.Fatalf("cursor replay=%#v seen=%v", result, sink.seen)
	}
	if _, err := os.Stat(o.pendingPath("sink", base.UUID)); err != nil {
		t.Fatalf("event before cursor was acknowledged: %v", err)
	}

	after := validEvent()
	after.Timestamp = time.Date(2026, 5, 22, 12, 0, 0, 0, time.UTC)
	after.UUID = "00000000-0000-4000-8000-000000000004"
	if err := o.Enqueue(after, []Subscriber{sink}); err != nil {
		t.Fatal(err)
	}
	result, err = o.ReplayFrom(context.Background(), []Subscriber{sink}, "sink", middle.UUID, 0)
	if err != nil || result.Delivered != 1 || sink.seen[len(sink.seen)-1] != after.UUID {
		t.Fatalf("acknowledged cursor replay=%#v err=%v seen=%v", result, err, sink.seen)
	}
}

func TestOutbox_ReplayFromRejectsUnknownPreparedAndUnaddressedCursor(t *testing.T) {
	sink := &outboxSub{name: "sink"}
	o := NewOutbox(t.TempDir())
	if _, err := o.ReplayFrom(context.Background(), []Subscriber{sink}, "sink", "00000000-0000-4000-8000-000000000099", 0); err == nil || !strings.Contains(err.Error(), "unknown --from") {
		t.Fatalf("unknown cursor err=%v", err)
	}
	prepared := validEvent()
	prepared.UUID = "00000000-0000-4000-8000-000000000098"
	if err := o.Prepare(prepared, []Subscriber{sink}); err != nil {
		t.Fatal(err)
	}
	if _, err := o.ReplayFrom(context.Background(), []Subscriber{sink}, "sink", prepared.UUID, 0); err == nil || !strings.Contains(err.Error(), "not committed") {
		t.Fatalf("prepared cursor err=%v", err)
	}
	other := validEvent()
	other.UUID = "00000000-0000-4000-8000-000000000097"
	if err := o.Enqueue(other, []Subscriber{&outboxSub{name: "other"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := o.ReplayFrom(context.Background(), []Subscriber{sink}, "sink", other.UUID, 0); err == nil || !strings.Contains(err.Error(), "not addressed") {
		t.Fatalf("unaddressed cursor err=%v", err)
	}
}

func TestOutbox_ConcurrentPrepareCannotOverwriteDifferentUUIDContent(t *testing.T) {
	o := NewOutbox(t.TempDir())
	one := validEvent()
	two := validEvent()
	two.Payload = []byte(`{"writer":2}`)
	subscribers := []Subscriber{&outboxSub{name: "sink"}}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, event := range []Event{one, two} {
		wg.Add(1)
		go func(e Event) {
			defer wg.Done()
			<-start
			results <- o.Prepare(e, subscribers)
		}(event)
	}
	close(start)
	wg.Wait()
	close(results)
	succeeded := 0
	failed := 0
	for err := range results {
		if err == nil {
			succeeded++
		} else {
			failed++
		}
	}
	if succeeded != 1 || failed != 1 {
		t.Fatalf("concurrent prepare successes=%d failures=%d", succeeded, failed)
	}
	record, err := o.readRecord(one.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if string(record.Event.Payload) != string(one.Payload) && string(record.Event.Payload) != string(two.Payload) {
		t.Fatalf("ledger contains torn/unknown payload: %s", record.Event.Payload)
	}
}

func TestOutbox_CommitAbortRaceHasSingleDurableDecision(t *testing.T) {
	o := NewOutbox(t.TempDir())
	e := validEvent()
	if err := o.Prepare(e, []Subscriber{&outboxSub{name: "sink"}}); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, decide := range []func(string) error{o.Commit, o.Abort} {
		wg.Add(1)
		go func(fn func(string) error) {
			defer wg.Done()
			<-start
			results <- fn(e.UUID)
		}(decide)
	}
	close(start)
	wg.Wait()
	close(results)
	succeeded := 0
	for err := range results {
		if err == nil {
			succeeded++
		}
	}
	if succeeded != 1 {
		t.Fatalf("expected exactly one CAS winner, got %d", succeeded)
	}
	state, err := o.readState(e.UUID)
	if err != nil {
		t.Fatal(err)
	}
	if state != committedState && state != abortedState {
		t.Fatalf("unexpected decision %q", state)
	}
	_, pendingErr := os.Stat(o.pendingPath("sink", e.UUID))
	if state == committedState && pendingErr != nil {
		t.Fatalf("committed decision omitted recipient: %v", pendingErr)
	}
	if state == abortedState && !os.IsNotExist(pendingErr) {
		t.Fatalf("aborted decision became deliverable: %v", pendingErr)
	}
}

func TestOutbox_PreparedArtifactEvidenceIsSurfacedUntilReconciled(t *testing.T) {
	root := t.TempDir()
	artifact := filepath.Join(root, "spec", "ideas", "demo.md")
	if err := os.MkdirAll(filepath.Dir(artifact), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifact, []byte("# demo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	o := NewOutbox(root)
	e := validEvent()
	if err := o.Prepare(e, []Subscriber{&outboxSub{name: "audit"}}); err != nil {
		t.Fatal(err)
	}
	prepared, err := o.Prepared()
	if err != nil || len(prepared) != 1 || !prepared[0].ArtifactExists {
		t.Fatalf("prepared evidence = %#v err=%v", prepared, err)
	}
	r, err := o.Replay(context.Background(), nil, "", 0)
	if err != nil || len(r.Prepared) != 1 {
		t.Fatalf("replay did not surface prepared recovery: %#v err=%v", r, err)
	}
	if err := o.Commit(e.UUID); err != nil {
		t.Fatal(err)
	}
	prepared, err = o.Prepared()
	if err != nil || len(prepared) != 0 {
		t.Fatalf("committed record still unresolved: %#v err=%v", prepared, err)
	}
}

func TestOutbox_AtLeastOnceRequiresSubscriberUUIDIdempotency(t *testing.T) {
	sink := &outboxSub{name: "sink"}
	o := NewOutbox(t.TempDir())
	e := validEvent()
	if err := o.Enqueue(e, []Subscriber{sink}); err != nil {
		t.Fatal(err)
	}
	// Simulate success followed by a process crash before the ack marker.
	if err := sink.Deliver(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	r, err := o.Replay(context.Background(), []Subscriber{sink}, "", 0)
	if err != nil || r.Delivered != 1 || len(sink.seen) != 2 || sink.seen[0] != e.UUID || sink.seen[1] != e.UUID {
		t.Fatalf("at-least-once replay = %#v seen=%v err=%v", r, sink.seen, err)
	}
}

func TestOutbox_RejectsCorruptLedgerBeforeAnySubscriberDelivery(t *testing.T) {
	tests := map[string]func(*testing.T, Outbox, Event){
		"invalid envelope": func(t *testing.T, o Outbox, e Event) {
			record, err := o.readRecord(e.UUID)
			if err != nil {
				t.Fatal(err)
			}
			record.Event.Name = "not-valid"
			writeTamperedLedger(t, o, e.UUID, record)
		},
		"duplicate subscribers": func(t *testing.T, o Outbox, e Event) {
			record, err := o.readRecord(e.UUID)
			if err != nil {
				t.Fatal(err)
			}
			record.Subscribers = []string{"a", "a"}
			writeTamperedLedger(t, o, e.UUID, record)
		},
		"unsorted subscribers": func(t *testing.T, o Outbox, e Event) {
			record, err := o.readRecord(e.UUID)
			if err != nil {
				t.Fatal(err)
			}
			record.Subscribers = []string{"b", "a"}
			writeTamperedLedger(t, o, e.UUID, record)
		},
		"empty subscriber": func(t *testing.T, o Outbox, e Event) {
			record, err := o.readRecord(e.UUID)
			if err != nil {
				t.Fatal(err)
			}
			record.Subscribers = []string{""}
			writeTamperedLedger(t, o, e.UUID, record)
		},
		"empty subscriber set": func(t *testing.T, o Outbox, e Event) {
			record, err := o.readRecord(e.UUID)
			if err != nil {
				t.Fatal(err)
			}
			record.Subscribers = []string{}
			writeTamperedLedger(t, o, e.UUID, record)
		},
		"unknown property": func(t *testing.T, o Outbox, e Event) {
			b, err := os.ReadFile(o.ledgerPath(e.UUID))
			if err != nil {
				t.Fatal(err)
			}
			b = bytes.TrimSpace(b)
			b = append(append(append([]byte(nil), b[:len(b)-1]...), []byte(`,"unexpected":true}`)...), '\n')
			if err := os.WriteFile(o.ledgerPath(e.UUID), b, 0o644); err != nil {
				t.Fatal(err)
			}
		},
		"trailing JSON": func(t *testing.T, o Outbox, e Event) {
			f, err := os.OpenFile(o.ledgerPath(e.UUID), os.O_APPEND|os.O_WRONLY, 0)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := f.WriteString("{}\n"); err != nil {
				_ = f.Close()
				t.Fatal(err)
			}
			if err := f.Close(); err != nil {
				t.Fatal(err)
			}
		},
	}

	for name, tamper := range tests {
		t.Run(name, func(t *testing.T) {
			a := &outboxSub{name: "a"}
			b := &outboxSub{name: "b"}
			o := NewOutbox(t.TempDir())
			e := validEvent()
			if err := o.Enqueue(e, []Subscriber{a, b}); err != nil {
				t.Fatal(err)
			}
			tamper(t, o, e)
			if _, err := o.Replay(context.Background(), []Subscriber{a, b}, "", 0); err == nil {
				t.Fatal("corrupt immutable ledger was accepted")
			}
			if len(a.seen) != 0 || len(b.seen) != 0 {
				t.Fatalf("corrupt ledger reached a subscriber: a=%v b=%v", a.seen, b.seen)
			}
		})
	}
}

func TestOutbox_PrepareRejectsRecipientlessLedger(t *testing.T) {
	o := NewOutbox(t.TempDir())
	e := validEvent()
	if err := o.Prepare(e, nil); err == nil {
		t.Fatal("recipientless immutable ledger was accepted")
	}
	if _, err := os.Stat(o.ledgerPath(e.UUID)); !os.IsNotExist(err) {
		t.Fatalf("recipientless prepare wrote a ledger: %v", err)
	}
}

func writeTamperedLedger(t *testing.T, o Outbox, id string, record ledgerRecord) {
	t.Helper()
	b, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	b = append(b, '\n')
	if err := os.WriteFile(o.ledgerPath(id), b, 0o644); err != nil {
		t.Fatal(err)
	}
}
