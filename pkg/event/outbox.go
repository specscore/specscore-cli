package event

// Durable per-subscriber delivery. The immutable ledger names every intended
// recipient, so pending markers are a reconstructible index rather than the
// source of truth. A crash between marker writes therefore cannot omit a sink.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

type Outbox struct{ Root string }

type outboxFile interface {
	Write([]byte) (int, error)
	Sync() error
	Close() error
	Name() string
}

type outboxFS interface {
	Stat(string) (os.FileInfo, error)
	ReadDir(string) ([]os.DirEntry, error)
	ReadFile(string) ([]byte, error)
	Mkdir(string, os.FileMode) error
	Open(string) (outboxFile, error)
	OpenFile(string, int, os.FileMode) (outboxFile, error)
	CreateTemp(string, string) (outboxFile, error)
	Link(string, string) error
	Remove(string) error
}

type osOutboxFS struct{}

func (osOutboxFS) Stat(path string) (os.FileInfo, error)      { return os.Stat(path) }
func (osOutboxFS) ReadDir(path string) ([]os.DirEntry, error) { return os.ReadDir(path) }
func (osOutboxFS) ReadFile(path string) ([]byte, error)       { return os.ReadFile(path) }
func (osOutboxFS) Mkdir(path string, perm os.FileMode) error  { return os.Mkdir(path, perm) }
func (osOutboxFS) Open(path string) (outboxFile, error)       { return os.Open(path) }
func (osOutboxFS) OpenFile(path string, flag int, perm os.FileMode) (outboxFile, error) {
	return os.OpenFile(path, flag, perm)
}
func (osOutboxFS) CreateTemp(dir, pattern string) (outboxFile, error) {
	return os.CreateTemp(dir, pattern)
}
func (osOutboxFS) Link(oldname, newname string) error { return os.Link(oldname, newname) }
func (osOutboxFS) Remove(path string) error           { return os.Remove(path) }

// outboxOperations carries an instance-local filesystem implementation for
// one operation. Public Outbox methods always construct it with osOutboxFS{};
// package tests may construct it with a deterministic fake without changing
// the exported Outbox representation or another operation's behavior.
type outboxOperations struct {
	Outbox
	fs    outboxFS
	codec outboxLedgerCodec
}

func (o Outbox) operation() outboxOperations { return outboxOperations{Outbox: o, fs: osOutboxFS{}} }

func (o outboxOperations) operations() outboxFS {
	if o.fs != nil {
		return o.fs
	}
	return osOutboxFS{}
}

// outboxLedgerCodec keeps decoding strict while allowing deterministic tests
// of malformed or unreadable durable ledgers without mutable package state.
type outboxLedgerCodec interface {
	Marshal(ledgerRecord) ([]byte, error)
	Unmarshal([]byte, *ledgerRecord) error
}

type jsonOutboxLedgerCodec struct{}

func (jsonOutboxLedgerCodec) Marshal(record ledgerRecord) ([]byte, error) {
	return json.Marshal(record)
}

func (jsonOutboxLedgerCodec) Unmarshal(data []byte, record *ledgerRecord) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(record); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err == nil {
		return fmt.Errorf("trailing JSON")
	} else if err != io.EOF {
		return err
	}
	return nil
}

func (o outboxOperations) ledgerCodec() outboxLedgerCodec {
	if o.codec != nil {
		return o.codec
	}
	return jsonOutboxLedgerCodec{}
}

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
func (o Outbox) ackPath(name, id string) string {
	return filepath.Join(o.Root, "ack", subscriberKey(name), id)
}
func (o Outbox) statePath(id string) string { return filepath.Join(o.Root, "state", id) }

type ledgerRecord struct {
	Event       Event    `json:"event"`
	Subscribers []string `json:"subscribers"`
	State       string   `json:"state"`
}

const preparedState = "prepared"
const committedState = "committed"
const abortedState = "aborted"

// Prepare durably records the envelope and its complete recipient set before
// an artifact mutation. Prepared records are visible/recoverable but not
// delivered until Commit.
func (o Outbox) Prepare(e Event, subscribers []Subscriber) error {
	return o.operation().prepare(e, subscribers)
}

func (o outboxOperations) prepare(e Event, subscribers []Subscriber) error {
	if err := Validate(e); err != nil {
		return err
	}
	names, err := subscriberNames(subscribers)
	if err != nil {
		return err
	}
	record := ledgerRecord{Event: e, Subscribers: names, State: preparedState}
	b, err := o.ledgerCodec().Marshal(record)
	if err != nil {
		return err
	}
	b = append(b, '\n')
	path := o.ledgerPath(e.UUID)
	if existing, readErr := o.operations().ReadFile(path); readErr == nil {
		if string(existing) != string(b) {
			return fmt.Errorf("event UUID %s already has different ledger content", e.UUID)
		}
		return o.syncOutboxDirectory(filepath.Dir(path))
	} else if !os.IsNotExist(readErr) {
		return readErr
	}
	if err := o.writeNewAtomic(path, b); err != nil {
		if !errors.Is(err, fs.ErrExist) {
			return err
		}
		existing, readErr := o.operations().ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if string(existing) != string(b) {
			return fmt.Errorf("event UUID %s already has different ledger content", e.UUID)
		}
	}
	return nil
}

// Commit makes a prepared event deliverable and reconstructs every pending
// marker from the ledger recipient set. Repeating Commit is safe.
func (o Outbox) Commit(id string) error { return o.operation().commit(id) }

func (o outboxOperations) commit(id string) error {
	record, err := o.readRecord(id)
	if err != nil {
		return err
	}
	state, err := o.readState(id)
	if err != nil {
		return err
	}
	return o.commitTransition(id, record, state)
}

func (o outboxOperations) commitTransition(id string, record ledgerRecord, state string) error {
	switch state {
	case abortedState:
		return fmt.Errorf("cannot commit aborted event %s", id)
	case preparedState:
		if err := o.decide(id, committedState); err != nil {
			return err
		}
	case committedState:
	default:
		return fmt.Errorf("event %s has invalid ledger state", id)
	}
	return o.recoverRecord(record)
}

func (o Outbox) Abort(id string) error { return o.operation().abort(id) }

func (o outboxOperations) abort(id string) error {
	_, err := o.readRecord(id)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	state, err := o.readState(id)
	if err != nil {
		return err
	}
	return o.abortTransition(id, state)
}

func (o outboxOperations) abortTransition(id, state string) error {
	switch state {
	case abortedState:
		return nil
	case committedState:
		return fmt.Errorf("cannot abort committed event %s", id)
	case preparedState:
		return o.decide(id, abortedState)
	default:
		return fmt.Errorf("event %s has invalid ledger state", id)
	}
}
func (o Outbox) Enqueue(e Event, subscribers []Subscriber) error {
	return o.operation().enqueue(e, subscribers)
}

func (o outboxOperations) enqueue(e Event, subscribers []Subscriber) error {
	if err := o.prepare(e, subscribers); err != nil {
		return err
	}
	return o.commit(e.UUID)
}

func (o Outbox) readRecord(id string) (ledgerRecord, error) { return o.operation().readRecord(id) }

func (o outboxOperations) readRecord(id string) (ledgerRecord, error) {
	var r ledgerRecord
	if !uuidRegex.MatchString(id) {
		return r, fmt.Errorf("invalid event UUID")
	}
	b, err := o.operations().ReadFile(o.ledgerPath(id))
	if err != nil {
		return r, err
	}
	if err := o.ledgerCodec().Unmarshal(b, &r); err != nil {
		return r, fmt.Errorf("invalid event ledger %s: %w", id, err)
	}
	if r.Event.UUID != id {
		return r, fmt.Errorf("event ledger filename and UUID differ")
	}
	if r.State != preparedState {
		return r, fmt.Errorf("event ledger %s is not an immutable prepared record", id)
	}
	if err := Validate(r.Event); err != nil {
		return r, fmt.Errorf("event ledger %s contains an invalid envelope: %w", id, err)
	}
	if err := validateLedgerSubscriberNames(r.Subscribers); err != nil {
		return r, fmt.Errorf("event ledger %s contains invalid subscribers: %w", id, err)
	}
	return r, nil
}

func validateLedgerSubscriberNames(names []string) error {
	if len(names) == 0 {
		return fmt.Errorf("subscriber set must not be empty")
	}
	previous := ""
	for i, name := range names {
		if name == "" {
			return fmt.Errorf("subscriber name must not be empty")
		}
		if i > 0 && name <= previous {
			return fmt.Errorf("subscriber names must be unique and sorted")
		}
		previous = name
	}
	return nil
}

func (o Outbox) readState(id string) (string, error) { return o.operation().readState(id) }

func (o outboxOperations) readState(id string) (string, error) {
	b, err := o.operations().ReadFile(o.statePath(id))
	if os.IsNotExist(err) {
		return preparedState, nil
	}
	if err != nil {
		return "", err
	}
	state := strings.TrimSpace(string(b))
	if state != committedState && state != abortedState {
		return "", fmt.Errorf("event %s has invalid decision marker", id)
	}
	return state, nil
}

// decide is a cross-process compare-and-set: commit and abort race to create
// the same immutable state marker. The winner is durable and the loser reads
// that decision rather than overwriting it.
func (o Outbox) decide(id, decision string) error { return o.operation().decide(id, decision) }

func (o outboxOperations) decide(id, decision string) error {
	if decision != committedState && decision != abortedState {
		return fmt.Errorf("invalid event decision")
	}
	err := o.writeNewAtomic(o.statePath(id), []byte(decision+"\n"))
	if err == nil {
		return nil
	}
	if !errors.Is(err, fs.ErrExist) {
		return err
	}
	actual, readErr := o.readState(id)
	if readErr != nil {
		return readErr
	}
	if actual != decision {
		return fmt.Errorf("event %s was already %s", id, actual)
	}
	return nil
}
func subscriberNames(subscribers []Subscriber) ([]string, error) {
	if len(subscribers) == 0 {
		return nil, fmt.Errorf("subscriber set must not be empty")
	}
	seen := map[string]bool{}
	names := make([]string, 0, len(subscribers))
	for _, s := range subscribers {
		name := s.Name()
		if name == "" {
			return nil, fmt.Errorf("subscriber name must not be empty")
		}
		if seen[name] {
			return nil, fmt.Errorf("duplicate subscriber name %q", name)
		}
		seen[name] = true
		names = append(names, name)
	}
	sort.Strings(names)
	return names, nil
}

// Recover rebuilds pending indexes for every committed ledger record. An ack
// is the durable per-subscriber cursor; a stale marker beside an ack is pruned.
func (o Outbox) Recover() error { return o.operation().recover() }

func (o outboxOperations) recover() error {
	entries, err := o.operations().ReadDir(filepath.Join(o.Root, "ledger"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := entry.Name()[:len(entry.Name())-5]
		record, err := o.readRecord(id)
		if err != nil {
			return err
		}
		state, err := o.readState(id)
		if err != nil {
			return err
		}
		if state == committedState {
			if err := o.recoverRecord(record); err != nil {
				return err
			}
		}
	}
	return nil
}
func (o outboxOperations) recoverRecord(record ledgerRecord) error {
	for _, name := range record.Subscribers {
		pending := o.pendingPath(name, record.Event.UUID)
		if _, err := o.operations().Stat(o.ackPath(name, record.Event.UUID)); err == nil {
			if err := o.removeOutboxFile(pending); err != nil {
				return err
			}
			continue
		} else if !os.IsNotExist(err) {
			return err
		}
		if err := o.touchExclusive(pending); err != nil {
			return err
		}
	}
	return nil
}

type ReplayResult struct {
	Delivered int
	Failed    []SubscriberFailure
	Pending   int
	Prepared  []PreparedRecord
}

// PreparedRecord is an interrupted two-phase event awaiting an explicit
// commit/abort decision. ArtifactExists is evidence for that decision, not an
// automatic conclusion: some mutations target an artifact that existed
// before the event was prepared.
type PreparedRecord struct {
	EventUUID      string    `json:"event_uuid"`
	EventName      string    `json:"event_name"`
	ArtifactPath   string    `json:"artifact_path"`
	ArtifactExists bool      `json:"artifact_exists"`
	Timestamp      time.Time `json:"timestamp"`
}

// Prepared lists unresolved prepared records in deterministic UUID order.
// It never follows absolute or parent-traversing artifact paths while
// collecting local evidence.
func (o Outbox) Prepared() ([]PreparedRecord, error) { return o.operation().prepared() }

func (o outboxOperations) prepared() ([]PreparedRecord, error) {
	entries, err := o.operations().ReadDir(filepath.Join(o.Root, "ledger"))
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	projectRoot := filepath.Dir(filepath.Dir(o.Root))
	var out []PreparedRecord
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" {
			continue
		}
		id := strings.TrimSuffix(entry.Name(), ".json")
		state, err := o.readState(id)
		if err != nil {
			return nil, err
		}
		if state != preparedState {
			continue
		}
		record, err := o.readRecord(id)
		if err != nil {
			return nil, err
		}
		item := PreparedRecord{EventUUID: id, EventName: record.Event.Name, ArtifactPath: record.Event.Artifact.Path, Timestamp: record.Event.Timestamp.UTC()}
		if safeArtifactEvidencePath(record.Event.Artifact.Path) {
			_, statErr := o.operations().Stat(filepath.Join(projectRoot, filepath.FromSlash(record.Event.Artifact.Path)))
			item.ArtifactExists = statErr == nil
			if statErr != nil && !os.IsNotExist(statErr) {
				return nil, statErr
			}
		}
		out = append(out, item)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].EventUUID < out[j].EventUUID })
	return out, nil
}

func safeArtifactEvidencePath(path string) bool {
	if path == "" || filepath.IsAbs(path) || strings.Contains(path, "\\") {
		return false
	}
	clean := filepath.ToSlash(filepath.Clean(path))
	return clean == path && clean != "." && clean != ".." && !strings.HasPrefix(clean, "../") && !strings.Contains(clean, "/../")
}

// ReconciliationToken binds an explicit operator decision to the unresolved
// event and the artifact-existence evidence they reviewed.
func ReconciliationToken(record PreparedRecord, decision string) string {
	sum := sha256.Sum256([]byte(record.EventUUID + "\x00" + decision + "\x00" + fmt.Sprint(record.ArtifactExists)))
	return hex.EncodeToString(sum[:8])
}

func (o Outbox) Replay(ctx context.Context, subscribers []Subscriber, name string, limit int) (ReplayResult, error) {
	return o.ReplayFrom(ctx, subscribers, name, "", limit)
}

// ReplayFrom retries pending deliveries in immutable ledger order. When from
// is non-empty, replay starts inclusively at that committed event's position
// in the ledger. The cursor remains useful after that event is acknowledged:
// ordering is derived from the ledger record, never from pending directory
// enumeration.
func (o Outbox) ReplayFrom(ctx context.Context, subscribers []Subscriber, name, from string, limit int) (ReplayResult, error) {
	return o.operation().replay(ctx, subscribers, name, from, limit)
}

type replayEntry struct {
	id        string
	timestamp time.Time
}

func (o outboxOperations) replay(ctx context.Context, subscribers []Subscriber, name, from string, limit int) (ReplayResult, error) {
	if err := o.recover(); err != nil {
		return ReplayResult{}, err
	}
	prepared, err := o.prepared()
	if err != nil {
		return ReplayResult{}, err
	}
	byName := map[string]Subscriber{}
	for _, s := range subscribers {
		if _, exists := byName[s.Name()]; exists {
			return ReplayResult{}, fmt.Errorf("duplicate subscriber name %q", s.Name())
		}
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
	result := ReplayResult{Prepared: prepared}
	var cursor *replayEntry
	if from != "" {
		record, err := o.readRecord(from)
		if err != nil {
			if os.IsNotExist(err) {
				return result, fmt.Errorf("unknown --from event %q", from)
			}
			return result, err
		}
		state, err := o.readState(from)
		if err != nil {
			return result, err
		}
		if state != committedState {
			return result, fmt.Errorf("--from event %s is not committed", from)
		}
		if name != "" && !containsSubscriber(record.Subscribers, name) {
			return result, fmt.Errorf("--from event %s was not addressed to subscriber %q", from, name)
		}
		cursor = &replayEntry{id: from, timestamp: record.Event.Timestamp.UTC()}
	}
	for _, n := range names {
		entries, err := o.operations().ReadDir(filepath.Dir(o.pendingPath(n, "x")))
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return result, err
		}
		ordered := make([]replayEntry, 0, len(entries))
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			record, err := o.readRecord(entry.Name())
			if err != nil {
				return result, err
			}
			ordered = append(ordered, replayEntry{id: entry.Name(), timestamp: record.Event.Timestamp.UTC()})
		}
		sort.Slice(ordered, func(i, j int) bool {
			if ordered[i].timestamp.Equal(ordered[j].timestamp) {
				return ordered[i].id < ordered[j].id
			}
			return ordered[i].timestamp.Before(ordered[j].timestamp)
		})
		for _, entry := range ordered {
			if cursor != nil && (entry.timestamp.Before(cursor.timestamp) || (entry.timestamp.Equal(cursor.timestamp) && entry.id < cursor.id)) {
				continue
			}
			if limit > 0 && result.Delivered+len(result.Failed) >= limit {
				return result, nil
			}
			result.Pending++
			id := entry.id
			record, err := o.readRecord(id)
			if err != nil {
				return result, err
			}
			state, err := o.readState(id)
			if err != nil {
				return result, err
			}
			if state != committedState {
				return result, fmt.Errorf("pending marker references non-committed event %s", id)
			}
			if err := byName[n].Deliver(ctx, record.Event); err != nil {
				result.Failed = append(result.Failed, SubscriberFailure{Name: n, Err: err})
				continue
			}
			if err := o.touchExclusive(o.ackPath(n, id)); err != nil {
				return result, err
			}
			if err := o.removeOutboxFile(o.pendingPath(n, id)); err != nil {
				return result, err
			}
			result.Delivered++
		}
	}
	return result, nil
}

func containsSubscriber(names []string, name string) bool {
	for _, candidate := range names {
		if candidate == name {
			return true
		}
	}
	return false
}

func (o outboxOperations) touchExclusive(path string) error {
	if err := o.ensureOutboxDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	f, err := o.operations().OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if os.IsExist(err) {
		return o.syncOutboxFileAndParent(path)
	}
	if err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return o.syncOutboxDirectory(filepath.Dir(path))
}

func (o outboxOperations) syncOutboxFileAndParent(path string) error {
	f, err := o.operations().Open(path)
	if err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return o.syncOutboxDirectory(filepath.Dir(path))
}

func (o outboxOperations) writeNewAtomic(path string, data []byte) error {
	if err := o.ensureOutboxDirectory(filepath.Dir(path)); err != nil {
		return err
	}
	f, err := o.operations().CreateTemp(filepath.Dir(path), ".event-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer func() { _ = o.operations().Remove(tmp) }()
	if _, err = f.Write(data); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = o.operations().Link(tmp, path); err != nil {
		if errors.Is(err, fs.ErrExist) {
			if syncErr := o.syncOutboxDirectory(filepath.Dir(path)); syncErr != nil {
				return syncErr
			}
		}
		return err
	}
	return o.syncOutboxDirectory(filepath.Dir(path))
}

func (o outboxOperations) syncOutboxDirectory(path string) error {
	f, err := o.operations().Open(path)
	if err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		_ = f.Close()
		return err
	}
	return f.Close()
}

func (o outboxOperations) ensureOutboxDirectory(path string) error {
	info, err := o.operations().Stat(path)
	if err == nil {
		if !info.IsDir() {
			return fmt.Errorf("outbox parent is not a directory")
		}
		return nil
	}
	if !os.IsNotExist(err) {
		return err
	}
	parent := filepath.Dir(path)
	if parent == path {
		return err
	}
	if err := o.ensureOutboxDirectory(parent); err != nil {
		return err
	}
	if err := o.operations().Mkdir(path, 0o755); err != nil && !os.IsExist(err) {
		return err
	}
	if err := o.syncOutboxDirectory(parent); err != nil {
		return err
	}
	return o.syncOutboxDirectory(path)
}

func (o outboxOperations) removeOutboxFile(path string) error {
	if err := o.operations().Remove(path); err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	return o.syncOutboxDirectory(filepath.Dir(path))
}
