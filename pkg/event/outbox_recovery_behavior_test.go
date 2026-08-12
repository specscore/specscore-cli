package event

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type faultOutboxFS struct {
	fail string
	file outboxFile
}

func (f faultOutboxFS) Stat(path string) (os.FileInfo, error) {
	if f.fail == "stat" {
		return nil, errors.New("injected stat failure")
	}
	return os.Stat(path)
}
func (f faultOutboxFS) ReadDir(path string) ([]os.DirEntry, error) {
	if f.fail == "read-dir" {
		return nil, errors.New("injected read-dir failure")
	}
	return os.ReadDir(path)
}
func (f faultOutboxFS) ReadFile(path string) ([]byte, error) {
	if f.fail == "read-file" {
		return nil, errors.New("injected read-file failure")
	}
	return os.ReadFile(path)
}
func (f faultOutboxFS) Mkdir(path string, perm os.FileMode) error {
	if f.fail == "mkdir" {
		return errors.New("injected mkdir failure")
	}
	return os.Mkdir(path, perm)
}
func (f faultOutboxFS) Open(path string) (outboxFile, error) {
	if f.fail == "open" {
		return nil, errors.New("injected open failure")
	}
	if f.file != nil {
		return f.file, nil
	}
	return os.Open(path)
}
func (f faultOutboxFS) OpenFile(path string, flag int, perm os.FileMode) (outboxFile, error) {
	if f.fail == "open-file" {
		return nil, errors.New("injected open-file failure")
	}
	if f.file != nil {
		return f.file, nil
	}
	return os.OpenFile(path, flag, perm)
}
func (f faultOutboxFS) CreateTemp(dir, pattern string) (outboxFile, error) {
	if f.fail == "create-temp" {
		return nil, errors.New("injected create-temp failure")
	}
	if f.file != nil {
		return f.file, nil
	}
	return os.CreateTemp(dir, pattern)
}
func (f faultOutboxFS) Link(oldname, newname string) error {
	if f.fail == "link" {
		return errors.New("injected link failure")
	}
	return os.Link(oldname, newname)
}
func (f faultOutboxFS) Remove(path string) error {
	if f.fail == "remove" {
		return errors.New("injected remove failure")
	}
	return os.Remove(path)
}

type faultOutboxFile struct {
	name       string
	writeError error
	syncError  error
	closeError error
}

type collisionLinkFS struct {
	faultOutboxFS
	data []byte
}

type absentCollisionLinkFS struct{ faultOutboxFS }

func (absentCollisionLinkFS) Link(string, string) error { return fs.ErrExist }

type existingMarkerFaultFS struct {
	faultOutboxFS
	existing outboxFile
}

func (f existingMarkerFaultFS) OpenFile(string, int, os.FileMode) (outboxFile, error) {
	return nil, fs.ErrExist
}

func (f existingMarkerFaultFS) Open(string) (outboxFile, error) { return f.existing, nil }

type faultLedgerCodec struct {
	marshalErr   error
	unmarshalErr error
}

func (c faultLedgerCodec) Marshal(record ledgerRecord) ([]byte, error) {
	if c.marshalErr != nil {
		return nil, c.marshalErr
	}
	return json.Marshal(record)
}

func (c faultLedgerCodec) Unmarshal(data []byte, record *ledgerRecord) error {
	if c.unmarshalErr != nil {
		return c.unmarshalErr
	}
	return jsonOutboxLedgerCodec{}.Unmarshal(data, record)
}

type pendingReadDirFaultFS struct{ faultOutboxFS }

func (f pendingReadDirFaultFS) ReadDir(path string) ([]os.DirEntry, error) {
	if strings.Contains(filepath.ToSlash(path), "/pending/") {
		return nil, errors.New("injected pending read-dir failure")
	}
	return os.ReadDir(path)
}

type sequencedReadFileFS struct {
	faultOutboxFS
	failAt int
	reads  int
}

func (f *sequencedReadFileFS) ReadFile(path string) ([]byte, error) {
	f.reads++
	if f.reads == f.failAt {
		return nil, errors.New("injected sequenced read-file failure")
	}
	return os.ReadFile(path)
}

type ackOpenFileFaultFS struct{ faultOutboxFS }

func (f ackOpenFileFaultFS) OpenFile(path string, flag int, perm os.FileMode) (outboxFile, error) {
	if strings.Contains(filepath.ToSlash(path), "/ack/") {
		return nil, errors.New("injected ack open-file failure")
	}
	return os.OpenFile(path, flag, perm)
}

type missingStatFS struct{ faultOutboxFS }

func (missingStatFS) Stat(string) (os.FileInfo, error) { return nil, os.ErrNotExist }

type childMissingParentFaultFS struct {
	faultOutboxFS
	child string
}

func (f childMissingParentFaultFS) Stat(path string) (os.FileInfo, error) {
	if path == f.child {
		return nil, os.ErrNotExist
	}
	return nil, errors.New("injected parent stat failure")
}

func (f collisionLinkFS) Link(_ string, destination string) error {
	if err := os.WriteFile(destination, f.data, 0o644); err != nil {
		return err
	}
	return fs.ErrExist
}

func (f faultOutboxFile) Write([]byte) (int, error) { return 0, f.writeError }
func (f faultOutboxFile) Sync() error               { return f.syncError }
func (f faultOutboxFile) Close() error              { return f.closeError }
func (f faultOutboxFile) Name() string              { return f.name }

func TestOutbox_RecoverPrunesStalePendingMarkerAfterAck(t *testing.T) {
	sink := &outboxSub{name: "sink"}
	o := NewOutbox(t.TempDir())
	e := validEvent()
	if err := o.Enqueue(e, []Subscriber{sink}); err != nil {
		t.Fatal(err)
	}
	if result, err := o.Replay(context.Background(), []Subscriber{sink}, "", 0); err != nil || result.Delivered != 1 {
		t.Fatalf("initial replay = %#v, %v", result, err)
	}

	// Model a crash after writing the durable ack but before an interrupted
	// cleanup re-created the pending marker. Recovery must treat the ack as the
	// cursor and must not redeliver the event.
	if err := os.WriteFile(o.pendingPath("sink", e.UUID), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := o.Recover(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(o.pendingPath("sink", e.UUID)); !os.IsNotExist(err) {
		t.Fatalf("stale pending marker remains after recovery: %v", err)
	}
	if result, err := o.Replay(context.Background(), []Subscriber{sink}, "", 0); err != nil || result.Delivered != 0 || len(sink.seen) != 1 {
		t.Fatalf("replay after acknowledged recovery = %#v, %v; seen=%v", result, err, sink.seen)
	}
}

func TestReconciliationToken_BindsDecisionAndReviewedEvidence(t *testing.T) {
	record := PreparedRecord{EventUUID: "00000000-0000-4000-8000-000000000000", ArtifactExists: true}
	commitWithArtifact := ReconciliationToken(record, committedState)
	if commitWithArtifact != ReconciliationToken(record, committedState) {
		t.Fatal("same reviewed decision must produce a stable reconciliation token")
	}
	if commitWithArtifact == ReconciliationToken(record, abortedState) {
		t.Fatal("different decision must not reuse reconciliation token")
	}
	record.ArtifactExists = false
	if commitWithArtifact == ReconciliationToken(record, committedState) {
		t.Fatal("changed reviewed evidence must not reuse reconciliation token")
	}
}

func TestOutbox_RejectsConflictingLedgerAndInvalidDurableMarkers(t *testing.T) {
	o := NewOutbox(t.TempDir())
	e := validEvent()
	sink := &outboxSub{name: "sink"}
	if err := o.Prepare(e, []Subscriber{sink}); err != nil {
		t.Fatal(err)
	}
	changed := e
	changed.Payload = []byte(`{"changed":true}`)
	if err := o.Prepare(changed, []Subscriber{sink}); err == nil || !strings.Contains(err.Error(), "different ledger") {
		t.Fatalf("conflicting retry must preserve immutable ledger, got %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(o.statePath(e.UUID)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(o.statePath(e.UUID), []byte("unknown\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := o.Commit(e.UUID); err == nil || !strings.Contains(err.Error(), "invalid decision marker") {
		t.Fatalf("invalid durable marker must fail closed, got %v", err)
	}
}

func TestOutbox_PrepareAndDecisionRejectUnsafeRecipientsAndStateTransitions(t *testing.T) {
	o := NewOutbox(t.TempDir())
	e := validEvent()
	if err := o.Prepare(e, nil); err == nil {
		t.Fatal("recipientless ledger must be refused")
	}
	if err := o.Prepare(e, []Subscriber{&outboxSub{name: "same"}, &outboxSub{name: "same"}}); err == nil {
		t.Fatal("duplicate subscriber names must be refused")
	}
	if err := o.Abort(e.UUID); err != nil {
		t.Fatalf("aborting an absent ledger must be idempotent: %v", err)
	}
	if err := o.Prepare(e, []Subscriber{&outboxSub{name: "sink"}}); err != nil {
		t.Fatal(err)
	}
	if err := o.decide(e.UUID, "not-a-decision"); err == nil {
		t.Fatal("unknown explicit decision must be refused")
	}
	if err := o.Abort(e.UUID); err != nil {
		t.Fatal(err)
	}
	if err := o.Commit(e.UUID); err == nil || !strings.Contains(err.Error(), "cannot commit aborted") {
		t.Fatalf("terminal abort must prevent commit, got %v", err)
	}
}

func TestOutbox_PreparedFiltersUnsafeEvidenceAndSortsUnresolvedRecords(t *testing.T) {
	root := t.TempDir()
	o := NewOutbox(root)
	first := validEvent()
	first.UUID = "00000000-0000-4000-8000-000000000001"
	first.Artifact.Path = "spec/ideas/absent.md"
	second := validEvent()
	second.UUID = "00000000-0000-4000-8000-000000000002"
	second.Artifact.Path = "spec/ideas/present.md"
	artifact := filepath.Join(root, "spec", "ideas", "present.md")
	if err := os.MkdirAll(filepath.Dir(artifact), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(artifact, []byte("present\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, e := range []Event{second, first} {
		if err := o.Prepare(e, []Subscriber{&outboxSub{name: "audit"}}); err != nil {
			t.Fatal(err)
		}
	}
	prepared, err := o.Prepared()
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared) != 2 || prepared[0].EventUUID != first.UUID || prepared[0].ArtifactExists || !prepared[1].ArtifactExists {
		t.Fatalf("prepared evidence must be sorted and containment-safe: %#v", prepared)
	}
	for _, path := range []string{"", "/absolute", "../escape", "a/../b", `a\\b`, ".", ".."} {
		if safeArtifactEvidencePath(path) {
			t.Fatalf("unsafe artifact path accepted: %q", path)
		}
	}
	if !safeArtifactEvidencePath("spec/ideas/present.md") {
		t.Fatal("normalized repository-relative evidence should be accepted")
	}
}

func TestOutbox_ReplayRejectsUnknownSubscriberAndNonCommittedPendingMarker(t *testing.T) {
	o := NewOutbox(t.TempDir())
	e := validEvent()
	sink := &outboxSub{name: "sink"}
	if err := o.Prepare(e, []Subscriber{sink}); err != nil {
		t.Fatal(err)
	}
	if _, err := o.Replay(context.Background(), []Subscriber{sink}, "missing", 0); err == nil || !strings.Contains(err.Error(), "unknown subscriber") {
		t.Fatalf("targeting an unconfigured subscriber must fail: %v", err)
	}
	if _, err := o.Replay(context.Background(), []Subscriber{sink, sink}, "", 0); err == nil || !strings.Contains(err.Error(), "duplicate") {
		t.Fatalf("duplicate runtime subscriber must fail: %v", err)
	}
	if err := os.MkdirAll(filepath.Dir(o.pendingPath("sink", e.UUID)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(o.pendingPath("sink", e.UUID), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := o.Replay(context.Background(), []Subscriber{sink}, "", 0); err == nil || !strings.Contains(err.Error(), "non-committed") {
		t.Fatalf("pending prepared event must not be delivered: %v", err)
	}
}

func TestOutbox_InstanceScopedFilesystemFailuresAreDeterministic(t *testing.T) {
	root := t.TempDir()
	base := Outbox{Root: filepath.Join(root, "base")}
	if err := os.MkdirAll(base.Root, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		run  func(outboxOperations) error
	}{
		{"stat", func(o outboxOperations) error { return o.ensureOutboxDirectory(filepath.Join(root, "missing")) }},
		{"mkdir", func(o outboxOperations) error { return o.ensureOutboxDirectory(filepath.Join(root, "new-dir")) }},
		{"open", func(o outboxOperations) error { return o.syncOutboxDirectory(base.Root) }},
		{"open-file", func(o outboxOperations) error { return o.touchExclusive(filepath.Join(base.Root, "entry")) }},
		{"create-temp", func(o outboxOperations) error {
			return o.writeNewAtomic(filepath.Join(base.Root, "ledger"), []byte("x"))
		}},
		{"link", func(o outboxOperations) error {
			return o.writeNewAtomic(filepath.Join(base.Root, "linked"), []byte("x"))
		}},
		{"remove", func(o outboxOperations) error { return o.removeOutboxFile(filepath.Join(base.Root, "absent")) }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := outboxOperations{Outbox: base, fs: faultOutboxFS{fail: tc.name}}
			if err := tc.run(o); err == nil || !strings.Contains(err.Error(), "injected") {
				t.Fatalf("%s failure = %v", tc.name, err)
			}
		})
	}
}

func TestOutbox_DirectorySyncAndCloseFailuresAreDurabilityErrors(t *testing.T) {
	root := t.TempDir()
	for name, file := range map[string]faultOutboxFile{
		"sync":  {name: root, syncError: errors.New("injected directory sync failure")},
		"close": {name: root, closeError: errors.New("injected directory close failure")},
	} {
		t.Run(name, func(t *testing.T) {
			o := outboxOperations{Outbox: Outbox{Root: root}, fs: faultOutboxFS{file: file}}
			if err := o.syncOutboxDirectory(root); err == nil || !strings.Contains(err.Error(), "injected") {
				t.Fatalf("directory %s failure = %v", name, err)
			}
		})
	}
}

func TestOutbox_IdenticalPublicationRetryRefencesParent(t *testing.T) {
	t.Run("existing ledger", func(t *testing.T) {
		base := NewOutbox(t.TempDir())
		e := validEvent()
		subscribers := []Subscriber{&outboxSub{name: "sink"}}
		if err := base.Prepare(e, subscribers); err != nil {
			t.Fatal(err)
		}
		faulty := outboxOperations{Outbox: base, fs: faultOutboxFS{fail: "open"}}
		if err := faulty.prepare(e, subscribers); err == nil || !strings.Contains(err.Error(), "injected open") {
			t.Fatalf("existing ledger retry skipped parent fence: %v", err)
		}
		if err := base.Prepare(e, subscribers); err != nil {
			t.Fatalf("clean ledger retry did not re-fence: %v", err)
		}
	})

	t.Run("existing marker", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "ack", "existing")
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, nil, 0o644); err != nil {
			t.Fatal(err)
		}
		faulty := outboxOperations{Outbox: Outbox{Root: root}, fs: faultOutboxFS{fail: "open"}}
		if err := faulty.touchExclusive(path); err == nil || !strings.Contains(err.Error(), "injected open") {
			t.Fatalf("existing marker retry skipped parent fence: %v", err)
		}
		if err := (outboxOperations{Outbox: Outbox{Root: root}}).touchExclusive(path); err != nil {
			t.Fatalf("clean marker retry did not re-fence: %v", err)
		}
		for name, file := range map[string]faultOutboxFile{
			"file sync":  {name: path, syncError: errors.New("marker sync")},
			"file close": {name: path, closeError: errors.New("marker close")},
		} {
			t.Run(name, func(t *testing.T) {
				faulty := outboxOperations{Outbox: Outbox{Root: root}, fs: existingMarkerFaultFS{existing: file}}
				if err := faulty.touchExclusive(path); err == nil || !strings.Contains(err.Error(), "marker") {
					t.Fatalf("existing marker %s retry = %v", name, err)
				}
			})
		}
	})

	t.Run("exclusive-link collision", func(t *testing.T) {
		root := t.TempDir()
		path := filepath.Join(root, "collision")
		faulty := outboxOperations{Outbox: Outbox{Root: root}, fs: collisionLinkFS{faultOutboxFS: faultOutboxFS{fail: "open"}, data: []byte("same")}}
		if err := faulty.writeNewAtomic(path, []byte("same")); err == nil || !strings.Contains(err.Error(), "injected open") {
			t.Fatalf("link collision skipped parent fence: %v", err)
		}
		if got, err := os.ReadFile(path); err != nil || string(got) != "same" {
			t.Fatalf("collision publication = %q, %v", got, err)
		}
	})
}

func TestOutbox_OperationsAreInstanceScopedUnderConcurrentFaultInjection(t *testing.T) {
	root := t.TempDir()
	failing := outboxOperations{Outbox: Outbox{Root: filepath.Join(root, "failing")}, fs: faultOutboxFS{fail: "link"}}
	working := NewOutbox(filepath.Join(root, "working"))
	e := validEvent()
	e.UUID = "00000000-0000-4000-8000-000000000009"
	start := make(chan struct{})
	var wg sync.WaitGroup
	var failErr, workErr error
	wg.Add(2)
	go func() {
		defer wg.Done()
		<-start
		failErr = failing.prepare(e, []Subscriber{&outboxSub{name: "sink"}})
	}()
	go func() {
		defer wg.Done()
		<-start
		workErr = working.Enqueue(e, []Subscriber{&outboxSub{name: "sink"}})
	}()
	close(start)
	wg.Wait()
	if failErr == nil || !strings.Contains(failErr.Error(), "injected link") {
		t.Fatalf("injected instance did not fail independently: %v", failErr)
	}
	if workErr != nil {
		t.Fatalf("default instance inherited injected failure: %v", workErr)
	}
	if _, err := os.Stat(working.ledgerPath(e.UUID)); err != nil {
		t.Fatalf("default outbox did not durably prepare its own ledger: %v", err)
	}
}

func TestOutbox_DurabilityFileFailuresStopPublicationBeforeStateChange(t *testing.T) {
	root := t.TempDir()
	for _, tc := range []struct {
		name string
		run  func(outboxOperations) error
	}{
		{"touch-sync", func(o outboxOperations) error { return o.touchExclusive(filepath.Join(root, "touch-sync")) }},
		{"touch-close", func(o outboxOperations) error { return o.touchExclusive(filepath.Join(root, "touch-close")) }},
		{"write-write", func(o outboxOperations) error {
			return o.writeNewAtomic(filepath.Join(root, "write-write"), []byte("x"))
		}},
		{"write-sync", func(o outboxOperations) error {
			return o.writeNewAtomic(filepath.Join(root, "write-sync"), []byte("x"))
		}},
		{"write-close", func(o outboxOperations) error {
			return o.writeNewAtomic(filepath.Join(root, "write-close"), []byte("x"))
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file := faultOutboxFile{name: filepath.Join(root, "temporary")}
			switch tc.name {
			case "touch-sync", "write-sync":
				file.syncError = errors.New("injected sync failure")
			case "touch-close", "write-close":
				file.closeError = errors.New("injected close failure")
			case "write-write":
				file.writeError = errors.New("injected write failure")
			}
			o := outboxOperations{Outbox: Outbox{Root: root}, fs: faultOutboxFS{file: file}}
			if err := tc.run(o); err == nil || !strings.Contains(err.Error(), "injected") {
				t.Fatalf("%s = %v", tc.name, err)
			}
		})
	}
}

func TestOutbox_ReadRecordRejectsEveryUnsafeDurableLedgerShape(t *testing.T) {
	o := NewOutbox(t.TempDir())
	id := validEvent().UUID
	valid := ledgerRecord{Event: validEvent(), Subscribers: []string{"sink"}, State: preparedState}
	encode := func(t *testing.T, record ledgerRecord) []byte {
		t.Helper()
		b, err := json.Marshal(record)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	write := func(t *testing.T, b []byte) {
		t.Helper()
		if err := os.MkdirAll(filepath.Dir(o.ledgerPath(id)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(o.ledgerPath(id), b, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := o.readRecord("not-a-uuid"); err == nil {
		t.Fatal("invalid event UUID must be rejected before a filesystem read")
	}
	for _, tc := range []struct {
		name string
		body []byte
	}{
		{"invalid-json", []byte("{")},
		{"trailing-json", append(append(encode(t, valid), '\n'), []byte("{}")...)},
		{"filename-mismatch", encode(t, ledgerRecord{Event: func() Event { e := valid.Event; e.UUID = "00000000-0000-4000-8000-000000000001"; return e }(), Subscribers: valid.Subscribers, State: preparedState})},
		{"non-prepared-record", encode(t, ledgerRecord{Event: valid.Event, Subscribers: valid.Subscribers, State: committedState})},
		{"invalid-envelope", encode(t, ledgerRecord{Event: Event{UUID: id}, Subscribers: valid.Subscribers, State: preparedState})},
		{"unsorted-subscribers", encode(t, ledgerRecord{Event: valid.Event, Subscribers: []string{"z", "a"}, State: preparedState})},
	} {
		t.Run(tc.name, func(t *testing.T) {
			write(t, tc.body)
			if _, err := o.readRecord(id); err == nil {
				t.Fatal("unsafe ledger must be rejected")
			}
		})
	}
}

func TestOutbox_DecisionRetriesReadExistingCASWinnerWithoutOverwritingIt(t *testing.T) {
	o := NewOutbox(t.TempDir())
	e := validEvent()
	if err := o.Prepare(e, []Subscriber{&outboxSub{name: "sink"}}); err != nil {
		t.Fatal(err)
	}
	if err := o.decide(e.UUID, committedState); err != nil {
		t.Fatal(err)
	}
	if err := o.decide(e.UUID, committedState); err != nil {
		t.Fatalf("same durable decision must be idempotent: %v", err)
	}
	if err := o.decide(e.UUID, abortedState); err == nil || !strings.Contains(err.Error(), "already committed") {
		t.Fatalf("conflicting durable decision must be refused: %v", err)
	}
}

func TestOutbox_FindPreparedIntentCanonicalizesPrivatelyAndFailsClosed(t *testing.T) {
	if found, err := NewOutbox(t.TempDir()).FindPreparedIntent(validEvent()); err != nil || found != nil {
		t.Fatalf("empty prepared intent lookup = %#v, %v", found, err)
	}
	o := NewOutbox(t.TempDir())
	e := validEvent()
	e.Payload = json.RawMessage(`{"z":1,"nested":{"b":2,"a":1}}`)
	if err := o.Prepare(e, []Subscriber{&outboxSub{name: "sink"}}); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(o.Root, "ledger", "ignored.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(o.Root, "ledger", "ignored.txt"), []byte("ignored"), 0o644); err != nil {
		t.Fatal(err)
	}
	decided := validEvent()
	decided.UUID = "00000000-0000-4000-8000-000000000031"
	if err := o.Prepare(decided, []Subscriber{&outboxSub{name: "sink"}}); err != nil {
		t.Fatal(err)
	}
	if err := o.Commit(decided.UUID); err != nil {
		t.Fatal(err)
	}
	intent := e
	intent.UUID = ""
	intent.Timestamp = time.Time{}
	intent.Payload = json.RawMessage(` { "nested" : { "a" : 1, "b" : 2 }, "z" : 1 } `)
	found, err := o.FindPreparedIntent(intent)
	if err != nil || found == nil || found.UUID != e.UUID {
		t.Fatalf("canonical prepared intent lookup = %#v, %v", found, err)
	}
	prepared, err := o.Prepared()
	if err != nil {
		t.Fatal(err)
	}
	public, err := json.Marshal(prepared)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(public, []byte("intent_fingerprint")) || bytes.Contains(public, []byte("nested")) {
		t.Fatalf("prepared reconciliation output exposed private intent material: %s", public)
	}

	different := intent
	different.Name = "lesson.different"
	if found, err := o.FindPreparedIntent(different); err != nil || found != nil {
		t.Fatalf("cross-command intent matched = %#v, %v", found, err)
	}
	readFault := outboxOperations{Outbox: o, fs: faultOutboxFS{fail: "read-file"}}
	if _, err := readFault.findPreparedIntent(intent); err == nil || !strings.Contains(err.Error(), "injected read-file") {
		t.Fatalf("prepared intent state read failure = %v", err)
	}
	constantHash := func([]byte) [sha256.Size]byte { return [sha256.Size]byte{} }
	if _, err := o.operation().findPreparedIntentWithHash(different, constantHash); err == nil || !strings.Contains(err.Error(), "collision") {
		t.Fatalf("full tuple did not reject a fingerprint collision: %v", err)
	}

	second := e
	second.UUID = "00000000-0000-4000-8000-000000000032"
	second.Timestamp = second.Timestamp.Add(time.Second)
	if err := o.Prepare(second, []Subscriber{&outboxSub{name: "sink"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := o.FindPreparedIntent(intent); err == nil || !strings.Contains(err.Error(), "multiple prepared") {
		t.Fatalf("ambiguous prepared intents were accepted: %v", err)
	}

	bad := intent
	bad.Payload = json.RawMessage(`{}` + `{}`)
	if _, err := o.FindPreparedIntent(bad); err == nil || !strings.Contains(err.Error(), "trailing JSON") {
		t.Fatalf("non-canonicalizable intent was accepted: %v", err)
	}
	bad.Payload = json.RawMessage(`{`)
	if _, err := o.FindPreparedIntent(bad); err == nil {
		t.Fatal("invalid intent payload was accepted")
	}
	readFault = outboxOperations{Outbox: o, fs: faultOutboxFS{fail: "read-dir"}}
	if _, err := readFault.findPreparedIntent(intent); err == nil || !strings.Contains(err.Error(), "injected read-dir") {
		t.Fatalf("prepared intent directory failure = %v", err)
	}
	corrupt := NewOutbox(t.TempDir())
	corruptID := "00000000-0000-4000-8000-000000000033"
	if err := os.MkdirAll(filepath.Dir(corrupt.ledgerPath(corruptID)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(corrupt.ledgerPath(corruptID), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := corrupt.FindPreparedIntent(intent); err == nil || !strings.Contains(err.Error(), "invalid event ledger") {
		t.Fatalf("corrupt prepared intent ledger = %v", err)
	}
}

func TestOutbox_TerminalDecisionRetriesReFenceStateParentAfterInterruptedSync(t *testing.T) {
	for _, tc := range []struct {
		name       string
		id         string
		decision   string
		transition func(outboxOperations, string, string) error
	}{
		{
			name:     "committed",
			id:       validEvent().UUID,
			decision: committedState,
			transition: func(o outboxOperations, id, state string) error {
				return o.commitTransition(id, ledgerRecord{}, state)
			},
		},
		{
			name:     "aborted",
			id:       "00000000-0000-4000-8000-000000000031",
			decision: abortedState,
			transition: func(o outboxOperations, id, state string) error {
				return o.abortTransition(id, state)
			},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := NewOutbox(t.TempDir())
			if err := os.MkdirAll(filepath.Dir(o.statePath(tc.id)), 0o755); err != nil {
				t.Fatal(err)
			}
			faulty := outboxOperations{Outbox: o, fs: faultOutboxFS{fail: "open"}}
			if err := tc.transition(faulty, tc.id, preparedState); err == nil || !strings.Contains(err.Error(), "injected open") {
				t.Fatalf("initial decision parent-sync failure = %v", err)
			}
			state, err := o.readState(tc.id)
			if err != nil || state != tc.decision {
				t.Fatalf("published decision after interrupted parent sync = %q, %v", state, err)
			}
			if err := tc.transition(faulty, tc.id, tc.decision); err == nil || !strings.Contains(err.Error(), "injected open") {
				t.Fatalf("terminal retry must re-fence the state parent: %v", err)
			}
			if err := tc.transition(o.operation(), tc.id, tc.decision); err != nil {
				t.Fatalf("clean terminal retry must converge after re-fencing: %v", err)
			}
		})
	}
}

func TestOutbox_RecoverAndReplayHandleInterruptedLedgerDirectorySafely(t *testing.T) {
	o := NewOutbox(t.TempDir())
	if err := o.Recover(); err != nil {
		t.Fatalf("empty outbox recovery must be harmless: %v", err)
	}
	e := validEvent()
	sink := &outboxSub{name: "sink"}
	if err := o.Enqueue(e, []Subscriber{sink}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(o.Root, "ledger", "notes.txt"), []byte("not a ledger"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(o.Root, "ledger", "ignored.json"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := o.Recover(); err != nil {
		t.Fatalf("non-ledger entries must not block recovery: %v", err)
	}
	if result, err := o.Replay(context.Background(), []Subscriber{sink}, "", 1); err != nil || result.Delivered != 1 || result.Pending != 1 {
		t.Fatalf("limit should deliver exactly one pending recipient: %#v, %v", result, err)
	}
	if err := os.Mkdir(filepath.Dir(o.pendingPath(sink.name, "x")), 0o755); err != nil && !os.IsExist(err) {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(filepath.Dir(o.pendingPath(sink.name, "x")), "directory-marker"), 0o755); err != nil {
		t.Fatal(err)
	}
	if result, err := o.Replay(context.Background(), []Subscriber{sink}, "", 0); err != nil || result.Delivered != 0 {
		t.Fatalf("directory marker must be ignored, got %#v, %v", result, err)
	}
}

func TestOutbox_TerminalStatesAndLedgerCollisionsFailClosedWithoutDelivery(t *testing.T) {
	o := NewOutbox(t.TempDir())
	e := validEvent()
	sink := &outboxSub{name: "sink"}
	if err := o.Commit(e.UUID); err == nil {
		t.Fatal("committing a missing immutable ledger must fail")
	}
	if err := o.Prepare(e, []Subscriber{sink}); err != nil {
		t.Fatal(err)
	}
	if err := o.Commit(e.UUID); err != nil {
		t.Fatal(err)
	}
	if err := o.Abort(e.UUID); err == nil || !strings.Contains(err.Error(), "cannot abort committed") {
		t.Fatalf("committed ledger cannot be retroactively aborted: %v", err)
	}
	if err := o.Commit(e.UUID); err != nil {
		t.Fatalf("repeating committed decision must reconstruct pending safely: %v", err)
	}
	if err := o.Prepare(e, []Subscriber{sink}); err != nil {
		t.Fatalf("byte-identical ledger retry must be accepted: %v", err)
	}
	if err := os.Remove(o.ledgerPath(e.UUID)); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(o.ledgerPath(e.UUID), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := o.Prepare(e, []Subscriber{sink}); err == nil {
		t.Fatal("a non-file ledger collision must not be overwritten")
	}
}

func TestOutbox_InvalidInputsAndBrokenDirectoryTopologyFailClosed(t *testing.T) {
	t.Run("invalid event and unnamed subscriber", func(t *testing.T) {
		o := NewOutbox(t.TempDir())
		invalid := validEvent()
		invalid.Name = "Invalid.Name"
		if err := o.Enqueue(invalid, []Subscriber{&outboxSub{name: "sink"}}); err == nil {
			t.Fatal("invalid envelope must fail before writing a ledger")
		}
		if err := o.Prepare(validEvent(), []Subscriber{&outboxSub{name: ""}}); err == nil {
			t.Fatal("unnamed subscriber must fail before writing a ledger")
		}
	})
	t.Run("existing ledger path is a directory", func(t *testing.T) {
		o := NewOutbox(t.TempDir())
		e := validEvent()
		if err := os.MkdirAll(o.ledgerPath(e.UUID), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := o.Prepare(e, []Subscriber{&outboxSub{name: "sink"}}); err == nil {
			t.Fatal("directory cannot masquerade as immutable ledger")
		}
	})
	t.Run("ledger directory unreadable shape", func(t *testing.T) {
		o := NewOutbox(t.TempDir())
		if err := os.MkdirAll(filepath.Dir(filepath.Join(o.Root, "ledger")), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(o.Root, "ledger"), []byte("file"), 0o644); err != nil {
			t.Fatal(err)
		}
		if err := o.Recover(); err == nil {
			t.Fatal("ledger file must not be treated as recoverable directory")
		}
		if _, err := o.Prepared(); err == nil {
			t.Fatal("ledger file must not be treated as prepared-record directory")
		}
	})
	t.Run("pending subscriber directory is a file", func(t *testing.T) {
		o := NewOutbox(t.TempDir())
		sink := &outboxSub{name: "sink"}
		if err := o.Enqueue(validEvent(), []Subscriber{sink}); err != nil {
			t.Fatal(err)
		}
		pendingDir := filepath.Dir(o.pendingPath(sink.name, "x"))
		if err := os.RemoveAll(pendingDir); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(pendingDir, []byte("file"), 0o644); err != nil {
			t.Fatal(err)
		}
		if _, err := o.Replay(context.Background(), []Subscriber{sink}, "", 0); err == nil {
			t.Fatal("file in place of pending directory must fail replay")
		}
	})
}

func TestOutbox_InterruptedDecisionsAndCorruptRecordsNeverBecomeDeliverable(t *testing.T) {
	root := t.TempDir()
	o := NewOutbox(root)
	e := validEvent()
	sink := &outboxSub{name: "sink"}
	if err := o.Prepare(e, []Subscriber{sink}); err != nil {
		t.Fatal(err)
	}
	if err := o.Abort(e.UUID); err != nil {
		t.Fatal(err)
	}
	if err := o.Abort(e.UUID); err != nil {
		t.Fatalf("repeating abort must preserve terminal decision: %v", err)
	}

	second := validEvent()
	second.UUID = "00000000-0000-4000-8000-000000000010"
	if err := o.Prepare(second, []Subscriber{sink}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(o.statePath(second.UUID), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := o.Commit(second.UUID); err == nil {
		t.Fatal("state marker directory must prevent commit")
	}
	if err := o.Abort(second.UUID); err == nil {
		t.Fatal("state marker directory must prevent abort")
	}

	third := validEvent()
	third.UUID = "00000000-0000-4000-8000-000000000011"
	if err := os.MkdirAll(filepath.Dir(o.ledgerPath(third.UUID)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(o.ledgerPath(third.UUID), []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := o.Recover(); err == nil {
		t.Fatal("corrupt ledger must stop recovery before delivery")
	}
	if _, err := o.Prepared(); err == nil {
		t.Fatal("corrupt ledger must stop prepared reconciliation view")
	}
}

func TestOutbox_RecoveryAndReplaySurfaceDurabilityCleanupFailures(t *testing.T) {
	root := t.TempDir()
	o := NewOutbox(root)
	e := validEvent()
	sink := &outboxSub{name: "sink"}
	if err := o.Enqueue(e, []Subscriber{sink}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Dir(o.ackPath(sink.name, e.UUID)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(o.ackPath(sink.name, e.UUID), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	removeFault := outboxOperations{Outbox: o, fs: faultOutboxFS{fail: "remove"}}
	if err := removeFault.recover(); err == nil || !strings.Contains(err.Error(), "injected remove") {
		t.Fatalf("ack cleanup failure must surface for operator recovery: %v", err)
	}
	if err := os.Remove(o.ackPath(sink.name, e.UUID)); err != nil {
		t.Fatal(err)
	}
	openFault := outboxOperations{Outbox: o, fs: faultOutboxFS{fail: "open-file"}}
	if _, err := openFault.replay(context.Background(), []Subscriber{sink}, "", "", 0); err == nil || !strings.Contains(err.Error(), "injected open-file") {
		t.Fatalf("ack publication failure must leave event pending and surface: %v", err)
	}
	removeFault = outboxOperations{Outbox: o, fs: faultOutboxFS{fail: "remove"}}
	if _, err := removeFault.replay(context.Background(), []Subscriber{sink}, "", "", 0); err == nil || !strings.Contains(err.Error(), "injected remove") {
		t.Fatalf("post-ack pending cleanup failure must surface: %v", err)
	}
}

func TestOutbox_ReconciliationViewsExposeCorruptionAndReplayLimits(t *testing.T) {
	o := NewOutbox(t.TempDir())
	if prepared, err := o.Prepared(); err != nil || prepared != nil {
		t.Fatalf("empty outbox prepared view = %#v, %v", prepared, err)
	}
	bad := validEvent()
	bad.UUID = "00000000-0000-4000-8000-000000000020"
	if err := os.MkdirAll(filepath.Dir(o.ledgerPath(bad.UUID)), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(o.ledgerPath(bad.UUID), []byte("{}\n{"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := o.Abort(bad.UUID); err == nil {
		t.Fatal("corrupt record must prevent abort")
	}
	if _, err := o.Prepared(); err == nil {
		t.Fatal("corrupt record must prevent prepared reconciliation")
	}

	root := t.TempDir()
	replay := NewOutbox(root)
	a := validEvent()
	a.UUID = "00000000-0000-4000-8000-000000000021"
	b := validEvent()
	b.UUID = "00000000-0000-4000-8000-000000000022"
	sink := &outboxSub{name: "sink"}
	for _, e := range []Event{a, b} {
		if err := replay.Enqueue(e, []Subscriber{sink}); err != nil {
			t.Fatal(err)
		}
	}
	result, err := replay.Replay(context.Background(), []Subscriber{sink}, "", 1)
	if err != nil || result.Delivered != 1 || result.Pending != 1 {
		t.Fatalf("replay limit must stop after one recipient: %#v, %v", result, err)
	}
}

func TestOutbox_PrepareAndDecideResolveCompetingDurablePublications(t *testing.T) {
	root := t.TempDir()
	e := validEvent()
	record := ledgerRecord{Event: e, Subscribers: []string{"sink"}, State: preparedState}
	data, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	data = append(data, '\n')
	collision := outboxOperations{Outbox: NewOutbox(root), fs: collisionLinkFS{data: data}}
	if err := collision.prepare(e, []Subscriber{&outboxSub{name: "sink"}}); err != nil {
		t.Fatalf("matching competing ledger publication must converge: %v", err)
	}
	if err := collision.decide(e.UUID, committedState); err == nil {
		t.Fatal("competing state publication with a ledger payload must not masquerade as a decision")
	}
	writeFailure := outboxOperations{Outbox: NewOutbox(t.TempDir()), fs: faultOutboxFS{fail: "link"}}
	if err := writeFailure.prepare(e, []Subscriber{&outboxSub{name: "sink"}}); err == nil {
		t.Fatal("non-collision publication failure must surface")
	}
}

func TestOutbox_PrivateOperationsFailClosedAtEveryDurabilityBoundary(t *testing.T) {
	root := t.TempDir()
	e := validEvent()
	sink := &outboxSub{name: "sink"}

	if _, ok := (outboxOperations{Outbox: Outbox{Root: root}}).operations().(osOutboxFS); !ok {
		t.Fatal("nil private filesystem must normalize to the stateless OS implementation")
	}
	if _, ok := (outboxOperations{codec: faultLedgerCodec{}}).ledgerCodec().(faultLedgerCodec); !ok {
		t.Fatal("injected private codec must remain operation scoped")
	}

	marshalFault := outboxOperations{Outbox: NewOutbox(root), codec: faultLedgerCodec{marshalErr: errors.New("injected marshal failure")}}
	if err := marshalFault.prepare(e, []Subscriber{sink}); err == nil || !strings.Contains(err.Error(), "injected marshal") {
		t.Fatalf("marshal failure = %v", err)
	}

	o := NewOutbox(root)
	if err := o.Prepare(e, []Subscriber{sink}); err != nil {
		t.Fatal(err)
	}
	unmarshalFault := outboxOperations{Outbox: o, codec: faultLedgerCodec{unmarshalErr: errors.New("injected unmarshal failure")}}
	if _, err := unmarshalFault.readRecord(e.UUID); err == nil || !strings.Contains(err.Error(), "injected unmarshal") {
		t.Fatalf("unmarshal failure = %v", err)
	}

	missingAfterCollision := outboxOperations{Outbox: NewOutbox(t.TempDir()), fs: absentCollisionLinkFS{}}
	if err := missingAfterCollision.prepare(e, []Subscriber{sink}); err == nil || !os.IsNotExist(err) {
		t.Fatalf("collision without a readable winner = %v", err)
	}

	transitionFault := outboxOperations{Outbox: NewOutbox(t.TempDir()), fs: faultOutboxFS{fail: "link"}}
	record := ledgerRecord{Event: e, Subscribers: []string{sink.name}, State: preparedState}
	if err := transitionFault.commitTransition(e.UUID, record, preparedState); err == nil || !strings.Contains(err.Error(), "injected link") {
		t.Fatalf("commit decision durability failure = %v", err)
	}
	if err := transitionFault.commitTransition(e.UUID, record, "impossible"); err == nil || !strings.Contains(err.Error(), "invalid ledger state") {
		t.Fatalf("invalid private commit state = %v", err)
	}
	if err := transitionFault.abortTransition(e.UUID, "impossible"); err == nil || !strings.Contains(err.Error(), "invalid ledger state") {
		t.Fatalf("invalid private abort state = %v", err)
	}
	if err := transitionFault.decide(e.UUID, committedState); err == nil || !strings.Contains(err.Error(), "injected link") {
		t.Fatalf("non-collision decision write failure = %v", err)
	}
}

func TestOutbox_OperationScopedReadFailuresNeverBecomeDelivery(t *testing.T) {
	root := t.TempDir()
	o := NewOutbox(root)
	e := validEvent()
	e.Artifact.Path = "spec/exists.md"
	sink := &outboxSub{name: "sink"}
	if err := o.Prepare(e, []Subscriber{sink}); err != nil {
		t.Fatal(err)
	}

	statFault := outboxOperations{Outbox: o, fs: faultOutboxFS{fail: "stat"}}
	if err := statFault.recoverRecord(ledgerRecord{Event: e, Subscribers: []string{sink.name}}); err == nil || !strings.Contains(err.Error(), "injected stat") {
		t.Fatalf("ack stat failure = %v", err)
	}
	if _, err := statFault.prepared(); err == nil || !strings.Contains(err.Error(), "injected stat") {
		t.Fatalf("artifact evidence stat failure = %v", err)
	}
	if err := o.Commit(e.UUID); err != nil {
		t.Fatal(err)
	}

	readDirFault := outboxOperations{Outbox: o, fs: faultOutboxFS{fail: "read-dir"}}
	if err := readDirFault.recover(); err == nil || !strings.Contains(err.Error(), "injected read-dir") {
		t.Fatalf("ledger read-dir failure = %v", err)
	}
	if _, err := readDirFault.prepared(); err == nil || !strings.Contains(err.Error(), "injected read-dir") {
		t.Fatalf("prepared read-dir failure = %v", err)
	}

	pendingFault := outboxOperations{Outbox: o, fs: pendingReadDirFaultFS{}}
	if _, err := pendingFault.replay(context.Background(), []Subscriber{sink}, "", "", 0); err == nil || !strings.Contains(err.Error(), "injected pending read-dir") {
		t.Fatalf("pending read-dir failure = %v", err)
	}

	readRecordFault := &sequencedReadFileFS{failAt: 4}
	if _, err := (outboxOperations{Outbox: o, fs: readRecordFault}).replay(context.Background(), []Subscriber{sink}, "", "", 0); err == nil || !strings.Contains(err.Error(), "sequenced read-file") {
		t.Fatalf("pending ledger read failure = %v", err)
	}
	readStateFault := &sequencedReadFileFS{failAt: 5}
	if _, err := (outboxOperations{Outbox: o, fs: readStateFault}).replay(context.Background(), []Subscriber{sink}, "", "", 0); err == nil || !strings.Contains(err.Error(), "sequenced read-file") {
		t.Fatalf("pending state read failure = %v", err)
	}
	for name, failAt := range map[string]int{"cursor-record": 4, "cursor-state": 5, "delivery-state": 8} {
		t.Run(name, func(t *testing.T) {
			fs := &sequencedReadFileFS{failAt: failAt}
			if _, err := (outboxOperations{Outbox: o, fs: fs}).replay(context.Background(), []Subscriber{sink}, sink.name, e.UUID, 0); err == nil || !strings.Contains(err.Error(), "sequenced read-file") {
				t.Fatalf("cursor/state read failure = %v (reads=%d)", err, fs.reads)
			}
		})
	}

	preparedFault := &sequencedReadFileFS{failAt: 3}
	if _, err := (outboxOperations{Outbox: o, fs: preparedFault}).replay(context.Background(), []Subscriber{sink}, "", "", 0); err == nil || !strings.Contains(err.Error(), "sequenced read-file") {
		t.Fatalf("prepared reconciliation read failure = %v", err)
	}

	ackFault := outboxOperations{Outbox: o, fs: ackOpenFileFaultFS{}}
	if _, err := ackFault.replay(context.Background(), []Subscriber{sink}, "", "", 0); err == nil || !strings.Contains(err.Error(), "injected ack open-file") {
		t.Fatalf("ack durability failure = %v", err)
	}
}

func TestOutbox_DirectoryCreationSurfacesRecursionAndSyncFailures(t *testing.T) {
	root := t.TempDir()
	base := Outbox{Root: filepath.Join(root, "base")}
	if err := os.MkdirAll(base.Root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := (outboxOperations{Outbox: base, fs: faultOutboxFS{fail: "stat"}}).writeNewAtomic(filepath.Join(base.Root, "ledger", "entry"), []byte("x")); err == nil || !strings.Contains(err.Error(), "injected stat") {
		t.Fatalf("recursive stat failure = %v", err)
	}
	if err := (outboxOperations{Outbox: base, fs: faultOutboxFS{fail: "open"}}).ensureOutboxDirectory(filepath.Join(base.Root, "new-dir")); err == nil || !strings.Contains(err.Error(), "injected open") {
		t.Fatalf("directory parent sync failure = %v", err)
	}
	if err := (outboxOperations{Outbox: base, fs: missingStatFS{}}).ensureOutboxDirectory("."); !os.IsNotExist(err) {
		t.Fatalf("rootless directory recursion must return its original not-exist error, got %v", err)
	}
	child := filepath.Join(root, "child")
	parentFault := outboxOperations{Outbox: base, fs: childMissingParentFaultFS{child: child}}
	if err := parentFault.ensureOutboxDirectory(child); err == nil || !strings.Contains(err.Error(), "injected parent stat") {
		t.Fatalf("parent directory recursion failure = %v", err)
	}
}
