//go:build !windows

package cli

import (
	"errors"
	"os"
	"syscall"
)

func acquireLifecycleFileLock(file *os.File) error {
	err := syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
	if errors.Is(err, syscall.EWOULDBLOCK) || errors.Is(err, syscall.EAGAIN) {
		return errLifecycleLockHeld
	}
	return err
}
