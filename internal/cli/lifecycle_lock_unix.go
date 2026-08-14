//go:build !windows

package cli

import (
	"errors"
	"os"
	"syscall"

	"github.com/specscore/specscore-cli/pkg/exitcode"
)

func acquireLifecycleFileLock(file *os.File) error {
	err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return errLifecycleLockHeld
	}
	return err
}

func releaseLifecycleLockedFile(lockPath string, lockFile *os.File) error {
	// Unlinking by pathname before or after close can delete a replacement lock
	// file opened by another process. Keep the benign stable pathname on every
	// platform; acquisition takes an OS lock on the opened inode, never trusts
	// pathname absence.
	if err := transactionCloseFile(lockFile); err != nil {
		return exitcode.UnexpectedErrorf("closing lifecycle transaction lock %s: %v", lockPath, err)
	}
	return nil
}
