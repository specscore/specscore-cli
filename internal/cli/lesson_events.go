package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/specscore/specscore-cli/pkg/event"
)

type preparedLessonEvent struct {
	outbox      event.Outbox
	subscribers []event.Subscriber
	event       event.Event
	disabled    bool
}

// prepareLessonEvent persists the complete recipient set before the artifact
// mutation. A caller must either Commit after the mutation or Abort after
// rolling it back; a crash leaves an inspectable prepared record.
func prepareLessonEvent(root, name, slug string, payload map[string]any, at time.Time) (*preparedLessonEvent, error) {
	return prepareLessonEventWithID(root, name, slug, payload, at, uuid.NewString())
}

// prepareLessonEventWithID lets deterministic migrations bind their durable
// artifact transaction to one stable event envelope across crash recovery.
func prepareLessonEventWithID(root, name, slug string, payload map[string]any, at time.Time, id string) (*preparedLessonEvent, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	e := event.Event{Name: name, Version: 1, UUID: id, Timestamp: at.UTC(), Actor: event.Actor{Kind: "external", ID: "specscore"}, Artifact: event.Artifact{Type: "lesson", ID: slug, Path: filepath.ToSlash(filepath.Join("spec", "lessons", slug)), Revision: "uncommitted"}, Payload: b}
	subs, err := event.LoadSubscribers(root)
	if err != nil {
		return nil, err
	}
	// An explicit empty subscriber list is the project-level opt-out. Do not
	// create an invalid recipient-less ledger record; mutation remains focused.
	if len(subs) == 0 {
		return &preparedLessonEvent{subscribers: subs, event: e, disabled: true}, nil
	}
	o := event.NewOutbox(root)
	if err := o.Prepare(e, subs); err != nil {
		return nil, err
	}
	return &preparedLessonEvent{outbox: o, subscribers: subs, event: e}, nil
}

func (p *preparedLessonEvent) Abort() error {
	if p == nil || p.disabled {
		return nil
	}
	return p.outbox.Abort(p.event.UUID)
}

func (p *preparedLessonEvent) Commit(ctx context.Context) (event.ReplayResult, error) {
	if p == nil || p.disabled {
		return event.ReplayResult{}, nil
	}
	if err := p.outbox.Commit(p.event.UUID); err != nil {
		return event.ReplayResult{}, err
	}
	// Best effort is intentional only after durable enqueue: a failure remains
	// inspectable/replayable and does not suppress other subscribers.
	return p.outbox.Replay(ctx, p.subscribers, "", 0)
}

// emitLessonEvent is retained for non-artifact emitters. Mutating Lesson
// commands use prepareLessonEvent before their first artifact write.
func emitLessonEvent(ctx context.Context, root, name, slug string, payload map[string]any, at time.Time) error {
	p, err := prepareLessonEvent(root, name, slug, payload, at)
	if err != nil {
		return err
	}
	_, err = p.Commit(ctx)
	return err
}
