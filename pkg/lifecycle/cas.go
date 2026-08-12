package lifecycle

import (
	"bytes"
	"errors"
	"os"
)

// ErrConcurrentMutation means the artifact changed after its caller read the
// exact bytes it intended to amend. Callers must re-read and make a fresh,
// explicit decision rather than overwrite the other writer's record.
var ErrConcurrentMutation = errors.New("lifecycle: artifact changed before amendment write")

// CompareAndSwap atomically replaces path only when it still contains before.
// It is deliberately byte based: lifecycle annotations must never silently
// merge a concurrent human edit with a stale parse of the artifact.
func CompareAndSwap(path string, before, after []byte) error {
	current, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	if !bytes.Equal(current, before) {
		return ErrConcurrentMutation
	}
	return writeFileAtomic(path, after)
}
