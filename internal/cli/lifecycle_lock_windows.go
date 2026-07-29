//go:build windows

package cli

import (
	"errors"
	"os"

	"github.com/specscore/specscore-cli/pkg/exitcode"
	"golang.org/x/sys/windows"
)

func acquireLifecycleFileLock(file *os.File) error {
	err := windows.LockFileEx(
		windows.Handle(file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0,
		1,
		0,
		&windows.Overlapped{},
	)
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return errLifecycleLockHeld
	}
	return err
}

// Windows refuses to unlink a file while the locking handle is open unless it
// was opened with delete sharing. Release the byte-range lock and close first;
// if a concurrent opener wins the pathname race, a leftover unlocked file is
// harmless because acquisition always takes an OS lock before proceeding.
func releaseLifecycleLockedFile(lockPath string, lockFile *os.File) error {
	handle := windows.Handle(lockFile.Fd())
	unlockErr := windows.UnlockFileEx(handle, 0, 1, 0, &windows.Overlapped{})
	closeErr := transactionCloseFile(lockFile)
	if closeErr != nil {
		if unlockErr != nil {
			return exitcode.UnexpectedErrorf("unlocking lifecycle transaction lock %s: %v; additionally closing it failed: %v", lockPath, unlockErr, closeErr)
		}
		return exitcode.UnexpectedErrorf("closing lifecycle transaction lock %s: %v", lockPath, closeErr)
	}
	if unlockErr != nil {
		return exitcode.UnexpectedErrorf("unlocking lifecycle transaction lock %s: %v", lockPath, unlockErr)
	}
	if err := transactionRemove(lockPath); err != nil && !os.IsNotExist(err) {
		// The handle is closed and the lock released. Do not turn an otherwise
		// completed command into a failure merely because a benign stale file
		// remains on a Windows filesystem with delayed delete semantics.
		return nil
	}
	return nil
}
