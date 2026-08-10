package event

// Durable outbox. A successful sink never acknowledges another sink's work:
// each (subscriber,event UUID) has its own pending marker. Consumers must
// remain UUID-idempotent because a crash after delivery before acknowledgement
// can replay once more.

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

type Outbox struct{ Root string }

func NewOutbox(projectRoot string) Outbox {
	return Outbox{Root: filepath.Join(projectRoot, ".specscore", "event-outbox")}
}
func (o Outbox) ledgerPath(id string) string { return filepath.Join(o.Root, "ledger", id+".json") }
func subscriberKey(name string) string {
	s := sha256.Sum256([]byte(name))
	return hex.EncodeToString(s[:])
}
func (o Outbox) pendingPath(name, id string) string {
	return filepath.Join(o.Root, "pending", subscriberKey(name), id)
}

// Enqueue persists the immutable envelope before any delivery and creates a
// separate pending item for every named subscriber. It is idempotent for a
// UUID: existing ledger/pending records are left byte-identical.
func (o Outbox) Enqueue(e Event, subscribers []Subscriber) error {
	if err := Validate(e); err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(o.ledgerPath(e.UUID)), 0o755); err != nil {
		return err
	}
	b, err := json.Marshal(e)
	if err != nil {
		return err
	}
	f, err := os.OpenFile(o.ledgerPath(e.UUID), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil && !os.IsExist(err) {
		return err
	}
	if err == nil {
		if _, err = f.Write(append(b, '\n')); err != nil {
			_ = f.Close()
			_ = os.Remove(o.ledgerPath(e.UUID))
			return err
		}
		if err = f.Close(); err != nil {
			return err
		}
	}
	for _, sub := range subscribers {
		path := o.pendingPath(sub.Name(), e.UUID)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			return err
		}
		f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
		if err != nil && !os.IsExist(err) {
			return err
		}
		if err == nil {
			_ = f.Close()
		}
	}
	return nil
}

type ReplayResult struct {
	Delivered int
	Failed    []SubscriberFailure
	Pending   int
}

// Replay delivers only pending entries for the named subscriber (or every
// subscriber when name is empty). Success removes only that subscriber's
// marker. Errors are retained and reported without blocking other sinks.
func (o Outbox) Replay(ctx context.Context, subscribers []Subscriber, name string, limit int) (ReplayResult, error) {
	byName := map[string]Subscriber{}
	for _, s := range subscribers {
		byName[s.Name()] = s
	}
	if name != "" {
		if _, ok := byName[name]; !ok {
			return ReplayResult{}, fmt.Errorf("unknown subscriber %q", name)
		}
	}
	var names []string
	for n := range byName {
		if name == "" || name == n {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	var result ReplayResult
	for _, n := range names {
		entries, err := os.ReadDir(filepath.Dir(o.pendingPath(n, "x")))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return result, err
		}
		for _, entry := range entries {
			if limit > 0 && result.Delivered+len(result.Failed) >= limit {
				return result, nil
			}
			if entry.IsDir() {
				continue
			}
			result.Pending++
			id := entry.Name()
			b, err := os.ReadFile(o.ledgerPath(id))
			if err != nil {
				return result, err
			}
			var e Event
			if err := json.Unmarshal(b, &e); err != nil {
				return result, err
			}
			if err := byName[n].Deliver(ctx, e); err != nil {
				result.Failed = append(result.Failed, SubscriberFailure{Name: n, Err: err})
				continue
			}
			if err := os.Remove(o.pendingPath(n, id)); err != nil {
				return result, err
			}
			result.Delivered++
		}
	}
	return result, nil
}
