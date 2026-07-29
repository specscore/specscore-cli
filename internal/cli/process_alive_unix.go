//go:build !windows

package cli

import (
	"os"
	"syscall"
)

func defaultProcessAlive(pid int) bool {
	process, err := os.FindProcess(pid)
	return err == nil && process.Signal(syscall.Signal(0)) == nil
}
