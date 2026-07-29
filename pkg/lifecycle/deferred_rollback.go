package lifecycle

import "errors"

// MutationReadyHook records exact lifecycle-owned state immediately before a
// lifecycle package invokes its post-mutation integration hook.
type MutationReadyHook func() error

// DeferredRollbackError tells a lifecycle package that its caller has already
// established an outer transaction that owns rollback. The package must return
// the original failure without restoring its artifact itself: the outer layer
// can then compare the current filesystem state with its captured post-mutation
// state and avoid overwriting an uncoordinated writer.
type DeferredRollbackError struct {
	cause error
}

func (err *DeferredRollbackError) Error() string { return err.cause.Error() }

func (err *DeferredRollbackError) Unwrap() error { return err.cause }

// DeferRollback marks a non-nil post-mutation failure for an outer
// conflict-aware transaction. Nil remains nil so callers can wrap a hook result
// without a special success branch.
func DeferRollback(err error) error {
	if err == nil {
		return nil
	}
	var deferred *DeferredRollbackError
	if errors.As(err, &deferred) {
		return err
	}
	return &DeferredRollbackError{cause: err}
}

// IsDeferredRollback reports whether err was marked by DeferRollback.
func IsDeferredRollback(err error) bool {
	var deferred *DeferredRollbackError
	return errors.As(err, &deferred)
}
