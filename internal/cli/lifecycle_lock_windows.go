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
// was opened with delete sharing. Release the byte-range lock and close it, but
// deliberately retain the stable unlocked pathname.  Removing it after close
// would race a successor that acquired and locked a new file at the same path.
// Acquisition always locks the opened handle, so a retained unlocked file is
// harmless and is the only safe cross-process release behaviour here.
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
	return nil
}
