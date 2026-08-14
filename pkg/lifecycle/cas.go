package lifecycle

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

// ErrConcurrentMutation means the artifact changed after its caller read the
// exact bytes it intended to amend. Callers must re-read and make a fresh,
// explicit decision rather than overwrite the other writer's record.
var ErrConcurrentMutation = errors.New("lifecycle: artifact changed before amendment write")

// CommittedMutationError reports that the canonical artifact bytes are already
// visible and must be retained even though later durable-fence or derived work
// failed. Callers must surface recovery-required state; they must never roll the
// artifact back from a stale snapshot.
type CommittedMutationError struct {
	Path  string
	Phase string
	Err   error
}

func (e *CommittedMutationError) Error() string {
	return fmt.Sprintf("artifact transaction at %s committed; recovery required after %s: %v", e.Path, e.Phase, e.Err)
}

func (e *CommittedMutationError) Unwrap() error { return e.Err }

// CommittedError constructs a typed recovery-required error for work that
// failed only after an artifact transaction became visible.
func CommittedError(path, phase string, err error) error {
	if err == nil {
		return nil
	}
	return &CommittedMutationError{Path: path, Phase: phase, Err: err}
}

// ArtifactTransaction is the one existing-artifact mutation boundary. Before
// is read only after the path lock is acquired. Commit may be called at most
// once and performs the final expected-byte check, atomic replacement, and
// directory durability fence without reacquiring the lock.
type ArtifactTransaction struct {
	path      string
	before    []byte
	committed bool
}

func (t *ArtifactTransaction) Before() []byte { return append([]byte(nil), t.before...) }

func (t *ArtifactTransaction) Commit(after []byte) error {
	if t.committed {
		return errors.New("lifecycle: artifact transaction already committed")
	}
	if bytes.Equal(t.before, after) {
		t.committed = true
		return nil
	}
	if err := writeFileAtomicExpected(t.path, t.before, after); err != nil {
		return err
	}
	t.committed = true
	return nil
}

// WithArtifactTransaction locks path, reads its exact bytes, and invokes fn.
// It never waits: contention is immediately ErrConcurrentMutation. The callback
// may perform pre-publication work for a classified new-artifact publisher,
// then commit the board/index bytes through tx.Commit; ordinary writers should
// use TransformArtifact's pure callback.
func WithArtifactTransaction(path string, fn func(*ArtifactTransaction) error) error {
	lock, err := acquireArtifactLock(path)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Unlock() }()
	before, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	tx := &ArtifactTransaction{path: path, before: before}
	if err := fn(tx); err != nil {
		var committed *CommittedMutationError
		if tx.committed && !errors.As(err, &committed) {
			return CommittedError(path, "post-commit transaction work", err)
		}
		return err
	}
	return nil
}

// TransformArtifact runs one complete existing-artifact transaction: acquire
// the per-artifact lifecycle fence, read the exact current bytes, resolve and
// validate against those bytes in transform, then atomically replace them when
// transform returns changed bytes. Transform must be pure: it must not call a
// public lifecycle writer (which would acquire the same non-reentrant lock).
func TransformArtifact(path string, transform func(before []byte) (after []byte, err error)) error {
	return WithArtifactTransaction(path, func(tx *ArtifactTransaction) error {
		before := tx.Before()
		after, err := transform(before)
		if err != nil {
			return err
		}
		return tx.Commit(after)
	})
}

type artifactLock interface {
	TryLock() (bool, error)
	Unlock() error
}

var newArtifactLock = func(path string) artifactLock { return flock.New(path) }

func acquireArtifactLock(path string) (artifactLock, error) {
	lock := newArtifactLock(filepath.Join(dirOf(path), "."+filepath.Base(path)+".lifecycle-transaction.lock"))
	locked, err := lock.TryLock()
	if err != nil {
		return nil, err
	}
	if !locked {
		return nil, ErrConcurrentMutation
	}
	return lock, nil
}
