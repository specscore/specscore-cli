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
	removeErr := transactionRemove(lockPath)
	closeErr := transactionCloseFile(lockFile)
	if removeErr != nil && !os.IsNotExist(removeErr) {
		if closeErr != nil {
			return exitcode.UnexpectedErrorf("releasing lifecycle transaction lock %s: %v; additionally closing it failed: %v", lockPath, removeErr, closeErr)
		}
		return exitcode.UnexpectedErrorf("releasing lifecycle transaction lock %s: %v", lockPath, removeErr)
	}
	if closeErr != nil {
		return exitcode.UnexpectedErrorf("closing lifecycle transaction lock %s: %v", lockPath, closeErr)
	}
	return nil
}
