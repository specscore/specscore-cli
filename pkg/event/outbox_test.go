package event

import (
	"context"
	"errors"
	"testing"
)

type outboxSub struct {
	name string
	fail bool
	seen []string
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
