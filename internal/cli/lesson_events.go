package cli

import (
	"context"
	"encoding/json"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/specscore/specscore-cli/pkg/event"
)

// emitLessonEvent commits the event to one ledger and independent subscriber
// outboxes before delivery. A failed sink is left pending for `event replay`;
// it never makes another successful sink look acknowledged.
func emitLessonEvent(ctx context.Context, root, name, slug string, payload map[string]any, at time.Time) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	if at.IsZero() {
		at = time.Now().UTC()
	}
	e := event.Event{Name: name, Version: 1, UUID: uuid.NewString(), Timestamp: at.UTC(), Actor: event.Actor{Kind: "external", ID: "specscore"}, Artifact: event.Artifact{Type: "lesson", ID: slug, Path: filepath.ToSlash(filepath.Join("spec", "lessons", slug)), Revision: "uncommitted"}, Payload: b}
	subs, err := event.LoadSubscribers(root)
	if err != nil {
		return err
	}
	o := event.NewOutbox(root)
	if err := o.Enqueue(e, subs); err != nil {
		return err
	}
	// Best effort is intentional only after durable enqueue: a failure remains
	// inspectable/replayable and does not suppress other subscribers.
	_, err = o.Replay(ctx, subs, "", 0)
	return err
}
