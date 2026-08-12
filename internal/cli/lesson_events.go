package cli

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"path/filepath"
	"time"

	"github.com/google/uuid"
	"github.com/specscore/specscore-cli/pkg/event"
	"github.com/specscore/specscore-cli/pkg/lesson"
)

func lessonIntentDigest(data []byte) string {
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

func marshalLessonIntentFacts(facts map[string]any) ([]byte, error) {
	if facts == nil {
		return []byte("{}"), nil
	}
	return json.Marshal(facts)
}

type preparedLessonEvent struct {
	outbox      event.Outbox
	subscribers []event.Subscriber
	event       event.Event
	disabled    bool
	resumed     bool
}

// lessonMutationCoordinator is the common fail-closed boundary for Lesson
// artifact commands. It owns prepared-event resolution, durability fencing,
// and commit ordering so individual verbs cannot invent weaker rollback or
// event semantics.
type lessonMutationCoordinator struct {
	prepared *preparedLessonEvent
	durable  durableFileOps
}

func newLessonMutationCoordinator(prepared *preparedLessonEvent, durable durableFileOps) lessonMutationCoordinator {
	return lessonMutationCoordinator{prepared: prepared, durable: durable}
}

func (c lessonMutationCoordinator) ResolveFailure(operation string, cause error) (bool, error) {
	return c.prepared.ResolveMutationFailure(operation, cause)
}

func (c lessonMutationCoordinator) Fence(paths ...string) error {
	for _, path := range paths {
		if err := durableFencePathWithOps(path, c.durable); err != nil {
			return fmt.Errorf("durably fencing %s: %w", path, err)
		}
	}
	return nil
}

func (c lessonMutationCoordinator) Commit(ctx context.Context) (event.ReplayResult, error) {
	return c.prepared.Commit(ctx)
}

// prepareLessonEvent persists the complete recipient set before the artifact
// mutation. A caller must either Commit after the mutation or Abort after
// rolling it back; a crash leaves an inspectable prepared record.
func prepareLessonEvent(root, name, slug string, payload map[string]any, at time.Time) (*preparedLessonEvent, error) {
	return prepareLessonEventWithID(root, name, slug, payload, at, uuid.NewString())
}

func prepareLessonIntentEvent(root, name, slug string, payload, intentFacts map[string]any, at time.Time) (*preparedLessonEvent, error) {
	return prepareLessonEventWithIntentID(root, name, slug, payload, intentFacts, at, uuid.NewString())
}

func resumePreparedLessonEvent(root, name, slug string, payload, intentFacts map[string]any) (*preparedLessonEvent, error) {
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	facts, err := marshalLessonIntentFacts(intentFacts)
	if err != nil {
		return nil, err
	}
	o := event.NewOutbox(root)
	intent := event.Event{
		Name: name, Version: 1,
		Actor:    event.Actor{Kind: "external", ID: "specscore"},
		Artifact: event.Artifact{Type: "lesson", ID: slug, Path: filepath.ToSlash(filepath.Join("spec", "lessons", slug)), Revision: "uncommitted"},
		Payload:  body,
	}
	match, err := o.FindPreparedIntent(intent, facts)
	if err != nil {
		return nil, err
	}
	if match == nil {
		return nil, nil
	}
	subscribers, err := event.LoadSubscribers(root)
	if err != nil {
		return nil, err
	}
	return &preparedLessonEvent{outbox: o, subscribers: subscribers, event: *match, resumed: true}, nil
}

// prepareLessonEventWithID lets deterministic migrations bind their durable
// artifact transaction to one stable event envelope across crash recovery.
func prepareLessonEventWithID(root, name, slug string, payload map[string]any, at time.Time, id string) (*preparedLessonEvent, error) {
	return prepareLessonEventWithIntentID(root, name, slug, payload, nil, at, id)
}

func prepareLessonEventWithIntentID(root, name, slug string, payload, intentFacts map[string]any, at time.Time, id string) (*preparedLessonEvent, error) {
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	facts, err := marshalLessonIntentFacts(intentFacts)
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
	if err := o.PrepareIntent(e, subs, facts); err != nil {
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

// ResolveMutationFailure is the only place a prepared Lesson event may be
// aborted after its writer has started.  A path merely existing is never proof
// of this writer's publication: writers return a MutationOutcome that records
// whether publication was impossible, compensation was durably completed, or
// a crash/fence failure leaves recovery work.  The latter must stay prepared
// and name its UUID so an operator can reconcile it deliberately.
func (p *preparedLessonEvent) ResolveMutationFailure(operation string, cause error) (recoveryRequired bool, err error) {
	outcome := lesson.MutationOutcomeOf(cause)
	if p == nil || p.disabled {
		if outcome == lesson.MutationPrePublication || outcome == lesson.MutationCompensated {
			return false, cause
		}
		ledger := "no prepared event is available"
		if p != nil && p.disabled {
			ledger = "event delivery was disabled, so there is no durable event record"
		}
		return true, fmt.Errorf("%s: %w; recovery required: artifact state is uncertain; %s; inspect the artifact before retrying", operation, cause, ledger)
	}
	if p.resumed {
		// This record predates the current attempt and may evidence publication
		// from that earlier attempt. A later prepublication/compensated failure
		// does not prove ownership of — or absence of — the earlier artifact.
		return true, fmt.Errorf("%s: %w; recovery required: resumed prepared event %s remains inspectable because this retry cannot abort an earlier attempt", operation, cause, p.event.UUID)
	}
	if outcome == lesson.MutationPrePublication || outcome == lesson.MutationCompensated {
		if err := p.Abort(); err == nil {
			return false, cause
		} else {
			cause = fmt.Errorf("%w; aborting fully-compensated event failed: %v", cause, err)
		}
	}
	return true, fmt.Errorf("%s: %w; recovery required: prepared event %s remains inspectable; reconcile it explicitly with `specscore event reconcile`", operation, cause, p.event.UUID)
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
