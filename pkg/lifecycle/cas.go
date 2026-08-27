package lifecycle

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

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

// RecoveryRequiredError reports a fail-closed pre-publication state. The
// requested postimage was not published, but a durable preimage receipt was
// retained because a non-cooperating writer or filesystem fault prevented a
// safe restoration. Callers must preserve both paths for explicit recovery.
type RecoveryRequiredError struct {
	Path         string
	RecoveryPath string
	Phase        string
	Err          error
}

func (e *RecoveryRequiredError) Error() string {
	return fmt.Sprintf("artifact transaction at %s requires recovery from %s after %s: %v", e.Path, e.RecoveryPath, e.Phase, e.Err)
}

func (e *RecoveryRequiredError) Unwrap() error { return e.Err }

func recoveryRequired(path, recoveryPath, phase string, err error) error {
	return &RecoveryRequiredError{Path: path, RecoveryPath: recoveryPath, Phase: phase, Err: err}
}

// ArtifactTransaction is the one existing-artifact mutation boundary. Before
// is read only after the path lock is acquired. Commit may be called at most
// once and performs the final expected-byte check, atomic replacement, and
// directory durability fence without reacquiring the lock.
type ArtifactTransaction struct {
	path      string
	before    []byte
	committed bool
	ops       artifactTransactionOps
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
	if err := writeFileAtomicExpectedWithOps(t.path, t.before, after, t.ops); err != nil {
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
	return withArtifactTransactionOps(path, defaultArtifactTransactionOps(), fn)
}

func withArtifactTransactionOps(path string, ops artifactTransactionOps, fn func(*ArtifactTransaction) error) error {
	lock, err := acquireArtifactLockWithOps(path, ops)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Unlock() }()
	before, err := ops.readFile(path)
	if err != nil {
		return err
	}
	tx := &ArtifactTransaction{path: path, before: before, ops: ops}
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
	return transformArtifactWithOps(path, defaultArtifactTransactionOps(), transform)
}

func transformArtifactWithOps(path string, ops artifactTransactionOps, transform func(before []byte) (after []byte, err error)) error {
	return withArtifactTransactionOps(path, ops, func(tx *ArtifactTransaction) error {
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

// artifactTransactionLock owns both the project lock and the per-artifact
// lock. The project lock serializes unlinking the per-artifact pathname with
// the next acquisition, so clean transactions can remove their zero-byte
// identity without opening a delete-and-recreate race. The project lock's
// stable pathname is retained for crash recovery; the artifact lock is
// removed only after its OS lock has been released.
type artifactTransactionLock struct {
	project      artifactLock
	artifact     artifactLock
	artifactPath string
	remove       func(string) error
	processState *lifecycleProcessLockState
	once         sync.Once
	unlockErr    error
}

func (l *artifactTransactionLock) TryLock() (bool, error) {
	return false, errors.New("lifecycle: transaction lock cannot be reacquired")
}

func (l *artifactTransactionLock) Unlock() error {
	l.once.Do(func() {
		var errs []error
		if err := l.artifact.Unlock(); err != nil {
			errs = append(errs, err)
		} else if err := l.remove(l.artifactPath); err != nil && !os.IsNotExist(err) {
			errs = append(errs, fmt.Errorf("removing lifecycle transaction lock %s: %w", l.artifactPath, err))
		}
		if err := l.project.Unlock(); err != nil {
			errs = append(errs, err)
		}
		l.processState.mu.Lock()
		l.processState.active.Delete(l.artifactPath)
		l.processState.mu.Unlock()
		l.processState.gate <- struct{}{}
		l.unlockErr = errors.Join(errs...)
	})
	return l.unlockErr
}

func newFlockArtifactLock(path string) artifactLock { return flock.New(path) }

func acquireArtifactLock(path string) (artifactLock, error) {
	return acquireArtifactLockWithOps(path, defaultArtifactTransactionOps())
}

func acquireArtifactLockWithOps(path string, ops artifactTransactionOps) (artifactLock, error) {
	artifactPath := filepath.Join(dirOf(path), "."+filepath.Base(path)+".lifecycle-transaction.lock")
	projectPath := filepath.Join(lifecycleLockRoot(path), ".specscore-lifecycle.lock")
	// Some Unix implementations scope flock contention differently for
	// independent descriptors in one process. A process guard makes the
	// cross-artifact serialization contract deterministic there as well as
	// across processes (where projectPath's flock is authoritative). Return
	// immediately for a same-process re-entry on the same artifact: callers
	// rely on the bounded ErrConcurrentMutation result rather than a deadlock.
	processState := &lifecycleProcessState
	processState.mu.Lock()
	if processState.active.Contains(artifactPath) {
		processState.mu.Unlock()
		return nil, ErrConcurrentMutation
	}
	processState.mu.Unlock()
	<-processState.gate
	projectLock := ops.newLock(projectPath)
	locked, err := projectLock.TryLock()
	if err != nil {
		processState.gate <- struct{}{}
		return nil, err
	}
	if !locked {
		processState.gate <- struct{}{}
		return nil, ErrConcurrentMutation
	}

	artifactLock := ops.newLock(artifactPath)
	locked, err = artifactLock.TryLock()
	if err != nil {
		_ = projectLock.Unlock()
		processState.gate <- struct{}{}
		return nil, err
	}
	if !locked {
		_ = projectLock.Unlock()
		processState.gate <- struct{}{}
		return nil, ErrConcurrentMutation
	}
	processState.mu.Lock()
	processState.active.Add(artifactPath)
	processState.mu.Unlock()
	return &artifactTransactionLock{
		project:      projectLock,
		artifact:     artifactLock,
		artifactPath: artifactPath,
		remove:       ops.remove,
		processState: processState,
	}, nil
}

type lifecycleProcessLockState struct {
	mu     sync.Mutex
	active lifecycleArtifactPathSet
	gate   chan struct{}
}

type lifecycleArtifactPathSet map[string]struct{}

func (s lifecycleArtifactPathSet) Contains(path string) bool {
	_, ok := s[path]
	return ok
}

func (s lifecycleArtifactPathSet) Add(path string) {
	s[path] = struct{}{}
}

func (s lifecycleArtifactPathSet) Delete(path string) {
	delete(s, path)
}

var lifecycleProcessState = lifecycleProcessLockState{
	active: make(lifecycleArtifactPathSet),
	gate:   lifecycleProcessGate(),
}

func lifecycleProcessGate() chan struct{} {
	gate := make(chan struct{}, 1)
	gate <- struct{}{}
	return gate
}

// lifecycleLockRoot returns the nearest project or staged-transaction root.
// A staged tree may be nested inside a project that already holds its stable
// project lock, so choosing the nearest root keeps nested artifact writes
// independent while live-project writers still coordinate through the same
// .specscore-lifecycle.lock pathname.
func lifecycleLockRoot(path string) string {
	abs, err := lifecycleLockAbs(path)
	if err != nil {
		return dirOf(path)
	}
	dir := filepath.Dir(abs)
	for {
		base := filepath.Base(dir)
		if strings.HasPrefix(base, ".specscore-txn-") {
			return dir
		}
		// Test fixtures and partially initialized projects may have no git
		// metadata or specscore.yaml yet. The conventional spec/ boundary is
		// still enough to keep one stable project fence above every artifact.
		if base == "spec" {
			return filepath.Dir(dir)
		}
		if fileOrDirExists(filepath.Join(dir, "specscore.yaml")) || fileOrDirExists(filepath.Join(dir, ".git")) {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return filepath.Dir(abs)
		}
		dir = parent
	}
}

var lifecycleLockAbs = filepath.Abs

func fileOrDirExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}
