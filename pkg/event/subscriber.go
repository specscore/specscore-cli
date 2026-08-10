package event

import (
	"context"
	"encoding/json"
	"time"
)

// Subscriber is the extension point for receiving dispatched events. Any type
// implementing Subscriber may be registered via the events: config block in
// specscore.yaml. Delivery is at-least-once: a process can crash after the
// subscriber succeeds but before its durable acknowledgement is written.
// Implementations MUST therefore be idempotent by Event.UUID across processes
// and invocations, not merely safe to call repeatedly within one invocation.
type Subscriber interface {
	// Deliver is invoked by the dispatcher with a validated envelope. It
	// returns nil on successful delivery and a non-nil error on any failure
	// (timeout, exec exit non-zero, filesystem error, etc.).
	Deliver(ctx context.Context, e Event) error

	// Name returns a stable identifier used in stderr failure logs. The
	// dispatcher does not interpret the string.
	Name() string
}

// Event is the common envelope passed to every Subscriber. Field shapes mirror
// the cross-repo event contract; see REQ:envelope-validation in
// spec/features/cli/event/README.md.
type Event struct {
	Name      string          `json:"name"`
	Version   int             `json:"version"`
	UUID      string          `json:"uuid"`
	Timestamp time.Time       `json:"timestamp"`
	Actor     Actor           `json:"actor"`
	Artifact  Artifact        `json:"artifact"`
	Payload   json.RawMessage `json:"payload"`
}

// Actor identifies the originator of an event.
type Actor struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// Artifact identifies the SpecScore artifact an event refers to.
type Artifact struct {
	Type     string `json:"type"`
	ID       string `json:"id"`
	Path     string `json:"path"`
	Revision string `json:"revision"`
}
