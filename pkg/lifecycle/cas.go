package lifecycle

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"
)

// ErrConcurrentMutation means the artifact changed after its caller read the
// exact bytes it intended to amend. Callers must re-read and make a fresh,
// explicit decision rather than overwrite the other writer's record.
var ErrConcurrentMutation = errors.New("lifecycle: artifact changed before amendment write")

// CompareAndSwap replaces path only when it still contains before. A sibling
// non-blocking advisory lock makes the read/compare/rename sequence one critical section for
// every SpecScore lifecycle writer. The lock is intentionally adjacent to the
// artifact (rather than process-global), so unrelated artifacts can progress.
// A concurrent lifecycle writer receives ErrConcurrentMutation and must re-read
// before deciding whether its amendment is still truthful.
func CompareAndSwap(path string, before, after []byte) error {
	lock, err := acquireCASLock(path)
	if err != nil {
		return err
	}
	defer func() { _ = lock.Unlock() }()
	current, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, before) {
		return ErrConcurrentMutation
	}
	return writeFileAtomic(path, after)
}

type casLock interface {
	TryLock() (bool, error)
	Unlock() error
}

var newCASLock = func(path string) casLock { return flock.New(path) }

func acquireCASLock(path string) (casLock, error) {
	lock := newCASLock(filepath.Join(dirOf(path), "."+filepath.Base(path)+".lifecycle-cas.lock"))
	locked, err := lock.TryLock()
	if err != nil {
		return nil, err
	}
	if !locked {
		return nil, ErrConcurrentMutation
	}
	return lock, nil
}
